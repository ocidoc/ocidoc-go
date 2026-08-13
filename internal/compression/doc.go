// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: Copyright 2026 WoozyMasta
// Source: github.com/ocidoc/ocidoc-go

// Package compression wraps gzip and zstd component-layer compression with deterministic settings:
// a fixed gzip header, and pinned zstd encoder options that do not depend on the build host
// (CPU count in particular).
package compression
