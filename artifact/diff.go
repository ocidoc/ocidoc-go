// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: Copyright 2026 WoozyMasta
// Source: github.com/ocidoc/ocidoc-go

package artifact

import (
	"context"
	"fmt"
	"slices"
	"sort"

	"github.com/opencontainers/go-digest"

	"github.com/ocidoc/ocidoc-go/spec"
)

// AnnotationDiff is one root manifest annotation key that differs between the two compared artifacts.
// Before or After is empty when the key is absent on that side.
type AnnotationDiff struct {
	// Key is the manifest annotation name.
	Key string

	// Before is the first artifact's value, or empty when absent.
	Before string

	// After is the second artifact's value, or empty when absent.
	After string
}

// ComponentPresence classifies whether a compared component exists on both sides.
type ComponentPresence int

const (
	// ComponentPresent means the component exists on both sides
	// (its digest, entrypoint or files may still differ).
	ComponentPresent ComponentPresence = iota

	// ComponentAdded means the component exists only in the second (after) artifact.
	ComponentAdded

	// ComponentRemoved means the component exists only in the first (before) artifact.
	ComponentRemoved
)

// FileChange classifies one path's change within a component.
type FileChange int

const (
	// FileAdded means the path exists only in the second (after) side.
	FileAdded FileChange = iota

	// FileRemoved means the path exists only in the first (before) side.
	FileRemoved

	// FileModified means the path exists on both sides with a different size.
	// File-level diffing works from tar headers alone (see List),
	// so a same-size content change without a size change cannot be detected this way;
	// that case is still caught at the component level by DigestChanged.
	FileModified
)

// FileDiff is one path that differs within a component's file-level diff.
// SizeBefore or SizeAfter is zero when the path does not exist on that side.
type FileDiff struct {
	// Path is the bundle-relative file path.
	Path string

	// Change classifies the file's presence or size change.
	Change FileChange

	// SizeBefore is the first artifact's tar-header size.
	SizeBefore int64

	// SizeAfter is the second artifact's tar-header size.
	SizeAfter int64
}

// ComponentDiff is one component type that differs between the two compared artifacts.
type ComponentDiff struct {
	// Component is the semantic component type.
	Component spec.ComponentType

	// DigestBefore is the first artifact's component digest.
	DigestBefore digest.Digest

	// DigestAfter is the second artifact's component digest.
	DigestAfter digest.Digest

	// EntrypointBefore is the first artifact's configured entrypoint.
	EntrypointBefore string

	// EntrypointAfter is the second artifact's configured entrypoint.
	EntrypointAfter string

	// Files is the component's file-level diff: populated when the/ component was added,
	// removed or DigestChanged, unless opts.MetadataOnly is set; nil otherwise.
	// A component that changed only by EntrypointChanged never gets a file-level diff -
	// an entrypoint is an artifact config fact, independent of the component's actual content.
	Files []FileDiff

	// Presence reports whether the component exists in both artifacts or on one side only.
	Presence ComponentPresence

	// DigestChanged is true when the component exists on both sides with a different descriptor digest.
	// DigestBefore/DigestAfter are the zero digest.Digest on the side where the component is absent.
	DigestChanged bool

	// EntrypointChanged is true when the component's configured entrypoint
	// differs between the two artifact configs.
	EntrypointChanged bool
}

// DiffOptions controls Diff's depth and scope.
type DiffOptions struct {
	// Component restricts the comparison to one component.
	// Errors with errors.Is(err, spec.ErrNotFound) if neither artifact has it.
	Component spec.ComponentType

	// MetadataOnly stops at component descriptor digests:
	// it never opens a component blob, so Files is always nil on every ComponentDiff.
	MetadataOnly bool
}

// DiffResult is Diff's comparison report.
// Equal is true when nothing at all differs; Components lists only the components that differ.
type DiffResult struct {

	// SchemaVersionBefore is the first artifact configuration's schema version.
	// It is empty when both artifact configurations use the same version.
	SchemaVersionBefore string

	// SchemaVersionAfter is the second artifact configuration's schema version.
	// It is empty when both artifact configurations use the same version.
	SchemaVersionAfter string

	// Annotations lists changed root manifest annotations.
	Annotations []AnnotationDiff

	// Components lists components with a detected difference.
	Components []ComponentDiff

	// Equal reports whether no compared metadata or component differs.
	Equal bool
}

// Diff compares two already-open artifacts: root manifest annotations,
// artifact config schemaVersion, component presence, component descriptor digests
// and, for components that were added, removed or whose digest changed,
// file-level changes read from tar headers alone (see List; no file content is ever read).
//
// Component presence and digest comparisons never open a component blob;
// only the file-level stage does, and it is skipped entirely when opts.MetadataOnly is set.
func Diff(ctx context.Context, a, b Reader, opts DiffOptions) (*DiffResult, error) {
	manifestA, err := a.Manifest(ctx)
	if err != nil {
		return nil, err
	}

	manifestB, err := b.Manifest(ctx)
	if err != nil {
		return nil, err
	}

	cfgA, err := a.Config(ctx)
	if err != nil {
		return nil, err
	}

	cfgB, err := b.Config(ctx)
	if err != nil {
		return nil, err
	}

	componentsA, err := a.Components(ctx)
	if err != nil {
		return nil, err
	}

	componentsB, err := b.Components(ctx)
	if err != nil {
		return nil, err
	}

	if opts.Component != "" {
		componentsA = filterComponents(componentsA, opts.Component)
		componentsB = filterComponents(componentsB, opts.Component)

		if len(componentsA) == 0 && len(componentsB) == 0 {
			return nil, fmt.Errorf("%w: component %q", spec.ErrNotFound, opts.Component)
		}
	}

	result := &DiffResult{
		Annotations: diffAnnotations(manifestA.Annotations, manifestB.Annotations),
	}

	if cfgA.SchemaVersion != cfgB.SchemaVersion {
		result.SchemaVersionBefore = cfgA.SchemaVersion
		result.SchemaVersionAfter = cfgB.SchemaVersion
	}

	components, err := diffComponents(ctx, a, b, cfgA, cfgB, componentsA, componentsB, opts)
	if err != nil {
		return nil, err
	}

	result.Components = components

	result.Equal = len(result.Annotations) == 0 && result.SchemaVersionBefore == "" && len(result.Components) == 0

	return result, nil
}

// diffAnnotations returns one AnnotationDiff per key present in before
// or after with a different (or one-sided) value, sorted by key.
func diffAnnotations(before, after map[string]string) []AnnotationDiff {
	keys := unionKeys(before, after)

	var diffs []AnnotationDiff

	for _, k := range keys {
		v1, v2 := before[k], after[k]
		if v1 == v2 {
			continue
		}

		diffs = append(diffs, AnnotationDiff{Key: k, Before: v1, After: v2})
	}

	return diffs
}

// unionKeys returns the sorted union of a's and b's keys.
func unionKeys[V any](a, b map[string]V) []string {
	seen := make(map[string]bool, len(a)+len(b))

	for k := range a {
		seen[k] = true
	}

	for k := range b {
		seen[k] = true
	}

	keys := make([]string, 0, len(seen))
	for k := range seen {
		keys = append(keys, k)
	}

	sort.Strings(keys)

	return keys
}

// diffComponents compares componentsA against componentsB,
// one entry per component type present on either side, reading file-level changes per opts.
func diffComponents(
	ctx context.Context,
	a, b Reader,
	cfgA, cfgB *spec.ArtifactConfig,
	componentsA, componentsB []ComponentDescriptor,
	opts DiffOptions,
) ([]ComponentDiff, error) {
	mapA := componentMap(componentsA)
	mapB := componentMap(componentsB)

	var results []ComponentDiff

	for _, t := range unionComponentTypes(mapA, mapB) {
		descA, inA := mapA[t]
		descB, inB := mapB[t]

		cd := ComponentDiff{Component: t}

		switch {
		case inA && !inB:
			cd.Presence = ComponentRemoved
			cd.DigestBefore = descA.Descriptor.Digest
		case !inA && inB:
			cd.Presence = ComponentAdded
			cd.DigestAfter = descB.Descriptor.Digest
		default:
			cd.Presence = ComponentPresent
			cd.DigestBefore = descA.Descriptor.Digest
			cd.DigestAfter = descB.Descriptor.Digest
			cd.DigestChanged = descA.Descriptor.Digest != descB.Descriptor.Digest
		}

		epA, epB := cfgA.Components[t].Entrypoint, cfgB.Components[t].Entrypoint
		if epA != epB {
			cd.EntrypointChanged = true
			cd.EntrypointBefore = epA
			cd.EntrypointAfter = epB
		}

		contentChanged := cd.Presence != ComponentPresent || cd.DigestChanged
		reportWorthy := contentChanged || cd.EntrypointChanged

		if !reportWorthy {
			continue
		}

		if !opts.MetadataOnly && contentChanged {
			files, err := diffComponentFiles(ctx, a, b, descA, descB, inA, inB)
			if err != nil {
				return nil, fmt.Errorf("component %q: %w", t, err)
			}

			cd.Files = files
		}

		results = append(results, cd)
	}

	return results, nil
}

// componentMap indexes components by type.
func componentMap(components []ComponentDescriptor) map[spec.ComponentType]ComponentDescriptor {
	m := make(map[spec.ComponentType]ComponentDescriptor, len(components))
	for _, c := range components {
		m[c.Type] = c
	}

	return m
}

// unionComponentTypes returns the sorted union of a's and b's keys.
func unionComponentTypes(a, b map[spec.ComponentType]ComponentDescriptor) []spec.ComponentType {
	seen := make(map[spec.ComponentType]bool, len(a)+len(b))

	for t := range a {
		seen[t] = true
	}

	for t := range b {
		seen[t] = true
	}

	types := make([]spec.ComponentType, 0, len(seen))
	for t := range seen {
		types = append(types, t)
	}

	slices.Sort(types)

	return types
}

// diffComponentFiles compares one component's file lists
// (tar headers only, via List's underlying listComponentFiles - no file content is read),
// returning one FileDiff per path added, removed or changed in size.
// descA/descB are only read when inA/inB is true.
func diffComponentFiles(
	ctx context.Context,
	a, b Reader,
	descA, descB ComponentDescriptor,
	inA, inB bool,
) ([]FileDiff, error) {
	sizesA, err := componentFileSizes(ctx, a, descA, inA)
	if err != nil {
		return nil, err
	}

	sizesB, err := componentFileSizes(ctx, b, descB, inB)
	if err != nil {
		return nil, err
	}

	var diffs []FileDiff

	for _, p := range unionKeys(sizesA, sizesB) {
		sizeA, existsA := sizesA[p]
		sizeB, existsB := sizesB[p]

		switch {
		case existsA && !existsB:
			diffs = append(diffs, FileDiff{Path: p, Change: FileRemoved, SizeBefore: sizeA})
		case !existsA && existsB:
			diffs = append(diffs, FileDiff{Path: p, Change: FileAdded, SizeAfter: sizeB})
		case sizeA != sizeB:
			diffs = append(diffs, FileDiff{Path: p, Change: FileModified, SizeBefore: sizeA, SizeAfter: sizeB})
		}
	}

	return diffs, nil
}

// componentFileSizes lists c's files (nil if present is false) as a map from path to size.
func componentFileSizes(ctx context.Context, r Reader, c ComponentDescriptor, present bool) (map[string]int64, error) {
	if !present {
		return nil, nil
	}

	files, err := listComponentFiles(ctx, r, c)
	if err != nil {
		return nil, err
	}

	sizes := make(map[string]int64, len(files))
	for _, f := range files {
		sizes[f.Path] = f.Size
	}

	return sizes, nil
}
