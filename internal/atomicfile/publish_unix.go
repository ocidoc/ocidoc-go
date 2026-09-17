// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: Copyright 2026 WoozyMasta
// Source: github.com/ocidoc/ocidoc-go

//go:build !windows

package atomicfile

import (
	"fmt"
	"os"
)

// publish renames source to destination, replacing the destination when present.
func publish(source, destination string) error {
	if err := os.Rename(source, destination); err != nil {
		return fmt.Errorf("rename %q to %q: %w", source, destination, err)
	}

	return nil
}

// publishNoReplace links source at destination without replacing an existing path.
func publishNoReplace(source, destination string) error {
	if err := os.Link(source, destination); err != nil {
		return fmt.Errorf("publish %q without replacing %q: %w", source, destination, err)
	}

	// The destination link is already the published result.
	// Failure to remove the private link is harmless
	// and must not turn a successful publication into an apparent failed write.
	_ = os.Remove(source)

	return nil
}
