// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: Copyright 2026 WoozyMasta
// Source: github.com/ocidoc/ocidoc-go

package spec

// Root manifest managed annotation keys.
// These are reserved for the implementation and specification;
// user-provided annotations cannot override them.
const (
	// AnnotationSchema records the OCIDoc format revision, e.g. "v1beta".
	AnnotationSchema = "org.ocidoc.schema"

	// AnnotationComponents is a sorted,
	// comma-separated summary of the component types present in the manifest.
	AnnotationComponents = "org.ocidoc.components"

	// AnnotationDocumentID identifies the logical documentation slot.
	AnnotationDocumentID = "org.ocidoc.document.id"

	// AnnotationDocumentLanguage identifies the document language, when set.
	AnnotationDocumentLanguage = "org.ocidoc.document.language"

	// AnnotationDocumentVariant identifies the document variant, when set.
	AnnotationDocumentVariant = "org.ocidoc.document.variant"
)

// Component descriptor managed annotation keys, set on each manifest layer.
const (
	// AnnotationComponentType is the ComponentType of the layer.
	AnnotationComponentType = "org.ocidoc.component.type"

	// AnnotationComponentEntrypoint is the component's default file, if any.
	AnnotationComponentEntrypoint = "org.ocidoc.component.entrypoint"

	// AnnotationComponentFileCount is the number of files in the component tar.
	AnnotationComponentFileCount = "org.ocidoc.component.file-count"

	// AnnotationComponentUncompressedSize is the component tar's uncompressed size in bytes.
	AnnotationComponentUncompressedSize = "org.ocidoc.component.uncompressed-size"
)

// DefaultDocumentID is the document.id value
// used when the build config does not set one explicitly.
const DefaultDocumentID = "default"
