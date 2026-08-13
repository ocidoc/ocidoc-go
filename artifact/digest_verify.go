// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: Copyright 2026 WoozyMasta
// Source: github.com/ocidoc/ocidoc-go

package artifact

import (
	"io"

	ocispec "github.com/opencontainers/image-spec/specs-go/v1"

	"github.com/ocidoc/ocidoc-go/internal/ociblob"
	"github.com/ocidoc/ocidoc-go/spec"
)

// newDigestVerifyingReadCloser returns a ReadCloser that verifies rc's content against expected as it is read,
// failing with a *spec.ErrVerification-wrapped error at EOF on mismatch.
// Every current caller (List, Extract, Verify) reads a component to completion.
func newDigestVerifyingReadCloser(rc io.ReadCloser, expected ocispec.Descriptor) (io.ReadCloser, error) {
	return ociblob.NewVerifyingReadCloser(rc, expected, "content", spec.ErrInvalid, spec.ErrVerification)
}
