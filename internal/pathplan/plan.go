// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: Copyright 2026 WoozyMasta
// Source: github.com/ocidoc/ocidoc-go

package pathplan

import (
	"fmt"
	"slices"
	"sort"

	"github.com/ocidoc/ocidoc-go/internal/sourcepath"
	"github.com/ocidoc/ocidoc-go/spec"
)

// Ownership maps each component to its matched bundle paths.
// Each component's paths are sorted; the map itself has no meaningful order -
// component ordering carries no semantics.
type Ownership map[spec.ComponentType][]string

// OwnershipConflictError reports a source file matched by more than one component:
// a file may belong to only one component, so any overlap is always an error.
type OwnershipConflictError struct {
	// Path is the bundle-relative path with multiple owners.
	Path string

	// Components lists the conflicting component types.
	Components []spec.ComponentType
}

// Error implements the error interface.
func (e *OwnershipConflictError) Error() string {
	return fmt.Sprintf("path %q matched more than one component: %v", e.Path, e.Components)
}

// Unwrap makes OwnershipConflictError match errors.Is(err, spec.ErrInvalid).
func (e *OwnershipConflictError) Unwrap() error {
	return spec.ErrInvalid
}

// Plan walks the source tree rooted at root and resolves component ownership for regular files,
// applying the global ignore matcher before the component matchers.
// Symlinked files and directories are safely dereferenced
// while their logical bundle paths are preserved; escapes and cycles are rejected.
//
// Directory-level ignore short-circuiting is deliberately not implemented:
// every directory is visited so that a later restoring ("!"-negated)
// ignore rule can still reach files under a directory an earlier rule would otherwise exclude.
// This trades walk performance for not guessing at unspecified short-circuit semantics.
//
// The returned ownership is validated with spec.ValidateBundlePaths before being returned,
// so it is always collision-free.
func Plan(root string, matchers *Matchers) (Ownership, error) {
	ownership := make(Ownership)

	resolver, err := sourcepath.New(root)
	if err != nil {
		return nil, err
	}

	walkErr := resolver.Walk(func(file sourcepath.File) error {
		if matchers.Ignore != nil && matchers.Ignore.Excluded(file.BundlePath, false) {
			return nil
		}

		owner, conflict := resolveOwner(matchers, file.BundlePath)
		if conflict != nil {
			return conflict
		}

		if owner == "" {
			return nil
		}

		ownership[owner] = append(ownership[owner], file.BundlePath)

		return nil
	})
	if walkErr != nil {
		return nil, walkErr
	}

	allPaths := make([]string, 0)

	for name := range ownership {
		sort.Strings(ownership[name])
		allPaths = append(allPaths, ownership[name]...)
	}

	if err := spec.ValidateBundlePaths(allPaths); err != nil {
		return nil, err
	}

	return ownership, nil
}

// resolveOwner returns the single component matching relSlash,
// or a non-nil *OwnershipConflictError if more than one component matches.
// An empty owner with a nil error means the path is unowned.
func resolveOwner(matchers *Matchers, relSlash string) (spec.ComponentType, *OwnershipConflictError) {
	var (
		owner   spec.ComponentType
		matched []spec.ComponentType
	)

	for name, matcher := range matchers.Components {
		if matcher.Included(relSlash, false) {
			owner = name

			matched = append(matched, name)
		}
	}

	if len(matched) <= 1 {
		return owner, nil
	}

	slices.Sort(matched)

	return "", &OwnershipConflictError{Path: relSlash, Components: matched}
}
