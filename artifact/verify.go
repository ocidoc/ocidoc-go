// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: Copyright 2026 WoozyMasta
// Source: github.com/ocidoc/ocidoc-go

package artifact

import (
	"archive/tar"
	"context"
	"errors"
	"fmt"
	"io"
	"slices"
	"sort"
	"strings"

	ocispec "github.com/opencontainers/image-spec/specs-go/v1"

	"github.com/ocidoc/ocidoc-go/internal/compression"
	"github.com/ocidoc/ocidoc-go/spec"
)

// VerifyIssue is one structural problem Verify found.
// Component is empty when the issue is not scoped to one component.
type VerifyIssue struct {
	// Component identifies the affected component, or is empty for artifact-wide issues.
	Component spec.ComponentType

	// Message explains the detected structural problem.
	Message string
}

// Verification is Verify's result. Valid is false whenever Issues is non-empty;
// Verify itself only returns a non-nil error when it could not complete the check at all
// (e.g. the manifest itself does not parse),
// not when the artifact turns out to be invalid - an invalid artifact is a successful,
// reported verification outcome, not a failed function call.
type Verification struct {
	// Issues lists every problem found during a completed verification.
	Issues []VerifyIssue

	// Valid reports whether Issues is empty.
	Valid bool
}

// VerifyOptions controls Verify's depth and scope.
type VerifyOptions struct {
	// Component restricts deep verification to one component.
	// Ignored when MetadataOnly is set, and does not narrow the metadata-level checks,
	// which always cover the whole artifact.
	Component spec.ComponentType

	// MetadataOnly restricts Verify to the manifest, config and descriptors:
	// it never opens a component blob.
	// This means entrypoint existence, tar safety and global path collisions -
	// all of which require reading a component's actual file list - are skipped;
	// only an entrypoint's syntactic shape (a well-formed bundle path) is still checked.
	// Default (deep) verification opens every component once
	// to check all of that and to verify its digest.
	MetadataOnly bool
}

// Verify checks r's structural validity. It does not imply signature verification.
//
// Always checked, from the manifest, config and descriptors alone:
// manifest and config media types; OCIDoc artifactType; at least one component layer;
// component type uniqueness; supported component media types;
// the org.ocidoc.components and org.ocidoc.schema/document.id annotations;
// artifact config shape (spec.ValidateArtifactConfig)
// and that its component keys exactly match the manifest's;
// and, for any declared entrypoint, that its value is at least a well-formed bundle path.
//
// Additionally checked unless opts.MetadataOnly is set,
// by opening every (or opts.Component's) component blob once: the blob's digest;
// that every tar entry is a regular file at a well-formed bundle path ("tar safety");
// that any declared entrypoint is actually among the component's files;
// and global path-collision freedom (spec.ValidateBundlePaths) across every opened component's files.
//
// Verify does not check subject consistency because Reader does not expose
// the source artifact's subject.
func Verify(ctx context.Context, r Reader, opts VerifyOptions) (*Verification, error) {
	result := &Verification{Valid: true}

	manifest, err := r.Manifest(ctx)
	if err != nil {
		return nil, err
	}

	verifyManifestShape(result, manifest)

	components, err := r.Components(ctx)
	if err != nil {
		return nil, err
	}

	verifyComponentUniqueness(result, components)
	verifyComponentMediaTypes(result, components)
	verifyComponentsAnnotation(result, manifest, components)
	verifyRootAnnotations(result, manifest)

	cfg, err := r.Config(ctx)
	if err != nil {
		return nil, err
	}

	if err := spec.ValidateArtifactConfig(cfg); err != nil {
		result.fail("", "artifact config: %v", err)
	}

	verifyConfigComponentsMatchManifest(result, cfg, components)
	verifyEntrypointShape(result, cfg)

	if opts.MetadataOnly {
		return result, nil
	}

	verifyComponentsDeep(ctx, r, result, cfg, components, opts.Component)

	return result, nil
}

// fail records one issue and marks the result invalid.
func (v *Verification) fail(component spec.ComponentType, format string, args ...any) {
	v.Valid = false
	v.Issues = append(v.Issues, VerifyIssue{Component: component, Message: fmt.Sprintf(format, args...)})
}

// verifyManifestShape checks the manifest and config media types,
// the OCIDoc artifactType, and that at least one component layer exists.
func verifyManifestShape(result *Verification, manifest *ocispec.Manifest) {
	if manifest.MediaType != ocispec.MediaTypeImageManifest {
		result.fail("", "manifest mediaType %q is not %q", manifest.MediaType, ocispec.MediaTypeImageManifest)
	}

	if manifest.ArtifactType != spec.ArtifactType {
		result.fail("", "manifest artifactType %q is not %q", manifest.ArtifactType, spec.ArtifactType)
	}

	if manifest.Config.MediaType != spec.ConfigMediaType {
		result.fail("", "config mediaType %q is not %q", manifest.Config.MediaType, spec.ConfigMediaType)
	}

	if len(manifest.Layers) == 0 {
		result.fail("", "manifest has no component layers")
	}
}

// verifyComponentUniqueness checks no component type appears twice.
func verifyComponentUniqueness(result *Verification, components []ComponentDescriptor) {
	seen := make(map[spec.ComponentType]bool, len(components))

	for _, c := range components {
		if seen[c.Type] {
			result.fail(c.Type, "component %q appears more than once in the manifest", c.Type)
		}

		seen[c.Type] = true
	}
}

// verifyComponentMediaTypes checks every component
// uses a supported component layer media type.
func verifyComponentMediaTypes(result *Verification, components []ComponentDescriptor) {
	for _, c := range components {
		switch c.Descriptor.MediaType {
		case spec.ComponentLayerGzip, spec.ComponentLayerZstd:
		default:
			result.fail(c.Type, "unsupported component media type %q", c.Descriptor.MediaType)
		}
	}
}

// verifyComponentsAnnotation checks org.ocidoc.components matches
// the manifest's actual component set.
func verifyComponentsAnnotation(result *Verification, manifest *ocispec.Manifest, components []ComponentDescriptor) {
	names := make([]string, 0, len(components))
	for _, c := range components {
		names = append(names, string(c.Type))
	}

	sort.Strings(names)

	want := strings.Join(names, ",")

	got := manifest.Annotations[spec.AnnotationComponents]
	if got != want {
		result.fail("", "%s annotation %q does not match manifest layers %q", spec.AnnotationComponents, got, want)
	}
}

// verifyRootAnnotations checks the managed root annotations are internally consistent:
// a recognized schema version and a non-empty document id.
func verifyRootAnnotations(result *Verification, manifest *ocispec.Manifest) {
	if got := manifest.Annotations[spec.AnnotationSchema]; got != spec.SchemaVersion {
		result.fail("", "%s annotation %q is not %q", spec.AnnotationSchema, got, spec.SchemaVersion)
	}

	if manifest.Annotations[spec.AnnotationDocumentID] == "" {
		result.fail("", "%s annotation is missing", spec.AnnotationDocumentID)
	}
}

// verifyConfigComponentsMatchManifest checks cfg's component keys exactly match
// the manifest's component set, in both directions.
func verifyConfigComponentsMatchManifest(result *Verification, cfg *spec.ArtifactConfig, components []ComponentDescriptor) {
	inManifest := make(map[spec.ComponentType]bool, len(components))
	for _, c := range components {
		inManifest[c.Type] = true
	}

	for name := range cfg.Components {
		if !inManifest[name] {
			result.fail(name, "artifact config declares component %q not present in manifest layers", name)
		}
	}

	for name := range inManifest {
		if _, ok := cfg.Components[name]; !ok {
			result.fail(name, "manifest layer %q is missing from the artifact config", name)
		}
	}
}

// verifyEntrypointShape checks that every declared entrypoint is at least a well-formed bundle path -
// the part of entrypoint verification that needs no component blob read,
// so it runs even under MetadataOnly.
// Whether the entrypoint actually exists among the component's files
// is checked separately, in verifyComponentsDeep.
func verifyEntrypointShape(result *Verification, cfg *spec.ArtifactConfig) {
	for name, cc := range cfg.Components {
		if cc.Entrypoint == "" {
			continue
		}

		if err := spec.ValidateBundlePath(cc.Entrypoint); err != nil {
			result.fail(name, "entrypoint %q: %v", cc.Entrypoint, err)
		}
	}
}

// verifyComponentsDeep opens each (or, if only is set, one) component once and,
// in a single pass over its tar entries:
// verifies every entry is a regular file at a well-formed bundle path ("tar safety");
// collects the component's file paths, both to confirm any declared entrypoint is actually among them and,
// across every opened component, to check the combined set for global path collisions;
// and verifies the component blob's digest.
//
// A component that cannot even be decompressed is reported as its own issue -
// surfacing exactly this kind of damage is Verify's job - rather than aborting the whole check,
// and its raw bytes are still drained afterward so its digest is verified regardless.
func verifyComponentsDeep(
	ctx context.Context,
	r Reader,
	result *Verification,
	cfg *spec.ArtifactConfig,
	components []ComponentDescriptor,
	only spec.ComponentType,
) {
	byComponent := make(map[spec.ComponentType][]string, len(components))

	var allPaths []string

	for _, c := range components {
		if only != "" && c.Type != only {
			continue
		}

		paths := verifyComponentDeep(ctx, r, result, c)
		byComponent[c.Type] = paths
		allPaths = append(allPaths, paths...)
	}

	for name, cc := range cfg.Components {
		if cc.Entrypoint == "" {
			continue
		}

		if files, ok := byComponent[name]; ok && !slices.Contains(files, cc.Entrypoint) {
			result.fail(name, "entrypoint %q is not among the component's files", cc.Entrypoint)
		}
	}

	if err := spec.ValidateBundlePaths(allPaths); err != nil {
		result.fail("", "global path tree: %v", err)
	}
}

// verifyComponentDeep opens, decompresses and reads every tar entry in one component,
// reporting tar-safety issues per entry, then drains the component's raw bytes
// in full so its digest is verified even if the decompressor did not itself need
// to read all the way to the raw stream's end.
//
// It returns the component's regular-file paths.
func verifyComponentDeep(ctx context.Context, r Reader, result *Verification, c ComponentDescriptor) []string {
	rc, _, err := r.OpenComponent(ctx, c.Type)
	if err != nil {
		result.fail(c.Type, "open: %v", err)
		return nil
	}
	//nolint:errcheck // read-only handle; a close error would not change the checks already recorded.
	defer rc.Close()

	var paths []string

	decompressed, err := compression.NewReader(rc, c.Descriptor.MediaType)
	if err != nil {
		result.fail(c.Type, "decompress: %v", err)
	} else {
		paths = verifyComponentTarSafety(result, c.Type, decompressed)

		if err := decompressed.Close(); err != nil {
			result.fail(c.Type, "close decompressor: %v", err)
		}
	}

	if _, err := io.Copy(io.Discard, rc); err != nil {
		result.fail(c.Type, "digest verification failed: %v", err)
	}

	return paths
}

// verifyComponentTarSafety reads every tar entry from decompressed,
// reporting an issue for any entry that is not a regular file
// at a well-formed bundle path, and returns the regular-file paths.
func verifyComponentTarSafety(result *Verification, componentType spec.ComponentType, decompressed io.Reader) []string {
	tr := tar.NewReader(decompressed)

	var paths []string

	for {
		header, err := tr.Next()
		if errors.Is(err, io.EOF) {
			break
		}

		if err != nil {
			result.fail(componentType, "read tar header: %v", err)
			break
		}

		if header.Typeflag != tar.TypeReg {
			result.fail(componentType, "entry %q is not a regular file", header.Name)
			continue
		}

		if err := spec.ValidateBundlePath(header.Name); err != nil {
			result.fail(componentType, "entry %q: %v", header.Name, err)
			continue
		}

		paths = append(paths, header.Name)
	}

	return paths
}
