// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: Copyright 2026 WoozyMasta
// Source: github.com/ocidoc/ocidoc-go

package pathplan

import (
	"slices"
	"testing"

	"github.com/ocidoc/ocidoc-go/spec"
)

func TestCompileIgnorePolarity(t *testing.T) {
	cfg := &spec.BuildConfig{
		SchemaVersion: spec.SchemaVersion,
		Ignore:        []string{"/docs/internal/**", "!/docs/internal/public.md"},
		Components:    map[spec.ComponentType]spec.ComponentBuildConfig{spec.ComponentDocumentation: {Paths: []string{"/docs/**"}}},
	}

	matchers, err := Compile(cfg)
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}

	if !matchers.Ignore.Excluded("docs/internal/secret.md", false) {
		t.Error("expected docs/internal/secret.md to be excluded by ignore rules")
	}

	if matchers.Ignore.Excluded("docs/internal/public.md", false) {
		t.Error("expected docs/internal/public.md to be restored by the negated ignore rule")
	}

	if matchers.Ignore.Excluded("docs/guide.md", false) {
		t.Error("expected docs/guide.md to be unaffected by ignore rules")
	}
}

func TestCompileComponentPolarity(t *testing.T) {
	cfg := &spec.BuildConfig{
		SchemaVersion: spec.SchemaVersion,
		Components: map[spec.ComponentType]spec.ComponentBuildConfig{
			spec.ComponentDocumentation: {Paths: []string{"/docs/**", "!/docs/internal/**"}},
		},
	}

	matchers, err := Compile(cfg)
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}

	doc := matchers.Components[spec.ComponentDocumentation]

	if !doc.Included("docs/guide.md", false) {
		t.Error("expected docs/guide.md to be included by the plain component rule")
	}

	if doc.Included("docs/internal/secret.md", false) {
		t.Error("expected docs/internal/secret.md to be excluded by the negated component rule")
	}

	if doc.Included("README.md", false) {
		t.Error("expected README.md to be excluded by default (no matching rule)")
	}
}

func TestCompileComponentBraceExpansion(t *testing.T) {
	cfg := &spec.BuildConfig{
		SchemaVersion: spec.SchemaVersion,
		Components: map[spec.ComponentType]spec.ComponentBuildConfig{
			spec.ComponentLicense: {Paths: []string{"/{LICENSE,LICENCE}{,.md,.txt}"}},
		},
	}

	matchers, err := Compile(cfg)
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}

	license := matchers.Components[spec.ComponentLicense]

	for _, path := range []string{"LICENSE", "LICENCE", "LICENSE.md", "LICENCE.txt"} {
		if !license.Included(path, false) {
			t.Errorf("expected %q to be included via brace expansion", path)
		}
	}

	if license.Included("LICENSE.rst", false) {
		t.Error("did not expect LICENSE.rst to match {,.md,.txt}")
	}
}

func TestCompileNilIgnoreWhenUnset(t *testing.T) {
	cfg := &spec.BuildConfig{
		SchemaVersion: spec.SchemaVersion,
		Components:    map[spec.ComponentType]spec.ComponentBuildConfig{spec.ComponentLicense: {Paths: []string{"/LICENSE"}}},
	}

	matchers, err := Compile(cfg)
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}

	if matchers.Ignore != nil {
		t.Fatal("expected nil Ignore matcher when the build config declares no ignore rules")
	}
}

func TestClassifyLocalesUsesResolvedDocumentsOnly(t *testing.T) {
	ownership := Ownership{
		spec.ComponentDocumentation: {"README.md", "docs/guide.md", "docs/logo.svg", "docs/README"},
		spec.ComponentLicense:       {"LICENSE"},
	}
	locales, err := ClassifyLocales(map[spec.ComponentType]spec.ComponentBuildConfig{
		spec.ComponentDocumentation: {Locales: map[string]spec.BuildLocaleConfig{
			"ru": {Paths: []string{"/docs/**", "!/docs/README"}},
			"en": {Paths: []string{"/**"}},
		}},
	}, ownership)
	if err != nil {
		t.Fatalf("ClassifyLocales: %v", err)
	}
	if got, want := locales[spec.ComponentDocumentation]["ru"], []string{"docs/guide.md"}; !slices.Equal(got, want) {
		t.Fatalf("ru = %v, want %v", got, want)
	}
	if got, want := locales[spec.ComponentDocumentation]["en"], []string{"README.md", "docs/README", "docs/guide.md"}; !slices.Equal(got, want) {
		t.Fatalf("en = %v, want %v", got, want)
	}
}
