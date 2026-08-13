// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: Copyright 2026 WoozyMasta
// Source: github.com/ocidoc/ocidoc-go

package artifact

import (
	"maps"
	"sort"
	"strings"

	specs "github.com/opencontainers/image-spec/specs-go"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"

	"github.com/ocidoc/ocidoc-go/spec"
)

// buildArtifactConfig assembles plan's ArtifactConfig:
// logical structure only, no component digests.
// One entry per component present in plan.Ownership - components with no planned files
// are absent from both the manifest layers and the artifact config,
// whether that was tolerated as a warning or never declared at all.
func buildArtifactConfig(plan *BuildPlan) *spec.ArtifactConfig {
	components := make(map[spec.ComponentType]spec.ComponentConfig, len(plan.Ownership))

	for name := range plan.Ownership {
		components[name] = spec.ComponentConfig{Entrypoint: plan.Entrypoints[name]}
	}

	return &spec.ArtifactConfig{
		SchemaVersion: spec.SchemaVersion,
		Components:    components,
	}
}

// buildManifest assembles the root OCI manifest:
// standard OCI image manifest media type, OCIDoc artifactType,
// the config and component layer descriptors,
// and the managed root annotations layered over plan's
// (already-validated, non-reserved) user annotations.
// subject is nil for a standalone artifact.
//
// Layers are sorted by component type name for deterministic manifest JSON -
// component ordering carries no semantic meaning,
// but the bytes must still be reproducible.
func buildManifest(
	plan *BuildPlan,
	configDescriptor ocispec.Descriptor,
	componentDescriptors map[spec.ComponentType]ocispec.Descriptor,
	subject *ocispec.Descriptor,
) ocispec.Manifest {
	names := make([]string, 0, len(componentDescriptors))
	for name := range componentDescriptors {
		names = append(names, string(name))
	}

	sort.Strings(names)

	layers := make([]ocispec.Descriptor, 0, len(names))
	for _, name := range names {
		layers = append(layers, componentDescriptors[spec.ComponentType(name)])
	}

	annotations := make(map[string]string, len(plan.Annotations)+5)
	maps.Copy(annotations, plan.Annotations)

	annotations[spec.AnnotationSchema] = spec.SchemaVersion
	annotations[spec.AnnotationComponents] = strings.Join(names, ",")
	annotations[spec.AnnotationDocumentID] = plan.Document.ID

	if plan.Document.Language != "" {
		annotations[spec.AnnotationDocumentLanguage] = plan.Document.Language
	}

	if plan.Document.Variant != "" {
		annotations[spec.AnnotationDocumentVariant] = plan.Document.Variant
	}

	return ocispec.Manifest{
		Versioned:    specs.Versioned{SchemaVersion: 2},
		MediaType:    ocispec.MediaTypeImageManifest,
		ArtifactType: spec.ArtifactType,
		Config:       configDescriptor,
		Layers:       layers,
		Subject:      subject,
		Annotations:  annotations,
	}
}
