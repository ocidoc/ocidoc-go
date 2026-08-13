// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: Copyright 2026 WoozyMasta
// Source: github.com/ocidoc/ocidoc-go

package registry

import (
	"context"
	"fmt"

	ocispec "github.com/opencontainers/image-spec/specs-go/v1"

	"github.com/ocidoc/ocidoc-go/internal/orasrepo"
	"github.com/ocidoc/ocidoc-go/spec"
)

// resolvedReference is the common result of parsing, opening and resolving an OCI reference
//
//	Keeping these values together prevents lifecycle operations
//	from accidentally mixing a repository or selector from another reference.
type resolvedReference struct {
	// repo is opened for the parsed repository portion of requested.
	repo orasrepo.Repository

	// descriptor is the immutable manifest selected by the tag or digest.
	descriptor ocispec.Descriptor

	// requested is the caller-provided reference.
	requested string

	// repository is the registry and repository name without a selector.
	repository string
}

// resolvedSubject adds the deterministic documentation tag derived from an immutable subject manifest.
type resolvedSubject struct {
	// resolvedReference identifies the selected subject and its repository.
	resolvedReference

	// documentationTag is the direct tag associated with the subject digest.
	documentationTag string
}

// resolveReference parses reference, opens its repository,
// and resolves its mandatory tag or digest to an immutable descriptor.
func (c *Client) resolveReference(
	ctx context.Context,
	reference string,
	role string,
) (resolvedReference, error) {
	repository, selector, err := orasrepo.ParseReference(reference)
	if err != nil {
		return resolvedReference{}, wrapError(err)
	}
	if selector == "" {
		return resolvedReference{}, fmt.Errorf("%w: %s %q has no tag or digest", ErrInvalid, role, reference)
	}

	repo, err := c.open(ctx, reference)
	if err != nil {
		return resolvedReference{}, err
	}
	descriptor, err := repo.Resolve(ctx, selector)
	if err != nil {
		return resolvedReference{}, wrapError(err)
	}

	return resolvedReference{
		repo:       repo,
		descriptor: descriptor,
		requested:  reference,
		repository: repository,
	}, nil
}

// resolveSubject resolves subject and derives its deterministic documentation tag from the subject digest.
func (c *Client) resolveSubject(ctx context.Context, subject string) (resolvedSubject, error) {
	resolved, err := c.resolveReference(ctx, subject, "subject")
	if err != nil {
		return resolvedSubject{}, err
	}
	if err := resolved.descriptor.Digest.Validate(); err != nil {
		return resolvedSubject{}, fmt.Errorf("%w: invalid subject digest: %v", ErrInvalid, err)
	}

	docTag, err := spec.DocumentationTag(resolved.descriptor.Digest)
	if err != nil {
		return resolvedSubject{}, fmt.Errorf("%w: derive documentation tag: %v", ErrUnsupported, err)
	}

	return resolvedSubject{resolvedReference: resolved, documentationTag: docTag}, nil
}
