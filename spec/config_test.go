// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: Copyright 2026 WoozyMasta
// Source: github.com/ocidoc/ocidoc-go

package spec

import (
	"encoding/json"
	"reflect"
	"testing"
)

// TestArtifactConfigJSONShape locks the artifact config's wire shape
// to the Go struct tags.
func TestArtifactConfigJSONShape(t *testing.T) {
	const input = `{
		"$schema": "https://ocidoc.org/schema/artifact-config-v1beta.json",
		"schemaVersion": "v1beta",
		"components": {
			"documentation": {"entrypoint": "README.md"},
			"license": {"entrypoint": "LICENSE"},
			"security": {"entrypoint": "SECURITY.md"},
			"changelog": {"entrypoint": "CHANGELOG.md"}
		}
	}`

	var cfg ArtifactConfig
	if err := json.Unmarshal([]byte(input), &cfg); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}

	if cfg.SchemaVersion != SchemaVersion {
		t.Fatalf("got schemaVersion %q, want %q", cfg.SchemaVersion, SchemaVersion)
	}
	if cfg.Schema != ArtifactConfigSchemaID {
		t.Fatalf("got $schema %q, want %q", cfg.Schema, ArtifactConfigSchemaID)
	}

	if got, want := cfg.Components[ComponentDocumentation].Entrypoint, "README.md"; got != want {
		t.Fatalf("got documentation entrypoint %q, want %q", got, want)
	}

	if err := ValidateArtifactConfig(&cfg); err != nil {
		t.Fatalf("ValidateArtifactConfig: %v", err)
	}

	out, err := json.Marshal(cfg)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}

	var roundTripped ArtifactConfig
	if err := json.Unmarshal(out, &roundTripped); err != nil {
		t.Fatalf("round-trip Unmarshal: %v", err)
	}

	if len(roundTripped.Components) != len(cfg.Components) {
		t.Fatalf("round-trip lost components: got %d, want %d", len(roundTripped.Components), len(cfg.Components))
	}
}

// TestBuildConfigYAMLFieldNames locks the build config's yaml tag casing to camelCase
// (schemaVersion, not schema_version).
func TestBuildConfigYAMLFieldNames(t *testing.T) {
	// Struct-typed fields use json "omitzero"
	// (encoding/json's omitempty has no effect on struct values) but yaml "omitempty",
	// since the yaml encoder chosen for this package is not fixed yet.
	wantJSON := map[string]string{
		"SchemaVersion": "schemaVersion",
		"Settings":      "settings,omitzero",
		"Document":      "document,omitzero",
		"Entrypoints":   "entrypoints,omitempty",
		"Annotations":   "annotations,omitempty",
		"Components":    "components",
		"Ignore":        "ignore,omitempty",
	}
	wantYAML := map[string]string{
		"SchemaVersion": "schemaVersion",
		"Settings":      "settings,omitempty",
		"Document":      "document,omitempty",
		"Entrypoints":   "entrypoints,omitempty",
		"Annotations":   "annotations,omitempty",
		"Components":    "components",
		"Ignore":        "ignore,omitempty",
	}

	typ := reflect.TypeOf(BuildConfig{})
	for i := range typ.NumField() {
		field := typ.Field(i)

		wantJSONTag, ok := wantJSON[field.Name]
		if !ok {
			t.Errorf("unexpected BuildConfig field %q, add it to the tag test tables", field.Name)
			continue
		}

		if got := field.Tag.Get("json"); got != wantJSONTag {
			t.Errorf("BuildConfig.%s json tag = %q, want %q", field.Name, got, wantJSONTag)
		}

		if got := field.Tag.Get("yaml"); got != wantYAML[field.Name] {
			t.Errorf("BuildConfig.%s yaml tag = %q, want %q", field.Name, got, wantYAML[field.Name])
		}
	}
}
