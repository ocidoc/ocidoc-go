// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: Copyright 2026 WoozyMasta
// Source: github.com/ocidoc/ocidoc-go

package artifact

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/ocidoc/ocidoc-go/spec"
)

func writeConfigTree(t *testing.T, root string, files map[string]string) {
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

func TestPlanBasic(t *testing.T) {
	root := t.TempDir()
	writeConfigTree(t, root, map[string]string{
		"ocidoc.yaml": `
schemaVersion: v1beta
components:
  documentation:
    - /README.md
  license:
    - /LICENSE
`,
		"README.md": "# hi",
		"LICENSE":   "MIT",
	})

	plan, err := Plan(t.Context(), root, PlanOptions{})
	if err != nil {
		t.Fatalf("Plan: %v", err)
	}

	if plan.Document.ID != spec.DefaultDocumentID {
		t.Fatalf("got document id %q, want %q", plan.Document.ID, spec.DefaultDocumentID)
	}

	if plan.Settings.Strict {
		t.Fatal("expected default strict=false")
	}

	if plan.Settings.Compression.Type != spec.CompressionGzip || plan.Settings.Compression.Level != 6 {
		t.Fatalf("got compression %+v, want gzip/6", plan.Settings.Compression)
	}

	if got := plan.Ownership[spec.ComponentDocumentation]; !reflect.DeepEqual(got, []string{"README.md"}) {
		t.Fatalf("got documentation ownership %v, want [README.md]", got)
	}

	if got := plan.Entrypoints[spec.ComponentDocumentation]; got != "README.md" {
		t.Fatalf("got documentation entrypoint %q, want README.md", got)
	}

	if len(plan.Warnings) != 0 {
		t.Fatalf("expected no warnings, got %v", plan.Warnings)
	}
}

func TestPlanRejectsCanceledContext(t *testing.T) {
	root := t.TempDir()
	writeConfigTree(t, root, map[string]string{
		"ocidoc.yaml": "schemaVersion: v1beta\ncomponents:\n  documentation:\n    - /README.md\n",
		"README.md":   "# hi",
	})

	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	if _, err := Plan(ctx, root, PlanOptions{}); !errors.Is(err, context.Canceled) {
		t.Fatalf("Plan: got %v, want errors.Is(err, context.Canceled)", err)
	}
}

func TestPlanDocumentOverride(t *testing.T) {
	root := t.TempDir()
	writeConfigTree(t, root, map[string]string{
		"ocidoc.yaml": `
schemaVersion: v1beta
document:
  id: default
  language: en
components:
  documentation:
    - /README.md
`,
		"README.md": "# hi",
	})

	plan, err := Plan(t.Context(), root, PlanOptions{
		Document: spec.DocumentSettings{Variant: "full"},
	})
	if err != nil {
		t.Fatalf("Plan: %v", err)
	}

	want := spec.DocumentSettings{ID: "default", Language: "en", Variant: "full"}
	if plan.Document != want {
		t.Fatalf("got %+v, want %+v", plan.Document, want)
	}
}

func TestPlanAnnotationOverrideWins(t *testing.T) {
	root := t.TempDir()
	writeConfigTree(t, root, map[string]string{
		"ocidoc.yaml": `
schemaVersion: v1beta
annotations:
  org.opencontainers.image.title: Old title
components:
  documentation:
    - /README.md
`,
		"README.md": "# hi",
	})

	plan, err := Plan(t.Context(), root, PlanOptions{
		Annotations: map[string]string{"org.opencontainers.image.title": "New title"},
	})
	if err != nil {
		t.Fatalf("Plan: %v", err)
	}

	if got := plan.Annotations["org.opencontainers.image.title"]; got != "New title" {
		t.Fatalf("got title %q, want %q", got, "New title")
	}
}

func TestPlanIgnoreOverrideAppendsAfterConfig(t *testing.T) {
	root := t.TempDir()
	writeConfigTree(t, root, map[string]string{
		"ocidoc.yaml": `
schemaVersion: v1beta
components:
  documentation:
    - /README.md
    - /docs/**
`,
		"README.md":        "# hi",
		"docs/guide.md":    "guide",
		"docs/internal.md": "internal",
	})

	plan, err := Plan(t.Context(), root, PlanOptions{Ignore: []string{"/docs/internal.md"}})
	if err != nil {
		t.Fatalf("Plan: %v", err)
	}

	for _, p := range plan.Ownership[spec.ComponentDocumentation] {
		if p == "docs/internal.md" {
			t.Fatalf("expected docs/internal.md to be excluded by the --ignore override, got ownership %v", plan.Ownership)
		}
	}

	found := false

	for _, p := range plan.Ownership[spec.ComponentDocumentation] {
		if p == "docs/guide.md" {
			found = true
		}
	}

	if !found {
		t.Fatalf("expected docs/guide.md to remain owned, got %v", plan.Ownership)
	}
}

func TestPlanSettingsOverrideWins(t *testing.T) {
	root := t.TempDir()
	writeConfigTree(t, root, map[string]string{
		"ocidoc.yaml": `
schemaVersion: v1beta
settings:
  strict: false
  compression:
    type: gzip
    level: 6
components:
  documentation:
    - /README.md
`,
		"README.md": "# hi",
	})

	overrideLevel := 19
	strictOverride := true

	plan, err := Plan(t.Context(), root, PlanOptions{
		Settings: &spec.BuildSettings{
			Strict: &strictOverride,
			Compression: &spec.CompressionSettings{
				Type:  spec.CompressionZstd,
				Level: &overrideLevel,
			},
		},
	})
	if err != nil {
		t.Fatalf("Plan: %v", err)
	}

	if !plan.Settings.Strict {
		t.Fatal("expected the strict override to win")
	}

	if plan.Settings.Compression.Type != spec.CompressionZstd {
		t.Fatalf("got compression type %q, want %q", plan.Settings.Compression.Type, spec.CompressionZstd)
	}

	if plan.Settings.Compression.Level != 19 {
		t.Fatalf("got compression level %d, want 19", plan.Settings.Compression.Level)
	}
}

func TestPlanSettingsOverridePartialCompressionKeepsOtherField(t *testing.T) {
	root := t.TempDir()
	writeConfigTree(t, root, map[string]string{
		"ocidoc.yaml": `
schemaVersion: v1beta
settings:
  compression:
    type: zstd
    level: 19
components:
  documentation:
    - /README.md
`,
		"README.md": "# hi",
	})

	overrideLevel := 3

	plan, err := Plan(t.Context(), root, PlanOptions{
		Settings: &spec.BuildSettings{
			Compression: &spec.CompressionSettings{Level: &overrideLevel},
		},
	})
	if err != nil {
		t.Fatalf("Plan: %v", err)
	}

	if plan.Settings.Compression.Type != spec.CompressionZstd {
		t.Fatalf("expected the config's compression type to survive a level-only override, got %q", plan.Settings.Compression.Type)
	}

	if plan.Settings.Compression.Level != 3 {
		t.Fatalf("got compression level %d, want 3", plan.Settings.Compression.Level)
	}
}

func TestPlanSettingsOverrideLeavesUnsetFieldsAlone(t *testing.T) {
	root := t.TempDir()
	writeConfigTree(t, root, map[string]string{
		"ocidoc.yaml": `
schemaVersion: v1beta
settings:
  strict: true
  compression:
    type: zstd
    level: 19
components:
  documentation:
    - /README.md
`,
		"README.md": "# hi",
	})

	plan, err := Plan(t.Context(), root, PlanOptions{Settings: &spec.BuildSettings{}})
	if err != nil {
		t.Fatalf("Plan: %v", err)
	}

	if !plan.Settings.Strict {
		t.Fatal("expected the loaded config's strict setting to survive an empty override")
	}

	if plan.Settings.Compression.Type != spec.CompressionZstd || plan.Settings.Compression.Level != 19 {
		t.Fatalf("expected the loaded config's compression to survive an empty override, got %+v", plan.Settings.Compression)
	}
}

func TestPlanRejectsReservedAnnotationOverride(t *testing.T) {
	root := t.TempDir()
	writeConfigTree(t, root, map[string]string{
		"ocidoc.yaml": "schemaVersion: v1beta\ncomponents:\n  documentation:\n    - /README.md\n",
		"README.md":   "# hi",
	})

	_, err := Plan(t.Context(), root, PlanOptions{
		Annotations: map[string]string{"org.ocidoc.schema": "v1beta"},
	})
	if err == nil {
		t.Fatal("expected error for reserved annotation override")
	}

	if !errors.Is(err, spec.ErrInvalid) {
		t.Fatalf("expected errors.Is(err, spec.ErrInvalid), got %v", err)
	}
}

func TestPlanEntrypointOverrideWins(t *testing.T) {
	root := t.TempDir()
	writeConfigTree(t, root, map[string]string{
		"ocidoc.yaml": `
schemaVersion: v1beta
entrypoints:
  documentation: /README.md
components:
  documentation:
    - /README.md
    - /docs/index.md
`,
		"README.md":     "# hi",
		"docs/index.md": "# index",
	})

	plan, err := Plan(t.Context(), root, PlanOptions{
		Entrypoints: map[spec.ComponentType]string{spec.ComponentDocumentation: "/docs/index.md"},
	})
	if err != nil {
		t.Fatalf("Plan: %v", err)
	}

	if got := plan.Entrypoints[spec.ComponentDocumentation]; got != "docs/index.md" {
		t.Fatalf("got entrypoint %q, want docs/index.md", got)
	}
}

func TestPlanNonStrictEmptyComponentWarns(t *testing.T) {
	root := t.TempDir()
	writeConfigTree(t, root, map[string]string{
		"ocidoc.yaml": `
schemaVersion: v1beta
components:
  documentation:
    - /README.md
  security:
    - /SECURITY.md
`,
		"README.md": "# hi",
	})

	plan, err := Plan(t.Context(), root, PlanOptions{})
	if err != nil {
		t.Fatalf("Plan: %v", err)
	}

	if len(plan.Warnings) != 1 {
		t.Fatalf("got warnings %v, want exactly one", plan.Warnings)
	}
}

func TestPlanStrictEmptyComponentFails(t *testing.T) {
	root := t.TempDir()
	writeConfigTree(t, root, map[string]string{
		"ocidoc.yaml": `
schemaVersion: v1beta
settings:
  strict: true
components:
  documentation:
    - /README.md
  security:
    - /SECURITY.md
`,
		"README.md": "# hi",
	})

	_, err := Plan(t.Context(), root, PlanOptions{})
	if err == nil {
		t.Fatal("expected error for empty component in strict mode")
	}

	var emptyErr *EmptyComponentsError
	if !errors.As(err, &emptyErr) {
		t.Fatalf("got %v, want *EmptyComponentsError", err)
	}

	if !errors.Is(err, spec.ErrInvalid) {
		t.Fatalf("expected errors.Is(err, spec.ErrInvalid), got %v", err)
	}

	want := []spec.ComponentType{spec.ComponentSecurity}
	if !reflect.DeepEqual(emptyErr.Components, want) {
		t.Fatalf("got %v, want %v", emptyErr.Components, want)
	}
}

func TestPlanRejectsWhenNoComponentMatchesAnything(t *testing.T) {
	root := t.TempDir()
	writeConfigTree(t, root, map[string]string{
		"ocidoc.yaml": "schemaVersion: v1beta\ncomponents:\n  documentation:\n    - /README.md\n",
	})

	_, err := Plan(t.Context(), root, PlanOptions{})
	if err == nil {
		t.Fatal("expected error when no component matches any file")
	}

	if !errors.Is(err, spec.ErrInvalid) {
		t.Fatalf("expected errors.Is(err, spec.ErrInvalid), got %v", err)
	}
}

func TestPlanDiscoversMarkdownDependencies(t *testing.T) {
	root := t.TempDir()
	writeConfigTree(t, root, map[string]string{
		"ocidoc.yaml":   "schemaVersion: v1beta\ncomponents:\n  documentation:\n    - /README.md\n",
		"README.md":     "[guide](docs/guide.md)\n",
		"docs/guide.md": "guide",
	})

	plan, err := Plan(t.Context(), root, PlanOptions{})
	if err != nil {
		t.Fatalf("Plan: %v", err)
	}

	got := plan.Ownership[spec.ComponentDocumentation]
	sort.Strings(got)

	want := []string{"README.md", "docs/guide.md"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestPlanWarnsForInvalidDependencyByDefault(t *testing.T) {
	root := t.TempDir()
	writeConfigTree(t, root, map[string]string{
		"ocidoc.yaml": "schemaVersion: v1beta\ncomponents:\n  documentation:\n    - /README.md\n",
		"README.md":   "[missing](missing.md)\n",
	})

	plan, err := Plan(t.Context(), root, PlanOptions{})
	if err != nil {
		t.Fatalf("Plan: %v", err)
	}
	if len(plan.Warnings) != 1 || !strings.Contains(plan.Warnings[0], "missing.md") {
		t.Fatalf("Warnings = %v, want missing dependency warning", plan.Warnings)
	}
}

func TestPlanStrictRejectsInvalidDependency(t *testing.T) {
	root := t.TempDir()
	writeConfigTree(t, root, map[string]string{
		"ocidoc.yaml": "schemaVersion: v1beta\nsettings:\n  strict: true\ncomponents:\n  documentation:\n    - /README.md\n",
		"README.md":   "[missing](missing.md)\n",
	})

	_, err := Plan(t.Context(), root, PlanOptions{})
	if !errors.Is(err, spec.ErrInvalid) {
		t.Fatalf("Plan error = %v, want spec.ErrInvalid", err)
	}
}
