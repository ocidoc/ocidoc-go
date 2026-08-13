// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: Copyright 2026 WoozyMasta
// Source: github.com/ocidoc/ocidoc-go

package artifact

import (
	"context"

	ocispec "github.com/opencontainers/image-spec/specs-go/v1"

	"github.com/ocidoc/ocidoc-go/spec"
)

// Inspection is the complete structural summary of one OCIDoc artifact,
// gathered from a Reader without extracting any component content.
type Inspection struct {
	// Manifest is the parsed root OCI manifest.
	Manifest ocispec.Manifest

	// Config is the parsed artifact configuration, or nil in manifest-only mode.
	Config *spec.ArtifactConfig // nil when InspectOptions.ManifestOnly is set.

	// Root identifies the root OCI manifest blob.
	Root ocispec.Descriptor

	// Components lists the manifest's component descriptors by type.
	Components []ComponentDescriptor
}

// InspectOptions controls how much Inspect reads.
type InspectOptions struct {
	// ManifestOnly skips fetching the artifact config blob:
	// the config, like each component, is a blob separate from the manifest itself.
	// Component descriptors are always included regardless of this flag:
	// they live directly in the manifest's layers, so listing them costs no extra read.
	ManifestOnly bool
}

// Inspect gathers r's root descriptor, manifest and component descriptors,
// and - unless opts.ManifestOnly is set - the parsed artifact config.
// It never opens a component's own blob content.
func Inspect(ctx context.Context, r Reader, opts InspectOptions) (*Inspection, error) {
	root, err := r.Root(ctx)
	if err != nil {
		return nil, err
	}

	manifest, err := r.Manifest(ctx)
	if err != nil {
		return nil, err
	}

	components, err := r.Components(ctx)
	if err != nil {
		return nil, err
	}

	inspection := &Inspection{Root: root, Manifest: *manifest, Components: components}

	if opts.ManifestOnly {
		return inspection, nil
	}

	cfg, err := r.Config(ctx)
	if err != nil {
		return nil, err
	}

	inspection.Config = cfg

	return inspection, nil
}
