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
	"github.com/ocidoc/ocidoc-go/internal/ociblob"
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
	manifestData, err := json.Marshal(manifest)
	if err != nil {
		return fmt.Errorf("marshal manifest: %w", err)
	}
	if err := ociblob.Verify(root, manifestData); err != nil {
		return fmt.Errorf("verify manifest: %w", err)
	}
	if err := os.WriteFile(blobPath(layoutDir, root.Digest), manifestData, 0o600); err != nil {
		return fmt.Errorf("write manifest: %w", err)
	}

	config, err := reader.Config(ctx)
	if err != nil {
		return err
	}
	configData, err := json.Marshal(config)
	if err != nil {
		return fmt.Errorf("marshal config: %w", err)
	}
	if err := ociblob.Verify(manifest.Config, configData); err != nil {
		return fmt.Errorf("verify config: %w", err)
	}
	if err := os.WriteFile(blobPath(layoutDir, manifest.Config.Digest), configData, 0o600); err != nil {
		return fmt.Errorf("write config: %w", err)
	}

	components, err := reader.Components(ctx)
	if err != nil {
		return err
	}
	for _, component := range components {
		source, _, err := reader.OpenComponent(ctx, component.Type)
		if err != nil {
			return err
		}
		target, err := os.OpenFile(blobPath(layoutDir, component.Descriptor.Digest), os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
		if err != nil {
			_ = source.Close()
			return fmt.Errorf("create component %q: %w", component.Type, err)
		}
		_, copyErr := io.Copy(target, source)
		closeSourceErr := source.Close()
		closeTargetErr := target.Close()
		if copyErr != nil {
			return fmt.Errorf("copy component %q: %w", component.Type, copyErr)
		}
		if closeSourceErr != nil {
			return fmt.Errorf("close component %q: %w", component.Type, closeSourceErr)
		}
		if closeTargetErr != nil {
			return fmt.Errorf("close component %q: %w", component.Type, closeTargetErr)
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

func blobPath(layoutDir string, d digest.Digest) string {
	return filepath.Join(layoutDir, "blobs", d.Algorithm().String(), d.Encoded())
}
