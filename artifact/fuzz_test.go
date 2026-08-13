// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: Copyright 2026 WoozyMasta
// Source: github.com/ocidoc/ocidoc-go

package artifact

import (
	"os"
	"path/filepath"
	"testing"

	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
)

// FuzzOpenLayout verifies malformed OCI layout metadata is rejected without a
// panic before any blob path is trusted.
func FuzzOpenLayout(f *testing.F) {
	f.Add([]byte(`{"schemaVersion":2,"manifests":[]}`))
	f.Add([]byte(`not json`))

	f.Fuzz(func(t *testing.T, index []byte) {
		dir := t.TempDir()
		if err := os.WriteFile(filepath.Join(dir, ocispec.ImageLayoutFile), []byte(`{"imageLayoutVersion":"1.0.0"}`), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, ocispec.ImageIndexFile), index, 0o600); err != nil {
			t.Fatal(err)
		}

		reader, err := OpenLayout(dir)
		if err == nil {
			_ = reader.Close()
		}
	})
}
