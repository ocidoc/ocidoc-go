// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: Copyright 2026 WoozyMasta
// Source: github.com/ocidoc/ocidoc-go

package artifact

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/opencontainers/go-digest"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
)

type rawMetadataReader struct {
	Reader
	root       ocispec.Descriptor
	manifest   *ocispec.Manifest
	rootData   []byte
	configData []byte
}

func (r *rawMetadataReader) Root(context.Context) (ocispec.Descriptor, error) {
	return r.root, nil
}

func (r *rawMetadataReader) Manifest(context.Context) (*ocispec.Manifest, error) {
	return r.manifest, nil
}

func (r *rawMetadataReader) OpenBlob(ctx context.Context, desc ocispec.Descriptor) (io.ReadCloser, error) {
	switch desc.Digest {
	case r.root.Digest:
		return io.NopCloser(bytes.NewReader(r.rootData)), nil

	case r.manifest.Config.Digest:
		return io.NopCloser(bytes.NewReader(r.configData)), nil

	default:
		return r.Reader.OpenBlob(ctx, desc)
	}
}

func TestPackageReaderPreservesRawMetadataBlobs(t *testing.T) {
	layoutDir, _ := buildTestLayout(t)
	base, err := OpenLayout(layoutDir)
	if err != nil {
		t.Fatalf("OpenLayout: %v", err)
	}

	ctx := context.Background()
	manifest, err := base.Manifest(ctx)
	if err != nil {
		t.Fatalf("Manifest: %v", err)
	}

	configReader, err := base.OpenBlob(ctx, manifest.Config)
	if err != nil {
		t.Fatalf("OpenBlob(config): %v", err)
	}
	originalConfig, err := io.ReadAll(configReader)
	_ = configReader.Close()
	if err != nil {
		t.Fatalf("Read config: %v", err)
	}

	manifest = cloneManifest(manifest)
	configData := wrapJSON(originalConfig)
	manifest.Config.Digest = digest.Canonical.FromBytes(configData)
	manifest.Config.Size = int64(len(configData))
	manifestData, err := json.Marshal(manifest)
	if err != nil {
		t.Fatalf("Marshal manifest: %v", err)
	}
	rootData := wrapJSON(manifestData)
	root := ocispec.Descriptor{
		MediaType:    ocispec.MediaTypeImageManifest,
		ArtifactType: manifest.ArtifactType,
		Digest:       digest.Canonical.FromBytes(rootData),
		Size:         int64(len(rootData)),
	}

	reader := &rawMetadataReader{
		Reader:     base,
		root:       root,
		manifest:   manifest,
		rootData:   rootData,
		configData: configData,
	}
	defer base.Close() //nolint:errcheck // layout reader owns no resources.

	var archive bytes.Buffer
	if err := PackageReader(ctx, reader, &archive, time.Unix(1700000000, 0).UTC()); err != nil {
		t.Fatalf("PackageReader: %v", err)
	}

	packaged, err := openArchiveBytes(t, archive.Bytes())
	if err != nil {
		t.Fatalf("OpenArchive: %v", err)
	}
	defer packaged.Close() //nolint:errcheck // temporary extraction cleanup.

	packagedRoot, err := packaged.Root(ctx)
	if err != nil {
		t.Fatalf("packaged Root: %v", err)
	}
	assertBlobBytes(t, packaged, packagedRoot, rootData)
	assertBlobBytes(t, packaged, manifest.Config, configData)
}

func wrapJSON(data []byte) []byte {
	return append(append([]byte{'\n'}, data...), '\n')
}

func openArchiveBytes(t *testing.T, data []byte) (Reader, error) {
	t.Helper()

	path := filepath.Join(t.TempDir(), "artifact.ocidoc")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		return nil, err
	}
	return OpenArchive(path)
}

func cloneManifest(manifest *ocispec.Manifest) *ocispec.Manifest {
	cloned := *manifest
	cloned.Config = manifest.Config
	cloned.Layers = append([]ocispec.Descriptor(nil), manifest.Layers...)
	return &cloned
}

func assertBlobBytes(t *testing.T, reader Reader, desc ocispec.Descriptor, want []byte) {
	t.Helper()

	rc, err := reader.OpenBlob(context.Background(), desc)
	if err != nil {
		t.Fatalf("OpenBlob(%s): %v", desc.Digest, err)
	}
	got, readErr := io.ReadAll(rc)
	closeErr := rc.Close()
	if readErr != nil {
		t.Fatalf("ReadBlob(%s): %v", desc.Digest, readErr)
	}
	if closeErr != nil {
		t.Fatalf("CloseBlob(%s): %v", desc.Digest, closeErr)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("blob %s changed during packaging", desc.Digest)
	}
}
