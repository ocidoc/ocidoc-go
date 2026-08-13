// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: Copyright 2026 WoozyMasta
// Source: github.com/ocidoc/ocidoc-go

// Package digestio provides a streaming writer that computes the canonical (sha256) digest
// and byte count of everything written to it,
// so callers never need to buffer a whole blob in memory just to learn its digest and size.
package digestio

import (
	// Registers digest.Canonical's algorithm (crypto.RegisterHash):
	// go-digest itself does not import a concrete hash implementation,
	// so digest.Canonical.Digester() panics ("sha256 not available")
	// at first use without this side-effect import somewhere in the binary.
	_ "crypto/sha256"
	"io"

	"github.com/opencontainers/go-digest"
)

// Writer wraps dst, computing dst's canonical digest and size as bytes pass through.
// The digest and size are only final once writing is complete;
// Digest and Size may be called at any time, but reflect only what has been written so far.
type Writer struct {
	dst      io.Writer
	digester digest.Digester
	size     int64
}

// NewWriter returns a Writer that forwards everything written to it to dst.
func NewWriter(dst io.Writer) *Writer {
	return &Writer{dst: dst, digester: digest.Canonical.Digester()}
}

// Write implements io.Writer.
func (w *Writer) Write(p []byte) (int, error) {
	n, err := w.dst.Write(p)
	w.digester.Hash().Write(p[:n]) //nolint:errcheck,gosec // hash.Hash.Write never returns an error.
	w.size += int64(n)

	return n, err
}

// Digest returns the canonical digest of everything written so far.
func (w *Writer) Digest() digest.Digest {
	return w.digester.Digest()
}

// Size returns the number of bytes written so far.
func (w *Writer) Size() int64 {
	return w.size
}
