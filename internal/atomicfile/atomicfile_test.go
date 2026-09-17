// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: Copyright 2026 WoozyMasta
// Source: github.com/ocidoc/ocidoc-go

package atomicfile

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"
)

func TestWriteFileNoReplaceKeepsExistingDestination(t *testing.T) {
	path := filepath.Join(t.TempDir(), "result")
	if err := os.WriteFile(path, []byte("old"), 0o600); err != nil {
		t.Fatalf("WriteFile existing: %v", err)
	}

	err := WriteFile(path, false, func(w io.Writer) error {
		_, err := w.Write([]byte("new"))
		return err
	})
	if err == nil {
		t.Fatal("WriteFile unexpectedly replaced an existing destination")
	}

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if string(got) != "old" {
		t.Fatalf("destination = %q, want old", got)
	}
}

func TestWriteFileWriteErrorLeavesNoDestination(t *testing.T) {
	path := filepath.Join(t.TempDir(), "result")
	want := errors.New("write failed")
	if err := WriteFile(path, false, func(io.Writer) error { return want }); !errors.Is(err, want) {
		t.Fatalf("WriteFile: got %v, want %v", err, want)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("destination exists after failed write: %v", err)
	}
}
