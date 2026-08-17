// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: Copyright 2026 WoozyMasta
// Source: github.com/ocidoc/ocidoc-go

package artifact

import (
	"context"
	"io"

	ocispec "github.com/opencontainers/image-spec/specs-go/v1"

	"github.com/ocidoc/ocidoc-go/spec"
)

// ComponentDescriptor pairs a component's semantic type with its OCI descriptor
// (media type, digest, size and the managed org.ocidoc.component.* annotations).
type ComponentDescriptor struct {
	// Type is the semantic type declared by the component annotation.
	Type spec.ComponentType

	// Descriptor identifies the component archive OCI blob.
	Descriptor ocispec.Descriptor
}

// Reader provides read-only access to one already-built OCIDoc artifact,
// regardless of whether it is backed by a directory OCI Image Layout (OpenLayout)
// or a packaged .ocidoc archive (OpenArchive).
//
// All methods accept context.Context so callers can bound or cancel I/O,
// even though a directory-backed Reader has little to cancel in practice.
type Reader interface {
	// Root returns the descriptor of the artifact's root manifest.
	Root(ctx context.Context) (ocispec.Descriptor, error)

	// OpenBlob opens an OCI blob identified by desc without parsing or re-serializing it.
	// The returned stream verifies its size and digest when read through EOF.
	// The caller must close the returned reader.
	OpenBlob(ctx context.Context, desc ocispec.Descriptor) (io.ReadCloser, error)

	// Manifest returns the parsed root manifest.
	Manifest(ctx context.Context) (*ocispec.Manifest, error)

	// Config returns the parsed artifact config, verified against its digest in the manifest.
	Config(ctx context.Context) (*spec.ArtifactConfig, error)

	// Components returns every component descriptor in the manifest, sorted by component type.
	Components(ctx context.Context) ([]ComponentDescriptor, error)

	// OpenComponent opens component's compressed tar blob for reading.
	// The caller must close the returned reader.
	// errors.Is(err, spec.ErrNotFound) when the manifest has no such component.
	OpenComponent(ctx context.Context, component spec.ComponentType) (io.ReadCloser, ComponentDescriptor, error)

	// Close releases any resources the Reader holds:
	// for OpenLayout, a no-op, since the caller owns the directory;
	// for OpenArchive, it removes the temporary extraction directory.
	// Callers must call Close exactly once when done with the Reader.
	Close() error
}
