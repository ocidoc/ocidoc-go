// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: Copyright 2026 WoozyMasta
// Source: github.com/ocidoc/ocidoc-go

// Package atomicfile writes files so that other processes only ever observe either the previous content
// or the fully written new content, never a partial write.
// Every write goes through a temporary file in the destination's own directory
// (so the final rename is same-filesystem) followed by os.Rename into place.
package atomicfile

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// Temp is a temporary file created alongside an eventual destination path,
// ready to be finalized with Rename or discarded with Cleanup.
type Temp struct {
	f    *os.File
	done bool
}

// CreateTemp creates a temporary file in dir for later renaming into place.
// dir must be the destination's own directory
// so the later Rename is same-filesystem and therefore atomic.
func CreateTemp(dir string) (*Temp, error) {
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return nil, fmt.Errorf("create directory %q: %w", dir, err)
	}

	f, err := os.CreateTemp(dir, "ocidoc-*.tmp")
	if err != nil {
		return nil, fmt.Errorf("create temp file in %q: %w", dir, err)
	}

	return &Temp{f: f}, nil
}

// File returns the underlying file for writing.
func (t *Temp) File() *os.File {
	return t.f
}

// Rename closes the temp file and publishes it at path.
// Cleanup after a successful Rename is a no-op.
func (t *Temp) Rename(path string) error {
	if err := t.f.Close(); err != nil {
		return fmt.Errorf("close temp file %q: %w", t.f.Name(), err)
	}
	if err := publish(t.f.Name(), path); err != nil {
		return err
	}
	t.done = true

	return nil
}

// Cleanup best-effort closes and removes the temp file if it was never renamed into place.
// Safe to call unconditionally, including after a successful Rename.
func (t *Temp) Cleanup() {
	if t.done {
		return
	}
	_ = t.f.Close()
	_ = os.Remove(t.f.Name())
}

// WriteFile writes path's content to a temporary file
// and publishes it only after the callback and temporary-file close both succeed.
// If overwrite is false, publication fails without replacing an existing path.
func WriteFile(path string, overwrite bool, write func(io.Writer) error) error {
	tmp, err := CreateTemp(filepath.Dir(path))
	if err != nil {
		return err
	}
	defer tmp.Cleanup()

	if err := write(tmp.File()); err != nil {
		return err
	}

	if overwrite {
		return tmp.Rename(path)
	}

	return tmp.renameNoReplace(path)
}

// renameNoReplace publishes the temporary file without replacing an existing path.
func (t *Temp) renameNoReplace(path string) error {
	if err := t.f.Close(); err != nil {
		return fmt.Errorf("close temp file %q: %w", t.f.Name(), err)
	}
	if err := publishNoReplace(t.f.Name(), path); err != nil {
		return err
	}
	t.done = true

	return nil
}
