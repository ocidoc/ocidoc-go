// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: Copyright 2026 WoozyMasta
// Source: github.com/ocidoc/ocidoc-go

package artifact

import (
	"archive/tar"
	"context"
	"errors"
	"fmt"
	"io"
	"sort"

	"github.com/ocidoc/ocidoc-go/internal/compression"
	"github.com/ocidoc/ocidoc-go/spec"
)

// FileInfo describes one file inside a component,
// read from its tar header without extracting the file's content.
type FileInfo struct {
	// Component owns the file.
	Component spec.ComponentType

	// Path is the bundle-relative file path.
	Path string

	// Size is the uncompressed file size from its tar header.
	Size int64
}

// ListOptions narrows List to one component.
// The zero value lists every component.
type ListOptions struct {
	// Component restricts listing to one semantic component type.
	Component spec.ComponentType
}

// List reads every component's tar headers
// (via Reader.OpenComponent, which verifies each blob's digest as it is read - see digestVerifyingReadCloser)
// and returns their files, sorted by component and then by path.
// It never writes file content anywhere; see Extract for that.
//
// If opts.Component is set and the artifact has no such component,
// List returns an error matching errors.Is(err, spec.ErrNotFound).
func List(ctx context.Context, r Reader, opts ListOptions) ([]FileInfo, error) {
	components, err := r.Components(ctx)
	if err != nil {
		return nil, err
	}

	if opts.Component != "" {
		components = filterComponents(components, opts.Component)
		if len(components) == 0 {
			return nil, fmt.Errorf("%w: component %q", spec.ErrNotFound, opts.Component)
		}
	}

	var files []FileInfo

	for _, c := range components {
		entries, err := listComponentFiles(ctx, r, c)
		if err != nil {
			return nil, err
		}

		files = append(files, entries...)
	}

	sort.Slice(files, func(i, j int) bool {
		if files[i].Component != files[j].Component {
			return files[i].Component < files[j].Component
		}

		return files[i].Path < files[j].Path
	})

	return files, nil
}

// filterComponents returns the subset of components matching want, if any.
func filterComponents(components []ComponentDescriptor, want spec.ComponentType) []ComponentDescriptor {
	for _, c := range components {
		if c.Type == want {
			return []ComponentDescriptor{c}
		}
	}

	return nil
}

// listComponentFiles opens, decompresses and reads every tar header in component c.
func listComponentFiles(ctx context.Context, r Reader, c ComponentDescriptor) ([]FileInfo, error) {
	rc, _, err := r.OpenComponent(ctx, c.Type)
	if err != nil {
		return nil, err
	}
	//nolint:errcheck // read-only handle; a close error here would not change the result already computed.
	defer rc.Close()

	decompressed, err := compression.NewReader(rc, c.Descriptor.MediaType)
	if err != nil {
		return nil, fmt.Errorf("component %q: %w", c.Type, err)
	}
	//nolint:errcheck // read-only handle; a close error here would not change the result already computed.
	defer decompressed.Close()

	tr := tar.NewReader(decompressed)

	var files []FileInfo

	for {
		header, err := tr.Next()
		if errors.Is(err, io.EOF) {
			break
		}

		if err != nil {
			return nil, fmt.Errorf("component %q: read tar header: %w", c.Type, err)
		}

		if header.Typeflag != tar.TypeReg {
			continue
		}

		files = append(files, FileInfo{Component: c.Type, Path: header.Name, Size: header.Size})
	}

	return files, nil
}
