// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: Copyright 2026 WoozyMasta
// Source: github.com/ocidoc/ocidoc-go

package artifact

import (
	"archive/tar"
	"bytes"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/ocidoc/ocidoc-go/internal/compression"
	"github.com/ocidoc/ocidoc-go/spec"
)

func TestBuildComponentBlobDescriptor(t *testing.T) {
	root := t.TempDir()
	writeConfigTree(t, root, map[string]string{
		"README.md":     "# hi",
		"docs/guide.md": "guide",
	})

	modTime := time.Unix(1700000000, 0).UTC()

	var buf bytes.Buffer

	desc, err := buildComponentBlob(
		t.Context(), &buf, root, spec.ComponentDocumentation,
		[]string{"README.md", "docs/guide.md"}, "README.md",
		spec.CompressionGzip, 6, modTime,
	)
	if err != nil {
		t.Fatalf("buildComponentBlob: %v", err)
	}

	wantMediaType, _ := spec.CompressionGzip.MediaType()
	if desc.MediaType != wantMediaType {
		t.Fatalf("got MediaType %q, want %q", desc.MediaType, wantMediaType)
	}

	if desc.Size != int64(buf.Len()) {
		t.Fatalf("got Size %d, want %d (buffer length)", desc.Size, buf.Len())
	}

	if desc.Digest == "" {
		t.Fatal("expected a non-empty digest")
	}

	if got := desc.Annotations[spec.AnnotationComponentType]; got != string(spec.ComponentDocumentation) {
		t.Fatalf("got component type annotation %q, want %q", got, spec.ComponentDocumentation)
	}

	if got := desc.Annotations[spec.AnnotationComponentFileCount]; got != "2" {
		t.Fatalf("got file-count annotation %q, want 2", got)
	}

	if got := desc.Annotations[spec.AnnotationComponentEntrypoint]; got != "README.md" {
		t.Fatalf("got entrypoint annotation %q, want README.md", got)
	}

	if _, hasSize := desc.Annotations[spec.AnnotationComponentUncompressedSize]; !hasSize {
		t.Fatal("expected an uncompressed-size annotation")
	}
}

func TestBuildComponentBlobOmitsEntrypointAnnotationWhenNone(t *testing.T) {
	root := t.TempDir()
	writeConfigTree(t, root, map[string]string{"NOTICE": "notice"})

	var buf bytes.Buffer

	desc, err := buildComponentBlob(
		t.Context(), &buf, root, spec.ComponentLicense, []string{"NOTICE"}, "",
		spec.CompressionGzip, 6, time.Time{},
	)
	if err != nil {
		t.Fatalf("buildComponentBlob: %v", err)
	}

	if _, has := desc.Annotations[spec.AnnotationComponentEntrypoint]; has {
		t.Fatal("did not expect an entrypoint annotation")
	}
}

func TestBuildComponentBlobDeterministic(t *testing.T) {
	root := t.TempDir()
	writeConfigTree(t, root, map[string]string{"README.md": "# hi"})

	modTime := time.Unix(1700000000, 0).UTC()

	build := func() []byte {
		var buf bytes.Buffer

		if _, err := buildComponentBlob(
			t.Context(), &buf, root, spec.ComponentDocumentation, []string{"README.md"}, "README.md",
			spec.CompressionGzip, 6, modTime,
		); err != nil {
			t.Fatalf("buildComponentBlob: %v", err)
		}

		return buf.Bytes()
	}

	first := build()
	second := build()

	if !bytes.Equal(first, second) {
		t.Fatal("expected identical component blob bytes across two builds with the same inputs")
	}
}

func TestBuildComponentBlobZstd(t *testing.T) {
	root := t.TempDir()
	writeConfigTree(t, root, map[string]string{"README.md": "# hi"})

	var buf bytes.Buffer

	desc, err := buildComponentBlob(
		t.Context(), &buf, root, spec.ComponentDocumentation, []string{"README.md"}, "",
		spec.CompressionZstd, 3, time.Time{},
	)
	if err != nil {
		t.Fatalf("buildComponentBlob: %v", err)
	}

	wantMediaType, _ := spec.CompressionZstd.MediaType()
	if desc.MediaType != wantMediaType {
		t.Fatalf("got MediaType %q, want %q", desc.MediaType, wantMediaType)
	}
}

func TestBuildComponentBlobDereferencesSymlinkUnderLogicalPath(t *testing.T) {
	root := t.TempDir()
	writeConfigTree(t, root, map[string]string{"docs/real.md": "content"})
	if err := os.Symlink(filepath.Join(root, "docs", "real.md"), filepath.Join(root, "guide.md")); err != nil {
		t.Skipf("create symlink: %v", err)
	}

	var buf bytes.Buffer
	if _, err := buildComponentBlob(
		t.Context(), &buf, root, spec.ComponentDocumentation, []string{"guide.md"}, "guide.md",
		spec.CompressionGzip, 6, time.Time{},
	); err != nil {
		t.Fatalf("buildComponentBlob: %v", err)
	}

	uncompressed, err := compression.NewReader(bytes.NewReader(buf.Bytes()), spec.ComponentLayerGzip)
	if err != nil {
		t.Fatalf("NewReader: %v", err)
	}
	defer uncompressed.Close() //nolint:errcheck // test cleanup.

	tr := tar.NewReader(uncompressed)
	header, err := tr.Next()
	if err != nil {
		t.Fatalf("Next: %v", err)
	}
	if header.Name != "guide.md" || header.Typeflag != tar.TypeReg {
		t.Fatalf("header = %+v, want regular guide.md", header)
	}
	content, err := io.ReadAll(tr)
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}
	if string(content) != "content" {
		t.Fatalf("content = %q, want content", content)
	}
}
