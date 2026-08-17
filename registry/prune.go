// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: Copyright 2026 WoozyMasta
// Source: github.com/ocidoc/ocidoc-go

package registry

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/opencontainers/go-digest"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"

	"github.com/ocidoc/ocidoc-go/internal/orasrepo"
	"github.com/ocidoc/ocidoc-go/spec"
)

// PruneOptions selects one logical document slot. Prune is a dry-run unless/ Execute is true.
type PruneOptions struct {
	// Document restricts pruning to a document ID.
	Document string

	// Variant restricts pruning to a document variant.
	Variant string

	// Keep selects the manifest retained when no direct tag primary exists.
	Keep digest.Digest

	// Execute performs deletion; false returns a dry-run plan.
	Execute bool
}

// PruneResult reports the matching slot and exact manifests selected or deleted as superseded.
type PruneResult struct {
	// SubjectReference is the caller-provided subject reference.
	SubjectReference string

	// Repository contains the selected documentation manifests.
	Repository string

	// Subject is the resolved immutable subject descriptor.
	Subject ocispec.Descriptor

	// Keep is the primary manifest retained for the selected document slot.
	Keep ocispec.Descriptor

	// Matched lists every manifest matching the provided selectors.
	Matched []ocispec.Descriptor

	// Selected lists superseded manifests targeted by the prune plan.
	Selected []ocispec.Descriptor

	// Deleted lists manifests successfully deleted when Execute is true.
	Deleted []ocispec.Descriptor

	// Warnings records non-fatal discovery or deletion conditions.
	Warnings []string

	// DryRun reports whether the result is only a plan.
	DryRun bool
}

// documentSlot identifies one logical document variant by manifest annotations.
type documentSlot struct {
	// document is the document identifier.
	document string

	// variant distinguishes alternate forms of the same document.
	variant string
}

// Prune removes superseded referrer manifests for one logical document slot.
// It never infers recency and never deletes blobs directly.
func (c *Client) Prune(ctx context.Context, subject string, opts PruneOptions) (*PruneResult, error) {
	if err := validatePruneOptions(opts); err != nil {
		return nil, err
	}
	resolved, err := c.resolveSubject(ctx, subject)
	if err != nil {
		return nil, err
	}

	primary, err := prunePrimary(ctx, resolved.repo, resolved.documentationTag, resolved.descriptor)
	if err != nil {
		return nil, err
	}
	referrerBranch := discoverReferrers(ctx, resolved.repo, resolved.descriptor, DiscoverOptions{})
	if referrerBranch.err != nil {
		return nil, referrerBranch.err
	}

	keep, matched, selected, err := planPrune(primary, referrerBranch.candidates, opts)
	if err != nil {
		return nil, err
	}
	result := &PruneResult{
		SubjectReference: subject,
		Repository:       resolved.repository,
		Subject:          resolved.descriptor,
		Keep:             keep.descriptor,
		Matched:          candidateDescriptors(matched),
		DryRun:           !opts.Execute,
	}
	if len(matched) == 0 {
		return result, nil
	}

	result.Selected = candidateDescriptors(selected)
	result.Warnings = []string{
		"prune deletes manifests, not config or component blobs",
		"registry garbage collection controls eventual storage reclamation",
	}
	if !opts.Execute {
		return result, nil
	}
	if err := executePrune(ctx, resolved.repo, result, selected); err != nil {
		return result, err
	}

	return result, nil
}

// validatePruneOptions validates an optional explicit keep digest.
func validatePruneOptions(opts PruneOptions) error {
	if opts.Keep == "" {
		return nil
	}
	if err := opts.Keep.Validate(); err != nil {
		return fmt.Errorf("%w: invalid keep digest: %v", ErrInvalid, err)
	}

	return nil
}

// planPrune selects the retained manifest and every other manifest in its logical document slot.
func planPrune(
	primary *discoveryCandidate,
	candidates []discoveryCandidate,
	opts PruneOptions,
) (discoveryCandidate, []discoveryCandidate, []discoveryCandidate, error) {
	matchingSelectors := filterPruneSelectors(candidates, opts)
	keep, slot, err := selectPruneKeep(primary, matchingSelectors, opts)
	if err != nil {
		return discoveryCandidate{}, nil, nil, err
	}
	matched := filterSlot(matchingSelectors, slot)
	selected := make([]discoveryCandidate, 0, len(matched))
	for _, candidate := range matched {
		if candidate.descriptor.Digest != keep.descriptor.Digest {
			selected = append(selected, candidate)
		}
	}

	return keep, matched, selected, nil
}

// executePrune deletes selected manifests in deterministic order
// and retains successful deletions in result when a later deletion fails.
func executePrune(
	ctx context.Context,
	repo orasrepo.Repository,
	result *PruneResult,
	selected []discoveryCandidate,
) error {
	for _, candidate := range selected {
		if err := repo.Delete(ctx, candidate.descriptor); err != nil {
			return fmt.Errorf("delete %s after %d successful deletions: %w",
				candidate.descriptor.Digest, len(result.Deleted), wrapError(err))
		}
		result.Deleted = append(result.Deleted, candidate.descriptor)
	}

	return nil
}

// prunePrimary resolves the direct documentation tag when it contains a valid attachment for subject.
// A missing tag has no primary.
func prunePrimary(
	ctx context.Context,
	repo orasrepo.Repository,
	docTag string,
	subject ocispec.Descriptor,
) (*discoveryCandidate, error) {
	branch := discoverTag(ctx, repo, docTag, subject, DiscoverOptions{})
	if branch.err != nil {
		if errors.Is(branch.err, ErrNotFound) {
			return nil, nil
		}
		return nil, branch.err
	}

	if len(branch.candidates) == 0 {
		return nil, nil
	}

	return &branch.candidates[0], nil
}

// selectPruneKeep prefers a matching direct-tag primary.
// Without one, callers must select a matching referrer explicitly through opts.Keep.
func selectPruneKeep(
	primary *discoveryCandidate,
	candidates []discoveryCandidate,
	opts PruneOptions,
) (discoveryCandidate, documentSlot, error) {
	if primary != nil && matchesPruneSelectors(*primary, opts) {
		if opts.Keep != "" && opts.Keep != primary.descriptor.Digest {
			return discoveryCandidate{}, documentSlot{},
				fmt.Errorf("%w: keep digest differs from the direct .doc primary", ErrConflict)
		}
		return *primary, candidateSlot(*primary), nil
	}

	if len(candidates) == 0 {
		if opts.Keep != "" {
			return discoveryCandidate{}, documentSlot{},
				fmt.Errorf("%w: keep digest %s is not a matching referrer", ErrNotFound, opts.Keep)
		}
		return discoveryCandidate{}, documentSlot{}, nil
	}
	if opts.Keep == "" {
		return discoveryCandidate{}, documentSlot{},
			fmt.Errorf("%w: no direct .doc primary matches; keep digest is required", ErrConflict)
	}

	for _, candidate := range candidates {
		if candidate.descriptor.Digest == opts.Keep {
			return candidate, candidateSlot(candidate), nil
		}
	}

	return discoveryCandidate{}, documentSlot{},
		fmt.Errorf("%w: keep digest %s is not a matching referrer", ErrNotFound, opts.Keep)
}

// filterPruneSelectors keeps candidates matching every non-empty selector.
func filterPruneSelectors(candidates []discoveryCandidate, opts PruneOptions) []discoveryCandidate {
	filtered := make([]discoveryCandidate, 0, len(candidates))
	for _, candidate := range candidates {
		if matchesPruneSelectors(candidate, opts) {
			filtered = append(filtered, candidate)
		}
	}
	return filtered
}

// matchesPruneSelectors reports whether candidate belongs to opts' document selector.
func matchesPruneSelectors(candidate discoveryCandidate, opts PruneOptions) bool {
	slot := candidateSlot(candidate)
	return (opts.Document == "" || slot.document == opts.Document) &&
		(opts.Variant == "" || slot.variant == opts.Variant)
}

// candidateSlot derives a document slot from candidate manifest annotations.
func candidateSlot(candidate discoveryCandidate) documentSlot {
	annotations := candidate.manifest.Annotations
	return documentSlot{
		document: annotations[spec.AnnotationDocumentID],
		variant:  annotations[spec.AnnotationDocumentVariant],
	}
}

// filterSlot returns candidates in slot, sorted by digest for stable pruning.
func filterSlot(candidates []discoveryCandidate, slot documentSlot) []discoveryCandidate {
	matched := make([]discoveryCandidate, 0, len(candidates))
	for _, candidate := range candidates {
		if candidateSlot(candidate) == slot {
			matched = append(matched, candidate)
		}
	}

	slices.SortFunc(matched, func(a, b discoveryCandidate) int {
		return strings.Compare(a.descriptor.Digest.String(), b.descriptor.Digest.String())
	})

	return matched
}
