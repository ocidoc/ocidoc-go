// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: Copyright 2026 WoozyMasta
// Source: github.com/ocidoc/ocidoc-go

package registry

import "context"

// SecretProvider supplies one secret value (a password or token) on demand,
// so a Client never forces a caller to hold a plaintext secret in memory for longer than one request:
// the provider can obtain the value only when Client needs it.
type SecretProvider func(ctx context.Context) (string, error)

// CredentialOptions configures how a Client authenticates to a registry.
// Password and Token are mutually exclusive in practice
// (a registry's token endpoint uses one or the other);
// Client does not reject supplying both,
// it simply passes both through to the same precedence internal/orasrepo already implements.
type CredentialOptions struct {
	// Username is used together with Password for basic authentication.
	Username string

	// Password supplies the basic-auth password when requested.
	Password SecretProvider

	// Token is used as a refresh (identity) token for a registry's OAuth2 token exchange,
	// not as a short-lived access token supplied as-is.
	Token SecretProvider

	// RegistryConfig, if set, is the sole Docker-compatible credential source.
	// Automatic credential discovery is disabled for this Client.
	RegistryConfig string
}

// resolveSecret calls p if non-nil, returning "" for a nil provider
// ("this credential was not supplied") rather than an error.
func resolveSecret(ctx context.Context, p SecretProvider) (string, error) {
	if p == nil {
		return "", nil
	}

	return p(ctx)
}
