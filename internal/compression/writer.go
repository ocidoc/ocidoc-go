// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: Copyright 2026 WoozyMasta
// Source: github.com/ocidoc/ocidoc-go

package compression

import (
	"compress/gzip"
	"fmt"
	"io"
	"time"

	"github.com/klauspost/compress/zstd"

	"github.com/ocidoc/ocidoc-go/spec"
)

// gzipOS is the fixed gzip header OS field (255 = unknown),
// pinned so output does not vary by build host.
const gzipOS = 255

// NewWriter wraps dst with typ's compression at level, applying fixed, deterministic settings
// so identical input always produces identical compressed output regardless of build host.
// The caller must Close the returned writer to flush trailers;
// Close does not close dst.
//
// gzip: level as given; name and comment empty; mtime set to modTime
// (see archive.ResolveModTime - the same value used for the tar entries being compressed);
// OS field pinned to 255.
//
// zstd: level converted from the same numeric scale gzip uses via zstd.EncoderLevelFromZstd
// (the format has no numbered level of its own in this package's config surface);
// single-threaded encoding
// (the library defaults to GOMAXPROCS, which is host-dependent and therefore not reproducible);
// no added CRC (the library's default is off, but pinned explicitly here
// rather than left to rely on that default silently continuing to hold in a future library version).
func NewWriter(dst io.Writer, typ spec.CompressionType, level int, modTime time.Time) (io.WriteCloser, error) {
	switch typ {
	case spec.CompressionGzip:
		return newGzipWriter(dst, level, modTime)

	case spec.CompressionZstd:
		return newZstdWriter(dst, level)

	default:
		return nil, fmt.Errorf("%w: compression type %q", spec.ErrUnsupported, typ)
	}
}

// newGzipWriter creates a reproducible gzip stream
// with all variable header fields pinned to deterministic values.
func newGzipWriter(dst io.Writer, level int, modTime time.Time) (io.WriteCloser, error) {
	w, err := gzip.NewWriterLevel(dst, level)
	if err != nil {
		return nil, fmt.Errorf("create gzip writer: %w", err)
	}

	w.Name = ""
	w.Comment = ""
	w.ModTime = modTime
	w.OS = gzipOS

	return w, nil
}

// newZstdWriter creates a reproducible single-threaded zstd
// stream without a content checksum, so identical input produces identical bytes.
func newZstdWriter(dst io.Writer, level int) (io.WriteCloser, error) {
	w, err := zstd.NewWriter(dst,
		zstd.WithEncoderLevel(zstd.EncoderLevelFromZstd(level)),
		zstd.WithEncoderConcurrency(1),
		zstd.WithEncoderCRC(false),
	)
	if err != nil {
		return nil, fmt.Errorf("create zstd writer: %w", err)
	}

	return w, nil
}
