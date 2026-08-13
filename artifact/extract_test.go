// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: Copyright 2026 WoozyMasta
// Source: github.com/ocidoc/ocidoc-go

package artifact

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/ocidoc/ocidoc-go/spec"
)

func TestExtractAllComponents(t *testing.T) {
	layoutDir, _ := buildTestLayout(t)

	reader, err := OpenLayout(layoutDir)
	if err != nil {
		t.Fatalf("OpenLayout: %v", err)
	}

	dest := t.TempDir()

	if err := Extract(context.Background(), reader, ExtractOptions{Output: dest}); err != nil {
		t.Fatalf("Extract: %v", err)
	}

	readme, err := os.ReadFile(filepath.Join(dest, "README.md"))
	if err != nil {
		t.Fatalf("ReadFile(README.md): %v", err)
	}

	if string(readme) != "# hi" {
		t.Fatalf("got %q, want %q", readme, "# hi")
	}

	license, err := os.ReadFile(filepath.Join(dest, "LICENSE"))
	if err != nil {
		t.Fatalf("ReadFile(LICENSE): %v", err)
	}

	if string(license) != "MIT" {
		t.Fatalf("got %q, want %q", license, "MIT")
	}
}

func TestExtractSingleComponent(t *testing.T) {
	layoutDir, _ := buildTestLayout(t)

	reader, err := OpenLayout(layoutDir)
	if err != nil {
		t.Fatalf("OpenLayout: %v", err)
	}

	dest := t.TempDir()

	err = Extract(context.Background(), reader, ExtractOptions{
		Output:    dest,
		Component: spec.ComponentLicense,
	})
	if err != nil {
		t.Fatalf("Extract: %v", err)
	}

	if _, err := os.Stat(filepath.Join(dest, "LICENSE")); err != nil {
		t.Fatalf("expected LICENSE to be extracted: %v", err)
	}

	if _, err := os.Stat(filepath.Join(dest, "README.md")); !os.IsNotExist(err) {
		t.Fatalf("did not expect README.md to be extracted, stat err: %v", err)
	}
}

func TestExtractRejectsUnknownComponent(t *testing.T) {
	layoutDir, _ := buildTestLayout(t)

	reader, err := OpenLayout(layoutDir)
	if err != nil {
		t.Fatalf("OpenLayout: %v", err)
	}

	err = Extract(context.Background(), reader, ExtractOptions{
		Output:    t.TempDir(),
		Component: spec.ComponentChangelog,
	})
	if err == nil {
		t.Fatal("expected error for a component not present in the artifact")
	}

	if !errors.Is(err, spec.ErrNotFound) {
		t.Fatalf("expected errors.Is(err, spec.ErrNotFound), got %v", err)
	}
}

func TestExtractRefusesOverwriteByDefault(t *testing.T) {
	layoutDir, _ := buildTestLayout(t)

	reader, err := OpenLayout(layoutDir)
	if err != nil {
		t.Fatalf("OpenLayout: %v", err)
	}

	dest := t.TempDir()
	writeFile(t, filepath.Join(dest, "README.md"), "pre-existing")

	err = Extract(context.Background(), reader, ExtractOptions{Output: dest})
	if err == nil {
		t.Fatal("expected error when a destination file already exists and Overwrite is false")
	}
}

func TestExtractOverwriteReplacesExistingFile(t *testing.T) {
	layoutDir, _ := buildTestLayout(t)

	reader, err := OpenLayout(layoutDir)
	if err != nil {
		t.Fatalf("OpenLayout: %v", err)
	}

	dest := t.TempDir()
	writeFile(t, filepath.Join(dest, "README.md"), "pre-existing")

	err = Extract(context.Background(), reader, ExtractOptions{Output: dest, Overwrite: true})
	if err != nil {
		t.Fatalf("Extract: %v", err)
	}

	got, err := os.ReadFile(filepath.Join(dest, "README.md"))
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}

	if string(got) != "# hi" {
		t.Fatalf("got %q, want %q", got, "# hi")
	}
}
