// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: Copyright 2026 WoozyMasta
// Source: github.com/ocidoc/ocidoc-go

package orasrepo

import "oras.land/oras-go/v2/registry"

// ParseReference splits a full reference into the repository accepted
// by Open and an optional tag or digest.
// A bare repository has an empty tagOrDigest.
func ParseReference(reference string) (repository string, tagOrDigest string, err error) {
	ref, err := registry.ParseReference(reference)
	if err != nil {
		return "", "", mapError(err)
	}

	return ref.Registry + "/" + ref.Repository, ref.Reference, nil
}
