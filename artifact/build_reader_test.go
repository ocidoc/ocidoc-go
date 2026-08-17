// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: Copyright 2026 WoozyMasta
// Source: github.com/ocidoc/ocidoc-go

package artifact

import (
	"context"
	"testing"
)

func TestBuildReaderReturnsVerifiedGraph(t *testing.T) {
	result, err := BuildReader(context.Background(), BuildReaderOptions{
		Root: newLayoutFixture(t),
	})
	if err != nil {
		t.Fatalf("BuildReader: %v", err)
	}
	defer result.Reader.Close() //nolint:errcheck // test cleanup.

	verification, err := Verify(context.Background(), result.Reader, VerifyOptions{})
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if !verification.Valid {
		t.Fatalf("expected a valid built graph, got issues: %+v", verification.Issues)
	}
	root, err := result.Reader.Root(context.Background())
	if err != nil {
		t.Fatalf("Root: %v", err)
	}
	if root.Digest == "" {
		t.Fatal("expected a manifest descriptor")
	}
}
