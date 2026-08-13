// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: Copyright 2026 WoozyMasta
// Source: github.com/ocidoc/ocidoc-go

package artifact

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/ocidoc/ocidoc-go/spec"
)

func TestOpenComponentDetectsTamperedBlob(t *testing.T) {
	layoutDir, built := buildTestLayout(t)

	desc := built.ComponentDescriptors[spec.ComponentDocumentation]
	blobFile := filepath.Join(layoutDir, "blobs", "sha256", desc.Digest.Encoded())

	original, err := os.ReadFile(blobFile) //nolint:gosec // fixed test path.
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}

	tampered := append([]byte{}, original...)
	tampered[0] ^= 0xFF

	if err := os.WriteFile(blobFile, tampered, 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	reader, err := OpenLayout(layoutDir)
	if err != nil {
		t.Fatalf("OpenLayout: %v", err)
	}

	rc, _, err := reader.OpenComponent(context.Background(), spec.ComponentDocumentation)
	if err != nil {
		t.Fatalf("OpenComponent: %v", err)
	}

	defer rc.Close()

	_, err = io.ReadAll(rc)
	if err == nil {
		t.Fatal("expected a digest verification error while reading a tampered component blob")
	}

	if !errors.Is(err, spec.ErrVerification) {
		t.Fatalf("expected errors.Is(err, spec.ErrVerification), got %v", err)
	}
}

func TestOpenComponentAcceptsUntamperedBlob(t *testing.T) {
	layoutDir, _ := buildTestLayout(t)

	reader, err := OpenLayout(layoutDir)
	if err != nil {
		t.Fatalf("OpenLayout: %v", err)
	}

	rc, _, err := reader.OpenComponent(context.Background(), spec.ComponentDocumentation)
	if err != nil {
		t.Fatalf("OpenComponent: %v", err)
	}

	defer rc.Close()

	if _, err := io.ReadAll(rc); err != nil {
		t.Fatalf("ReadAll: %v", err)
	}
}
