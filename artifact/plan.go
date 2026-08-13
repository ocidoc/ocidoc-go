// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: Copyright 2026 WoozyMasta
// Source: github.com/ocidoc/ocidoc-go

package artifact

import (
	"context"
	"fmt"
	"maps"
	"slices"

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

	// Document is the effective document identity with defaults applied.
	Document spec.DocumentSettings

	// Warnings reports non-fatal planning conditions in non-strict mode.
	Warnings []string

	// Settings is the effective build settings with defaults applied.
	Settings spec.EffectiveSettings
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
// it only reads under root. ctx is checked once before that work begins;
// Plan's own work is comparatively fast, so it is not checked again partway through.
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

	cfg, err := LoadBuildConfig(root, opts.ConfigPath)
	if err != nil {
		return nil, err
	}

	annotations := mergeStrings(cfg.Annotations, opts.Annotations)
	if err := spec.ValidateUserAnnotations(annotations); err != nil {
		return nil, err
	}

	if len(opts.Ignore) > 0 {
		cfg.Ignore = append(append([]string{}, cfg.Ignore...), opts.Ignore...)
	}

	matchers, err := pathplan.Compile(cfg)
	if err != nil {
		return nil, err
	}

	ownership, err := pathplan.Plan(root, matchers)
	if err != nil {
		return nil, err
	}

	settings := spec.ResolveSettings(mergeSettings(cfg.Settings, opts.Settings))

	ownership, dependencyWarnings, err := pathplan.DiscoverDependencies(root, ownership, pathplan.DependencyOptions{
		Ignore: matchers.Ignore,
		Strict: settings.Strict,
	})
	if err != nil {
		return nil, err
	}

	if len(ownership) == 0 {
		return nil, fmt.Errorf("%w: no component matched any file", spec.ErrInvalid)
	}

	entrypointOverrides := mergeEntrypoints(cfg.Entrypoints, opts.Entrypoints)

	entrypoints, err := pathplan.ResolveEntrypoints(ownership, entrypointOverrides)
	if err != nil {
		return nil, err
	}

	emptyWarnings, err := emptyComponentWarnings(cfg.Components, ownership, settings.Strict)
	if err != nil {
		return nil, err
	}
	warnings := slices.Concat(dependencyWarnings, emptyWarnings)

	return &BuildPlan{
		Config:      cfg,
		Settings:    settings,
		Document:    spec.ResolveDocument(mergeDocument(cfg.Document, opts.Document)),
		Annotations: annotations,
		Ownership:   map[spec.ComponentType][]string(ownership),
		Entrypoints: entrypoints,
		Warnings:    warnings,
	}, nil
}

// emptyComponentWarnings finds declared components with no planned files.
// In strict mode that is an *EmptyComponentsError;
// otherwise it is returned as sorted, human-readable warning strings.
func emptyComponentWarnings(
	declared map[spec.ComponentType][]string,
	ownership pathplan.Ownership,
	strict bool,
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

	if override.Language != "" {
		base.Language = override.Language
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

// mergeEntrypoints overlays override onto base, key by key.
func mergeEntrypoints(
	base map[spec.ComponentType]string,
	override map[spec.ComponentType]string,
) map[spec.ComponentType]string {
	merged := make(map[spec.ComponentType]string, len(base)+len(override))

	maps.Copy(merged, base)
	maps.Copy(merged, override)

	return merged
}

// mergeStrings overlays override onto base, key by key.
func mergeStrings(base, override map[string]string) map[string]string {
	merged := make(map[string]string, len(base)+len(override))

	maps.Copy(merged, base)
	maps.Copy(merged, override)

	return merged
}
