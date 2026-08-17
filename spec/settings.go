// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: Copyright 2026 WoozyMasta
// Source: github.com/ocidoc/ocidoc-go

package spec

// Fixed effective defaults, stable across releases:
// changing any of these would silently change the behavior
// of an existing build config that omits the corresponding field,
// so they must not vary by code version, schema version or environment.
const (
	// DefaultStrict is the effective strict-mode value when it is omitted.
	DefaultStrict = false
	// DefaultCompressionType is the effective compressor when it is omitted.
	DefaultCompressionType = CompressionGzip
	// DefaultCompressionLevel is the effective compressor level when it is omitted.
	DefaultCompressionLevel = 6
	// MaxGzipCompressionLevel is the highest supported gzip level.
	MaxGzipCompressionLevel = 9
	// MaxZstdCompressionLevel is the highest supported zstd level in the config scale.
	MaxZstdCompressionLevel = 19
	// MaxCompressionLevel is the schema-wide upper bound across supported compressors.
	MaxCompressionLevel = MaxZstdCompressionLevel
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
		compressionType := settings.Compression.Type
		if compressionType == "" {
			compressionType = DefaultCompressionType
		}
		resolved.Compression.Type = compressionType

		if settings.Compression.Level != nil {
			resolved.Compression.Level = *settings.Compression.Level
		}

		switch resolved.Compression.Type {
		case CompressionGzip:
			if resolved.Compression.Level > MaxGzipCompressionLevel {
				resolved.Compression.Level = MaxGzipCompressionLevel
			}
		case CompressionZstd:
			if resolved.Compression.Level > MaxZstdCompressionLevel {
				resolved.Compression.Level = MaxZstdCompressionLevel
			}
		}
	}

	return resolved
}

// ResolveDocument applies the document identity defaults:
// id defaults to DefaultDocumentID;
// variant has no default and stays omitted (empty) when not set.
func ResolveDocument(document DocumentSettings) DocumentSettings {
	if document.ID == "" {
		document.ID = DefaultDocumentID
	}

	return document
}
