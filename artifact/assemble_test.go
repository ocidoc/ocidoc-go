// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: Copyright 2026 WoozyMasta
// Source: github.com/ocidoc/ocidoc-go

package artifact

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"reflect"
	"testing"
	"time"

	"github.com/ocidoc/ocidoc-go/spec"
)

func TestAssembleEndToEnd(t *testing.T) {
	root := t.TempDir()
	writeConfigTree(t, root, map[string]string{
		"ocidoc.yaml": `
schemaVersion: v1beta
components:
  documentation:
    - /README.md
  license:
    - /LICENSE
`,
		"README.md": "# hi",
		"LICENSE":   "MIT",
	})

	plan, err := Plan(t.Context(), root, PlanOptions{})
	if err != nil {
		t.Fatalf("Plan: %v", err)
	}

	docBuf := &bytes.Buffer{}
	licenseBuf := &bytes.Buffer{}
	componentBlobs := map[spec.ComponentType]io.Writer{
		spec.ComponentDocumentation: docBuf,
		spec.ComponentLicense:       licenseBuf,
	}

	var configBuf bytes.Buffer

	result, err := Assemble(t.Context(), AssembleOptions{
		Plan:           plan,
		Root:           root,
		ComponentBlobs: componentBlobs,
		ConfigBlob:     &configBuf,
		ModTime:        time.Unix(1700000000, 0).UTC(),
	})
	if err != nil {
		t.Fatalf("Assemble: %v", err)
	}

	if len(result.ComponentDescriptors) != 2 {
		t.Fatalf("got %d component descriptors, want 2", len(result.ComponentDescriptors))
	}

	if got, want := result.ComponentDescriptors[spec.ComponentDocumentation].Size, int64(docBuf.Len()); got != want {
		t.Errorf("documentation descriptor size %d != written bytes %d", got, want)
	}

	if got, want := result.ComponentDescriptors[spec.ComponentLicense].Size, int64(licenseBuf.Len()); got != want {
		t.Errorf("license descriptor size %d != written bytes %d", got, want)
	}

	if result.ConfigDescriptor.Size != int64(configBuf.Len()) {
		t.Fatalf("config descriptor size %d != written bytes %d", result.ConfigDescriptor.Size, configBuf.Len())
	}

	var artifactConfig spec.ArtifactConfig
	if err := json.Unmarshal(configBuf.Bytes(), &artifactConfig); err != nil {
		t.Fatalf("unmarshal config blob: %v", err)
	}

	if err := spec.ValidateArtifactConfig(&artifactConfig); err != nil {
		t.Fatalf("ValidateArtifactConfig: %v", err)
	}

	if !reflect.DeepEqual(result.Manifest.Config, result.ConfigDescriptor) {
		t.Fatalf("manifest.Config %+v != ConfigDescriptor %+v", result.Manifest.Config, result.ConfigDescriptor)
	}

	if len(result.Manifest.Layers) != 2 {
		t.Fatalf("got %d manifest layers, want 2", len(result.Manifest.Layers))
	}
}

func TestAssembleRejectsNilPlan(t *testing.T) {
	_, err := Assemble(t.Context(), AssembleOptions{})
	if err == nil {
		t.Fatal("expected error for a nil Plan")
	}
}

func TestAssembleRejectsCanceledContext(t *testing.T) {
	plan := &BuildPlan{
		Ownership: map[spec.ComponentType][]string{spec.ComponentDocumentation: {"README.md"}},
	}

	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	var buf bytes.Buffer

	_, err := Assemble(ctx, AssembleOptions{
		Plan:           plan,
		ComponentBlobs: map[spec.ComponentType]io.Writer{spec.ComponentDocumentation: &buf},
		ConfigBlob:     &bytes.Buffer{},
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Assemble: got %v, want errors.Is(err, context.Canceled)", err)
	}
	if buf.Len() != 0 {
		t.Fatalf("expected no component bytes written after cancellation, got %d", buf.Len())
	}
}

func TestAssembleRejectsMissingComponentWriter(t *testing.T) {
	plan := &BuildPlan{
		Ownership: map[spec.ComponentType][]string{spec.ComponentDocumentation: {"README.md"}},
	}

	_, err := Assemble(t.Context(), AssembleOptions{
		Plan:           plan,
		ComponentBlobs: map[spec.ComponentType]io.Writer{},
		ConfigBlob:     &bytes.Buffer{},
	})
	if err == nil {
		t.Fatal("expected error for a missing component blob writer")
	}
}
