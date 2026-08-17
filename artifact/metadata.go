// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: Copyright 2026 WoozyMasta
// Source: github.com/ocidoc/ocidoc-go

package artifact

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"reflect"

	ocispec "github.com/opencontainers/image-spec/specs-go/v1"

	"github.com/ocidoc/ocidoc-go/internal/ociblob"
	"github.com/ocidoc/ocidoc-go/spec"
)

// ValidateMetadata checks the metadata trust boundary of an OCIDoc Reader.
// It validates the root and config blobs, confirms their parsed values match the Reader methods,
// and checks that component descriptors match the manifest.
// Component content is not read; transfer operations verify it while streaming.
func ValidateMetadata(ctx context.Context, r Reader) error {
	root, err := r.Root(ctx)
	if err != nil {
		return err
	}
	if err := validateMetadataDescriptor(root, "root manifest"); err != nil {
		return err
	}

	rootData, err := readMetadataBlob(ctx, r, root)
	if err != nil {
		return fmt.Errorf("read root manifest: %w", err)
	}
	var rawManifest ocispec.Manifest
	if err := json.Unmarshal(rootData, &rawManifest); err != nil {
		return fmt.Errorf("parse root manifest: %w", err)
	}

	manifest, err := r.Manifest(ctx)
	if err != nil {
		return err
	}
	if !reflect.DeepEqual(&rawManifest, manifest) {
		return fmt.Errorf("%w: Reader.Manifest does not match the root manifest blob", spec.ErrInvalid)
	}
	if err := validateManifestDescriptors(&rawManifest); err != nil {
		return err
	}

	configData, err := readMetadataBlob(ctx, r, rawManifest.Config)
	if err != nil {
		return fmt.Errorf("read artifact config: %w", err)
	}
	var rawConfig spec.ArtifactConfig
	if err := json.Unmarshal(configData, &rawConfig); err != nil {
		return fmt.Errorf("parse artifact config: %w", err)
	}
	if err := spec.ValidateArtifactConfig(&rawConfig); err != nil {
		return fmt.Errorf("artifact config: %w", err)
	}

	config, err := r.Config(ctx)
	if err != nil {
		return err
	}
	if !reflect.DeepEqual(&rawConfig, config) {
		return fmt.Errorf("%w: Reader.Config does not match the config blob", spec.ErrInvalid)
	}

	components, err := r.Components(ctx)
	if err != nil {
		return err
	}
	if err := validateReaderComponents(rawManifest, components); err != nil {
		return err
	}

	result, err := Verify(ctx, r, VerifyOptions{MetadataOnly: true})
	if err != nil {
		return err
	}
	if !result.Valid {
		return fmt.Errorf("%w: %s", spec.ErrInvalid, result.Issues[0].Message)
	}

	return nil
}

// readMetadataBlob reads and independently verifies one bounded metadata blob.
func readMetadataBlob(ctx context.Context, r Reader, desc ocispec.Descriptor) ([]byte, error) {
	if desc.Size < 0 || desc.Size > ociblob.MaxMetadataSize {
		return nil, fmt.Errorf("%w: metadata blob size %d is outside the allowed range", spec.ErrInvalid, desc.Size)
	}

	rc, err := r.OpenBlob(ctx, desc)
	if err != nil {
		return nil, err
	}
	data, readErr := io.ReadAll(io.LimitReader(rc, ociblob.MaxMetadataSize+1))
	closeErr := rc.Close()
	if readErr != nil {
		return nil, readErr
	}
	if closeErr != nil {
		return nil, closeErr
	}
	if int64(len(data)) > ociblob.MaxMetadataSize {
		return nil, fmt.Errorf("%w: metadata blob exceeds limit %d", spec.ErrInvalid, ociblob.MaxMetadataSize)
	}
	if err := ociblob.Verify(desc, data); err != nil {
		return nil, fmt.Errorf("%w: %v", spec.ErrVerification, err)
	}

	return data, nil
}

// validateMetadataDescriptor validates one descriptor before it is trusted.
func validateMetadataDescriptor(desc ocispec.Descriptor, name string) error {
	if err := ociblob.Validate(desc); err != nil {
		return fmt.Errorf("%w: %s: %v", spec.ErrInvalid, name, err)
	}

	return nil
}

// validateReaderComponents confirms Reader.Components is the manifest's layer set.
func validateReaderComponents(manifest ocispec.Manifest, components []ComponentDescriptor) error {
	expected := make(map[spec.ComponentType]ocispec.Descriptor, len(manifest.Layers))
	for _, layer := range manifest.Layers {
		componentType, ok := layer.Annotations[spec.AnnotationComponentType]
		if !ok {
			return fmt.Errorf("%w: layer %s is missing %s annotation",
				spec.ErrInvalid, layer.Digest, spec.AnnotationComponentType)
		}
		name := spec.ComponentType(componentType)
		if _, exists := expected[name]; exists {
			return fmt.Errorf("%w: component %q appears more than once", spec.ErrInvalid, name)
		}
		expected[name] = layer
	}

	if len(components) != len(expected) {
		return fmt.Errorf("%w: Reader.Components returned %d components, want %d",
			spec.ErrInvalid, len(components), len(expected))
	}
	for _, component := range components {
		want, ok := expected[component.Type]
		if !ok || !reflect.DeepEqual(want, component.Descriptor) {
			return fmt.Errorf("%w: Reader.Components does not match component %q in the manifest",
				spec.ErrInvalid, component.Type)
		}
	}

	return nil
}
