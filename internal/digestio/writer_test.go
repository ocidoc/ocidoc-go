// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: Copyright 2026 WoozyMasta
// Source: github.com/ocidoc/ocidoc-go

package digestio

import (
	"bytes"
	"testing"

	"github.com/opencontainers/go-digest"
)

func TestWriterComputesDigestAndSize(t *testing.T) {
	var dst bytes.Buffer

	w := NewWriter(&dst)

	content := []byte("hello, ocidoc")

	n, err := w.Write(content)
	if err != nil {
		t.Fatalf("Write: %v", err)
	}

	if n != len(content) {
		t.Fatalf("got n=%d, want %d", n, len(content))
	}

	if w.Size() != int64(len(content)) {
		t.Fatalf("got Size()=%d, want %d", w.Size(), len(content))
	}

	want := digest.Canonical.FromBytes(content)
	if w.Digest() != want {
		t.Fatalf("got Digest()=%s, want %s", w.Digest(), want)
	}

	if dst.String() != string(content) {
		t.Fatalf("dst got %q, want %q (writes must still reach dst)", dst.String(), content)
	}
}

func TestWriterAccumulatesAcrossMultipleWrites(t *testing.T) {
	var dst bytes.Buffer

	w := NewWriter(&dst)

	if _, err := w.Write([]byte("hello, ")); err != nil {
		t.Fatalf("Write: %v", err)
	}

	if _, err := w.Write([]byte("ocidoc")); err != nil {
		t.Fatalf("Write: %v", err)
	}

	want := digest.Canonical.FromBytes([]byte("hello, ocidoc"))
	if w.Digest() != want {
		t.Fatalf("got Digest()=%s, want %s", w.Digest(), want)
	}

	if w.Size() != int64(len("hello, ocidoc")) {
		t.Fatalf("got Size()=%d, want %d", w.Size(), len("hello, ocidoc"))
	}
}
