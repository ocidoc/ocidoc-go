// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: Copyright 2026 WoozyMasta
// Source: github.com/ocidoc/ocidoc-go

package artifact

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	ocispec "github.com/opencontainers/image-spec/specs-go/v1"

	"github.com/ocidoc/ocidoc-go/spec"
)

// overrideReader wraps a real Reader and substitutes the Manifest,
// Config and/or Components results, letting a test simulate
// a structurally malformed artifact without hand-building one on disk.
type overrideReader struct {
	Reader
	manifest   *ocispec.Manifest
	config     *spec.ArtifactConfig
	components []ComponentDescriptor
}

func (o *overrideReader) Manifest(ctx context.Context) (*ocispec.Manifest, error) {
	if o.manifest != nil {
		return o.manifest, nil
	}

	return o.Reader.Manifest(ctx)
}

func (o *overrideReader) Config(ctx context.Context) (*spec.ArtifactConfig, error) {
	if o.config != nil {
		return o.config, nil
	}

	return o.Reader.Config(ctx)
}

func (o *overrideReader) Components(ctx context.Context) ([]ComponentDescriptor, error) {
	if o.components != nil {
		return o.components, nil
	}

	return o.Reader.Components(ctx)
}

func tamperBlob(t *testing.T, layoutDir string, desc ocispec.Descriptor) {
	t.Helper()

	blobFile := filepath.Join(layoutDir, "blobs", "sha256", desc.Digest.Encoded())

	original, err := os.ReadFile(blobFile) //nolint:gosec // fixed test path.
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}

	tampered := append([]byte{}, original...)
	tampered[0] ^= 0xFF

	if err := os.WriteFile(blobFile, tampered, 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
}

func TestVerifyValidArtifactHasNoIssues(t *testing.T) {
	layoutDir, _ := buildTestLayout(t)

	reader, err := OpenLayout(layoutDir)
	if err != nil {
		t.Fatalf("OpenLayout: %v", err)
	}

	result, err := Verify(context.Background(), reader, VerifyOptions{})
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}

	if !result.Valid {
		t.Fatalf("expected a valid artifact, got issues: %+v", result.Issues)
	}
}

func TestVerifyDetectsTamperedComponentDigest(t *testing.T) {
	layoutDir, built := buildTestLayout(t)
	tamperBlob(t, layoutDir, built.ComponentDescriptors[spec.ComponentDocumentation])

	reader, err := OpenLayout(layoutDir)
	if err != nil {
		t.Fatalf("OpenLayout: %v", err)
	}

	result, err := Verify(context.Background(), reader, VerifyOptions{})
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}

	if result.Valid {
		t.Fatal("expected a tampered component blob to be reported invalid")
	}

	found := false

	for _, issue := range result.Issues {
		if issue.Component == spec.ComponentDocumentation {
			found = true
		}
	}

	if !found {
		t.Fatalf("expected an issue for component %q, got %+v", spec.ComponentDocumentation, result.Issues)
	}
}

func TestVerifyMetadataOnlySkipsComponentReads(t *testing.T) {
	layoutDir, built := buildTestLayout(t)
	tamperBlob(t, layoutDir, built.ComponentDescriptors[spec.ComponentDocumentation])

	reader, err := OpenLayout(layoutDir)
	if err != nil {
		t.Fatalf("OpenLayout: %v", err)
	}

	result, err := Verify(context.Background(), reader, VerifyOptions{MetadataOnly: true})
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}

	if !result.Valid {
		t.Fatalf("expected MetadataOnly to never open a component blob, got issues: %+v", result.Issues)
	}
}

func TestVerifyComponentOptionRestrictsDeepCheck(t *testing.T) {
	layoutDir, built := buildTestLayout(t)
	tamperBlob(t, layoutDir, built.ComponentDescriptors[spec.ComponentDocumentation])

	reader, err := OpenLayout(layoutDir)
	if err != nil {
		t.Fatalf("OpenLayout: %v", err)
	}

	result, err := Verify(context.Background(), reader, VerifyOptions{Component: spec.ComponentLicense})
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}

	if !result.Valid {
		t.Fatalf("expected Component to restrict the deep check to license, got issues: %+v", result.Issues)
	}
}

func TestVerifyDetectsBadEntrypoint(t *testing.T) {
	layoutDir, _ := buildTestLayout(t)

	reader, err := OpenLayout(layoutDir)
	if err != nil {
		t.Fatalf("OpenLayout: %v", err)
	}

	cfg, err := reader.Config(context.Background())
	if err != nil {
		t.Fatalf("Config: %v", err)
	}

	badCfg := *cfg
	badCfg.Components = map[spec.ComponentType]spec.ComponentConfig{
		spec.ComponentDocumentation: {Entrypoint: "MISSING.md"},
		spec.ComponentLicense:       cfg.Components[spec.ComponentLicense],
	}

	result, err := Verify(context.Background(), &overrideReader{Reader: reader, config: &badCfg}, VerifyOptions{})
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}

	if result.Valid {
		t.Fatal("expected a non-existent entrypoint to be reported invalid")
	}

	found := false

	for _, issue := range result.Issues {
		if issue.Component == spec.ComponentDocumentation {
			found = true
		}
	}

	if !found {
		t.Fatalf("expected an issue for component %q, got %+v", spec.ComponentDocumentation, result.Issues)
	}
}

func TestVerifyDetectsComponentsAnnotationMismatch(t *testing.T) {
	layoutDir, _ := buildTestLayout(t)

	reader, err := OpenLayout(layoutDir)
	if err != nil {
		t.Fatalf("OpenLayout: %v", err)
	}

	manifest, err := reader.Manifest(context.Background())
	if err != nil {
		t.Fatalf("Manifest: %v", err)
	}

	badManifest := *manifest
	badManifest.Annotations = make(map[string]string, len(manifest.Annotations))

	for k, v := range manifest.Annotations {
		badManifest.Annotations[k] = v
	}

	badManifest.Annotations[spec.AnnotationComponents] = "documentation"

	result, err := Verify(context.Background(), &overrideReader{Reader: reader, manifest: &badManifest}, VerifyOptions{})
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}

	if result.Valid {
		t.Fatal("expected a mismatched org.ocidoc.components annotation to be reported invalid")
	}
}

func TestVerifyDetectsMissingConfigComponent(t *testing.T) {
	layoutDir, _ := buildTestLayout(t)

	reader, err := OpenLayout(layoutDir)
	if err != nil {
		t.Fatalf("OpenLayout: %v", err)
	}

	cfg, err := reader.Config(context.Background())
	if err != nil {
		t.Fatalf("Config: %v", err)
	}

	badCfg := *cfg
	badCfg.Components = map[spec.ComponentType]spec.ComponentConfig{
		spec.ComponentDocumentation: cfg.Components[spec.ComponentDocumentation],
	}

	result, err := Verify(context.Background(), &overrideReader{Reader: reader, config: &badCfg}, VerifyOptions{})
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}

	if result.Valid {
		t.Fatal("expected a manifest component missing from the artifact config to be reported invalid")
	}

	found := false

	for _, issue := range result.Issues {
		if issue.Component == spec.ComponentLicense {
			found = true
		}
	}

	if !found {
		t.Fatalf("expected an issue for component %q, got %+v", spec.ComponentLicense, result.Issues)
	}
}

func TestVerifyDetectsDuplicateComponent(t *testing.T) {
	layoutDir, _ := buildTestLayout(t)

	reader, err := OpenLayout(layoutDir)
	if err != nil {
		t.Fatalf("OpenLayout: %v", err)
	}

	components, err := reader.Components(context.Background())
	if err != nil {
		t.Fatalf("Components: %v", err)
	}

	duplicated := append(append([]ComponentDescriptor{}, components...), components[0])

	result, err := Verify(context.Background(), &overrideReader{Reader: reader, components: duplicated}, VerifyOptions{})
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}

	if result.Valid {
		t.Fatal("expected a duplicated component type to be reported invalid")
	}
}

func TestVerifyDetectsUnsafeTarEntry(t *testing.T) {
	layoutDir, built := buildTestLayout(t)

	desc := built.ComponentDescriptors[spec.ComponentDocumentation]
	blobFile := filepath.Join(layoutDir, "blobs", "sha256", desc.Digest.Encoded())

	var buf bytes.Buffer

	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)

	if err := tw.WriteHeader(&tar.Header{
		Typeflag: tar.TypeSymlink,
		Name:     "README.md",
		Linkname: "/etc/passwd",
		Mode:     0o644,
	}); err != nil {
		t.Fatalf("WriteHeader: %v", err)
	}

	if err := tw.Close(); err != nil {
		t.Fatalf("tar Close: %v", err)
	}

	if err := gz.Close(); err != nil {
		t.Fatalf("gzip Close: %v", err)
	}

	if err := os.WriteFile(blobFile, buf.Bytes(), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	reader, err := OpenLayout(layoutDir)
	if err != nil {
		t.Fatalf("OpenLayout: %v", err)
	}

	result, err := Verify(context.Background(), reader, VerifyOptions{})
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}

	if result.Valid {
		t.Fatal("expected a symlink tar entry to be reported invalid")
	}

	found := false

	for _, issue := range result.Issues {
		if issue.Component == spec.ComponentDocumentation && strings.Contains(issue.Message, "not a regular file") {
			found = true
		}
	}

	if !found {
		t.Fatalf("expected a \"not a regular file\" issue for component %q, got %+v", spec.ComponentDocumentation, result.Issues)
	}
}
