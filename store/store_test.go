// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: Copyright 2026 WoozyMasta
// Source: github.com/ocidoc/ocidoc-go

package store

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/gofrs/flock"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"

	"github.com/ocidoc/ocidoc-go/artifact"
	"github.com/ocidoc/ocidoc-go/internal/testfixture"
)

// buildTestArtifact builds a minimal local OCI Image Layout and returns
// a Reader open on it, closing it automatically at test cleanup.
func buildTestArtifact(t *testing.T, readme string) artifact.Reader {
	return testfixture.BuildArtifact(t, readme)
}

func TestOpenCreatesStoreLayout(t *testing.T) {
	root := filepath.Join(t.TempDir(), "store")

	s, err := Open(root)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}

	if s.Root() != root {
		t.Fatalf("got root %q, want %q", s.Root(), root)
	}

	for _, want := range []string{"oci-layout", "index.json", "locks", "tmp"} {
		if _, err := os.Stat(filepath.Join(root, want)); err != nil {
			t.Fatalf("stat %s: %v", want, err)
		}
	}
}

func TestOpenIsIdempotent(t *testing.T) {
	root := filepath.Join(t.TempDir(), "store")

	if _, err := Open(root); err != nil {
		t.Fatalf("first Open: %v", err)
	}

	if _, err := Open(root); err != nil {
		t.Fatalf("second Open: %v", err)
	}
}

func TestDocumentsEmptyOnFreshStore(t *testing.T) {
	s, err := Open(t.TempDir())
	if err != nil {
		t.Fatalf("Open: %v", err)
	}

	docs, err := s.Documents()
	if err != nil {
		t.Fatalf("Documents: %v", err)
	}

	if len(docs) != 0 {
		t.Fatalf("got %d documents, want 0", len(docs))
	}
}

func TestDocumentsRejectsMalformedCatalog(t *testing.T) {
	root := t.TempDir()

	s, err := Open(root)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}

	if err := os.WriteFile(filepath.Join(root, catalogFileName), []byte("not json"), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	if _, err := s.Documents(); !errors.Is(err, ErrInvalid) {
		t.Fatalf("Documents: got %v, want errors.Is(err, ErrInvalid)", err)
	}
}

func TestCommitFailsWhenCatalogLockedByAnotherHolder(t *testing.T) {
	root := t.TempDir()

	s, err := Open(root)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}

	holder := flock.New(filepath.Join(root, "locks", "catalog.lock"))
	locked, err := holder.TryLock()
	if err != nil || !locked {
		t.Fatalf("TryLock: locked=%v err=%v", locked, err)
	}
	defer holder.Unlock() //nolint:errcheck // test cleanup.

	reader := buildTestArtifact(t, "# hi")

	ctx, cancel := context.WithTimeout(t.Context(), 200*time.Millisecond)
	defer cancel()

	if _, err := s.Commit(ctx, reader, Origin{Source: "build"}); !errors.Is(err, ErrLocked) {
		t.Fatalf("Commit: got %v, want errors.Is(err, ErrLocked)", err)
	}
}

func mustRoot(t *testing.T, r artifact.Reader) ocispec.Descriptor {
	t.Helper()

	desc, err := r.Root(t.Context())
	if err != nil {
		t.Fatalf("Root: %v", err)
	}

	return desc
}
