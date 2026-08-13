// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: Copyright 2026 WoozyMasta
// Source: github.com/ocidoc/ocidoc-go

package registry

import (
	"errors"
	"fmt"

	"github.com/ocidoc/ocidoc-go/internal/orasrepo"
)

var (
	// ErrNotFound reports that the requested OCIDoc artifact or registry object does not exist.
	ErrNotFound = errors.New("ocidoc artifact not found")

	// ErrAmbiguous reports that selectors match more than one OCIDoc artifact.
	ErrAmbiguous = errors.New("multiple ocidoc artifacts match")

	// ErrConflict reports that publication would replace a different existing artifact,
	// or that discovery mechanisms disagree.
	ErrConflict = errors.New("ocidoc publication conflict")

	// ErrUnsupported reports an operation or registry capability
	// that is not supported by this package.
	ErrUnsupported = errors.New("unsupported operation")

	// ErrInvalid reports an invalid reference, descriptor, manifest,
	// or other OCIDoc input.
	ErrInvalid = errors.New("invalid ocidoc artifact or reference")

	// ErrVerification reports content that does not match its expected digest or size.
	ErrVerification = errors.New("ocidoc content verification failed")

	// ErrUnauthorized reports missing or rejected authentication credentials.
	ErrUnauthorized = errors.New("authentication required")

	// ErrForbidden reports authenticated access that lacks required permission.
	ErrForbidden = errors.New("access denied")
)

// wrapError classifies err against internal/orasrepo's sentinels
// and wraps it with this package's own public sentinel above,
// so callers never need to depend on internal/orasrepo directly;
// an err that does not match any of them
// (including one already produced by this package) is returned unchanged.
func wrapError(err error) error {
	if err == nil {
		return nil
	}

	switch {
	case errors.Is(err, orasrepo.ErrNotFound):
		return fmt.Errorf("%w: %s", ErrNotFound, err)
	case errors.Is(err, orasrepo.ErrUnauthorized):
		return fmt.Errorf("%w: %s", ErrUnauthorized, err)
	case errors.Is(err, orasrepo.ErrForbidden):
		return fmt.Errorf("%w: %s", ErrForbidden, err)
	case errors.Is(err, orasrepo.ErrUnsupported):
		return fmt.Errorf("%w: %s", ErrUnsupported, err)
	case errors.Is(err, orasrepo.ErrInvalid):
		return fmt.Errorf("%w: %s", ErrInvalid, err)
	}

	return err
}
