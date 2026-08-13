// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: Copyright 2026 WoozyMasta
// Source: github.com/ocidoc/ocidoc-go

package spec

// Fixed effective defaults, stable across releases:
// changing any of these would silently change the behavior
// of an existing build config that omits the corresponding field,
// so they must not vary by code version, schema version or environment.
const (
	DefaultStrict           = false
	DefaultCompressionType  = CompressionGzip
	DefaultCompressionLevel = 6
)

// EffectiveSettings is BuildSettings with every default applied:
// unlike BuildSettings, it has no notion of an omitted value,
// so it uses plain (non-pointer) fields.
type EffectiveSettings struct {
	// Compression is the resolved compression type and level.
	Compression EffectiveCompression

	// Strict reports whether planning warnings become errors.
	Strict bool
}

// EffectiveCompression is CompressionSettings with every default applied.
type EffectiveCompression struct {
	// Type selects gzip or zstd component compression.
	Type CompressionType

	// Level is the encoder level for Type.
	Level int
}

// ResolveSettings applies the fixed effective defaults to settings,
// which may be nil (meaning a build config with no "settings" section at all).
func ResolveSettings(settings *BuildSettings) EffectiveSettings {
	resolved := EffectiveSettings{
		Strict: DefaultStrict,
		Compression: EffectiveCompression{
			Type:  DefaultCompressionType,
			Level: DefaultCompressionLevel,
		},
	}

	if settings == nil {
		return resolved
	}

	if settings.Strict != nil {
		resolved.Strict = *settings.Strict
	}

	if settings.Compression != nil {
		resolved.Compression.Type = settings.Compression.Type

		if settings.Compression.Level != nil {
			resolved.Compression.Level = *settings.Compression.Level
		}
	}

	return resolved
}

// ResolveDocument applies the document identity defaults:
// id defaults to DefaultDocumentID;
// language and variant have no default and stay omitted (empty) when not set.
func ResolveDocument(document DocumentSettings) DocumentSettings {
	if document.ID == "" {
		document.ID = DefaultDocumentID
	}

	return document
}
