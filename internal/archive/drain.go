// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: Copyright 2026 WoozyMasta
// Source: github.com/ocidoc/ocidoc-go

package archive

import (
	"context"
	"fmt"
	"io"

	"github.com/ocidoc/ocidoc-go/spec"
)

// drain consumes the bytes after a tar terminator
// while enforcing the same uncompressed-size budget as the regular tar entries.
func drain(ctx context.Context, r io.Reader, maxBytes, alreadyRead int64) (int64, error) {
	buf := make([]byte, 32*1024)
	var drained int64

	for {
		if err := ctx.Err(); err != nil {
			return drained, err
		}

		n, err := r.Read(buf)
		if n > 0 {
			if int64(n) > maxBytes-alreadyRead-drained {
				return drained, fmt.Errorf("%w: scan exceeds max total size %d bytes", spec.ErrUnsupported, maxBytes)
			}
			drained += int64(n)
		}
		if err == io.EOF {
			return drained, nil
		}
		if err != nil {
			return drained, err
		}
	}
}

type contextReader struct {
	ctx    context.Context
	reader io.Reader
}

// Read checks the context before reading from the wrapped reader.
func (r contextReader) Read(p []byte) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}

	return r.reader.Read(p)
}
