// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: Copyright 2026 WoozyMasta
// Source: github.com/ocidoc/ocidoc-go

package orasrepo

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"

	"oras.land/oras-go/v2/errdef"
	"oras.land/oras-go/v2/registry/remote/errcode"
)

// IsTemporary reports whether err is a timeout or registry 5xx failure.
func IsTemporary(err error) bool {
	if errors.Is(err, context.DeadlineExceeded) {
		return true
	}

	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		return true
	}

	var errResp *errcode.ErrorResponse
	return errors.As(err, &errResp) && errResp.StatusCode >= 500 && errResp.StatusCode <= 599
}

// Sentinel errors classify ORAS errors without exposing ORAS error types to callers of this adapter.
var (
	ErrNotFound     = errors.New("not found")
	ErrUnauthorized = errors.New("authentication required")
	ErrForbidden    = errors.New("access denied")
	ErrUnsupported  = errors.New("unsupported operation")
	ErrInvalid      = errors.New("invalid reference or content")
)

// mapError wraps recognized ORAS errors with this package's sentinel errors.
// It preserves unrecognized errors and does not treat authentication
// or authorization failures as missing content.
func mapError(err error) error {
	if err == nil {
		return nil
	}

	var errResp *errcode.ErrorResponse
	if errors.As(err, &errResp) {
		switch errResp.StatusCode {
		case http.StatusUnauthorized:
			return fmt.Errorf("%w: %s", ErrUnauthorized, err)

		case http.StatusForbidden:
			return fmt.Errorf("%w: %s", ErrForbidden, err)

		case http.StatusNotFound:
			return fmt.Errorf("%w: %s", ErrNotFound, err)

		case http.StatusMethodNotAllowed, http.StatusNotImplemented:
			return fmt.Errorf("%w: %s", ErrUnsupported, err)
		}
	}

	switch {
	case errors.Is(err, errdef.ErrNotFound):
		return fmt.Errorf("%w: %s", ErrNotFound, err)

	case errors.Is(err, errdef.ErrUnsupported), errors.Is(err, errdef.ErrUnsupportedVersion):
		return fmt.Errorf("%w: %s", ErrUnsupported, err)

	case errors.Is(err, errdef.ErrInvalidReference),
		errors.Is(err, errdef.ErrInvalidDigest),
		errors.Is(err, errdef.ErrInvalidMediaType),
		errors.Is(err, errdef.ErrMissingReference):
		return fmt.Errorf("%w: %s", ErrInvalid, err)
	}

	return err
}
