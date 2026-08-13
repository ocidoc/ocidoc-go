// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: Copyright 2026 WoozyMasta
// Source: github.com/ocidoc/ocidoc-go

// Package testfixture builds local OCI Image Layout test artifacts
// shared by registry and store's test suites,
// so both build the same minimal artifact shape from one implementation
// instead of two independently hand-synced copies.
package testfixture

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/ocidoc/ocidoc-go/artifact"
)

// WriteFiles writes files (path -> content, "/"-separated, relative to root) under root,
// creating parent directories as needed.
func WriteFiles(t *testing.T, root string, files map[string]string) {
	t.Helper()

	for rel, content := range files {
		full := filepath.Join(root, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(full), 0o750); err != nil {
			t.Fatalf("MkdirAll: %v", err)
		}

		if err := os.WriteFile(full, []byte(content), 0o600); err != nil {
			t.Fatalf("WriteFile(%s): %v", full, err)
		}
	}
}

// BuildArtifact builds a minimal local OCI Image Layout -
// a single "documentation" component whose entrypoint is README.md,
// containing content - and returns a Reader open on it,
// closing it automatically at test cleanup.
func BuildArtifact(t *testing.T, content string) artifact.Reader {
	t.Helper()

	root := t.TempDir()
	WriteFiles(t, root, map[string]string{
		"ocidoc.yaml": `
schemaVersion: v1beta
components:
  documentation:
    - /README.md
`,
		"README.md": content,
	})

	layoutDir := t.TempDir()

	if _, _, err := artifact.BuildLayout(t.Context(), root, layoutDir, artifact.BuildLayoutOptions{
		ModTime: time.Unix(1700000000, 0).UTC(),
	}); err != nil {
		t.Fatalf("BuildLayout: %v", err)
	}

	reader, err := artifact.OpenLayout(layoutDir)
	if err != nil {
		t.Fatalf("OpenLayout: %v", err)
	}

	t.Cleanup(func() { _ = reader.Close() })

	return reader
}
