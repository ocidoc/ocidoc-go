// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: Copyright 2026 WoozyMasta
// Source: github.com/ocidoc/ocidoc-go

package store

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/opencontainers/go-digest"

	"github.com/ocidoc/ocidoc-go/internal/atomicfile"
)

// catalogVersion is store.json's own internal schema version,
// unrelated to the OCIDoc artifact format's schemaVersion:
// the two evolve independently, since one describes local implementation state
// and the other a portable, published contract.
const catalogVersion = 1

// catalogFileName is the rebuildable local metadata index stored at the store root.
const catalogFileName = "store.json"

// Document is one locally-known OCIDoc document root and its catalog observations.
// Every field is derived from OCI content the store already has except Origins and UpdatedAt,
// which the catalog is the only place that remembers.
type Document struct {
	// UpdatedAt is when this document was last committed or observed.
	UpdatedAt time.Time

	// Manifest is the root manifest digest.
	Manifest digest.Digest

	// Subject is the attached subject digest, or empty for a standalone document.
	Subject digest.Digest

	// Source identifies how the document entered the store.
	Source string

	// Origins lists known registry references for this manifest.
	Origins []string
}

// documentRecord is Document's store.json shape:
// the manifest digest is the map key in catalogFile,
// so it is not duplicated in the value.
type documentRecord struct {
	// UpdatedAt records when this document was last committed or observed.
	UpdatedAt time.Time `json:"updatedAt,omitzero"`

	// Subject is the attached subject digest, if the document is attached.
	Subject digest.Digest `json:"subject,omitempty"`

	// Source identifies how the document entered the store.
	Source string `json:"source,omitempty"`

	// Origins lists registry references known for the document manifest.
	Origins []string `json:"origins,omitempty"`
}

// catalogFile is store.json's on-disk shape.
type catalogFile struct {
	// Documents maps each root manifest digest to its local catalog record.
	Documents map[digest.Digest]documentRecord `json:"documents"`

	// Version identifies the catalog schema version.
	Version int `json:"version"`
}

// loadCatalog reads store.json, returning an empty (version-stamped)
// catalog if it does not exist yet - a missing catalog is not an error,
// since store.json is never the source of truth and is fully rebuildable
// from OCI content already in the store.
func (s *Store) loadCatalog() (*catalogFile, error) {
	//nolint:gosec // path built from the store's own root, not external input.
	data, err := os.ReadFile(filepath.Join(s.root, catalogFileName))
	if errors.Is(err, os.ErrNotExist) {
		return &catalogFile{Version: catalogVersion, Documents: map[digest.Digest]documentRecord{}}, nil
	}

	if err != nil {
		return nil, fmt.Errorf("read %s: %w", catalogFileName, err)
	}

	var cat catalogFile
	if err := json.Unmarshal(data, &cat); err != nil {
		//nolint:errorlint // ErrInvalid classifies malformed on-disk state; the JSON error is diagnostic only.
		return nil, fmt.Errorf("%w: parse %s: %v", ErrInvalid, catalogFileName, err)
	}

	if cat.Documents == nil {
		cat.Documents = map[digest.Digest]documentRecord{}
	}

	return &cat, nil
}

// saveCatalog writes cat to store.json via a temporary file in the store's own tmp/ directory,
// renamed into place: store.json is never rewritten in place,
// so a crash or a concurrent reader never observes a partially-written catalog.
func (s *Store) saveCatalog(cat *catalogFile) error {
	data, err := json.MarshalIndent(cat, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal %s: %w", catalogFileName, err)
	}

	tmp, err := atomicfile.CreateTemp(filepath.Join(s.root, "tmp"))
	if err != nil {
		return fmt.Errorf("create temp %s: %w", catalogFileName, err)
	}
	defer tmp.Cleanup()

	if _, err := tmp.File().Write(data); err != nil {
		return fmt.Errorf("write temp %s: %w", catalogFileName, err)
	}

	if err := tmp.Rename(filepath.Join(s.root, catalogFileName)); err != nil {
		return fmt.Errorf("finalize %s: %w", catalogFileName, err)
	}

	return nil
}

// Documents returns every locally-known document root, sorted by manifest digest.
func (s *Store) Documents() ([]Document, error) {
	cat, err := s.loadCatalog()
	if err != nil {
		return nil, err
	}

	docs := make([]Document, 0, len(cat.Documents))
	for manifestDigest, rec := range cat.Documents {
		docs = append(docs, Document{
			Manifest:  manifestDigest,
			Subject:   rec.Subject,
			Source:    rec.Source,
			Origins:   rec.Origins,
			UpdatedAt: rec.UpdatedAt,
		})
	}

	sort.Slice(docs, func(i, j int) bool { return docs[i].Manifest < docs[j].Manifest })

	return docs, nil
}
