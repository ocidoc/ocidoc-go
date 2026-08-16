// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: Copyright 2026 WoozyMasta
// Source: github.com/ocidoc/ocidoc-go

package store

import (
	"fmt"
	"io/fs"
	"path/filepath"
)

// Usage describes deduplicated OCI blob storage used by the local store.
type Usage struct {
	// BlobCount is the number of content-addressed blobs in the store.
	BlobCount int

	// BlobBytes is the total size in bytes of content-addressed blobs.
	BlobBytes int64
}

// Usage returns the total size of content-addressed blobs in the store.
func (s *Store) Usage() (Usage, error) {
	var usage Usage
	blobsRoot := filepath.Join(s.root, "blobs")
	err := filepath.WalkDir(blobsRoot, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		if entry.IsDir() {
			return nil
		}

		info, err := entry.Info()
		if err != nil {
			return fmt.Errorf("stat blob %q: %w", path, err)
		}

		usage.BlobCount++
		usage.BlobBytes += info.Size()
		return nil
	})
	if err != nil {
		return Usage{}, fmt.Errorf("walk store blobs: %w", err)
	}

	return usage, nil
}
