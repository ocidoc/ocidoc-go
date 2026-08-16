// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: Copyright 2026 WoozyMasta
// Source: github.com/ocidoc/ocidoc-go

package artifact

import (
	"bytes"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"go.yaml.in/yaml/v3"

	"github.com/ocidoc/ocidoc-go/spec"
)

// defaultConfigYAML is the embedded default build config:
// permissive rules for the standard components only, no custom "x-" components.
//
//go:embed default-config.yaml
var defaultConfigYAML []byte

// conventionalConfigNames is the search order used
// when no explicit explicit configuration path is given.
var conventionalConfigNames = []string{"ocidoc.yaml", "ocidoc.yml", "ocidoc.json"}

// LoadBuildConfig loads and validates the build config
// for the source tree rooted at root, following this search order:
//
//  1. explicitPath, if non-empty;
//  2. exactly one of ocidoc.yaml, ocidoc.yml or ocidoc.json under root;
//  3. the embedded default config.
//
// It is an error for more than one conventional file to exist under root,
// and an error for explicitPath to not exist.
// The returned config is validated with spec.ValidateBuildConfig before being returned;
// it carries no defaulting beyond what the source itself sets
// (see spec.BuildConfig for the omitted-vs-explicit distinction).
func LoadBuildConfig(root, explicitPath string) (*spec.BuildConfig, error) {
	cfg, _, err := loadBuildConfig(root, explicitPath)
	return cfg, err
}

// loadBuildConfig is LoadBuildConfig with the resolved source name
// returned to planning code that needs to distinguish the embedded optional defaults
// from an authored project configuration.
func loadBuildConfig(root, explicitPath string) (*spec.BuildConfig, string, error) {
	data, name, err := readBuildConfigSource(root, explicitPath)
	if err != nil {
		return nil, "", err
	}

	cfg, err := parseBuildConfig(data, name)
	if err != nil {
		return nil, "", fmt.Errorf("parse build config %s: %w", name, err)
	}

	if err := spec.ValidateBuildConfig(cfg); err != nil {
		return nil, "", fmt.Errorf("validate build config %s: %w", name, err)
	}

	return cfg, name, nil
}

// DefaultBuildConfig parses and returns the embedded default build config.
// It returns a structured value so callers can marshal it as
// YAML or JSON instead of handling the embedded bytes directly.
func DefaultBuildConfig() (*spec.BuildConfig, error) {
	return parseBuildConfig(defaultConfigYAML, "embedded default")
}

// readBuildConfigSource resolves which build config to load
// and returns its raw bytes together with a name used for error messages and format detection.
func readBuildConfigSource(root, explicitPath string) ([]byte, string, error) {
	if explicitPath != "" {
		//nolint:gosec // caller explicitly selected this configuration path.
		data, err := os.ReadFile(explicitPath)
		if err != nil {
			return nil, "", fmt.Errorf("read build config %s: %w", explicitPath, err)
		}

		return data, explicitPath, nil
	}

	var found []string

	for _, candidate := range conventionalConfigNames {
		p := filepath.Join(root, candidate)

		_, statErr := os.Stat(p)

		switch {
		case statErr == nil:
			found = append(found, p)
		case errors.Is(statErr, os.ErrNotExist):
			continue
		default:
			return nil, "", fmt.Errorf("stat build config %s: %w", p, statErr)
		}
	}

	switch len(found) {
	case 0:
		return defaultConfigYAML, "embedded default", nil

	case 1:
		data, err := os.ReadFile(found[0]) //nolint:gosec // path built from a fixed, conventional filename under root.
		if err != nil {
			return nil, "", fmt.Errorf("read build config %s: %w", found[0], err)
		}

		return data, found[0], nil

	default:
		return nil, "", fmt.Errorf("multiple build config files found: %s", strings.Join(found, ", "))
	}
}

// parseBuildConfig decodes data as JSON when name has a ".json" extension,
// and as YAML otherwise (covers ".yaml", ".yml" and the embedded default).
// Both decoders reject unknown top-level and nested fields rather than silently ignoring them,
// so a typo in ocidoc.yaml (e.g. "strics" instead of "strict") is a load error,
// not a config that quietly loses the setting it looks like it sets.
func parseBuildConfig(data []byte, name string) (*spec.BuildConfig, error) {
	var cfg spec.BuildConfig

	if strings.HasSuffix(name, ".json") {
		decoder := json.NewDecoder(bytes.NewReader(data))
		decoder.DisallowUnknownFields()

		if err := decoder.Decode(&cfg); err != nil {
			return nil, err
		}

		return &cfg, nil
	}

	decoder := yaml.NewDecoder(bytes.NewReader(data))
	decoder.KnownFields(true)

	if err := decoder.Decode(&cfg); err != nil {
		return nil, err
	}

	return &cfg, nil
}
