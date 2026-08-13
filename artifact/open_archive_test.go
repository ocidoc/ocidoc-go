// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: Copyright 2026 WoozyMasta
// Source: github.com/ocidoc/ocidoc-go

package artifact

import (
	"archive/tar"
	"context"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/ocidoc/ocidoc-go/spec"
)

func buildTestArchive(t *testing.T) string {
	t.Helper()

	layoutDir, _ := buildTestLayout(t)

	archivePath := filepath.Join(t.TempDir(), "documentation.ocidoc")

	f, err := os.Create(archivePath) //nolint:gosec // fixed test path under t.TempDir().
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	if err := PackageArchive(t.Context(), f, layoutDir, time.Unix(1700000000, 0).UTC()); err != nil {
		f.Close() //nolint:errcheck,gosec // best-effort cleanup before Fatalf.
		t.Fatalf("PackageArchive: %v", err)
	}

	if err := f.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	return archivePath
}

func TestOpenArchiveRoundTrip(t *testing.T) {
	archivePath := buildTestArchive(t)

	reader, err := OpenArchive(archivePath)
	if err != nil {
		t.Fatalf("OpenArchive: %v", err)
	}

	defer reader.Close()

	ctx := context.Background()

	manifest, err := reader.Manifest(ctx)
	if err != nil {
		t.Fatalf("Manifest: %v", err)
	}

	if manifest.ArtifactType != spec.ArtifactType {
		t.Fatalf("got artifactType %q, want %q", manifest.ArtifactType, spec.ArtifactType)
	}

	cfg, err := reader.Config(ctx)
	if err != nil {
		t.Fatalf("Config: %v", err)
	}

	if err := spec.ValidateArtifactConfig(cfg); err != nil {
		t.Fatalf("ValidateArtifactConfig: %v", err)
	}

	rc, _, err := reader.OpenComponent(ctx, spec.ComponentDocumentation)
	if err != nil {
		t.Fatalf("OpenComponent: %v", err)
	}

	defer rc.Close()

	data, err := io.ReadAll(rc)
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}

	if len(data) == 0 {
		t.Fatal("expected non-empty documentation component blob")
	}
}

func TestOpenArchiveMatchesOpenLayout(t *testing.T) {
	layoutDir, _ := buildTestLayout(t)

	archivePath := filepath.Join(t.TempDir(), "documentation.ocidoc")

	f, err := os.Create(archivePath) //nolint:gosec // fixed test path under t.TempDir().
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	if err := PackageArchive(t.Context(), f, layoutDir, time.Unix(1700000000, 0).UTC()); err != nil {
		t.Fatalf("PackageArchive: %v", err)
	}

	if err := f.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	layoutR, err := OpenLayout(layoutDir)
	if err != nil {
		t.Fatalf("OpenLayout: %v", err)
	}

	archiveR, err := OpenArchive(archivePath)
	if err != nil {
		t.Fatalf("OpenArchive: %v", err)
	}

	defer archiveR.Close()

	ctx := context.Background()

	wantManifest, err := layoutR.Manifest(ctx)
	if err != nil {
		t.Fatalf("layout Manifest: %v", err)
	}

	gotManifest, err := archiveR.Manifest(ctx)
	if err != nil {
		t.Fatalf("archive Manifest: %v", err)
	}

	if !reflect.DeepEqual(gotManifest, wantManifest) {
		t.Fatalf("got manifest %+v, want %+v", gotManifest, wantManifest)
	}
}

func TestOpenArchiveCloseRemovesTempDir(t *testing.T) {
	archivePath := buildTestArchive(t)

	reader, err := OpenArchive(archivePath)
	if err != nil {
		t.Fatalf("OpenArchive: %v", err)
	}

	ar, ok := reader.(*archiveReader)
	if !ok {
		t.Fatalf("got reader type %T, want *archiveReader", reader)
	}

	tempDir := ar.tempDir

	if _, err := os.Stat(tempDir); err != nil {
		t.Fatalf("expected temp dir to exist before Close: %v", err)
	}

	if err := reader.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	if _, err := os.Stat(tempDir); !os.IsNotExist(err) {
		t.Fatalf("expected temp dir to be removed after Close, stat err: %v", err)
	}
}

func TestOpenArchiveRejectsMissingFile(t *testing.T) {
	_, err := OpenArchive(filepath.Join(t.TempDir(), "missing.ocidoc"))
	if err == nil {
		t.Fatal("expected error for a missing archive file")
	}
}

func TestOpenArchiveCleansUpOnInvalidLayout(t *testing.T) {
	// A well-formed tar that is not a valid OCI Image Layout (no
	// oci-layout/index.json entries) must not leak a temp directory.
	archivePath := filepath.Join(t.TempDir(), "bad.ocidoc")

	f, err := os.Create(archivePath) //nolint:gosec // fixed test path under t.TempDir().
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	tw := tar.NewWriter(f)
	if err := tw.WriteHeader(&tar.Header{
		Name: "not-a-layout.txt", Typeflag: tar.TypeReg, Size: 5, Mode: 0o644,
	}); err != nil {
		t.Fatalf("WriteHeader: %v", err)
	}

	if _, err := tw.Write([]byte("hello")); err != nil {
		t.Fatalf("Write: %v", err)
	}

	if err := tw.Close(); err != nil {
		t.Fatalf("tar Close: %v", err)
	}

	if err := f.Close(); err != nil {
		t.Fatalf("file Close: %v", err)
	}

	before, err := os.ReadDir(os.TempDir())
	if err != nil {
		t.Fatalf("ReadDir(TempDir): %v", err)
	}

	_, err = OpenArchive(archivePath)
	if err == nil {
		t.Fatal("expected error for an archive that is not a valid OCI Image Layout")
	}

	after, err := os.ReadDir(os.TempDir())
	if err != nil {
		t.Fatalf("ReadDir(TempDir): %v", err)
	}

	if len(after) > len(before) {
		t.Fatalf("OpenArchive leaked a temp directory on failure: before=%d after=%d", len(before), len(after))
	}
}
