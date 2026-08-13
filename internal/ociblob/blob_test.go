// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: Copyright 2026 WoozyMasta
// Source: github.com/ocidoc/ocidoc-go

package ociblob

import (
	"errors"
	"path/filepath"
	"testing"

	"github.com/opencontainers/go-digest"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
)

func TestPathRejectsMalformedAndUnsupportedDigests(t *testing.T) {
	t.Parallel()

	tests := []digest.Digest{
		"",
		"sha256:../../outside",
		"unknown:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
	}

	for _, d := range tests {
		d := d
		t.Run(string(d), func(t *testing.T) {
			t.Parallel()

			_, err := Path(t.TempDir(), ocispec.Descriptor{Digest: d})
			if !errors.Is(err, ErrInvalid) {
				t.Fatalf("Path(%q) error = %v, want ErrInvalid", d, err)
			}
		})
	}
}

func TestPathUsesCanonicalBlobDirectory(t *testing.T) {
	t.Parallel()

	desc := ocispec.Descriptor{Digest: digest.FromString("content")}
	got, err := Path("layout", desc)
	if err != nil {
		t.Fatalf("Path: %v", err)
	}

	want := filepath.Join("layout", "blobs", "sha256", desc.Digest.Encoded())
	if got != want {
		t.Fatalf("Path = %q, want %q", got, want)
	}
}

func TestVerifyRejectsUnsupportedAlgorithmWithoutPanic(t *testing.T) {
	t.Parallel()

	desc := ocispec.Descriptor{
		Digest: "unknown:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		Size:   7,
	}

	if err := Verify(desc, []byte("content")); !errors.Is(err, ErrInvalid) {
		t.Fatalf("Verify error = %v, want ErrInvalid", err)
	}
}
