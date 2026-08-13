// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: Copyright 2026 WoozyMasta
// Source: github.com/ocidoc/ocidoc-go

package artifact

import (
	"reflect"
	"testing"

	ocispec "github.com/opencontainers/image-spec/specs-go/v1"

	"github.com/ocidoc/ocidoc-go/spec"
)

func TestBuildArtifactConfig(t *testing.T) {
	plan := &BuildPlan{
		Ownership: map[spec.ComponentType][]string{
			spec.ComponentDocumentation: {"README.md"},
			spec.ComponentLicense:       {"LICENSE"},
		},
		Entrypoints: map[spec.ComponentType]string{
			spec.ComponentDocumentation: "README.md",
		},
	}

	cfg := buildArtifactConfig(plan)

	if cfg.SchemaVersion != spec.SchemaVersion {
		t.Fatalf("got schemaVersion %q, want %q", cfg.SchemaVersion, spec.SchemaVersion)
	}

	want := map[spec.ComponentType]spec.ComponentConfig{
		spec.ComponentDocumentation: {Entrypoint: "README.md"},
		spec.ComponentLicense:       {Entrypoint: ""},
	}
	if !reflect.DeepEqual(cfg.Components, want) {
		t.Fatalf("got %+v, want %+v", cfg.Components, want)
	}

	if err := spec.ValidateArtifactConfig(cfg); err != nil {
		t.Fatalf("ValidateArtifactConfig: %v", err)
	}
}

func TestBuildManifestStandalone(t *testing.T) {
	plan := &BuildPlan{
		Document:    spec.DocumentSettings{ID: "default"},
		Annotations: map[string]string{"org.opencontainers.image.title": "Docs"},
		Ownership: map[spec.ComponentType][]string{
			spec.ComponentDocumentation: {"README.md"},
			spec.ComponentChangelog:     {"CHANGELOG.md"},
		},
	}

	componentDescriptors := map[spec.ComponentType]ocispec.Descriptor{
		spec.ComponentDocumentation: {MediaType: spec.ComponentLayerGzip, Digest: "sha256:aaaa", Size: 10},
		spec.ComponentChangelog:     {MediaType: spec.ComponentLayerGzip, Digest: "sha256:bbbb", Size: 20},
	}

	configDescriptor := ocispec.Descriptor{MediaType: spec.ConfigMediaType, Digest: "sha256:cccc", Size: 5}

	manifest := buildManifest(plan, configDescriptor, componentDescriptors, nil)

	if manifest.SchemaVersion != 2 {
		t.Fatalf("got schemaVersion %d, want 2", manifest.SchemaVersion)
	}

	if manifest.MediaType != ocispec.MediaTypeImageManifest {
		t.Fatalf("got mediaType %q, want %q", manifest.MediaType, ocispec.MediaTypeImageManifest)
	}

	if manifest.ArtifactType != spec.ArtifactType {
		t.Fatalf("got artifactType %q, want %q", manifest.ArtifactType, spec.ArtifactType)
	}

	if !reflect.DeepEqual(manifest.Config, configDescriptor) {
		t.Fatalf("got config %+v, want %+v", manifest.Config, configDescriptor)
	}

	if manifest.Subject != nil {
		t.Fatal("expected nil subject for a standalone artifact")
	}

	// Layers must be sorted by component type name: "changelog" before "documentation".
	if len(manifest.Layers) != 2 {
		t.Fatalf("got %d layers, want 2", len(manifest.Layers))
	}

	if manifest.Layers[0].Digest != "sha256:bbbb" || manifest.Layers[1].Digest != "sha256:aaaa" {
		t.Fatalf("layers not sorted by component name: %+v", manifest.Layers)
	}

	if got := manifest.Annotations[spec.AnnotationSchema]; got != spec.SchemaVersion {
		t.Fatalf("got schema annotation %q, want %q", got, spec.SchemaVersion)
	}

	if got := manifest.Annotations[spec.AnnotationComponents]; got != "changelog,documentation" {
		t.Fatalf("got components annotation %q, want %q", got, "changelog,documentation")
	}

	if got := manifest.Annotations[spec.AnnotationDocumentID]; got != "default" {
		t.Fatalf("got document id annotation %q, want default", got)
	}

	if got := manifest.Annotations["org.opencontainers.image.title"]; got != "Docs" {
		t.Fatalf("got user annotation %q, want Docs", got)
	}

	if _, has := manifest.Annotations[spec.AnnotationDocumentLanguage]; has {
		t.Fatal("did not expect a language annotation when Document.Language is empty")
	}
}

func TestBuildManifestAttached(t *testing.T) {
	plan := &BuildPlan{
		Document:  spec.DocumentSettings{ID: "default", Language: "en"},
		Ownership: map[spec.ComponentType][]string{spec.ComponentDocumentation: {"README.md"}},
	}

	subject := &ocispec.Descriptor{MediaType: ocispec.MediaTypeImageIndex, Digest: "sha256:dddd", Size: 100}
	componentDescriptors := map[spec.ComponentType]ocispec.Descriptor{
		spec.ComponentDocumentation: {Digest: "sha256:aaaa"},
	}

	manifest := buildManifest(plan, ocispec.Descriptor{}, componentDescriptors, subject)

	if manifest.Subject == nil || manifest.Subject.Digest != subject.Digest {
		t.Fatalf("got subject %+v, want %+v", manifest.Subject, subject)
	}

	if got := manifest.Annotations[spec.AnnotationDocumentLanguage]; got != "en" {
		t.Fatalf("got language annotation %q, want en", got)
	}
}

func TestBuildManifestDoesNotMutatePlanAnnotations(t *testing.T) {
	plan := &BuildPlan{
		Document:    spec.DocumentSettings{ID: "default"},
		Annotations: map[string]string{"org.opencontainers.image.title": "Docs"},
		Ownership:   map[spec.ComponentType][]string{spec.ComponentDocumentation: {"README.md"}},
	}

	componentDescriptors := map[spec.ComponentType]ocispec.Descriptor{
		spec.ComponentDocumentation: {Digest: "sha256:aaaa"},
	}

	buildManifest(plan, ocispec.Descriptor{}, componentDescriptors, nil)

	if _, has := plan.Annotations[spec.AnnotationSchema]; has {
		t.Fatal("buildManifest must not mutate plan.Annotations")
	}

	if len(plan.Annotations) != 1 {
		t.Fatalf("plan.Annotations grew unexpectedly: %v", plan.Annotations)
	}
}
