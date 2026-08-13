// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: Copyright 2026 WoozyMasta
// Source: github.com/ocidoc/ocidoc-go

package orasrepo

import "testing"

// FuzzParseReference verifies registry references from external callers do not
// panic the ORAS parser or this adapter's error mapping.
func FuzzParseReference(f *testing.F) {
	for _, seed := range []string{
		"registry.example/team/document:latest",
		"registry.example/team/document@sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		"repository", "", "//invalid",
	} {
		f.Add(seed)
	}

	f.Fuzz(func(t *testing.T, reference string) {
		_, _, _ = ParseReference(reference)
	})
}
