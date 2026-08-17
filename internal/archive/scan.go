// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: Copyright 2026 WoozyMasta
// Source: github.com/ocidoc/ocidoc-go

package archive

import (
	"archive/tar"
	"context"
	"errors"
	"fmt"
	"io"

	"github.com/ocidoc/ocidoc-go/spec"
)

// ScanEntry describes one regular file found while scanning a tar stream.
type ScanEntry struct {
	// Name is the validated tar entry name.
	Name string

	// Size is the declared uncompressed file size.
	Size int64
}

// Scan reads and validates a tar stream without writing its files.
// It applies the same safety limits as Extract and consumes the stream through EOF
// so a digest-verifying source can validate the complete compressed blob.
func Scan(ctx context.Context, r io.Reader, opts ExtractOptions) ([]ScanEntry, Info, error) {
	opts = applyExtractDefaults(opts)
	tr := tar.NewReader(r)
	entries := make([]ScanEntry, 0)
	var totalSize int64

	for {
		if err := ctx.Err(); err != nil {
			return nil, Info{}, err
		}

		header, err := tr.Next()
		if errors.Is(err, io.EOF) {
			return entries, Info{FileCount: len(entries), UncompressedSize: totalSize}, nil
		}
		if err != nil {
			return nil, Info{}, fmt.Errorf("read tar header: %w", err)
		}

		if header.Typeflag != tar.TypeReg {
			return nil, Info{}, fmt.Errorf("%w: entry %q is not a regular file", spec.ErrInvalid, header.Name)
		}

		name, err := sanitizeExtractPath(header.Name)
		if err != nil {
			return nil, Info{}, err
		}

		if len(entries) >= opts.MaxFiles {
			return nil, Info{}, fmt.Errorf("%w: scan exceeds max file count %d", spec.ErrUnsupported, opts.MaxFiles)
		}
		if header.Size > opts.MaxFileSize {
			return nil, Info{}, fmt.Errorf("%w: entry %q exceeds max file size %d bytes",
				spec.ErrUnsupported, header.Name, opts.MaxFileSize)
		}
		if header.Size < 0 || totalSize > opts.MaxTotalSize-header.Size {
			return nil, Info{}, fmt.Errorf("%w: scan exceeds max total size %d bytes",
				spec.ErrUnsupported, opts.MaxTotalSize)
		}

		read, err := io.Copy(io.Discard, io.LimitReader(tr, header.Size))
		if err != nil {
			return nil, Info{}, fmt.Errorf("read entry %q: %w", header.Name, err)
		}
		if read != header.Size {
			return nil, Info{}, fmt.Errorf("%w: entry %q contains %d bytes, want %d",
				spec.ErrInvalid, header.Name, read, header.Size)
		}

		entries = append(entries, ScanEntry{Name: name, Size: header.Size})
		totalSize += header.Size
	}
}
