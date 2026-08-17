// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: Copyright 2026 WoozyMasta
// Source: github.com/ocidoc/ocidoc-go

// Package ociclone copies OCI metadata before it crosses a public Reader boundary.
package ociclone

import (
	"maps"

	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
)

// Descriptor returns a deep copy of an OCI descriptor.
func Descriptor(value ocispec.Descriptor) ocispec.Descriptor {
	value.URLs = append([]string(nil), value.URLs...)
	value.Annotations = cloneStrings(value.Annotations)
	value.Data = append([]byte(nil), value.Data...)
	return value
}

// Manifest returns a deep copy of an OCI manifest and all mutable metadata it owns.
func Manifest(value *ocispec.Manifest) *ocispec.Manifest {
	if value == nil {
		return nil
	}

	clone := *value
	clone.Config = Descriptor(value.Config)
	clone.Layers = make([]ocispec.Descriptor, len(value.Layers))
	for i, layer := range value.Layers {
		clone.Layers[i] = Descriptor(layer)
	}
	if value.Subject != nil {
		subject := Descriptor(*value.Subject)
		clone.Subject = &subject
	}
	clone.Annotations = cloneStrings(value.Annotations)
	return &clone
}

// cloneStrings returns a copy of a string map, preserving nil maps.
func cloneStrings(values map[string]string) map[string]string {
	if values == nil {
		return nil
	}

	clone := make(map[string]string, len(values))
	maps.Copy(clone, values)
	return clone
}
