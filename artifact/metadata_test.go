// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: Copyright 2026 WoozyMasta
// Source: github.com/ocidoc/ocidoc-go

package artifact

import (
	"errors"
	"testing"

	"github.com/ocidoc/ocidoc-go/spec"
)

func TestValidateMetadataRejectsReaderManifestMismatch(t *testing.T) {
	layoutDir, _ := buildTestLayout(t)
	reader, err := OpenLayout(layoutDir)
	if err != nil {
		t.Fatalf("OpenLayout: %v", err)
	}

	manifest, err := reader.Manifest(t.Context())
	if err != nil {
		t.Fatalf("Manifest: %v", err)
	}
	invalid := *manifest
	invalid.ArtifactType = "application/example.invalid"

	err = ValidateMetadata(t.Context(), &overrideReader{Reader: reader, manifest: &invalid})
	if err == nil {
		t.Fatal("expected ValidateMetadata to reject a manifest different from the raw root blob")
	}
	if !errors.Is(err, spec.ErrInvalid) {
		t.Fatalf("expected invalid artifact error, got %v", err)
	}
}
