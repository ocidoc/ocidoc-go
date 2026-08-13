// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: Copyright 2026 WoozyMasta
// Source: github.com/ocidoc/ocidoc-go

package sourcepath

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/ocidoc/ocidoc-go/spec"
)

func TestResolvePreservesLogicalSymlinkPath(t *testing.T) {
	root := t.TempDir()
	realPath := filepath.Join(root, "real.md")
	if err := os.WriteFile(realPath, []byte("content"), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	if err := os.Symlink(realPath, filepath.Join(root, "linked.md")); err != nil {
		t.Skipf("create symlink: %v", err)
	}

	r, err := New(root)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	got, err := r.Resolve("linked.md")
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	gotInfo, err := os.Stat(got.File.SourcePath)
	if err != nil {
		t.Fatalf("Stat resolved source: %v", err)
	}
	wantInfo, err := os.Stat(realPath)
	if err != nil {
		t.Fatalf("Stat real source: %v", err)
	}
	if got.File.BundlePath != "linked.md" || !os.SameFile(gotInfo, wantInfo) {
		t.Fatalf("Resolve = %+v, want logical linked.md and source %q", got, realPath)
	}
}

func TestResolveRejectsSymlinkEscape(t *testing.T) {
	outer := t.TempDir()
	root := filepath.Join(outer, "root")
	if err := os.Mkdir(root, 0o750); err != nil {
		t.Fatalf("Mkdir: %v", err)
	}
	outside := filepath.Join(outer, "outside.md")
	if err := os.WriteFile(outside, []byte("outside"), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	if err := os.Symlink(outside, filepath.Join(root, "escape.md")); err != nil {
		t.Skipf("create symlink: %v", err)
	}

	r, err := New(root)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	_, err = r.Resolve("escape.md")
	if !errors.Is(err, spec.ErrInvalid) {
		t.Fatalf("Resolve error = %v, want spec.ErrInvalid", err)
	}
}

func TestWalkTraversesDirectorySymlinkAndRejectsCycle(t *testing.T) {
	root := t.TempDir()
	realDir := filepath.Join(root, "real")
	if err := os.Mkdir(realDir, 0o750); err != nil {
		t.Fatalf("Mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(realDir, "page.md"), []byte("page"), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	if err := os.Symlink(realDir, filepath.Join(root, "alias")); err != nil {
		t.Skipf("create symlink: %v", err)
	}

	r, err := New(root)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	var paths []string
	if err := r.Walk(func(file File) error {
		paths = append(paths, file.BundlePath)
		return nil
	}); err != nil {
		t.Fatalf("Walk: %v", err)
	}
	if want := []string{"alias/page.md", "real/page.md"}; !reflect.DeepEqual(paths, want) {
		t.Fatalf("Walk paths = %v, want %v", paths, want)
	}

	if err := os.Symlink(root, filepath.Join(realDir, "cycle")); err != nil {
		t.Skipf("create cycle symlink: %v", err)
	}
	if err := r.Walk(func(File) error { return nil }); !errors.Is(err, spec.ErrInvalid) {
		t.Fatalf("Walk cycle error = %v, want spec.ErrInvalid", err)
	}
}
