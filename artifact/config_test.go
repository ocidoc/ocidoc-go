// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: Copyright 2026 WoozyMasta
// Source: github.com/ocidoc/ocidoc-go

package artifact

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/ocidoc/ocidoc-go/internal/pathplan"
	"github.com/ocidoc/ocidoc-go/spec"
)

func TestLoadBuildConfigEmbeddedDefault(t *testing.T) {
	root := t.TempDir()

	cfg, err := LoadBuildConfig(root, "")
	if err != nil {
		t.Fatalf("LoadBuildConfig: %v", err)
	}

	if cfg.SchemaVersion != spec.SchemaVersion {
		t.Fatalf("got schemaVersion %q, want %q", cfg.SchemaVersion, spec.SchemaVersion)
	}

	for _, want := range []spec.ComponentType{
		spec.ComponentDocumentation, spec.ComponentLicense, spec.ComponentChangelog,
		spec.ComponentReleaseNotes, spec.ComponentSecurity, spec.ComponentContributing,
		spec.ComponentCodeOfConduct, spec.ComponentSupport,
	} {
		if len(cfg.Components[want]) == 0 {
			t.Errorf("embedded default: expected non-empty rules for component %q", want)
		}
	}

	if len(cfg.Components) != 8 {
		t.Fatalf("got %d components in embedded default, want 8", len(cfg.Components))
	}

	if _, err := pathplan.Compile(cfg); err != nil {
		t.Fatalf("embedded default must compile through pathplan: %v", err)
	}
}

func TestDefaultBuildConfigMatchesLoadBuildConfigFallback(t *testing.T) {
	root := t.TempDir()

	viaLoad, err := LoadBuildConfig(root, "")
	if err != nil {
		t.Fatalf("LoadBuildConfig: %v", err)
	}

	viaDefault, err := DefaultBuildConfig()
	if err != nil {
		t.Fatalf("DefaultBuildConfig: %v", err)
	}

	if !reflect.DeepEqual(viaLoad, viaDefault) {
		t.Fatalf("DefaultBuildConfig() = %+v, want %+v (LoadBuildConfig's embedded-default fallback)", viaDefault, viaLoad)
	}
}

func TestLoadBuildConfigConventionalYAML(t *testing.T) {
	root := t.TempDir()

	writeFile(t, filepath.Join(root, "ocidoc.yaml"), `
schemaVersion: v1beta
components:
  documentation:
    - /README.md
`)

	cfg, err := LoadBuildConfig(root, "")
	if err != nil {
		t.Fatalf("LoadBuildConfig: %v", err)
	}

	if got := cfg.Components[spec.ComponentDocumentation]; len(got) != 1 || got[0] != "/README.md" {
		t.Fatalf("got documentation rules %v, want [/README.md]", got)
	}
}

func TestLoadBuildConfigConventionalJSON(t *testing.T) {
	root := t.TempDir()

	writeFile(t, filepath.Join(root, "ocidoc.json"), `{
		"schemaVersion": "v1beta",
		"components": {"license": ["/LICENSE"]}
	}`)

	cfg, err := LoadBuildConfig(root, "")
	if err != nil {
		t.Fatalf("LoadBuildConfig: %v", err)
	}

	if got := cfg.Components[spec.ComponentLicense]; len(got) != 1 || got[0] != "/LICENSE" {
		t.Fatalf("got license rules %v, want [/LICENSE]", got)
	}
}

func TestLoadBuildConfigRejectsUnknownYAMLField(t *testing.T) {
	root := t.TempDir()

	writeFile(t, filepath.Join(root, "ocidoc.yaml"), `
schemaVersion: v1beta
unexpected: true
components:
  documentation:
    - /README.md
`)

	if _, err := LoadBuildConfig(root, ""); err == nil {
		t.Fatal("LoadBuildConfig: expected an error for an unknown top-level field, got nil")
	}
}

func TestLoadBuildConfigRejectsUnknownJSONField(t *testing.T) {
	root := t.TempDir()

	writeFile(t, filepath.Join(root, "ocidoc.json"), `{
		"schemaVersion": "v1beta",
		"unexpected": true,
		"components": {"documentation": ["/README.md"]}
	}`)

	if _, err := LoadBuildConfig(root, ""); err == nil {
		t.Fatal("LoadBuildConfig: expected an error for an unknown top-level field, got nil")
	}
}

func TestLoadBuildConfigRejectsMultipleConventionalFiles(t *testing.T) {
	root := t.TempDir()

	writeFile(t, filepath.Join(root, "ocidoc.yaml"), "schemaVersion: v1beta\ncomponents:\n  license:\n    - /LICENSE\n")
	writeFile(t, filepath.Join(root, "ocidoc.json"), `{"schemaVersion":"v1beta","components":{"license":["/LICENSE"]}}`)

	if _, err := LoadBuildConfig(root, ""); err == nil {
		t.Fatal("expected error when multiple conventional config files exist")
	}
}

func TestLoadBuildConfigExplicitPath(t *testing.T) {
	root := t.TempDir()
	custom := filepath.Join(root, "custom.yml")

	writeFile(t, custom, "schemaVersion: v1beta\ncomponents:\n  support:\n    - /SUPPORT\n")

	cfg, err := LoadBuildConfig(root, custom)
	if err != nil {
		t.Fatalf("LoadBuildConfig: %v", err)
	}

	if _, ok := cfg.Components[spec.ComponentSupport]; !ok {
		t.Fatal("expected support component from explicit config")
	}
}

func TestLoadBuildConfigExplicitPathMissing(t *testing.T) {
	root := t.TempDir()

	if _, err := LoadBuildConfig(root, filepath.Join(root, "missing.yaml")); err == nil {
		t.Fatal("expected error for missing explicit config path")
	}
}

func TestLoadBuildConfigRejectsInvalidContent(t *testing.T) {
	root := t.TempDir()

	writeFile(t, filepath.Join(root, "ocidoc.yaml"), "schemaVersion: v1beta\ncomponents: {}\n")

	if _, err := LoadBuildConfig(root, ""); err == nil {
		t.Fatal("expected validation error for empty components map")
	}
}

func TestLoadBuildConfigRejectsMalformedYAML(t *testing.T) {
	root := t.TempDir()

	writeFile(t, filepath.Join(root, "ocidoc.yaml"), "schemaVersion: [unterminated\n")

	if _, err := LoadBuildConfig(root, ""); err == nil {
		t.Fatal("expected parse error for malformed YAML")
	}
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()

	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("WriteFile(%s): %v", path, err)
	}
}
