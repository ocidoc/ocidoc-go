// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: Copyright 2026 WoozyMasta
// Source: github.com/ocidoc/ocidoc-go

package orasrepo

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/opencontainers/go-digest"
	specs "github.com/opencontainers/image-spec/specs-go"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"oras.land/oras-go/v2/registry/remote/auth"
)

// startTestRegistry starts a local,
// ephemeral "registry:2" container with deletes enabled and returns its "host:port" address.
// It skips the test if Docker is not available,
// so this package's other tests still run in environments without it.
func startTestRegistry(t *testing.T) string {
	t.Helper()

	if testing.Short() {
		t.Skip("skipping Docker registry integration test in short mode")
	}
	if _, err := exec.LookPath("docker"); err != nil {
		t.Skip("docker not available, skipping registry integration test")
	}

	out, err := exec.Command("docker", "run", "-d", "--rm",
		"-e", "REGISTRY_STORAGE_DELETE_ENABLED=true",
		"-p", "127.0.0.1:0:5000",
		"registry:2",
	).Output()
	if err != nil {
		t.Skipf("docker run registry:2: %v", err)
	}

	containerID := strings.TrimSpace(string(out))

	t.Cleanup(func() {
		_ = exec.Command("docker", "stop", containerID).Run()
	})

	portOut, err := exec.Command("docker", "port", containerID, "5000/tcp").CombinedOutput()
	if err != nil {
		t.Fatalf("docker port: %v: %s", err, strings.TrimSpace(string(portOut)))
	}

	addr := firstLine(string(portOut))
	addr = strings.Replace(addr, "0.0.0.0", "127.0.0.1", 1)

	waitRegistryReady(t, addr)

	return addr
}

func TestNewAuthClientResolvesScopedCredential(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.json")
	const config = `{
  "auths": {
    "registry.example": {"auth": "aG9zdDpwYXNzd29yZA=="},
    "registry.example/team-a": {"auth": "c2NvcGVkOnBhc3N3b3Jk"}
  }
}`
	if err := os.WriteFile(configPath, []byte(config), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	client, err := newAuthClient(Options{RegistryConfigPath: configPath})
	if err != nil {
		t.Fatalf("newAuthClient: %v", err)
	}
	ctx := auth.WithScopesForHost(
		context.Background(),
		"registry.example",
		auth.ScopeRepository("team-a/document", auth.ActionPull),
	)
	cred, err := client.Credential(ctx, "registry.example")
	if err != nil {
		t.Fatalf("Credential: %v", err)
	}
	if cred.Username != "scoped" || cred.Password != "password" {
		t.Fatalf("got %+v, want scoped Docker credential", cred)
	}
}

func TestNewAuthClientRegistryConfigPathDisablesDiscovery(t *testing.T) {
	dir := t.TempDir()
	discoveredDir := filepath.Join(dir, "discovered")
	if err := os.MkdirAll(discoveredDir, 0o750); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(filepath.Join(discoveredDir, "config.json"), []byte(
		`{"auths":{"registry.example":{"auth":"ZGlzY292ZXJlZDpzZWNyZXQ="}}}`), 0o600); err != nil {
		t.Fatalf("WriteFile discovered config: %v", err)
	}
	t.Setenv("DOCKER_CONFIG", discoveredDir)

	explicitPath := filepath.Join(dir, "explicit.json")
	if err := os.WriteFile(explicitPath, []byte(`{"auths":{}}`), 0o600); err != nil {
		t.Fatalf("WriteFile explicit config: %v", err)
	}

	client, err := newAuthClient(Options{RegistryConfigPath: explicitPath})
	if err != nil {
		t.Fatalf("newAuthClient: %v", err)
	}
	ctx := auth.WithScopesForHost(
		context.Background(),
		"registry.example",
		auth.ScopeRepository("team-a/document", auth.ActionPull),
	)
	cred, err := client.Credential(ctx, "registry.example")
	if err != nil {
		t.Fatalf("Credential: %v", err)
	}
	if cred != auth.EmptyCredential {
		t.Fatalf("got %+v, want anonymous credential from explicit empty config", cred)
	}
}

func TestNewAuthClientExplicitCredentialOverridesScopedConfig(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.json")
	const config = `{"auths":{"registry.example/team-a":{"auth":"c2NvcGVkOnBhc3N3b3Jk"}}}`
	if err := os.WriteFile(configPath, []byte(config), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	client, err := newAuthClient(Options{
		Username:           "explicit",
		Password:           "secret",
		RegistryConfigPath: configPath,
	})
	if err != nil {
		t.Fatalf("newAuthClient: %v", err)
	}
	ctx := auth.WithScopesForHost(
		context.Background(),
		"registry.example",
		auth.ScopeRepository("team-a/document", auth.ActionPull),
	)
	cred, err := client.Credential(ctx, "registry.example")
	if err != nil {
		t.Fatalf("Credential: %v", err)
	}
	if cred.Username != "explicit" || cred.Password != "secret" {
		t.Fatalf("got %+v, want explicit credential", cred)
	}
}

// firstLine returns s's first line, trimmed.
func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		s = s[:i]
	}

	return strings.TrimSpace(s)
}

// waitRegistryReady polls addr's /v2/ endpoint until it responds or the timeout elapses.
func waitRegistryReady(t *testing.T, addr string) {
	t.Helper()

	deadline := time.Now().Add(15 * time.Second)

	for time.Now().Before(deadline) {
		//nolint:gosec,noctx // fixed local test address, no user input.
		resp, err := http.Get("http://" + addr + "/v2/")
		if err == nil {
			_ = resp.Body.Close()

			if resp.StatusCode == http.StatusOK {
				return
			}
		}

		time.Sleep(200 * time.Millisecond)
	}

	t.Fatalf("registry at %s did not become ready in time", addr)
}

// pushTestManifest builds and pushes a minimal valid OCI manifest
// (an empty config blob, no layers) to repo,
// tags it as tag, and returns the manifest and config descriptors.
func pushTestManifest(t *testing.T, repo Repository, tag string) (ocispec.Descriptor, ocispec.Descriptor) {
	t.Helper()

	ctx := context.Background()

	configContent := []byte("{}")
	configDesc := ocispec.Descriptor{
		MediaType: "application/vnd.oci.empty.v1+json",
		Digest:    digestOf(configContent),
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
		Digest:    digestOf(manifestBytes),
		Size:      int64(len(manifestBytes)),
	}

	if err := repo.Push(ctx, manifestDesc, bytes.NewReader(manifestBytes)); err != nil {
		t.Fatalf("push manifest: %v", err)
	}

	if err := repo.Tag(ctx, manifestDesc, tag); err != nil {
		t.Fatalf("tag: %v", err)
	}

	return manifestDesc, configDesc
}

// digestOf returns content's canonical (sha256) digest.
func digestOf(content []byte) digest.Digest {
	return digest.FromBytes(content)
}

func TestOpenRoundTripsPushResolveFetchDelete(t *testing.T) {
	addr := startTestRegistry(t)
	reference := fmt.Sprintf("%s/test:latest", addr)

	repo, err := Open(reference, Options{PlainHTTP: true})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}

	manifestDesc, configDesc := pushTestManifest(t, repo, "latest")

	ctx := context.Background()

	resolved, err := repo.Resolve(ctx, "latest")
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}

	if resolved.Digest != manifestDesc.Digest {
		t.Fatalf("got digest %s, want %s", resolved.Digest, manifestDesc.Digest)
	}

	rc, err := repo.Fetch(ctx, manifestDesc)
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}

	fetched, err := io.ReadAll(rc)
	_ = rc.Close()

	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}

	var roundTripped ocispec.Manifest
	if err := json.Unmarshal(fetched, &roundTripped); err != nil {
		t.Fatalf("Unmarshal fetched manifest: %v", err)
	}

	if roundTripped.Config.Digest != configDesc.Digest {
		t.Fatalf("fetched manifest config digest mismatch: got %s, want %s",
			roundTripped.Config.Digest, configDesc.Digest)
	}

	if err := repo.Delete(ctx, manifestDesc); err != nil {
		t.Fatalf("Delete: %v", err)
	}

	if _, err := repo.Resolve(ctx, "latest"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected errors.Is(err, ErrNotFound) after delete, got %v", err)
	}
}

func TestResolveNonexistentTagIsNotFound(t *testing.T) {
	addr := startTestRegistry(t)
	reference := fmt.Sprintf("%s/test:latest", addr)

	repo, err := Open(reference, Options{PlainHTTP: true})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}

	_, err = repo.Resolve(context.Background(), "does-not-exist")
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected errors.Is(err, ErrNotFound), got %v", err)
	}
}

func TestReferrersOnFreshManifestIsEmpty(t *testing.T) {
	addr := startTestRegistry(t)
	reference := fmt.Sprintf("%s/test:latest", addr)

	repo, err := Open(reference, Options{PlainHTTP: true})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}

	manifestDesc, _ := pushTestManifest(t, repo, "latest")

	referrers, err := repo.Referrers(context.Background(), manifestDesc, "")
	if err != nil {
		t.Fatalf("Referrers: %v", err)
	}

	if len(referrers) != 0 {
		t.Fatalf("expected no referrers for a fresh manifest, got %+v", referrers)
	}
}
