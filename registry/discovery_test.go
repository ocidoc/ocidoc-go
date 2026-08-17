// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: Copyright 2026 WoozyMasta
// Source: github.com/ocidoc/ocidoc-go

package registry

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/ocidoc/ocidoc-go/artifact"
	"github.com/ocidoc/ocidoc-go/internal/orasrepo"
)

func TestClientDiscoverAutoReconcilesBothPublication(t *testing.T) {
	addr := startTestRegistry(t)
	subject := fmt.Sprintf("%s/test:image", addr)
	ctx := context.Background()
	repo, err := orasrepo.Open(subject, orasrepo.Options{PlainHTTP: true})
	if err != nil {
		t.Fatalf("orasrepo.Open: %v", err)
	}
	pushPlainManifest(t, repo, "image")

	client := NewClient(ClientOptions{PlainHTTP: true})
	attached, err := client.Attach(ctx, buildTestArtifact(t), subject, AttachOptions{})
	if err != nil {
		t.Fatalf("Attach: %v", err)
	}

	result, err := client.Discover(ctx, subject, DiscoverOptions{})
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}
	if result.Manifest.Digest != attached.Manifest.Digest || !result.TagAvailable || !result.ReferrerAvailable {
		t.Fatalf("unexpected discovery result: %+v", result)
	}
	if result.Conflict || len(result.Warnings) != 0 || result.Mode != DiscoveryAuto {
		t.Fatalf("unexpected reconciliation metadata: %+v", result)
	}

	for _, mode := range []DiscoveryMode{DiscoveryTag, DiscoveryReferrer} {
		modeResult, err := client.Discover(ctx, subject, DiscoverOptions{Mode: mode})
		if err != nil {
			t.Fatalf("Discover(%s): %v", mode, err)
		}
		if modeResult.Manifest.Digest != attached.Manifest.Digest {
			t.Fatalf("Discover(%s) digest %s, want %s", mode, modeResult.Manifest.Digest, attached.Manifest.Digest)
		}
	}
}

func TestClientDiscoverSelectorsResolveMultipleReferrers(t *testing.T) {
	addr := startTestRegistry(t)
	subject := fmt.Sprintf("%s/test:image", addr)
	ctx := context.Background()
	repo, err := orasrepo.Open(subject, orasrepo.Options{PlainHTTP: true})
	if err != nil {
		t.Fatalf("orasrepo.Open: %v", err)
	}
	pushPlainManifest(t, repo, "image")

	client := NewClient(ClientOptions{PlainHTTP: true})
	en, err := client.Attach(ctx, buildTestArtifactWithDocument(t, "api-en", "en", "full"), subject, AttachOptions{Publication: PublicationReferrer})
	if err != nil {
		t.Fatalf("Attach en: %v", err)
	}
	fr, err := client.Attach(ctx, buildTestArtifactWithDocument(t, "api-fr", "fr", "full"), subject, AttachOptions{Publication: PublicationReferrer})
	if err != nil {
		t.Fatalf("Attach fr: %v", err)
	}

	_, err = client.Discover(ctx, subject, DiscoverOptions{})
	if !errors.Is(err, ErrAmbiguous) {
		t.Fatalf("expected errors.Is(err, ErrAmbiguous), got %v", err)
	}

	result, err := client.Discover(ctx, subject, DiscoverOptions{Document: "api-en", Variant: "full"})
	if err != nil {
		t.Fatalf("Discover selectors: %v", err)
	}
	if result.Manifest.Digest != en.Manifest.Digest || result.Manifest.Digest == fr.Manifest.Digest {
		t.Fatalf("selected %s, want en %s", result.Manifest.Digest, en.Manifest.Digest)
	}

	byDigest, err := client.Discover(ctx, subject, DiscoverOptions{Digest: fr.Manifest.Digest})
	if err != nil {
		t.Fatalf("Discover digest: %v", err)
	}
	if byDigest.Manifest.Digest != fr.Manifest.Digest {
		t.Fatalf("selected %s, want fr %s", byDigest.Manifest.Digest, fr.Manifest.Digest)
	}
}

func TestClientDiscoverConflictUsesReferrerUnlessStrict(t *testing.T) {
	addr := startTestRegistry(t)
	subject := fmt.Sprintf("%s/test:image", addr)
	ctx := context.Background()
	repo, err := orasrepo.Open(subject, orasrepo.Options{PlainHTTP: true})
	if err != nil {
		t.Fatalf("orasrepo.Open: %v", err)
	}
	pushPlainManifest(t, repo, "image")

	client := NewClient(ClientOptions{PlainHTTP: true})
	tagged, err := client.Attach(ctx, buildTestArtifactWithContent(t, "tagged"), subject, AttachOptions{Publication: PublicationTag})
	if err != nil {
		t.Fatalf("Attach tag: %v", err)
	}
	referred, err := client.Attach(ctx, buildTestArtifactWithContent(t, "referred"), subject, AttachOptions{Publication: PublicationReferrer})
	if err != nil {
		t.Fatalf("Attach referrer: %v", err)
	}

	result, err := client.Discover(ctx, subject, DiscoverOptions{})
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}
	if result.Manifest.Digest != referred.Manifest.Digest || result.Manifest.Digest == tagged.Manifest.Digest {
		t.Fatalf("unexpected selected digest: %+v", result)
	}
	if !result.Conflict || len(result.Warnings) != 1 {
		t.Fatalf("expected reported conflict: %+v", result)
	}

	_, err = client.Discover(ctx, subject, DiscoverOptions{Strict: true})
	if !errors.Is(err, ErrConflict) {
		t.Fatalf("expected strict errors.Is(err, ErrConflict), got %v", err)
	}
}

func TestClientDiscoverNotFoundAndInvalidMode(t *testing.T) {
	client := NewClient(ClientOptions{})
	_, err := client.Discover(context.Background(), "example.invalid/repo:image", DiscoverOptions{Mode: DiscoveryMode("newest")})
	if !errors.Is(err, ErrInvalid) {
		t.Fatalf("expected errors.Is(err, ErrInvalid), got %v", err)
	}
}

func buildTestArtifactWithDocument(t *testing.T, id, label, variant string) artifact.Reader {
	return buildTestArtifactWithDocumentContent(t, id, label, variant, fmt.Sprintf("# %s-%s-%s", id, label, variant))
}

func buildTestArtifactWithDocumentContent(t *testing.T, id, label, variant, content string) artifact.Reader {
	t.Helper()

	root := t.TempDir()
	writeFiles(t, root, map[string]string{
		"ocidoc.yaml": fmt.Sprintf(`
schemaVersion: v1beta
document:
  id: %s
  variant: %s
components:
  documentation:
    - /README.md
`, id, variant),
		"README.md": content,
	})
	layoutDir := t.TempDir()
	if _, _, err := artifact.BuildLayout(t.Context(), root, layoutDir, artifact.BuildLayoutOptions{
		ModTime: time.Unix(1700000000, 0).UTC(),
	}); err != nil {
		t.Fatalf("BuildLayout: %v", err)
	}
	reader, err := artifact.OpenLayout(layoutDir)
	if err != nil {
		t.Fatalf("OpenLayout: %v", err)
	}
	t.Cleanup(func() { _ = reader.Close() })
	return reader
}
