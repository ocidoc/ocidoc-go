// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: Copyright 2026 WoozyMasta
// Source: github.com/ocidoc/ocidoc-go

package artifact

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
)

func ExampleBuildArchive() {
	ctx := context.Background()
	root, err := os.MkdirTemp("", "ocidoc-example-*")
	if err != nil {
		panic(err)
	}
	defer os.RemoveAll(root) //nolint:errcheck // temporary example directory.

	if err := os.WriteFile(filepath.Join(root, "README.md"), []byte("# Example\n"), 0o600); err != nil {
		panic(err)
	}
	if err := os.WriteFile(filepath.Join(root, "ocidoc.yaml"), []byte(
		"schemaVersion: v1beta\ncomponents:\n  documentation:\n    - /README.md\n"), 0o600); err != nil {
		panic(err)
	}

	result, err := BuildArchive(ctx, BuildArchiveOptions{
		Root:   root,
		Output: Destination{Path: filepath.Join(root, "documentation.ocidoc")},
	})
	if err != nil {
		panic(err)
	}

	reader, err := OpenArchive(result.Output)
	if err != nil {
		panic(err)
	}
	defer reader.Close() //nolint:errcheck // temporary extracted archive.

	verification, err := Verify(ctx, reader, VerifyOptions{})
	if err != nil {
		panic(err)
	}

	fmt.Println(verification.Valid)
	// Output: true
}
