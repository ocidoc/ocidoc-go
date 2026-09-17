// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: Copyright 2026 WoozyMasta
// Source: github.com/ocidoc/ocidoc-go

package store

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/gofrs/flock"
	"github.com/opencontainers/go-digest"
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

func TestDocumentsRejectsUnsupportedCatalogVersion(t *testing.T) {
	root := t.TempDir()

	s, err := Open(root)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}

	data, err := json.Marshal(catalogFile{
		Version:   catalogVersion + 1,
		Documents: map[digest.Digest]documentRecord{},
	})
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, catalogFileName), data, 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	if _, err := s.Documents(); !errors.Is(err, ErrInvalid) {
		t.Fatalf("Documents: got %v, want errors.Is(err, ErrInvalid)", err)
	}
}

func TestDocumentsRejectsMissingCatalogVersion(t *testing.T) {
	root := t.TempDir()

	s, err := Open(root)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}

	if err := os.WriteFile(filepath.Join(root, catalogFileName), []byte(`{"documents":{}}`), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	if _, err := s.Documents(); !errors.Is(err, ErrInvalid) {
		t.Fatalf("Documents: got %v, want errors.Is(err, ErrInvalid)", err)
	}
}

func TestDocumentsUsesIndexWhenCatalogIsMissing(t *testing.T) {
	root := t.TempDir()
	s, err := Open(root)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	committed, err := s.Commit(t.Context(), buildTestArtifact(t, "# indexed"), Origin{Source: "build"})
	if err != nil {
		t.Fatalf("Commit: %v", err)
	}
	if err := os.Remove(filepath.Join(root, catalogFileName)); err != nil {
		t.Fatalf("Remove catalog: %v", err)
	}

	docs, err := s.Documents()
	if err != nil {
		t.Fatalf("Documents: %v", err)
	}
	if len(docs) != 1 || docs[0].Manifest != committed.Manifest {
		t.Fatalf("documents from index = %+v", docs)
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

func TestOpenStoresSerializeCommitsAcrossHandles(t *testing.T) {
	root := filepath.Join(t.TempDir(), "store")

	first, err := Open(root)
	if err != nil {
		t.Fatalf("Open first: %v", err)
	}
	second, err := Open(root)
	if err != nil {
		t.Fatalf("Open second: %v", err)
	}

	firstArtifact := buildTestArtifact(t, "# first")
	secondArtifact := buildTestArtifact(t, "# second")
	firstRoot := mustRoot(t, firstArtifact).Digest
	secondRoot := mustRoot(t, secondArtifact).Digest

	if _, err := first.Commit(t.Context(), firstArtifact, Origin{Source: "build"}); err != nil {
		t.Fatalf("first Commit: %v", err)
	}
	if _, err := second.Commit(t.Context(), secondArtifact, Origin{Source: "build"}); err != nil {
		t.Fatalf("second Commit: %v", err)
	}

	index, err := first.readIndex()
	if err != nil {
		t.Fatalf("readIndex: %v", err)
	}
	got := make(map[digest.Digest]bool, len(index.Manifests))
	for _, manifest := range index.Manifests {
		got[manifest.Digest] = true
	}
	if !got[firstRoot] || !got[secondRoot] {
		t.Fatalf("index roots = %v, want %s and %s", got, firstRoot, secondRoot)
	}

	docs, err := first.Documents()
	if err != nil {
		t.Fatalf("Documents: %v", err)
	}
	if len(docs) != 2 {
		t.Fatalf("catalog documents = %d, want 2: %+v", len(docs), docs)
	}
}

func TestVerifyRepairRebuildsMalformedCatalog(t *testing.T) {
	root := t.TempDir()

	s, err := Open(root)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	artifactReader := buildTestArtifact(t, "# repair")
	committed, err := s.Commit(t.Context(), artifactReader, Origin{Source: "build"})
	if err != nil {
		t.Fatalf("Commit: %v", err)
	}

	if err := os.WriteFile(filepath.Join(root, catalogFileName), []byte("not json"), 0o600); err != nil {
		t.Fatalf("WriteFile catalog: %v", err)
	}

	result, err := s.Verify(t.Context(), true, true)
	if err != nil {
		t.Fatalf("Verify repair: %v", err)
	}
	if !result.Valid {
		t.Fatalf("Verify repair is invalid: %+v", result)
	}

	docs, err := s.Documents()
	if err != nil {
		t.Fatalf("Documents after repair: %v", err)
	}
	if len(docs) != 1 || docs[0].Manifest != committed.Manifest {
		t.Fatalf("repaired documents = %+v", docs)
	}
}

func TestStoreSerializesConcurrentCommitsOnOneHandle(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "store"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}

	first := buildTestArtifact(t, "# first")
	second := buildTestArtifact(t, "# second")
	want := []digest.Digest{mustRoot(t, first).Digest, mustRoot(t, second).Digest}

	var wg sync.WaitGroup
	errs := make(chan error, 2)
	for _, source := range []artifact.Reader{first, second} {
		wg.Add(1)
		go func(source artifact.Reader) {
			defer wg.Done()
			_, commitErr := s.Commit(t.Context(), source, Origin{Source: "build"})
			errs <- commitErr
		}(source)
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatalf("concurrent Commit: %v", err)
		}
	}

	index, err := s.readIndex()
	if err != nil {
		t.Fatalf("readIndex: %v", err)
	}
	got := make(map[digest.Digest]bool, len(index.Manifests))
	for _, manifest := range index.Manifests {
		got[manifest.Digest] = true
	}
	for _, root := range want {
		if !got[root] {
			t.Errorf("index is missing committed root %s", root)
		}
	}
}

func TestVerifyRepairDoesNotOverwriteFutureCatalog(t *testing.T) {
	root := t.TempDir()

	s, err := Open(root)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if _, err := s.Commit(t.Context(), buildTestArtifact(t, "# future"), Origin{Source: "build"}); err != nil {
		t.Fatalf("Commit: %v", err)
	}

	data := []byte(`{"version":999,"documents":{}}`)
	path := filepath.Join(root, catalogFileName)
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatalf("WriteFile catalog: %v", err)
	}

	if _, err := s.Verify(t.Context(), true, true); err == nil {
		t.Fatal("Verify repair succeeded for a future catalog version")
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile catalog: %v", err)
	}
	if string(got) != string(data) {
		t.Fatalf("future catalog changed to %q", got)
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
