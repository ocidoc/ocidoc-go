// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: Copyright 2026 WoozyMasta
// Source: github.com/ocidoc/ocidoc-go

package registry

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	ocispec "github.com/opencontainers/image-spec/specs-go/v1"

	"github.com/ocidoc/ocidoc-go/artifact"
)

func TestClientCopyStandaloneDirections(t *testing.T) {
	addr := startTestRegistry(t)
	ctx := context.Background()
	client := NewClient(ClientOptions{PlainHTTP: true})

	sourceReference := fmt.Sprintf("%s/source:1.0.0", addr)
	pushed, err := client.Push(ctx, buildTestArtifact(t), sourceReference)
	if err != nil {
		t.Fatalf("Push source: %v", err)
	}

	archivePath := filepath.Join(t.TempDir(), "copied.ocidoc")
	assertCopyDigest(t, client, Source{Kind: LocationRegistry, Value: sourceReference}, Destination{
		Kind: LocationArchive, Value: archivePath,
	}, pushed.Manifest)

	archiveReference := fmt.Sprintf("%s/from-archive:1.0.0", addr)
	assertCopyDigest(t, client, Source{Kind: LocationArchive, Value: archivePath}, Destination{
		Kind: LocationRegistry, Value: archiveReference,
	}, pushed.Manifest)

	layoutPath := filepath.Join(t.TempDir(), "layout")
	assertCopyDigest(t, client, Source{Kind: LocationRegistry, Value: sourceReference}, Destination{
		Kind: LocationLayout, Value: layoutPath,
	}, pushed.Manifest)

	layoutReference := fmt.Sprintf("%s/from-layout:1.0.0", addr)
	assertCopyDigest(t, client, Source{Kind: LocationLayout, Value: layoutPath}, Destination{
		Kind: LocationRegistry, Value: layoutReference,
	}, pushed.Manifest)

	remoteReference := fmt.Sprintf("%s/from-remote:1.0.0", addr)
	assertCopyDigest(t, client, Source{Kind: LocationRegistry, Value: sourceReference}, Destination{
		Kind: LocationRegistry, Value: remoteReference,
	}, pushed.Manifest)
}

func TestClientCopySubjectToSubjectRebuildsOnlyRootManifest(t *testing.T) {
	addr := startTestRegistry(t)
	ctx := context.Background()
	client := NewClient(ClientOptions{PlainHTTP: true})
	sourceSubject := fmt.Sprintf("%s/subject-copy:source", addr)
	destinationSubject := fmt.Sprintf("%s/subject-copy:destination", addr)
	sourceSubjectResult, err := client.Push(
		ctx,
		buildTestArtifactWithContent(t, "source subject"),
		sourceSubject,
	)
	if err != nil {
		t.Fatalf("Push source subject: %v", err)
	}
	destinationSubjectResult, err := client.Push(
		ctx,
		buildTestArtifactWithContent(t, "destination subject"),
		destinationSubject,
	)
	if err != nil {
		t.Fatalf("Push destination subject: %v", err)
	}
	if sourceSubjectResult.Manifest.Digest == destinationSubjectResult.Manifest.Digest {
		t.Fatal("test subjects unexpectedly have the same digest")
	}

	sourceAttached, err := client.Attach(
		ctx,
		buildTestArtifactWithDocument(t, "api", "en", "full"),
		sourceSubject,
		AttachOptions{},
	)
	if err != nil {
		t.Fatalf("Attach source documentation: %v", err)
	}
	result, err := client.Copy(ctx,
		Source{Kind: LocationSubject, Value: sourceSubject},
		Destination{Kind: LocationSubject, Value: destinationSubject},
		CopyOptions{Discover: DiscoverOptions{Document: "api", Variant: "full"}},
	)
	if err != nil {
		t.Fatalf("Copy subject to subject: %v", err)
	}
	if result.SourceManifest.Digest != sourceAttached.Manifest.Digest || result.Manifest.Digest == sourceAttached.Manifest.Digest {
		t.Fatalf("unexpected copy roots: %+v", result)
	}
	if result.Publication != PublicationBoth {
		t.Fatalf("publication %q, want %q", result.Publication, PublicationBoth)
	}

	destinationDoc, err := client.Discover(ctx, destinationSubject, DiscoverOptions{})
	if err != nil {
		t.Fatalf("Discover destination documentation: %v", err)
	}
	if destinationDoc.Manifest.Digest != result.Manifest.Digest {
		t.Fatalf("discovered digest %s, want %s", destinationDoc.Manifest.Digest, result.Manifest.Digest)
	}
	sourceReader, err := client.Open(ctx, sourceAttached.Reference)
	if err != nil {
		t.Fatalf("Open source attached: %v", err)
	}
	defer sourceReader.Close() //nolint:errcheck // test cleanup.
	destinationReader, err := client.Open(ctx, destinationDoc.Reference)
	if err != nil {
		t.Fatalf("Open destination attached: %v", err)
	}
	defer destinationReader.Close() //nolint:errcheck // test cleanup.
	sourceManifest, err := sourceReader.Manifest(ctx)
	if err != nil {
		t.Fatalf("source Manifest: %v", err)
	}
	destinationManifest, err := destinationReader.Manifest(ctx)
	if err != nil {
		t.Fatalf("destination Manifest: %v", err)
	}
	if destinationManifest.Subject == nil || destinationManifest.Subject.Digest != destinationSubjectResult.Manifest.Digest {
		t.Fatalf("unexpected destination subject: %+v", destinationManifest.Subject)
	}
	if sourceManifest.Config.Digest != destinationManifest.Config.Digest || len(sourceManifest.Layers) != len(destinationManifest.Layers) {
		t.Fatalf("copied graph changed: source=%+v destination=%+v", sourceManifest, destinationManifest)
	}
	for i := range sourceManifest.Layers {
		if sourceManifest.Layers[i].Digest != destinationManifest.Layers[i].Digest {
			t.Fatalf("layer %d changed from %s to %s", i, sourceManifest.Layers[i].Digest, destinationManifest.Layers[i].Digest)
		}
	}
}

func TestClientCopyLayoutOverwrite(t *testing.T) {
	addr := startTestRegistry(t)
	ctx := context.Background()
	client := NewClient(ClientOptions{PlainHTTP: true})
	reference := fmt.Sprintf("%s/source:1.0.0", addr)

	if _, err := client.Push(ctx, buildTestArtifact(t), reference); err != nil {
		t.Fatalf("Push source: %v", err)
	}

	layoutPath := filepath.Join(t.TempDir(), "layout")
	if err := os.Mkdir(layoutPath, 0o750); err != nil {
		t.Fatalf("Mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(layoutPath, "old"), []byte("old"), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	source := Source{Kind: LocationRegistry, Value: reference}
	destination := Destination{Kind: LocationLayout, Value: layoutPath}
	if _, err := client.Copy(ctx, source, destination, CopyOptions{}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("expected errors.Is(err, ErrInvalid), got %v", err)
	}

	destination.Overwrite = true
	if _, err := client.Copy(ctx, source, destination, CopyOptions{}); err != nil {
		t.Fatalf("Copy overwrite: %v", err)
	}

	reader, err := artifact.OpenLayout(layoutPath)
	if err != nil {
		t.Fatalf("OpenLayout: %v", err)
	}
	defer reader.Close() //nolint:errcheck // test cleanup.
}

func TestClientCopyRejectsLocalToLocal(t *testing.T) {
	client := NewClient(ClientOptions{})

	_, err := client.Copy(context.Background(),
		Source{Kind: LocationArchive, Value: "source.ocidoc"},
		Destination{Kind: LocationLayout, Value: "layout"},
		CopyOptions{},
	)
	if !errors.Is(err, ErrUnsupported) {
		t.Fatalf("expected errors.Is(err, ErrUnsupported), got %v", err)
	}
}

func assertCopyDigest(
	t *testing.T,
	client *Client,
	source Source,
	destination Destination,
	want ocispec.Descriptor,
) {
	t.Helper()

	result, err := client.Copy(context.Background(), source, destination, CopyOptions{})
	if err != nil {
		t.Fatalf("Copy %s to %s: %v", source.Kind, destination.Kind, err)
	}
	if result.Manifest.Digest != want.Digest {
		t.Fatalf("got manifest digest %s, want %s", result.Manifest.Digest, want.Digest)
	}
}
