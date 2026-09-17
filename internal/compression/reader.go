// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: Copyright 2026 WoozyMasta
// Source: github.com/ocidoc/ocidoc-go

package compression

import (
	"compress/gzip"
	"fmt"
	"io"

	"github.com/klauspost/compress/zstd"

	"github.com/ocidoc/ocidoc-go/spec"
)

const (
	// maxDecoderMemory bounds allocations for a single zstd decode stream.
	// Archive traversal applies the stricter uncompressed-size limits separately.
	maxDecoderMemory uint64 = 4 << 30

	// maxDecoderWindow rejects zstd frames advertising a larger history window.
	maxDecoderWindow uint64 = 512 << 20
)

// NewReader wraps src with the decompression algorithm matching mediaType
// (spec.ComponentLayerGzip or spec.ComponentLayerZstd).
// The caller must Close the returned reader.
func NewReader(src io.Reader, mediaType string) (io.ReadCloser, error) {
	switch mediaType {
	case spec.ComponentLayerGzip:
		r, err := gzip.NewReader(src)
		if err != nil {
			return nil, fmt.Errorf("create gzip reader: %w", err)
		}

		return r, nil

	case spec.ComponentLayerZstd:
		dec, err := zstd.NewReader(src,
			zstd.WithDecoderConcurrency(1),
			zstd.WithDecoderLowmem(true),
			zstd.WithDecoderMaxMemory(maxDecoderMemory),
			zstd.WithDecoderMaxWindow(maxDecoderWindow),
		)
		if err != nil {
			return nil, fmt.Errorf("create zstd reader: %w", err)
		}

		return dec.IOReadCloser(), nil

	default:
		return nil, fmt.Errorf("%w: component media type %q", spec.ErrUnsupported, mediaType)
	}
}
