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
		dec, err := zstd.NewReader(src)
		if err != nil {
			return nil, fmt.Errorf("create zstd reader: %w", err)
		}

		return dec.IOReadCloser(), nil

	default:
		return nil, fmt.Errorf("%w: component media type %q", spec.ErrUnsupported, mediaType)
	}
}
