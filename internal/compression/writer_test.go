// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: Copyright 2026 WoozyMasta
// Source: github.com/ocidoc/ocidoc-go

package compression

import (
	"bytes"
	"compress/gzip"
	"errors"
	"io"
	"testing"
	"time"

	zstdlib "github.com/klauspost/compress/zstd"

	"github.com/ocidoc/ocidoc-go/spec"
)

func compress(t *testing.T, typ spec.CompressionType, level int, modTime time.Time, content []byte) []byte {
	t.Helper()

	var buf bytes.Buffer

	w, err := NewWriter(&buf, typ, level, modTime)
	if err != nil {
		t.Fatalf("NewWriter: %v", err)
	}

	if _, err := w.Write(content); err != nil {
		t.Fatalf("Write: %v", err)
	}

	if err := w.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	return buf.Bytes()
}

func TestNewWriterGzipDeterministicAcrossRuns(t *testing.T) {
	modTime := time.Unix(1700000000, 0).UTC()
	content := []byte("hello, ocidoc")

	first := compress(t, spec.CompressionGzip, 6, modTime, content)
	second := compress(t, spec.CompressionGzip, 6, modTime, content)

	if !bytes.Equal(first, second) {
		t.Fatal("expected identical gzip output across two runs with the same inputs")
	}
}

func TestNewWriterGzipFixedHeader(t *testing.T) {
	modTime := time.Unix(1700000000, 0).UTC()

	out := compress(t, spec.CompressionGzip, 6, modTime, []byte("content"))

	r, err := gzip.NewReader(bytes.NewReader(out))
	if err != nil {
		t.Fatalf("gzip.NewReader: %v", err)
	}

	if r.Name != "" {
		t.Errorf("got Name %q, want empty", r.Name)
	}

	if r.Comment != "" {
		t.Errorf("got Comment %q, want empty", r.Comment)
	}

	if r.OS != gzipOS {
		t.Errorf("got OS %d, want %d", r.OS, gzipOS)
	}

	if !r.ModTime.Equal(modTime) {
		t.Errorf("got ModTime %v, want %v", r.ModTime, modTime)
	}

	decoded, err := io.ReadAll(r)
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}

	if string(decoded) != "content" {
		t.Fatalf("got content %q, want %q", decoded, "content")
	}
}

func TestNewWriterGzipRoundTrip(t *testing.T) {
	content := []byte("round trip content, repeated repeated repeated")

	out := compress(t, spec.CompressionGzip, 6, time.Time{}, content)

	r, err := gzip.NewReader(bytes.NewReader(out))
	if err != nil {
		t.Fatalf("gzip.NewReader: %v", err)
	}

	decoded, err := io.ReadAll(r)
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}

	if !bytes.Equal(decoded, content) {
		t.Fatalf("got %q, want %q", decoded, content)
	}
}

func TestNewWriterZstdDeterministicAcrossRuns(t *testing.T) {
	content := []byte("hello, ocidoc, via zstd")

	first := compress(t, spec.CompressionZstd, 3, time.Time{}, content)
	second := compress(t, spec.CompressionZstd, 3, time.Time{}, content)

	if !bytes.Equal(first, second) {
		t.Fatal("expected identical zstd output across two runs with the same inputs")
	}
}

func TestNewWriterZstdRoundTrip(t *testing.T) {
	content := []byte("round trip content via zstd, repeated repeated repeated")

	out := compress(t, spec.CompressionZstd, 3, time.Time{}, content)

	dec, err := zstdlib.NewReader(nil)
	if err != nil {
		t.Fatalf("zstd.NewReader: %v", err)
	}
	defer dec.Close()

	decoded, err := dec.DecodeAll(out, nil)
	if err != nil {
		t.Fatalf("DecodeAll: %v", err)
	}

	if !bytes.Equal(decoded, content) {
		t.Fatalf("got %q, want %q", decoded, content)
	}
}

func TestNewWriterRejectsUnsupportedType(t *testing.T) {
	var buf bytes.Buffer

	_, err := NewWriter(&buf, spec.CompressionType("lzma"), 6, time.Time{})
	if err == nil {
		t.Fatal("expected error for unsupported compression type")
	}

	if !errors.Is(err, spec.ErrUnsupported) {
		t.Fatalf("expected errors.Is(err, spec.ErrUnsupported), got %v", err)
	}
}

func TestNewWriterClampsCompressionLevel(t *testing.T) {
	content := []byte("content")
	for _, typ := range []spec.CompressionType{spec.CompressionGzip, spec.CompressionZstd} {
		got := compress(t, typ, spec.MaxCompressionLevel+1, time.Time{}, content)
		if len(got) == 0 {
			t.Fatalf("%s: expected compressed output", typ)
		}
	}
}
