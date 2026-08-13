// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: Copyright 2026 WoozyMasta
// Source: github.com/ocidoc/ocidoc-go

package ociblob

import (
	"fmt"
	"io"

	"github.com/opencontainers/go-digest"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
)

// VerifyingReadCloser wraps a ReadCloser,
// verifying that everything read from it matches an expected descriptor's digest
// and size by the time the caller reaches EOF.
// Content is never trusted just because its name or descriptor claims a given digest -
// that claim could be wrong or tampered with, so the actual bytes are always checked.
//
// A caller that stops reading before EOF (or calls Close without ever reading to EOF)
// does not get verification - there is nothing to verify for content that was never read.
type VerifyingReadCloser struct {
	// rc supplies the untrusted blob bytes.
	rc io.ReadCloser

	// verifier accumulates the digest while rc is read.
	verifier digest.Verifier

	// mismatchErr classifies a size or digest mismatch returned from Read.
	mismatchErr error

	// expected declares the digest and size required for rc's content.
	expected ocispec.Descriptor

	// what identifies the content in diagnostic errors without exposing bytes.
	what string

	// size is the number of bytes read and submitted to verifier.
	size int64
}

// NewVerifyingReadCloser returns a ReadCloser that verifies rc's content against expected as it is read.
// what names the content being verified (for example "content" or "component")
// and is used only in error text.
// A read-time mismatch is reported as an error wrapping mismatchErr;
// a malformed expected descriptor is reported as an error wrapping invalidErr.
func NewVerifyingReadCloser(
	rc io.ReadCloser, expected ocispec.Descriptor, what string, invalidErr, mismatchErr error,
) (io.ReadCloser, error) {
	verifier, err := Verifier(expected)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", invalidErr, err)
	}

	return &VerifyingReadCloser{rc: rc, verifier: verifier, expected: expected, what: what, mismatchErr: mismatchErr}, nil
}

// Read implements io.Reader. On the read that reaches EOF,
// it also checks the accumulated digest
// and returns a mismatchErr-wrapped error instead of io.EOF if it does not match.
func (r *VerifyingReadCloser) Read(p []byte) (int, error) {
	n, err := r.rc.Read(p)
	if n > 0 {
		_, _ = r.verifier.Write(p[:n]) // a hash-based Verifier never returns an error.
		r.size += int64(n)
		if r.size > r.expected.Size {
			return n, fmt.Errorf("%w: %s size exceeds expected %d", r.mismatchErr, r.what, r.expected.Size)
		}
	}

	if err == io.EOF { //nolint:errorlint // io.EOF is a sentinel returned by convention, not wrapped.
		if r.size != r.expected.Size {
			return n, fmt.Errorf("%w: %s size %d does not match expected %d",
				r.mismatchErr, r.what, r.size, r.expected.Size)
		}
		if !r.verifier.Verified() {
			return n, fmt.Errorf("%w: %s does not match expected digest %s",
				r.mismatchErr, r.what, r.expected.Digest)
		}
	}

	return n, err
}

// Close implements io.Closer.
func (r *VerifyingReadCloser) Close() error {
	return r.rc.Close()
}
