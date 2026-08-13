// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: Copyright 2026 WoozyMasta
// Source: github.com/ocidoc/ocidoc-go

package registry

import (
	"context"
	"encoding/json"
	"fmt"

	ocispec "github.com/opencontainers/image-spec/specs-go/v1"

	"github.com/ocidoc/ocidoc-go/spec"
)

// DetachOptions controls attached selection and deletion.
type DetachOptions struct {
	// Discover selects the attached manifest to detach.
	Discover DiscoverOptions

	// DryRun reports the intended deletion without mutating the registry.
	DryRun bool
}

// RemoveOptions controls exact-reference deletion.
type RemoveOptions struct {
	// DryRun reports the intended deletion without mutating the registry.
	DryRun bool
}

// DeleteResult reports the immutable manifest selected for deletion.
type DeleteResult struct {
	// Manifest identifies the selected OCIDoc manifest.
	Manifest ocispec.Descriptor

	// Subject is the manifest subject when the removed artifact was attached.
	Subject *ocispec.Descriptor

	// RequestedReference is the user-supplied reference.
	RequestedReference string

	// Reference is the immutable manifest reference selected for deletion.
	Reference string

	// Repository is the registry repository containing Manifest.
	Repository string

	// Warnings records non-fatal lifecycle conditions.
	Warnings []string

	// DryRun reports whether no deletion was attempted.
	DryRun bool

	// Deleted reports whether the registry accepted manifest deletion.
	Deleted bool
}

// Detach discovers one manifest attached to subject and deletes only that root manifest.
// ORAS updates a fallback referrers index when required;
// blobs remain for registry garbage collection.
func (c *Client) Detach(ctx context.Context, subject string, opts DetachOptions) (*DeleteResult, error) {
	discovered, err := c.Discover(ctx, subject, opts.Discover)
	if err != nil {
		return nil, err
	}

	result, err := c.Remove(ctx, discovered.Reference, RemoveOptions{DryRun: opts.DryRun})
	if err != nil {
		return nil, err
	}

	result.RequestedReference = subject
	result.Warnings = append(discovered.Warnings, result.Warnings...)
	return result, nil
}

// Remove resolves reference, verifies that it is an OCIDoc manifest,
// and deletes the manifest by immutable digest.
// It never deletes config or layer blobs directly.
// Digest deletion may invalidate every tag targeting it.
func (c *Client) Remove(ctx context.Context, reference string, opts RemoveOptions) (*DeleteResult, error) {
	resolved, err := c.resolveReference(ctx, reference, "reference")
	if err != nil {
		return nil, err
	}

	manifestData, err := fetchBlob(ctx, resolved.repo, resolved.descriptor)
	if err != nil {
		return nil, fmt.Errorf("manifest: %w", err)
	}
	var manifest ocispec.Manifest
	if err := json.Unmarshal(manifestData, &manifest); err != nil {
		return nil, fmt.Errorf("%w: parse manifest: %v", ErrInvalid, err) //nolint:errorlint // ErrInvalid classifies malformed remote content.
	}
	if manifest.ArtifactType != spec.ArtifactType {
		return nil, fmt.Errorf("%w: manifest artifactType %q is not an OCIDoc artifact", ErrInvalid, manifest.ArtifactType)
	}

	result := &DeleteResult{
		Manifest:           resolved.descriptor,
		Subject:            manifest.Subject,
		RequestedReference: reference,
		Reference:          resolved.repository + "@" + resolved.descriptor.Digest.String(),
		Repository:         resolved.repository,
		Warnings: []string{
			"digest deletion may remove every tag pointing to this manifest",
			"config and component blobs remain subject to registry garbage collection",
		},
		DryRun: opts.DryRun,
	}
	if opts.DryRun {
		return result, nil
	}
	if err := resolved.repo.Delete(ctx, resolved.descriptor); err != nil {
		return nil, wrapError(err)
	}

	result.Deleted = true
	return result, nil
}
