// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: Copyright 2026 WoozyMasta
// Source: github.com/ocidoc/ocidoc-go

package store

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"time"

	ocispec "github.com/opencontainers/image-spec/specs-go/v1"

	"github.com/ocidoc/ocidoc-go/artifact"
	"github.com/ocidoc/ocidoc-go/internal/ociblob"
)

// Origin describes where a committed document came from,
// for the catalog's origin/source observation fields.
type Origin struct {
	// Reference is the registry reference this document was built for or pulled from,
	// or empty for a plain local build with no known remote counterpart yet.
	Reference string

	// Source is "build", "pull" or "import".
	Source string
}

// Commit copies source's complete graph
// (artifact config, every component blob, and the root manifest)
// into the store, deduplicating against content already present by digest,
// and records a catalog entry for the result.
//
// Commit trusts source's own shape
// (a Reader only ever exposes an already-recognized OCIDoc manifest);
// it re-verifies each blob's digest against its own descriptor before storing it regardless,
// the same defensive check registry.Client.Push
// and Pull both already make at their own trust boundaries.
func (s *Store) Commit(ctx context.Context, source artifact.Reader, origin Origin) (*Document, error) {
	var committed *Document
	if err := s.withCatalogLock(ctx, func() error {
		var err error
		committed, err = s.commit(ctx, source, origin)
		return err
	}); err != nil {
		return nil, err
	}

	return committed, nil
}

// commit writes graph objects, updates the OCI index, then records the root in the catalog.
// The caller must hold the catalog lock for this ordering to stay consistent across processes.
func (s *Store) commit(ctx context.Context, source artifact.Reader, origin Origin) (*Document, error) {
	manifest, err := source.Manifest(ctx)
	if err != nil {
		return nil, err
	}

	root, err := source.Root(ctx)
	if err != nil {
		return nil, err
	}

	if err := s.commitConfig(ctx, source, manifest.Config); err != nil {
		return nil, fmt.Errorf("config: %w", err)
	}

	if err := s.commitComponents(ctx, source); err != nil {
		return nil, err
	}

	if err := s.commitManifest(ctx, *manifest, root); err != nil {
		return nil, err
	}

	doc := &Document{
		Manifest:  root.Digest,
		Source:    origin.Source,
		UpdatedAt: time.Now().UTC(),
	}

	if manifest.Subject != nil {
		doc.Subject = manifest.Subject.Digest
	}

	if origin.Reference != "" {
		doc.Origins = []string{origin.Reference}
	}

	if err := s.recordDocument(doc); err != nil {
		return nil, err
	}

	return doc, nil
}

// commitConfig re-serializes source's artifact config,
// checks it still hashes to expected (the manifest's own record of it), and stores it.
func (s *Store) commitConfig(ctx context.Context, source artifact.Reader, expected ocispec.Descriptor) error {
	cfg, err := source.Config(ctx)
	if err != nil {
		return err
	}

	data, err := json.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("marshal artifact config: %w", err)
	}

	if err := ociblob.Verify(expected, data); err != nil {
		return err
	}

	return s.pushIfMissing(ctx, expected, bytes.NewReader(data))
}

// commitComponents stores every component blob exactly as source streams it,
// unmodified - oci.Store's own ingest sequence already verifies each against
// its descriptor as it is written, so no additional in-memory check is needed here.
func (s *Store) commitComponents(ctx context.Context, source artifact.Reader) error {
	components, err := source.Components(ctx)
	if err != nil {
		return err
	}

	for _, comp := range components {
		if err := s.commitComponent(ctx, source, comp); err != nil {
			return fmt.Errorf("component %q: %w", comp.Type, err)
		}
	}

	return nil
}

func (s *Store) commitComponent(ctx context.Context, source artifact.Reader, comp artifact.ComponentDescriptor) error {
	rc, _, err := source.OpenComponent(ctx, comp.Type)
	if err != nil {
		return err
	}
	//nolint:errcheck // read-only handle; a close error here would not change an already-committed blob.
	defer rc.Close()

	return s.pushIfMissing(ctx, comp.Descriptor, rc)
}

// commitManifest re-serializes manifest, checks it still hashes to root
// (Reader.Root's own record of it), and stores it.
func (s *Store) commitManifest(ctx context.Context, manifest ocispec.Manifest, root ocispec.Descriptor) error {
	data, err := json.Marshal(manifest)
	if err != nil {
		return fmt.Errorf("marshal manifest: %w", err)
	}

	if err := ociblob.Verify(root, data); err != nil {
		return err
	}

	return s.pushIfMissing(ctx, root, bytes.NewReader(data))
}

// pushIfMissing stores desc's content unless it is already present -
// the store's deduplication boundary:
// identical blobs from independent documents exist only once.
func (s *Store) pushIfMissing(ctx context.Context, desc ocispec.Descriptor, content io.Reader) error {
	exists, err := s.oci.Exists(ctx, desc)
	if err != nil {
		return fmt.Errorf("check %s: %w", desc.Digest, err)
	}

	if exists {
		return nil
	}

	if err := s.oci.Push(ctx, desc, content); err != nil {
		return fmt.Errorf("store %s: %w", desc.Digest, err)
	}

	return nil
}

// recordDocument merges doc into the catalog.
// The caller must hold s's exclusive catalog lock:
// an existing record for the same manifest digest keeps
// its accumulated Origins (deduplicated) rather than losing them.
// doc.Origins is updated in place to the merged result,
// so the Document Commit returns to its caller reflects what was actually persisted,
// not just the single origin that call itself contributed.
func (s *Store) recordDocument(doc *Document) error {
	cat, err := s.loadCatalog()
	if err != nil {
		return err
	}

	existing := cat.Documents[doc.Manifest]
	doc.Origins = mergeOrigins(existing.Origins, doc.Origins)

	cat.Documents[doc.Manifest] = documentRecord{
		Subject:   doc.Subject,
		Source:    doc.Source,
		Origins:   doc.Origins,
		UpdatedAt: doc.UpdatedAt,
	}

	return s.saveCatalog(cat)
}

// mergeOrigins appends added to base, skipping any value already present.
func mergeOrigins(base, added []string) []string {
	seen := make(map[string]bool, len(base))
	for _, o := range base {
		seen[o] = true
	}

	merged := base

	for _, o := range added {
		if seen[o] {
			continue
		}

		merged = append(merged, o)
		seen[o] = true
	}

	return merged
}
