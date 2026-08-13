// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: Copyright 2026 WoozyMasta
// Source: github.com/ocidoc/ocidoc-go

package spec

// Media types for the v1beta OCIDoc artifact.
// The root manifest itself uses the standard OCI image manifest media type (ocispec.MediaTypeImageManifest);
// OCIDoc identity comes from ArtifactType, not from a custom manifest media type.
const (
	// ArtifactType is the manifest artifactType for an OCIDoc documentation artifact.
	ArtifactType = "application/vnd.ocidoc.documentation.v1beta"

	// ConfigMediaType is the media type of the OCIDoc artifact config blob.
	ConfigMediaType = "application/vnd.ocidoc.documentation.config.v1beta+json"

	// ComponentLayerGzip is the media type of a gzip-compressed component tar layer.
	ComponentLayerGzip = "application/vnd.ocidoc.documentation.component.v1beta.tar+gzip"

	// ComponentLayerZstd is the media type of a zstd-compressed component tar layer.
	ComponentLayerZstd = "application/vnd.ocidoc.documentation.component.v1beta.tar+zstd"
)

// CompressionType selects the component layer compression algorithm.
type CompressionType string

// Supported component layer compression algorithms.
const (
	// CompressionGzip stores component layers as gzip-compressed tar archives.
	CompressionGzip CompressionType = "gzip"

	// CompressionZstd stores component layers as Zstandard-compressed tar archives.
	CompressionZstd CompressionType = "zstd"
)

// MediaType returns the component layer media type for c.
func (c CompressionType) MediaType() (string, error) {
	switch c {
	case CompressionGzip:
		return ComponentLayerGzip, nil

	case CompressionZstd:
		return ComponentLayerZstd, nil

	default:
		return "", &ValidationError{Code: CodeUnsupportedCompression, Message: "unsupported compression type: " + string(c)}
	}
}
