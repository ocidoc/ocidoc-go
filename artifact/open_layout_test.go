// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: Copyright 2026 WoozyMasta
// Source: github.com/ocidoc/ocidoc-go

package artifact

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/opencontainers/go-digest"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"

	"github.com/ocidoc/ocidoc-go/internal/ociblob"
	"github.com/ocidoc/ocidoc-go/spec"
)

func buildTestLayout(t *testing.T) (layoutDir string, built *AssembleResult) {
	t.Helper()

	root := newLayoutFixture(t)
	layoutDir = t.TempDir()

	_, built, err := BuildLayout(t.Context(), root, layoutDir, BuildLayoutOptions{
		ModTime: time.Unix(1700000000, 0).UTC(),
	})
	if err != nil {
		t.Fatalf("BuildLayout: %v", err)
	}

	return layoutDir, built
}

func TestOpenLayoutRoundTrip(t *testing.T) {
	layoutDir, built := buildTestLayout(t)

	reader, err := OpenLayout(layoutDir)
	if err != nil {
		t.Fatalf("OpenLayout: %v", err)
	}

	ctx := context.Background()

	root, err := reader.Root(ctx)
	if err != nil {
		t.Fatalf("Root: %v", err)
	}

	if root.MediaType != ocispec.MediaTypeImageManifest {
		t.Fatalf("got root mediaType %q, want %q", root.MediaType, ocispec.MediaTypeImageManifest)
	}

	manifest, err := reader.Manifest(ctx)
	if err != nil {
		t.Fatalf("Manifest: %v", err)
	}

	if !reflect.DeepEqual(manifest.Config, built.ConfigDescriptor) {
		t.Fatalf("got manifest.Config %+v, want %+v", manifest.Config, built.ConfigDescriptor)
	}

	cfg, err := reader.Config(ctx)
	if err != nil {
		t.Fatalf("Config: %v", err)
	}

	if err := spec.ValidateArtifactConfig(cfg); err != nil {
		t.Fatalf("ValidateArtifactConfig: %v", err)
	}

	components, err := reader.Components(ctx)
	if err != nil {
		t.Fatalf("Components: %v", err)
	}

	if len(components) != len(built.ComponentDescriptors) {
		t.Fatalf("got %d components, want %d", len(components), len(built.ComponentDescriptors))
	}

	// Components must be sorted by type.
	for i := 1; i < len(components); i++ {
		if components[i-1].Type >= components[i].Type {
			t.Fatalf("components not sorted: %v", components)
		}
	}

	for _, c := range components {
		want, ok := built.ComponentDescriptors[c.Type]
		if !ok {
			t.Fatalf("unexpected component %q in reader output", c.Type)
		}

		if !reflect.DeepEqual(c.Descriptor, want) {
			t.Fatalf("component %q descriptor mismatch: got %+v, want %+v", c.Type, c.Descriptor, want)
		}
	}
}

func TestOpenLayoutOpenComponentReadsCorrectContent(t *testing.T) {
	layoutDir, _ := buildTestLayout(t)

	reader, err := OpenLayout(layoutDir)
	if err != nil {
		t.Fatalf("OpenLayout: %v", err)
	}

	ctx := context.Background()

	rc, desc, err := reader.OpenComponent(ctx, spec.ComponentDocumentation)
	if err != nil {
		t.Fatalf("OpenComponent: %v", err)
	}

	defer rc.Close()

	data, err := io.ReadAll(rc)
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}

	if int64(len(data)) != desc.Descriptor.Size {
		t.Fatalf("got %d bytes, descriptor says %d", len(data), desc.Descriptor.Size)
	}

	if desc.Type != spec.ComponentDocumentation {
		t.Fatalf("got component type %q, want %q", desc.Type, spec.ComponentDocumentation)
	}
}

func TestOpenLayoutOpenComponentNotFound(t *testing.T) {
	layoutDir, _ := buildTestLayout(t)

	reader, err := OpenLayout(layoutDir)
	if err != nil {
		t.Fatalf("OpenLayout: %v", err)
	}

	_, _, err = reader.OpenComponent(context.Background(), spec.ComponentChangelog)
	if err == nil {
		t.Fatal("expected error for a component not present in the artifact")
	}

	if !errors.Is(err, spec.ErrNotFound) {
		t.Fatalf("expected errors.Is(err, spec.ErrNotFound), got %v", err)
	}
}

func TestOpenLayoutRejectsMissingLayoutFile(t *testing.T) {
	dir := t.TempDir()

	_, err := OpenLayout(dir)
	if err == nil {
		t.Fatal("expected error for a directory with no oci-layout file")
	}
}

func TestOpenLayoutRejectsUnsupportedVersion(t *testing.T) {
	layoutDir, _ := buildTestLayout(t)

	data, err := json.Marshal(ocispec.ImageLayout{Version: "99.0.0"})
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}

	if err := os.WriteFile(filepath.Join(layoutDir, ocispec.ImageLayoutFile), data, 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	_, err = OpenLayout(layoutDir)
	if err == nil {
		t.Fatal("expected error for an unsupported oci-layout version")
	}

	if !errors.Is(err, spec.ErrUnsupported) {
		t.Fatalf("expected errors.Is(err, spec.ErrUnsupported), got %v", err)
	}
}

func TestOpenLayoutRejectsMultipleManifests(t *testing.T) {
	layoutDir, built := buildTestLayout(t)

	index := ocispec.Index{
		Manifests: []ocispec.Descriptor{
			{MediaType: ocispec.MediaTypeImageManifest, Digest: built.ConfigDescriptor.Digest, Size: 1},
			{MediaType: ocispec.MediaTypeImageManifest, Digest: built.ConfigDescriptor.Digest, Size: 1},
		},
	}

	data, err := json.Marshal(index)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}

	if err := os.WriteFile(filepath.Join(layoutDir, ocispec.ImageIndexFile), data, 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	_, err = OpenLayout(layoutDir)
	if err == nil {
		t.Fatal("expected error for an index with more than one manifest")
	}

	if !errors.Is(err, spec.ErrAmbiguous) {
		t.Fatalf("expected errors.Is(err, spec.ErrAmbiguous), got %v", err)
	}
}

func TestOpenLayoutRejectsCorruptManifestBlob(t *testing.T) {
	layoutDir, _ := buildTestLayout(t)

	// Find the manifest blob via index.json rather than assuming a digest.
	indexData, err := os.ReadFile(filepath.Join(layoutDir, ocispec.ImageIndexFile))
	if err != nil {
		t.Fatalf("ReadFile(index.json): %v", err)
	}

	var index ocispec.Index
	if err := json.Unmarshal(indexData, &index); err != nil {
		t.Fatalf("unmarshal index.json: %v", err)
	}

	blobFile := filepath.Join(layoutDir, "blobs", "sha256", index.Manifests[0].Digest.Encoded())
	if err := os.WriteFile(blobFile, []byte("corrupted"), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	_, err = OpenLayout(layoutDir)
	if err == nil {
		t.Fatal("expected error for a manifest blob that does not match its digest")
	}

	if !errors.Is(err, spec.ErrVerification) {
		t.Fatalf("expected errors.Is(err, spec.ErrVerification), got %v", err)
	}
}

func TestOpenLayoutRejectsPathLikeRootDigest(t *testing.T) {
	layoutDir, _ := buildTestLayout(t)

	index := ocispec.Index{Manifests: []ocispec.Descriptor{{
		MediaType: ocispec.MediaTypeImageManifest,
		Digest:    "sha256:../../outside",
		Size:      2,
	}}}
	data, err := json.Marshal(index)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	if err := os.WriteFile(filepath.Join(layoutDir, ocispec.ImageIndexFile), data, 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	_, err = OpenLayout(layoutDir)
	if !errors.Is(err, spec.ErrInvalid) {
		t.Fatalf("expected errors.Is(err, spec.ErrInvalid), got %v", err)
	}
}

func TestOpenLayoutRejectsUnsupportedRootDigestWithoutPanic(t *testing.T) {
	layoutDir, _ := buildTestLayout(t)
	d := digest.Digest("unknown:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")

	unsupportedDir := filepath.Join(layoutDir, "blobs", "unknown")
	if err := os.MkdirAll(unsupportedDir, 0o750); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(filepath.Join(unsupportedDir, d.Encoded()), []byte("{}"), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	index := ocispec.Index{Manifests: []ocispec.Descriptor{{
		MediaType: ocispec.MediaTypeImageManifest,
		Digest:    d,
		Size:      2,
	}}}
	data, err := json.Marshal(index)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	if err := os.WriteFile(filepath.Join(layoutDir, ocispec.ImageIndexFile), data, 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	_, err = OpenLayout(layoutDir)
	if !errors.Is(err, spec.ErrInvalid) {
		t.Fatalf("expected errors.Is(err, spec.ErrInvalid), got %v", err)
	}
}

func TestOpenLayoutRejectsRootSizeMismatch(t *testing.T) {
	layoutDir, _ := buildTestLayout(t)

	indexData, err := os.ReadFile(filepath.Join(layoutDir, ocispec.ImageIndexFile))
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	var index ocispec.Index
	if err := json.Unmarshal(indexData, &index); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	index.Manifests[0].Size++

	indexData, err = json.Marshal(index)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	if err := os.WriteFile(filepath.Join(layoutDir, ocispec.ImageIndexFile), indexData, 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	_, err = OpenLayout(layoutDir)
	if !errors.Is(err, spec.ErrVerification) {
		t.Fatalf("expected errors.Is(err, spec.ErrVerification), got %v", err)
	}
}

func TestOpenLayoutRejectsOversizedIndex(t *testing.T) {
	layoutDir, _ := buildTestLayout(t)
	oversized := make([]byte, ociblob.MaxMetadataSize+1)

	if err := os.WriteFile(filepath.Join(layoutDir, ocispec.ImageIndexFile), oversized, 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	_, err := OpenLayout(layoutDir)
	if !errors.Is(err, spec.ErrInvalid) {
		t.Fatalf("expected errors.Is(err, spec.ErrInvalid), got %v", err)
	}
}
