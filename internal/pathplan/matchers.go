// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: Copyright 2026 WoozyMasta
// Source: github.com/ocidoc/ocidoc-go

package pathplan

import (
	"fmt"
	"strings"

	"github.com/woozymasta/pathrules"

	"github.com/ocidoc/ocidoc-go/spec"
)

// ignoreParseOptions is the global ignore list's polarity:
// a plain rule excludes, a negated rule restores (includes).
// This is pathrules' own zero-value ParseOptions,
// spelled out for clarity rather than relied on implicitly.
var ignoreParseOptions = pathrules.ParseOptions{
	PlainAction:   pathrules.ActionExclude,
	NegatedAction: pathrules.ActionInclude,
}

// componentParseOptions is the inverse polarity used for a component's own rule list:
// a plain rule *includes* a path into the component;
// a negated rule excludes it from that component.
var componentParseOptions = pathrules.ParseOptions{
	PlainAction:   pathrules.ActionInclude,
	NegatedAction: pathrules.ActionExclude,
}

// buildConfigMatcherOptions is shared by the ignore and component matchers:
// both rule sets are written by the OCIDoc user directly in the build config,
// so both may use pathrules brace alternation ("{md,txt}") and backslash escaping.
var buildConfigMatcherOptions = pathrules.MatcherOptions{
	EnableBraceExpansion: true,
	EnableEscaping:       true,
}

// Matchers holds the compiled global-ignore and per-component matchers for one build.
// Ignore is nil when the build config declares no ignore rules.
//
// There is deliberately no automatic ".gitignore" layer:
// the build config is a whitelist
// (every component rule is anchored under an explicit path, e.g. "/docs/**", not "/**"),
// so noise like "node_modules" already cannot enter through
// a component rule unless the rule's own author points at it.
// The one narrow case that needs carving out
// (e.g. a doc generator that nests "node_modules" inside "docs/")
// is exactly what the build config's own "ignore" list and per-component "!" negation are for - explicit,
// user-authored, no external file with its own semantics to reconcile.
type Matchers struct {
	// Ignore filters paths before component matching; nil means no global rules.
	Ignore *pathrules.Matcher
	// Components maps each component to its compiled inclusion matcher.
	Components map[spec.ComponentType]*pathrules.Matcher
}

// Compile compiles cfg's ignore and component rules into matchers.
//
// cfg is assumed to have already passed spec.ValidateBuildConfig:
// this function does not repeat component-name or non-empty-rule-list checks.
func Compile(cfg *spec.BuildConfig) (*Matchers, error) {
	matchers := &Matchers{
		Components: make(map[spec.ComponentType]*pathrules.Matcher, len(cfg.Components)),
	}

	if len(cfg.Ignore) > 0 {
		ignore, err := compileRules(cfg.Ignore, ignoreParseOptions, pathrules.ActionInclude)
		if err != nil {
			return nil, fmt.Errorf("compile ignore rules: %w", err)
		}

		matchers.Ignore = ignore
	}

	for name, rules := range cfg.Components {
		matcher, err := compileRules(rules, componentParseOptions, pathrules.ActionExclude)
		if err != nil {
			return nil, fmt.Errorf("compile component %q rules: %w", name, err)
		}

		matchers.Components[name] = matcher
	}

	return matchers, nil
}

// compileRules parses patterns (one rule per element) with parseOpts
// and compiles them into a matcher that falls back to defaultAction
// when no rule matches a given path.
func compileRules(patterns []string, parseOpts pathrules.ParseOptions, defaultAction pathrules.Action) (*pathrules.Matcher, error) {
	rules, err := pathrules.ParseRulesString(strings.Join(patterns, "\n"), parseOpts)
	if err != nil {
		return nil, err
	}

	opts := buildConfigMatcherOptions
	opts.DefaultAction = defaultAction

	return pathrules.NewMatcher(rules, opts)
}
