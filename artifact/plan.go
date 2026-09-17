// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: Copyright 2026 WoozyMasta
// Source: github.com/ocidoc/ocidoc-go

package artifact

import (
	"context"
	"fmt"
	"maps"
	"slices"
	"sort"
	"strings"

	"github.com/ocidoc/ocidoc-go/internal/pathplan"
	"github.com/ocidoc/ocidoc-go/spec"
)

// PlanOptions carries Plan's inputs beyond the source tree itself:
// an explicit configuration path, and caller-provided overrides
// that win over the loaded build config's own values.
//
// A zero-valued field means "no override was given":
// Plan cannot tell an explicit empty override from an absent flag,
// so the caller must set only the fields it actually received a flag for.
type PlanOptions struct {
	// Annotations overlays user annotations from the build configuration.
	Annotations map[string]string

	// Entrypoints overlays component entrypoints from the build configuration.
	Entrypoints map[spec.ComponentType]string

	// Settings overrides the loaded config's strict/compression settings, field by field:
	// a nil Settings.Strict or Settings.Compression means "no override for this field"
	// mirroring spec.BuildSettings' own omitted-vs-explicit convention.
	// Settings itself may be nil for "no overrides at all."
	Settings *spec.BuildSettings

	// Document overlays the document identity from the build configuration.
	Document spec.DocumentSettings

	// ConfigPath selects a build configuration relative to Root; empty uses discovery.
	ConfigPath string

	// Ignore rules are appended after the loaded config's own "ignore"
	// list, not merged or deduplicated.
	Ignore []string
}

// BuildPlan is the complete, side-effect-free result of planning a build from a source tree,
// computed without creating any blobs.
type BuildPlan struct {
	// Config is the loaded build configuration after caller overrides are merged.
	Config *spec.BuildConfig

	// Annotations is the validated root annotation set for the artifact manifest.
	Annotations map[string]string

	// Ownership maps each component to its sorted bundle-relative file paths.
	Ownership map[spec.ComponentType][]string

	// Entrypoints maps components to their resolved bundle-relative entrypoint paths.
	Entrypoints map[spec.ComponentType]string

	// Locales maps components to locale plans with sorted, deduplicated document paths.
	Locales map[spec.ComponentType]map[string]LocalePlan

	// Document is the effective document identity with defaults applied.
	Document spec.DocumentSettings

	// Warnings reports non-fatal planning conditions in non-strict mode.
	Warnings []string

	// Settings is the effective build settings with defaults applied.
	Settings spec.EffectiveSettings
}

// LocalePlan is one component locale after source rules and entrypoints resolve.
type LocalePlan struct {
	// Entrypoint is the resolved primary document for this locale.
	Entrypoint string

	// Paths contains sorted bundle-relative document paths in this locale.
	Paths []string

	// Default marks the effective fallback locale for the component.
	Default bool
}

// EmptyComponentsError reports declared components
// with no planned files while spec.EffectiveSettings.Strict is true:
// every declared component must contain at least one file in strict mode.
// In non-strict mode, the same condition is a BuildPlan.Warnings entry instead.
type EmptyComponentsError struct {
	// Components lists the declared components that matched no files.
	Components []spec.ComponentType
}

// Error implements the error interface.
func (e *EmptyComponentsError) Error() string {
	return fmt.Sprintf("empty components in strict mode: %v", e.Components)
}

// Unwrap makes EmptyComponentsError match errors.Is(err, spec.ErrInvalid).
func (e *EmptyComponentsError) Unwrap() error {
	return spec.ErrInvalid
}

// Plan loads the build config for root (LoadBuildConfig), applies opts' overrides,
// resolves component ownership - including Markdown dependencies -
// and entrypoints, and returns the complete result.
// It performs no registry requests and writes nothing;
// it only reads under root. ctx is checked during source walks and dependency waves.
//
// Plan returns an error when:
//
//   - the build config fails to load or validate (LoadBuildConfig);
//   - the merged annotations use the reserved "org.ocidoc." namespace (spec.ValidateUserAnnotations);
//   - component ownership cannot be resolved
//     (pathplan.Plan, pathplan.DiscoverDependencies: path collisions, ownership conflicts, Markdown errors);
//   - an explicit entrypoint does not belong to its component (pathplan.ResolveEntrypoints);
//   - every component ends up with no planned files,
//     regardless of strict mode - an artifact needs at least one component;
//   - strict mode is set and any *declared* component has no planned files (*EmptyComponentsError).
func Plan(ctx context.Context, root string, opts PlanOptions) (*BuildPlan, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	cfg, configSource, err := loadBuildConfig(root, opts.ConfigPath)
	if err != nil {
		return nil, err
	}

	cfg = cloneBuildConfig(cfg)
	cfg.Annotations = mergeStrings(cfg.Annotations, opts.Annotations)
	for component, entrypoint := range opts.Entrypoints {
		selected, ok := cfg.Components[component]
		if !ok {
			return nil, fmt.Errorf("%w: entrypoint override references undeclared component %q", spec.ErrInvalid, component)
		}

		selected.Entrypoint = entrypoint
		cfg.Components[component] = selected
	}
	cfg.Document = mergeDocument(cfg.Document, opts.Document)
	cfg.Ignore = append(cfg.Ignore, opts.Ignore...)
	cfg.Settings = *mergeSettings(cfg.Settings, opts.Settings)
	if err := spec.ValidateBuildConfig(cfg); err != nil {
		return nil, err
	}
	if err := spec.ValidateUserAnnotations(cfg.Annotations); err != nil {
		return nil, err
	}

	settings := spec.ResolveSettings(&cfg.Settings)
	cfg.Settings = effectiveBuildSettings(settings)
	cfg.Document = spec.ResolveDocument(cfg.Document)

	matchers, err := pathplan.Compile(cfg)
	if err != nil {
		return nil, err
	}

	ownership, err := pathplan.PlanContext(ctx, root, matchers)
	if err != nil {
		return nil, err
	}

	ownership, dependencyWarnings, err := pathplan.DiscoverDependenciesContext(ctx, root, ownership, pathplan.DependencyOptions{
		Ignore: matchers.Ignore,
		Strict: settings.Strict,
	})
	if err != nil {
		return nil, err
	}

	if len(ownership) == 0 {
		return nil, fmt.Errorf("%w: no component matched any file", spec.ErrInvalid)
	}

	explicitEntrypoints := make(map[spec.ComponentType]string, len(cfg.Components))
	for component, config := range cfg.Components {
		if config.Entrypoint != "" {
			explicitEntrypoints[component] = config.Entrypoint
		}
	}

	entrypoints, err := pathplan.ResolveEntrypoints(ownership, explicitEntrypoints)
	if err != nil {
		return nil, err
	}

	locales, err := pathplan.ClassifyLocales(cfg.Components, ownership)
	if err != nil {
		return nil, err
	}

	localePlans, localeWarnings, err := resolveLocalePlans(cfg.Components, entrypoints, locales)
	if err != nil {
		return nil, err
	}

	if settings.Strict && len(localeWarnings) > 0 {
		return nil, fmt.Errorf("%w: empty locales in strict mode: %v", spec.ErrInvalid, localeWarnings)
	}

	emptyWarnings, err := emptyComponentWarnings(
		cfg.Components, ownership, settings.Strict, configSource == "embedded default",
	)
	if err != nil {
		return nil, err
	}
	warnings := slices.Concat(dependencyWarnings, emptyWarnings, localeWarnings)

	return &BuildPlan{
		Config:      cfg,
		Settings:    settings,
		Document:    cfg.Document,
		Annotations: cfg.Annotations,
		Ownership:   map[spec.ComponentType][]string(ownership),
		Entrypoints: entrypoints,
		Warnings:    warnings,
		Locales:     localePlans,
	}, nil
}

// resolveLocalePlans resolves locale ownership, fallback, and entrypoints.
func resolveLocalePlans(
	components map[spec.ComponentType]spec.ComponentBuildConfig,
	entrypoints map[spec.ComponentType]string,
	locales map[spec.ComponentType]map[string][]string,
) (map[spec.ComponentType]map[string]LocalePlan, []string, error) {
	resolved := make(map[spec.ComponentType]map[string]LocalePlan, len(locales))
	var warnings []string

	for component, componentLocales := range locales {
		if len(componentLocales) == 0 {
			continue
		}

		keys := make([]string, 0, len(componentLocales))
		for key := range componentLocales {
			keys = append(keys, key)
		}
		sort.Strings(keys)

		defaultLocale := keys[0]
		for _, key := range keys {
			if components[component].Locales[key].Default {
				defaultLocale = key
				break
			}
		}

		plans := make(map[string]LocalePlan, len(keys))
		for _, key := range keys {
			paths := append([]string(nil), componentLocales[key]...)
			plan := LocalePlan{Default: key == defaultLocale, Paths: paths}
			if len(paths) == 0 {
				warnings = append(warnings, fmt.Sprintf("component %q locale %q matched no documents", component, key))
				plans[key] = plan
				continue
			}

			localeConfig := components[component].Locales[key]
			entrypoint := strings.TrimPrefix(localeConfig.Entrypoint, "/")
			if entrypoint != "" && !slices.Contains(paths, entrypoint) {
				return nil, nil, fmt.Errorf(
					"%w: locale entrypoint %q for component %q and locale %q is not among locale paths",
					spec.ErrInvalid, localeConfig.Entrypoint, component, key,
				)
			}

			if entrypoint == "" && slices.Contains(paths, entrypoints[component]) {
				entrypoint = entrypoints[component]
			}
			if entrypoint == "" {
				entrypoint, _ = pathplan.DetectEntrypoint(component, paths)
			}

			plan.Entrypoint = entrypoint
			plans[key] = plan
		}

		resolved[component] = plans
	}

	return resolved, warnings, nil
}

// cloneBuildConfig returns a deep copy of a build configuration.
func cloneBuildConfig(cfg *spec.BuildConfig) *spec.BuildConfig {
	copyConfig := *cfg
	copyConfig.Components = make(map[spec.ComponentType]spec.ComponentBuildConfig, len(cfg.Components))

	for name, component := range cfg.Components {
		component.Paths = append([]string(nil), component.Paths...)
		locales := component.Locales
		component.Locales = make(map[string]spec.BuildLocaleConfig, len(locales))

		for locale, config := range locales {
			config.Paths = append([]string(nil), config.Paths...)
			component.Locales[locale] = config
		}

		copyConfig.Components[name] = component
	}

	copyConfig.Annotations = mergeStrings(cfg.Annotations, nil)
	copyConfig.Ignore = append([]string(nil), cfg.Ignore...)

	if cfg.Settings.Compression != nil {
		compression := *cfg.Settings.Compression
		if compression.Level != nil {
			level := *compression.Level
			compression.Level = &level
		}
		copyConfig.Settings.Compression = &compression
	}

	if cfg.Settings.Strict != nil {
		strict := *cfg.Settings.Strict
		copyConfig.Settings.Strict = &strict
	}

	return &copyConfig
}

// effectiveBuildSettings converts resolved settings back to public config form.
func effectiveBuildSettings(settings spec.EffectiveSettings) spec.BuildSettings {
	strict := settings.Strict
	level := settings.Compression.Level
	return spec.BuildSettings{
		Strict: &strict,
		Compression: &spec.CompressionSettings{
			Type:  settings.Compression.Type,
			Level: &level,
		},
	}
}

// emptyComponentWarnings finds declared components with no planned files.
// The embedded default config declares optional conventional components,
// so their absence is silent unless strict mode was explicitly requested.
func emptyComponentWarnings(
	declared map[spec.ComponentType]spec.ComponentBuildConfig,
	ownership pathplan.Ownership,
	strict bool,
	embeddedDefault bool,
) ([]string, error) {
	var empty []spec.ComponentType

	for name := range declared {
		if len(ownership[name]) == 0 {
			empty = append(empty, name)
		}
	}

	if len(empty) == 0 {
		return nil, nil
	}

	slices.Sort(empty)

	if strict {
		return nil, &EmptyComponentsError{Components: empty}
	}
	if embeddedDefault {
		return nil, nil
	}

	warnings := make([]string, 0, len(empty))
	for _, name := range empty {
		warnings = append(warnings, fmt.Sprintf("component %q matched no files", name))
	}

	return warnings, nil
}

// mergeDocument overlays override's non-empty fields onto base.
func mergeDocument(base, override spec.DocumentSettings) spec.DocumentSettings {
	if override.ID != "" {
		base.ID = override.ID
	}

	if override.Variant != "" {
		base.Variant = override.Variant
	}

	return base
}

// mergeSettings overlays override's non-nil fields onto a copy of base,
// field by field, recursing into Compression's own Type/Level
// so a caller can override just one field without wiping the other back to its zero value;
// override itself may be nil for "no overrides at all."
func mergeSettings(base spec.BuildSettings, override *spec.BuildSettings) *spec.BuildSettings {
	if override == nil {
		return &base
	}

	if override.Strict != nil {
		base.Strict = override.Strict
	}

	if override.Compression != nil {
		base.Compression = mergeCompressionSettings(base.Compression, override.Compression)
	}

	return &base
}

// mergeCompressionSettings overlays override's non-zero fields onto a copy of base
// (or a zero CompressionSettings if base is nil).
func mergeCompressionSettings(base, override *spec.CompressionSettings) *spec.CompressionSettings {
	merged := spec.CompressionSettings{}
	if base != nil {
		merged = *base
	}

	if override.Type != "" {
		merged.Type = override.Type
	}

	if override.Level != nil {
		merged.Level = override.Level
	}

	return &merged
}

// mergeStrings overlays override onto base, key by key.
func mergeStrings(base, override map[string]string) map[string]string {
	merged := make(map[string]string, len(base)+len(override))

	maps.Copy(merged, base)
	maps.Copy(merged, override)

	return merged
}
