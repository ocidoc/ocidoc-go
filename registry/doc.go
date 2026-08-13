// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: Copyright 2026 WoozyMasta
// Source: github.com/ocidoc/ocidoc-go

// Package registry publishes and resolves OCIDoc artifacts against an OCI-compliant registry,
// on top of the ORAS-backed adapter in internal/orasrepo.
// It exposes no ORAS types: every public method takes
// and returns only spec/artifact/OCI image-spec types.
//
// Client currently implements standalone Resolve, Open, Push, Pull and Copy,
// attachment publication and discovery, and remove/detach/prune lifecycle operations.
package registry
