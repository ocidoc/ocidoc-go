// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: Copyright 2026 WoozyMasta
// Source: github.com/ocidoc/ocidoc-go

package store

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
)

// DefaultPath returns the platform-specific per-user OCIDoc store location.
func DefaultPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve user home directory: %w", err)
	}

	switch runtime.GOOS {
	case "windows":
		if localAppData := os.Getenv("LOCALAPPDATA"); localAppData != "" {
			return filepath.Join(localAppData, "OCIDoc"), nil
		}
		return filepath.Join(home, "AppData", "Local", "OCIDoc"), nil
	case "darwin":
		return filepath.Join(home, "Library", "Application Support", "OCIDoc"), nil
	default:
		dataHome := os.Getenv("XDG_DATA_HOME")
		if dataHome == "" {
			dataHome = filepath.Join(home, ".local", "share")
		}
		return filepath.Join(dataHome, ".ocidoc"), nil
	}
}

// ResolvePath returns explicitPath when set, then OCIDOC_STORE, then the
// platform-specific default path.
func ResolvePath(explicitPath string) (string, error) {
	if explicitPath != "" {
		return explicitPath, nil
	}
	if path := os.Getenv("OCIDOC_STORE"); path != "" {
		return path, nil
	}

	return DefaultPath()
}
