// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: Copyright 2026 WoozyMasta
// Source: github.com/ocidoc/ocidoc-go

package store

import (
	"testing"
)

func TestCommitStoresGraphAndCatalog(t *testing.T) {
	s, err := Open(t.TempDir())
	if err != nil {
		t.Fatalf("Open: %v", err)
	}

	source := buildTestArtifact(t, "# hi")
	root := mustRoot(t, source)

	doc, err := s.Commit(t.Context(), source, Origin{Reference: "ghcr.io/example/app:1.0", Source: "build"})
	if err != nil {
		t.Fatalf("Commit: %v", err)
	}

	if doc.Manifest != root.Digest {
		t.Fatalf("got committed manifest %s, want %s", doc.Manifest, root.Digest)
	}

	if doc.Source != "build" {
		t.Fatalf("got source %q, want %q", doc.Source, "build")
	}

	if len(doc.Origins) != 1 || doc.Origins[0] != "ghcr.io/example/app:1.0" {
		t.Fatalf("unexpected origins: %+v", doc.Origins)
	}

	if doc.Subject != "" {
		t.Fatalf("expected no subject for a standalone artifact, got %s", doc.Subject)
	}

	exists, err := s.oci.Exists(t.Context(), root)
	if err != nil {
		t.Fatalf("Exists: %v", err)
	}

	if !exists {
		t.Fatal("expected manifest blob to exist in the store after Commit")
	}

	docs, err := s.Documents()
	if err != nil {
		t.Fatalf("Documents: %v", err)
	}

	if len(docs) != 1 || docs[0].Manifest != root.Digest {
		t.Fatalf("unexpected catalog documents: %+v", docs)
	}
}

func TestOpenDocumentReadsCommittedGraph(t *testing.T) {
	s, err := Open(t.TempDir())
	if err != nil {
		t.Fatalf("Open: %v", err)
	}

	committed, err := s.Commit(t.Context(), buildTestArtifact(t, "# stored"), Origin{Source: "build"})
	if err != nil {
		t.Fatalf("Commit: %v", err)
	}

	reader, err := s.OpenDocument(t.Context(), committed.Manifest)
	if err != nil {
		t.Fatalf("OpenDocument: %v", err)
	}
	defer reader.Close() //nolint:errcheck // test reader owns no persistent resource.

	root, err := reader.Root(t.Context())
	if err != nil {
		t.Fatalf("Root: %v", err)
	}
	if root.Digest != committed.Manifest {
		t.Fatalf("got root %s, want %s", root.Digest, committed.Manifest)
	}

	components, err := reader.Components(t.Context())
	if err != nil {
		t.Fatalf("Components: %v", err)
	}
	if len(components) == 0 {
		t.Fatal("expected committed components")
	}
}

func TestRemoveDeletesCatalogEntryAndRoot(t *testing.T) {
	s, err := Open(t.TempDir())
	if err != nil {
		t.Fatalf("Open: %v", err)
	}

	committed, err := s.Commit(t.Context(), buildTestArtifact(t, "# remove"), Origin{Source: "build"})
	if err != nil {
		t.Fatalf("Commit: %v", err)
	}
	if err := s.Remove(t.Context(), committed.Manifest); err != nil {
		t.Fatalf("Remove: %v", err)
	}

	docs, err := s.Documents()
	if err != nil {
		t.Fatalf("Documents: %v", err)
	}
	if len(docs) != 0 {
		t.Fatalf("got %+v, want no catalog documents", docs)
	}
	if _, err := s.OpenDocument(t.Context(), committed.Manifest); err == nil {
		t.Fatal("OpenDocument after Remove succeeded")
	}
}

func TestCommitDeduplicatesIdenticalContent(t *testing.T) {
	s, err := Open(t.TempDir())
	if err != nil {
		t.Fatalf("Open: %v", err)
	}

	first := buildTestArtifact(t, "# hi")
	if _, err := s.Commit(t.Context(), first, Origin{Source: "build"}); err != nil {
		t.Fatalf("first Commit: %v", err)
	}

	second := buildTestArtifact(t, "# hi")
	if _, err := s.Commit(t.Context(), second, Origin{Source: "build"}); err != nil {
		t.Fatalf("second Commit (identical content): %v", err)
	}

	docs, err := s.Documents()
	if err != nil {
		t.Fatalf("Documents: %v", err)
	}

	if len(docs) != 1 {
		t.Fatalf("got %d documents, want 1 (identical content should not duplicate)", len(docs))
	}
}

func TestCommitMergesOriginsForSameDocument(t *testing.T) {
	s, err := Open(t.TempDir())
	if err != nil {
		t.Fatalf("Open: %v", err)
	}

	first := buildTestArtifact(t, "# hi")
	if _, err := s.Commit(t.Context(), first, Origin{Reference: "registry-a/app:1.0", Source: "build"}); err != nil {
		t.Fatalf("first Commit: %v", err)
	}

	second := buildTestArtifact(t, "# hi")
	doc, err := s.Commit(t.Context(), second, Origin{Reference: "registry-b/app:1.0", Source: "pull"})
	if err != nil {
		t.Fatalf("second Commit: %v", err)
	}

	if len(doc.Origins) != 2 {
		t.Fatalf("got origins %+v, want both registry-a and registry-b", doc.Origins)
	}
}

func TestCommitStoresComponentsAndConfig(t *testing.T) {
	s, err := Open(t.TempDir())
	if err != nil {
		t.Fatalf("Open: %v", err)
	}

	source := buildTestArtifact(t, "# hi")

	manifest, err := source.Manifest(t.Context())
	if err != nil {
		t.Fatalf("Manifest: %v", err)
	}

	if _, err := s.Commit(t.Context(), source, Origin{Source: "build"}); err != nil {
		t.Fatalf("Commit: %v", err)
	}

	exists, err := s.oci.Exists(t.Context(), manifest.Config)
	if err != nil {
		t.Fatalf("Exists(config): %v", err)
	}

	if !exists {
		t.Fatal("expected config blob to exist in the store after Commit")
	}

	for _, layer := range manifest.Layers {
		exists, err := s.oci.Exists(t.Context(), layer)
		if err != nil {
			t.Fatalf("Exists(layer %s): %v", layer.Digest, err)
		}

		if !exists {
			t.Fatalf("expected component blob %s to exist in the store after Commit", layer.Digest)
		}
	}
}
