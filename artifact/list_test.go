// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: Copyright 2026 WoozyMasta
// Source: github.com/ocidoc/ocidoc-go

package artifact

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/ocidoc/ocidoc-go/spec"
)

func TestListAllComponents(t *testing.T) {
	layoutDir, _ := buildTestLayout(t)

	reader, err := OpenLayout(layoutDir)
	if err != nil {
		t.Fatalf("OpenLayout: %v", err)
	}

	files, err := List(context.Background(), reader, ListOptions{})
	if err != nil {
		t.Fatalf("List: %v", err)
	}

	want := []FileInfo{
		{Component: spec.ComponentDocumentation, Path: "README.md", Size: 4},
		{Component: spec.ComponentLicense, Path: "LICENSE", Size: 3},
	}

	if !reflect.DeepEqual(files, want) {
		t.Fatalf("got %+v, want %+v", files, want)
	}
}

func TestListSingleComponent(t *testing.T) {
	layoutDir, _ := buildTestLayout(t)

	reader, err := OpenLayout(layoutDir)
	if err != nil {
		t.Fatalf("OpenLayout: %v", err)
	}

	files, err := List(context.Background(), reader, ListOptions{Component: spec.ComponentLicense})
	if err != nil {
		t.Fatalf("List: %v", err)
	}

	want := []FileInfo{{Component: spec.ComponentLicense, Path: "LICENSE", Size: 3}}
	if !reflect.DeepEqual(files, want) {
		t.Fatalf("got %+v, want %+v", files, want)
	}
}

func TestListRejectsUnknownComponent(t *testing.T) {
	layoutDir, _ := buildTestLayout(t)

	reader, err := OpenLayout(layoutDir)
	if err != nil {
		t.Fatalf("OpenLayout: %v", err)
	}

	_, err = List(context.Background(), reader, ListOptions{Component: spec.ComponentChangelog})
	if err == nil {
		t.Fatal("expected error for a component not present in the artifact")
	}

	if !errors.Is(err, spec.ErrNotFound) {
		t.Fatalf("expected errors.Is(err, spec.ErrNotFound), got %v", err)
	}
}

func TestListMultiFileComponent(t *testing.T) {
	root := t.TempDir()
	writeConfigTree(t, root, map[string]string{
		"ocidoc.yaml": `
schemaVersion: v1beta
components:
  documentation:
    - /README.md
    - /docs/**
`,
		"README.md":     "# hi",
		"docs/guide.md": "guide content here",
	})

	plan, err := Plan(t.Context(), root, PlanOptions{})
	if err != nil {
		t.Fatalf("Plan: %v", err)
	}

	layoutDir := t.TempDir()

	if _, _, err := BuildLayout(t.Context(), root, layoutDir, BuildLayoutOptions{}); err != nil {
		t.Fatalf("BuildLayout: %v", err)
	}

	reader, err := OpenLayout(layoutDir)
	if err != nil {
		t.Fatalf("OpenLayout: %v", err)
	}

	files, err := List(context.Background(), reader, ListOptions{Component: spec.ComponentDocumentation})
	if err != nil {
		t.Fatalf("List: %v", err)
	}

	wantPaths := plan.Ownership[spec.ComponentDocumentation]
	if len(files) != len(wantPaths) {
		t.Fatalf("got %d files, want %d (%v)", len(files), len(wantPaths), wantPaths)
	}

	for i, f := range files {
		if f.Component != spec.ComponentDocumentation {
			t.Errorf("file %d: got component %q, want %q", i, f.Component, spec.ComponentDocumentation)
		}
	}
}

func TestListEnforcesScanLimits(t *testing.T) {
	root := t.TempDir()
	writeConfigTree(t, root, map[string]string{
		"ocidoc.yaml": `
schemaVersion: v1beta
components:
  documentation:
    - /README.md
    - /docs/**
`,
		"README.md":     "# hi",
		"docs/guide.md": "guide content here",
	})

	layoutDir := t.TempDir()
	if _, _, err := BuildLayout(t.Context(), root, layoutDir, BuildLayoutOptions{}); err != nil {
		t.Fatalf("BuildLayout: %v", err)
	}
	reader, err := OpenLayout(layoutDir)
	if err != nil {
		t.Fatalf("OpenLayout: %v", err)
	}

	_, err = List(t.Context(), reader, ListOptions{Component: spec.ComponentDocumentation, MaxFiles: 1})
	if err == nil {
		t.Fatal("expected List to reject a component over MaxFiles")
	}
	if !errors.Is(err, spec.ErrUnsupported) {
		t.Fatalf("expected errors.Is(err, spec.ErrUnsupported), got %v", err)
	}
}
