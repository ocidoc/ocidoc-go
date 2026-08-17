// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: Copyright 2026 WoozyMasta
// Source: github.com/ocidoc/ocidoc-go

package artifact

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/opencontainers/go-digest"
	specs "github.com/opencontainers/image-spec/specs-go"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"

	"github.com/ocidoc/ocidoc-go/internal/archive"
)

// PackageReader writes reader's complete OCI graph as a portable .ocidoc archive.
// The reader may be backed by a local layout, the local store, or a registry.
func PackageReader(ctx context.Context, reader Reader, dst io.Writer, modTime time.Time) error {
	tempRoot, err := os.MkdirTemp("", "ocidoc-export-*")
	if err != nil {
		return fmt.Errorf("create export directory: %w", err)
	}
	defer os.RemoveAll(tempRoot) //nolint:errcheck // best-effort cleanup.

	layoutDir := filepath.Join(tempRoot, "layout")
	if err := os.MkdirAll(filepath.Join(layoutDir, "blobs", digest.Canonical.String()), 0o750); err != nil {
		return fmt.Errorf("create export blobs directory: %w", err)
	}

	root, err := reader.Root(ctx)
	if err != nil {
		return err
	}
	manifest, err := reader.Manifest(ctx)
	if err != nil {
		return err
	}
	if err := copyBlob(ctx, reader, root, blobPath(layoutDir, root.Digest)); err != nil {
		return fmt.Errorf("copy manifest: %w", err)
	}

	if err := copyBlob(ctx, reader, manifest.Config, blobPath(layoutDir, manifest.Config.Digest)); err != nil {
		return fmt.Errorf("copy config: %w", err)
	}

	components, err := reader.Components(ctx)
	if err != nil {
		return err
	}
	for _, component := range components {
		if err := copyBlob(ctx, reader, component.Descriptor,
			blobPath(layoutDir, component.Descriptor.Digest)); err != nil {
			return fmt.Errorf("copy component %q: %w", component.Type, err)
		}
	}

	layoutData, err := json.Marshal(ocispec.ImageLayout{Version: ocispec.ImageLayoutVersion})
	if err != nil {
		return fmt.Errorf("marshal oci-layout: %w", err)
	}
	if err := os.WriteFile(filepath.Join(layoutDir, ocispec.ImageLayoutFile), layoutData, 0o600); err != nil {
		return fmt.Errorf("write oci-layout: %w", err)
	}
	indexData, err := json.Marshal(ocispec.Index{
		Versioned: specs.Versioned{SchemaVersion: 2},
		MediaType: ocispec.MediaTypeImageIndex,
		Manifests: []ocispec.Descriptor{root},
	})
	if err != nil {
		return fmt.Errorf("marshal index: %w", err)
	}
	if err := os.WriteFile(filepath.Join(layoutDir, ocispec.ImageIndexFile), indexData, 0o600); err != nil {
		return fmt.Errorf("write index: %w", err)
	}

	if modTime.IsZero() {
		modTime = archive.ResolveModTime()
	}

	return PackageArchive(ctx, dst, layoutDir, modTime)
}

// copyBlob copies one verified raw OCI blob to path without transforming it.
func copyBlob(ctx context.Context, reader Reader, desc ocispec.Descriptor, path string) error {
	source, err := reader.OpenBlob(ctx, desc)
	if err != nil {
		return err
	}

	target, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		_ = source.Close()
		return err
	}

	_, copyErr := io.Copy(target, source)
	closeSourceErr := source.Close()
	closeTargetErr := target.Close()
	if copyErr != nil {
		return copyErr
	}
	if closeSourceErr != nil {
		return closeSourceErr
	}

	return closeTargetErr
}

func blobPath(layoutDir string, d digest.Digest) string {
	return filepath.Join(layoutDir, "blobs", d.Algorithm().String(), d.Encoded())
}
