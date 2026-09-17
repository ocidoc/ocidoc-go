// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: Copyright 2026 WoozyMasta
// Source: github.com/ocidoc/ocidoc-go

package artifact

import (
	"context"
	"fmt"
	"maps"
	"reflect"
	"slices"
	"sort"

	"github.com/opencontainers/go-digest"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"

	"github.com/ocidoc/ocidoc-go/internal/archive"
	"github.com/ocidoc/ocidoc-go/spec"
)

// AnnotationDiff describes one changed root manifest annotation.
type AnnotationDiff struct {
	// Key is the manifest annotation name.
	Key string `json:"key" yaml:"key"`

	// Before is the first artifact's value, or empty when absent.
	Before string `json:"before" yaml:"before"`

	// After is the second artifact's value, or empty when absent.
	After string `json:"after" yaml:"after"`

	// BeforeSet reports whether the annotation exists in the first artifact.
	BeforeSet bool `json:"beforeSet" yaml:"beforeSet"`

	// AfterSet reports whether the annotation exists in the second artifact.
	AfterSet bool `json:"afterSet" yaml:"afterSet"`
}

// LocaleDiff describes one changed locale membership set.
type LocaleDiff struct {
	// Component is the component whose locale membership changed.
	Component spec.ComponentType `json:"component" yaml:"component"`

	// Locale is the opaque locale key.
	Locale string `json:"locale" yaml:"locale"`

	// EntrypointBefore is the resolved locale entrypoint in the first artifact.
	EntrypointBefore string `json:"entrypointBefore,omitempty" yaml:"entrypointBefore,omitempty"`

	// EntrypointAfter is the resolved locale entrypoint in the second artifact.
	EntrypointAfter string `json:"entrypointAfter,omitempty" yaml:"entrypointAfter,omitempty"`

	// Before contains sorted bundle-relative document paths in the first artifact.
	Before []string `json:"before,omitempty" yaml:"before,omitempty"`

	// After contains sorted bundle-relative document paths in the second artifact.
	After []string `json:"after,omitempty" yaml:"after,omitempty"`

	// BeforeSet reports whether the locale exists in the first artifact.
	BeforeSet bool `json:"beforeSet" yaml:"beforeSet"`

	// AfterSet reports whether the locale exists in the second artifact.
	AfterSet bool `json:"afterSet" yaml:"afterSet"`

	// DefaultBefore reports whether this locale is the component fallback in the first artifact.
	DefaultBefore bool `json:"defaultBefore" yaml:"defaultBefore"`

	// DefaultAfter reports whether this locale is the component fallback in the second artifact.
	DefaultAfter bool `json:"defaultAfter" yaml:"defaultAfter"`
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

	// FileModified means the path exists on both sides with different content.
	FileModified
)

// FileDiff is one path that differs within a component's file-level diff.
// SizeBefore or SizeAfter is zero when the path does not exist on that side.
type FileDiff struct {
	// Path is the bundle-relative file path.
	Path string `json:"path" yaml:"path"`

	// Change classifies the file's presence or content change.
	Change FileChange `json:"change" yaml:"change"`

	// SizeBefore is the first artifact's tar-header size.
	SizeBefore int64 `json:"sizeBefore" yaml:"sizeBefore"`

	// SizeAfter is the second artifact's tar-header size.
	SizeAfter int64 `json:"sizeAfter" yaml:"sizeAfter"`
}

// ComponentDiff is one component type that differs between the two compared artifacts.
type ComponentDiff struct {
	// Component is the semantic component type.
	Component spec.ComponentType `json:"component" yaml:"component"`

	// DigestBefore is the first artifact's component digest.
	DigestBefore digest.Digest `json:"digestBefore" yaml:"digestBefore"`

	// DigestAfter is the second artifact's component digest.
	DigestAfter digest.Digest `json:"digestAfter" yaml:"digestAfter"`

	// EntrypointBefore is the first artifact's configured entrypoint.
	EntrypointBefore string `json:"entrypointBefore" yaml:"entrypointBefore"`

	// EntrypointAfter is the second artifact's configured entrypoint.
	EntrypointAfter string `json:"entrypointAfter" yaml:"entrypointAfter"`

	// Files is the component's file-level diff.
	// It is populated when the component was added, removed or DigestChanged, unless opts.MetadataOnly is set.
	// A component that changed only by EntrypointChanged has no file-level diff
	// because an entrypoint is an artifact config fact independent of its content.
	Files []FileDiff `json:"files,omitempty" yaml:"files,omitempty"`

	// Presence reports whether the component exists in both artifacts or on one side only.
	Presence ComponentPresence `json:"presence" yaml:"presence"`

	// DigestChanged is true when the component exists on both sides with a different descriptor digest.
	// DigestBefore/DigestAfter are the zero digest.Digest on the side where the component is absent.
	DigestChanged bool `json:"digestChanged" yaml:"digestChanged"`

	// EntrypointChanged is true when the component's configured entrypoint
	// differs between the two artifact configs.
	EntrypointChanged bool `json:"entrypointChanged" yaml:"entrypointChanged"`
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
	SchemaVersionBefore string `json:"schemaVersionBefore,omitempty" yaml:"schemaVersionBefore,omitempty"`

	// SchemaVersionAfter is the second artifact configuration's schema version.
	// It is empty when both artifact configurations use the same version.
	SchemaVersionAfter string `json:"schemaVersionAfter,omitempty" yaml:"schemaVersionAfter,omitempty"`

	// SubjectBefore is the root manifest subject in the first artifact.
	// A nil value means that the manifest did not contain a subject.
	SubjectBefore *ocispec.Descriptor `json:"subjectBefore,omitempty" yaml:"subjectBefore,omitempty"`

	// SubjectAfter is the root manifest subject in the second artifact.
	// A nil value means that the manifest did not contain a subject.
	SubjectAfter *ocispec.Descriptor `json:"subjectAfter,omitempty" yaml:"subjectAfter,omitempty"`

	// Annotations lists changed root manifest annotations.
	Annotations []AnnotationDiff `json:"annotations,omitempty" yaml:"annotations,omitempty"`

	// Components lists components with a detected difference.
	Components []ComponentDiff `json:"components,omitempty" yaml:"components,omitempty"`

	// Locales lists changed resolved locale membership sets.
	Locales []LocaleDiff `json:"locales,omitempty" yaml:"locales,omitempty"`

	// Equal reports whether no compared metadata or component differs.
	Equal bool `json:"equal" yaml:"equal"`
}

// Diff compares two already-open artifacts: root manifest annotations,
// artifact config schemaVersion, component presence, component descriptor digests
// and, for components that were added, removed or whose digest changed,
// file-level changes including SHA-256 content comparisons.
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
	if !reflect.DeepEqual(manifestA.Subject, manifestB.Subject) {
		result.SubjectBefore = cloneDescriptor(manifestA.Subject)
		result.SubjectAfter = cloneDescriptor(manifestB.Subject)
	}

	if cfgA.SchemaVersion != cfgB.SchemaVersion {
		result.SchemaVersionBefore = cfgA.SchemaVersion
		result.SchemaVersionAfter = cfgB.SchemaVersion
	}
	result.Locales = diffLocales(cfgA.Components, cfgB.Components)

	components, err := diffComponents(ctx, a, b, cfgA, cfgB, componentsA, componentsB, opts)
	if err != nil {
		return nil, err
	}

	result.Components = components

	result.Equal = len(result.Annotations) == 0 && result.SchemaVersionBefore == "" &&
		reflect.DeepEqual(manifestA.Subject, manifestB.Subject) && len(result.Components) == 0 &&
		len(result.Locales) == 0

	return result, nil
}

// diffLocales compares locale metadata for all components.
func diffLocales(before, after map[spec.ComponentType]spec.ComponentConfig) []LocaleDiff {
	var diffs []LocaleDiff
	for _, component := range unionComponentConfigTypes(before, after) {
		beforeLocales, beforeComponent := before[component]
		afterLocales, afterComponent := after[component]

		if !beforeComponent {
			beforeLocales = spec.ComponentConfig{}
		}
		if !afterComponent {
			afterLocales = spec.ComponentConfig{}
		}

		for _, locale := range unionKeys(beforeLocales.Locales, afterLocales.Locales) {
			beforeConfig, beforeSet := beforeLocales.Locales[locale]
			afterConfig, afterSet := afterLocales.Locales[locale]
			if beforeSet && afterSet && reflect.DeepEqual(beforeConfig, afterConfig) {
				continue
			}

			diffs = append(diffs, LocaleDiff{
				Component: component, Locale: locale,
				Before: append([]string(nil), beforeConfig.Files...), After: append([]string(nil), afterConfig.Files...),
				BeforeSet: beforeSet, AfterSet: afterSet,
				EntrypointBefore: beforeConfig.Entrypoint, EntrypointAfter: afterConfig.Entrypoint,
				DefaultBefore: beforeConfig.Default, DefaultAfter: afterConfig.Default,
			})
		}
	}

	return diffs
}

// unionComponentConfigTypes returns all component names from both configs.
func unionComponentConfigTypes(a, b map[spec.ComponentType]spec.ComponentConfig) []spec.ComponentType {
	seen := make(map[spec.ComponentType]struct{}, len(a)+len(b))
	for component := range a {
		seen[component] = struct{}{}
	}
	for component := range b {
		seen[component] = struct{}{}
	}

	result := make([]spec.ComponentType, 0, len(seen))
	for component := range seen {
		result = append(result, component)
	}

	slices.Sort(result)
	return result
}

// diffAnnotations returns one AnnotationDiff per key present in before
// or after with a different (or one-sided) value, sorted by key.
func diffAnnotations(before, after map[string]string) []AnnotationDiff {
	keys := unionKeys(before, after)

	var diffs []AnnotationDiff

	for _, k := range keys {
		v1, v2 := before[k], after[k]
		_, beforeSet := before[k]
		_, afterSet := after[k]
		if v1 == v2 && beforeSet == afterSet {
			continue
		}

		diffs = append(diffs, AnnotationDiff{
			Key: k, Before: v1, After: v2, BeforeSet: beforeSet, AfterSet: afterSet,
		})
	}

	return diffs
}

// cloneDescriptor returns a deep copy of an OCI descriptor.
func cloneDescriptor(value *ocispec.Descriptor) *ocispec.Descriptor {
	if value == nil {
		return nil
	}

	clone := *value
	if value.URLs != nil {
		clone.URLs = append([]string(nil), value.URLs...)
	}
	if value.Annotations != nil {
		clone.Annotations = make(map[string]string, len(value.Annotations))
		maps.Copy(clone.Annotations, value.Annotations)
	}
	if value.Data != nil {
		clone.Data = append([]byte(nil), value.Data...)
	}

	return &clone
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

// diffComponentFiles compares one component's file lists and content digests,
// returning one FileDiff per path added, removed or changed in content.
// descA/descB are only read when inA/inB is true.
func diffComponentFiles(
	ctx context.Context,
	a, b Reader,
	descA, descB ComponentDescriptor,
	inA, inB bool,
) ([]FileDiff, error) {
	sizesA, err := componentFileFingerprints(ctx, a, descA, inA)
	if err != nil {
		return nil, err
	}

	sizesB, err := componentFileFingerprints(ctx, b, descB, inB)
	if err != nil {
		return nil, err
	}

	var diffs []FileDiff

	for _, p := range unionKeys(sizesA, sizesB) {
		fileA, existsA := sizesA[p]
		fileB, existsB := sizesB[p]

		switch {
		case existsA && !existsB:
			diffs = append(diffs, FileDiff{Path: p, Change: FileRemoved, SizeBefore: fileA.Size})

		case !existsA && existsB:
			diffs = append(diffs, FileDiff{Path: p, Change: FileAdded, SizeAfter: fileB.Size})

		case fileA.Size != fileB.Size || fileA.Digest != fileB.Digest:
			diffs = append(diffs, FileDiff{Path: p, Change: FileModified, SizeBefore: fileA.Size, SizeAfter: fileB.Size})
		}
	}

	return diffs, nil
}

// componentFileFingerprints lists c's files (nil if present is false) by path.
func componentFileFingerprints(
	ctx context.Context,
	r Reader,
	c ComponentDescriptor, present bool,
) (map[string]archive.ScanEntry, error) {
	if !present {
		return nil, nil
	}

	files, err := scanComponentFiles(ctx, r, c, archive.ExtractOptions{ComputeDigest: true})
	if err != nil {
		return nil, err
	}

	sizes := make(map[string]archive.ScanEntry, len(files))
	for _, f := range files {
		sizes[f.Name] = f
	}

	return sizes, nil
}
