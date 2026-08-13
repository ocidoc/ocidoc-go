// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: Copyright 2026 WoozyMasta
// Source: github.com/ocidoc/ocidoc-go

// Package sourcepath resolves source-tree paths without allowing symlink
// or junction targets to escape the source root.
package sourcepath

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/ocidoc/ocidoc-go/spec"
)

// Kind classifies a resolved source path.
type Kind uint8

// Resolved source path kinds.
const (
	KindMissing Kind = iota
	KindFile
	KindDirectory
)

// File separates the stable bundle name from the dereferenced filesystem source
// whose bytes must be archived under that name.
type File struct {
	// BundlePath is the logical path retained in the artifact.
	BundlePath string

	// SourcePath is the resolved filesystem file containing the bytes.
	SourcePath string
}

// Resolution is the result of resolving one logical bundle path.
type Resolution struct {
	// File identifies the resolved file when Kind is KindFile.
	File File
	// Kind reports whether the logical path is missing, a file or a directory.
	Kind Kind
}

// Resolver confines path resolution to one canonical source root.
type Resolver struct {
	root string
}

// New validates root and returns a confined resolver for it.
func New(root string) (*Resolver, error) {
	resolved, err := filepath.EvalSymlinks(root)
	if err != nil {
		return nil, fmt.Errorf("resolve source root %q: %w", root, err)
	}

	resolved, err = filepath.Abs(resolved)
	if err != nil {
		return nil, fmt.Errorf("make source root absolute: %w", err)
	}

	info, err := os.Stat(resolved)
	if err != nil {
		return nil, fmt.Errorf("stat source root: %w", err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("%w: source root %q is not a directory", spec.ErrInvalid, root)
	}

	return &Resolver{root: filepath.Clean(resolved)}, nil
}

// Resolve resolves bundlePath through symlinks and junctions.
// The returned File preserves bundlePath while SourcePath names the dereferenced regular file.
// Missing paths and directories are classified without being errors;
// escapes, cycles and special files are invalid.
func (r *Resolver) Resolve(bundlePath string) (Resolution, error) {
	if err := spec.ValidateBundlePath(bundlePath); err != nil {
		return Resolution{}, err
	}

	candidate := filepath.Join(r.root, filepath.FromSlash(bundlePath))
	resolved, err := filepath.EvalSymlinks(candidate)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return Resolution{File: File{BundlePath: bundlePath}, Kind: KindMissing}, nil
		}
		return Resolution{}, fmt.Errorf("%w: resolve source path %q: %v", spec.ErrInvalid, bundlePath, err)
	}

	resolved, err = filepath.Abs(resolved)
	if err != nil {
		return Resolution{}, fmt.Errorf("make source path %q absolute: %w", bundlePath, err)
	}
	if !withinRoot(r.root, resolved) {
		return Resolution{}, fmt.Errorf("%w: source path %q resolves outside root", spec.ErrInvalid, bundlePath)
	}

	info, err := os.Stat(resolved)
	if err != nil {
		return Resolution{}, fmt.Errorf("stat source path %q: %w", bundlePath, err)
	}

	result := Resolution{File: File{BundlePath: bundlePath, SourcePath: resolved}}
	switch {
	case info.Mode().IsRegular():
		result.Kind = KindFile

	case info.IsDir():
		result.Kind = KindDirectory

	default:
		return Resolution{}, fmt.Errorf("%w: source path %q is not a regular file or directory", spec.ErrInvalid, bundlePath)
	}

	return result, nil
}

// ResolveReference resolves target relative to the referring bundle path.
// A leading slash means the source root, matching browser URL-path semantics.
func (r *Resolver) ResolveReference(referrer, target string) (Resolution, error) {
	var bundlePath string
	if strings.HasPrefix(target, "/") {
		bundlePath = path.Clean(strings.TrimLeft(target, "/"))
	} else {
		bundlePath = path.Clean(path.Join(path.Dir(referrer), target))
	}
	if bundlePath == "." || bundlePath == ".." || strings.HasPrefix(bundlePath, "../") {
		return Resolution{}, fmt.Errorf("%w: reference %q from %q escapes source root",
			spec.ErrInvalid, target, referrer)
	}

	return r.Resolve(bundlePath)
}

// Walk visits every logical regular file under the root in deterministic order.
// Symlinked directories are traversed while ancestor cycles are rejected;
// symlink entries themselves are never emitted.
func (r *Resolver) Walk(visit func(File) error) error {
	rootInfo, err := os.Stat(r.root)
	if err != nil {
		return fmt.Errorf("stat source root: %w", err)
	}

	return r.walkDirectory("", r.root, []fs.FileInfo{rootInfo}, visit)
}

// walkDirectory traverses sourceDir and maps its entries beneath bundleDir.
// ancestors contains resolved directories in the active traversal path for symlink-cycle detection.
func (r *Resolver) walkDirectory(
	bundleDir string,
	sourceDir string,
	ancestors []fs.FileInfo,
	visit func(File) error,
) error {
	entries, err := os.ReadDir(sourceDir)
	if err != nil {
		return fmt.Errorf("read source directory %q: %w", bundleDir, err)
	}

	for _, entry := range entries {
		bundlePath := path.Join(bundleDir, entry.Name())
		resolved, err := r.Resolve(bundlePath)
		if err != nil {
			return err
		}

		switch resolved.Kind {
		case KindFile:
			if err := visit(resolved.File); err != nil {
				return err
			}

		case KindDirectory:
			info, err := os.Stat(resolved.File.SourcePath)
			if err != nil {
				return fmt.Errorf("stat source directory %q: %w", bundlePath, err)
			}
			if sameAsAny(info, ancestors) {
				return fmt.Errorf("%w: source directory %q forms a symlink cycle", spec.ErrInvalid, bundlePath)
			}
			if err := r.walkDirectory(bundlePath, resolved.File.SourcePath, append(ancestors, info), visit); err != nil {
				return err
			}

		case KindMissing:
			return fmt.Errorf("%w: source path %q disappeared during traversal", spec.ErrInvalid, bundlePath)

		default:
			return fmt.Errorf("%w: source path %q has unknown kind", spec.ErrInvalid, bundlePath)
		}
	}

	return nil
}

// sameAsAny reports whether info identifies a directory already on the active traversal path.
func sameAsAny(info fs.FileInfo, ancestors []fs.FileInfo) bool {
	for _, ancestor := range ancestors {
		if os.SameFile(info, ancestor) {
			return true
		}
	}

	return false
}

// withinRoot reports whether target is within root after path cleaning.
func withinRoot(root, target string) bool {
	rel, err := filepath.Rel(root, target)
	if err != nil {
		return false
	}

	return rel == "." || (!filepath.IsAbs(rel) && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)))
}
