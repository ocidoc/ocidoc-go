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
	"os"
	"path/filepath"
	"testing"

	"github.com/opencontainers/go-digest"
	specs "github.com/opencontainers/image-spec/specs-go"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"

	"github.com/ocidoc/ocidoc-go/artifact"
	"github.com/ocidoc/ocidoc-go/internal/orasrepo"
	"github.com/ocidoc/ocidoc-go/spec"
)

func TestClientPullStandaloneRoundTrip(t *testing.T) {
	addr := startTestRegistry(t)
	reference := fmt.Sprintf("%s/test:1.0.0", addr)

	client := NewClient(ClientOptions{PlainHTTP: true})

	pushed, err := client.Push(context.Background(), buildTestArtifact(t), reference)
	if err != nil {
		t.Fatalf("Push: %v", err)
	}

	dest := filepath.Join(t.TempDir(), "pulled.ocidoc")

	result, err := client.Pull(context.Background(), reference, artifact.Destination{Path: dest})
	if err != nil {
		t.Fatalf("Pull: %v", err)
	}

	if result.Output != dest {
		t.Fatalf("got output %q, want %q", result.Output, dest)
	}

	if result.Manifest.Digest != pushed.Manifest.Digest {
		t.Fatalf("got pulled manifest digest %s, want %s", result.Manifest.Digest, pushed.Manifest.Digest)
	}

	pulled, err := artifact.OpenArchive(dest)
	if err != nil {
		t.Fatalf("OpenArchive: %v", err)
	}
	defer pulled.Close() //nolint:errcheck // test cleanup.

	components, err := pulled.Components(context.Background())
	if err != nil {
		t.Fatalf("Components: %v", err)
	}

	if len(components) != 1 || components[0].Type != spec.ComponentDocumentation {
		t.Fatalf("unexpected pulled components: %+v", components)
	}
}

func TestClientPullDiscoveredAttachedRoundTrip(t *testing.T) {
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
	discovered, err := client.Discover(ctx, subject, DiscoverOptions{})
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}

	dest := filepath.Join(t.TempDir(), "attached.ocidoc")
	result, err := client.Pull(ctx, discovered.Reference, artifact.Destination{Path: dest})
	if err != nil {
		t.Fatalf("Pull: %v", err)
	}
	if result.Manifest.Digest != attached.Manifest.Digest {
		t.Fatalf("pulled digest %s, want %s", result.Manifest.Digest, attached.Manifest.Digest)
	}
	reader, err := artifact.OpenArchive(dest)
	if err != nil {
		t.Fatalf("OpenArchive: %v", err)
	}
	defer reader.Close() //nolint:errcheck // test cleanup.
	manifest, err := reader.Manifest(ctx)
	if err != nil {
		t.Fatalf("Manifest: %v", err)
	}
	if manifest.Subject == nil || manifest.Subject.Digest != discovered.Subject.Digest {
		t.Fatalf("unexpected pulled subject: %+v", manifest.Subject)
	}
}

func TestClientPullRejectsExistingDestinationWithoutOverwrite(t *testing.T) {
	addr := startTestRegistry(t)
	reference := fmt.Sprintf("%s/test:1.0.0", addr)

	client := NewClient(ClientOptions{PlainHTTP: true})

	if _, err := client.Push(context.Background(), buildTestArtifact(t), reference); err != nil {
		t.Fatalf("Push: %v", err)
	}

	dest := filepath.Join(t.TempDir(), "pulled.ocidoc")
	if err := os.WriteFile(dest, []byte("existing"), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	_, err := client.Pull(context.Background(), reference, artifact.Destination{Path: dest})
	if !errors.Is(err, ErrInvalid) {
		t.Fatalf("expected errors.Is(err, ErrInvalid), got %v", err)
	}
}

func TestClientPullOverwritesExistingDestination(t *testing.T) {
	addr := startTestRegistry(t)
	reference := fmt.Sprintf("%s/test:1.0.0", addr)

	client := NewClient(ClientOptions{PlainHTTP: true})

	if _, err := client.Push(context.Background(), buildTestArtifact(t), reference); err != nil {
		t.Fatalf("Push: %v", err)
	}

	dest := filepath.Join(t.TempDir(), "pulled.ocidoc")
	if err := os.WriteFile(dest, []byte("existing"), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	if _, err := client.Pull(
		context.Background(),
		reference,
		artifact.Destination{Path: dest, Overwrite: true},
	); err != nil {
		t.Fatalf("Pull: %v", err)
	}
}

func TestClientPullNonexistentReferenceIsNotFound(t *testing.T) {
	addr := startTestRegistry(t)
	reference := fmt.Sprintf("%s/test:does-not-exist", addr)

	client := NewClient(ClientOptions{PlainHTTP: true})
	dest := filepath.Join(t.TempDir(), "pulled.ocidoc")

	_, err := client.Pull(context.Background(), reference, artifact.Destination{Path: dest})
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected errors.Is(err, ErrNotFound), got %v", err)
	}
}

func TestClientPullRejectsNonOCIDocManifest(t *testing.T) {
	addr := startTestRegistry(t)
	reference := fmt.Sprintf("%s/test:1.0.0", addr)

	repo, err := orasrepo.Open(reference, orasrepo.Options{PlainHTTP: true})
	if err != nil {
		t.Fatalf("orasrepo.Open: %v", err)
	}

	pushPlainManifest(t, repo, "1.0.0")

	client := NewClient(ClientOptions{PlainHTTP: true})
	dest := filepath.Join(t.TempDir(), "pulled.ocidoc")

	_, err = client.Pull(context.Background(), reference, artifact.Destination{Path: dest})
	if !errors.Is(err, ErrInvalid) {
		t.Fatalf("expected errors.Is(err, ErrInvalid), got %v", err)
	}
}

// pushPlainManifest pushes a minimal, valid, but non-OCIDoc OCI manifest
// (no artifactType, no layers) to repo and tags it as tag -- exercising
// Pull's artifactType check without needing a whole OCIDoc build.
func pushPlainManifest(t *testing.T, repo orasrepo.Repository, tag string) {
	t.Helper()

	ctx := context.Background()

	configContent := []byte("{}")
	configDesc := ocispec.Descriptor{
		MediaType: "application/vnd.oci.empty.v1+json",
		Digest:    digest.FromBytes(configContent),
		Size:      int64(len(configContent)),
	}

	if err := repo.Push(ctx, configDesc, bytes.NewReader(configContent)); err != nil {
		t.Fatalf("push config: %v", err)
	}

	manifest := ocispec.Manifest{
		Versioned: specs.Versioned{SchemaVersion: 2},
		MediaType: ocispec.MediaTypeImageManifest,
		Config:    configDesc,
		Layers:    []ocispec.Descriptor{},
	}

	manifestBytes, err := json.Marshal(manifest)
	if err != nil {
		t.Fatalf("marshal manifest: %v", err)
	}

	manifestDesc := ocispec.Descriptor{
		MediaType: ocispec.MediaTypeImageManifest,
		Digest:    digest.FromBytes(manifestBytes),
		Size:      int64(len(manifestBytes)),
	}

	if err := repo.Push(ctx, manifestDesc, bytes.NewReader(manifestBytes)); err != nil {
		t.Fatalf("push manifest: %v", err)
	}

	if err := repo.Tag(ctx, manifestDesc, tag); err != nil {
		t.Fatalf("tag: %v", err)
	}
}
