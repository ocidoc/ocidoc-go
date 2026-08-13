// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: Copyright 2026 WoozyMasta
// Source: github.com/ocidoc/ocidoc-go

package registry

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/ocidoc/ocidoc-go/internal/orasrepo"
	"github.com/ocidoc/ocidoc-go/spec"
)

func TestClientRemoveDryRunAndDeleteStandalone(t *testing.T) {
	addr := startTestRegistry(t)
	reference := fmt.Sprintf("%s/test:docs", addr)
	ctx := context.Background()
	client := NewClient(ClientOptions{PlainHTTP: true})
	pushed, err := client.Push(ctx, buildTestArtifact(t), reference)
	if err != nil {
		t.Fatalf("Push: %v", err)
	}

	preview, err := client.Remove(ctx, reference, RemoveOptions{DryRun: true})
	if err != nil {
		t.Fatalf("Remove dry-run: %v", err)
	}
	if preview.Deleted || !preview.DryRun || preview.Manifest.Digest != pushed.Manifest.Digest {
		t.Fatalf("unexpected preview: %+v", preview)
	}
	if _, err := client.Resolve(ctx, reference); err != nil {
		t.Fatalf("Resolve after dry-run: %v", err)
	}

	removed, err := client.Remove(ctx, preview.Reference, RemoveOptions{})
	if err != nil {
		t.Fatalf("Remove: %v", err)
	}
	if !removed.Deleted || removed.DryRun {
		t.Fatalf("unexpected delete result: %+v", removed)
	}
	if _, err := client.Resolve(ctx, reference); !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound after delete, got %v", err)
	}
}

func TestClientRemoveRefusesNonOCIDocManifest(t *testing.T) {
	addr := startTestRegistry(t)
	reference := fmt.Sprintf("%s/test:image", addr)
	repo, err := orasrepo.Open(reference, orasrepo.Options{PlainHTTP: true})
	if err != nil {
		t.Fatalf("orasrepo.Open: %v", err)
	}
	pushPlainManifest(t, repo, "image")
	plain, err := repo.Resolve(context.Background(), "image")
	if err != nil {
		t.Fatalf("Resolve plain manifest: %v", err)
	}

	client := NewClient(ClientOptions{PlainHTTP: true})
	_, err = client.Remove(context.Background(), reference, RemoveOptions{})
	if !errors.Is(err, ErrInvalid) {
		t.Fatalf("expected errors.Is(err, ErrInvalid), got %v", err)
	}
	resolved, err := repo.Resolve(context.Background(), "image")
	if err != nil || resolved.Digest != plain.Digest {
		t.Fatalf("non-OCIDoc manifest was modified: %v, %+v", err, resolved)
	}
}

func TestClientDetachUpdatesFallbackAndRemovesDirectTag(t *testing.T) {
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
	attached, err := client.Attach(ctx, buildTestArtifact(t), subject, AttachOptions{})
	if err != nil {
		t.Fatalf("Attach: %v", err)
	}
	preview, err := client.Detach(ctx, subject, DetachOptions{DryRun: true})
	if err != nil {
		t.Fatalf("Detach dry-run: %v", err)
	}
	if preview.Manifest.Digest != attached.Manifest.Digest || preview.Subject == nil {
		t.Fatalf("unexpected preview: %+v", preview)
	}
	assertSingleReferrer(t, repo, subjectDescriptor, attached.Manifest)

	removed, err := client.Detach(ctx, subject, DetachOptions{})
	if err != nil {
		t.Fatalf("Detach: %v", err)
	}
	if !removed.Deleted {
		t.Fatalf("unexpected detach result: %+v", removed)
	}
	referrers, err := repo.Referrers(ctx, subjectDescriptor, spec.ArtifactType)
	if err != nil {
		t.Fatalf("Referrers: %v", err)
	}
	if len(referrers) != 0 {
		t.Fatalf("fallback index still contains deleted manifest: %+v", referrers)
	}
	if _, err := repo.Resolve(ctx, attached.Tag); !errors.Is(err, orasrepo.ErrNotFound) {
		t.Fatalf("expected .doc tag removal, got %v", err)
	}
}
