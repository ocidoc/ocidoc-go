// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: Copyright 2026 WoozyMasta
// Source: github.com/ocidoc/ocidoc-go

package store

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"

	"github.com/opencontainers/go-digest"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"

	"github.com/ocidoc/ocidoc-go/artifact"
	"github.com/ocidoc/ocidoc-go/internal/ociblob"
	"github.com/ocidoc/ocidoc-go/spec"
)

// OpenDocument opens the committed OCIDoc manifest identified by manifest.
// The returned reader uses the store's shared blob set
// and does not retain an open file between method calls.
func (s *Store) OpenDocument(ctx context.Context, manifest digest.Digest) (artifact.Reader, error) {
	if err := ociblob.ValidateDigest(manifest); err != nil {
		return nil, fmt.Errorf("%w: manifest: %v", ErrInvalid, err)
	}

	root, err := s.rootDescriptor(manifest)
	if err != nil {
		return nil, err
	}
	data, err := s.fetchMetadata(ctx, root)
	if err != nil {
		return nil, fmt.Errorf("manifest: %w", err)
	}

	var decoded ocispec.Manifest
	if err := json.Unmarshal(data, &decoded); err != nil {
		return nil, fmt.Errorf("%w: parse manifest: %v", ErrInvalid, err)
	}
	if decoded.ArtifactType != spec.ArtifactType {
		return nil, fmt.Errorf("%w: manifest artifactType %q is not an OCIDoc artifact", ErrInvalid, decoded.ArtifactType)
	}
	if err := validateManifestDescriptors(&decoded); err != nil {
		return nil, err
	}

	return &storeReader{manifest: &decoded, root: root, store: s}, nil
}

// storeReader implements artifact.Reader over one manifest in Store.
type storeReader struct {
	// manifest is the verified root manifest selected by OpenDocument.
	manifest *ocispec.Manifest

	// store provides lazy config and component reads.
	store *Store

	// root identifies manifest in the shared OCI content store.
	root ocispec.Descriptor
}

// Close implements artifact.Reader. Store readers do not retain resources.
func (r *storeReader) Close() error {
	return nil
}

// Root implements artifact.Reader.
func (r *storeReader) Root(ctx context.Context) (ocispec.Descriptor, error) {
	if err := ctx.Err(); err != nil {
		return ocispec.Descriptor{}, err
	}

	return r.root, nil
}

// OpenBlob implements artifact.Reader.
func (r *storeReader) OpenBlob(ctx context.Context, desc ocispec.Descriptor) (io.ReadCloser, error) {
	if err := ociblob.Validate(desc); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalid, err)
	}

	rc, err := r.store.oci.Fetch(ctx, desc)
	if err != nil {
		return nil, err
	}
	verified, err := ociblob.NewVerifyingReadCloser(rc, desc, "blob", ErrInvalid, spec.ErrVerification)
	if err != nil {
		_ = rc.Close()
		return nil, err
	}
	return verified, nil
}

// Manifest implements artifact.Reader.
func (r *storeReader) Manifest(ctx context.Context) (*ocispec.Manifest, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	return r.manifest, nil
}

// Config implements artifact.Reader.
func (r *storeReader) Config(ctx context.Context) (*spec.ArtifactConfig, error) {
	data, err := r.store.fetchMetadata(ctx, r.manifest.Config)
	if err != nil {
		return nil, fmt.Errorf("config: %w", err)
	}

	var cfg spec.ArtifactConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("%w: parse artifact config: %v", ErrInvalid, err)
	}

	return &cfg, nil
}

// Components implements artifact.Reader without opening component blobs.
func (r *storeReader) Components(ctx context.Context) ([]artifact.ComponentDescriptor, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	components := make([]artifact.ComponentDescriptor, 0, len(r.manifest.Layers))
	for _, layer := range r.manifest.Layers {
		componentType, ok := layer.Annotations[spec.AnnotationComponentType]
		if !ok {
			return nil, fmt.Errorf("%w: layer %s missing %s annotation", ErrInvalid, layer.Digest, spec.AnnotationComponentType)
		}
		components = append(components, artifact.ComponentDescriptor{
			Type: spec.ComponentType(componentType), Descriptor: layer,
		})
	}
	sort.Slice(components, func(i, j int) bool { return components[i].Type < components[j].Type })

	return components, nil
}

// OpenComponent implements artifact.Reader with streaming size
// and digest verification when the caller reads through EOF.
func (r *storeReader) OpenComponent(ctx context.Context, component spec.ComponentType) (io.ReadCloser, artifact.ComponentDescriptor, error) {
	components, err := r.Components(ctx)
	if err != nil {
		return nil, artifact.ComponentDescriptor{}, err
	}

	for _, candidate := range components {
		if candidate.Type != component {
			continue
		}

		verified, err := r.OpenBlob(ctx, candidate.Descriptor)
		if err != nil {
			return nil, artifact.ComponentDescriptor{}, err
		}
		return verified, candidate, nil
	}

	return nil, artifact.ComponentDescriptor{}, fmt.Errorf("%w: component %q", spec.ErrNotFound, component)
}

// rootDescriptor finds manifest's complete descriptor in the store OCI index.
func (s *Store) rootDescriptor(manifest digest.Digest) (ocispec.Descriptor, error) {
	data, err := os.ReadFile(filepath.Join(s.root, ocispec.ImageIndexFile))
	if err != nil {
		return ocispec.Descriptor{}, fmt.Errorf("read %s: %w", ocispec.ImageIndexFile, err)
	}

	var index ocispec.Index
	if err := json.Unmarshal(data, &index); err != nil {
		return ocispec.Descriptor{}, fmt.Errorf("%w: parse %s: %v", ErrInvalid, ocispec.ImageIndexFile, err)
	}
	for _, candidate := range index.Manifests {
		if candidate.Digest == manifest {
			return candidate, nil
		}
	}

	return ocispec.Descriptor{}, fmt.Errorf("%w: manifest %s", spec.ErrNotFound, manifest)
}

// fetchMetadata reads one bounded metadata blob and verifies its descriptor.
func (s *Store) fetchMetadata(ctx context.Context, desc ocispec.Descriptor) ([]byte, error) {
	if err := ociblob.Validate(desc); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalid, err)
	}
	if desc.Size > ociblob.MaxMetadataSize {
		return nil, fmt.Errorf("%w: metadata blob size %d exceeds limit %d", ErrInvalid, desc.Size, ociblob.MaxMetadataSize)
	}

	rc, err := s.oci.Fetch(ctx, desc)
	if err != nil {
		return nil, err
	}
	defer rc.Close() //nolint:errcheck // read result determines success.

	data, err := io.ReadAll(io.LimitReader(rc, ociblob.MaxMetadataSize+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > ociblob.MaxMetadataSize {
		return nil, fmt.Errorf("%w: metadata blob exceeds limit %d", ErrInvalid, ociblob.MaxMetadataSize)
	}
	if err := ociblob.Verify(desc, data); err != nil {
		return nil, fmt.Errorf("%w: %v", spec.ErrVerification, err)
	}

	return data, nil
}

// validateManifestDescriptors validates descriptors before they are fetched.
func validateManifestDescriptors(manifest *ocispec.Manifest) error {
	if err := ociblob.Validate(manifest.Config); err != nil {
		return fmt.Errorf("%w: config descriptor: %v", ErrInvalid, err)
	}
	for i, layer := range manifest.Layers {
		if err := ociblob.Validate(layer); err != nil {
			return fmt.Errorf("%w: layer %d descriptor: %v", ErrInvalid, i, err)
		}
	}
	return nil
}
