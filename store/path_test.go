// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: Copyright 2026 WoozyMasta
// Source: github.com/ocidoc/ocidoc-go

package store

import "testing"

func TestResolvePathExplicitOverridesEnvironment(t *testing.T) {
	t.Setenv("OCIDOC_STORE", "environment-store")

	path, err := ResolvePath("explicit-store")
	if err != nil {
		t.Fatalf("ResolvePath: %v", err)
	}
	if path != "explicit-store" {
		t.Fatalf("got %q, want explicit-store", path)
	}
}

func TestResolvePathUsesEnvironment(t *testing.T) {
	t.Setenv("OCIDOC_STORE", "environment-store")

	path, err := ResolvePath("")
	if err != nil {
		t.Fatalf("ResolvePath: %v", err)
	}
	if path != "environment-store" {
		t.Fatalf("got %q, want environment-store", path)
	}
}
