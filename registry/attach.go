// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: Copyright 2026 WoozyMasta
// Source: github.com/ocidoc/ocidoc-go

package registry

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"maps"

	"github.com/opencontainers/go-digest"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"

	"github.com/ocidoc/ocidoc-go/artifact"
	"github.com/ocidoc/ocidoc-go/internal/orasrepo"
	"github.com/ocidoc/ocidoc-go/spec"
)

// PublicationMode selects how an attached artifact is made discoverable.
type PublicationMode string

const (
	// PublicationTag publishes the attached manifest through the direct .doc tag.
	PublicationTag PublicationMode = "tag"

	// PublicationReferrer publishes through OCI referrers only.
	PublicationReferrer PublicationMode = "referrer"

	// PublicationBoth publishes one manifest through both mechanisms.
	PublicationBoth PublicationMode = "both"
)

// AttachOptions controls attached publication.
type AttachOptions struct {
	// Publication selects direct-tag, referrer, or dual publication.
	Publication PublicationMode

	// Replace permits replacing an existing direct documentation tag.
	Replace bool
}

// AttachResult reports the resolved subject and attached manifest. Tag is empty
// for referrer-only publication.
type AttachResult struct {
	// SubjectReference is the caller-provided mutable or immutable subject reference.
	SubjectReference string

	// Reference identifies the attached manifest by Tag when directly published,
	// or by digest for referrer-only publication.
	Reference string

	// Tag is the direct documentation tag, or empty for referrer-only publication.
	Tag string

	// Subject is the resolved immutable subject descriptor.
	Subject ocispec.Descriptor

	// Manifest identifies the attached OCIDoc root manifest.
	Manifest ocispec.Descriptor

	// Publication is the mechanism used to publish Manifest.
	Publication PublicationMode

	// Replaced reports whether Attach replaced an existing direct tag.
	Replaced bool

	// Idempotent reports whether the requested artifact was already published.
	Idempotent bool
}

// Attach resolves subject to its immutable descriptor,
// creates a new OCIDoc root manifest carrying that exact descriptor,
// reuses source's config and component blobs,
// and publishes the manifest in the subject's repository.
func (c *Client) Attach(ctx context.Context, source artifact.Reader, subject string, opts AttachOptions) (*AttachResult, error) {
	if err := artifact.ValidateMetadata(ctx, source); err != nil {
		return nil, err
	}

	publication, err := normalizeAttachOptions(opts)
	if err != nil {
		return nil, err
	}
	resolved, err := c.resolveSubject(ctx, subject)
	if err != nil {
		return nil, err
	}

	manifest, manifestData, manifestDescriptor, err := attachedManifest(ctx, source, resolved.descriptor)
	if err != nil {
		return nil, err
	}

	var replaced, idempotent bool
	if publication.publishTag {
		existing, tagReplaced, tagIdempotent, err := checkTagReplacement(
			ctx, resolved.repo, resolved.documentationTag, manifestDescriptor, publication.replace,
		)
		if err != nil {
			return nil, err
		}
		replaced, idempotent = tagReplaced, tagIdempotent
		if idempotent && !publication.publishReferrer {
			return newAttachResult(resolved, resolved.documentationTag, existing, publication.mode, false, true), nil
		}
	}

	if err := c.publishAttached(ctx, source, resolved, manifest, manifestData, manifestDescriptor, publication); err != nil {
		return nil, err
	}

	docTag := ""
	if publication.publishTag {
		docTag = resolved.documentationTag
	}

	return newAttachResult(resolved, docTag, manifestDescriptor, publication.mode, replaced, idempotent), nil
}

// publicationPlan is the validated publication policy derived from AttachOptions.
type publicationPlan struct {
	// mode is the validated publication mode reported to the caller.
	mode PublicationMode

	// publishTag controls publication through the direct documentation tag.
	publishTag bool

	// publishReferrer controls publication through OCI referrers.
	publishReferrer bool

	// replace permits changing an existing direct documentation tag.
	replace bool
}

// normalizeAttachOptions applies Attach defaults
// and derives the publication operations required by the selected mode.
func normalizeAttachOptions(opts AttachOptions) (publicationPlan, error) {
	mode := opts.Publication
	if mode == "" {
		mode = PublicationBoth
	}
	if mode != PublicationTag && mode != PublicationReferrer && mode != PublicationBoth {
		return publicationPlan{}, fmt.Errorf("%w: invalid publication mode %q", ErrInvalid, mode)
	}
	if opts.Replace && mode == PublicationReferrer {
		return publicationPlan{}, fmt.Errorf("%w: replace requires tag or both publication", ErrInvalid)
	}

	return publicationPlan{
		mode:            mode,
		publishTag:      mode == PublicationTag || mode == PublicationBoth,
		publishReferrer: mode == PublicationReferrer || mode == PublicationBoth,
		replace:         opts.Replace,
	}, nil
}

// publishAttached uploads source content,
// then publishes its subject-bound manifest through the mechanisms selected by publication.
func (c *Client) publishAttached(
	ctx context.Context,
	source artifact.Reader,
	subject resolvedSubject,
	manifest ocispec.Manifest,
	manifestData []byte,
	manifestDescriptor ocispec.Descriptor,
	publication publicationPlan,
) error {
	if err := c.pushGraph(ctx, source, subject.repo, manifest.Config); err != nil {
		return err
	}
	if err := subject.repo.PushManifest(
		ctx, manifestDescriptor, bytes.NewReader(manifestData), publication.publishReferrer,
	); err != nil {
		return wrapError(err)
	}
	if publication.publishTag {
		if err := subject.repo.Tag(ctx, manifestDescriptor, subject.documentationTag); err != nil {
			return wrapError(err)
		}
	}

	return nil
}

// attachedManifest clones source's root manifest, binds it to subject,
// and returns the manifest, its serialized form, and matching descriptor.
func attachedManifest(
	ctx context.Context,
	source artifact.Reader,
	subject ocispec.Descriptor,
) (ocispec.Manifest, []byte, ocispec.Descriptor, error) {
	manifest, err := source.Manifest(ctx)
	if err != nil {
		return ocispec.Manifest{}, nil, ocispec.Descriptor{}, err
	}
	if manifest.Subject != nil {
		return ocispec.Manifest{}, nil, ocispec.Descriptor{}, fmt.Errorf("%w: source already has a subject", ErrUnsupported)
	}

	attached := *manifest
	attached.Subject = &subject
	attached.Annotations = maps.Clone(manifest.Annotations)

	data, err := json.Marshal(attached)
	if err != nil {
		return ocispec.Manifest{}, nil, ocispec.Descriptor{}, fmt.Errorf("marshal attached manifest: %w", err)
	}

	desc := ocispec.Descriptor{
		MediaType:    ocispec.MediaTypeImageManifest,
		ArtifactType: spec.ArtifactType,
		Digest:       digest.Canonical.FromBytes(data),
		Size:         int64(len(data)),
		Annotations:  maps.Clone(attached.Annotations),
	}

	return attached, data, desc, nil
}

// checkTagReplacement determines whether tag is absent,
// already points to wanted, or may be changed according to replace.
func checkTagReplacement(
	ctx context.Context,
	repo orasrepo.Repository,
	tag string,
	wanted ocispec.Descriptor,
	replace bool,
) (existing ocispec.Descriptor, replaced, idempotent bool, err error) {
	existing, err = repo.Resolve(ctx, tag)
	if err != nil {
		if errors.Is(err, orasrepo.ErrNotFound) {
			return ocispec.Descriptor{}, false, false, nil
		}
		return ocispec.Descriptor{}, false, false, wrapError(err)
	}

	if existing.Digest == wanted.Digest {
		return existing, false, true, nil
	}
	if !replace {
		return ocispec.Descriptor{}, false, false, fmt.Errorf(
			"%w: tag %s points to %s, new manifest is %s; enable replace to update it",
			ErrConflict, tag, existing.Digest, wanted.Digest,
		)
	}

	return existing, true, false, nil
}

// newAttachResult builds the result from resolved publication state.
// Direct publication uses the documentation tag as Reference;
// referrer-only publication uses the manifest digest.
func newAttachResult(
	subject resolvedSubject,
	tag string,
	manifestDescriptor ocispec.Descriptor,
	publication PublicationMode,
	replaced, idempotent bool,
) *AttachResult {
	reference := subject.repository + "@" + manifestDescriptor.Digest.String()
	if tag != "" {
		reference = subject.repository + ":" + tag
	}

	return &AttachResult{
		SubjectReference: subject.requested,
		Reference:        reference,
		Tag:              tag,
		Subject:          subject.descriptor,
		Manifest:         manifestDescriptor,
		Publication:      publication,
		Replaced:         replaced,
		Idempotent:       idempotent,
	}
}
