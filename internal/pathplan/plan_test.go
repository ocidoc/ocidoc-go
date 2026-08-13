// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: Copyright 2026 WoozyMasta
// Source: github.com/ocidoc/ocidoc-go

package pathplan

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"testing"

	"github.com/ocidoc/ocidoc-go/spec"
)

func writeTree(t *testing.T, root string, files map[string]string) {
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

func TestPlanResolvesOwnership(t *testing.T) {
	root := t.TempDir()
	writeTree(t, root, map[string]string{
		"README.md":         "# readme",
		"docs/guide.md":     "guide",
		"LICENSE":           "license",
		"unowned/random.md": "nobody claims this",
	})

	cfg := &spec.BuildConfig{
		SchemaVersion: spec.SchemaVersion,
		Components: map[spec.ComponentType][]string{
			spec.ComponentDocumentation: {"/README.md", "/docs/**"},
			spec.ComponentLicense:       {"/LICENSE"},
		},
	}

	matchers, err := Compile(cfg)
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}

	ownership, err := Plan(root, matchers)
	if err != nil {
		t.Fatalf("Plan: %v", err)
	}

	want := Ownership{
		spec.ComponentDocumentation: {"README.md", "docs/guide.md"},
		spec.ComponentLicense:       {"LICENSE"},
	}

	for name := range ownership {
		sort.Strings(ownership[name])
	}

	if !reflect.DeepEqual(ownership, want) {
		t.Fatalf("got %#v, want %#v", ownership, want)
	}
}

func TestPlanAppliesGlobalIgnore(t *testing.T) {
	root := t.TempDir()
	writeTree(t, root, map[string]string{
		"docs/guide.md":        "guide",
		"docs/internal/x.md":   "internal",
		"docs/internal/pub.md": "restored",
	})

	cfg := &spec.BuildConfig{
		SchemaVersion: spec.SchemaVersion,
		Ignore:        []string{"/docs/internal/**", "!/docs/internal/pub.md"},
		Components: map[spec.ComponentType][]string{
			spec.ComponentDocumentation: {"/docs/**"},
		},
	}

	matchers, err := Compile(cfg)
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}

	ownership, err := Plan(root, matchers)
	if err != nil {
		t.Fatalf("Plan: %v", err)
	}

	got := ownership[spec.ComponentDocumentation]
	sort.Strings(got)

	want := []string{"docs/guide.md", "docs/internal/pub.md"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestPlanRejectsOwnershipOverlap(t *testing.T) {
	root := t.TempDir()
	writeTree(t, root, map[string]string{
		"docs/shared.md": "shared",
	})

	cfg := &spec.BuildConfig{
		SchemaVersion: spec.SchemaVersion,
		Components: map[spec.ComponentType][]string{
			spec.ComponentDocumentation: {"/docs/**"},
			"x-runbooks":                {"/docs/**"},
		},
	}

	matchers, err := Compile(cfg)
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}

	_, err = Plan(root, matchers)
	if err == nil {
		t.Fatal("expected ownership conflict error")
	}

	var conflict *OwnershipConflictError
	if !errors.As(err, &conflict) {
		t.Fatalf("got %v, want *OwnershipConflictError", err)
	}

	if !errors.Is(err, spec.ErrInvalid) {
		t.Fatalf("expected errors.Is(err, spec.ErrInvalid), got %v", err)
	}

	if conflict.Path != "docs/shared.md" {
		t.Fatalf("got conflict path %q, want docs/shared.md", conflict.Path)
	}
}

func TestPlanDereferencesSymlinkAsLogicalBundlePath(t *testing.T) {
	root := t.TempDir()
	writeTree(t, root, map[string]string{"docs/real.md": "real"})

	link := filepath.Join(root, "docs", "linked.md")
	if err := os.Symlink(filepath.Join(root, "docs", "real.md"), link); err != nil {
		t.Skipf("symlink not supported in this environment: %v", err)
	}

	cfg := &spec.BuildConfig{
		SchemaVersion: spec.SchemaVersion,
		Components: map[spec.ComponentType][]string{
			spec.ComponentDocumentation: {"/docs/**"},
		},
	}

	matchers, err := Compile(cfg)
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}

	ownership, err := Plan(root, matchers)
	if err != nil {
		t.Fatalf("Plan: %v", err)
	}

	got := ownership[spec.ComponentDocumentation]

	want := []string{"docs/linked.md", "docs/real.md"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestPlanRejectsSymlinkEscape(t *testing.T) {
	outer := t.TempDir()
	root := filepath.Join(outer, "root")
	writeTree(t, root, map[string]string{"README.md": "readme"})
	outside := filepath.Join(outer, "outside.md")
	if err := os.WriteFile(outside, []byte("outside"), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	if err := os.Symlink(outside, filepath.Join(root, "escape.md")); err != nil {
		t.Skipf("symlink not supported in this environment: %v", err)
	}

	cfg := &spec.BuildConfig{
		SchemaVersion: spec.SchemaVersion,
		Components: map[spec.ComponentType][]string{
			spec.ComponentDocumentation: {"/*.md"},
		},
	}
	matchers, err := Compile(cfg)
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}

	_, err = Plan(root, matchers)
	if !errors.Is(err, spec.ErrInvalid) {
		t.Fatalf("Plan error = %v, want spec.ErrInvalid", err)
	}
}

func TestPlanBuildConfigIgnoreWinsOverComponentMatch(t *testing.T) {
	root := t.TempDir()
	writeTree(t, root, map[string]string{
		"docs/secret.md": "not for shipping",
	})

	cfg := &spec.BuildConfig{
		SchemaVersion: spec.SchemaVersion,
		Ignore:        []string{"/docs/secret.md"},
		Components: map[spec.ComponentType][]string{
			spec.ComponentDocumentation: {"/docs/secret.md"},
		},
	}

	matchers, err := Compile(cfg)
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}

	ownership, err := Plan(root, matchers)
	if err != nil {
		t.Fatalf("Plan: %v", err)
	}

	if len(ownership) != 0 {
		t.Fatalf("got %#v, want empty ownership (build config ignore always wins over a component match)", ownership)
	}
}

func TestPlanEmptyTreeYieldsNoOwnership(t *testing.T) {
	root := t.TempDir()

	cfg := &spec.BuildConfig{
		SchemaVersion: spec.SchemaVersion,
		Components: map[spec.ComponentType][]string{
			spec.ComponentDocumentation: {"/README.md"},
		},
	}

	matchers, err := Compile(cfg)
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}

	ownership, err := Plan(root, matchers)
	if err != nil {
		t.Fatalf("Plan: %v", err)
	}

	if len(ownership) != 0 {
		t.Fatalf("got %#v, want empty ownership", ownership)
	}
}
