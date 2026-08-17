// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: Copyright 2026 WoozyMasta
// Source: github.com/ocidoc/ocidoc-go

package archive

import (
	"archive/tar"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/ocidoc/ocidoc-go/spec"
)

// Default extraction limits.
const (
	DefaultMaxFiles     = 100000
	DefaultMaxTotalSize = 4 << 30 // 4 GiB
	DefaultMaxFileSize  = 1 << 30 // 1 GiB
)

// ExtractOptions bounds a safe tar extraction.
// Zero-valued limit fields mean "use the default" (see DefaultExtractOptions);
// to genuinely disable a limit, set it to a very large explicit value,
// since a tar this package extracts is treated as untrusted input by default -
// a `.ocidoc` file may come from an untrusted source and be malicious.
type ExtractOptions struct {
	// MaxFiles limits regular files extracted from one archive.
	MaxFiles int

	// MaxTotalSize limits total extracted file bytes.
	MaxTotalSize int64

	// MaxFileSize limits bytes extracted for one file.
	MaxFileSize int64

	// ComputeDigest computes a SHA-256 digest for every scanned file.
	ComputeDigest bool

	// Overwrite allows extraction to replace an existing file.
	// Default false: create files without overwrite unless explicitly asked.
	Overwrite bool
}

// DefaultExtractOptions returns the package's default extraction limits.
func DefaultExtractOptions() ExtractOptions {
	return ExtractOptions{
		MaxFiles:     DefaultMaxFiles,
		MaxTotalSize: DefaultMaxTotalSize,
		MaxFileSize:  DefaultMaxFileSize,
	}
}

// Extract safely extracts a POSIX tar read from r into destDir.
// For each entry:
//
//  1. reject anything but a regular file;
//  2. reject an unsafe name
//     (empty, NUL, backslash-separated, absolute, or containing a ".." segment);
//  3. join under destDir and verify the joined path is still under it
//     (defense in depth beyond step 2's syntax check);
//  4. reject symlinked destination and parent path components;
//  5. enforce MaxFiles/MaxTotalSize/MaxFileSize before writing;
//  6. create the file without overwrite unless Overwrite is set;
//  7. write exactly the entry's declared size, then close before continuing to the next entry;
//  8. delete the partial file if writing that entry fails.
//
// destDir is created if it does not already exist.
// Existing symlinks in destDir or its parent chain are rejected both
// before and after directory creation.
// This prevents extraction through pre-existing links;
// callers must still avoid extracting into a directory concurrently mutated
// by an untrusted process because portable path APIs cannot make the full check
// and file creation sequence atomic.
func Extract(r io.Reader, destDir string, opts ExtractOptions) (Info, error) {
	opts = applyExtractDefaults(opts)

	destAbs, err := filepath.Abs(destDir)
	if err != nil {
		return Info{}, fmt.Errorf("resolve destination %q: %w", destDir, err)
	}

	if err := ensureDirectoryPath(destAbs); err != nil {
		return Info{}, fmt.Errorf("validate destination %q: %w", destDir, err)
	}

	if err := os.MkdirAll(destAbs, 0o750); err != nil {
		return Info{}, fmt.Errorf("create destination %q: %w", destDir, err)
	}
	if err := ensureDirectoryPath(destAbs); err != nil {
		return Info{}, fmt.Errorf("validate created destination %q: %w", destDir, err)
	}

	tr := tar.NewReader(r)

	var fileCount int

	var totalSize int64

	for {
		header, err := tr.Next()
		if errors.Is(err, io.EOF) {
			break
		}

		if err != nil {
			return Info{}, fmt.Errorf("read tar header: %w", err)
		}

		if header.Typeflag != tar.TypeReg {
			return Info{}, fmt.Errorf("%w: entry %q is not a regular file", spec.ErrInvalid, header.Name)
		}

		relPath, err := sanitizeExtractPath(header.Name)
		if err != nil {
			return Info{}, err
		}

		fileCount++
		if fileCount > opts.MaxFiles {
			return Info{}, fmt.Errorf("%w: extraction exceeds max file count %d", spec.ErrUnsupported, opts.MaxFiles)
		}

		if header.Size > opts.MaxFileSize {
			return Info{}, fmt.Errorf("%w: entry %q exceeds max file size %d bytes",
				spec.ErrUnsupported, header.Name, opts.MaxFileSize)
		}

		totalSize += header.Size
		if totalSize > opts.MaxTotalSize {
			return Info{}, fmt.Errorf("%w: extraction exceeds max total size %d bytes",
				spec.ErrUnsupported, opts.MaxTotalSize)
		}

		target := filepath.Join(destAbs, filepath.FromSlash(relPath))
		if !isWithinRoot(destAbs, target) {
			return Info{}, fmt.Errorf("%w: entry %q escapes destination", spec.ErrInvalid, header.Name)
		}

		if err := extractFile(tr, target, header.Size, opts.Overwrite); err != nil {
			return Info{}, fmt.Errorf("extract %q: %w", header.Name, err)
		}
	}

	return Info{FileCount: fileCount, UncompressedSize: totalSize}, nil
}

// applyExtractDefaults fills zero-valued limit fields with DefaultExtractOptions' values.
func applyExtractDefaults(opts ExtractOptions) ExtractOptions {
	if opts.MaxFiles == 0 {
		opts.MaxFiles = DefaultMaxFiles
	}

	if opts.MaxTotalSize == 0 {
		opts.MaxTotalSize = DefaultMaxTotalSize
	}

	if opts.MaxFileSize == 0 {
		opts.MaxFileSize = DefaultMaxFileSize
	}

	return opts
}

// sanitizeExtractPath rejects an unsafe tar entry name
// and otherwise returns it unchanged for joining under a destination root.
func sanitizeExtractPath(name string) (string, error) {
	if name == "" || strings.ContainsRune(name, 0) {
		return "", fmt.Errorf("%w: invalid entry name %q", spec.ErrInvalid, name)
	}

	if strings.Contains(name, "\\") {
		return "", fmt.Errorf("%w: entry %q uses backslash separators", spec.ErrInvalid, name)
	}

	if path.IsAbs(name) {
		return "", fmt.Errorf("%w: entry %q is an absolute path", spec.ErrInvalid, name)
	}

	for seg := range strings.SplitSeq(name, "/") {
		if seg == ".." {
			return "", fmt.Errorf("%w: entry %q contains a \"..\" segment", spec.ErrInvalid, name)
		}
	}

	return name, nil
}

// isWithinRoot reports whether target is root itself or a descendant of it.
func isWithinRoot(root, target string) bool {
	rel, err := filepath.Rel(root, target)
	if err != nil {
		return false
	}

	return rel == "." || (rel != ".." &&
		!strings.HasPrefix(rel, ".."+string(filepath.Separator)) &&
		!filepath.IsAbs(rel))
}

// extractFile writes exactly size bytes read from r to target,
// creating target (and its parent directories) fresh unless overwrite is set,
// and removing a partially written file on any error.
func extractFile(r io.Reader, target string, size int64, overwrite bool) error {
	parent := filepath.Dir(target)
	if err := ensureDirectoryPath(parent); err != nil {
		return fmt.Errorf("validate parent directory: %w", err)
	}
	if err := os.MkdirAll(parent, 0o750); err != nil {
		return fmt.Errorf("create parent directory: %w", err)
	}
	if err := ensureDirectoryPath(parent); err != nil {
		return fmt.Errorf("validate created parent directory: %w", err)
	}
	if err := ensureSafeTarget(target, overwrite); err != nil {
		return err
	}

	flags := os.O_WRONLY | os.O_CREATE | os.O_EXCL
	if overwrite {
		flags = os.O_WRONLY | os.O_CREATE | os.O_TRUNC
	}

	//nolint:gosec // target is validated and joined under destAbs above.
	f, err := os.OpenFile(target, flags, 0o640)
	if err != nil {
		return fmt.Errorf("create file: %w", err)
	}

	written, copyErr := io.CopyN(f, r, size)
	closeErr := f.Close()

	if copyErr != nil {
		_ = os.Remove(target)

		return fmt.Errorf("write content (%d/%d bytes): %w", written, size, copyErr)
	}

	if closeErr != nil {
		return fmt.Errorf("close file: %w", closeErr)
	}

	return nil
}

// ensureDirectoryPath rejects every existing symlink in target's absolute path
// and requires every existing component to be a directory.
func ensureDirectoryPath(target string) error {
	paths := pathPrefixes(filepath.Clean(target))
	for _, current := range paths {
		info, err := os.Lstat(current)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return fmt.Errorf("inspect %q: %w", current, err)
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("%w: path component %q is a symlink", spec.ErrInvalid, current)
		}
		if !info.IsDir() {
			return fmt.Errorf("%w: path component %q is not a directory", spec.ErrInvalid, current)
		}
	}

	return nil
}

// ensureSafeTarget rejects an existing symlink,
// directory or protected file before extraction writes target.
// Lstat is required so a symlink is never followed even when overwrite is explicitly allowed.
func ensureSafeTarget(target string, overwrite bool) error {
	info, err := os.Lstat(target)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("inspect target: %w", err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("%w: target %q is a symlink", spec.ErrInvalid, target)
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("%w: target %q is not a regular file", spec.ErrInvalid, target)
	}
	if !overwrite {
		return fmt.Errorf("target %q already exists", target)
	}

	return nil
}

// pathPrefixes returns path and all its parents ordered from the filesystem root to path.
// Walking upward avoids platform-specific parsing of drive and UNC roots.
func pathPrefixes(target string) []string {
	var reversed []string
	for current := target; ; current = filepath.Dir(current) {
		reversed = append(reversed, current)
		parent := filepath.Dir(current)
		if parent == current {
			break
		}
	}

	paths := make([]string, len(reversed))
	for i := range reversed {
		paths[len(reversed)-1-i] = reversed[i]
	}

	return paths
}
