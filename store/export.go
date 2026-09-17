// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: Copyright 2026 WoozyMasta
// Source: github.com/ocidoc/ocidoc-go

package store

import (
	"context"
	"io"
	"time"

	"github.com/ocidoc/ocidoc-go/artifact"
	"github.com/ocidoc/ocidoc-go/internal/atomicfile"
	"github.com/opencontainers/go-digest"
)

// Export writes one stored document as a portable .ocidoc archive.
// The store remains the source of the graph; this method only materializes an explicit export copy.
func (s *Store) Export(ctx context.Context, manifest digest.Digest, dst io.Writer, modTime time.Time) error {
	reader, err := s.OpenDocument(ctx, manifest)
	if err != nil {
		return err
	}
	defer reader.Close() //nolint:errcheck // export result determines success.

	return artifact.PackageReader(ctx, reader, dst, modTime)
}

// ExportFile exports one stored document through the SDK's safe file finalization path.
// Existing destinations are preserved unless overwrite is true.
func (s *Store) ExportFile(
	ctx context.Context,
	manifest digest.Digest,
	path string,
	overwrite bool,
	modTime time.Time,
) error {
	return atomicfile.WriteFile(path, overwrite, func(dst io.Writer) error {
		return s.Export(ctx, manifest, dst, modTime)
	})
}
