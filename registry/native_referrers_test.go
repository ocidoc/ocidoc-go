// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: Copyright 2026 WoozyMasta
// Source: github.com/ocidoc/ocidoc-go

package registry

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os/exec"
	"strings"
	"testing"

	ocispec "github.com/opencontainers/image-spec/specs-go/v1"

	"github.com/ocidoc/ocidoc-go/internal/orasrepo"
	"github.com/ocidoc/ocidoc-go/spec"
)

const zotMinimalImage = "ghcr.io/project-zot/zot-minimal:v2.1.11"

// TestClientAttachReferrerUsesNativeAPI verifies publication
// and discovery against a registry that implements the OCI 1.1 Referrers API.
// registry:2 remains the separate fixture for ORAS's referrers-tag fallback path.
func TestClientAttachReferrerUsesNativeAPI(t *testing.T) {
	addr := startTestZot(t)
	ctx := context.Background()
	subject := fmt.Sprintf("%s/test:image", addr)

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
	attached, err := client.Attach(ctx, buildTestArtifact(t), subject, AttachOptions{Publication: PublicationReferrer})
	if err != nil {
		t.Fatalf("Attach: %v", err)
	}
	if attached.Tag != "" {
		t.Fatalf("got direct tag %q for referrer-only publication", attached.Tag)
	}

	index := fetchNativeReferrers(t, addr, "test", subjectDescriptor.Digest.String())
	if !containsDescriptor(index.Manifests, attached.Manifest) {
		t.Fatalf("native referrers response does not contain %s: %+v", attached.Manifest.Digest, index.Manifests)
	}

	discovered, err := client.Discover(ctx, subject, DiscoverOptions{Mode: DiscoveryReferrer})
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}
	if discovered.Manifest.Digest != attached.Manifest.Digest {
		t.Fatalf("got discovered manifest %s, want %s", discovered.Manifest.Digest, attached.Manifest.Digest)
	}

	docTag, err := spec.DocumentationTag(subjectDescriptor.Digest)
	if err != nil {
		t.Fatalf("DocumentationTag: %v", err)
	}
	if _, err := repo.Resolve(ctx, docTag); err == nil {
		t.Fatalf("referrer-only publication unexpectedly created direct tag %q", docTag)
	}
}

// startTestZot starts a local OCI 1.1 registry with native referrers support.
func startTestZot(t *testing.T) string {
	t.Helper()

	if testing.Short() {
		t.Skip("skipping Docker Zot integration test in short mode")
	}
	if _, err := exec.LookPath("docker"); err != nil {
		t.Skip("docker not available, skipping native referrers integration test")
	}

	out, err := exec.Command("docker", "run", "-d", "--rm",
		"-p", "127.0.0.1:0:5000",
		zotMinimalImage,
	).Output()
	if err != nil {
		t.Skipf("docker run %s: %v", zotMinimalImage, err)
	}
	containerID := strings.TrimSpace(string(out))
	t.Cleanup(func() {
		_ = exec.Command("docker", "stop", containerID).Run()
	})

	portOut, err := exec.Command("docker", "port", containerID, "5000/tcp").Output()
	if err != nil {
		t.Fatalf("docker port: %v", err)
	}
	addr := firstLine(string(portOut))
	addr = strings.Replace(addr, "0.0.0.0", "127.0.0.1", 1)
	waitRegistryReady(t, addr)

	return addr
}

// fetchNativeReferrers reads the native OCI Referrers API directly so the
// test proves the registry endpoint, rather than only ORAS fallback behavior.
func fetchNativeReferrers(t *testing.T, addr, repository, subjectDigest string) ocispec.Index {
	t.Helper()

	//nolint:gosec,noctx // fixed local test registry.
	resp, err := http.Get(fmt.Sprintf("http://%s/v2/%s/referrers/%s", addr, repository, subjectDigest))
	if err != nil {
		t.Fatalf("GET native referrers: %v", err)
	}
	defer resp.Body.Close() //nolint:errcheck // response is fully decoded below.
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET native referrers: got HTTP %d, want %d", resp.StatusCode, http.StatusOK)
	}

	var index ocispec.Index
	if err := json.NewDecoder(resp.Body).Decode(&index); err != nil {
		t.Fatalf("decode native referrers response: %v", err)
	}
	return index
}

// containsDescriptor reports whether descriptors contain wanted by digest.
func containsDescriptor(descriptors []ocispec.Descriptor, wanted ocispec.Descriptor) bool {
	for _, descriptor := range descriptors {
		if descriptor.Digest == wanted.Digest {
			return true
		}
	}
	return false
}
