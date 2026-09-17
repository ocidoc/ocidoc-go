// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: Copyright 2026 WoozyMasta
// Source: github.com/ocidoc/ocidoc-go

package artifact

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/opencontainers/go-digest"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"

	"github.com/ocidoc/ocidoc-go/spec"
)

// buildDiffLayout builds a layout from files under a fresh root,
// with a fixed ModTime so otherwise-identical inputs produce byte-identical output
// (see the determinism work logged for BuildLayout).
func buildDiffLayout(t *testing.T, files map[string]string, opts BuildLayoutOptions) Reader {
	t.Helper()

	root := t.TempDir()
	writeConfigTree(t, root, files)

	layoutDir := t.TempDir()

	if opts.ModTime.IsZero() {
		opts.ModTime = time.Unix(1700000000, 0).UTC()
	}

	if _, _, err := BuildLayout(t.Context(), root, layoutDir, opts); err != nil {
		t.Fatalf("BuildLayout: %v", err)
	}

	reader, err := OpenLayout(layoutDir)
	if err != nil {
		t.Fatalf("OpenLayout: %v", err)
	}

	return reader
}

func diffFixtureFiles() map[string]string {
	return map[string]string{
		"ocidoc.yaml": `
schemaVersion: v1beta
components:
  documentation:
    paths:
      - /README.md
      - /docs/en.md
  license:
    paths:
      - /LICENSE
`,
		"README.md":  "# hi",
		"docs/en.md": "# en",
		"LICENSE":    "MIT",
	}
}

func TestDiffIdenticalArtifactsAreEqual(t *testing.T) {
	files := diffFixtureFiles()
	a := buildDiffLayout(t, files, BuildLayoutOptions{})
	b := buildDiffLayout(t, files, BuildLayoutOptions{})

	result, err := Diff(context.Background(), a, b, DiffOptions{})
	if err != nil {
		t.Fatalf("Diff: %v", err)
	}

	if !result.Equal {
		t.Fatalf("expected identical artifacts to be Equal, got %+v", result)
	}

	if len(result.Annotations) != 0 || len(result.Components) != 0 {
		t.Fatalf("expected no diffs, got %+v", result)
	}
}

func TestDiffDetectsAnnotationChange(t *testing.T) {
	files := diffFixtureFiles()
	a := buildDiffLayout(t, files, BuildLayoutOptions{})
	b := buildDiffLayout(t, files, BuildLayoutOptions{
		Plan: PlanOptions{Annotations: map[string]string{"org.example.custom": "changed"}},
	})

	result, err := Diff(context.Background(), a, b, DiffOptions{})
	if err != nil {
		t.Fatalf("Diff: %v", err)
	}

	if result.Equal {
		t.Fatal("expected an annotation change to make the artifacts unequal")
	}

	found := false

	for _, d := range result.Annotations {
		if d.Key == "org.example.custom" && d.Before == "" && d.After == "changed" {
			found = true
		}
	}

	if !found {
		t.Fatalf("expected an added org.example.custom annotation diff, got %+v", result.Annotations)
	}
}

func TestDiffDetectsLocaleMembershipChangeWithoutComponentDiff(t *testing.T) {
	result := diffLocales(
		map[spec.ComponentType]spec.ComponentConfig{
			spec.ComponentDocumentation: {Locales: map[string]spec.ArtifactLocaleConfig{
				"en": {Files: []string{"README.md"}},
			}},
		},
		map[spec.ComponentType]spec.ComponentConfig{
			spec.ComponentDocumentation: {Locales: map[string]spec.ArtifactLocaleConfig{
				"en": {Files: []string{"README.md"}},
				"ru": {Files: []string{"README.md"}},
			}},
		},
	)
	if len(result) != 1 || result[0].Locale != "ru" || !result[0].AfterSet {
		t.Fatalf("unexpected locale diff: %+v", result)
	}
}

func TestDiffDetectsResolvedLocaleChangeWithoutComponentDigestChange(t *testing.T) {
	filesA := map[string]string{
		"ocidoc.yaml": `
schemaVersion: v1beta
components:
  documentation:
    paths:
      - /README.md
      - /docs/en.md
    locales:
      en:
        paths:
          - /README.md
  license:
    paths:
      - /LICENSE
`,
		"README.md":  "# hi",
		"docs/en.md": "# en",
		"LICENSE":    "MIT",
	}
	filesB := map[string]string{
		"ocidoc.yaml": `
schemaVersion: v1beta
components:
  documentation:
    paths:
      - /README.md
      - /docs/en.md
    locales:
      en:
        paths:
          - /docs/en.md
  license:
    paths:
      - /LICENSE
`,
		"README.md":  "# hi",
		"docs/en.md": "# en",
		"LICENSE":    "MIT",
	}
	a := buildDiffLayout(t, filesA, BuildLayoutOptions{})
	b := buildDiffLayout(t, filesB, BuildLayoutOptions{})

	result, err := Diff(t.Context(), a, b, DiffOptions{MetadataOnly: true})
	if err != nil {
		t.Fatalf("Diff: %v", err)
	}
	if result.Equal || len(result.Locales) != 1 || len(result.Components) != 0 {
		t.Fatalf("expected locale-only diff, got %+v", result)
	}
}

func TestDiffDistinguishesEmptyAndAbsentAnnotation(t *testing.T) {
	files := diffFixtureFiles()
	a := buildDiffLayout(t, files, BuildLayoutOptions{Plan: PlanOptions{
		Annotations: map[string]string{"org.example.empty": ""},
	}})
	b := buildDiffLayout(t, files, BuildLayoutOptions{})

	result, err := Diff(t.Context(), a, b, DiffOptions{})
	if err != nil {
		t.Fatalf("Diff: %v", err)
	}
	if result.Equal || len(result.Annotations) != 1 {
		t.Fatalf("expected one annotation diff, got %+v", result)
	}
	annotation := result.Annotations[0]
	if !annotation.BeforeSet || annotation.AfterSet || annotation.Before != "" || annotation.After != "" {
		t.Fatalf("expected empty-versus-absent flags, got %+v", annotation)
	}
}

func TestDiffDetectsSubjectChange(t *testing.T) {
	files := diffFixtureFiles()
	subject := &ocispec.Descriptor{MediaType: "application/vnd.oci.image.manifest.v1+json", Digest: digest.FromString("subject"), Size: 1}
	a := buildDiffLayout(t, files, BuildLayoutOptions{Subject: subject})
	b := buildDiffLayout(t, files, BuildLayoutOptions{})

	result, err := Diff(t.Context(), a, b, DiffOptions{MetadataOnly: true})
	if err != nil {
		t.Fatalf("Diff: %v", err)
	}
	if result.Equal || result.SubjectBefore == nil || result.SubjectAfter != nil {
		t.Fatalf("expected subject-only difference, got %+v", result)
	}
}

func TestDiffDetectsComponentContentChange(t *testing.T) {
	filesA := diffFixtureFiles()
	filesB := diffFixtureFiles()
	filesB["README.md"] = "# hi, changed"

	a := buildDiffLayout(t, filesA, BuildLayoutOptions{})
	b := buildDiffLayout(t, filesB, BuildLayoutOptions{})

	result, err := Diff(context.Background(), a, b, DiffOptions{})
	if err != nil {
		t.Fatalf("Diff: %v", err)
	}

	if result.Equal {
		t.Fatal("expected a changed component to make the artifacts unequal")
	}

	var doc *ComponentDiff

	for i := range result.Components {
		if result.Components[i].Component == spec.ComponentDocumentation {
			doc = &result.Components[i]
		}
	}

	if doc == nil {
		t.Fatalf("expected a documentation component diff, got %+v", result.Components)
	}

	if doc.Presence != ComponentPresent || !doc.DigestChanged {
		t.Fatalf("expected documentation to be present with a changed digest, got %+v", doc)
	}

	if len(doc.Files) != 1 || doc.Files[0].Path != "README.md" || doc.Files[0].Change != FileModified {
		t.Fatalf("expected exactly one modified README.md file diff, got %+v", doc.Files)
	}

	if doc.Files[0].SizeBefore != 4 || doc.Files[0].SizeAfter != 13 {
		t.Fatalf("got sizes %d -> %d, want 4 -> 13", doc.Files[0].SizeBefore, doc.Files[0].SizeAfter)
	}
}

func TestDiffDetectsSameSizeComponentContentChange(t *testing.T) {
	filesA := diffFixtureFiles()
	filesB := diffFixtureFiles()
	filesB["README.md"] = "same"

	a := buildDiffLayout(t, filesA, BuildLayoutOptions{})
	b := buildDiffLayout(t, filesB, BuildLayoutOptions{})

	result, err := Diff(context.Background(), a, b, DiffOptions{})
	if err != nil {
		t.Fatalf("Diff: %v", err)
	}

	var doc *ComponentDiff
	for i := range result.Components {
		if result.Components[i].Component == spec.ComponentDocumentation {
			doc = &result.Components[i]
		}
	}

	if doc == nil {
		t.Fatalf("expected a documentation component diff, got %+v", result.Components)
	}
	if len(doc.Files) != 1 || doc.Files[0].Path != "README.md" || doc.Files[0].Change != FileModified {
		t.Fatalf("expected same-size README.md replacement to be modified, got %+v", doc.Files)
	}
	if doc.Files[0].SizeBefore != 4 || doc.Files[0].SizeAfter != 4 {
		t.Fatalf("got sizes %d -> %d, want 4 -> 4", doc.Files[0].SizeBefore, doc.Files[0].SizeAfter)
	}
}

func TestDiffDetectsComponentAddedAndRemoved(t *testing.T) {
	filesA := diffFixtureFiles()

	filesB := map[string]string{
		"ocidoc.yaml": `
schemaVersion: v1beta
components:
  documentation:
    paths:
      - /README.md
  changelog:
    paths:
      - /CHANGELOG.md
`,
		"README.md":    "# hi",
		"CHANGELOG.md": "# changelog",
	}

	a := buildDiffLayout(t, filesA, BuildLayoutOptions{})
	b := buildDiffLayout(t, filesB, BuildLayoutOptions{})

	result, err := Diff(context.Background(), a, b, DiffOptions{})
	if err != nil {
		t.Fatalf("Diff: %v", err)
	}

	byType := make(map[spec.ComponentType]ComponentDiff, len(result.Components))
	for _, cd := range result.Components {
		byType[cd.Component] = cd
	}

	license, ok := byType[spec.ComponentLicense]
	if !ok || license.Presence != ComponentRemoved {
		t.Fatalf("expected license to be reported removed, got %+v", byType)
	}

	changelog, ok := byType[spec.ComponentChangelog]
	if !ok || changelog.Presence != ComponentAdded {
		t.Fatalf("expected changelog to be reported added, got %+v", byType)
	}

	if len(changelog.Files) != 1 || changelog.Files[0].Change != FileAdded {
		t.Fatalf("expected changelog's file diff to show one added file, got %+v", changelog.Files)
	}
}

func TestDiffMetadataOnlySkipsFileLevelDiff(t *testing.T) {
	filesA := diffFixtureFiles()
	filesB := diffFixtureFiles()
	filesB["README.md"] = "# hi, changed"

	a := buildDiffLayout(t, filesA, BuildLayoutOptions{})
	b := buildDiffLayout(t, filesB, BuildLayoutOptions{})

	result, err := Diff(context.Background(), a, b, DiffOptions{MetadataOnly: true})
	if err != nil {
		t.Fatalf("Diff: %v", err)
	}

	if result.Equal {
		t.Fatal("expected the digest change to still be reported under MetadataOnly")
	}

	for _, cd := range result.Components {
		if cd.Files != nil {
			t.Fatalf("expected MetadataOnly to skip file-level diffs, got %+v", cd.Files)
		}
	}
}

func TestDiffDetectsEntrypointChangeWithoutFileLevelDiff(t *testing.T) {
	filesA := map[string]string{
		"ocidoc.yaml": `
schemaVersion: v1beta
components:
  documentation:
    paths:
      - /README.md
      - /docs/guide.md
`,
		"README.md":     "# hi",
		"docs/guide.md": "guide",
	}

	filesB := map[string]string{
		"ocidoc.yaml": `
schemaVersion: v1beta
components:
  documentation:
    entrypoint: /docs/guide.md
    paths:
      - /README.md
      - /docs/guide.md
`,
		"README.md":     "# hi",
		"docs/guide.md": "guide",
	}

	a := buildDiffLayout(t, filesA, BuildLayoutOptions{})
	b := buildDiffLayout(t, filesB, BuildLayoutOptions{})

	result, err := Diff(context.Background(), a, b, DiffOptions{})
	if err != nil {
		t.Fatalf("Diff: %v", err)
	}

	if result.Equal {
		t.Fatal("expected an entrypoint change to make the artifacts unequal")
	}

	var doc *ComponentDiff

	for i := range result.Components {
		if result.Components[i].Component == spec.ComponentDocumentation {
			doc = &result.Components[i]
		}
	}

	if doc == nil || !doc.EntrypointChanged {
		t.Fatalf("expected an entrypoint change on documentation, got %+v", result.Components)
	}

	if doc.DigestChanged {
		t.Fatalf("expected the digest to be unchanged (same file content), got %+v", doc)
	}

	if doc.Files != nil {
		t.Fatalf("expected no file-level diff for an entrypoint-only change, got %+v", doc.Files)
	}
}

func TestDiffComponentOptionNarrowsScope(t *testing.T) {
	filesA := diffFixtureFiles()
	filesB := diffFixtureFiles()
	filesB["README.md"] = "# hi, changed"
	filesB["LICENSE"] = "Apache-2.0"

	a := buildDiffLayout(t, filesA, BuildLayoutOptions{})
	b := buildDiffLayout(t, filesB, BuildLayoutOptions{})

	result, err := Diff(context.Background(), a, b, DiffOptions{Component: spec.ComponentLicense})
	if err != nil {
		t.Fatalf("Diff: %v", err)
	}

	if len(result.Components) != 1 || result.Components[0].Component != spec.ComponentLicense {
		t.Fatalf("expected exactly one license component diff, got %+v", result.Components)
	}
}

func TestDiffComponentOptionRejectsUnknownComponent(t *testing.T) {
	files := diffFixtureFiles()
	a := buildDiffLayout(t, files, BuildLayoutOptions{})
	b := buildDiffLayout(t, files, BuildLayoutOptions{})

	_, err := Diff(context.Background(), a, b, DiffOptions{Component: spec.ComponentChangelog})
	if !errors.Is(err, spec.ErrNotFound) {
		t.Fatalf("expected errors.Is(err, spec.ErrNotFound), got %v", err)
	}
}
