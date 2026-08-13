// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: Copyright 2026 WoozyMasta
// Source: github.com/ocidoc/ocidoc-go

package registry

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/opencontainers/go-digest"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"

	"github.com/ocidoc/ocidoc-go/internal/orasrepo"
	"github.com/ocidoc/ocidoc-go/spec"
)

// DiscoveryMode selects the attached-artifact discovery mechanism.
type DiscoveryMode string

const (
	// DiscoveryAuto queries the direct tag and referrers concurrently.
	DiscoveryAuto DiscoveryMode = "auto"

	// DiscoveryTag queries only the direct .doc tag.
	DiscoveryTag DiscoveryMode = "tag"

	// DiscoveryReferrer queries only OCI referrers.
	DiscoveryReferrer DiscoveryMode = "referrer"
)

// DiscoverOptions controls candidate selection and reconciliation.
type DiscoverOptions struct {
	// Mode selects the discovery mechanism.
	Mode DiscoveryMode

	// Document restricts candidates by document ID.
	Document string

	// Language restricts candidates by document language.
	Language string

	// Variant restricts candidates by document variant.
	Variant string

	// Digest selects one exact candidate manifest digest.
	Digest digest.Digest

	// Strict rejects inconsistent or unavailable discovery mechanisms.
	Strict bool
}

// DiscoveryResult reports the selected immutable attached manifest.
type DiscoveryResult struct {
	// Subject is the resolved immutable subject descriptor.
	Subject ocispec.Descriptor

	// Manifest identifies the selected attached manifest.
	Manifest ocispec.Descriptor

	// SubjectReference is the caller-provided subject reference.
	SubjectReference string

	// Reference is the selected manifest's immutable registry reference.
	Reference string

	// Mode is the mechanism used for the selected result.
	Mode DiscoveryMode

	// Candidates lists matching attached manifest descriptors.
	Candidates []ocispec.Descriptor

	// Warnings records non-fatal reconciliation conditions.
	Warnings []string

	// TagAvailable reports whether direct-tag discovery succeeded.
	TagAvailable bool

	// ReferrerAvailable reports whether OCI-referrer discovery succeeded.
	ReferrerAvailable bool

	// Conflict reports disagreement between available discovery mechanisms.
	Conflict bool
}

// discoveryCandidate combines a registry descriptor
// with the decoded manifest required for selector and subject checks.
type discoveryCandidate struct {
	// manifest supplies annotations and subject metadata for filtering.
	manifest ocispec.Manifest

	// descriptor identifies the candidate in the registry.
	descriptor ocispec.Descriptor
}

// discoveryBranch holds one direct-tag or referrer lookup result.
type discoveryBranch struct {
	// err is the branch failure, if the lookup could not complete.
	err error

	// candidates are validated artifacts matching the requested selectors.
	candidates []discoveryCandidate
}

// discoverySelection is the reconciled result of one or both discovery paths.
type discoverySelection struct {
	// selected is the unambiguous artifact chosen for the caller.
	selected discoveryCandidate

	// candidates are all matching artifacts exposed in DiscoveryResult.
	candidates []discoveryCandidate

	// warnings record accepted non-strict inconsistencies or fallbacks.
	warnings []string

	// tagAvailable reports that direct-tag lookup contributed a result.
	tagAvailable bool

	// referrerAvailable reports that referrer lookup contributed a result.
	referrerAvailable bool

	// conflict reports a non-strict disagreement between the lookup paths.
	conflict bool
}

// Discover resolves subject and selects one attached OCIDoc artifact using direct-tag,
// referrer or deterministic parallel discovery.
func (c *Client) Discover(ctx context.Context, subject string, opts DiscoverOptions) (*DiscoveryResult, error) {
	opts, err := normalizeDiscoverOptions(opts)
	if err != nil {
		return nil, err
	}

	resolved, err := c.resolveSubject(ctx, subject)
	if err != nil {
		return nil, err
	}

	tagBranch, referrerBranch := runDiscovery(ctx, resolved, opts)

	selection, err := reconcileDiscovery(opts, tagBranch, referrerBranch)
	if err != nil {
		return nil, err
	}

	return &DiscoveryResult{
		SubjectReference:  subject,
		Reference:         resolved.repository + "@" + selection.selected.descriptor.Digest.String(),
		Subject:           resolved.descriptor,
		Manifest:          selection.selected.descriptor,
		Candidates:        candidateDescriptors(selection.candidates),
		Warnings:          selection.warnings,
		Mode:              opts.Mode,
		TagAvailable:      selection.tagAvailable,
		ReferrerAvailable: selection.referrerAvailable,
		Conflict:          selection.conflict,
	}, nil
}

// normalizeDiscoverOptions applies the default mode and validates selectors.
func normalizeDiscoverOptions(opts DiscoverOptions) (DiscoverOptions, error) {
	if opts.Mode == "" {
		opts.Mode = DiscoveryAuto
	}
	if opts.Mode != DiscoveryAuto && opts.Mode != DiscoveryTag && opts.Mode != DiscoveryReferrer {
		return DiscoverOptions{}, fmt.Errorf("%w: invalid discovery mode %q", ErrInvalid, opts.Mode)
	}
	if opts.Digest != "" {
		if err := opts.Digest.Validate(); err != nil {
			return DiscoverOptions{}, fmt.Errorf("%w: invalid selector digest: %v", ErrInvalid, err)
		}
	}

	return opts, nil
}

// runDiscovery performs the lookup paths selected by opts.
// Auto mode runs independent tag and referrer requests concurrently.
func runDiscovery(
	ctx context.Context,
	subject resolvedSubject,
	opts DiscoverOptions,
) (discoveryBranch, discoveryBranch) {
	switch opts.Mode {
	case DiscoveryTag:
		return discoverTag(ctx, subject.repo, subject.documentationTag, subject.descriptor, opts), discoveryBranch{}

	case DiscoveryReferrer:
		return discoveryBranch{}, discoverReferrers(ctx, subject.repo, subject.descriptor, opts)

	default:
		tagResult := make(chan discoveryBranch, 1)
		referrerResult := make(chan discoveryBranch, 1)
		go func() {
			tagResult <- discoverTag(ctx, subject.repo, subject.documentationTag, subject.descriptor, opts)
		}()
		go func() {
			referrerResult <- discoverReferrers(ctx, subject.repo, subject.descriptor, opts)
		}()

		return <-tagResult, <-referrerResult
	}
}

// discoverTag resolves the direct documentation tag
// and validates its target as a matching attached artifact.
func discoverTag(
	ctx context.Context,
	repo orasrepo.Repository,
	tag string,
	subject ocispec.Descriptor,
	opts DiscoverOptions,
) discoveryBranch {
	desc, err := repo.Resolve(ctx, tag)
	if err != nil {
		return discoveryBranch{err: wrapError(err)}
	}

	candidate, applicable, err := loadDiscoveryCandidate(ctx, repo, desc, subject)
	if err != nil {
		return discoveryBranch{err: err}
	}
	if !applicable {
		return discoveryBranch{err: fmt.Errorf("%w: direct .doc tag does not point to an OCIDoc artifact", ErrInvalid)}
	}
	if !matchesSelectors(candidate, opts) {
		return discoveryBranch{}
	}

	return discoveryBranch{candidates: []discoveryCandidate{candidate}}
}

// discoverReferrers loads and validates matching OCIDoc referrers in digest
// order so ambiguous results remain deterministic.
func discoverReferrers(
	ctx context.Context,
	repo orasrepo.Repository,
	subject ocispec.Descriptor,
	opts DiscoverOptions,
) discoveryBranch {
	descriptors, err := repo.Referrers(ctx, subject, spec.ArtifactType)
	if err != nil {
		return discoveryBranch{err: wrapError(err)}
	}

	candidates := make([]discoveryCandidate, 0, len(descriptors))
	for _, desc := range descriptors {
		if desc.ArtifactType != "" && desc.ArtifactType != spec.ArtifactType {
			continue
		}

		candidate, applicable, err := loadDiscoveryCandidate(ctx, repo, desc, subject)
		if err != nil {
			return discoveryBranch{err: err}
		}
		if applicable && matchesSelectors(candidate, opts) {
			candidates = append(candidates, candidate)
		}
	}

	slices.SortFunc(candidates, func(a, b discoveryCandidate) int {
		return strings.Compare(a.descriptor.Digest.String(), b.descriptor.Digest.String())
	})

	return discoveryBranch{candidates: candidates}
}

// loadDiscoveryCandidate fetches desc
// and accepts it only when its manifest is an OCIDoc attachment for subject.
func loadDiscoveryCandidate(
	ctx context.Context,
	repo orasrepo.Repository,
	desc, subject ocispec.Descriptor,
) (discoveryCandidate, bool, error) {
	data, err := fetchBlob(ctx, repo, desc)
	if err != nil {
		return discoveryCandidate{}, false, fmt.Errorf("candidate %s: %w", desc.Digest, err)
	}

	var manifest ocispec.Manifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		return discoveryCandidate{}, false, fmt.Errorf("%w: parse candidate %s: %v", ErrInvalid, desc.Digest, err) //nolint:errorlint // ErrInvalid classifies remote content.
	}
	if manifest.ArtifactType != spec.ArtifactType {
		return discoveryCandidate{}, false, nil
	}
	if manifest.Subject == nil || !sameSubject(*manifest.Subject, subject) {
		return discoveryCandidate{}, false, fmt.Errorf("%w: candidate %s subject does not match %s", ErrInvalid, desc.Digest, subject.Digest)
	}

	desc.ArtifactType = manifest.ArtifactType
	desc.Annotations = manifest.Annotations

	return discoveryCandidate{descriptor: desc, manifest: manifest}, true, nil
}

// sameSubject compares all descriptor fields that identify an OCI subject.
func sameSubject(a, b ocispec.Descriptor) bool {
	return a.Digest == b.Digest && a.MediaType == b.MediaType && a.Size == b.Size
}

// matchesSelectors reports whether candidate satisfies
// every non-empty selector in opts.
func matchesSelectors(candidate discoveryCandidate, opts DiscoverOptions) bool {
	annotations := candidate.manifest.Annotations
	return (opts.Document == "" || annotations[spec.AnnotationDocumentID] == opts.Document) &&
		(opts.Language == "" || annotations[spec.AnnotationDocumentLanguage] == opts.Language) &&
		(opts.Variant == "" || annotations[spec.AnnotationDocumentVariant] == opts.Variant) &&
		(opts.Digest == "" || candidate.descriptor.Digest == opts.Digest)
}

// reconcileDiscovery selects a result for the requested discovery mode.
func reconcileDiscovery(
	opts DiscoverOptions,
	tagBranch, referrerBranch discoveryBranch,
) (discoverySelection, error) {
	if opts.Mode == DiscoveryTag {
		return selectSingle(tagBranch, true, false)
	}
	if opts.Mode == DiscoveryReferrer {
		return selectSingle(referrerBranch, false, true)
	}

	return reconcileAutoDiscovery(opts, tagBranch, referrerBranch)
}

// reconcileAutoDiscovery combines direct-tag and referrer results.
// In non-strict mode it may use a direct tag after a temporary referrer failure.
func reconcileAutoDiscovery(
	opts DiscoverOptions,
	tagBranch, referrerBranch discoveryBranch,
) (discoverySelection, error) {
	tagMissing := errors.Is(tagBranch.err, ErrNotFound)
	if tagBranch.err != nil && !tagMissing {
		return discoverySelection{}, tagBranch.err
	}

	if referrerBranch.err != nil {
		if len(tagBranch.candidates) == 1 && !opts.Strict && orasrepo.IsTemporary(referrerBranch.err) {
			warning := fmt.Sprintf("referrer discovery failed; selected direct tag: %v", referrerBranch.err)
			return discoverySelection{
				selected: tagBranch.candidates[0], candidates: tagBranch.candidates,
				tagAvailable: true, warnings: []string{warning},
			}, nil
		}
		return discoverySelection{}, referrerBranch.err
	}

	tagFound := len(tagBranch.candidates) == 1
	refs := referrerBranch.candidates
	if len(refs) == 0 {
		if tagFound {
			return discoverySelection{
				selected: tagBranch.candidates[0], candidates: tagBranch.candidates, tagAvailable: true,
			}, nil
		}
		return discoverySelection{}, fmt.Errorf("%w: no attached artifact matches", ErrNotFound)
	}

	if tagFound {
		return reconcileTagAndReferrers(opts, tagBranch.candidates[0], refs)
	}

	if len(refs) == 1 {
		return discoverySelection{selected: refs[0], candidates: refs, referrerAvailable: true}, nil
	}

	return discoverySelection{}, fmt.Errorf("%w: %d referrers match", ErrAmbiguous, len(refs))
}

// reconcileTagAndReferrers prefers the common artifact.
// A single referrer may win a non-strict conflict;
// multiple distinct referrers remain ambiguous.
func reconcileTagAndReferrers(
	opts DiscoverOptions,
	tag discoveryCandidate,
	refs []discoveryCandidate,
) (discoverySelection, error) {
	for _, candidate := range refs {
		if candidate.descriptor.Digest == tag.descriptor.Digest {
			return discoverySelection{
				selected: candidate, candidates: refs, tagAvailable: true, referrerAvailable: true,
			}, nil
		}
	}
	if len(refs) > 1 {
		return discoverySelection{}, fmt.Errorf(
			"%w: %w: direct tag matches none of %d referrers", ErrConflict, ErrAmbiguous, len(refs),
		)
	}
	if opts.Strict {
		return discoverySelection{}, fmt.Errorf("%w: direct tag and referrer point to different manifests", ErrConflict)
	}

	return discoverySelection{
		selected: refs[0], candidates: refs, warnings: []string{"direct tag and referrer point to different manifests"},
		tagAvailable: true, referrerAvailable: true, conflict: true,
	}, nil
}

// selectSingle returns branch's only candidate and maps an absent result to ErrNotFound.
func selectSingle(
	branch discoveryBranch,
	tagAvailable, referrerAvailable bool,
) (discoverySelection, error) {
	if branch.err != nil {
		if errors.Is(branch.err, ErrNotFound) {
			return discoverySelection{}, fmt.Errorf("%w: no attached artifact matches", ErrNotFound)
		}
		return discoverySelection{}, branch.err
	}
	if len(branch.candidates) == 0 {
		return discoverySelection{}, fmt.Errorf("%w: no attached artifact matches", ErrNotFound)
	}
	if len(branch.candidates) > 1 {
		return discoverySelection{}, fmt.Errorf("%w: %d candidates match", ErrAmbiguous, len(branch.candidates))
	}

	return discoverySelection{
		selected: branch.candidates[0], candidates: branch.candidates,
		tagAvailable: tagAvailable, referrerAvailable: referrerAvailable,
	}, nil
}

// candidateDescriptors projects internal candidates into the public result.
func candidateDescriptors(candidates []discoveryCandidate) []ocispec.Descriptor {
	descriptors := make([]ocispec.Descriptor, len(candidates))
	for i := range candidates {
		descriptors[i] = candidates[i].descriptor
	}

	return descriptors
}
