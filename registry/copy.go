// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: Copyright 2026 WoozyMasta
// Source: github.com/ocidoc/ocidoc-go

package registry

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"maps"
	"os"
	"path/filepath"

	"github.com/opencontainers/go-digest"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"

	"github.com/ocidoc/ocidoc-go/artifact"
	"github.com/ocidoc/ocidoc-go/spec"
)

// LocationKind identifies one supported copy endpoint representation.
type LocationKind string

const (
	// LocationArchive is a packaged .ocidoc file.
	LocationArchive LocationKind = "archive"

	// LocationLayout is an unpacked OCI Image Layout directory.
	LocationLayout LocationKind = "layout"

	// LocationRegistry is an OCI registry reference.
	LocationRegistry LocationKind = "registry"

	// LocationSubject is a subject reference requiring attached discovery or publication.
	LocationSubject LocationKind = "subject"
)

// Source identifies Copy's source. Value is a local filesystem path
// for archive/layout sources and an OCI reference for registry/subject sources.
type Source struct {
	// Kind identifies the source representation.
	Kind LocationKind

	// Value is a path or registry reference according to Kind.
	Value string
}

// Destination identifies Copy's destination.
// Value follows Source's convention.
// Overwrite applies only to local archive/layout destinations.
type Destination struct {
	// Kind identifies the destination representation.
	Kind LocationKind

	// Value is a path or registry reference according to Kind.
	Value string

	// Overwrite permits replacing an existing local destination.
	Overwrite bool
}

// CopyOptions controls subject discovery and destination publication.
// These fields are ignored for standalone copy endpoints.
type CopyOptions struct {
	// Publication selects publication for subject destinations.
	Publication PublicationMode

	// Discover controls discovery for subject sources.
	Discover DiscoverOptions

	// Replace permits replacing an existing direct documentation tag.
	Replace bool
}

// CopyResult reports the endpoints and destination root manifest.
// For subject copy, SourceManifest and Publication describe
// the selected source root and destination publication mode.
type CopyResult struct {
	// Manifest identifies the copied destination manifest.
	Manifest ocispec.Descriptor

	// SourceManifest identifies the selected source manifest for subject copy.
	SourceManifest ocispec.Descriptor

	// Source describes the requested copy source.
	Source Source

	// Publication is used only for subject destinations.
	Publication PublicationMode

	// Destination describes the requested copy destination.
	Destination Destination
}

// subjectlessReader exposes an attached graph as Attach's standalone source shape.
// Config and component descriptors/blobs remain unchanged;
// Attach serializes a new root with the destination subject.
type subjectlessReader struct {
	artifact.Reader
}

// Copy copies a complete OCIDoc graph.
// Subject-to-subject copy discovers one source document
// and rebuilds only its root manifest for the destination's immutable subject descriptor.
func (c *Client) Copy(ctx context.Context, source Source, destination Destination, opts CopyOptions) (*CopyResult, error) {
	if err := validateCopyEndpoint("source", source.Kind, source.Value); err != nil {
		return nil, err
	}
	if err := validateCopyEndpoint("destination", destination.Kind, destination.Value); err != nil {
		return nil, err
	}

	var (
		root, sourceRoot ocispec.Descriptor
		publication      PublicationMode
		err              error
	)

	switch {
	case isLocal(source.Kind) && destination.Kind == LocationRegistry:
		root, err = c.copyLocalToRegistry(ctx, source, destination.Value)

	case source.Kind == LocationRegistry && destination.Kind == LocationArchive:
		var result *PullResult
		result, err = c.PullArchive(ctx, source.Value, artifact.Destination{
			Path: destination.Value, Overwrite: destination.Overwrite,
		})
		if result != nil {
			root = result.Manifest
		}

	case source.Kind == LocationRegistry && destination.Kind == LocationLayout:
		root, err = c.copyRegistryToLayout(ctx, source.Value, destination)

	case source.Kind == LocationRegistry && destination.Kind == LocationRegistry:
		root, err = c.copyRegistryToRegistry(ctx, source.Value, destination.Value)

	case source.Kind == LocationSubject && destination.Kind == LocationSubject:
		root, sourceRoot, publication, err = c.copySubjectToSubject(ctx, source.Value, destination.Value, opts)

	default:
		return nil, fmt.Errorf("%w: copy from %s to %s is not supported", ErrUnsupported, source.Kind, destination.Kind)
	}
	if err != nil {
		return nil, err
	}

	return &CopyResult{
		Source: source, Destination: destination, Manifest: root,
		SourceManifest: sourceRoot, Publication: publication,
	}, nil
}

// Manifest implements artifact.Reader by returning a clone without Subject.
// This lets Attach bind the copied graph to the destination subject.
func (r subjectlessReader) Manifest(ctx context.Context) (*ocispec.Manifest, error) {
	manifest, err := r.Reader.Manifest(ctx)
	if err != nil {
		return nil, err
	}

	cloned := *manifest
	cloned.Subject = nil
	cloned.Annotations = maps.Clone(manifest.Annotations)
	return &cloned, nil
}

// Root implements artifact.Reader for the transformed subjectless manifest.
func (r subjectlessReader) Root(ctx context.Context) (ocispec.Descriptor, error) {
	manifest, err := r.Manifest(ctx)
	if err != nil {
		return ocispec.Descriptor{}, err
	}
	data, err := json.Marshal(manifest)
	if err != nil {
		return ocispec.Descriptor{}, fmt.Errorf("marshal subjectless manifest: %w", err)
	}

	return ocispec.Descriptor{
		MediaType:    ocispec.MediaTypeImageManifest,
		ArtifactType: spec.ArtifactType,
		Digest:       digest.Canonical.FromBytes(data),
		Size:         int64(len(data)),
		Annotations:  maps.Clone(manifest.Annotations),
	}, nil
}

// OpenBlob implements artifact.Reader for the transformed root manifest.
func (r subjectlessReader) OpenBlob(ctx context.Context, desc ocispec.Descriptor) (io.ReadCloser, error) {
	root, err := r.Root(ctx)
	if err != nil {
		return nil, err
	}
	if desc.Digest != root.Digest {
		return r.Reader.OpenBlob(ctx, desc)
	}

	manifest, err := r.Manifest(ctx)
	if err != nil {
		return nil, err
	}
	data, err := json.Marshal(manifest)
	if err != nil {
		return nil, fmt.Errorf("marshal subjectless manifest: %w", err)
	}

	return io.NopCloser(bytes.NewReader(data)), nil
}

// validateCopyEndpoint rejects an empty value
// or unsupported endpoint kind before Copy selects a transfer path.
func validateCopyEndpoint(name string, kind LocationKind, value string) error {
	if value == "" {
		return fmt.Errorf("%w: %s value is required", ErrInvalid, name)
	}

	switch kind {
	case LocationArchive, LocationLayout, LocationRegistry, LocationSubject:
		return nil
	default:
		return fmt.Errorf("%w: unsupported %s kind %q", ErrInvalid, name, kind)
	}
}

// copySubjectToSubject discovers the source attachment, removes its subject,
// and attaches the resulting graph to destination.
// It returns the destination manifest, selected source manifest, and effective publication mode.
func (c *Client) copySubjectToSubject(
	ctx context.Context,
	source, destination string,
	opts CopyOptions,
) (ocispec.Descriptor, ocispec.Descriptor, PublicationMode, error) {
	discovered, err := c.Discover(ctx, source, opts.Discover)
	if err != nil {
		return ocispec.Descriptor{}, ocispec.Descriptor{}, "", err
	}
	reader, err := c.Open(ctx, discovered.Reference)
	if err != nil {
		return ocispec.Descriptor{}, ocispec.Descriptor{}, "", err
	}
	defer reader.Close() //nolint:errcheck // read-only registry reader owns no persistent resource.
	publication := opts.Publication
	if publication == "" {
		publication = PublicationBoth
	}
	attached, err := c.Attach(ctx, subjectlessReader{Reader: reader}, destination, AttachOptions{
		Publication: publication,
		Replace:     opts.Replace,
	})
	if err != nil {
		return ocispec.Descriptor{}, ocispec.Descriptor{}, "", err
	}
	return attached.Manifest, discovered.Manifest, publication, nil
}

func isLocal(kind LocationKind) bool {
	return kind == LocationArchive || kind == LocationLayout
}

// copyLocalToRegistry opens a local archive
// or layout and pushes its complete OCIDoc graph to reference.
func (c *Client) copyLocalToRegistry(ctx context.Context, source Source, reference string) (ocispec.Descriptor, error) {
	var (
		reader artifact.Reader
		err    error
	)

	if source.Kind == LocationArchive {
		reader, err = artifact.OpenArchive(source.Value)
	} else {
		reader, err = artifact.OpenLayout(source.Value)
	}
	if err != nil {
		return ocispec.Descriptor{}, err
	}
	defer reader.Close() //nolint:errcheck // read-only handle; cleanup failure cannot change the completed copy.

	result, err := c.Push(ctx, reader, reference)
	if err != nil {
		return ocispec.Descriptor{}, err
	}

	return result.Manifest, nil
}

// copyRegistryToRegistry opens the source graph and publishes it to destination.
func (c *Client) copyRegistryToRegistry(ctx context.Context, source, destination string) (ocispec.Descriptor, error) {
	reader, err := c.Open(ctx, source)
	if err != nil {
		return ocispec.Descriptor{}, err
	}
	defer reader.Close() //nolint:errcheck // remote Reader owns no persistent resource.

	result, err := c.Push(ctx, reader, destination)
	if err != nil {
		return ocispec.Descriptor{}, err
	}

	return result.Manifest, nil
}

// copyRegistryToLayout pulls source into a temporary sibling layout,
// then installs it at destination atomically.
func (c *Client) copyRegistryToLayout(ctx context.Context, source string, destination Destination) (ocispec.Descriptor, error) {
	parent := filepath.Dir(destination.Value)
	if err := os.MkdirAll(parent, 0o750); err != nil {
		return ocispec.Descriptor{}, fmt.Errorf("create layout parent directory: %w", err)
	}

	tempDir, err := os.MkdirTemp(parent, ".ocidoc-layout-*")
	if err != nil {
		return ocispec.Descriptor{}, fmt.Errorf("create temp layout directory: %w", err)
	}
	if err := os.Remove(tempDir); err != nil {
		return ocispec.Descriptor{}, fmt.Errorf("prepare temp layout directory: %w", err)
	}
	defer os.RemoveAll(tempDir) //nolint:errcheck // best-effort cleanup if publication or finalization fails.

	root, err := c.pullToLayout(ctx, source, tempDir)
	if err != nil {
		return ocispec.Descriptor{}, err
	}

	if err := replaceDirectory(tempDir, destination.Value, destination.Overwrite); err != nil {
		return ocispec.Descriptor{}, err
	}

	return root, nil
}

// replaceDirectory atomically installs source beside destination.
// When overwrite is enabled, the prior destination
// is first renamed to a backup and restored if installing source fails.
func replaceDirectory(source, destination string, overwrite bool) error {
	_, err := os.Lstat(destination)
	if os.IsNotExist(err) {
		if err := os.Rename(source, destination); err != nil {
			return fmt.Errorf("finalize layout %q: %w", destination, err)
		}
		return nil
	}
	if err != nil {
		return fmt.Errorf("stat layout destination %q: %w", destination, err)
	}
	if !overwrite {
		return fmt.Errorf("%w: %s already exists", ErrInvalid, destination)
	}

	backup, err := os.CreateTemp(filepath.Dir(destination), ".ocidoc-backup-*")
	if err != nil {
		return fmt.Errorf("reserve layout backup path: %w", err)
	}
	backupPath := backup.Name()
	if err := backup.Close(); err != nil {
		return fmt.Errorf("close layout backup placeholder: %w", err)
	}
	if err := os.Remove(backupPath); err != nil {
		return fmt.Errorf("prepare layout backup path: %w", err)
	}

	if err := os.Rename(destination, backupPath); err != nil {
		return fmt.Errorf("back up layout destination %q: %w", destination, err)
	}

	if err := os.Rename(source, destination); err != nil {
		if restoreErr := os.Rename(backupPath, destination); restoreErr != nil {
			return fmt.Errorf("finalize layout %q: %w (restore failed: %v)", destination, err, restoreErr)
		}
		return fmt.Errorf("finalize layout %q: %w", destination, err)
	}

	if err := os.RemoveAll(backupPath); err != nil {
		return fmt.Errorf("remove replaced layout %q: %w", backupPath, err)
	}

	return nil
}
