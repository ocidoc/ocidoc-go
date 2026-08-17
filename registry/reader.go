// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: Copyright 2026 WoozyMasta
// Source: github.com/ocidoc/ocidoc-go

package registry

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"sort"

	ocispec "github.com/opencontainers/image-spec/specs-go/v1"

	"github.com/ocidoc/ocidoc-go/artifact"
	"github.com/ocidoc/ocidoc-go/internal/ociblob"
	"github.com/ocidoc/ocidoc-go/internal/ociclone"
	"github.com/ocidoc/ocidoc-go/internal/orasrepo"
	"github.com/ocidoc/ocidoc-go/spec"
)

// remoteReader implements artifact.Reader over one immutable manifest resolved from a registry reference.
// Manifest metadata is loaded by Open;
// config and component blobs remain lazy until their Reader methods need them.
type remoteReader struct {
	// manifest is the validated root manifest fetched by Open.
	manifest *ocispec.Manifest

	// repo fetches lazy config and component content.
	repo orasrepo.Repository

	// root identifies manifest in repo.
	root ocispec.Descriptor
}

// Open resolves reference and returns a lazy artifact.Reader backed by its registry repository.
// The root manifest is fetched and digest-checked before Open returns;
// config and component blobs are fetched only when requested.
// An attached manifest may be opened through its exact tag or digest after discovery;
// standalone-only operations enforce their own subject policy.
func (c *Client) Open(ctx context.Context, reference string) (artifact.Reader, error) {
	resolved, err := c.resolveReference(ctx, reference, "reference")
	if err != nil {
		return nil, err
	}

	manifestData, err := fetchBlob(ctx, resolved.repo, resolved.descriptor)
	if err != nil {
		return nil, fmt.Errorf("manifest: %w", err)
	}

	var manifest ocispec.Manifest
	if err := json.Unmarshal(manifestData, &manifest); err != nil {
		//nolint:errorlint // ErrInvalid classifies malformed remote content; the JSON error is diagnostic only.
		return nil, fmt.Errorf("%w: parse manifest: %v", ErrInvalid, err)
	}
	if manifest.ArtifactType != spec.ArtifactType {
		return nil, fmt.Errorf(
			"%w: manifest artifactType %q is not an OCIDoc artifact",
			ErrInvalid, manifest.ArtifactType)
	}

	return &remoteReader{repo: resolved.repo, root: resolved.descriptor, manifest: &manifest}, nil
}

// Close implements artifact.Reader.
// Registry fetches have request-scoped bodies,
// so the reader itself holds no resource that needs closing.
func (r *remoteReader) Close() error { return nil }

// Root implements artifact.Reader.
func (r *remoteReader) Root(ctx context.Context) (ocispec.Descriptor, error) {
	if err := ctx.Err(); err != nil {
		return ocispec.Descriptor{}, err
	}
	return ociclone.Descriptor(r.root), nil
}

// OpenBlob implements artifact.Reader.
func (r *remoteReader) OpenBlob(ctx context.Context, desc ocispec.Descriptor) (io.ReadCloser, error) {
	if err := validateRegistryBlob(desc); err != nil {
		return nil, fmt.Errorf("blob descriptor: %w", err)
	}

	rc, err := r.repo.Fetch(ctx, desc)
	if err != nil {
		return nil, wrapError(err)
	}

	verified, err := ociblob.NewVerifyingReadCloser(rc, desc, "blob", ErrInvalid, ErrVerification)
	if err != nil {
		_ = rc.Close()
		return nil, err
	}

	return verified, nil
}

// Manifest implements artifact.Reader.
func (r *remoteReader) Manifest(ctx context.Context) (*ocispec.Manifest, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return ociclone.Manifest(r.manifest), nil
}

// Config implements artifact.Reader.
// The config remains lazy so a manifest-only inspection does not download it.
func (r *remoteReader) Config(ctx context.Context) (*spec.ArtifactConfig, error) {
	data, err := fetchBlob(ctx, r.repo, r.manifest.Config)
	if err != nil {
		return nil, fmt.Errorf("config: %w", err)
	}

	var cfg spec.ArtifactConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		//nolint:errorlint // ErrInvalid classifies malformed remote content; the JSON error is diagnostic only.
		return nil, fmt.Errorf("%w: parse artifact config: %v", ErrInvalid, err)
	}

	return &cfg, nil
}

// Components implements artifact.Reader without fetching any component blob.
func (r *remoteReader) Components(ctx context.Context) ([]artifact.ComponentDescriptor, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	components := make([]artifact.ComponentDescriptor, 0, len(r.manifest.Layers))
	for _, layer := range r.manifest.Layers {
		if err := validateRegistryBlob(layer); err != nil {
			return nil, fmt.Errorf("layer descriptor: %w", err)
		}
		componentType, ok := layer.Annotations[spec.AnnotationComponentType]
		if !ok {
			return nil, fmt.Errorf("%w: layer %s missing %s annotation", ErrInvalid, layer.Digest, spec.AnnotationComponentType)
		}
		components = append(components, artifact.ComponentDescriptor{
			Type: spec.ComponentType(componentType), Descriptor: ociclone.Descriptor(layer),
		})
	}

	sort.Slice(components, func(i, j int) bool { return components[i].Type < components[j].Type })
	return components, nil
}

// OpenComponent implements artifact.Reader.
// The returned stream verifies both descriptor digest and size when its caller reads through EOF.
func (r *remoteReader) OpenComponent(
	ctx context.Context,
	component spec.ComponentType,
) (io.ReadCloser, artifact.ComponentDescriptor, error) {
	components, err := r.Components(ctx)
	if err != nil {
		return nil, artifact.ComponentDescriptor{}, err
	}

	for _, candidate := range components {
		if candidate.Type != component {
			continue
		}
		if candidate.Descriptor.Size < 0 {
			return nil, artifact.ComponentDescriptor{}, fmt.Errorf("%w: component %q has negative size", ErrInvalid, component)
		}

		verified, err := r.OpenBlob(ctx, candidate.Descriptor)
		if err != nil {
			return nil, artifact.ComponentDescriptor{}, err
		}

		return verified, candidate, nil
	}

	return nil, artifact.ComponentDescriptor{}, fmt.Errorf("%w: component %q", spec.ErrNotFound, component)
}
