// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: Copyright 2026 WoozyMasta
// Source: github.com/ocidoc/ocidoc-go

package artifact

import (
	"context"
	"fmt"
	"io"
	"strconv"
	"time"

	ocispec "github.com/opencontainers/image-spec/specs-go/v1"

	"github.com/ocidoc/ocidoc-go/internal/archive"
	"github.com/ocidoc/ocidoc-go/internal/compression"
	"github.com/ocidoc/ocidoc-go/internal/digestio"
	"github.com/ocidoc/ocidoc-go/internal/sourcepath"
	"github.com/ocidoc/ocidoc-go/spec"
)

// buildComponentBlob builds componentType's compressed tar blob,
// streaming it to dst, and returns the resulting OCI descriptor
// with the managed org.ocidoc.component.* annotations.
//
// paths are bundle paths (relative, POSIX) read from under root;
// entrypoint is the component's entrypoint bundle path, or "" for none.
func buildComponentBlob(
	ctx context.Context,
	dst io.Writer,
	root string,
	componentType spec.ComponentType,
	paths []string,
	entrypoint string,
	compressionType spec.CompressionType,
	level int,
	modTime time.Time,
) (ocispec.Descriptor, error) {
	mediaType, err := compressionType.MediaType()
	if err != nil {
		return ocispec.Descriptor{}, err
	}

	entries := make([]archive.Entry, 0, len(paths))
	resolver, err := sourcepath.New(root)
	if err != nil {
		return ocispec.Descriptor{}, err
	}

	for _, p := range paths {
		resolved, err := resolver.Resolve(p)
		if err != nil {
			return ocispec.Descriptor{}, err
		}
		if resolved.Kind != sourcepath.KindFile {
			return ocispec.Descriptor{}, fmt.Errorf("%w: planned source %q is no longer a regular file", spec.ErrInvalid, p)
		}
		entries = append(entries, archive.Entry{
			BundlePath: p,
			SourcePath: resolved.File.SourcePath,
		})
	}

	counted := digestio.NewWriter(dst)

	compressed, err := compression.NewWriter(counted, compressionType, level, modTime)
	if err != nil {
		return ocispec.Descriptor{}, fmt.Errorf("component %q: %w", componentType, err)
	}

	tarInfo, err := archive.BuildTar(ctx, compressed, entries, modTime)
	if err != nil {
		return ocispec.Descriptor{}, fmt.Errorf("component %q: build tar: %w", componentType, err)
	}

	if err := compressed.Close(); err != nil {
		return ocispec.Descriptor{}, fmt.Errorf("component %q: close compressor: %w", componentType, err)
	}

	annotations := map[string]string{
		spec.AnnotationComponentType:             string(componentType),
		spec.AnnotationComponentFileCount:        strconv.Itoa(tarInfo.FileCount),
		spec.AnnotationComponentUncompressedSize: strconv.FormatInt(tarInfo.UncompressedSize, 10),
	}

	if entrypoint != "" {
		annotations[spec.AnnotationComponentEntrypoint] = entrypoint
	}

	return ocispec.Descriptor{
		MediaType:   mediaType,
		Digest:      counted.Digest(),
		Size:        counted.Size(),
		Annotations: annotations,
	}, nil
}
