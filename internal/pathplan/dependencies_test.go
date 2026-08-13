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

func TestDiscoverDependenciesAddsLinkedAsset(t *testing.T) {
	root := t.TempDir()
	writeTree(t, root, map[string]string{
		"README.md":    "![Diagram](diagram.png)\n",
		"diagram.png":  "fake png",
		"CHANGELOG.md": "no links here",
	})

	ownership := Ownership{
		spec.ComponentDocumentation: {"README.md"},
		spec.ComponentChangelog:     {"CHANGELOG.md"},
	}

	got, warnings, err := DiscoverDependencies(root, ownership, DependencyOptions{})
	if err != nil {
		t.Fatalf("DiscoverDependencies: %v", err)
	}
	if len(warnings) != 0 {
		t.Fatalf("unexpected warnings: %v", warnings)
	}

	want := []string{"README.md", "diagram.png"}
	sort.Strings(got[spec.ComponentDocumentation])

	if !reflect.DeepEqual(got[spec.ComponentDocumentation], want) {
		t.Fatalf("got %v, want %v", got[spec.ComponentDocumentation], want)
	}

	if !reflect.DeepEqual(got[spec.ComponentChangelog], []string{"CHANGELOG.md"}) {
		t.Fatalf("changelog ownership changed unexpectedly: %v", got[spec.ComponentChangelog])
	}
}

func TestDiscoverDependenciesRecursesIntoDiscoveredMarkdown(t *testing.T) {
	root := t.TempDir()
	writeTree(t, root, map[string]string{
		"README.md":     "[guide](docs/guide.md)\n",
		"docs/guide.md": "![shot](shot.png)\n",
		"docs/shot.png": "fake png",
	})

	ownership := Ownership{
		spec.ComponentDocumentation: {"README.md"},
	}

	got, _, err := DiscoverDependencies(root, ownership, DependencyOptions{})
	if err != nil {
		t.Fatalf("DiscoverDependencies: %v", err)
	}

	want := []string{"README.md", "docs/guide.md", "docs/shot.png"}
	sort.Strings(got[spec.ComponentDocumentation])

	if !reflect.DeepEqual(got[spec.ComponentDocumentation], want) {
		t.Fatalf("got %v, want %v", got[spec.ComponentDocumentation], want)
	}
}

func TestDiscoverDependenciesDoesNotDuplicateAlreadyOwnedPath(t *testing.T) {
	root := t.TempDir()
	writeTree(t, root, map[string]string{
		"README.md": "[license](LICENSE)\n",
		"LICENSE":   "MIT",
	})

	ownership := Ownership{
		spec.ComponentDocumentation: {"README.md"},
		spec.ComponentLicense:       {"LICENSE"},
	}

	got, _, err := DiscoverDependencies(root, ownership, DependencyOptions{})
	if err != nil {
		t.Fatalf("DiscoverDependencies: %v", err)
	}

	if !reflect.DeepEqual(got[spec.ComponentDocumentation], []string{"README.md"}) {
		t.Fatalf("expected LICENSE to stay owned by license, got documentation=%v", got[spec.ComponentDocumentation])
	}

	if !reflect.DeepEqual(got[spec.ComponentLicense], []string{"LICENSE"}) {
		t.Fatalf("got license=%v, want unchanged [LICENSE]", got[spec.ComponentLicense])
	}
}

func TestDiscoverDependenciesConflictBetweenSiblingComponents(t *testing.T) {
	root := t.TempDir()
	writeTree(t, root, map[string]string{
		"README.md":    "[shared](shared.md)\n",
		"CHANGELOG.md": "[shared](shared.md)\n",
		"shared.md":    "shared content",
	})

	ownership := Ownership{
		spec.ComponentDocumentation: {"README.md"},
		spec.ComponentChangelog:     {"CHANGELOG.md"},
	}

	_, _, err := DiscoverDependencies(root, ownership, DependencyOptions{})
	if err == nil {
		t.Fatal("expected an ownership conflict error")
	}

	var conflict *OwnershipConflictError
	if !errors.As(err, &conflict) {
		t.Fatalf("got %v, want *OwnershipConflictError", err)
	}

	if !errors.Is(err, spec.ErrInvalid) {
		t.Fatalf("expected errors.Is(err, spec.ErrInvalid), got %v", err)
	}

	if conflict.Path != "shared.md" {
		t.Fatalf("got conflict path %q, want shared.md", conflict.Path)
	}
}

func TestDiscoverDependenciesLeavesMissingReferenceUnresolved(t *testing.T) {
	root := t.TempDir()
	writeTree(t, root, map[string]string{
		"README.md": "[gone](missing.md)\n",
	})

	ownership := Ownership{
		spec.ComponentDocumentation: {"README.md"},
	}

	got, warnings, err := DiscoverDependencies(root, ownership, DependencyOptions{})
	if err != nil {
		t.Fatalf("DiscoverDependencies: %v", err)
	}

	if !reflect.DeepEqual(got[spec.ComponentDocumentation], []string{"README.md"}) {
		t.Fatalf("got %v, want unchanged [README.md]", got[spec.ComponentDocumentation])
	}
	if len(warnings) != 1 {
		t.Fatalf("warnings = %v, want one missing-dependency warning", warnings)
	}
}

func TestDiscoverDependenciesDoesNotMutateInput(t *testing.T) {
	root := t.TempDir()
	writeTree(t, root, map[string]string{
		"README.md": "![img](img.png)\n",
		"img.png":   "fake png",
	})

	ownership := Ownership{
		spec.ComponentDocumentation: {"README.md"},
	}

	if _, _, err := DiscoverDependencies(root, ownership, DependencyOptions{}); err != nil {
		t.Fatalf("DiscoverDependencies: %v", err)
	}

	if !reflect.DeepEqual(ownership[spec.ComponentDocumentation], []string{"README.md"}) {
		t.Fatalf("input ownership was mutated: %v", ownership[spec.ComponentDocumentation])
	}
}

func TestDiscoverDependenciesStrictRejectsMissingReference(t *testing.T) {
	root := t.TempDir()
	writeTree(t, root, map[string]string{"README.md": "[gone](missing.md)\n"})
	ownership := Ownership{spec.ComponentDocumentation: {"README.md"}}

	_, _, err := DiscoverDependencies(root, ownership, DependencyOptions{Strict: true})
	if !errors.Is(err, spec.ErrInvalid) {
		t.Fatalf("error = %v, want spec.ErrInvalid", err)
	}
}

func TestDiscoverDependenciesPreservesSymlinkBundlePath(t *testing.T) {
	root := t.TempDir()
	writeTree(t, root, map[string]string{
		"README.md":     "[guide](guide.md)\n",
		"docs/guide.md": "guide",
	})
	if err := os.Symlink(filepath.Join(root, "docs", "guide.md"), filepath.Join(root, "guide.md")); err != nil {
		t.Skipf("create symlink: %v", err)
	}
	ownership := Ownership{spec.ComponentDocumentation: {"README.md"}}

	got, warnings, err := DiscoverDependencies(root, ownership, DependencyOptions{})
	if err != nil {
		t.Fatalf("DiscoverDependencies: %v", err)
	}
	if len(warnings) != 0 {
		t.Fatalf("unexpected warnings: %v", warnings)
	}
	want := []string{"README.md", "guide.md"}
	if !reflect.DeepEqual(got[spec.ComponentDocumentation], want) {
		t.Fatalf("ownership = %v, want %v", got[spec.ComponentDocumentation], want)
	}
}

func TestDiscoverDependenciesAppliesGlobalIgnore(t *testing.T) {
	root := t.TempDir()
	writeTree(t, root, map[string]string{
		"README.md":        "[internal](docs/internal.md)\n",
		"docs/internal.md": "internal",
	})
	matchers, err := Compile(&spec.BuildConfig{
		Ignore: []string{"/docs/internal.md"},
		Components: map[spec.ComponentType][]string{
			spec.ComponentDocumentation: {"/README.md"},
		},
	})
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}
	ownership := Ownership{spec.ComponentDocumentation: {"README.md"}}

	got, warnings, err := DiscoverDependencies(root, ownership, DependencyOptions{Ignore: matchers.Ignore})
	if err != nil {
		t.Fatalf("DiscoverDependencies: %v", err)
	}
	if len(warnings) != 0 {
		t.Fatalf("unexpected warnings: %v", warnings)
	}
	if !reflect.DeepEqual(got[spec.ComponentDocumentation], []string{"README.md"}) {
		t.Fatalf("ownership = %v, want ignored dependency omitted", got[spec.ComponentDocumentation])
	}
}
