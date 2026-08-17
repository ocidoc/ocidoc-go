// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: Copyright 2026 WoozyMasta
// Source: github.com/ocidoc/ocidoc-go

package store

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/gofrs/flock"
	"oras.land/oras-go/v2/content/oci"
)

// Store is a persistent local OCIDoc content-addressed store:
// an OCI Image Layout (blob storage, delegated to oras-go's content/oci.Store)
// plus an OCIDoc-local catalog (store.json).
type Store struct {
	oci  *oci.Store
	lock *flock.Flock
	root string
}

// Open opens (creating if necessary) a local OCIDoc store rooted at path:
// path's directory tree, the underlying OCI Image Layout
// ("oci-layout", "index.json", "blobs/"),
// and the store's own "locks/" and "tmp/" directories are all created if they do not already exist.
func Open(path string) (*Store, error) {
	root, err := filepath.Abs(path)
	if err != nil {
		return nil, fmt.Errorf("resolve store path %q: %w", path, err)
	}

	locksDir := filepath.Join(root, "locks")
	if err := os.MkdirAll(locksDir, 0o750); err != nil {
		return nil, fmt.Errorf("create store locks directory: %w", err)
	}

	if err := os.MkdirAll(filepath.Join(root, "tmp"), 0o750); err != nil {
		return nil, fmt.Errorf("create store tmp directory: %w", err)
	}

	ociStore, err := oci.New(root)
	if err != nil {
		return nil, fmt.Errorf("open OCI layout at %s: %w", root, err)
	}
	// Keep deletion separate from garbage collection. Remove forgets one root;
	// Prune is the explicit operation that reclaims blobs no longer reachable from the store index.
	ociStore.AutoGC = false

	return &Store{
		root: root,
		oci:  ociStore,
		lock: flock.New(filepath.Join(locksDir, "catalog.lock")),
	}, nil
}

// Root returns the store's absolute root path.
func (s *Store) Root() string {
	return s.root
}

// withCatalogLock runs fn while holding the store's exclusive catalog lock,
// honoring ctx's cancellation while waiting for it.
// Catalog mutation is the one operation this package requires cross-process exclusivity for:
// blob writes are already safe under oci.Store's own ingest-to-temp-file-then-verify-then-rename sequence,
// so an in-process mutex alone would be insufficient only for the catalog,
// not the OCI content store, which oras-go already gets right.
//
// TryLockContext reports a context that became done
// before the lock was acquired by returning ctx.Err() itself as its error
// (not locked=false, err=nil), so that is the case classified as ErrLocked here;
// any other error is a genuine lock-file I/O failure, left generically wrapped.
func (s *Store) withCatalogLock(ctx context.Context, fn func() error) error {
	locked, err := s.lock.TryLockContext(ctx, 50*time.Millisecond)
	switch {
	case errors.Is(err, context.DeadlineExceeded), errors.Is(err, context.Canceled):
		return fmt.Errorf("%w: %w", ErrLocked, err)

	case err != nil:
		return fmt.Errorf("acquire store catalog lock: %w", err)

	case !locked:
		return fmt.Errorf("%w: acquire store catalog lock", ErrLocked)
	}
	//nolint:errcheck // best-effort unlock; a stuck lock is the real problem, not this return value.
	defer s.lock.Unlock()

	return fn()
}
