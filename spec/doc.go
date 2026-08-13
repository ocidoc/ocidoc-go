// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: Copyright 2026 WoozyMasta
// Source: github.com/ocidoc/ocidoc-go

// Package spec defines the OCIDoc v1beta artifact format:
// media types, managed annotations, standard component types,
// build/artifact config structures and format-level validation.
package spec

const (
	// SchemaVersion is the build and artifact config schemaVersion value for this package's format revision.
	SchemaVersion = "v1beta"

	// BuildConfigSchemaID is the canonical JSON Schema identifier for build configuration input.
	BuildConfigSchemaID = "https://ocidoc.org/schema/build-config-v1beta.json"

	// ArtifactConfigSchemaID is the canonical JSON Schema identifier for an artifact config blob.
	ArtifactConfigSchemaID = "https://ocidoc.org/schema/artifact-config-v1beta.json"
)
