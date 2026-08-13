// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: Copyright 2026 WoozyMasta
// Source: github.com/ocidoc/ocidoc-go

package registry

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"

	specs "github.com/opencontainers/image-spec/specs-go"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"

	"github.com/ocidoc/ocidoc-go/artifact"
	"github.com/ocidoc/ocidoc-go/internal/archive"
	"github.com/ocidoc/ocidoc-go/internal/atomicfile"
	"github.com/ocidoc/ocidoc-go/internal/digestio"
	"github.com/ocidoc/ocidoc-go/internal/ociblob"
	"github.com/ocidoc/ocidoc-go/internal/orasrepo"
	"github.com/ocidoc/ocidoc-go/spec"
)

// maxMetadataBlobSize bounds manifest and artifact-config reads.
// Component blobs are always streamed and are not subject to this in-memory limit.
const maxMetadataBlobSize = ociblob.MaxMetadataSize

// PullResult is Pull's result: the reference it resolved,
// the pulled root manifest's descriptor, and the local path Pull wrote.
type PullResult struct {
	// Reference is the immutable reference resolved by Pull.
	Reference string

	// Output is the local archive path written by Pull.
	Output string

	// Manifest identifies the fetched root manifest.
	Manifest ocispec.Descriptor
}

// Pull resolves reference ("host/repository:tag" or "host/repository@digest"),
// fetches its manifest, artifact config and every component blob,
// and packages them as a .ocidoc archive at destination.Path -
// the exact mirror of Push, and of what a local`ocidoc build` followed by `ocidoc push` would have produced.
// Every fetched blob is digest-checked against the descriptor that named it before it is trusted
// (a registry's own claimed digest response header is not itself proof the body bytes are correct),
// and Pull refuses a manifest that is not an OCIDoc artifact.
//
// The reference may identify a standalone or attached OCIDoc manifest;
// callers performing subject selection should pass Discover's immutable Reference.
func (c *Client) Pull(ctx context.Context, reference string, destination artifact.Destination) (*PullResult, error) {
	if destination.Path == "" {
		return nil, fmt.Errorf("%w: destination path is required", ErrInvalid)
	}

	if !destination.Overwrite {
		if _, err := os.Stat(destination.Path); err == nil {
			return nil, fmt.Errorf("%w: %s already exists", ErrInvalid, destination.Path)
		} else if !os.IsNotExist(err) {
			return nil, fmt.Errorf("stat destination %q: %w", destination.Path, err)
		}
	}

	tempRoot, err := os.MkdirTemp("", "ocidoc-pull-*")
	if err != nil {
		return nil, fmt.Errorf("create temp pull directory: %w", err)
	}
	defer os.RemoveAll(tempRoot) //nolint:errcheck // best-effort cleanup of a temp directory.

	layoutDir := filepath.Join(tempRoot, "layout")

	root, err := c.pullToLayout(ctx, reference, layoutDir)
	if err != nil {
		return nil, err
	}

	if err := packagePulledLayout(ctx, layoutDir, destination); err != nil {
		return nil, err
	}

	return &PullResult{Reference: reference, Output: destination.Path, Manifest: root}, nil
}

// pullToLayout resolves and validates one standalone OCIDoc reference,
// then writes its complete graph to layoutDir as an OCI Image Layout.
// layoutDir must not already exist.
func (c *Client) pullToLayout(ctx context.Context, reference, layoutDir string) (ocispec.Descriptor, error) {
	resolved, err := c.resolveReference(ctx, reference, "reference")
	if err != nil {
		return ocispec.Descriptor{}, err
	}

	manifestData, err := fetchBlob(ctx, resolved.repo, resolved.descriptor)
	if err != nil {
		return ocispec.Descriptor{}, fmt.Errorf("manifest: %w", err)
	}

	var manifest ocispec.Manifest
	if err := json.Unmarshal(manifestData, &manifest); err != nil {
		//nolint:errorlint // wrapping %v deliberately: ErrInvalid is this function's own sentinel, not err's.
		return ocispec.Descriptor{}, fmt.Errorf("%w: parse manifest: %v", ErrInvalid, err)
	}

	if manifest.ArtifactType != spec.ArtifactType {
		return ocispec.Descriptor{}, fmt.Errorf("%w: manifest artifactType %q is not an OCIDoc artifact", ErrInvalid, manifest.ArtifactType)
	}

	if err := writePulledLayout(
		ctx, resolved.repo, layoutDir, resolved.descriptor, manifestData, manifest,
	); err != nil {
		return ocispec.Descriptor{}, err
	}

	return resolved.descriptor, nil
}

// writePulledLayout writes a complete OCI Image Layout to layoutDir:
// root's manifest blob (already fetched, as manifestData),
// the artifact config blob and every component layer blob (fetched here),
// plus "oci-layout" and "index.json".
func writePulledLayout(
	ctx context.Context,
	repo orasrepo.Repository,
	layoutDir string,
	root ocispec.Descriptor,
	manifestData []byte,
	manifest ocispec.Manifest,
) error {
	blobsDir := filepath.Join(layoutDir, filepath.FromSlash("blobs/sha256"))
	if err := os.MkdirAll(blobsDir, 0o750); err != nil {
		return fmt.Errorf("create blobs directory: %w", err)
	}

	rootPath, err := pulledBlobPath(layoutDir, root)
	if err != nil {
		return fmt.Errorf("manifest descriptor: %w", err)
	}
	if err := os.WriteFile(rootPath, manifestData, 0o600); err != nil {
		return fmt.Errorf("write manifest blob: %w", err)
	}

	if err := fetchBlobToFile(ctx, repo, layoutDir, manifest.Config); err != nil {
		return fmt.Errorf("config: %w", err)
	}

	for _, layer := range manifest.Layers {
		if err := fetchBlobToFile(ctx, repo, layoutDir, layer); err != nil {
			return fmt.Errorf("component layer %s: %w", layer.Digest, err)
		}
	}

	return writePulledLayoutMetadata(layoutDir, root)
}

// writePulledLayoutMetadata writes the two small top-level OCI Image
// Layout files: "oci-layout" and "index.json" (referencing root as the
// layout's sole entry).
func writePulledLayoutMetadata(layoutDir string, root ocispec.Descriptor) error {
	layoutBytes, err := json.Marshal(ocispec.ImageLayout{Version: ocispec.ImageLayoutVersion})
	if err != nil {
		return fmt.Errorf("marshal %s: %w", ocispec.ImageLayoutFile, err)
	}

	if err := os.WriteFile(filepath.Join(layoutDir, ocispec.ImageLayoutFile), layoutBytes, 0o600); err != nil {
		return fmt.Errorf("write %s: %w", ocispec.ImageLayoutFile, err)
	}

	index := ocispec.Index{
		Versioned: specs.Versioned{SchemaVersion: 2},
		MediaType: ocispec.MediaTypeImageIndex,
		Manifests: []ocispec.Descriptor{root},
	}

	indexBytes, err := json.Marshal(index)
	if err != nil {
		return fmt.Errorf("marshal %s: %w", ocispec.ImageIndexFile, err)
	}

	if err := os.WriteFile(filepath.Join(layoutDir, ocispec.ImageIndexFile), indexBytes, 0o600); err != nil {
		return fmt.Errorf("write %s: %w", ocispec.ImageIndexFile, err)
	}

	return nil
}

// packagePulledLayout packages layoutDir as a .ocidoc archive at
// dest.Path, via a temporary file in dest.Path's own directory renamed
// into place only once the archive is fully written -- mirroring how
// artifact.Build's own packageToDestination protects a build's output,
// so a failed or interrupted Pull never leaves a partial file at
// dest.Path either.
func packagePulledLayout(ctx context.Context, layoutDir string, dest artifact.Destination) error {
	return atomicfile.WriteFile(dest.Path, dest.Overwrite, func(w io.Writer) error {
		return artifact.PackageArchive(ctx, w, layoutDir, archive.ResolveModTime())
	})
}

// fetchBlob fetches desc's content from repo and returns it in memory,
// verified against desc.Digest. Used only for the manifest, which
// content-addressed OCI registries guarantee is small; component and
// config blobs go through fetchBlobToFile instead, which never holds a
// whole blob in memory.
func fetchBlob(ctx context.Context, repo orasrepo.Repository, desc ocispec.Descriptor) ([]byte, error) {
	if err := validateRegistryBlob(desc); err != nil {
		return nil, err
	}
	if desc.Size > maxMetadataBlobSize {
		return nil, fmt.Errorf("%w: metadata blob size %d exceeds limit %d", ErrInvalid, desc.Size, maxMetadataBlobSize)
	}

	rc, err := repo.Fetch(ctx, desc)
	if err != nil {
		return nil, wrapError(err)
	}
	defer rc.Close() //nolint:errcheck // read-only handle; a close error here would not change an already-read result.

	data, err := io.ReadAll(io.LimitReader(rc, maxMetadataBlobSize+1))
	if err != nil {
		return nil, fmt.Errorf("fetch %s: %w", desc.Digest, err)
	}
	if int64(len(data)) > maxMetadataBlobSize {
		return nil, fmt.Errorf("%w: fetched metadata blob exceeds limit %d", ErrInvalid, maxMetadataBlobSize)
	}

	if err := ociblob.Verify(desc, data); err != nil {
		return nil, fmt.Errorf("%w: fetched blob: %v", ErrVerification, err)
	}

	return data, nil
}

// fetchBlobToFile fetches desc's content from repo and streams it,
// digest-checked as it is copied, to a new file under blobsDir named by
// its digest -- written to a temporary file first and renamed into place
// only once the digest check passes, so a verification failure never
// leaves a wrongly-named blob behind.
func fetchBlobToFile(ctx context.Context, repo orasrepo.Repository, layoutDir string, desc ocispec.Descriptor) error {
	if err := validateRegistryBlob(desc); err != nil {
		return err
	}
	if desc.Size == math.MaxInt64 {
		return fmt.Errorf("%w: invalid blob size %d", ErrInvalid, desc.Size)
	}
	if desc.MediaType == spec.ConfigMediaType && desc.Size > maxMetadataBlobSize {
		return fmt.Errorf("%w: artifact config size %d exceeds limit %d", ErrInvalid, desc.Size, maxMetadataBlobSize)
	}

	rc, err := repo.Fetch(ctx, desc)
	if err != nil {
		return wrapError(err)
	}
	//nolint:errcheck // read-only handle; a close error here would not change an already-read result.
	defer rc.Close()

	blobsDir := filepath.Join(layoutDir, filepath.FromSlash("blobs/sha256"))
	tmp, err := atomicfile.CreateTemp(blobsDir)
	if err != nil {
		return fmt.Errorf("create temp blob: %w", err)
	}
	defer tmp.Cleanup()

	counted := digestio.NewWriter(tmp.File())
	if _, err := io.Copy(counted, io.LimitReader(rc, desc.Size+1)); err != nil {
		return fmt.Errorf("fetch blob %s: %w", desc.Digest, err)
	}

	if counted.Digest() != desc.Digest {
		return fmt.Errorf("%w: fetched blob digest %s does not match expected %s", ErrVerification, counted.Digest(), desc.Digest)
	}
	if counted.Size() != desc.Size {
		return fmt.Errorf("%w: fetched blob size %d does not match expected %d", ErrVerification, counted.Size(), desc.Size)
	}

	finalPath, err := pulledBlobPath(layoutDir, desc)
	if err != nil {
		return err
	}
	if err := tmp.Rename(finalPath); err != nil {
		return fmt.Errorf("finalize blob %s: %w", desc.Digest, err)
	}

	return nil
}

// validateRegistryBlob rejects descriptors that cannot name a safe OCI blob.
func validateRegistryBlob(desc ocispec.Descriptor) error {
	if err := ociblob.Validate(desc); err != nil {
		return fmt.Errorf("%w: %v", ErrInvalid, err)
	}

	return nil
}

// pulledBlobPath returns desc's validated OCI Image Layout blob location.
func pulledBlobPath(layoutDir string, desc ocispec.Descriptor) (string, error) {
	path, err := ociblob.Path(layoutDir, desc)
	if err != nil {
		return "", fmt.Errorf("%w: %v", ErrInvalid, err)
	}

	return path, nil
}
