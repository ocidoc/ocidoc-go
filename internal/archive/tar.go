// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: Copyright 2026 WoozyMasta
// Source: github.com/ocidoc/ocidoc-go

package archive

import (
	"archive/tar"
	"context"
	"fmt"
	"io"
	"os"
	"sort"
	"strconv"
	"time"
)

// regularFileMode is the single fixed mode recorded for every tar entry,
// regardless of the source file's actual permissions -
// part of what makes a component tar byte-identical across builds.
const regularFileMode = 0o644

// sourceDateEpochEnv is the reproducible-builds convention environment variable
// (https://reproducible-builds.org/docs/source-date-epoch/)
// used as a determinism input for every entry's mtime.
const sourceDateEpochEnv = "SOURCE_DATE_EPOCH"

// Entry is one file to include in a component tar.
type Entry struct {
	// BundlePath is the entry's path inside the tar and inside the final OCIDoc component tree:
	// relative, POSIX, already validated
	// (spec.ValidateBundlePath/ValidateBundlePaths is the caller's job -
	// BuildTar does not repeat that check).
	BundlePath string

	// SourcePath is the local filesystem path to read content from.
	SourcePath string
}

// Info summarizes a built tar for the component descriptor's
// org.ocidoc.component.file-count and
// org.ocidoc.component.uncompressed-size annotations.
type Info struct {
	// FileCount is the number of regular files written to the tar.
	FileCount int

	// UncompressedSize is the sum of tar entry content sizes.
	UncompressedSize int64
}

// ResolveModTime resolves the fixed modification time used for every tar entry
// (and the gzip header written over it - see internal/compression):
// SOURCE_DATE_EPOCH, a Unix timestamp, or the Unix epoch when it is unset or not a valid integer.
func ResolveModTime() time.Time {
	raw := os.Getenv(sourceDateEpochEnv)
	if raw == "" {
		return time.Unix(0, 0).UTC()
	}

	seconds, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		return time.Unix(0, 0).UTC()
	}

	return time.Unix(seconds, 0).UTC()
}

// BuildTar writes a deterministic POSIX tar to w containing entries, sorted by BundlePath,
// with every entry's owner/group set to 0, owner/group names empty,
// mode fixed to regularFileMode, and mtime set to modTime (see ResolveModTime);
// access and change times are left zero so they are not emitted.
// Format is pinned to PAX for every entry so output does not depend on path length
// or Go's per-header format auto-selection.
//
// Only regular files are allowed; BuildTar returns an error for anything else.
//
// ctx is checked once per entry, so a caller that cancels it stops
// a large multi-file tar promptly instead of only after every entry has already been copied.
func BuildTar(ctx context.Context, w io.Writer, entries []Entry, modTime time.Time) (Info, error) {
	sorted := make([]Entry, len(entries))
	copy(sorted, entries)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].BundlePath < sorted[j].BundlePath })

	counting := &countingWriter{w: w}
	tw := tar.NewWriter(counting)

	for _, e := range sorted {
		if err := ctx.Err(); err != nil {
			return Info{}, err
		}

		if err := writeEntry(tw, e, modTime); err != nil {
			return Info{}, fmt.Errorf("tar entry %q: %w", e.BundlePath, err)
		}
	}

	if err := tw.Close(); err != nil {
		return Info{}, fmt.Errorf("close tar: %w", err)
	}

	return Info{FileCount: len(sorted), UncompressedSize: counting.n}, nil
}

// writeEntry writes one regular-file entry and its content.
func writeEntry(tw *tar.Writer, e Entry, modTime time.Time) error {
	info, err := os.Lstat(e.SourcePath)
	if err != nil {
		return err
	}

	if !info.Mode().IsRegular() {
		return fmt.Errorf("%s: not a regular file", e.SourcePath)
	}

	header := &tar.Header{
		Format:   tar.FormatPAX,
		Typeflag: tar.TypeReg,
		Name:     e.BundlePath,
		Size:     info.Size(),
		Mode:     regularFileMode,
		ModTime:  modTime,
	}

	if err := tw.WriteHeader(header); err != nil {
		return err
	}

	//nolint:gosec // path is from already-planned, validated ownership.
	f, err := os.Open(e.SourcePath)
	if err != nil {
		return err
	}
	//nolint:errcheck // read-only handle; nothing to flush on close.
	defer f.Close()

	//nolint:gosec // size is bounded by the source file itself, not attacker-controlled input.
	_, err = io.Copy(tw, f)

	return err
}

// countingWriter counts bytes written,
// so BuildTar can report Info.UncompressedSize without buffering the tar in memory.
type countingWriter struct {
	w io.Writer
	n int64
}

// Write implements io.Writer.
func (c *countingWriter) Write(p []byte) (int, error) {
	n, err := c.w.Write(p)
	c.n += int64(n)

	return n, err
}
