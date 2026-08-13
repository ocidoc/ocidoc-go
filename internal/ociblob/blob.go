// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: Copyright 2026 WoozyMasta
// Source: github.com/ocidoc/ocidoc-go

// Package ociblob provides the validation boundary for OCIDoc OCI blobs.
package ociblob

import (
	_ "crypto/sha256" // Registers the canonical digest algorithm.
	"errors"
	"fmt"
	"path/filepath"

	"github.com/opencontainers/go-digest"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
)

// MaxMetadataSize is the maximum in-memory size of OCI metadata blobs.
const MaxMetadataSize int64 = 4 << 20

// ErrInvalid identifies a malformed or unsupported blob descriptor.
var ErrInvalid = errors.New("invalid OCI blob descriptor")

// ErrMismatch identifies content that does not match its descriptor.
var ErrMismatch = errors.New("OCI blob content mismatch")

// Validate validates desc for use as an OCIDoc blob descriptor.
// OCIDoc v1beta uses the canonical SHA-256 algorithm for every blob.
func Validate(desc ocispec.Descriptor) error {
	if err := ValidateDigest(desc.Digest); err != nil {
		return err
	}
	if desc.Size < 0 {
		return fmt.Errorf("%w: negative size %d", ErrInvalid, desc.Size)
	}

	return nil
}

// ValidateDigest validates d before it is used for hashing or path creation.
func ValidateDigest(d digest.Digest) error {
	if err := d.Validate(); err != nil {
		return fmt.Errorf("%w: digest %q: %v", ErrInvalid, d, err)
	}
	if d.Algorithm() != digest.Canonical {
		return fmt.Errorf("%w: unsupported digest algorithm %q", ErrInvalid, d.Algorithm())
	}

	return nil
}

// Path returns desc's path in the OCI Image Layout rooted at layoutDir.
func Path(layoutDir string, desc ocispec.Descriptor) (string, error) {
	if err := Validate(desc); err != nil {
		return "", err
	}

	return filepath.Join(layoutDir, "blobs", digest.Canonical.String(), desc.Digest.Encoded()), nil
}

// Verify checks that data has exactly the size and digest declared by desc.
func Verify(desc ocispec.Descriptor, data []byte) error {
	if err := Validate(desc); err != nil {
		return err
	}
	if int64(len(data)) != desc.Size {
		return fmt.Errorf("%w: size %d does not match expected %d", ErrMismatch, len(data), desc.Size)
	}

	actual := digest.Canonical.FromBytes(data)
	if actual != desc.Digest {
		return fmt.Errorf("%w: digest %s does not match expected %s", ErrMismatch, actual, desc.Digest)
	}

	return nil
}

// Verifier returns a streaming verifier after validating desc.
func Verifier(desc ocispec.Descriptor) (digest.Verifier, error) {
	if err := Validate(desc); err != nil {
		return nil, err
	}

	return desc.Digest.Verifier(), nil
}
