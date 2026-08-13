// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: Copyright 2026 WoozyMasta
// Source: github.com/ocidoc/ocidoc-go

package spec

import "strings"

// reservedAnnotationPrefix is the namespace reserved for the implementation
// and specification (see AnnotationSchema and related constants).
// User-provided annotations cannot use it.
const reservedAnnotationPrefix = "org.ocidoc."

// ValidateBundlePaths reports whether paths, taken together as one global virtual tree,
// are free of exact duplicates and case-insensitive collisions.
// Components do not have overlay semantics:
// the same normalized path cannot occur twice, even across different components.
//
// Each path must already be individually valid;
// ValidateBundlePaths returns the first ValidateBundlePath error
// it finds before checking for collisions.
func ValidateBundlePaths(paths []string) error {
	seen := make(map[string]string, len(paths)) // normalized (lowercase) -> original

	for _, path := range paths {
		if err := ValidateBundlePath(path); err != nil {
			return err
		}

		key := strings.ToLower(path)

		original, exists := seen[key]
		if !exists {
			seen[key] = path
			continue
		}

		if original == path {
			return &ValidationError{Code: CodePathCollision, Path: path, Message: "duplicate path"}
		}

		return &ValidationError{
			Code: CodePathCollision, Path: path,
			Message: "case-insensitive collision with " + original,
		}
	}

	return nil
}

// ValidateUserAnnotations reports whether annotations may be applied
// by a user through build configuration or another caller interface.
// The "org.ocidoc." namespace is reserved for managed keys
// and cannot be set or overridden by user input.
func ValidateUserAnnotations(annotations map[string]string) error {
	for key := range annotations {
		if strings.HasPrefix(key, reservedAnnotationPrefix) {
			return &ValidationError{
				Code: CodeReservedAnnotation, Annotation: key,
				Message: `keys under "` + reservedAnnotationPrefix + `" are reserved and cannot be set by user config`,
			}
		}
	}

	return nil
}
