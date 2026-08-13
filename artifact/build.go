// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: Copyright 2026 WoozyMasta
// Source: github.com/ocidoc/ocidoc-go

package artifact

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"io"

	ocispec "github.com/opencontainers/image-spec/specs-go/v1"

	"github.com/ocidoc/ocidoc-go/internal/archive"
	"github.com/ocidoc/ocidoc-go/internal/atomicfile"
	"github.com/ocidoc/ocidoc-go/spec"
)

// Destination is Build's output target:
// the local path to write a packaged .ocidoc archive to.
// Build always produces a single archive file;
// BuildLayout remains directly available for a caller
// that wants an unpacked OCI Image Layout directory instead.
type Destination struct {
	// Path is the .ocidoc file to write.
	Path string

	// Overwrite allows Build to replace an existing file at Path. Default false.
	Overwrite bool
}

// Observer receives non-fatal events during Build. A nil Observer discards them.
type Observer interface {
	// Warn reports one non-fatal planning warning -
	// e.g. a declared component that matched no files while not in strict mode.
	// In strict mode the same condition is a build error instead
	// (*EmptyComponentsError), never a Warn call.
	Warn(message string)
}

// BuildOptions carries Build's inputs: a source tree, planning inputs,
// an output destination, and an optional Observer for non-fatal events.
//
// This carries planning inputs as PlanOptions
// (a config path plus caller-provided overrides) rather than a raw,
// already-loaded build config value:
// PlanOptions is what Plan and BuildLayout already consistently take,
// and reusing it here means planning (Plan) and a real build (Build)
// load and merge config through the exact same code path
// instead of requiring a caller to pre-merge a build config by hand.
type BuildOptions struct {
	// ModTime is the fixed modification time recorded in every tar entry and gzip header
	// Zero means archive.ResolveModTime()'s default (SOURCE_DATE_EPOCH, or the Unix epoch).
	ModTime time.Time

	// Observer receives non-fatal planning warnings before artifact creation.
	Observer Observer

	// Subject is the resolved, immutable descriptor of the artifact
	// this manifest attaches to, or nil for a standalone artifact.
	Subject *ocispec.Descriptor

	// Plan configures build planning and its caller-supplied overrides.
	Plan PlanOptions

	// Root is the source-tree directory to package.
	Root string

	// Output selects the archive path and overwrite behavior.
	Output Destination
}

// BuildResult is Build's outcome.
type BuildResult struct {
	// Plan is the resolved build plan used to create the artifact.
	Plan *BuildPlan

	// Manifest is the root OCI manifest written to Output.
	Manifest ocispec.Manifest

	// ConfigDescriptor describes the artifact configuration blob.
	ConfigDescriptor ocispec.Descriptor

	// ComponentDescriptors maps component types to their archive blobs.
	ComponentDescriptors map[spec.ComponentType]ocispec.Descriptor

	// Output is the .ocidoc file Build wrote, i.e. opts.Output.Path.
	Output string
}

// Build plans a build from opts.Root (see Plan),
// assembles a complete OCI Image Layout in a temporary directory (see BuildLayout),
// and packages it as a single .ocidoc archive at opts.Output.Path (see PackageArchive).
// Every plan warning is reported to opts.Observer, if set, before the layout is built.
//
// The archive is written to a temporary file beside opts.Output.Path first
// and renamed into place once complete,
// so a failed build never leaves a partial or truncated file at the destination.
func Build(ctx context.Context, opts BuildOptions) (*BuildResult, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	if opts.Output.Path == "" {
		return nil, fmt.Errorf("%w: output path is required", spec.ErrInvalid)
	}

	if !opts.Output.Overwrite {
		if _, err := os.Stat(opts.Output.Path); err == nil {
			return nil, fmt.Errorf("%w: %s already exists", spec.ErrInvalid, opts.Output.Path)
		} else if !os.IsNotExist(err) {
			return nil, fmt.Errorf("stat output path %q: %w", opts.Output.Path, err)
		}
	}

	tempRoot, err := os.MkdirTemp("", "ocidoc-build-*")
	if err != nil {
		return nil, fmt.Errorf("create temp build directory: %w", err)
	}
	//nolint:errcheck // best-effort cleanup of a temp directory.
	defer os.RemoveAll(tempRoot)

	modTime := opts.ModTime
	if modTime.IsZero() {
		modTime = archive.ResolveModTime()
	}

	plan, assembled, err := BuildLayout(ctx, opts.Root, filepath.Join(tempRoot, "layout"), BuildLayoutOptions{
		ModTime: modTime,
		Subject: opts.Subject,
		Plan:    opts.Plan,
	})
	if err != nil {
		return nil, err
	}

	if opts.Observer != nil {
		for _, w := range plan.Warnings {
			opts.Observer.Warn(w)
		}
	}

	if err := packageToDestination(ctx, tempRoot, opts.Output, modTime); err != nil {
		return nil, err
	}

	return &BuildResult{
		Plan:                 plan,
		Manifest:             assembled.Manifest,
		ConfigDescriptor:     assembled.ConfigDescriptor,
		ComponentDescriptors: assembled.ComponentDescriptors,
		Output:               opts.Output.Path,
	}, nil
}

// packageToDestination packages the OCI Image Layout at filepath.Join(tempRoot, "layout")
// and writes it to dest.Path via a temporary file in dest.Path's own directory,
// renamed into place only once the archive is fully written.
func packageToDestination(ctx context.Context, tempRoot string, dest Destination, modTime time.Time) error {
	return atomicfile.WriteFile(dest.Path, dest.Overwrite, func(w io.Writer) error {
		return PackageArchive(ctx, w, filepath.Join(tempRoot, "layout"), modTime)
	})
}
