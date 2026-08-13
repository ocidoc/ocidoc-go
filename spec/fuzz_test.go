// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: Copyright 2026 WoozyMasta
// Source: github.com/ocidoc/ocidoc-go

package spec

import "testing"

// FuzzValidateIdentifiers verifies component and bundle-path validation stays
// panic-free for untrusted configuration values.
func FuzzValidateIdentifiers(f *testing.F) {
	for _, seed := range []string{"documentation", "x-custom", "../escape", "README.md", "", "a/b"} {
		f.Add(seed)
	}

	f.Fuzz(func(t *testing.T, value string) {
		_ = ValidateComponentType(value)
		_ = ValidateBundlePath(value)
		_ = ValidateUserAnnotations(map[string]string{value: value})
	})
}
