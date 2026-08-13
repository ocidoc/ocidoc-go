// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: Copyright 2026 WoozyMasta
// Source: github.com/ocidoc/ocidoc-go

package spec

import (
	"errors"
	"fmt"
)

// Sentinel errors returned by spec-level validation
// and by other ocidoc-go packages that read or verify artifacts.
// Other packages wrap these rather than defining their own sentinels for the same condition.
// New sentinels are added here only once a real caller needs them;
// ErrConflict, ErrUnauthorized and ErrForbidden are not defined yet for that reason.
var (
	// ErrInvalid reports an artifact, config or path that violates the v1beta format.
	ErrInvalid = errors.New("invalid ocidoc artifact")

	// ErrUnsupported reports a request the v1beta format does not support.
	ErrUnsupported = errors.New("unsupported operation")

	// ErrNotFound reports a requested artifact, component or reference that does not exist.
	ErrNotFound = errors.New("ocidoc artifact not found")

	// ErrAmbiguous reports more than one candidate where exactly one was expected.
	ErrAmbiguous = errors.New("multiple ocidoc artifacts found")

	// ErrVerification reports content that does not match its expected digest.
	ErrVerification = errors.New("ocidoc verification failed")
)

// ValidationCode identifies a category of spec-level validation failure.
type ValidationCode string

// Validation failure categories returned by this package.
const (
	CodeInvalidComponentName       ValidationCode = "invalid-component-name"
	CodeUnknownComponentName       ValidationCode = "unknown-component-name"
	CodeInvalidPath                ValidationCode = "invalid-path"
	CodePathCollision              ValidationCode = "path-collision"
	CodeMissingSchemaVersion       ValidationCode = "missing-schema-version"
	CodeUnsupportedSchemaVersion   ValidationCode = "unsupported-schema-version"
	CodeNoComponents               ValidationCode = "no-components"
	CodeUnsupportedDigestAlgorithm ValidationCode = "unsupported-digest-algorithm"
	CodeUnsupportedCompression     ValidationCode = "unsupported-compression"
	CodeReservedAnnotation         ValidationCode = "reserved-annotation"
	CodeEmptyComponentRules        ValidationCode = "empty-component-rules"
	CodeUndeclaredComponent        ValidationCode = "undeclared-component"
)

// ValidationError reports one spec-level validation failure.
//
// Component, Path and Annotation are set only when the failure is scoped
// to that specific component name, bundle path or annotation key;
// each is the empty string otherwise.
type ValidationError struct {
	// Code identifies the validation rule that failed.
	Code ValidationCode

	// Component identifies the affected component, when applicable.
	Component string

	// Path identifies the affected bundle path, when applicable.
	Path string

	// Annotation identifies the affected annotation key, when applicable.
	Annotation string

	// Message gives a human-readable explanation of the failed rule.
	Message string
}

// Error implements the error interface.
func (e *ValidationError) Error() string {
	switch {
	case e.Component != "" && e.Path != "":
		return fmt.Sprintf("%s: component %q, path %q: %s", e.Code, e.Component, e.Path, e.Message)

	case e.Component != "":
		return fmt.Sprintf("%s: component %q: %s", e.Code, e.Component, e.Message)

	case e.Path != "":
		return fmt.Sprintf("%s: path %q: %s", e.Code, e.Path, e.Message)

	case e.Annotation != "":
		return fmt.Sprintf("%s: annotation %q: %s", e.Code, e.Annotation, e.Message)

	default:
		return fmt.Sprintf("%s: %s", e.Code, e.Message)
	}
}

// Unwrap makes ValidationError match errors.Is(err, ErrInvalid).
func (e *ValidationError) Unwrap() error {
	return ErrInvalid
}
