// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: Copyright 2026 WoozyMasta
// Source: github.com/ocidoc/ocidoc-go

package spec

import (
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
		return &ValidationError{Code: CodeInvalidPath, Path: path, Message: "path must not be empty"}
	}

	if !utf8.ValidString(path) {
		return &ValidationError{Code: CodeInvalidPath, Path: path, Message: "path must be valid UTF-8"}
	}

	if strings.ContainsRune(path, 0) {
		return &ValidationError{Code: CodeInvalidPath, Path: path, Message: "path must not contain a NUL byte"}
	}

	if strings.Contains(path, "\\") {
		return &ValidationError{Code: CodeInvalidPath, Path: path, Message: `path must use "/" as the separator, not "\"`}
	}

	if strings.HasPrefix(path, "/") {
		return &ValidationError{Code: CodeInvalidPath, Path: path, Message: "path must be relative"}
	}

	if driveLetterPattern.MatchString(path) {
		return &ValidationError{Code: CodeInvalidPath, Path: path, Message: "path must not contain a drive letter"}
	}

	for seg := range strings.SplitSeq(path, "/") {
		switch seg {
		case "":
			return &ValidationError{Code: CodeInvalidPath, Path: path, Message: "path must not contain empty segments"}
		case ".":
			return &ValidationError{Code: CodeInvalidPath, Path: path, Message: `path must not contain "." segments`}
		case "..":
			return &ValidationError{Code: CodeInvalidPath, Path: path, Message: `path must not contain ".." segments`}
		}

		if strings.HasSuffix(seg, ".") || strings.HasSuffix(seg, " ") || strings.HasPrefix(seg, " ") {
			return &ValidationError{
				Code: CodeInvalidPath, Path: path,
				Message: "path segments must not start or end with a space, or end with a dot",
			}
		}

		base, _, _ := strings.Cut(seg, ".")
		if _, reserved := reservedWindowsNames[strings.ToUpper(base)]; reserved {
			return &ValidationError{
				Code: CodeInvalidPath, Path: path,
				Message: "path must not contain the Windows reserved name " + strings.ToUpper(base),
			}
		}
	}

	return nil
}

// ValidateArtifactConfig reports whether cfg is a well-formed v1beta artifact config:
// a matching schemaVersion, at least one component,
// and syntactically valid component names and entrypoints.
// It does not check against the manifest layers or the filesystem.
func ValidateArtifactConfig(cfg *ArtifactConfig) error {
	if cfg == nil {
		return &ValidationError{Code: CodeMissingSchemaVersion, Message: "artifact config must not be nil"}
	}

	if cfg.SchemaVersion == "" {
		return &ValidationError{Code: CodeMissingSchemaVersion, Message: "schemaVersion is required"}
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
		return &ValidationError{Code: CodeNoComponents, Message: "artifact config must declare at least one component"}
	}

	for name, component := range cfg.Components {
		if err := ValidateComponentType(string(name)); err != nil {
			return err
		}

		if component.Entrypoint == "" {
			continue
		}

		if err := ValidateBundlePath(component.Entrypoint); err != nil {
			return err
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
