// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: Copyright 2026 WoozyMasta
// Source: github.com/ocidoc/ocidoc-go

//go:build windows

package atomicfile

import (
	"fmt"
	"os"

	"golang.org/x/sys/windows"
)

// publish moves source to destination, replacing the destination when present.
func publish(source, destination string) error {
	err := windows.MoveFileEx(
		windows.StringToUTF16Ptr(source),
		windows.StringToUTF16Ptr(destination),
		windows.MOVEFILE_REPLACE_EXISTING|windows.MOVEFILE_WRITE_THROUGH,
	)
	if err != nil {
		return fmt.Errorf("replace %q with %q: %w", destination, source, err)
	}

	return nil
}

// publishNoReplace links source at destination without replacing an existing path.
func publishNoReplace(source, destination string) error {
	if err := os.Link(source, destination); err != nil {
		return fmt.Errorf("publish %q without replacing %q: %w", source, destination, err)
	}
	_ = os.Remove(source)

	return nil
}
