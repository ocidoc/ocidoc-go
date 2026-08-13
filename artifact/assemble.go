// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: Copyright 2026 WoozyMasta
// Source: github.com/ocidoc/ocidoc-go

package artifact

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"time"

	ocispec "github.com/opencontainers/image-spec/specs-go/v1"

	"github.com/ocidoc/ocidoc-go/internal/digestio"
	"github.com/ocidoc/ocidoc-go/spec"
)

// AssembleOptions ties a BuildPlan (see Plan) to the output writers for its blobs.
// Assemble does not decide where those blobs ultimately live
// (a temp file, an OCI Image Layout's blobs/sha256/ directory, an in-memory buffer):
// the caller owns that decision and supplies one writer per non-empty component in Plan.Ownership,
// plus one for the artifact config blob.
type AssembleOptions struct {
	// ModTime is recorded in every component archive.
	// Zero uses the reproducible default from archive.ResolveModTime.
	ModTime time.Time

	// ConfigBlob receives the serialized artifact configuration.
	ConfigBlob io.Writer

	// Plan supplies the resolved files, entrypoints and effective settings.
	Plan *BuildPlan

	// ComponentBlobs maps each planned component to its destination writer.
	ComponentBlobs map[spec.ComponentType]io.Writer

	// Subject is the resolved, immutable descriptor of the artifact this manifest attaches to,
	// or nil for a standalone artifact - subject is omitted from the manifest entirely
	// for standalone artifacts, not just left empty.
	Subject *ocispec.Descriptor

	// Root is the source-tree directory containing the planned files.
	Root string
}

// AssembleResult is the complete, in-memory description of one OCIDoc artifact:
// everything needed to write an OCI manifest and its referenced blobs,
// once the blob bytes (already streamed to AssembleOptions' writers)
// are in their final storage location.
type AssembleResult struct {
	// ComponentDescriptors describes each component blob written by Assemble.
	ComponentDescriptors map[spec.ComponentType]ocispec.Descriptor

	// Manifest references the configuration and component descriptors.
	Manifest ocispec.Manifest

	// ConfigDescriptor describes the serialized artifact configuration blob.
	ConfigDescriptor ocispec.Descriptor
}

// Assemble builds every component blob and the artifact config blob from opts.Plan,
// streaming each to its corresponding writer in opts.ComponentBlobs/opts.ConfigBlob,
// and returns the resulting root manifest and descriptors.
// It performs no registry requests.
//
// ctx is checked once per component,
// on top of the finer per-file check each component's own tar build already makes,
// so a caller that cancels it stops a large multi-component build promptly.
func Assemble(ctx context.Context, opts AssembleOptions) (*AssembleResult, error) {
	if opts.Plan == nil {
		return nil, fmt.Errorf("%w: plan is required", spec.ErrInvalid)
	}

	componentDescriptors := make(map[spec.ComponentType]ocispec.Descriptor, len(opts.Plan.Ownership))

	for name, paths := range opts.Plan.Ownership {
		if err := ctx.Err(); err != nil {
			return nil, err
		}

		w, ok := opts.ComponentBlobs[name]
		if !ok {
			return nil, fmt.Errorf("%w: missing output writer for component %q", spec.ErrInvalid, name)
		}

		desc, err := buildComponentBlob(
			ctx, w, opts.Root, name, paths, opts.Plan.Entrypoints[name],
			opts.Plan.Settings.Compression.Type, opts.Plan.Settings.Compression.Level, opts.ModTime,
		)
		if err != nil {
			return nil, err
		}

		componentDescriptors[name] = desc
	}

	configDescriptor, err := writeArtifactConfigBlob(opts.ConfigBlob, opts.Plan)
	if err != nil {
		return nil, err
	}

	manifest := buildManifest(opts.Plan, configDescriptor, componentDescriptors, opts.Subject)

	return &AssembleResult{
		Manifest:             manifest,
		ConfigDescriptor:     configDescriptor,
		ComponentDescriptors: componentDescriptors,
	}, nil
}

// writeArtifactConfigBlob marshals plan's artifact config as JSON,
// writes it to dst, and returns its descriptor.
func writeArtifactConfigBlob(dst io.Writer, plan *BuildPlan) (ocispec.Descriptor, error) {
	data, err := json.Marshal(buildArtifactConfig(plan))
	if err != nil {
		return ocispec.Descriptor{}, fmt.Errorf("marshal artifact config: %w", err)
	}

	counted := digestio.NewWriter(dst)
	if _, err := counted.Write(data); err != nil {
		return ocispec.Descriptor{}, fmt.Errorf("write artifact config blob: %w", err)
	}

	return ocispec.Descriptor{
		MediaType: spec.ConfigMediaType,
		Digest:    counted.Digest(),
		Size:      counted.Size(),
	}, nil
}
