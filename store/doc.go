// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: Copyright 2026 WoozyMasta
// Source: github.com/ocidoc/ocidoc-go

// Package store implements OCIDoc's persistent local content-addressed store:
// the normal working representation for a locally built or pulled artifact,
// distinct from the portable .ocidoc export/import format.
// The store root is a valid OCI Image Layout
// (blob storage delegated to oras-go's content/oci.Store,
// so this package never reimplements OCI blob addressing)
// plus an OCIDoc-local catalog (store.json) recording document-level observations
// the OCI content alone does not carry, such as where a document was pulled from.
//
// The catalog is an accelerating index, never the source of truth:
// every fact it records is, in principle, re-derivable from the OCI content already committed to the store.
//
// Path resolution is owned by the application layer, not this package;
// Open always takes an already-resolved absolute or relative filesystem path.
package store
