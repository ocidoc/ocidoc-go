// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: Copyright 2026 WoozyMasta
// Source: github.com/ocidoc/ocidoc-go

package spec

import (
	"errors"
	"testing"
)

func validBuildConfig() *BuildConfig {
	return &BuildConfig{
		SchemaVersion: SchemaVersion,
		Components: map[ComponentType][]string{
			ComponentDocumentation: {"/README.md", "/docs/**"},
			ComponentLicense:       {"/LICENSE"},
		},
		Entrypoints: map[ComponentType]string{
			ComponentDocumentation: "/docs/index.md",
		},
	}
}

func TestValidateBuildConfigValid(t *testing.T) {
	if err := ValidateBuildConfig(validBuildConfig()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestValidateBuildConfigRejectsNil(t *testing.T) {
	if err := ValidateBuildConfig(nil); err == nil {
		t.Fatal("expected error for nil config")
	}
}

func TestValidateBuildConfigRejectsMissingSchemaVersion(t *testing.T) {
	cfg := validBuildConfig()
	cfg.SchemaVersion = ""

	err := ValidateBuildConfig(cfg)

	var verr *ValidationError
	if !errors.As(err, &verr) || verr.Code != CodeMissingSchemaVersion {
		t.Fatalf("got %v, want CodeMissingSchemaVersion", err)
	}
}

func TestValidateBuildConfigRejectsUnsupportedSchemaVersion(t *testing.T) {
	cfg := validBuildConfig()
	cfg.SchemaVersion = "v2"

	err := ValidateBuildConfig(cfg)

	var verr *ValidationError
	if !errors.As(err, &verr) || verr.Code != CodeUnsupportedSchemaVersion {
		t.Fatalf("got %v, want CodeUnsupportedSchemaVersion", err)
	}
}

func TestValidateBuildConfigRejectsNoComponents(t *testing.T) {
	cfg := validBuildConfig()
	cfg.Components = nil

	err := ValidateBuildConfig(cfg)

	var verr *ValidationError
	if !errors.As(err, &verr) || verr.Code != CodeNoComponents {
		t.Fatalf("got %v, want CodeNoComponents", err)
	}
}

func TestValidateBuildConfigRejectsBadComponentName(t *testing.T) {
	cfg := validBuildConfig()
	cfg.Components["runbooks"] = []string{"/runbooks/**"}

	err := ValidateBuildConfig(cfg)

	var verr *ValidationError
	if !errors.As(err, &verr) || verr.Code != CodeUnknownComponentName {
		t.Fatalf("got %v, want CodeUnknownComponentName", err)
	}
}

func TestValidateBuildConfigRejectsEmptyComponentRules(t *testing.T) {
	cfg := validBuildConfig()
	cfg.Components[ComponentSecurity] = []string{}

	err := ValidateBuildConfig(cfg)

	var verr *ValidationError
	if !errors.As(err, &verr) || verr.Code != CodeEmptyComponentRules {
		t.Fatalf("got %v, want CodeEmptyComponentRules", err)
	}
}

func TestValidateBuildConfigRejectsUndeclaredEntrypointComponent(t *testing.T) {
	cfg := validBuildConfig()
	cfg.Entrypoints[ComponentChangelog] = "/CHANGELOG.md"

	err := ValidateBuildConfig(cfg)

	var verr *ValidationError
	if !errors.As(err, &verr) || verr.Code != CodeUndeclaredComponent {
		t.Fatalf("got %v, want CodeUndeclaredComponent", err)
	}
}

func TestValidateBuildConfigRejectsReservedAnnotation(t *testing.T) {
	cfg := validBuildConfig()
	cfg.Annotations = map[string]string{"org.ocidoc.schema": "v1beta"}

	err := ValidateBuildConfig(cfg)

	var verr *ValidationError
	if !errors.As(err, &verr) || verr.Code != CodeReservedAnnotation {
		t.Fatalf("got %v, want CodeReservedAnnotation", err)
	}
}

func TestValidateBuildConfigRejectsUnsupportedCompression(t *testing.T) {
	cfg := validBuildConfig()
	cfg.Settings.Compression = &CompressionSettings{Type: "lzma"}

	err := ValidateBuildConfig(cfg)

	var verr *ValidationError
	if !errors.As(err, &verr) || verr.Code != CodeUnsupportedCompression {
		t.Fatalf("got %v, want CodeUnsupportedCompression", err)
	}
}

func TestValidateBuildConfigAcceptsSupportedCompression(t *testing.T) {
	cfg := validBuildConfig()
	cfg.Settings.Compression = &CompressionSettings{Type: CompressionZstd}

	if err := ValidateBuildConfig(cfg); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}
