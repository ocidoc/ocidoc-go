// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: Copyright 2026 WoozyMasta
// Source: github.com/ocidoc/ocidoc-go

package artifact

import (
	"fmt"
	"os"

	"github.com/ocidoc/ocidoc-go/internal/archive"
)

// archiveReader wraps a Reader opened from a temporary extraction directory,
// so Close can remove that directory once the caller is done.
// All Reader methods except Close are promoted from the embedded Reader unchanged.
type archiveReader struct {
	Reader

	tempDir string
}

// Close implements Reader:
// removes the temporary directory OpenArchive extracted the .ocidoc archive into.
func (r *archiveReader) Close() error {
	if err := os.RemoveAll(r.tempDir); err != nil {
		return fmt.Errorf("remove temporary directory %s: %w", r.tempDir, err)
	}

	return nil
}

// OpenArchive opens the ".ocidoc" archive at path -
// an uncompressed POSIX tar containing exactly one OCI Image Layout -
// and returns a Reader for it.
//
// The archive is extracted into a fresh temporary directory using internal/archive
// Extract's default safety limits, since a ".ocidoc" file is treated as untrusted input by default.
// The caller must Close the returned Reader to remove that temporary directory;
// OpenArchive itself removes it on any error before returning.
func OpenArchive(path string) (Reader, error) {
	//nolint:gosec // path is caller-supplied, same trust level as os.Open generally.
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open %s: %w", path, err)
	}
	defer f.Close() //nolint:errcheck // read-only handle; nothing to flush on close.

	tempDir, err := os.MkdirTemp("", "ocidoc-archive-*")
	if err != nil {
		return nil, fmt.Errorf("create temporary directory: %w", err)
	}

	if _, err := archive.Extract(f, tempDir, archive.DefaultExtractOptions()); err != nil {
		_ = os.RemoveAll(tempDir)

		return nil, fmt.Errorf("extract %s: %w", path, err)
	}

	reader, err := OpenLayout(tempDir)
	if err != nil {
		_ = os.RemoveAll(tempDir)

		return nil, err
	}

	return &archiveReader{Reader: reader, tempDir: tempDir}, nil
}
