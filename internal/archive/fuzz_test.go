// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: Copyright 2026 WoozyMasta
// Source: github.com/ocidoc/ocidoc-go

package archive

import "testing"

// FuzzSanitizeExtractPath verifies untrusted tar entry names never panic path
// validation before extraction uses them as filesystem paths.
func FuzzSanitizeExtractPath(f *testing.F) {
	for _, seed := range []string{"README.md", "../escape", "/absolute", `dir\\file`, "", "a/../../b"} {
		f.Add(seed)
	}

	f.Fuzz(func(t *testing.T, name string) {
		_, _ = sanitizeExtractPath(name)
	})
}
