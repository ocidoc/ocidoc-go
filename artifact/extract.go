// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: Copyright 2026 WoozyMasta
// Source: github.com/ocidoc/ocidoc-go

package artifact

import (
	"context"
	"fmt"

	"github.com/ocidoc/ocidoc-go/internal/archive"
	"github.com/ocidoc/ocidoc-go/internal/compression"
	"github.com/ocidoc/ocidoc-go/spec"
)

// ExtractOptions controls Extract's destination, scope and limits.
type ExtractOptions struct {
	// Output is the destination directory.
	// It is created if it does not already exist.
	Output string

	// Component restricts extraction to one component.
	// Empty extracts every component into the global virtual tree.
	Component spec.ComponentType

	// Overwrite allows extraction to replace an existing file.
	// Default false.
	Overwrite bool

	// MaxFiles limits files extracted from each component.
	// Zero uses the safe default from internal/archive.
	MaxFiles int

	// MaxTotalSize limits total extracted bytes from each component.
	// Zero uses the safe default from internal/archive.
	MaxTotalSize int64

	// MaxFileSize limits bytes extracted for one file.
	// Zero uses the safe default from internal/archive.
	MaxFileSize int64
}

// Extract writes every requested component's files under opts.Output,
// forming one merged "global virtual tree" when more than one component is extracted
// (no opts.Component means every component).
//
// Before writing anything, Extract performs a bounded scan of every requested
// component, lists its files and validates the combined set has no path collisions (spec.ValidateBundlePaths) -
// catching a malformed or malicious artifact's overlapping paths
// as one clear error before any file is written, rather than an O_EXCL failure partway through extraction.
// This means each requested component is read twice (once to list, once to extract);
// acceptable for local, documentation-scale artifacts,
// but worth reconsidering if Extract is later used against a remote Reader
// where a second full fetch is not free.
func Extract(ctx context.Context, r Reader, opts ExtractOptions) error {
	components, err := r.Components(ctx)
	if err != nil {
		return err
	}

	if opts.Component != "" {
		components = filterComponents(components, opts.Component)
		if len(components) == 0 {
			return fmt.Errorf("%w: component %q", spec.ErrNotFound, opts.Component)
		}
	}

	archiveOpts := archive.ExtractOptions{
		MaxFiles:     opts.MaxFiles,
		MaxTotalSize: opts.MaxTotalSize,
		MaxFileSize:  opts.MaxFileSize,
		Overwrite:    opts.Overwrite,
	}

	if err := checkNoCollisions(ctx, r, components, archiveOpts); err != nil {
		return err
	}

	for _, c := range components {
		if err := extractComponent(ctx, r, c, opts.Output, archiveOpts); err != nil {
			return fmt.Errorf("component %q: %w", c.Type, err)
		}
	}

	return nil
}

// checkNoCollisions lists every file in components
// and validates the combined path set has no exact or case-insensitive duplicates.
func checkNoCollisions(ctx context.Context, r Reader, components []ComponentDescriptor, opts archive.ExtractOptions) error {
	var allPaths []string

	for _, c := range components {
		files, err := listComponentFiles(ctx, r, c, opts)
		if err != nil {
			return err
		}

		for _, f := range files {
			allPaths = append(allPaths, f.Path)
		}
	}

	return spec.ValidateBundlePaths(allPaths)
}

// extractComponent opens, decompresses and safely extracts one component's files under destDir.
func extractComponent(ctx context.Context, r Reader, c ComponentDescriptor, destDir string, opts archive.ExtractOptions) error {
	rc, _, err := r.OpenComponent(ctx, c.Type)
	if err != nil {
		return err
	}
	//nolint:errcheck // read-only handle; a close error here would not change an already-successful extraction.
	defer rc.Close()

	decompressed, err := compression.NewReader(rc, c.Descriptor.MediaType)
	if err != nil {
		return err
	}
	//nolint:errcheck // read-only handle; a close error here would not change an already-successful extraction.
	defer decompressed.Close()

	if _, err := archive.Extract(decompressed, destDir, opts); err != nil {
		return err
	}

	return nil
}
