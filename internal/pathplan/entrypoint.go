// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: Copyright 2026 WoozyMasta
// Source: github.com/ocidoc/ocidoc-go

package pathplan

import (
	"fmt"
	"slices"
	"strings"

	"github.com/ocidoc/ocidoc-go/spec"
)

// entrypointCandidates lists recommended auto-detection filenames per component type, in priority order.
// A candidate ending in ".*" matches any bundle path with that prefix plus an extension
// (e.g. "LICENSE.*" matches "LICENSE.md").
//
// A component type absent here has no auto-detection candidates:
// a component can exist without an entrypoint.
var entrypointCandidates = map[spec.ComponentType][]string{
	spec.ComponentDocumentation: {"README.md", "README", "README.markdown", "README.mdown", "docs/index.md"},
	spec.ComponentLicense:       {"LICENSE", "LICENSE.*", "LICENCE", "COPYING"},
	spec.ComponentSecurity:      {"SECURITY.md", "SECURITY"},
	spec.ComponentContributing:  {"CONTRIBUTING.md", "CONTRIBUTING"},
	spec.ComponentChangelog:     {"CHANGELOG.md", "CHANGELOG", "CHANGES.md", "HISTORY.md", "NEWS.md"},
	spec.ComponentReleaseNotes:  {"RELEASE_NOTES.md", "RELEASE-NOTES.md"},
}

// textDocumentExtensions backs documentation's
// "first supported text document by lexical order"
// fallback when none of its named candidates match.
var textDocumentExtensions = []string{".md", ".markdown", ".mdown", ".txt"}

// EntrypointError reports an explicit entrypoint that does not belong to its component after planning:
// the path must actually be among that component's own planned files.
type EntrypointError struct {
	// Component is the component declaring the invalid entrypoint.
	Component spec.ComponentType

	// Path is the requested bundle-relative entrypoint path.
	Path string

	// Message explains why Path cannot be used as the entrypoint.
	Message string
}

// Error implements the error interface.
func (e *EntrypointError) Error() string {
	return fmt.Sprintf("entrypoint %q for component %q: %s", e.Path, e.Component, e.Message)
}

// Unwrap makes EntrypointError match errors.Is(err, spec.ErrInvalid).
func (e *EntrypointError) Unwrap() error {
	return spec.ErrInvalid
}

// ResolveEntrypoints picks at most one entrypoint bundle path per component:
// an explicit value from explicit wins when present
// (already root-relative with a leading "/", as written in
// spec.BuildConfig.Entrypoints or a caller-provided override);
// otherwise deterministic auto-detection runs against that component's planned ownership.
//
// An explicit entrypoint for a component with no planned files,
// or a path not among that component's planned files, is an *EntrypointError.
// A component with no explicit value and no matching candidate simply has no entry in the result:
// a component can exist without an entrypoint.
func ResolveEntrypoints(ownership Ownership, explicit map[spec.ComponentType]string) (map[spec.ComponentType]string, error) {
	result := make(map[spec.ComponentType]string, len(explicit))

	for component, path := range explicit {
		files, owned := ownership[component]
		if !owned {
			return nil, &EntrypointError{Component: component, Path: path, Message: "component has no planned files"}
		}

		bundlePath := strings.TrimPrefix(path, "/")
		if !slices.Contains(files, bundlePath) {
			return nil, &EntrypointError{Component: component, Path: path, Message: "not among the component's planned files"}
		}

		result[component] = bundlePath
	}

	for component, files := range ownership {
		if _, explicitlySet := result[component]; explicitlySet {
			continue
		}

		if detected, ok := detectEntrypoint(component, files); ok {
			result[component] = detected
		}
	}

	return result, nil
}

// detectEntrypoint applies entrypointCandidates, in priority order, against files
// (a component's planned ownership, already sorted - see the Ownership doc comment).
// documentation additionally falls back to the first file with a recognized text-document extension.
func detectEntrypoint(component spec.ComponentType, files []string) (string, bool) {
	for _, candidate := range entrypointCandidates[component] {
		for _, path := range files {
			if matchesEntrypointCandidate(candidate, path) {
				return path, true
			}
		}
	}

	if component != spec.ComponentDocumentation {
		return "", false
	}

	for _, path := range files {
		for _, ext := range textDocumentExtensions {
			if strings.HasSuffix(path, ext) {
				return path, true
			}
		}
	}

	return "", false
}

// matchesEntrypointCandidate reports whether path satisfies candidate,
// where a candidate ending in ".*" matches any path sharing its prefix plus a "."
// and at least one more character (an extension).
func matchesEntrypointCandidate(candidate, path string) bool {
	prefix, isExtensionWildcard := strings.CutSuffix(candidate, "*")
	if !isExtensionWildcard {
		return candidate == path
	}

	return strings.HasPrefix(path, prefix) && len(path) > len(prefix)
}
