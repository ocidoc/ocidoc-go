// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: Copyright 2026 WoozyMasta
// Source: github.com/ocidoc/ocidoc-go

package archive

import (
	"archive/tar"
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func writeFile(t *testing.T, path, content string) {
	t.Helper()

	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}

	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("WriteFile(%s): %v", path, err)
	}
}

func TestBuildTarRejectsCanceledContext(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "a.md"), "a content")

	entries := []Entry{{BundlePath: "a.md", SourcePath: filepath.Join(dir, "a.md")}}

	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	var buf bytes.Buffer

	if _, err := BuildTar(ctx, &buf, entries, time.Time{}); !errors.Is(err, context.Canceled) {
		t.Fatalf("BuildTar: got %v, want errors.Is(err, context.Canceled)", err)
	}
	if buf.Len() != 0 {
		t.Fatalf("expected no bytes written after cancellation, got %d", buf.Len())
	}
}

func TestBuildTarDeterministicAcrossRuns(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "b.md"), "b content")
	writeFile(t, filepath.Join(dir, "a.md"), "a content")

	entries := []Entry{
		{BundlePath: "b.md", SourcePath: filepath.Join(dir, "b.md")},
		{BundlePath: "a.md", SourcePath: filepath.Join(dir, "a.md")},
	}

	modTime := time.Unix(1700000000, 0).UTC()

	var buf1, buf2 bytes.Buffer

	if _, err := BuildTar(t.Context(), &buf1, entries, modTime); err != nil {
		t.Fatalf("BuildTar (1): %v", err)
	}

	if _, err := BuildTar(t.Context(), &buf2, entries, modTime); err != nil {
		t.Fatalf("BuildTar (2): %v", err)
	}

	if !bytes.Equal(buf1.Bytes(), buf2.Bytes()) {
		t.Fatal("expected identical output across two BuildTar calls with the same inputs")
	}
}

func TestBuildTarSortsEntriesByBundlePath(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "z.md"), "z")
	writeFile(t, filepath.Join(dir, "a.md"), "a")

	// Deliberately out of order.
	entries := []Entry{
		{BundlePath: "z.md", SourcePath: filepath.Join(dir, "z.md")},
		{BundlePath: "a.md", SourcePath: filepath.Join(dir, "a.md")},
	}

	var buf bytes.Buffer

	if _, err := BuildTar(t.Context(), &buf, entries, time.Time{}); err != nil {
		t.Fatalf("BuildTar: %v", err)
	}

	names := readTarNames(t, buf.Bytes())

	want := []string{"a.md", "z.md"}
	if names[0] != want[0] || names[1] != want[1] {
		t.Fatalf("got %v, want %v", names, want)
	}
}

func TestBuildTarFixesOwnershipModeAndTime(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "exec.sh")
	writeFile(t, path, "#!/bin/sh\n")

	if err := os.Chmod(path, 0o755); err != nil {
		t.Fatalf("Chmod: %v", err)
	}

	modTime := time.Unix(1700000000, 0).UTC()

	var buf bytes.Buffer

	if _, err := BuildTar(t.Context(), &buf, []Entry{{BundlePath: "exec.sh", SourcePath: path}}, modTime); err != nil {
		t.Fatalf("BuildTar: %v", err)
	}

	tr := tar.NewReader(bytes.NewReader(buf.Bytes()))

	header, err := tr.Next()
	if err != nil {
		t.Fatalf("tar.Next: %v", err)
	}

	if header.Uid != 0 || header.Gid != 0 {
		t.Fatalf("got uid/gid %d/%d, want 0/0", header.Uid, header.Gid)
	}

	if header.Uname != "" || header.Gname != "" {
		t.Fatalf("got uname/gname %q/%q, want empty", header.Uname, header.Gname)
	}

	if header.Mode != regularFileMode {
		t.Fatalf("got mode %o, want %o (source file mode must not leak through)", header.Mode, regularFileMode)
	}

	if !header.ModTime.Equal(modTime) {
		t.Fatalf("got mtime %v, want %v", header.ModTime, modTime)
	}

	if !header.AccessTime.IsZero() || !header.ChangeTime.IsZero() {
		t.Fatalf("expected zero access/change time, got %v/%v", header.AccessTime, header.ChangeTime)
	}
}

func TestBuildTarReportsFileCountAndSize(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "a.md"), "12345")
	writeFile(t, filepath.Join(dir, "b.md"), "1234567890")

	entries := []Entry{
		{BundlePath: "a.md", SourcePath: filepath.Join(dir, "a.md")},
		{BundlePath: "b.md", SourcePath: filepath.Join(dir, "b.md")},
	}

	var buf bytes.Buffer

	info, err := BuildTar(t.Context(), &buf, entries, time.Time{})
	if err != nil {
		t.Fatalf("BuildTar: %v", err)
	}

	if info.FileCount != 2 {
		t.Fatalf("got FileCount %d, want 2", info.FileCount)
	}

	if info.UncompressedSize != int64(buf.Len()) {
		t.Fatalf("got UncompressedSize %d, want %d (buffer length)", info.UncompressedSize, buf.Len())
	}

	if info.UncompressedSize <= 15 {
		t.Fatalf("expected UncompressedSize to include tar headers, got %d", info.UncompressedSize)
	}
}

func TestBuildTarRejectsDirectorySource(t *testing.T) {
	dir := t.TempDir()
	sub := filepath.Join(dir, "sub")

	if err := os.Mkdir(sub, 0o750); err != nil {
		t.Fatalf("Mkdir: %v", err)
	}

	_, err := BuildTar(t.Context(), nil, []Entry{{BundlePath: "sub", SourcePath: sub}}, time.Time{})
	if err == nil {
		t.Fatal("expected error for a directory source path")
	}
}

func TestResolveModTimeDefaultsToEpoch(t *testing.T) {
	t.Setenv(sourceDateEpochEnv, "")

	got := ResolveModTime()
	if !got.Equal(time.Unix(0, 0).UTC()) {
		t.Fatalf("got %v, want Unix epoch", got)
	}
}

func TestResolveModTimeReadsSourceDateEpoch(t *testing.T) {
	t.Setenv(sourceDateEpochEnv, "1700000000")

	got := ResolveModTime()
	if !got.Equal(time.Unix(1700000000, 0).UTC()) {
		t.Fatalf("got %v, want 1700000000", got)
	}
}

func TestResolveModTimeIgnoresInvalidValue(t *testing.T) {
	t.Setenv(sourceDateEpochEnv, "not-a-number")

	got := ResolveModTime()
	if !got.Equal(time.Unix(0, 0).UTC()) {
		t.Fatalf("got %v, want Unix epoch fallback", got)
	}
}

// readTarNames returns entry names in the order they appear in data.
func readTarNames(t *testing.T, data []byte) []string {
	t.Helper()

	tr := tar.NewReader(bytes.NewReader(data))

	var names []string

	for {
		header, err := tr.Next()
		if err == io.EOF {
			break
		}

		if err != nil {
			t.Fatalf("tar.Next: %v", err)
		}

		names = append(names, header.Name)
	}

	return names
}
