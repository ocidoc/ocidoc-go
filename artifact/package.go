// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: Copyright 2026 WoozyMasta
// Source: github.com/ocidoc/ocidoc-go

package artifact

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	ocispec "github.com/opencontainers/image-spec/specs-go/v1"

	"github.com/ocidoc/ocidoc-go/internal/archive"
)

// PackageArchive writes layoutDir -
// an already-built OCI Image Layout (see BuildLayout) -
// to dst as an uncompressed POSIX tar:
// that is exactly what a ".ocidoc" file is.
// The component blobs inside are already compressed;
// PackageArchive does not compress the outer tar.
//
// modTime should be the same value passed to BuildLayout,
// so the packaged archive's own entries are reproducible
// for the same reason the component tars inside it are.
func PackageArchive(ctx context.Context, dst io.Writer, layoutDir string, modTime time.Time) error {
	entries, err := layoutEntries(layoutDir)
	if err != nil {
		return err
	}

	if _, err := archive.BuildTar(ctx, dst, entries, modTime); err != nil {
		return fmt.Errorf("package .ocidoc archive: %w", err)
	}

	return nil
}

// layoutEntries lists an OCI Image Layout's files as archive entries:
// "oci-layout", "index.json", and every blob under blobs/sha256/.
func layoutEntries(layoutDir string) ([]archive.Entry, error) {
	entries := []archive.Entry{
		{BundlePath: ocispec.ImageLayoutFile, SourcePath: filepath.Join(layoutDir, ocispec.ImageLayoutFile)},
		{BundlePath: ocispec.ImageIndexFile, SourcePath: filepath.Join(layoutDir, ocispec.ImageIndexFile)},
	}

	blobsDir := filepath.Join(layoutDir, filepath.FromSlash(blobsSubdir))

	files, err := os.ReadDir(blobsDir)
	if err != nil {
		return nil, fmt.Errorf("read blobs directory: %w", err)
	}

	for _, f := range files {
		if f.IsDir() {
			continue
		}

		entries = append(entries, archive.Entry{
			BundlePath: blobsSubdir + "/" + f.Name(),
			SourcePath: filepath.Join(blobsDir, f.Name()),
		})
	}

	return entries, nil
}
