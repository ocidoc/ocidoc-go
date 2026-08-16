// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: Copyright 2026 WoozyMasta
// Source: github.com/ocidoc/ocidoc-go

package store

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/opencontainers/go-digest"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"

	"github.com/ocidoc/ocidoc-go/artifact"
	"github.com/ocidoc/ocidoc-go/internal/ociblob"
)

// Verification describes the structural state of a local store.
type Verification struct {
	// Valid reports whether all indexed documents and catalog references passed verification.
	Valid bool

	// Documents is the number of root manifests found in the OCI index.
	Documents int

	// Issues contains structural or catalog problems found during verification.
	Issues []string
}

// PruneResult describes a local garbage-collection operation.
type PruneResult struct {
	// DryRun reports whether no content was deleted.
	DryRun bool `json:"dryRun"`

	// BlobCount is the number of unreachable blobs found.
	BlobCount int `json:"blobCount"`

	// BlobBytes is the total size of unreachable blobs found.
	BlobBytes int64 `json:"blobBytes"`
}

// Verify checks indexed OCIDoc documents and catalog references.
// When repair is true, the catalog is rebuilt from valid OCI manifests while
// preserving observations that still belong to the same manifest.
// When metadataOnly is true, component blobs are not opened during verification.
func (s *Store) Verify(ctx context.Context, repair, metadataOnly bool) (Verification, error) {
	var result Verification
	err := s.withCatalogLock(ctx, func() error {
		index, err := s.readIndex()
		if err != nil {
			return err
		}
		catalog, err := s.loadCatalog()
		if err != nil {
			return err
		}

		result.Valid = true
		result.Documents = len(index.Manifests)
		seen := make(map[digest.Digest]bool, len(index.Manifests))
		repaired := &catalogFile{Version: catalogVersion, Documents: make(map[digest.Digest]documentRecord)}

		for _, root := range index.Manifests {
			reader, openErr := s.OpenDocument(ctx, root.Digest)
			if openErr != nil {
				result.addIssue("open %s: %v", root.Digest, openErr)
				continue
			}

			verification, verifyErr := artifact.Verify(ctx, reader, artifact.VerifyOptions{MetadataOnly: metadataOnly})
			closeErr := reader.Close()
			if verifyErr != nil {
				result.addIssue("verify %s: %v", root.Digest, verifyErr)
				continue
			}

			if closeErr != nil {
				result.addIssue("close %s: %v", root.Digest, closeErr)
			}

			if !verification.Valid {
				for _, issue := range verification.Issues {
					result.addIssue("verify %s: %s", root.Digest, issue.Message)
				}
				continue
			}

			seen[root.Digest] = true
			if _, ok := catalog.Documents[root.Digest]; !ok {
				result.addIssue("catalog is missing %s", root.Digest)
			}
			if repair {
				repaired.Documents[root.Digest] = s.repairedRecord(ctx, root.Digest, catalog.Documents[root.Digest])
			}
		}

		for manifest := range catalog.Documents {
			if !seen[manifest] {
				result.addIssue("catalog references missing root %s", manifest)
			}
		}

		if repair {
			if err := s.saveCatalog(repaired); err != nil {
				return fmt.Errorf("repair catalog: %w", err)
			}
		}

		return nil
	})
	if err != nil {
		return Verification{}, err
	}

	return result, nil
}

// Prune removes unreachable OCI blobs. With dryRun, it only reports what would be removed.
func (s *Store) Prune(ctx context.Context, dryRun bool) (PruneResult, error) {
	var result PruneResult
	err := s.withCatalogLock(ctx, func() error {
		index, err := s.readIndex()
		if err != nil {
			return err
		}

		reachable, err := s.reachableBlobs(ctx, index)
		if err != nil {
			return err
		}

		result.DryRun = dryRun
		if err := s.measureUnreachable(reachable, &result); err != nil {
			return err
		}

		if !dryRun {
			if err := s.oci.GC(ctx); err != nil {
				return fmt.Errorf("garbage-collect store: %w", err)
			}
		}

		return nil
	})

	if err != nil {
		return PruneResult{}, err
	}

	return result, nil
}

// addIssue records a verification failure and marks the result invalid.
func (v *Verification) addIssue(format string, args ...any) {
	v.Valid = false
	v.Issues = append(v.Issues, fmt.Sprintf(format, args...))
}

// readIndex loads the store's authoritative OCI image index.
func (s *Store) readIndex() (ocispec.Index, error) {
	data, err := os.ReadFile(filepath.Join(s.root, ocispec.ImageIndexFile))
	if err != nil {
		return ocispec.Index{}, fmt.Errorf("read %s: %w", ocispec.ImageIndexFile, err)
	}

	var index ocispec.Index
	if err := json.Unmarshal(data, &index); err != nil {
		return ocispec.Index{}, fmt.Errorf("parse %s: %w", ocispec.ImageIndexFile, err)
	}

	return index, nil
}

// repairedRecord derives manifest-owned catalog fields
// while preserving observations from the previous catalog record.
func (s *Store) repairedRecord(ctx context.Context, manifest digest.Digest, previous documentRecord) documentRecord {
	reader, err := s.OpenDocument(ctx, manifest)
	if err != nil {
		return previous
	}

	defer reader.Close() //nolint:errcheck // repair result is best effort after verification.
	value, err := reader.Manifest(ctx)
	if err != nil {
		return previous
	}

	previous.Subject = ""
	if value.Subject != nil {
		previous.Subject = value.Subject.Digest
	}

	return previous
}

// reachableBlobs returns the manifest,
// config and component blobs referenced by the roots in the OCI index.
func (s *Store) reachableBlobs(ctx context.Context, index ocispec.Index) (map[digest.Digest]bool, error) {
	reachable := make(map[digest.Digest]bool)
	for _, root := range index.Manifests {
		reachable[root.Digest] = true
		data, err := s.readBlob(ctx, root)
		if err != nil {
			return nil, fmt.Errorf("read root %s: %w", root.Digest, err)
		}

		var manifest ocispec.Manifest
		if err := json.Unmarshal(data, &manifest); err != nil {
			return nil, fmt.Errorf("parse manifest %s: %w", root.Digest, err)
		}

		reachable[manifest.Config.Digest] = true
		for _, layer := range manifest.Layers {
			reachable[layer.Digest] = true
		}
	}

	return reachable, nil
}

// readBlob fetches and verifies one descriptor from the local OCI store.
func (s *Store) readBlob(ctx context.Context, descriptor ocispec.Descriptor) ([]byte, error) {
	if err := ociblob.Validate(descriptor); err != nil {
		return nil, err
	}

	rc, err := s.oci.Fetch(ctx, descriptor)
	if err != nil {
		return nil, err
	}
	defer rc.Close() //nolint:errcheck // read result determines success.

	data, err := io.ReadAll(rc)
	if err != nil {
		return nil, err
	}

	if err := ociblob.Verify(descriptor, data); err != nil {
		return nil, err
	}

	return data, nil
}

// measureUnreachable counts blob files not reachable from the OCI index.
func (s *Store) measureUnreachable(reachable map[digest.Digest]bool, result *PruneResult) error {
	return filepath.WalkDir(filepath.Join(s.root, "blobs"), func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}

		digestValue, err := digest.Parse(filepath.Base(filepath.Dir(path)) + ":" + filepath.Base(path))
		if err != nil {
			return fmt.Errorf("parse blob path %q: %w", path, err)
		}
		if reachable[digestValue] {
			return nil
		}

		info, err := entry.Info()
		if err != nil {
			return err
		}

		result.BlobCount++
		result.BlobBytes += info.Size()

		return nil
	})
}
