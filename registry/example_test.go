// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: Copyright 2026 WoozyMasta
// Source: github.com/ocidoc/ocidoc-go

package registry_test

import (
	"context"
	"fmt"

	"github.com/ocidoc/ocidoc-go/artifact"
	"github.com/ocidoc/ocidoc-go/registry"
)

// ExampleClient_Attach attaches an existing OCIDoc archive to an OCI image.
func ExampleClient_Attach() {
	ctx := context.Background()
	reader, err := artifact.OpenArchive("documentation.ocidoc")
	if err != nil {
		panic(err)
	}
	defer reader.Close() //nolint:errcheck // releases the archive extraction directory.

	client := registry.NewClient(registry.ClientOptions{})
	attached, err := client.Attach(
		ctx, reader, "registry.example/team/app:1.2.3", registry.AttachOptions{
			Publication: registry.PublicationBoth,
		})
	if err != nil {
		panic(err)
	}

	fmt.Println(attached.Reference)
}
