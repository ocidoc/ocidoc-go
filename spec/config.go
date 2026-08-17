// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: Copyright 2026 WoozyMasta
// Source: github.com/ocidoc/ocidoc-go

package spec

// BuildConfig is the writer input read from ocidoc.yaml/ocidoc.json or an explicitly selected configuration file.
// It is distinct from ArtifactConfig: BuildConfig describes how to build an artifact from a source tree;
// ArtifactConfig is the resulting blob shipped inside the artifact.
type BuildConfig struct {
	// SchemaVersion is the build config format version.
	// The only currently supported value is "v1beta".
	SchemaVersion string `json:"schemaVersion" yaml:"schemaVersion" jsonschema:"required,enum=v1beta,default=v1beta"`

	// Settings controls build-time behavior that does not change the resulting artifact's logical identity:
	// strict mode and component compression.
	// Omit it to use the documented defaults (strict mode off, gzip at level 6).
	Settings BuildSettings `json:"settings,omitzero" yaml:"settings,omitempty"`

	// Document sets the built artifact's document identity (id and variant).
	// Omit it to build the default document.
	Document DocumentSettings `json:"document,omitzero" yaml:"document,omitempty"`

	// Entrypoints overrides the automatically detected primary file for one or more components, keyed by component name.
	// A component not listed here falls back to automatic detection
	// (for example, README.md for the documentation component).
	Entrypoints map[ComponentType]string `json:"entrypoints,omitempty" yaml:"entrypoints,omitempty"`

	// Annotations adds custom key/value pairs to the built artifact's root OCI manifest.
	// Keys starting with "org.ocidoc." are reserved for OCIDoc itself and rejected here.
	Annotations map[string]string `json:"annotations,omitempty" yaml:"annotations,omitempty"`

	// Components maps each component name to the path rules that select its files from the source tree.
	// At least one component with at least one rule is required.
	// A name is either one of the standard component types
	// (documentation, license, changelog, release-notes, security, contributing, code-of-conduct, support)
	// or a custom "x-"-prefixed extension name.
	Components map[ComponentType][]string `json:"components" yaml:"components" jsonschema:"required"`

	// Ignore lists path rules excluded from every component, applied after component matching.
	// A rule prefixed with "!" is a negation: it restores a path an earlier ignore rule
	// (here or in a component's own rules) would otherwise have excluded.
	Ignore []string `json:"ignore,omitempty" yaml:"ignore,omitempty"`
}

// BuildSettings holds writer-behavior options that are not part of the resulting artifact's identity.
type BuildSettings struct {
	// Compression selects how OCIDoc component layers are compressed.
	// Omit it to use the default: gzip at level 6.
	Compression *CompressionSettings `json:"compression,omitempty" yaml:"compression,omitempty"`

	// Strict rejects a build where a declared component matches no files in the source tree,
	// instead of only warning about it.
	Strict *bool `json:"strict,omitempty" yaml:"strict,omitempty" jsonschema:"default=false"`
}

// CompressionSettings selects the component layer compression algorithm
// and, for gzip, its level.
type CompressionSettings struct {
	// Level controls the compression level used by the selected compressor.
	// Higher levels may reduce artifact size at the cost of additional CPU time during build.
	Level *int `json:"level,omitempty" yaml:"level,omitempty" jsonschema:"minimum=0,maximum=9,default=6"`

	// Type selects the compression algorithm used for component layers.
	// Gzip is the default and has the broadest registry client support;
	// zstd typically compresses faster and smaller at a comparable level,
	// at the cost of less universal tooling support.
	Type CompressionType `json:"type" yaml:"type" jsonschema:"enum=gzip,enum=zstd,default=gzip"`
}

// DocumentSettings sets the document identity tuple
// (see AnnotationDocumentID and related constants) for the artifact being built.
type DocumentSettings struct {
	// ID identifies this artifact among multiple documentation artifacts
	// that may be attached to the same subject.
	ID string `json:"id,omitempty" yaml:"id,omitempty" jsonschema:"default=default"`

	// Variant distinguishes multiple documents that share the same ID,
	// for example "operator" versus "user".
	// Omit it when only one variant of this document exists.
	Variant string `json:"variant,omitempty" yaml:"variant,omitempty"`
}

// ArtifactConfig is the OCIDoc artifact config blob (ConfigMediaType).
// It contains logical structure only:
// it must not duplicate component digests, sizes or media types already present in the manifest layers,
// and it must not carry release-specific metadata (that belongs in root manifest annotations).
type ArtifactConfig struct {
	// Components maps each component name present in the artifact to its per-component config.
	// At least one component is required, and it must match the manifest's own set of component layers exactly.
	Components map[ComponentType]ComponentConfig `json:"components" jsonschema:"required"`

	// Schema, when set, must equal this format's canonical JSON Schema identifier.
	// It is optional: a reader must not reject a config that omits it.
	Schema string `json:"$schema,omitempty" jsonschema:"example=https://ocidoc.org/schema/artifact-config-v1beta.json"`

	// SchemaVersion is the artifact config format version.
	// The only currently supported value is "v1beta".
	SchemaVersion string `json:"schemaVersion" jsonschema:"required,enum=v1beta,default=v1beta"`
}

// ComponentConfig is the per-component entry in ArtifactConfig.
type ComponentConfig struct {
	// Entrypoint is this component's primary file, as a bundle-relative path (no leading "/", no ".." segments).
	// Omit it when the component has no designated primary file.
	Entrypoint string `json:"entrypoint,omitempty"`
}
