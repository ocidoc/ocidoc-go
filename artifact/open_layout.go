// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: Copyright 2026 WoozyMasta
// Source: github.com/ocidoc/ocidoc-go

package artifact

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"

	ocispec "github.com/opencontainers/image-spec/specs-go/v1"

	"github.com/ocidoc/ocidoc-go/internal/ociblob"
	"github.com/ocidoc/ocidoc-go/internal/ociclone"
	"github.com/ocidoc/ocidoc-go/spec"
)

// layoutReader implements Reader over a directory OCI Image Layout.
type layoutReader struct {
	// manifest is the validated root manifest parsed during OpenLayout.
	manifest *ocispec.Manifest

	// root identifies manifest's verified OCI blob.
	root ocispec.Descriptor

	// dir is the caller-owned OCI Image Layout root.
	dir string
}

// OpenLayout opens the OCI Image Layout directory at path
// and returns a Reader for its single OCIDoc manifest.
//
// It validates the layout version, requires exactly one manifest in index.json,
// verifies the manifest blob's digest, and rejects non-OCIDoc manifests.
func OpenLayout(path string) (Reader, error) {
	layoutBytes, err := readMetadataFile(filepath.Join(path, ocispec.ImageLayoutFile))
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", ocispec.ImageLayoutFile, err)
	}

	var layout ocispec.ImageLayout
	if err := json.Unmarshal(layoutBytes, &layout); err != nil {
		return nil, fmt.Errorf("parse %s: %w", ocispec.ImageLayoutFile, err)
	}

	if layout.Version != ocispec.ImageLayoutVersion {
		return nil, fmt.Errorf("%w: unsupported oci-layout version %q", spec.ErrUnsupported, layout.Version)
	}

	indexBytes, err := readMetadataFile(filepath.Join(path, ocispec.ImageIndexFile))
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", ocispec.ImageIndexFile, err)
	}

	var index ocispec.Index
	if err := json.Unmarshal(indexBytes, &index); err != nil {
		return nil, fmt.Errorf("parse %s: %w", ocispec.ImageIndexFile, err)
	}

	switch len(index.Manifests) {
	case 0:
		return nil, fmt.Errorf("%w: %s has no manifests", spec.ErrNotFound, ocispec.ImageIndexFile)
	case 1:
		// exactly one, continue below.
	default:
		return nil, fmt.Errorf("%w: %s lists %d manifests, want exactly one",
			spec.ErrAmbiguous, ocispec.ImageIndexFile, len(index.Manifests))
	}

	root := index.Manifests[0]

	manifestBytes, err := readLayoutMetadataBlob(path, root)
	if err != nil {
		return nil, fmt.Errorf("read manifest blob: %w", err)
	}

	var manifest ocispec.Manifest
	if err := json.Unmarshal(manifestBytes, &manifest); err != nil {
		return nil, fmt.Errorf("parse manifest: %w", err)
	}

	if manifest.ArtifactType != spec.ArtifactType {
		return nil, fmt.Errorf("%w: manifest artifactType %q is not an OCIDoc artifact",
			spec.ErrInvalid, manifest.ArtifactType)
	}
	if err := validateManifestDescriptors(&manifest); err != nil {
		return nil, err
	}

	return &layoutReader{dir: path, root: root, manifest: &manifest}, nil
}

// Close implements Reader.
// It is a no-op because the caller owns the layout directory passed to OpenLayout.
func (r *layoutReader) Close() error {
	return nil
}

// Root implements Reader.
func (r *layoutReader) Root(ctx context.Context) (ocispec.Descriptor, error) {
	if err := ctx.Err(); err != nil {
		return ocispec.Descriptor{}, err
	}

	return ociclone.Descriptor(r.root), nil
}

// OpenBlob implements Reader.
func (r *layoutReader) OpenBlob(ctx context.Context, desc ocispec.Descriptor) (io.ReadCloser, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	path, err := layoutBlobPath(r.dir, desc)
	if err != nil {
		return nil, err
	}

	//nolint:gosec // path is confined by layoutBlobPath after digest validation.
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open blob %s: %w", desc.Digest, err)
	}

	verified, err := newDigestVerifyingReadCloser(f, desc)
	if err != nil {
		_ = f.Close()
		return nil, err
	}

	return verified, nil
}

// Manifest implements Reader.
func (r *layoutReader) Manifest(ctx context.Context) (*ocispec.Manifest, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	return ociclone.Manifest(r.manifest), nil
}

// Config implements Reader.
func (r *layoutReader) Config(ctx context.Context) (*spec.ArtifactConfig, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	data, err := readLayoutMetadataBlob(r.dir, r.manifest.Config)
	if err != nil {
		return nil, fmt.Errorf("read config blob: %w", err)
	}

	var cfg spec.ArtifactConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parse artifact config: %w", err)
	}

	return &cfg, nil
}

// Components implements Reader.
func (r *layoutReader) Components(ctx context.Context) ([]ComponentDescriptor, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	components := make([]ComponentDescriptor, 0, len(r.manifest.Layers))

	for _, layer := range r.manifest.Layers {
		componentType, ok := layer.Annotations[spec.AnnotationComponentType]
		if !ok {
			return nil, fmt.Errorf("%w: layer %s missing %s annotation",
				spec.ErrInvalid, layer.Digest, spec.AnnotationComponentType)
		}

		components = append(components, ComponentDescriptor{
			Type:       spec.ComponentType(componentType),
			Descriptor: ociclone.Descriptor(layer),
		})
	}

	sort.Slice(components, func(i, j int) bool { return components[i].Type < components[j].Type })

	return components, nil
}

// OpenComponent implements Reader.
func (r *layoutReader) OpenComponent(
	ctx context.Context,
	component spec.ComponentType,
) (io.ReadCloser, ComponentDescriptor, error) {
	if err := ctx.Err(); err != nil {
		return nil, ComponentDescriptor{}, err
	}

	components, err := r.Components(ctx)
	if err != nil {
		return nil, ComponentDescriptor{}, err
	}

	for _, c := range components {
		if c.Type != component {
			continue
		}

		verified, err := r.OpenBlob(ctx, c.Descriptor)
		if err != nil {
			return nil, ComponentDescriptor{}, fmt.Errorf("component %q: %w", component, err)
		}

		return verified, c, nil
	}

	return nil, ComponentDescriptor{}, fmt.Errorf("%w: component %q", spec.ErrNotFound, component)
}

// validateManifestDescriptors validates the config
// and layer descriptors before they are used to construct filesystem paths or read blobs.
func validateManifestDescriptors(manifest *ocispec.Manifest) error {
	if err := validateBlobDescriptor(manifest.Config); err != nil {
		return fmt.Errorf("config descriptor: %w", err)
	}
	for i, layer := range manifest.Layers {
		if err := validateBlobDescriptor(layer); err != nil {
			return fmt.Errorf("layer %d descriptor: %w", i, err)
		}
	}

	return nil
}

// validateBlobDescriptor translates an invalid OCI blob descriptor
// into the public artifact validation error vocabulary.
func validateBlobDescriptor(desc ocispec.Descriptor) error {
	if err := ociblob.Validate(desc); err != nil {
		return fmt.Errorf("%w: %v", spec.ErrInvalid, err)
	}

	return nil
}

// layoutBlobPath returns desc's digest-derived path within an OCI layout.
func layoutBlobPath(dir string, desc ocispec.Descriptor) (string, error) {
	path, err := ociblob.Path(dir, desc)
	if err != nil {
		return "", fmt.Errorf("%w: %v", spec.ErrInvalid, err)
	}

	return path, nil
}

// readLayoutMetadataBlob reads and verifies a bounded manifest or config blob.
func readLayoutMetadataBlob(dir string, desc ocispec.Descriptor) ([]byte, error) {
	if desc.Size > ociblob.MaxMetadataSize {
		return nil, fmt.Errorf("%w: metadata blob size %d exceeds limit %d",
			spec.ErrInvalid, desc.Size, ociblob.MaxMetadataSize)
	}

	path, err := layoutBlobPath(dir, desc)
	if err != nil {
		return nil, err
	}

	data, err := readMetadataFile(path)
	if err != nil {
		return nil, err
	}
	if err := ociblob.Verify(desc, data); err != nil {
		if errors.Is(err, ociblob.ErrInvalid) {
			return nil, fmt.Errorf("%w: %v", spec.ErrInvalid, err)
		}
		return nil, fmt.Errorf("%w: %v", spec.ErrVerification, err)
	}

	return data, nil
}

// readMetadataFile reads one bounded metadata file without trusting its size.
func readMetadataFile(path string) ([]byte, error) {
	//nolint:gosec // caller supplies a layout path; digest-derived paths are validated first.
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	//nolint:errcheck // read-only handle; the read result determines success.
	defer f.Close()

	data, err := io.ReadAll(io.LimitReader(f, ociblob.MaxMetadataSize+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > ociblob.MaxMetadataSize {
		return nil, fmt.Errorf("%w: metadata file exceeds limit %d", spec.ErrInvalid, ociblob.MaxMetadataSize)
	}

	return data, nil
}
