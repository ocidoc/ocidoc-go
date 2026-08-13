// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: Copyright 2026 WoozyMasta
// Source: github.com/ocidoc/ocidoc-go

// Package archive builds deterministic POSIX component tars:
// regular files only, sorted by bundle path, fixed ownership/mode/mtime.
// It does not compress; see internal/compression.
package archive
