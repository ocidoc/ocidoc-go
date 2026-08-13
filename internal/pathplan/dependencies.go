// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: Copyright 2026 WoozyMasta
// Source: github.com/ocidoc/ocidoc-go

package pathplan

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"sort"

	"github.com/woozymasta/pathrules"

	"github.com/ocidoc/ocidoc-go/internal/markdown"
	"github.com/ocidoc/ocidoc-go/internal/sourcepath"
	"github.com/ocidoc/ocidoc-go/spec"
)

// markdownExtensions identifies which owned files get parsed for local dependencies.
var markdownExtensions = []string{".md", ".markdown", ".mdown"}

// DependencyOptions controls dependency filtering and invalid-reference handling.
// Ignore must be the same compiled global matcher used by Plan.
type DependencyOptions struct {
	// Ignore filters discovered local dependencies.
	Ignore *pathrules.Matcher

	// Strict turns missing or invalid local dependencies into errors.
	Strict bool
}

// DiscoverDependencies extends ownership with local documents
// and assets referenced from every owned Markdown file's links and images,
// recursively (a discovered Markdown dependency is itself scanned).
//
// A reference to a path some component already owns -
// whether from the original component rules or discovered earlier in this same call -
// is left exactly where it is:
// a dependency already explicitly owned by another component is never duplicated.
// A previously-unowned path referenced, in the same discovery round,
// by documents belonging to two different components is an *OwnershipConflictError,
// exactly like a direct component-rule overlap.
//
// Missing and non-file local targets produce deterministic warnings unless Strict is set,
// in which case they fail with spec.ErrInvalid.
// Root escapes and symlink cycles are always fatal. ownership is not mutated.
func DiscoverDependencies(
	root string,
	ownership Ownership,
	opts DependencyOptions,
) (Ownership, []string, error) {
	resolver, err := sourcepath.New(root)
	if err != nil {
		return nil, nil, err
	}

	result := make(Ownership, len(ownership))
	owner := make(map[string]spec.ComponentType)
	warningSet := make(map[string]struct{})

	frontier := make(map[string]spec.ComponentType)

	for component, paths := range ownership {
		result[component] = slices.Clone(paths)

		for _, p := range paths {
			owner[p] = component
			frontier[p] = component
		}
	}

	for len(frontier) > 0 {
		next, err := discoverWave(resolver, frontier, owner, opts, warningSet)
		if err != nil {
			return nil, nil, err
		}

		for path, component := range next {
			owner[path] = component
			result[component] = append(result[component], path)
		}

		frontier = next
	}

	for component := range result {
		sort.Strings(result[component])
	}

	allPaths := make([]string, 0)

	for _, paths := range result {
		allPaths = append(allPaths, paths...)
	}

	if err := spec.ValidateBundlePaths(allPaths); err != nil {
		return nil, nil, err
	}

	warnings := make([]string, 0, len(warningSet))
	for warning := range warningSet {
		warnings = append(warnings, warning)
	}
	sort.Strings(warnings)

	return result, warnings, nil
}

// discoverWave parses every Markdown file in frontier
// and returns the previously-unowned (per owner) local references it found,
// each tentatively assigned to the component whose document referenced it.
// A path claimed by two different components within this same wave is an *OwnershipConflictError.
func discoverWave(
	resolver *sourcepath.Resolver,
	frontier map[string]spec.ComponentType,
	owner map[string]spec.ComponentType,
	opts DependencyOptions,
	warnings map[string]struct{},
) (map[string]spec.ComponentType, error) {
	paths := make([]string, 0, len(frontier))
	for p := range frontier {
		paths = append(paths, p)
	}

	sort.Strings(paths)

	claims := make(map[string][]spec.ComponentType)

	for _, p := range paths {
		if !isMarkdown(p) {
			continue
		}

		component := frontier[p]

		resolvedDocument, err := resolver.Resolve(p)
		if err != nil {
			return nil, err
		}
		if resolvedDocument.Kind != sourcepath.KindFile {
			return nil, fmt.Errorf("%w: owned Markdown path %q is no longer a regular file", spec.ErrInvalid, p)
		}

		//nolint:gosec // sourcepath confines and dereferences the path.
		content, err := os.ReadFile(resolvedDocument.File.SourcePath)
		if err != nil {
			return nil, fmt.Errorf("read %q: %w", p, err)
		}

		for _, target := range markdown.LocalTargets(content) {
			resolved, err := resolver.ResolveReference(p, target)
			if err != nil {
				return nil, fmt.Errorf("%s: reference %q: %w", p, target, err)
			}
			ref := resolved.File.BundlePath
			if opts.Ignore != nil && opts.Ignore.Excluded(ref, false) {
				continue
			}
			if resolved.Kind != sourcepath.KindFile {
				warning := invalidDependencyMessage(p, target, resolved.Kind)
				if opts.Strict {
					return nil, fmt.Errorf("%w: %s", spec.ErrInvalid, warning)
				}
				warnings[warning] = struct{}{}
				continue
			}
			if _, owned := owner[ref]; owned {
				continue
			}

			claims[ref] = append(claims[ref], component)
		}
	}

	next := make(map[string]spec.ComponentType, len(claims))

	refs := make([]string, 0, len(claims))
	for ref := range claims {
		refs = append(refs, ref)
	}

	sort.Strings(refs)

	for _, ref := range refs {
		components := uniqueSorted(claims[ref])
		if len(components) > 1 {
			return nil, &OwnershipConflictError{Path: ref, Components: components}
		}

		next[ref] = components[0]
	}

	return next, nil
}

func invalidDependencyMessage(referrer, target string, kind sourcepath.Kind) string {
	reason := "is not a regular file"
	if kind == sourcepath.KindMissing {
		reason = "does not exist"
	}

	return fmt.Sprintf("invalid local dependency %q from %q: %s", target, referrer, reason)
}

// isMarkdown reports whether path has a recognized Markdown extension.
func isMarkdown(path string) bool {
	return slices.Contains(markdownExtensions, filepath.Ext(path))
}

// uniqueSorted returns components deduplicated and sorted.
func uniqueSorted(components []spec.ComponentType) []spec.ComponentType {
	unique := slices.Clone(components)
	slices.Sort(unique)

	return slices.Compact(unique)
}
