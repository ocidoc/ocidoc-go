// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: Copyright 2026 WoozyMasta
// Source: github.com/ocidoc/ocidoc-go

package orasrepo

import (
	"context"
	"testing"

	"oras.land/oras-go/v2/registry/remote/auth"
)

func TestResolveCredentialExplicitUsernamePassword(t *testing.T) {
	credFunc := resolveCredential(Options{Username: "alice", Password: "secret"})

	cred, err := credFunc(context.Background(), "registry.example")
	if err != nil {
		t.Fatalf("credFunc: %v", err)
	}

	if cred.Username != "alice" || cred.Password != "secret" {
		t.Fatalf("got %+v, want Username=alice Password=secret", cred)
	}
}

func TestResolveCredentialExplicitApplilesToAnyHost(t *testing.T) {
	credFunc := resolveCredential(Options{Username: "alice", Password: "secret"})

	// Open already binds Options to one specific reference/registry, so
	// the credential must not depend on matching a particular hostname.
	cred, err := credFunc(context.Background(), "some-other-registry.example")
	if err != nil {
		t.Fatalf("credFunc: %v", err)
	}

	if cred.Username != "alice" {
		t.Fatalf("got %+v, want it to apply regardless of hostname", cred)
	}
}

func TestResolveCredentialExplicitTokenMapsToRefreshToken(t *testing.T) {
	credFunc := resolveCredential(Options{Token: "my-token"})

	cred, err := credFunc(context.Background(), "registry.example")
	if err != nil {
		t.Fatalf("credFunc: %v", err)
	}

	if cred.RefreshToken != "my-token" {
		t.Fatalf("got %+v, want RefreshToken=my-token", cred)
	}
}

func TestResolveCredentialNoOptionsIsAnonymous(t *testing.T) {
	credFunc := resolveCredential(Options{})

	cred, err := credFunc(context.Background(), "registry.example")
	if err != nil {
		t.Fatalf("credFunc: %v", err)
	}

	if cred != (auth.Credential{}) {
		t.Fatalf("got %+v, want EmptyCredential (anonymous)", cred)
	}
}
