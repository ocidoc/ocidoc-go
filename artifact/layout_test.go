// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: Copyright 2026 WoozyMasta
// Source: github.com/ocidoc/ocidoc-go

package artifact

import (
	"archive/tar"
	"bytes"
	"encoding/json"
	"os"
	"path"
	"path/filepath"
	"testing"
	"time"

	ocispec "github.com/opencontainers/image-spec/specs-go/v1"

	"github.com/ocidoc/ocidoc-go/spec"
)

func newLayoutFixture(t *testing.T) string {
	t.Helper()

	root := t.TempDir()
	writeConfigTree(t, root, map[string]string{
		"ocidoc.yaml": `
schemaVersion: v1beta
components:
  documentation:
    - /README.md
  license:
    - /LICENSE
`,
		"README.md": "# hi",
		"LICENSE":   "MIT",
	})

	return root
}

func TestBuildLayoutWritesExpectedTree(t *testing.T) {
	root := newLayoutFixture(t)
	layoutDir := t.TempDir()

	_, result, err := BuildLayout(t.Context(), root, layoutDir, BuildLayoutOptions{
		ModTime: time.Unix(1700000000, 0).UTC(),
	})
	if err != nil {
		t.Fatalf("BuildLayout: %v", err)
	}

	if _, err := os.Stat(filepath.Join(layoutDir, ocispec.ImageLayoutFile)); err != nil {
		t.Errorf("oci-layout missing: %v", err)
	}

	if _, err := os.Stat(filepath.Join(layoutDir, ocispec.ImageIndexFile)); err != nil {
		t.Errorf("index.json missing: %v", err)
	}

	blobsDir := filepath.Join(layoutDir, "blobs", "sha256")

	entries, err := os.ReadDir(blobsDir)
	if err != nil {
		t.Fatalf("ReadDir(blobs/sha256): %v", err)
	}

	// 2 components + config + manifest.
	if len(entries) != 4 {
		t.Fatalf("got %d blobs, want 4: %v", len(entries), entries)
	}

	for _, e := range entries {
		if filepath.Ext(e.Name()) == ".tmp" {
			t.Errorf("leftover temp file in blobs directory: %s", e.Name())
		}
	}

	for name, desc := range result.ComponentDescriptors {
		if _, err := os.Stat(filepath.Join(blobsDir, desc.Digest.Encoded())); err != nil {
			t.Errorf("component %q blob missing at digest path: %v", name, err)
		}
	}
}

func TestBuildLayoutIndexReferencesManifest(t *testing.T) {
	root := newLayoutFixture(t)
	layoutDir := t.TempDir()

	if _, _, err := BuildLayout(t.Context(), root, layoutDir, BuildLayoutOptions{}); err != nil {
		t.Fatalf("BuildLayout: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(layoutDir, ocispec.ImageIndexFile))
	if err != nil {
		t.Fatalf("ReadFile(index.json): %v", err)
	}

	var index ocispec.Index
	if err := json.Unmarshal(data, &index); err != nil {
		t.Fatalf("unmarshal index.json: %v", err)
	}

	if len(index.Manifests) != 1 {
		t.Fatalf("got %d manifests in index, want 1", len(index.Manifests))
	}

	manifestDesc := index.Manifests[0]
	if manifestDesc.MediaType != ocispec.MediaTypeImageManifest {
		t.Fatalf("got manifest mediaType %q, want %q", manifestDesc.MediaType, ocispec.MediaTypeImageManifest)
	}

	blobPath := filepath.Join(layoutDir, "blobs", "sha256", manifestDesc.Digest.Encoded())

	manifestBytes, err := os.ReadFile(blobPath)
	if err != nil {
		t.Fatalf("manifest blob not found at referenced digest: %v", err)
	}

	if int64(len(manifestBytes)) != manifestDesc.Size {
		t.Fatalf("manifest blob size %d != descriptor size %d", len(manifestBytes), manifestDesc.Size)
	}

	var manifest ocispec.Manifest
	if err := json.Unmarshal(manifestBytes, &manifest); err != nil {
		t.Fatalf("unmarshal manifest blob: %v", err)
	}

	if manifest.ArtifactType != spec.ArtifactType {
		t.Fatalf("got artifactType %q, want %q", manifest.ArtifactType, spec.ArtifactType)
	}
}

func TestBuildLayoutDeterministicAcrossRuns(t *testing.T) {
	root := newLayoutFixture(t)
	modTime := time.Unix(1700000000, 0).UTC()

	dir1 := t.TempDir()
	dir2 := t.TempDir()

	if _, _, err := BuildLayout(t.Context(), root, dir1, BuildLayoutOptions{ModTime: modTime}); err != nil {
		t.Fatalf("BuildLayout (1): %v", err)
	}

	if _, _, err := BuildLayout(t.Context(), root, dir2, BuildLayoutOptions{ModTime: modTime}); err != nil {
		t.Fatalf("BuildLayout (2): %v", err)
	}

	for _, name := range []string{ocispec.ImageLayoutFile, ocispec.ImageIndexFile} {
		b1, err := os.ReadFile(filepath.Join(dir1, name))
		if err != nil {
			t.Fatalf("ReadFile(%s) dir1: %v", name, err)
		}

		b2, err := os.ReadFile(filepath.Join(dir2, name))
		if err != nil {
			t.Fatalf("ReadFile(%s) dir2: %v", name, err)
		}

		if !bytes.Equal(b1, b2) {
			t.Errorf("%s differs between two builds with the same inputs", name)
		}
	}

	entries1, err := os.ReadDir(filepath.Join(dir1, "blobs", "sha256"))
	if err != nil {
		t.Fatalf("ReadDir dir1: %v", err)
	}

	entries2, err := os.ReadDir(filepath.Join(dir2, "blobs", "sha256"))
	if err != nil {
		t.Fatalf("ReadDir dir2: %v", err)
	}

	if len(entries1) != len(entries2) {
		t.Fatalf("got %d blobs in dir1, %d in dir2", len(entries1), len(entries2))
	}

	names1 := make(map[string]bool, len(entries1))
	for _, e := range entries1 {
		names1[e.Name()] = true
	}

	for _, e := range entries2 {
		if !names1[e.Name()] {
			t.Errorf("blob %s in dir2 not found in dir1 (digests diverged)", e.Name())
		}
	}
}

func TestBuildLayoutFailurePropagatesFromPlan(t *testing.T) {
	root := t.TempDir() // no ocidoc.yaml, no matching files at all: Plan fails.
	layoutDir := t.TempDir()

	_, _, err := BuildLayout(t.Context(), root, layoutDir, BuildLayoutOptions{})
	if err == nil {
		t.Fatal("expected error when Plan itself fails")
	}
}

func TestPackageArchiveContainsExpectedEntries(t *testing.T) {
	root := newLayoutFixture(t)
	layoutDir := t.TempDir()

	if _, _, err := BuildLayout(t.Context(), root, layoutDir, BuildLayoutOptions{}); err != nil {
		t.Fatalf("BuildLayout: %v", err)
	}

	var buf bytes.Buffer
	if err := PackageArchive(t.Context(), &buf, layoutDir, time.Time{}); err != nil {
		t.Fatalf("PackageArchive: %v", err)
	}

	tr := tar.NewReader(&buf)

	var names []string

	for {
		header, err := tr.Next()
		if err != nil {
			break
		}

		names = append(names, header.Name)

		if header.Typeflag != tar.TypeReg {
			t.Errorf("entry %q: got typeflag %v, want regular file", header.Name, header.Typeflag)
		}
	}

	hasLayout, hasIndex, blobCount := false, false, 0

	for _, name := range names {
		switch {
		case name == ocispec.ImageLayoutFile:
			hasLayout = true
		case name == ocispec.ImageIndexFile:
			hasIndex = true
		case path.Dir(name) == "blobs/sha256":
			blobCount++
		}
	}

	if !hasLayout {
		t.Error("missing oci-layout entry in .ocidoc archive")
	}

	if !hasIndex {
		t.Error("missing index.json entry in .ocidoc archive")
	}

	if blobCount != 4 {
		t.Errorf("got %d blob entries, want 4", blobCount)
	}
}

func TestPackageArchiveIsUncompressed(t *testing.T) {
	root := newLayoutFixture(t)
	layoutDir := t.TempDir()

	if _, _, err := BuildLayout(t.Context(), root, layoutDir, BuildLayoutOptions{}); err != nil {
		t.Fatalf("BuildLayout: %v", err)
	}

	var buf bytes.Buffer
	if err := PackageArchive(t.Context(), &buf, layoutDir, time.Time{}); err != nil {
		t.Fatalf("PackageArchive: %v", err)
	}

	// A valid, plain (non-gzip) tar must parse via archive/tar directly.
	tr := tar.NewReader(bytes.NewReader(buf.Bytes()))
	if _, err := tr.Next(); err != nil {
		t.Fatalf("expected a readable plain tar, got: %v", err)
	}
}
