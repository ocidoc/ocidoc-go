// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: Copyright 2026 WoozyMasta
// Source: github.com/ocidoc/ocidoc-go

package registry

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/opencontainers/go-digest"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"

	"github.com/ocidoc/ocidoc-go/internal/orasrepo"
	"github.com/ocidoc/ocidoc-go/spec"
)

func TestClientPruneRetainsPrimaryAndUnrelatedSlots(t *testing.T) {
	addr := startTestRegistry(t)
	subject := fmt.Sprintf("%s/prune:image", addr)
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
	primary, err := client.Attach(ctx,
		buildTestArtifactWithDocumentContent(t, "default", "en", "full", "primary"),
		subject, AttachOptions{})
	if err != nil {
		t.Fatalf("Attach primary: %v", err)
	}
	superseded, err := client.Attach(ctx,
		buildTestArtifactWithDocumentContent(t, "default", "en", "full", "superseded"),
		subject, AttachOptions{Publication: PublicationReferrer})
	if err != nil {
		t.Fatalf("Attach superseded: %v", err)
	}
	unrelated, err := client.Attach(ctx,
		buildTestArtifactWithDocumentContent(t, "api", "en", "full", "unrelated"),
		subject, AttachOptions{Publication: PublicationReferrer})
	if err != nil {
		t.Fatalf("Attach unrelated: %v", err)
	}

	preview, err := client.Prune(ctx, subject, PruneOptions{})
	if err != nil {
		t.Fatalf("Prune dry-run: %v", err)
	}
	if !preview.DryRun || preview.Keep.Digest != primary.Manifest.Digest {
		t.Fatalf("unexpected keep target: %+v", preview)
	}
	if len(preview.Selected) != 1 || preview.Selected[0].Digest != superseded.Manifest.Digest {
		t.Fatalf("unexpected selected manifests: %+v", preview.Selected)
	}
	assertReferrerDigests(t, repo, subjectDescriptor,
		primary.Manifest.Digest, superseded.Manifest.Digest, unrelated.Manifest.Digest)

	executed, err := client.Prune(ctx, subject, PruneOptions{Execute: true})
	if err != nil {
		t.Fatalf("Prune execute: %v", err)
	}
	if executed.DryRun ||
		len(executed.Deleted) != 1 ||
		executed.Deleted[0].Digest != superseded.Manifest.Digest {
		t.Fatalf("unexpected executed result: %+v", executed)
	}
	assertReferrerDigests(t, repo, subjectDescriptor, primary.Manifest.Digest, unrelated.Manifest.Digest)
}

func TestClientPruneRequiresExplicitKeepWithoutPrimary(t *testing.T) {
	addr := startTestRegistry(t)
	subject := fmt.Sprintf("%s/prune:image", addr)
	ctx := context.Background()
	repo, err := orasrepo.Open(subject, orasrepo.Options{PlainHTTP: true})
	if err != nil {
		t.Fatalf("orasrepo.Open: %v", err)
	}
	pushPlainManifest(t, repo, "image")

	client := NewClient(ClientOptions{PlainHTTP: true})
	keep, err := client.Attach(
		ctx,
		buildTestArtifactWithContent(t, "keep"),
		subject,
		AttachOptions{Publication: PublicationReferrer},
	)
	if err != nil {
		t.Fatalf("Attach keep: %v", err)
	}
	remove, err := client.Attach(
		ctx,
		buildTestArtifactWithContent(t, "remove"),
		subject,
		AttachOptions{Publication: PublicationReferrer},
	)
	if err != nil {
		t.Fatalf("Attach remove: %v", err)
	}

	_, err = client.Prune(ctx, subject, PruneOptions{})
	if !errors.Is(err, ErrConflict) {
		t.Fatalf("expected keep-required ErrConflict, got %v", err)
	}
	preview, err := client.Prune(ctx, subject, PruneOptions{Keep: keep.Manifest.Digest})
	if err != nil {
		t.Fatalf("Prune with keep: %v", err)
	}
	if preview.Keep.Digest != keep.Manifest.Digest ||
		len(preview.Selected) != 1 ||
		preview.Selected[0].Digest != remove.Manifest.Digest {
		t.Fatalf("unexpected preview: %+v", preview)
	}
}

func assertReferrerDigests(t *testing.T, repo orasrepo.Repository, subject ocispec.Descriptor, wants ...digest.Digest) {
	t.Helper()
	referrers, err := repo.Referrers(context.Background(), subject, spec.ArtifactType)
	if err != nil {
		t.Fatalf("Referrers: %v", err)
	}
	got := make(map[digest.Digest]bool, len(referrers))
	for _, referrer := range referrers {
		got[referrer.Digest] = true
	}
	if len(got) != len(wants) {
		t.Fatalf("got referrers %+v, want %v", referrers, wants)
	}
	for _, want := range wants {
		if !got[want] {
			t.Fatalf("missing referrer %s in %+v", want, referrers)
		}
	}
}
