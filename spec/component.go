// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: Copyright 2026 WoozyMasta
// Source: github.com/ocidoc/ocidoc-go

package spec

import (
	"regexp"
	"strings"
)

// componentNamePattern matches both standard and x-prefixed extension component names.
var componentNamePattern = regexp.MustCompile(`^[a-z][a-z0-9]*(?:-[a-z0-9]+)*$`)

// extensionPrefix marks a component name as a custom, non-standard component.
const extensionPrefix = "x-"

// ComponentType identifies the semantic role of a documentation component.
type ComponentType string

// Standard v1beta component types.
// A component name outside this set must use the ExtensionPrefix-equivalent "x-" prefix.
const (
	ComponentDocumentation ComponentType = "documentation"
	ComponentLicense       ComponentType = "license"
	ComponentChangelog     ComponentType = "changelog"
	ComponentReleaseNotes  ComponentType = "release-notes"
	ComponentSecurity      ComponentType = "security"
	ComponentContributing  ComponentType = "contributing"
	ComponentCodeOfConduct ComponentType = "code-of-conduct"
	ComponentSupport       ComponentType = "support"
)

// standardComponents is the lookup set backing IsStandardComponent.
var standardComponents = map[ComponentType]struct{}{
	ComponentDocumentation: {},
	ComponentLicense:       {},
	ComponentChangelog:     {},
	ComponentReleaseNotes:  {},
	ComponentSecurity:      {},
	ComponentContributing:  {},
	ComponentCodeOfConduct: {},
	ComponentSupport:       {},
}

// ValidateComponentType reports whether name is a syntactically valid component name
// that is either one of the standard v1beta types or a custom component using the required "x-" prefix.
func ValidateComponentType(name string) error {
	if !componentNamePattern.MatchString(name) {
		return &ValidationError{
			Code:      CodeInvalidComponentName,
			Component: name,
			Message:   "component name must match " + componentNamePattern.String(),
		}
	}

	if IsStandardComponent(name) || IsExtensionComponent(name) {
		return nil
	}

	return &ValidationError{
		Code:      CodeUnknownComponentName,
		Component: name,
		Message:   `unknown component name must use the "x-" prefix`,
	}
}

// IsStandardComponent reports whether name is one of the eight standard v1beta component types.
func IsStandardComponent(name string) bool {
	_, ok := standardComponents[ComponentType(name)]
	return ok
}

// IsExtensionComponent reports whether name
// is a syntactically valid custom "x-"-prefixed component name.
func IsExtensionComponent(name string) bool {
	return strings.HasPrefix(name, extensionPrefix) && componentNamePattern.MatchString(name)
}
