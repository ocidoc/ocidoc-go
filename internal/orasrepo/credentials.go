// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: Copyright 2026 WoozyMasta
// Source: github.com/ocidoc/ocidoc-go

package orasrepo

import (
	"context"

	"oras.land/oras-go/v2/registry/remote/auth"
)

// resolveCredential returns the explicit credential supplied to Open.
// Implicit credentials, including Docker-compatible config files,
// are resolved exclusively by ORAScope in newAuthClient.
func resolveCredential(opts Options) auth.CredentialFunc {
	cred := auth.Credential{
		Username:     opts.Username,
		Password:     opts.Password,
		RefreshToken: opts.Token,
	}

	return func(context.Context, string) (auth.Credential, error) {
		return cred, nil
	}
}
