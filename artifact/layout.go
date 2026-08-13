// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: Copyright 2026 WoozyMasta
// Source: github.com/ocidoc/ocidoc-go

package artifact

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/opencontainers/go-digest"
	specs "github.com/opencontainers/image-spec/specs-go"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"

	"github.com/ocidoc/ocidoc-go/internal/atomicfile"
	"github.com/ocidoc/ocidoc-go/spec"
)

// blobsSubdir is the sha256 blob directory's path relative
// to an OCI Image Layout root, per the OCI Image Layout spec.
const blobsSubdir = "blobs/sha256"

// BuildLayoutOptions carries BuildLayout's inputs:
// everything Plan needs, plus assembly-time values that are not build-config-derived.
type BuildLayoutOptions struct {
	// ModTime is recorded in component archives. Zero uses the reproducible default.
	ModTime time.Time

	// Subject is the resolved, immutable descriptor
	// of the artifact this manifest attaches to, or nil for a standalone artifact.
	Subject *ocispec.Descriptor

	// Plan configures planning and caller-supplied build overrides.
	Plan PlanOptions
}

// BuildLayout plans a build from root (see Plan),
// builds every component blob and the artifact config blob,
// and writes a complete OCI Image Layout to layoutDir:
// "oci-layout", "index.json", and blobs/sha256/<digest> for every component,
// the config and the root manifest.
// layoutDir is created if it does not already exist.
//
// Each blob is written to a temporary file inside layoutDir's blob directory first
// and renamed to its digest-derived name once known,
// so no blob is ever buffered whole in memory,
// and a failed build never leaves a partially-named blob behind.
//
// BuildLayout returns the *BuildPlan it computed alongside the assembly result,
// so a caller that also wants planning details
// (e.g. Build, which needs BuildPlan.Warnings and BuildPlan.Document)
// does not have to call Plan a second time.
func BuildLayout(ctx context.Context, root string, layoutDir string, opts BuildLayoutOptions) (*BuildPlan, *AssembleResult, error) {
	plan, err := Plan(ctx, root, opts.Plan)
	if err != nil {
		return nil, nil, err
	}

	blobsDir := filepath.Join(layoutDir, filepath.FromSlash(blobsSubdir))
	if err := os.MkdirAll(blobsDir, 0o750); err != nil {
		return nil, nil, fmt.Errorf("create blobs directory: %w", err)
	}

	var tempFiles []*atomicfile.Temp

	defer func() { cleanupTempBlobs(tempFiles) }()

	componentFiles := make(map[spec.ComponentType]*atomicfile.Temp, len(plan.Ownership))
	componentWriters := make(map[spec.ComponentType]io.Writer, len(plan.Ownership))

	for name := range plan.Ownership {
		f, err := atomicfile.CreateTemp(blobsDir)
		if err != nil {
			return nil, nil, fmt.Errorf("create temp blob for component %q: %w", name, err)
		}

		tempFiles = append(tempFiles, f)
		componentFiles[name] = f
		componentWriters[name] = f.File()
	}

	configFile, err := atomicfile.CreateTemp(blobsDir)
	if err != nil {
		return nil, nil, fmt.Errorf("create temp blob for artifact config: %w", err)
	}

	tempFiles = append(tempFiles, configFile)

	result, err := Assemble(ctx, AssembleOptions{
		Plan:           plan,
		Root:           root,
		ComponentBlobs: componentWriters,
		ConfigBlob:     configFile.File(),
		ModTime:        opts.ModTime,
		Subject:        opts.Subject,
	})
	if err != nil {
		return nil, nil, err
	}

	for name, f := range componentFiles {
		if err := f.Rename(filepath.Join(blobsDir, result.ComponentDescriptors[name].Digest.Encoded())); err != nil {
			return nil, nil, fmt.Errorf("finalize blob for component %q: %w", name, err)
		}
	}

	if err := configFile.Rename(filepath.Join(blobsDir, result.ConfigDescriptor.Digest.Encoded())); err != nil {
		return nil, nil, fmt.Errorf("finalize config blob: %w", err)
	}

	manifestDescriptor, err := writeManifestBlob(blobsDir, result.Manifest)
	if err != nil {
		return nil, nil, err
	}

	if err := writeLayoutMetadata(layoutDir, manifestDescriptor); err != nil {
		return nil, nil, err
	}

	return plan, result, nil
}

// cleanupTempBlobs removes any temp blob that was not already renamed into place.
// Called via defer, so it is a no-op along the success path and best-effort on any error path.
func cleanupTempBlobs(files []*atomicfile.Temp) {
	for _, f := range files {
		f.Cleanup()
	}
}

// writeManifestBlob marshals manifest as JSON,
// writes it to blobsDir/<digest>, and returns its descriptor.
func writeManifestBlob(blobsDir string, manifest ocispec.Manifest) (ocispec.Descriptor, error) {
	data, err := json.Marshal(manifest)
	if err != nil {
		return ocispec.Descriptor{}, fmt.Errorf("marshal manifest: %w", err)
	}

	manifestDigest := digest.Canonical.FromBytes(data)

	if err := os.WriteFile(filepath.Join(blobsDir, manifestDigest.Encoded()), data, 0o600); err != nil {
		return ocispec.Descriptor{}, fmt.Errorf("write manifest blob: %w", err)
	}

	return ocispec.Descriptor{
		MediaType:    ocispec.MediaTypeImageManifest,
		ArtifactType: spec.ArtifactType,
		Digest:       manifestDigest,
		Size:         int64(len(data)),
	}, nil
}

// writeLayoutMetadata writes the two small top-level OCI Image Layout files:
// "oci-layout" and "index.json" (referencing manifestDescriptor as the layout's sole entry).
func writeLayoutMetadata(layoutDir string, manifestDescriptor ocispec.Descriptor) error {
	layoutBytes, err := json.Marshal(ocispec.ImageLayout{Version: ocispec.ImageLayoutVersion})
	if err != nil {
		return fmt.Errorf("marshal %s: %w", ocispec.ImageLayoutFile, err)
	}

	if err := os.WriteFile(filepath.Join(layoutDir, ocispec.ImageLayoutFile), layoutBytes, 0o600); err != nil {
		return fmt.Errorf("write %s: %w", ocispec.ImageLayoutFile, err)
	}

	index := ocispec.Index{
		Versioned: specs.Versioned{SchemaVersion: 2},
		MediaType: ocispec.MediaTypeImageIndex,
		Manifests: []ocispec.Descriptor{manifestDescriptor},
	}

	indexBytes, err := json.Marshal(index)
	if err != nil {
		return fmt.Errorf("marshal %s: %w", ocispec.ImageIndexFile, err)
	}

	if err := os.WriteFile(filepath.Join(layoutDir, ocispec.ImageIndexFile), indexBytes, 0o600); err != nil {
		return fmt.Errorf("write %s: %w", ocispec.ImageIndexFile, err)
	}

	return nil
}
