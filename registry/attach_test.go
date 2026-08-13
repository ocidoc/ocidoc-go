// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: Copyright 2026 WoozyMasta
// Source: github.com/ocidoc/ocidoc-go

package registry

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"testing"

	ocispec "github.com/opencontainers/image-spec/specs-go/v1"

	"github.com/ocidoc/ocidoc-go/internal/orasrepo"
	"github.com/ocidoc/ocidoc-go/spec"
)

func TestClientAttachTagPublishesImmutableSubject(t *testing.T) {
	addr := startTestRegistry(t)
	subject := fmt.Sprintf("%s/test:image", addr)
	ctx := context.Background()

	repo, err := orasrepo.Open(subject, orasrepo.Options{PlainHTTP: true})
	if err != nil {
		t.Fatalf("orasrepo.Open: %v", err)
	}
	pushPlainManifest(t, repo, "image")

	client := NewClient(ClientOptions{PlainHTTP: true})
	source := buildTestArtifact(t)
	result, err := client.Attach(ctx, source, subject, AttachOptions{Publication: PublicationTag})
	if err != nil {
		t.Fatalf("Attach: %v", err)
	}

	wantSubject, err := repo.Resolve(ctx, "image")
	if err != nil {
		t.Fatalf("Resolve subject: %v", err)
	}
	if result.Subject.Digest != wantSubject.Digest {
		t.Fatalf("got subject %s, want %s", result.Subject.Digest, wantSubject.Digest)
	}

	wantTag, err := spec.DocumentationTag(wantSubject.Digest)
	if err != nil {
		t.Fatalf("DocumentationTag: %v", err)
	}
	if result.Tag != wantTag {
		t.Fatalf("got tag %q, want %q", result.Tag, wantTag)
	}

	resolved, err := repo.Resolve(ctx, wantTag)
	if err != nil {
		t.Fatalf("Resolve .doc: %v", err)
	}
	if resolved.Digest != result.Manifest.Digest {
		t.Fatalf(".doc digest %s, want %s", resolved.Digest, result.Manifest.Digest)
	}

	manifest := fetchTestManifest(t, repo, resolved)
	if manifest.Subject == nil || manifest.Subject.Digest != wantSubject.Digest {
		t.Fatalf("unexpected attached subject: %+v", manifest.Subject)
	}

	sourceManifest, err := source.Manifest(ctx)
	if err != nil {
		t.Fatalf("source.Manifest: %v", err)
	}
	if manifest.Config.Digest != sourceManifest.Config.Digest || len(manifest.Layers) != len(sourceManifest.Layers) {
		t.Fatalf("attached manifest did not reuse source graph: %+v", manifest)
	}
	for i := range manifest.Layers {
		if manifest.Layers[i].Digest != sourceManifest.Layers[i].Digest {
			t.Fatalf("layer %d digest %s, want %s", i, manifest.Layers[i].Digest, sourceManifest.Layers[i].Digest)
		}
	}
	sourceRoot, err := source.Root(ctx)
	if err != nil {
		t.Fatalf("source.Root: %v", err)
	}
	if result.Manifest.Digest == sourceRoot.Digest {
		t.Fatal("attached manifest digest unexpectedly matches standalone source")
	}

	referrers, err := repo.Referrers(ctx, wantSubject, spec.ArtifactType)
	if err != nil {
		t.Fatalf("Referrers: %v", err)
	}
	if len(referrers) != 0 {
		t.Fatalf("tag-only publication created fallback referrers: %+v", referrers)
	}
}

func TestClientAttachReferrerPublishesFallbackIndexIdempotently(t *testing.T) {
	addr := startTestRegistry(t)
	subject := fmt.Sprintf("%s/test:image", addr)
	ctx := context.Background()

	repo, err := orasrepo.Open(subject, orasrepo.Options{PlainHTTP: true})
	if err != nil {
		t.Fatalf("orasrepo.Open: %v", err)
	}
	pushPlainManifest(t, repo, "image")
	subjectDescriptor, err := repo.Resolve(ctx, "image")
	if err != nil {
		t.Fatalf("Resolve subject: %v", err)
	}

	client := NewClient(ClientOptions{PlainHTTP: true})
	source := buildTestArtifact(t)
	first, err := client.Attach(ctx, source, subject, AttachOptions{Publication: PublicationReferrer})
	if err != nil {
		t.Fatalf("Attach: %v", err)
	}
	if first.Tag != "" || first.Publication != PublicationReferrer {
		t.Fatalf("unexpected result: %+v", first)
	}
	if first.Reference != fmt.Sprintf("%s/test@%s", addr, first.Manifest.Digest) {
		t.Fatalf("unexpected digest reference %q", first.Reference)
	}

	assertSingleReferrer(t, repo, subjectDescriptor, first.Manifest)
	second, err := client.Attach(ctx, source, subject, AttachOptions{Publication: PublicationReferrer})
	if err != nil {
		t.Fatalf("repeated Attach: %v", err)
	}
	if second.Manifest.Digest != first.Manifest.Digest {
		t.Fatalf("repeated digest %s, want %s", second.Manifest.Digest, first.Manifest.Digest)
	}
	assertSingleReferrer(t, repo, subjectDescriptor, first.Manifest)
}

func TestClientAttachBothPublishesSameManifestByTagAndReferrer(t *testing.T) {
	addr := startTestRegistry(t)
	subject := fmt.Sprintf("%s/test:image", addr)
	ctx := context.Background()

	repo, err := orasrepo.Open(subject, orasrepo.Options{PlainHTTP: true})
	if err != nil {
		t.Fatalf("orasrepo.Open: %v", err)
	}
	pushPlainManifest(t, repo, "image")
	subjectDescriptor, err := repo.Resolve(ctx, "image")
	if err != nil {
		t.Fatalf("Resolve subject: %v", err)
	}

	client := NewClient(ClientOptions{PlainHTTP: true})
	result, err := client.Attach(ctx, buildTestArtifact(t), subject, AttachOptions{})
	if err != nil {
		t.Fatalf("Attach: %v", err)
	}
	if result.Publication != PublicationBoth || result.Tag == "" {
		t.Fatalf("unexpected result: %+v", result)
	}
	resolved, err := repo.Resolve(ctx, result.Tag)
	if err != nil {
		t.Fatalf("Resolve .doc: %v", err)
	}
	if resolved.Digest != result.Manifest.Digest {
		t.Fatalf(".doc digest %s, want %s", resolved.Digest, result.Manifest.Digest)
	}
	assertSingleReferrer(t, repo, subjectDescriptor, result.Manifest)
}

func TestClientAttachTagReplacementGuardAndIdempotency(t *testing.T) {
	addr := startTestRegistry(t)
	subject := fmt.Sprintf("%s/test:image", addr)
	ctx := context.Background()

	repo, err := orasrepo.Open(subject, orasrepo.Options{PlainHTTP: true})
	if err != nil {
		t.Fatalf("orasrepo.Open: %v", err)
	}
	pushPlainManifest(t, repo, "image")

	client := NewClient(ClientOptions{PlainHTTP: true})
	firstSource := buildTestArtifactWithContent(t, "first")
	first, err := client.Attach(ctx, firstSource, subject, AttachOptions{Publication: PublicationTag})
	if err != nil {
		t.Fatalf("first Attach: %v", err)
	}

	idempotent, err := client.Attach(ctx, firstSource, subject, AttachOptions{Publication: PublicationTag})
	if err != nil {
		t.Fatalf("idempotent Attach: %v", err)
	}
	if !idempotent.Idempotent || idempotent.Manifest.Digest != first.Manifest.Digest {
		t.Fatalf("unexpected idempotent result: %+v", idempotent)
	}

	secondSource := buildTestArtifactWithContent(t, "second")
	_, err = client.Attach(ctx, secondSource, subject, AttachOptions{Publication: PublicationTag})
	if !errors.Is(err, ErrConflict) {
		t.Fatalf("expected errors.Is(err, ErrConflict), got %v", err)
	}

	replaced, err := client.Attach(ctx, secondSource, subject, AttachOptions{
		Publication: PublicationTag,
		Replace:     true,
	})
	if err != nil {
		t.Fatalf("replacement Attach: %v", err)
	}
	if !replaced.Replaced || replaced.Manifest.Digest == first.Manifest.Digest {
		t.Fatalf("unexpected replacement result: %+v", replaced)
	}

	resolved, err := repo.Resolve(ctx, replaced.Tag)
	if err != nil {
		t.Fatalf("Resolve replaced .doc: %v", err)
	}
	if resolved.Digest != replaced.Manifest.Digest {
		t.Fatalf(".doc digest %s, want replacement %s", resolved.Digest, replaced.Manifest.Digest)
	}
}

func TestClientAttachRejectsInvalidPublicationOptions(t *testing.T) {
	client := NewClient(ClientOptions{})
	_, err := client.Attach(context.Background(), buildTestArtifact(t), "example.invalid/repo:image", AttachOptions{
		Publication: PublicationMode("unknown"),
	})
	if !errors.Is(err, ErrInvalid) {
		t.Fatalf("expected errors.Is(err, ErrInvalid), got %v", err)
	}

	_, err = client.Attach(context.Background(), buildTestArtifact(t), "example.invalid/repo:image", AttachOptions{
		Publication: PublicationReferrer,
		Replace:     true,
	})
	if !errors.Is(err, ErrInvalid) {
		t.Fatalf("expected replace errors.Is(err, ErrInvalid), got %v", err)
	}
}

func assertSingleReferrer(
	t *testing.T,
	repo orasrepo.Repository,
	subject, want ocispec.Descriptor,
) {
	t.Helper()

	referrers, err := repo.Referrers(context.Background(), subject, spec.ArtifactType)
	if err != nil {
		t.Fatalf("Referrers: %v", err)
	}
	if len(referrers) != 1 || referrers[0].Digest != want.Digest {
		t.Fatalf("got referrers %+v, want only %s", referrers, want.Digest)
	}
}

func fetchTestManifest(t *testing.T, repo orasrepo.Repository, desc ocispec.Descriptor) ocispec.Manifest {
	t.Helper()

	rc, err := repo.Fetch(context.Background(), desc)
	if err != nil {
		t.Fatalf("Fetch manifest: %v", err)
	}
	defer rc.Close() //nolint:errcheck // test cleanup.

	data, err := io.ReadAll(rc)
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}

	var manifest ocispec.Manifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}

	return manifest
}
