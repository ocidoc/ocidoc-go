// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: Copyright 2026 WoozyMasta
// Source: github.com/ocidoc/ocidoc-go

package registry

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/opencontainers/go-digest"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"

	"github.com/ocidoc/ocidoc-go/artifact"
	"github.com/ocidoc/ocidoc-go/internal/ociblob"
	"github.com/ocidoc/ocidoc-go/spec"
)

func TestClientOpenSupportsRemoteReadOperations(t *testing.T) {
	addr := startTestRegistry(t)
	reference := fmt.Sprintf("%s/test:1.0.0", addr)
	ctx := context.Background()
	client := NewClient(ClientOptions{PlainHTTP: true})

	if _, err := client.Push(ctx, buildTestArtifact(t), reference); err != nil {
		t.Fatalf("Push: %v", err)
	}

	r, err := client.Open(ctx, reference)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer r.Close() //nolint:errcheck // test cleanup.

	if _, err := artifact.Inspect(ctx, r, artifact.InspectOptions{}); err != nil {
		t.Fatalf("Inspect: %v", err)
	}

	files, err := artifact.List(ctx, r, artifact.ListOptions{})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(files) != 1 || files[0].Path != "README.md" {
		t.Fatalf("unexpected files: %+v", files)
	}

	verification, err := artifact.Verify(ctx, r, artifact.VerifyOptions{})
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if !verification.Valid {
		t.Fatalf("remote artifact is invalid: %+v", verification.Issues)
	}

	output := t.TempDir()
	if err := artifact.Extract(ctx, r, artifact.ExtractOptions{Output: output}); err != nil {
		t.Fatalf("Extract: %v", err)
	}
	//nolint:gosec // fixed path under the test's temporary directory.
	content, err := os.ReadFile(filepath.Join(output, "README.md"))
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if string(content) != "# hi" {
		t.Fatalf("got extracted content %q", content)
	}

	other, err := client.Open(ctx, reference)
	if err != nil {
		t.Fatalf("Open other: %v", err)
	}
	defer other.Close() //nolint:errcheck // test cleanup.

	diff, err := artifact.Diff(ctx, r, other, artifact.DiffOptions{})
	if err != nil {
		t.Fatalf("Diff: %v", err)
	}
	if !diff.Equal {
		t.Fatalf("same remote artifact differs: %+v", diff)
	}
}

func TestRemoteReaderKeepsComponentsLazyForUnchangedDiff(t *testing.T) {
	configData, err := json.Marshal(spec.ArtifactConfig{
		SchemaVersion: spec.SchemaVersion,
		Components: map[spec.ComponentType]spec.ComponentConfig{
			spec.ComponentDocumentation: {},
		},
	})
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}

	configDesc := descriptorFor(spec.ConfigMediaType, configData)
	componentData := []byte("not fetched")
	componentDesc := descriptorFor(spec.ComponentLayerGzip, componentData)
	componentDesc.Annotations = map[string]string{spec.AnnotationComponentType: string(spec.ComponentDocumentation)}

	repo := &countingRepository{blobs: map[digest.Digest][]byte{
		configDesc.Digest:    configData,
		componentDesc.Digest: componentData,
	}}
	manifest := &ocispec.Manifest{
		ArtifactType: spec.ArtifactType,
		Config:       configDesc,
		Layers:       []ocispec.Descriptor{componentDesc},
	}
	r := &remoteReader{repo: repo, manifest: manifest}

	result, err := artifact.Diff(context.Background(), r, r, artifact.DiffOptions{})
	if err != nil {
		t.Fatalf("Diff: %v", err)
	}
	if !result.Equal {
		t.Fatalf("same reader differs: %+v", result)
	}
	if got := repo.fetches[componentDesc.Digest]; got != 0 {
		t.Fatalf("component fetched %d times, want 0", got)
	}
	if got := repo.fetches[configDesc.Digest]; got != 2 {
		t.Fatalf("config fetched %d times, want 2", got)
	}
}

func TestFetchBlobRejectsOversizedMetadataBeforeFetch(t *testing.T) {
	repo := &countingRepository{}
	desc := ocispec.Descriptor{Digest: digest.FromString("oversized"), Size: maxMetadataBlobSize + 1}

	_, err := fetchBlob(context.Background(), repo, desc)
	if err == nil {
		t.Fatal("expected oversized metadata error")
	}
	if len(repo.fetches) != 0 {
		t.Fatalf("repository Fetch called for oversized descriptor: %+v", repo.fetches)
	}
}

func TestRemoteComponentRejectsSizeMismatch(t *testing.T) {
	expected := descriptorFor(spec.ComponentLayerGzip, []byte("short"))
	r, err := ociblob.NewVerifyingReadCloser(
		io.NopCloser(bytes.NewReader([]byte("too long"))), expected, "component", ErrInvalid, ErrVerification,
	)
	if err != nil {
		t.Fatalf("ociblob.NewVerifyingReadCloser: %v", err)
	}
	defer r.Close() //nolint:errcheck // test cleanup.

	_, err = io.ReadAll(r)
	if !errors.Is(err, ErrVerification) {
		t.Fatalf("expected errors.Is(err, ErrVerification), got %v", err)
	}
}

func TestRemoteComponentRejectsUnsupportedDigestWithoutPanic(t *testing.T) {
	expected := ocispec.Descriptor{
		Digest: "unknown:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		Size:   7,
	}

	_, err := ociblob.NewVerifyingReadCloser(
		io.NopCloser(bytes.NewReader([]byte("content"))), expected, "component", ErrInvalid, ErrVerification,
	)
	if !errors.Is(err, ErrInvalid) {
		t.Fatalf("expected errors.Is(err, ErrInvalid), got %v", err)
	}
}

func descriptorFor(mediaType string, data []byte) ocispec.Descriptor {
	return ocispec.Descriptor{MediaType: mediaType, Digest: digest.FromBytes(data), Size: int64(len(data))}
}

type countingRepository struct {
	blobs   map[digest.Digest][]byte
	fetches map[digest.Digest]int
}

func (r *countingRepository) Resolve(context.Context, string) (ocispec.Descriptor, error) {
	return ocispec.Descriptor{}, nil
}

func (r *countingRepository) Fetch(_ context.Context, desc ocispec.Descriptor) (io.ReadCloser, error) {
	if r.fetches == nil {
		r.fetches = make(map[digest.Digest]int)
	}
	r.fetches[desc.Digest]++
	data, ok := r.blobs[desc.Digest]
	if !ok {
		return nil, fmt.Errorf("missing blob %s", desc.Digest)
	}
	return io.NopCloser(bytes.NewReader(data)), nil
}

func (r *countingRepository) Push(context.Context, ocispec.Descriptor, io.Reader) error {
	return nil
}

func (r *countingRepository) PushManifest(context.Context, ocispec.Descriptor, io.Reader, bool) error {
	return nil
}

func (r *countingRepository) Tag(context.Context, ocispec.Descriptor, string) error { return nil }

func (r *countingRepository) Referrers(context.Context, ocispec.Descriptor, string) ([]ocispec.Descriptor, error) {
	return nil, nil
}

func (r *countingRepository) Delete(context.Context, ocispec.Descriptor) error { return nil }
