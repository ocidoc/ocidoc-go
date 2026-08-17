// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: Copyright 2026 WoozyMasta
// Source: github.com/ocidoc/ocidoc-go

package spec

// ValidateBuildConfig reports whether cfg is a well-formed v1beta build config:
// a matching schemaVersion, at least one component with a non-empty rule list
// syntactically valid component names, entrypoints that reference a declared component,
// non-reserved annotations and a supported compression type.
//
// It validates config shape only.
// Path rule syntax, actual component ownership after matching,
// and entrypoint-belongs-to-component checks after planning
// are the build planner's job, not this function's: they require the source tree.
func ValidateBuildConfig(cfg *BuildConfig) error {
	if cfg == nil {
		return &ValidationError{Code: CodeMissingSchemaVersion, Message: "build config must not be nil"}
	}

	if cfg.SchemaVersion == "" {
		return &ValidationError{Code: CodeMissingSchemaVersion, Message: "schemaVersion is required"}
	}

	if cfg.SchemaVersion != SchemaVersion {
		return &ValidationError{
			Code:    CodeUnsupportedSchemaVersion,
			Message: "unsupported schemaVersion: " + cfg.SchemaVersion,
		}
	}

	if len(cfg.Components) == 0 {
		return &ValidationError{Code: CodeNoComponents, Message: "build config must declare at least one component"}
	}

	for name, rules := range cfg.Components {
		if err := ValidateComponentType(string(name)); err != nil {
			return err
		}

		if len(rules) == 0 {
			return &ValidationError{
				Code: CodeEmptyComponentRules, Component: string(name),
				Message: "component must declare at least one path rule",
			}
		}
	}

	for name := range cfg.Entrypoints {
		if _, declared := cfg.Components[name]; !declared {
			return &ValidationError{
				Code: CodeUndeclaredComponent, Component: string(name),
				Message: "entrypoint references a component not present in components",
			}
		}
	}

	if err := ValidateUserAnnotations(cfg.Annotations); err != nil {
		return err
	}

	if cfg.Settings.Compression != nil {
		if cfg.Settings.Compression.Type != "" {
			if _, err := cfg.Settings.Compression.Type.MediaType(); err != nil {
				return err
			}
		}
	}

	return nil
}
