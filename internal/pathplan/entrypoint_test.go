// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: Copyright 2026 WoozyMasta
// Source: github.com/ocidoc/ocidoc-go

package pathplan

import (
	"errors"
	"testing"

	"github.com/ocidoc/ocidoc-go/spec"
)

func TestResolveEntrypointsAutoDetectPrefersHigherPriorityCandidate(t *testing.T) {
	ownership := Ownership{
		spec.ComponentDocumentation: {"README", "README.md", "docs/index.md"},
	}

	got, err := ResolveEntrypoints(ownership, nil)
	if err != nil {
		t.Fatalf("ResolveEntrypoints: %v", err)
	}

	if want := "README.md"; got[spec.ComponentDocumentation] != want {
		t.Fatalf("got %q, want %q", got[spec.ComponentDocumentation], want)
	}
}

func TestResolveEntrypointsAutoDetectFallsBackDownThePriorityList(t *testing.T) {
	ownership := Ownership{
		spec.ComponentDocumentation: {"docs/guide.md", "docs/index.md"},
	}

	got, err := ResolveEntrypoints(ownership, nil)
	if err != nil {
		t.Fatalf("ResolveEntrypoints: %v", err)
	}

	if want := "docs/index.md"; got[spec.ComponentDocumentation] != want {
		t.Fatalf("got %q, want %q", got[spec.ComponentDocumentation], want)
	}
}

func TestResolveEntrypointsDocumentationFallsBackToFirstTextDocument(t *testing.T) {
	ownership := Ownership{
		spec.ComponentDocumentation: {"docs/a.png", "docs/guide.md", "docs/zzz.md"},
	}

	got, err := ResolveEntrypoints(ownership, nil)
	if err != nil {
		t.Fatalf("ResolveEntrypoints: %v", err)
	}

	if want := "docs/guide.md"; got[spec.ComponentDocumentation] != want {
		t.Fatalf("got %q, want %q (first text document in sorted order)", got[spec.ComponentDocumentation], want)
	}
}

// TestResolveEntrypointsDocumentationSkipsRSTAndAsciiDoc confirms
// .rst/.adoc are not recognized text-document extensions: a component
// containing only such files falls back to no entrypoint, and one
// containing both an .rst file and a recognized extension still picks
// the recognized one.
func TestResolveEntrypointsDocumentationSkipsRSTAndAsciiDoc(t *testing.T) {
	ownership := Ownership{
		spec.ComponentDocumentation: {"docs/a.adoc", "docs/manual.rst"},
	}

	got, err := ResolveEntrypoints(ownership, nil)
	if err != nil {
		t.Fatalf("ResolveEntrypoints: %v", err)
	}

	if _, ok := got[spec.ComponentDocumentation]; ok {
		t.Fatalf("expected no auto-detected entrypoint among only .rst/.adoc files, got %q", got[spec.ComponentDocumentation])
	}

	ownership[spec.ComponentDocumentation] = append(ownership[spec.ComponentDocumentation], "docs/guide.md")

	got, err = ResolveEntrypoints(ownership, nil)
	if err != nil {
		t.Fatalf("ResolveEntrypoints: %v", err)
	}

	if want := "docs/guide.md"; got[spec.ComponentDocumentation] != want {
		t.Fatalf("got %q, want %q (the one recognized text document, ahead of .rst/.adoc)", got[spec.ComponentDocumentation], want)
	}
}

func TestResolveEntrypointsLicenseExtensionWildcard(t *testing.T) {
	ownership := Ownership{
		spec.ComponentLicense: {"LICENSE.md", "NOTICE"},
	}

	got, err := ResolveEntrypoints(ownership, nil)
	if err != nil {
		t.Fatalf("ResolveEntrypoints: %v", err)
	}

	if want := "LICENSE.md"; got[spec.ComponentLicense] != want {
		t.Fatalf("got %q, want %q", got[spec.ComponentLicense], want)
	}
}

func TestResolveEntrypointsNoMatchLeavesComponentAbsent(t *testing.T) {
	ownership := Ownership{
		spec.ComponentLicense:       {"NOTICE"},
		spec.ComponentCodeOfConduct: {"CODE_OF_CONDUCT.md"},
		"x-runbooks":                {"runbooks/deploy.md"},
	}

	got, err := ResolveEntrypoints(ownership, nil)
	if err != nil {
		t.Fatalf("ResolveEntrypoints: %v", err)
	}

	for _, component := range []spec.ComponentType{spec.ComponentLicense, spec.ComponentCodeOfConduct, "x-runbooks"} {
		if _, ok := got[component]; ok {
			t.Errorf("expected component %q to have no detected entrypoint, got %q", component, got[component])
		}
	}
}

func TestResolveEntrypointsExplicitOverridesAutoDetection(t *testing.T) {
	ownership := Ownership{
		spec.ComponentDocumentation: {"README.md", "docs/index.md"},
	}

	got, err := ResolveEntrypoints(ownership, map[spec.ComponentType]string{
		spec.ComponentDocumentation: "/docs/index.md",
	})
	if err != nil {
		t.Fatalf("ResolveEntrypoints: %v", err)
	}

	if want := "docs/index.md"; got[spec.ComponentDocumentation] != want {
		t.Fatalf("got %q, want %q", got[spec.ComponentDocumentation], want)
	}
}

func TestResolveEntrypointsRejectsPathOutsideComponent(t *testing.T) {
	ownership := Ownership{
		spec.ComponentDocumentation: {"README.md"},
	}

	_, err := ResolveEntrypoints(ownership, map[spec.ComponentType]string{
		spec.ComponentDocumentation: "/docs/other.md",
	})
	if err == nil {
		t.Fatal("expected error for entrypoint not among the component's planned files")
	}

	var epErr *EntrypointError
	if !errors.As(err, &epErr) {
		t.Fatalf("got %v, want *EntrypointError", err)
	}

	if !errors.Is(err, spec.ErrInvalid) {
		t.Fatalf("expected errors.Is(err, spec.ErrInvalid), got %v", err)
	}
}

func TestResolveEntrypointsRejectsUnplannedComponent(t *testing.T) {
	ownership := Ownership{}

	_, err := ResolveEntrypoints(ownership, map[spec.ComponentType]string{
		spec.ComponentLicense: "/LICENSE",
	})
	if err == nil {
		t.Fatal("expected error for entrypoint on a component with no planned files")
	}
}
