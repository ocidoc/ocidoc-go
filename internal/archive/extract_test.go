// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: Copyright 2026 WoozyMasta
// Source: github.com/ocidoc/ocidoc-go

package archive

import (
	"archive/tar"
	"bytes"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func buildTestTar(t *testing.T, entries []Entry) []byte {
	t.Helper()

	var buf bytes.Buffer
	if _, err := BuildTar(t.Context(), &buf, entries, time.Time{}); err != nil {
		t.Fatalf("BuildTar: %v", err)
	}

	return buf.Bytes()
}

func TestExtractWritesFilesUnderDest(t *testing.T) {
	src := t.TempDir()
	writeFile(t, filepath.Join(src, "a.md"), "alpha")
	writeFile(t, filepath.Join(src, "docs", "b.md"), "bravo")

	data := buildTestTar(t, []Entry{
		{BundlePath: "a.md", SourcePath: filepath.Join(src, "a.md")},
		{BundlePath: "docs/b.md", SourcePath: filepath.Join(src, "docs", "b.md")},
	})

	dest := t.TempDir()

	info, err := Extract(bytes.NewReader(data), dest, ExtractOptions{})
	if err != nil {
		t.Fatalf("Extract: %v", err)
	}

	if info.FileCount != 2 {
		t.Fatalf("got FileCount %d, want 2", info.FileCount)
	}

	got, err := os.ReadFile(filepath.Join(dest, "a.md"))
	if err != nil {
		t.Fatalf("ReadFile(a.md): %v", err)
	}

	if string(got) != "alpha" {
		t.Fatalf("got %q, want %q", got, "alpha")
	}

	got, err = os.ReadFile(filepath.Join(dest, "docs", "b.md"))
	if err != nil {
		t.Fatalf("ReadFile(docs/b.md): %v", err)
	}

	if string(got) != "bravo" {
		t.Fatalf("got %q, want %q", got, "bravo")
	}
}

func TestExtractRefusesOverwriteByDefault(t *testing.T) {
	src := t.TempDir()
	writeFile(t, filepath.Join(src, "a.md"), "alpha")

	data := buildTestTar(t, []Entry{{BundlePath: "a.md", SourcePath: filepath.Join(src, "a.md")}})

	dest := t.TempDir()
	writeFile(t, filepath.Join(dest, "a.md"), "existing")

	if _, err := Extract(bytes.NewReader(data), dest, ExtractOptions{}); err == nil {
		t.Fatal("expected error when target file already exists and Overwrite is false")
	}

	got, err := os.ReadFile(filepath.Join(dest, "a.md"))
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}

	if string(got) != "existing" {
		t.Fatalf("existing file must be untouched on refusal, got %q", got)
	}
}

func TestExtractOverwriteReplacesExistingFile(t *testing.T) {
	src := t.TempDir()
	writeFile(t, filepath.Join(src, "a.md"), "alpha")

	data := buildTestTar(t, []Entry{{BundlePath: "a.md", SourcePath: filepath.Join(src, "a.md")}})

	dest := t.TempDir()
	writeFile(t, filepath.Join(dest, "a.md"), "existing")

	if _, err := Extract(bytes.NewReader(data), dest, ExtractOptions{Overwrite: true}); err != nil {
		t.Fatalf("Extract: %v", err)
	}

	got, err := os.ReadFile(filepath.Join(dest, "a.md"))
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}

	if string(got) != "alpha" {
		t.Fatalf("got %q, want %q", got, "alpha")
	}
}

func TestExtractRejectsPathTraversal(t *testing.T) {
	dest := t.TempDir()

	var buf bytes.Buffer

	tw := tar.NewWriter(&buf)
	if err := tw.WriteHeader(&tar.Header{
		Name: "../escape.md", Typeflag: tar.TypeReg, Size: 4, Mode: 0o644,
	}); err != nil {
		t.Fatalf("WriteHeader: %v", err)
	}

	if _, err := tw.Write([]byte("evil")); err != nil {
		t.Fatalf("Write: %v", err)
	}

	if err := tw.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	if _, err := Extract(bytes.NewReader(buf.Bytes()), dest, ExtractOptions{}); err == nil {
		t.Fatal("expected error for a path-traversal entry name")
	}

	if _, err := os.Stat(filepath.Join(filepath.Dir(dest), "escape.md")); err == nil {
		t.Fatal("path traversal entry must not have been written outside dest")
	}
}

func TestExtractRejectsAbsolutePath(t *testing.T) {
	dest := t.TempDir()

	var buf bytes.Buffer

	tw := tar.NewWriter(&buf)
	if err := tw.WriteHeader(&tar.Header{
		Name: "/etc/passwd", Typeflag: tar.TypeReg, Size: 0, Mode: 0o644,
	}); err != nil {
		t.Fatalf("WriteHeader: %v", err)
	}

	if err := tw.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	if _, err := Extract(bytes.NewReader(buf.Bytes()), dest, ExtractOptions{}); err == nil {
		t.Fatal("expected error for an absolute entry path")
	}
}

func TestIsWithinRootChecksParentPathSegments(t *testing.T) {
	root := filepath.Join(t.TempDir(), "root")

	tests := map[string]bool{
		root:                                 true,
		filepath.Join(root, "..foo"):         true,
		filepath.Join(root, "...", "file"):   true,
		filepath.Join(root, "child", "file"): true,
		filepath.Join(root, ".."):            false,
		filepath.Join(root, "..", "file"):    false,
	}

	for target, want := range tests {
		if got := isWithinRoot(root, target); got != want {
			t.Errorf("isWithinRoot(%q, %q) = %v, want %v", root, target, got, want)
		}
	}
}

func TestExtractRejectsNonRegularEntry(t *testing.T) {
	dest := t.TempDir()

	var buf bytes.Buffer

	tw := tar.NewWriter(&buf)
	if err := tw.WriteHeader(&tar.Header{
		Name: "link", Typeflag: tar.TypeSymlink, Linkname: "/etc/passwd",
	}); err != nil {
		t.Fatalf("WriteHeader: %v", err)
	}

	if err := tw.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	if _, err := Extract(bytes.NewReader(buf.Bytes()), dest, ExtractOptions{}); err == nil {
		t.Fatal("expected error for a symlink entry")
	}
}

func TestExtractRejectsSymlinkDestination(t *testing.T) {
	src := t.TempDir()
	writeFile(t, filepath.Join(src, "a.md"), "alpha")
	data := buildTestTar(t, []Entry{{BundlePath: "a.md", SourcePath: filepath.Join(src, "a.md")}})

	root := t.TempDir()
	outside := t.TempDir()
	dest := filepath.Join(root, "dest")
	if err := os.Symlink(outside, dest); err != nil {
		t.Skipf("create directory symlink: %v", err)
	}

	if _, err := Extract(bytes.NewReader(data), dest, ExtractOptions{}); err == nil {
		t.Fatal("expected error for a symlink destination")
	}
	if _, err := os.Stat(filepath.Join(outside, "a.md")); !os.IsNotExist(err) {
		t.Fatalf("archive wrote through destination symlink: %v", err)
	}
}

func TestExtractRejectsSymlinkParent(t *testing.T) {
	src := t.TempDir()
	writeFile(t, filepath.Join(src, "b.md"), "bravo")
	data := buildTestTar(t, []Entry{{BundlePath: "docs/b.md", SourcePath: filepath.Join(src, "b.md")}})

	dest := t.TempDir()
	outside := t.TempDir()
	if err := os.Symlink(outside, filepath.Join(dest, "docs")); err != nil {
		t.Skipf("create directory symlink: %v", err)
	}

	if _, err := Extract(bytes.NewReader(data), dest, ExtractOptions{}); err == nil {
		t.Fatal("expected error for a symlink parent")
	}
	if _, err := os.Stat(filepath.Join(outside, "b.md")); !os.IsNotExist(err) {
		t.Fatalf("archive wrote through parent symlink: %v", err)
	}
}

func TestExtractRejectsSymlinkTargetWithOverwrite(t *testing.T) {
	src := t.TempDir()
	writeFile(t, filepath.Join(src, "a.md"), "replacement")
	data := buildTestTar(t, []Entry{{BundlePath: "a.md", SourcePath: filepath.Join(src, "a.md")}})

	dest := t.TempDir()
	outside := filepath.Join(t.TempDir(), "outside.md")
	writeFile(t, outside, "original")
	if err := os.Symlink(outside, filepath.Join(dest, "a.md")); err != nil {
		t.Skipf("create file symlink: %v", err)
	}

	if _, err := Extract(bytes.NewReader(data), dest, ExtractOptions{Overwrite: true}); err == nil {
		t.Fatal("expected error for a symlink target")
	}
	got, err := os.ReadFile(outside)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if string(got) != "original" {
		t.Fatalf("symlink target was overwritten: %q", got)
	}
}

func TestExtractEnforcesMaxFileSize(t *testing.T) {
	src := t.TempDir()
	writeFile(t, filepath.Join(src, "big.md"), "0123456789")

	data := buildTestTar(t, []Entry{{BundlePath: "big.md", SourcePath: filepath.Join(src, "big.md")}})

	dest := t.TempDir()

	_, err := Extract(bytes.NewReader(data), dest, ExtractOptions{MaxFileSize: 5})
	if err == nil {
		t.Fatal("expected error when an entry exceeds MaxFileSize")
	}
}

func TestExtractEnforcesMaxFiles(t *testing.T) {
	src := t.TempDir()
	writeFile(t, filepath.Join(src, "a.md"), "a")
	writeFile(t, filepath.Join(src, "b.md"), "b")

	data := buildTestTar(t, []Entry{
		{BundlePath: "a.md", SourcePath: filepath.Join(src, "a.md")},
		{BundlePath: "b.md", SourcePath: filepath.Join(src, "b.md")},
	})

	dest := t.TempDir()

	_, err := Extract(bytes.NewReader(data), dest, ExtractOptions{MaxFiles: 1})
	if err == nil {
		t.Fatal("expected error when entry count exceeds MaxFiles")
	}
}

func TestExtractEnforcesMaxTotalSize(t *testing.T) {
	src := t.TempDir()
	writeFile(t, filepath.Join(src, "a.md"), "12345")
	writeFile(t, filepath.Join(src, "b.md"), "12345")

	data := buildTestTar(t, []Entry{
		{BundlePath: "a.md", SourcePath: filepath.Join(src, "a.md")},
		{BundlePath: "b.md", SourcePath: filepath.Join(src, "b.md")},
	})

	dest := t.TempDir()

	_, err := Extract(bytes.NewReader(data), dest, ExtractOptions{MaxTotalSize: 6})
	if err == nil {
		t.Fatal("expected error when total size exceeds MaxTotalSize")
	}
}

func TestDefaultExtractOptionsValues(t *testing.T) {
	got := DefaultExtractOptions()

	if got.MaxFiles != DefaultMaxFiles || got.MaxTotalSize != DefaultMaxTotalSize || got.MaxFileSize != DefaultMaxFileSize {
		t.Fatalf("got %+v, want the Default* constants", got)
	}
}
