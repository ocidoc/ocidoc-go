// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: Copyright 2026 WoozyMasta
// Source: github.com/ocidoc/ocidoc-go

package spec

import "strings"

// reservedAnnotationPrefix is the namespace reserved for the implementation
// and specification (see AnnotationSchema and related constants).
// User-provided annotations cannot use it.
const reservedAnnotationPrefix = "org.ocidoc."

type bundlePathNode struct {
	children map[string]*bundlePathNode
	path     string
	anyPath  string
}

// IsMarkdownPath reports whether path has a supported Markdown extension.
func IsMarkdownPath(path string) bool {
	index := strings.LastIndex(path, ".")
	if index < 0 {
		return false
	}

	switch strings.ToLower(path[index:]) {
	case ".md", ".markdown", ".mdown", ".mkd":
		return true
	default:
		return false
	}
}

// IsDocumentPath reports whether path has a supported document name or extension.
// It intentionally excludes assets and source files even when they live below docs/.
func IsDocumentPath(path string) bool {
	if IsMarkdownPath(path) {
		return true
	}

	lower := strings.ToLower(path)
	for _, extension := range []string{".txt", ".html", ".htm"} {
		if strings.HasSuffix(lower, extension) {
			return true
		}
	}

	name := lower
	if slash := strings.LastIndexByte(name, '/'); slash >= 0 {
		name = name[slash+1:]
	}
	if strings.Contains(name, ".") {
		return false
	}

	switch name {
	case "readme", "license", "licence", "copying", "notice", "legal",
		"copyright", "changelog", "changes", "history", "news", "security",
		"support", "contributing", "code_of_conduct", "code-of-conduct",
		"release_notes", "release-notes":
		return true
	default:
		return false
	}
}

// ValidateBundlePaths reports whether paths, taken together as one global virtual tree,
// are free of exact duplicates and case-insensitive collisions.
// Components do not have overlay semantics:
// the same normalized path cannot occur twice, even across different components.
//
// Each path must already be individually valid;
// ValidateBundlePaths returns the first ValidateBundlePath error
// it finds before checking for collisions.
func ValidateBundlePaths(paths []string) error {
	root := &bundlePathNode{children: make(map[string]*bundlePathNode)}

	for _, path := range paths {
		if err := ValidateBundlePath(path); err != nil {
			return err
		}

		node := root
		for segment := range strings.SplitSeq(path, "/") {
			if node.path != "" {
				return pathCollision(path, node.path, "ancestor")
			}

			key := strings.ToLower(segment)
			child := node.children[key]
			if child == nil {
				child = &bundlePathNode{children: make(map[string]*bundlePathNode), anyPath: path}
				node.children[key] = child
			}
			node = child
		}

		if node.path != "" {
			if node.path == path {
				return &ValidationError{Code: CodePathCollision, Path: path, Message: "duplicate path"}
			}

			return pathCollision(path, node.path, "case-insensitive")
		}
		if node.anyPath != "" && len(node.children) > 0 {
			return pathCollision(path, node.anyPath, "descendant")
		}

		node.path = path
		if node.anyPath == "" {
			node.anyPath = path
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

// pathCollision creates a validation error for two paths that collide.
func pathCollision(path, original, kind string) error {
	message := kind + " path collision with " + original
	if kind == "case-insensitive" {
		message = "case-insensitive collision with " + original
	}

	return &ValidationError{Code: CodePathCollision, Path: path, Message: message}
}
