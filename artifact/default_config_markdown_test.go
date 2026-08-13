// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: Copyright 2026 WoozyMasta
// Source: github.com/ocidoc/ocidoc-go

package artifact

import (
	"testing"

	"github.com/ocidoc/ocidoc-go/internal/pathplan"
	"github.com/ocidoc/ocidoc-go/spec"
)

// compileDefaultMatchers compiles the embedded default build config's own component rules,
// the same way Plan does for a source tree with no ocidoc.yaml.
func compileDefaultMatchers(t *testing.T) *pathplan.Matchers {
	t.Helper()

	cfg, err := DefaultBuildConfig()
	if err != nil {
		t.Fatalf("DefaultBuildConfig: %v", err)
	}

	matchers, err := pathplan.Compile(cfg)
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}

	return matchers
}

// TestDefaultConfigMatchesConventionalMarkdownAndTextFiles confirms conventional extensionless,
// Markdown and plain-text filenames remain automatically matched by the embedded default component rules.
func TestDefaultConfigMatchesConventionalMarkdownAndTextFiles(t *testing.T) {
	matchers := compileDefaultMatchers(t)

	cases := map[spec.ComponentType][]string{
		spec.ComponentDocumentation: {
			"README", "README.md", "README.markdown", "README.mdown", "README.txt",
		},
		spec.ComponentLicense: {
			"LICENSE", "LICENSE.md", "LICENSE.txt",
		},
		spec.ComponentChangelog: {
			"CHANGELOG", "CHANGELOG.md", "CHANGELOG.txt",
		},
	}

	for component, paths := range cases {
		matcher := matchers.Components[component]
		for _, path := range paths {
			if !matcher.Included(path, false) {
				t.Errorf("expected %q to be matched by the default %q component rules", path, component)
			}
		}
	}
}

// TestDefaultConfigDoesNotMatchRSTOrAsciiDoc confirms .rst/.adoc/.asciidoc
// are not conventional filename alternatives in the embedded default component rules,
// even though the base names they decorate are.
func TestDefaultConfigDoesNotMatchRSTOrAsciiDoc(t *testing.T) {
	matchers := compileDefaultMatchers(t)

	cases := map[spec.ComponentType][]string{
		spec.ComponentDocumentation: {
			"README.rst", "README.adoc", "README.asciidoc",
		},
		spec.ComponentLicense: {
			"LICENSE.rst", "LICENSE.adoc", "LICENSE.asciidoc",
		},
		spec.ComponentChangelog: {
			"CHANGELOG.rst", "CHANGELOG.adoc", "CHANGELOG.asciidoc",
		},
	}

	for component, paths := range cases {
		matcher := matchers.Components[component]
		for _, path := range paths {
			if matcher.Included(path, false) {
				t.Errorf("did not expect %q to be matched by the default %q component rules", path, component)
			}
		}
	}
}

// TestDefaultConfigDirectoryPatternsRemainFormatAgnostic confirms
// that removing conventional-filename .rst/.adoc/.asciidoc alternatives
// does not affect recursive directory patterns, which stay format-agnostic.
func TestDefaultConfigDirectoryPatternsRemainFormatAgnostic(t *testing.T) {
	matchers := compileDefaultMatchers(t)

	doc := matchers.Components[spec.ComponentDocumentation]

	for _, path := range []string{
		"docs/manual.rst",
		"docs/manual.adoc",
		"docs/manual.pdf",
		"docs/images/diagram.png",
	} {
		if !doc.Included(path, false) {
			t.Errorf("expected %q under docs/** to remain matched regardless of format", path)
		}
	}
}

// TestExplicitPatternsStillPackageArbitraryFormats confirms explicit (non-default)
// component patterns can still select any format;
// no semantic parser is required for explicit inclusion.
func TestExplicitPatternsStillPackageArbitraryFormats(t *testing.T) {
	cfg := &spec.BuildConfig{
		SchemaVersion: spec.SchemaVersion,
		Components: map[spec.ComponentType][]string{
			spec.ComponentDocumentation: {"/README.rst", "/manual.pdf", "/guide.docx", "/includes/**"},
		},
	}

	matchers, err := pathplan.Compile(cfg)
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}

	doc := matchers.Components[spec.ComponentDocumentation]

	for _, path := range []string{"README.rst", "manual.pdf", "guide.docx", "includes/appendix.rst"} {
		if !doc.Included(path, false) {
			t.Errorf("expected explicitly configured %q to be included", path)
		}
	}
}
