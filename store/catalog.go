// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: Copyright 2026 WoozyMasta
// Source: github.com/ocidoc/ocidoc-go

package store

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
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

// maxStoreMetadataSize bounds rebuildable store metadata files read from disk.
// The limit protects catalog loading from oversized or malicious metadata
// and is enforced both before opening the file and during the bounded read.
const maxStoreMetadataSize int64 = 16 << 20

var errCatalogFutureVersion = errors.New("catalog version is newer than this implementation")

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
	data, err := readStoreMetadata(filepath.Join(s.root, catalogFileName), catalogFileName)
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

	if cat.Version != catalogVersion {
		if cat.Version > catalogVersion {
			return nil, fmt.Errorf(
				"%w: %w: unsupported %s version %d, want %d",
				ErrInvalid, errCatalogFutureVersion, catalogFileName, cat.Version, catalogVersion,
			)
		}
		return nil, fmt.Errorf(
			"%w: unsupported %s version %d, want %d",
			ErrInvalid, catalogFileName, cat.Version, catalogVersion,
		)
	}

	if cat.Documents == nil {
		cat.Documents = map[digest.Digest]documentRecord{}
	}

	return &cat, nil
}

// readStoreMetadata reads and size-checks one store metadata file.
func readStoreMetadata(path, name string) ([]byte, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("%s is not a regular file", name)
	}
	if info.Size() > maxStoreMetadataSize {
		return nil, fmt.Errorf("%w: %s size %d exceeds limit %d", ErrInvalid, name, info.Size(), maxStoreMetadataSize)
	}

	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close() //nolint:errcheck // read result determines success.

	data, err := io.ReadAll(io.LimitReader(f, maxStoreMetadataSize+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > maxStoreMetadataSize {
		return nil, fmt.Errorf("%w: %s exceeds limit %d", ErrInvalid, name, maxStoreMetadataSize)
	}

	return data, nil
}

// emptyCatalog returns a new empty catalog with the current format version.
func emptyCatalog() *catalogFile {
	return &catalogFile{Version: catalogVersion, Documents: map[digest.Digest]documentRecord{}}
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

// Documents returns every root present in the authoritative OCI index,
// joined with any matching catalog observations. Catalog-only records are ignored.
func (s *Store) Documents() ([]Document, error) {
	index, err := s.readIndex()
	if err != nil {
		return nil, err
	}

	cat, err := s.loadCatalog()
	if err != nil {
		return nil, err
	}

	docs := make([]Document, 0, len(index.Manifests))
	for _, root := range index.Manifests {
		rec := cat.Documents[root.Digest]
		docs = append(docs, Document{
			Manifest:  root.Digest,
			Subject:   rec.Subject,
			Source:    rec.Source,
			Origins:   rec.Origins,
			UpdatedAt: rec.UpdatedAt,
		})
	}

	sort.Slice(docs, func(i, j int) bool { return docs[i].Manifest < docs[j].Manifest })

	return docs, nil
}
