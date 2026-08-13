// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: Copyright 2026 WoozyMasta
// Source: github.com/ocidoc/ocidoc-go

package spec

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// Conformance fixtures under ../testdata/conformance are internal test data,
// not a public API or a stable conformance package.

func TestConformanceArtifactConfigValid(t *testing.T) {
	for _, path := range conformanceFiles(t, "artifact-config/valid") {
		t.Run(filepath.Base(path), func(t *testing.T) {
			var cfg ArtifactConfig
			if err := json.Unmarshal(readFixture(t, path), &cfg); err != nil {
				t.Fatalf("Unmarshal: %v", err)
			}

			if err := ValidateArtifactConfig(&cfg); err != nil {
				t.Fatalf("ValidateArtifactConfig: %v", err)
			}
		})
	}
}

func TestConformanceArtifactConfigInvalid(t *testing.T) {
	for _, path := range conformanceFiles(t, "artifact-config/invalid") {
		t.Run(filepath.Base(path), func(t *testing.T) {
			var cfg ArtifactConfig
			if err := json.Unmarshal(readFixture(t, path), &cfg); err != nil {
				// A fixture may be invalid at the JSON level too; that
				// still demonstrates rejection.
				return
			}

			if err := ValidateArtifactConfig(&cfg); err == nil {
				t.Fatalf("ValidateArtifactConfig: expected error for %s, got nil", path)
			}
		})
	}
}

func TestConformanceBuildConfigValid(t *testing.T) {
	for _, path := range conformanceFiles(t, "build-config/valid") {
		t.Run(filepath.Base(path), func(t *testing.T) {
			var cfg BuildConfig
			if err := json.Unmarshal(readFixture(t, path), &cfg); err != nil {
				t.Fatalf("Unmarshal: %v", err)
			}

			if err := ValidateBuildConfig(&cfg); err != nil {
				t.Fatalf("ValidateBuildConfig: %v", err)
			}
		})
	}
}

func TestConformanceBuildConfigInvalid(t *testing.T) {
	for _, path := range conformanceFiles(t, "build-config/invalid") {
		t.Run(filepath.Base(path), func(t *testing.T) {
			var cfg BuildConfig
			if err := json.Unmarshal(readFixture(t, path), &cfg); err != nil {
				return
			}
			if err := ValidateBuildConfig(&cfg); err == nil {
				t.Fatalf("ValidateBuildConfig: expected error for %s, got nil", path)
			}
		})
	}
}

// conformanceFiles lists the fixture files directly under ../testdata/conformance/<sub>,
// failing the test if none are found so a broken path silently skips coverage instead of passing empty.
func conformanceFiles(t *testing.T, sub string) []string {
	t.Helper()

	dir := filepath.Join("..", "testdata", "conformance", filepath.FromSlash(sub))

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("ReadDir(%s): %v", dir, err)
	}

	var files []string

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		files = append(files, filepath.Join(dir, entry.Name()))
	}

	if len(files) == 0 {
		t.Fatalf("no conformance fixtures found in %s", dir)
	}

	return files
}

func readFixture(t *testing.T, path string) []byte {
	t.Helper()

	//nolint:gosec // fixed test fixture directory, not user input.
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(%s): %v", path, err)
	}

	return data
}
