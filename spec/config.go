// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: Copyright 2026 WoozyMasta
// Source: github.com/ocidoc/ocidoc-go

package spec

// BuildConfig is the source-tree configuration read from `ocidoc.yaml`, `ocidoc.json`,
// or an explicitly selected configuration file.
// It describes how source files become an OCIDoc artifact.
type BuildConfig struct {
	// SchemaVersion is the build config format version.
	// The only currently supported value is `v1beta`.
	SchemaVersion string `json:"schemaVersion" yaml:"schemaVersion" jsonschema_extras:"x-order=1" jsonschema:"required,enum=v1beta,default=v1beta"`

	// Annotations adds custom key/value pairs to the built artifact's root OCI manifest.
	// Keys starting with `org.ocidoc.` are reserved for OCIDoc itself and rejected here.
	Annotations map[string]string `json:"annotations,omitempty" yaml:"annotations,omitempty" jsonschema_extras:"x-order=2"`

	// Components maps component names to their source selection
	// and optional localized document sets.
	//
	// Built-in v1beta component names:
	//
	//   - documentation
	//   - license
	//   - changelog
	//   - release-notes
	//   - security
	//   - contributing
	//   - code-of-conduct
	//   - support
	//
	// Custom components are allowed when their names use the `x-` prefix,
	// for example, `x-api` or `x-runbooks`. Other names are rejected.
	// At least one component with one or more path rules is required.
	// The same source file cannot be assigned to more than one component.
	Components map[ComponentType]ComponentBuildConfig `json:"components" yaml:"components" jsonschema_extras:"x-order=3" jsonschema:"required"`

	// Settings controls build-time behavior that does not change the resulting artifact's logical identity:
	// strict mode and component compression.
	// Omit it to use the documented defaults (strict mode off, gzip at level 6).
	Settings BuildSettings `json:"settings,omitzero" yaml:"settings,omitempty" jsonschema_extras:"x-order=4"`

	// Document sets the built artifact's document identity (id and variant).
	// Omit it to build the default document.
	Document DocumentSettings `json:"document,omitzero" yaml:"document,omitempty" jsonschema_extras:"x-order=5"`

	// Ignore lists path rules excluded from every component, applied after component matching.
	// A rule prefixed with `!` is a negation:
	// it restores a path an earlier ignore rule would otherwise have excluded.
	// Ignore rules are useful for repository metadata or generated files that match a broad component rule.
	Ignore []string `json:"ignore,omitempty" yaml:"ignore,omitempty" jsonschema_extras:"x-order=6"`
}

// ComponentBuildConfig selects one component's source files and optional localized document sets.
type ComponentBuildConfig struct {
	// Locales maps locale keys to path rules selecting this component's localized documents.
	//
	// Locale keys are opaque identifiers such as `en` or `ru` and are local to this component;
	// the same key may select different files in different components.
	//
	// Locale rules never add files to the component and cannot select files owned elsewhere.
	// The default locale is optional; when omitted, renderers use the first locale in sorted key order.
	Locales map[string]BuildLocaleConfig `json:"locales,omitempty" yaml:"locales,omitempty" jsonschema_extras:"x-order=3"`

	// Entrypoint overrides automatic detection of this component's primary file.
	// The path is root-relative and may begin with `/` in build configuration.
	// It must resolve to a file selected by Paths.
	// Omit it to use deterministic component-specific detection, such as `README.md` for documentation.
	Entrypoint string `json:"entrypoint,omitempty" yaml:"entrypoint,omitempty" jsonschema_extras:"x-order=2"`

	// Paths selects this component's files from the source tree.
	//
	// Rules are repository-root-relative and normally begin with `/`.
	// Glob patterns are supported, and a rule beginning with `!` excludes a previously matched path.
	//
	// These rules select the component's complete file set, including assets.
	// Locale rules below classify only its document files.
	Paths []string `json:"paths" yaml:"paths" jsonschema_extras:"x-order=1" jsonschema:"required,minItems=1"`
}

// BuildLocaleConfig selects one component's document files for one locale.
type BuildLocaleConfig struct {
	// Entrypoint overrides automatic selection of this locale's primary document.
	// The path is root-relative and may begin with `/` in build configuration.
	// It must be one of the documents selected by this locale's Paths.
	Entrypoint string `json:"entrypoint,omitempty" yaml:"entrypoint,omitempty" jsonschema_extras:"x-order=2"`

	// Paths selects this component's document files for the locale.
	//
	// Rules are evaluated only against documents already owned by the parent component.
	// Assets and source files are not locale members.
	//
	// An empty match is a warning by default and an error in strict mode.
	Paths []string `json:"paths" yaml:"paths" jsonschema_extras:"x-order=3" jsonschema:"required,minItems=1"`

	// Default marks the preferred fallback locale for the component.
	// At most one locale per component may set it.
	// It affects renderer fallback and the fallback information stored in artifact metadata, not component ownership.
	Default bool `json:"default,omitempty" yaml:"default,omitempty" jsonschema_extras:"x-order=1"`
}

// BuildSettings holds optional build behavior that is not part of the resulting artifact's identity.
type BuildSettings struct {
	// Compression selects how OCIDoc component layers are compressed.
	// Omit it to use the default: gzip at level 6.
	Compression *CompressionSettings `json:"compression,omitempty" yaml:"compression,omitempty" jsonschema_extras:"x-order=1"`

	// Strict rejects a build where a declared component or locale matches no files, instead of only warning about it.
	Strict *bool `json:"strict,omitempty" yaml:"strict,omitempty" jsonschema_extras:"x-order=2" jsonschema:"default=false"`
}

// CompressionSettings selects the component layer compression algorithm
// and its level on the shared OCIDoc compression scale.
type CompressionSettings struct {
	// Level controls the compression level used by the selected compressor.
	// Higher levels may reduce artifact size at the cost of additional CPU time during build.
	// Values above the selected compressor's maximum are accepted and clamped
	// when the final compression settings are applied.
	Level *int `json:"level,omitempty" yaml:"level,omitempty" jsonschema_extras:"x-order=1" jsonschema:"minimum=0,default=6"`

	// Type selects the compression algorithm used for component layers.
	//
	//   - `gzip` is the default and has the broadest registry client support;
	//   - `zstd` typically compresses faster and smaller at a comparable level, at the cost of less universal tooling support.
	Type CompressionType `json:"type,omitempty" yaml:"type,omitempty" jsonschema_extras:"x-order=2" jsonschema:"enum=gzip,enum=zstd,default=gzip"`
}

// DocumentSettings sets the identity shown for the document artifact.
type DocumentSettings struct {
	// ID identifies this artifact among multiple documentation artifacts that may be attached to the same subject.
	ID string `json:"id,omitempty" yaml:"id,omitempty" jsonschema_extras:"x-order=1" jsonschema:"default=default"`

	// Variant distinguishes multiple documents that share the same ID, for example `operator` versus `user`.
	// Omit it when only one variant of this document exists.
	Variant string `json:"variant,omitempty" yaml:"variant,omitempty" jsonschema_extras:"x-order=2"`
}

// ArtifactConfig is the configuration metadata stored inside an OCIDoc artifact.
// It contains logical structure only:
// it must not duplicate component digests, sizes or media types already present in the manifest layers,
// and it must not carry release-specific metadata (that belongs in root manifest annotations).
type ArtifactConfig struct {
	// Components maps each component name present in the artifact to its per-component config.
	//
	// The names use the same v1beta set as BuildConfig:
	//
	//   - `documentation`
	//   - `license`
	//   - `changelog`
	//   - `release-notes`
	//   - `security`
	//   - `contributing`
	//   - `code-of-conduct`
	//   - `support`
	//
	// Custom names are allowed only with the `x-` prefix.
	// At least one component is required, and the set must match the manifest's component layers exactly.
	Components map[ComponentType]ComponentConfig `json:"components" yaml:"components" jsonschema_extras:"x-order=3" jsonschema:"required"`

	// Schema, when set, must equal this format's canonical JSON Schema identifier.
	// It is optional: a reader must not reject a config that omits it.
	Schema string `json:"$schema,omitempty" yaml:"$schema,omitempty" jsonschema_extras:"x-order=2" jsonschema:"example=https://ocidoc.org/schema/artifact-config-v1beta.json"`

	// SchemaVersion is the artifact config format version.
	// The only currently supported value is "v1beta".
	SchemaVersion string `json:"schemaVersion" yaml:"schemaVersion" jsonschema_extras:"x-order=1" jsonschema:"required,enum=v1beta,default=v1beta"`
}

// ComponentConfig is the per-component entry in ArtifactConfig.
type ComponentConfig struct {
	// Locales maps locale keys to resolved files and entrypoints for this component.
	// It is produced from BuildConfig locale rules and records the exact files included in each locale.
	// A component may have no locales, and locale keys remain component-local.
	Locales map[string]ArtifactLocaleConfig `json:"locales,omitempty" yaml:"locales,omitempty" jsonschema_extras:"x-order=2"`

	// Entrypoint is this component's primary file, as a bundle-relative path (no leading `/`, no `..` segments).
	// Omit it when the component has no designated primary file.
	// It is a bundle path, not a source-tree matching rule.
	Entrypoint string `json:"entrypoint,omitempty" yaml:"entrypoint,omitempty" jsonschema_extras:"x-order=1"`
}

// ArtifactLocaleConfig describes which bundle documents belong to one locale.
type ArtifactLocaleConfig struct {
	// Entrypoint is this locale's primary document, as a bundle-relative path.
	Entrypoint string `json:"entrypoint,omitempty" yaml:"entrypoint,omitempty" jsonschema_extras:"x-order=2"`

	// Files lists resolved bundle-relative document paths in this locale.
	// This is an exact bundle membership list, not a glob or source rule.
	Files []string `json:"files" yaml:"files" jsonschema_extras:"x-order=3" jsonschema:"required"`

	// Default marks the preferred fallback locale for the component.
	// Exactly one locale is normally marked default when the component has locales;
	// consumers should use the first key in deterministic order only when metadata was produced without an explicit default.
	Default bool `json:"default,omitempty" yaml:"default,omitempty" jsonschema_extras:"x-order=1"`
}
