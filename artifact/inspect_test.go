// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: Copyright 2026 WoozyMasta
// Source: github.com/ocidoc/ocidoc-go

package artifact

import (
	"context"
	"reflect"
	"testing"
)

func TestInspectFull(t *testing.T) {
	layoutDir, built := buildTestLayout(t)

	reader, err := OpenLayout(layoutDir)
	if err != nil {
		t.Fatalf("OpenLayout: %v", err)
	}

	inspection, err := Inspect(context.Background(), reader, InspectOptions{})
	if err != nil {
		t.Fatalf("Inspect: %v", err)
	}

	if !reflect.DeepEqual(inspection.Manifest, built.Manifest) {
		t.Fatalf("got manifest %+v, want %+v", inspection.Manifest, built.Manifest)
	}

	if inspection.Config == nil {
		t.Fatal("expected Config to be populated by default")
	}

	if len(inspection.Components) != len(built.ComponentDescriptors) {
		t.Fatalf("got %d components, want %d", len(inspection.Components), len(built.ComponentDescriptors))
	}
}

func TestInspectManifestOnlySkipsConfig(t *testing.T) {
	layoutDir, built := buildTestLayout(t)

	reader, err := OpenLayout(layoutDir)
	if err != nil {
		t.Fatalf("OpenLayout: %v", err)
	}

	inspection, err := Inspect(context.Background(), reader, InspectOptions{ManifestOnly: true})
	if err != nil {
		t.Fatalf("Inspect: %v", err)
	}

	if inspection.Config != nil {
		t.Fatal("expected Config to be nil in manifest-only mode")
	}

	if len(inspection.Components) != len(built.ComponentDescriptors) {
		t.Fatalf("got %d components, want %d (components are free from the manifest)",
			len(inspection.Components), len(built.ComponentDescriptors))
	}
}
