// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: Copyright 2026 WoozyMasta
// Source: github.com/ocidoc/ocidoc-go

package spec

import (
	"errors"
	"testing"
)

func TestValidateBundlePathsAccepts(t *testing.T) {
	err := ValidateBundlePaths([]string{"README.md", "docs/index.md", "LICENSE"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestValidateBundlePathsRejectsInvalidPath(t *testing.T) {
	err := ValidateBundlePaths([]string{"README.md", "/absolute.md"})
	if err == nil {
		t.Fatal("expected error for invalid path")
	}

	var verr *ValidationError
	if !errors.As(err, &verr) || verr.Code != CodeInvalidPath {
		t.Fatalf("got %v, want CodeInvalidPath", err)
	}
}

func TestValidateBundlePathsRejectsExactDuplicate(t *testing.T) {
	err := ValidateBundlePaths([]string{"README.md", "docs/a.md", "README.md"})
	if err == nil {
		t.Fatal("expected error for duplicate path")
	}

	var verr *ValidationError
	if !errors.As(err, &verr) || verr.Code != CodePathCollision {
		t.Fatalf("got %v, want CodePathCollision", err)
	}
}

func TestValidateBundlePathsRejectsCaseInsensitiveCollision(t *testing.T) {
	err := ValidateBundlePaths([]string{"docs/Readme.md", "docs/README.md"})
	if err == nil {
		t.Fatal("expected error for case-insensitive collision")
	}

	var verr *ValidationError
	if !errors.As(err, &verr) || verr.Code != CodePathCollision {
		t.Fatalf("got %v, want CodePathCollision", err)
	}
}

func TestValidateUserAnnotationsAccepts(t *testing.T) {
	err := ValidateUserAnnotations(map[string]string{
		"org.opencontainers.image.title":   "Project documentation",
		"org.opencontainers.image.version": "1.2.3",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestValidateUserAnnotationsRejectsReservedNamespace(t *testing.T) {
	err := ValidateUserAnnotations(map[string]string{"org.ocidoc.schema": "v1beta"})
	if err == nil {
		t.Fatal("expected error for reserved annotation namespace")
	}

	var verr *ValidationError
	if !errors.As(err, &verr) || verr.Code != CodeReservedAnnotation {
		t.Fatalf("got %v, want CodeReservedAnnotation", err)
	}

	if verr.Annotation != "org.ocidoc.schema" {
		t.Fatalf("got annotation %q, want org.ocidoc.schema", verr.Annotation)
	}
}
