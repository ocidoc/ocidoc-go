// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: Copyright 2026 WoozyMasta
// Source: github.com/ocidoc/ocidoc-go

package compression

import (
	"bytes"
	"errors"
	"io"
	"testing"
	"time"

	"github.com/ocidoc/ocidoc-go/spec"
)

func TestNewReaderGzipRoundTrip(t *testing.T) {
	content := []byte("hello via gzip reader")

	compressed := compress(t, spec.CompressionGzip, 6, time.Time{}, content)

	r, err := NewReader(bytes.NewReader(compressed), spec.ComponentLayerGzip)
	if err != nil {
		t.Fatalf("NewReader: %v", err)
	}

	defer r.Close()

	got, err := io.ReadAll(r)
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}

	if !bytes.Equal(got, content) {
		t.Fatalf("got %q, want %q", got, content)
	}
}

func TestNewReaderZstdRoundTrip(t *testing.T) {
	content := []byte("hello via zstd reader")

	compressed := compress(t, spec.CompressionZstd, 3, time.Time{}, content)

	r, err := NewReader(bytes.NewReader(compressed), spec.ComponentLayerZstd)
	if err != nil {
		t.Fatalf("NewReader: %v", err)
	}

	defer r.Close()

	got, err := io.ReadAll(r)
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}

	if !bytes.Equal(got, content) {
		t.Fatalf("got %q, want %q", got, content)
	}
}

func TestNewReaderRejectsUnsupportedMediaType(t *testing.T) {
	_, err := NewReader(bytes.NewReader(nil), "application/x-unknown")
	if err == nil {
		t.Fatal("expected error for an unsupported media type")
	}

	if !errors.Is(err, spec.ErrUnsupported) {
		t.Fatalf("expected errors.Is(err, spec.ErrUnsupported), got %v", err)
	}
}
