// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: Copyright 2026 WoozyMasta
// Source: github.com/ocidoc/ocidoc-go

package spec

import (
	"fmt"
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/opencontainers/go-digest"
)

// driveLetterPattern matches a Windows drive letter prefix such as "C:".
var driveLetterPattern = regexp.MustCompile(`^[A-Za-z]:`)

// reservedWindowsNames are device names Windows reserves regardless of extension
// (NUL.txt is reserved exactly like NUL).
// Matched case-insensitively against the segment name up to the first ".".
var reservedWindowsNames = map[string]struct{}{
	"CON": {}, "PRN": {}, "AUX": {}, "NUL": {},
	"COM1": {}, "COM2": {}, "COM3": {}, "COM4": {}, "COM5": {},
	"COM6": {}, "COM7": {}, "COM8": {}, "COM9": {},
	"LPT1": {}, "LPT2": {}, "LPT3": {}, "LPT4": {}, "LPT5": {},
	"LPT6": {}, "LPT7": {}, "LPT8": {}, "LPT9": {},
}

// ValidateBundlePath reports whether path
// is a syntactically valid relative POSIX path for a component tar entry:
// valid UTF-8, "/" separated, relative, and free of ".." segments, empty segments,
// backslashes, drive letters, NUL bytes, trailing dot/space segments
// and Windows reserved device names - portable across every filesystem
// an extracted artifact might land on, not just POSIX ones.
//
// It validates the path string only. Entry-type restrictions
// (regular files only; no symlinks, hardlinks, devices, FIFOs or sockets)
// and cross-component collisions are enforced by ValidateBundlePaths
// and for actual archive content, the archive builder.
func ValidateBundlePath(path string) error {
	if path == "" {
		return &ValidationError{
			Code:    CodeInvalidPath,
			Path:    path,
			Message: "path must not be empty",
		}
	}

	if !utf8.ValidString(path) {
		return &ValidationError{
			Code:    CodeInvalidPath,
			Path:    path,
			Message: "path must be valid UTF-8",
		}
	}

	if strings.ContainsRune(path, 0) {
		return &ValidationError{
			Code:    CodeInvalidPath,
			Path:    path,
			Message: "path must not contain a NUL byte",
		}
	}

	if strings.Contains(path, "\\") {
		return &ValidationError{
			Code:    CodeInvalidPath,
			Path:    path,
			Message: `path must use "/" as the separator, not "\"`,
		}
	}

	if strings.HasPrefix(path, "/") {
		return &ValidationError{
			Code:    CodeInvalidPath,
			Path:    path,
			Message: "path must be relative",
		}
	}

	if driveLetterPattern.MatchString(path) {
		return &ValidationError{
			Code:    CodeInvalidPath,
			Path:    path,
			Message: "path must not contain a drive letter",
		}
	}

	for seg := range strings.SplitSeq(path, "/") {
		switch seg {
		case "":
			return &ValidationError{
				Code:    CodeInvalidPath,
				Path:    path,
				Message: "path must not contain empty segments",
			}

		case ".":
			return &ValidationError{
				Code:    CodeInvalidPath,
				Path:    path,
				Message: `path must not contain "." segments`,
			}

		case "..":
			return &ValidationError{
				Code:    CodeInvalidPath,
				Path:    path,
				Message: `path must not contain ".." segments`,
			}
		}

		if strings.HasSuffix(seg, ".") ||
			strings.HasSuffix(seg, " ") ||
			strings.HasPrefix(seg, " ") {
			return &ValidationError{
				Code:    CodeInvalidPath,
				Path:    path,
				Message: "path segments must not start or end with a space, or end with a dot",
			}
		}

		for _, r := range seg {
			if r < 0x20 || r == 0x7f || strings.ContainsRune(`< > : " | ? *`, r) {
				return &ValidationError{
					Code:    CodeInvalidPath,
					Path:    path,
					Message: fmt.Sprintf("path segment %q contains a character not portable to Windows", seg),
				}
			}
		}

		base, _, _ := strings.Cut(seg, ".")
		if isReservedWindowsName(strings.ToUpper(base)) {
			return &ValidationError{
				Code:    CodeInvalidPath,
				Path:    path,
				Message: "path must not contain the Windows reserved name " + strings.ToUpper(base),
			}
		}
	}

	return nil
}

// ValidateArtifactConfig reports whether cfg is a well-formed v1beta artifact config:
// a matching schemaVersion, at least one component,
// and syntactically valid component, locale and entrypoint data.
// It does not check against the manifest layers or the filesystem.
func ValidateArtifactConfig(cfg *ArtifactConfig) error {
	if cfg == nil {
		return &ValidationError{
			Code:    CodeMissingSchemaVersion,
			Message: "artifact config must not be nil",
		}
	}

	if cfg.SchemaVersion == "" {
		return &ValidationError{
			Code:    CodeMissingSchemaVersion,
			Message: "schemaVersion is required",
		}
	}

	if cfg.SchemaVersion != SchemaVersion {
		return &ValidationError{
			Code:    CodeUnsupportedSchemaVersion,
			Message: "unsupported schemaVersion: " + cfg.SchemaVersion,
		}
	}

	if cfg.Schema != "" && cfg.Schema != ArtifactConfigSchemaID {
		return &ValidationError{
			Code:    CodeUnsupportedSchemaVersion,
			Message: "unsupported $schema: " + cfg.Schema,
		}
	}

	if len(cfg.Components) == 0 {
		return &ValidationError{
			Code:    CodeNoComponents,
			Message: "artifact config must declare at least one component",
		}
	}

	for name, component := range cfg.Components {
		if err := ValidateComponentType(string(name)); err != nil {
			return err
		}

		if err := ValidateBundleEntrypoint(string(name), component.Entrypoint); err != nil {
			return err
		}
		if err := ValidateArtifactLocales(string(name), component.Locales); err != nil {
			return err
		}
	}

	return nil
}

// ValidateLocaleKey reports whether an opaque locale key is safe as metadata.
// It does not normalize or interpret the key as a language tag.
func ValidateLocaleKey(key string) error {
	if key == "" {
		return &ValidationError{
			Code:    CodeInvalidLocale,
			Message: "locale key must not be empty",
		}
	}

	if !utf8.ValidString(key) {
		return &ValidationError{
			Code:    CodeInvalidLocale,
			Locale:  key,
			Message: "locale key must be valid UTF-8",
		}
	}

	if strings.TrimSpace(key) != key {
		return &ValidationError{
			Code:    CodeInvalidLocale,
			Locale:  key,
			Message: "locale key must not have leading or trailing whitespace",
		}
	}

	for _, r := range key {
		if r < 0x20 || r == 0x7f {
			return &ValidationError{
				Code:    CodeInvalidLocale,
				Locale:  key,
				Message: "locale key must not contain control characters",
			}
		}
	}

	return nil
}

// ValidateBuildLocales validates locale keys and build-time path-rule lists.
// An explicitly declared locale must contain at least one rule.
func ValidateBuildLocales(component string, locales map[string]BuildLocaleConfig) error {
	defaults := 0
	for key, locale := range locales {
		if err := ValidateLocaleKey(key); err != nil {
			return err
		}

		if len(locale.Paths) == 0 {
			return &ValidationError{
				Code:      CodeInvalidLocale,
				Component: component,
				Locale:    key,
				Message:   "locale must declare at least one path rule",
			}
		}

		if locale.Default {
			defaults++
		}
		if defaults > 1 {
			return &ValidationError{
				Code:      CodeInvalidLocale,
				Component: component,
				Locale:    key,
				Message:   "component may declare only one default locale",
			}
		}
	}

	return nil
}

// ValidateArtifactLocales validates locale keys and resolved bundle-relative paths.
// It does not check that paths exist or have a document type.
func ValidateArtifactLocales(component string, locales map[string]ArtifactLocaleConfig) error {
	defaults := 0
	for key, locale := range locales {
		if err := ValidateLocaleKey(key); err != nil {
			return err
		}
		if err := ValidateBundleEntrypoint(component, locale.Entrypoint); err != nil {
			return err
		}

		if locale.Default {
			defaults++
		}

		if defaults > 1 {
			return &ValidationError{
				Code:      CodeInvalidLocale,
				Component: component,
				Locale:    key,
				Message:   "component may declare only one default locale",
			}
		}

		seen := make(map[string]struct{}, len(locale.Files))
		for _, path := range locale.Files {
			if err := ValidateBundlePath(path); err != nil {
				return &ValidationError{
					Code:    CodeInvalidLocale,
					Locale:  key,
					Path:    path,
					Message: err.Error(),
				}
			}
			if _, ok := seen[path]; ok {
				return &ValidationError{
					Code:    CodeInvalidLocale,
					Locale:  key,
					Path:    path,
					Message: "duplicate locale path",
				}
			}
			seen[path] = struct{}{}
		}
	}

	return nil
}

// ValidateBundleEntrypoint validates an optional component or locale entrypoint.
func ValidateBundleEntrypoint(component, entrypoint string) error {
	if entrypoint == "" {
		return nil
	}

	if err := ValidateBundlePath(entrypoint); err != nil {
		return &ValidationError{
			Code:      CodeInvalidPath,
			Component: component,
			Path:      entrypoint,
			Message:   "invalid entrypoint: " + err.Error(),
		}
	}

	return nil
}

// DocumentationTag derives the ".doc" tag name for the given subject digest.
// Only sha256 subjects are supported in v1beta.
func DocumentationTag(d digest.Digest) (string, error) {
	if err := d.Validate(); err != nil {
		return "", &ValidationError{
			Code:    CodeUnsupportedDigestAlgorithm,
			Message: "invalid digest: " + err.Error(),
		}
	}

	if d.Algorithm() != digest.SHA256 {
		return "", &ValidationError{
			Code:    CodeUnsupportedDigestAlgorithm,
			Message: "only sha256 subjects are supported in v1beta, got " + d.Algorithm().String(),
		}
	}

	return d.Algorithm().String() + "-" + d.Encoded() + ".doc", nil
}

// isReservedWindowsName reports whether base is reserved by Windows.
func isReservedWindowsName(base string) bool {
	if _, reserved := reservedWindowsNames[base]; reserved {
		return true
	}

	if !strings.HasPrefix(base, "COM") && !strings.HasPrefix(base, "LPT") {
		return false
	}

	suffix := strings.TrimPrefix(strings.TrimPrefix(base, "COM"), "LPT")
	switch suffix {
	case "1", "2", "3", "4", "5", "6", "7", "8", "9", "¹", "²", "³":
		return true
	default:
		return false
	}
}
