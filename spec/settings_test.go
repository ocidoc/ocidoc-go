// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: Copyright 2026 WoozyMasta
// Source: github.com/ocidoc/ocidoc-go

package spec

import "testing"

func TestResolveSettingsNilUsesFixedDefaults(t *testing.T) {
	got := ResolveSettings(nil)

	want := EffectiveSettings{
		Strict:      false,
		Compression: EffectiveCompression{Type: CompressionGzip, Level: 6},
	}

	if got != want {
		t.Fatalf("got %+v, want %+v", got, want)
	}
}

func TestResolveSettingsEmptyStructUsesFixedDefaults(t *testing.T) {
	got := ResolveSettings(&BuildSettings{})

	want := EffectiveSettings{
		Strict:      false,
		Compression: EffectiveCompression{Type: CompressionGzip, Level: 6},
	}

	if got != want {
		t.Fatalf("got %+v, want %+v", got, want)
	}
}

func TestResolveSettingsExplicitStrictOverridesDefault(t *testing.T) {
	strict := true

	got := ResolveSettings(&BuildSettings{Strict: &strict})

	if !got.Strict {
		t.Fatal("expected Strict to be true")
	}
}

func TestResolveSettingsExplicitCompressionType(t *testing.T) {
	got := ResolveSettings(&BuildSettings{
		Compression: &CompressionSettings{Type: CompressionZstd},
	})

	if got.Compression.Type != CompressionZstd {
		t.Fatalf("got type %q, want %q", got.Compression.Type, CompressionZstd)
	}

	// Level was not set explicitly: still defaults to 6, even for zstd.
	if got.Compression.Level != 6 {
		t.Fatalf("got level %d, want 6", got.Compression.Level)
	}
}

func TestResolveSettingsExplicitCompressionLevel(t *testing.T) {
	level := 9

	got := ResolveSettings(&BuildSettings{
		Compression: &CompressionSettings{Type: CompressionGzip, Level: &level},
	})

	if got.Compression.Level != 9 {
		t.Fatalf("got level %d, want 9", got.Compression.Level)
	}
}

func TestResolveSettingsDefaultsEmptyCompressionType(t *testing.T) {
	level := 19
	got := ResolveSettings(&BuildSettings{
		Compression: &CompressionSettings{Level: &level},
	})

	if got.Compression.Type != CompressionGzip {
		t.Fatalf("got type %q, want %q", got.Compression.Type, CompressionGzip)
	}
	if got.Compression.Level != MaxGzipCompressionLevel {
		t.Fatalf("got level %d, want gzip maximum %d", got.Compression.Level, MaxGzipCompressionLevel)
	}
}

func TestResolveSettingsClampsCompressionLevels(t *testing.T) {
	for _, test := range []struct {
		name string
		typ  CompressionType
		want int
	}{
		{name: "gzip", typ: CompressionGzip, want: MaxGzipCompressionLevel},
		{name: "zstd", typ: CompressionZstd, want: MaxZstdCompressionLevel},
	} {
		t.Run(test.name, func(t *testing.T) {
			level := MaxCompressionLevel + 1
			got := ResolveSettings(&BuildSettings{
				Compression: &CompressionSettings{Type: test.typ, Level: &level},
			})
			if got.Compression.Level != test.want {
				t.Fatalf("got level %d, want %d", got.Compression.Level, test.want)
			}
		})
	}
}

func TestResolveDocumentDefaultsID(t *testing.T) {
	got := ResolveDocument(DocumentSettings{})

	if got.ID != DefaultDocumentID {
		t.Fatalf("got ID %q, want %q", got.ID, DefaultDocumentID)
	}

	if got.Variant != "" {
		t.Fatalf("expected variant to stay empty, got %+v", got)
	}
}

func TestResolveDocumentPreservesExplicitValues(t *testing.T) {
	got := ResolveDocument(DocumentSettings{ID: "secondary", Variant: "full"})

	want := DocumentSettings{ID: "secondary", Variant: "full"}
	if got != want {
		t.Fatalf("got %+v, want %+v", got, want)
	}
}
