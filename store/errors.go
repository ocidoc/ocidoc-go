// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: Copyright 2026 WoozyMasta
// Source: github.com/ocidoc/ocidoc-go

package store

import "errors"

// Sentinel errors this package can produce.
var (
	// ErrInvalid identifies a malformed store: an unparsable store.json catalog.
	ErrInvalid = errors.New("invalid ocidoc store")

	// ErrLocked identifies a catalog mutation that could not acquire
	// the store's exclusive catalog lock before its context was done -
	// another process, or a stuck lock file, is holding it.
	ErrLocked = errors.New("store catalog is locked")
)
