// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: Copyright 2026 WoozyMasta
// Source: github.com/ocidoc/ocidoc-go

package artifact

import (
	"context"
	"fmt"
	"io"
)

// finishComponentStream validates the decompressor and the underlying blob
// after a tar reader has reached its logical end.
func finishComponentStream(ctx context.Context, decompressed, raw io.ReadCloser) error {
	if _, err := io.Copy(io.Discard, contextReader{ctx: ctx, reader: decompressed}); err != nil {
		_ = decompressed.Close()
		_ = raw.Close()
		return fmt.Errorf("read decompressed component tail: %w", err)
	}

	if err := decompressed.Close(); err != nil {
		_ = raw.Close()
		return fmt.Errorf("close decompressor: %w", err)
	}

	if _, err := io.Copy(io.Discard, contextReader{ctx: ctx, reader: raw}); err != nil {
		_ = raw.Close()
		return fmt.Errorf("read compressed component tail: %w", err)
	}

	if err := raw.Close(); err != nil {
		return fmt.Errorf("close component blob: %w", err)
	}

	return nil
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
