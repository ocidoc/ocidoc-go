// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: Copyright 2026 WoozyMasta
// Source: github.com/ocidoc/ocidoc-go

package store

import (
	"context"
	"fmt"

	"github.com/opencontainers/go-digest"
	"oras.land/oras-go/v2/errdef"

	"github.com/ocidoc/ocidoc-go/internal/ociblob"
)

// Remove forgets manifest in the catalog and deletes its root manifest from the OCI store.
// Shared config and component blobs remain reachable from other manifests;
// unreachable content is removed by the OCI store's graph GC.
func (s *Store) Remove(ctx context.Context, manifest digest.Digest) error {
	if err := ociblob.ValidateDigest(manifest); err != nil {
		return fmt.Errorf("%w: manifest: %v", ErrInvalid, err)
	}

	return s.withCatalogLock(ctx, func() error {
		root, err := s.rootDescriptor(manifest)
		if err != nil {
			return err
		}

		cat, err := s.loadCatalog()
		if err != nil {
			return err
		}
		if _, ok := cat.Documents[manifest]; !ok {
			return fmt.Errorf("%w: manifest %s", errdef.ErrNotFound, manifest)
		}

		delete(cat.Documents, manifest)
		if err := s.saveCatalog(cat); err != nil {
			return err
		}
		if err := s.oci.Delete(ctx, root); err != nil {
			return fmt.Errorf("delete manifest %s: %w", manifest, err)
		}

		return nil
	})
}
