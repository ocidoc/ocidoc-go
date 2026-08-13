// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: Copyright 2026 WoozyMasta
// Source: github.com/ocidoc/ocidoc-go

package markdown

import (
	"net/url"
	"sort"
	"strings"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/text"
)

// LocalTargets parses content and returns the deduplicated,
// sorted URL paths of local link and image destinations.
// It deliberately performs no filesystem access:
// source-root confinement and symlink resolution belong to the caller's source-path policy.
//
// Skipped:
//
//   - fragment-only ("#anchor") and empty destinations;
//   - destinations with a URL scheme or a protocol-relative "//" prefix (external);
func LocalTargets(content []byte) []string {
	doc := goldmark.DefaultParser().Parse(text.NewReader(content))

	targets := make(map[string]struct{})

	_ = ast.Walk(doc, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}

		dest, ok := destinationOf(n)
		if !ok {
			return ast.WalkContinue, nil
		}

		target, local := classify(string(dest))
		if !local {
			return ast.WalkContinue, nil
		}

		targets[target] = struct{}{}

		return ast.WalkContinue, nil
	})

	sorted := make([]string, 0, len(targets))
	for target := range targets {
		sorted = append(sorted, target)
	}

	sort.Strings(sorted)

	return sorted
}

// destinationOf returns the link/image destination byte slice of n, if n carries one.
func destinationOf(n ast.Node) ([]byte, bool) {
	switch node := n.(type) { //nolint:exhaustive // only Link/Image carry a destination.
	case *ast.Link:
		return node.Destination, true

	case *ast.Image:
		return node.Destination, true

	default:
		return nil, false
	}
}

// classify reports whether dest is a local reference worth resolving, and the URL path to resolve.
func classify(dest string) (target string, local bool) {
	if dest == "" || strings.HasPrefix(dest, "#") || strings.HasPrefix(dest, "//") {
		return "", false
	}

	u, err := url.Parse(dest)
	if err != nil || u.Scheme != "" || u.Path == "" {
		// Unparseable, scheme-qualified (http:, mailto:, ...)
		// or path-less (query/fragment only) destinations are external
		// or meaningless here, not local documents or assets.
		return "", false
	}

	return u.Path, true
}
