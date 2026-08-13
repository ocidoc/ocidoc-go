// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: Copyright 2026 WoozyMasta
// Source: github.com/ocidoc/ocidoc-go

package spec

import (
	"errors"
	"testing"

	"github.com/opencontainers/go-digest"
)

func TestValidateBundlePathAccepts(t *testing.T) {
	for _, path := range []string{"README.md", "docs/index.md", "a/b/c.txt", "文档.md"} {
		if err := ValidateBundlePath(path); err != nil {
			t.Errorf("ValidateBundlePath(%q): unexpected error: %v", path, err)
		}
	}
}

func TestValidateBundlePathRejects(t *testing.T) {
	cases := map[string]string{
		"empty":           "",
		"absolute":        "/README.md",
		"parent":          "../README.md",
		"parent-nested":   "docs/../README.md",
		"dot-segment":     "./README.md",
		"empty-segment":   "docs//README.md",
		"backslash":       `docs\README.md`,
		"drive-letter":    `C:README.md`,
		"nul-byte":        "read\x00me.md",
		"trailing-dot":    "docs/notes.",
		"trailing-space":  "docs/notes ",
		"leading-space":   "docs/ notes.md",
		"reserved-name":   "docs/NUL.md",
		"reserved-mixed":  "docs/Con.txt",
		"reserved-no-ext": "docs/aux",
	}

	for name, path := range cases {
		t.Run(name, func(t *testing.T) {
			err := ValidateBundlePath(path)
			if err == nil {
				t.Fatalf("ValidateBundlePath(%q): expected error, got nil", path)
			}

			if !errors.Is(err, ErrInvalid) {
				t.Fatalf("expected errors.Is(err, ErrInvalid), got %v", err)
			}
		})
	}
}

func TestValidateArtifactConfigValid(t *testing.T) {
	cfg := &ArtifactConfig{
		SchemaVersion: SchemaVersion,
		Components: map[ComponentType]ComponentConfig{
			ComponentDocumentation: {Entrypoint: "README.md"},
			ComponentLicense:       {Entrypoint: "LICENSE"},
			"x-runbooks":           {},
		},
	}

	if err := ValidateArtifactConfig(cfg); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestValidateArtifactConfigRejectsMissingSchemaVersion(t *testing.T) {
	cfg := &ArtifactConfig{
		Components: map[ComponentType]ComponentConfig{ComponentDocumentation: {}},
	}

	err := ValidateArtifactConfig(cfg)
	if err == nil {
		t.Fatal("expected error for missing schemaVersion")
	}

	var verr *ValidationError
	if !errors.As(err, &verr) || verr.Code != CodeMissingSchemaVersion {
		t.Fatalf("got %v, want CodeMissingSchemaVersion", err)
	}
}

func TestValidateArtifactConfigRejectsUnsupportedSchemaVersion(t *testing.T) {
	cfg := &ArtifactConfig{
		SchemaVersion: "v2",
		Components:    map[ComponentType]ComponentConfig{ComponentDocumentation: {}},
	}

	err := ValidateArtifactConfig(cfg)

	var verr *ValidationError
	if !errors.As(err, &verr) || verr.Code != CodeUnsupportedSchemaVersion {
		t.Fatalf("got %v, want CodeUnsupportedSchemaVersion", err)
	}
}

func TestValidateArtifactConfigRejectsUnsupportedSchemaID(t *testing.T) {
	cfg := &ArtifactConfig{
		Schema:        "https://example.com/other-schema.json",
		SchemaVersion: SchemaVersion,
		Components:    map[ComponentType]ComponentConfig{ComponentDocumentation: {}},
	}

	err := ValidateArtifactConfig(cfg)

	var verr *ValidationError
	if !errors.As(err, &verr) || verr.Code != CodeUnsupportedSchemaVersion {
		t.Fatalf("got %v, want CodeUnsupportedSchemaVersion", err)
	}
}

func TestValidateArtifactConfigRejectsNoComponents(t *testing.T) {
	cfg := &ArtifactConfig{SchemaVersion: SchemaVersion}

	err := ValidateArtifactConfig(cfg)

	var verr *ValidationError
	if !errors.As(err, &verr) || verr.Code != CodeNoComponents {
		t.Fatalf("got %v, want CodeNoComponents", err)
	}
}

func TestValidateArtifactConfigRejectsBadEntrypoint(t *testing.T) {
	cfg := &ArtifactConfig{
		SchemaVersion: SchemaVersion,
		Components: map[ComponentType]ComponentConfig{
			ComponentDocumentation: {Entrypoint: "/README.md"},
		},
	}

	err := ValidateArtifactConfig(cfg)

	var verr *ValidationError
	if !errors.As(err, &verr) || verr.Code != CodeInvalidPath {
		t.Fatalf("got %v, want CodeInvalidPath", err)
	}
}

func TestValidateArtifactConfigRejectsNil(t *testing.T) {
	if err := ValidateArtifactConfig(nil); err == nil {
		t.Fatal("expected error for nil config")
	}
}

func TestDocumentationTagSHA256(t *testing.T) {
	d := digest.NewDigestFromEncoded(digest.SHA256, "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855")

	tag, err := DocumentationTag(d)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	want := "sha256-e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855.doc"
	if tag != want {
		t.Fatalf("got %q, want %q", tag, want)
	}
}

func TestDocumentationTagRejectsNonSHA256(t *testing.T) {
	d := digest.NewDigestFromEncoded(digest.SHA512, "cf83e1357eefb8bdf1542850d66d8007d620e4050b5715dc83f4a921d36ce9ce47d0d13c5d85f2b0ff8318d2877eec2f63b931bd47417a81a538327af927da3e")

	_, err := DocumentationTag(d)
	if err == nil {
		t.Fatal("expected error for non-sha256 digest")
	}

	var verr *ValidationError
	if !errors.As(err, &verr) || verr.Code != CodeUnsupportedDigestAlgorithm {
		t.Fatalf("got %v, want CodeUnsupportedDigestAlgorithm", err)
	}
}

func TestDocumentationTagRejectsInvalidDigest(t *testing.T) {
	_, err := DocumentationTag(digest.Digest("not-a-digest"))
	if err == nil {
		t.Fatal("expected error for malformed digest")
	}
}
