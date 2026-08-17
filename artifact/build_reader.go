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

	ocispec "github.com/opencontainers/image-spec/specs-go/v1"

	"github.com/ocidoc/ocidoc-go/spec"
)

// BuildReaderOptions carries inputs for building an in-memory graph view backed
// by a temporary OCI Image Layout.
type BuildReaderOptions struct {
	// ModTime is recorded in component archives. Zero uses the reproducible default.
	ModTime time.Time

	// Observer receives non-fatal planning warnings.
	Observer Observer

	// Subject is the immutable descriptor of the artifact this manifest attaches to.
	Subject *ocispec.Descriptor

	// Plan configures build planning and caller-supplied overrides.
	Plan PlanOptions

	// Root is the source-tree directory to package.
	Root string
}

// BuildReaderResult is the result of BuildReader and its temporary graph reader.
type BuildReaderResult struct {
	// Plan is the resolved build plan.
	Plan *BuildPlan

	// Reader provides access to the built OCI graph.
	Reader Reader

	// Manifest is the built root manifest.
	Manifest ocispec.Manifest

	// ConfigDescriptor describes the artifact configuration blob.
	ConfigDescriptor ocispec.Descriptor

	// ComponentDescriptors maps component types to their archive blobs.
	ComponentDescriptors map[spec.ComponentType]ocispec.Descriptor
}

// temporaryReader removes the temporary layout when the graph reader closes.
type temporaryReader struct {
	// Reader is the layout-backed graph reader.
	Reader

	// root is the temporary build directory to remove on close.
	root string
}

// BuildReader builds an OCIDoc graph and returns a Reader for it.
// The graph is backed by a temporary OCI Image Layout owned by the returned Reader;
// callers must close result.Reader when finished.
func BuildReader(ctx context.Context, opts BuildReaderOptions) (*BuildReaderResult, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	tempRoot, err := os.MkdirTemp("", "ocidoc-build-reader-*")
	if err != nil {
		return nil, fmt.Errorf("create temporary build directory: %w", err)
	}

	layoutDir := filepath.Join(tempRoot, "layout")
	plan, assembled, err := BuildLayout(ctx, opts.Root, layoutDir, BuildLayoutOptions{
		ModTime: opts.ModTime,
		Subject: opts.Subject,
		Plan:    opts.Plan,
	})
	if err != nil {
		_ = os.RemoveAll(tempRoot)
		return nil, err
	}

	if opts.Observer != nil {
		for _, warning := range plan.Warnings {
			opts.Observer.Warn(warning)
		}
	}

	reader, err := OpenLayout(layoutDir)
	if err != nil {
		_ = os.RemoveAll(tempRoot)
		return nil, err
	}

	return &BuildReaderResult{
		Plan:                 plan,
		Reader:               &temporaryReader{Reader: reader, root: tempRoot},
		Manifest:             assembled.Manifest,
		ConfigDescriptor:     assembled.ConfigDescriptor,
		ComponentDescriptors: assembled.ComponentDescriptors,
	}, nil
}

// Close implements Reader and removes the temporary build directory.
func (r *temporaryReader) Close() error {
	if err := r.Reader.Close(); err != nil {
		return err
	}

	if err := os.RemoveAll(r.root); err != nil {
		return fmt.Errorf("remove temporary build directory: %w", err)
	}

	return nil
}
