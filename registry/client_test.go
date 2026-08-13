// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: Copyright 2026 WoozyMasta
// Source: github.com/ocidoc/ocidoc-go

package registry

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/ocidoc/ocidoc-go/artifact"
	"github.com/ocidoc/ocidoc-go/internal/testfixture"
)

// startTestRegistry starts a local,
// ephemeral "registry:2" container with deletes enabled and returns its "host:port" address.
// It skips the test if Docker is not available,
// so this package's other tests still run in environments without it.
// Mirrors internal/orasrepo's own test registry helper - kept local rather than shared,
// to keep this package's test dependencies self-contained.
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

	portOut, err := exec.Command("docker", "port", containerID, "5000/tcp").Output()
	if err != nil {
		t.Fatalf("docker port: %v", err)
	}

	addr := firstLine(string(portOut))
	addr = strings.Replace(addr, "0.0.0.0", "127.0.0.1", 1)

	waitRegistryReady(t, addr)

	return addr
}

// firstLine returns s's first line, trimmed.
func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		s = s[:i]
	}

	return strings.TrimSpace(s)
}

// waitRegistryReady polls addr's /v2/ endpoint
// until it responds or the timeout elapses.
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

// buildTestArtifact builds a minimal local OCI Image Layout
// and returns a Reader open on it, closing it automatically at test cleanup.
func buildTestArtifact(t *testing.T) artifact.Reader {
	return buildTestArtifactWithContent(t, "# hi")
}

func buildTestArtifactWithContent(t *testing.T, content string) artifact.Reader {
	return testfixture.BuildArtifact(t, content)
}

// writeFiles writes files (path -> content, "/"-separated, relative to root) under root,
// creating parent directories as needed.
func writeFiles(t *testing.T, root string, files map[string]string) {
	testfixture.WriteFiles(t, root, files)
}

func TestClientPushAndResolveRoundTrip(t *testing.T) {
	addr := startTestRegistry(t)
	reference := fmt.Sprintf("%s/test:latest", addr)

	source := buildTestArtifact(t)

	root, err := source.Root(context.Background())
	if err != nil {
		t.Fatalf("Root: %v", err)
	}

	client := NewClient(ClientOptions{PlainHTTP: true})

	result, err := client.Push(context.Background(), source, reference)
	if err != nil {
		t.Fatalf("Push: %v", err)
	}

	if result.Manifest.Digest != root.Digest {
		t.Fatalf("got pushed manifest digest %s, want %s", result.Manifest.Digest, root.Digest)
	}

	resolved, err := client.Resolve(context.Background(), reference)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}

	if resolved.Digest != root.Digest {
		t.Fatalf("got resolved digest %s, want %s", resolved.Digest, root.Digest)
	}
}

func TestClientResolveNonexistentTagIsNotFound(t *testing.T) {
	addr := startTestRegistry(t)
	reference := fmt.Sprintf("%s/test:does-not-exist", addr)

	client := NewClient(ClientOptions{PlainHTTP: true})

	_, err := client.Resolve(context.Background(), reference)
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected errors.Is(err, ErrNotFound), got %v", err)
	}
}

func TestClientResolveRejectsBareRepository(t *testing.T) {
	client := NewClient(ClientOptions{PlainHTTP: true})

	_, err := client.Resolve(context.Background(), "example.registry/repo")
	if !errors.Is(err, ErrInvalid) {
		t.Fatalf("expected errors.Is(err, ErrInvalid), got %v", err)
	}
}

func TestClientPushRejectsBareRepository(t *testing.T) {
	source := buildTestArtifact(t)
	client := NewClient(ClientOptions{PlainHTTP: true})

	_, err := client.Push(context.Background(), source, "example.registry/repo")
	if !errors.Is(err, ErrInvalid) {
		t.Fatalf("expected errors.Is(err, ErrInvalid), got %v", err)
	}
}

func TestCredentialOptionsResolveSecretProviders(t *testing.T) {
	client := NewClient(ClientOptions{
		Credentials: CredentialOptions{
			Username: "alice",
			Password: func(context.Context) (string, error) { return "hunter2", nil },
			Token:    func(context.Context) (string, error) { return "refresh-token", nil },
		},
	})

	opts, err := client.repositoryOptions(context.Background())
	if err != nil {
		t.Fatalf("repositoryOptions: %v", err)
	}

	if opts.Username != "alice" || opts.Password != "hunter2" || opts.Token != "refresh-token" {
		t.Fatalf("unexpected resolved options: %+v", opts)
	}
}

func TestCredentialOptionsPropagateProviderError(t *testing.T) {
	boom := errors.New("prompt failed")

	client := NewClient(ClientOptions{
		Credentials: CredentialOptions{
			Password: func(context.Context) (string, error) { return "", boom },
		},
	})

	if _, err := client.repositoryOptions(context.Background()); !errors.Is(err, boom) {
		t.Fatalf("expected errors.Is(err, boom), got %v", err)
	}
}
