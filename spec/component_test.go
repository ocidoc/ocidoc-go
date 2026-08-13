// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: Copyright 2026 WoozyMasta
// Source: github.com/ocidoc/ocidoc-go

package spec

import (
	"errors"
	"testing"
)

func TestValidateComponentTypeStandard(t *testing.T) {
	for _, name := range []ComponentType{
		ComponentDocumentation, ComponentLicense, ComponentChangelog,
		ComponentReleaseNotes, ComponentSecurity, ComponentContributing,
		ComponentCodeOfConduct, ComponentSupport,
	} {
		if err := ValidateComponentType(string(name)); err != nil {
			t.Errorf("ValidateComponentType(%q): unexpected error: %v", name, err)
		}
	}
}

func TestValidateComponentTypeExtension(t *testing.T) {
	for _, name := range []string{"x-runbooks", "x-compliance", "x-api-reference"} {
		if err := ValidateComponentType(name); err != nil {
			t.Errorf("ValidateComponentType(%q): unexpected error: %v", name, err)
		}
	}
}

func TestValidateComponentTypeRejectsUnknownWithoutPrefix(t *testing.T) {
	err := ValidateComponentType("runbooks")
	if err == nil {
		t.Fatal("expected error for unknown component without x- prefix")
	}

	if !errors.Is(err, ErrInvalid) {
		t.Fatalf("expected errors.Is(err, ErrInvalid), got %v", err)
	}

	var verr *ValidationError
	if !errors.As(err, &verr) {
		t.Fatalf("expected *ValidationError, got %T", err)
	}

	if verr.Code != CodeUnknownComponentName {
		t.Fatalf("got code %q, want %q", verr.Code, CodeUnknownComponentName)
	}
}

func TestValidateComponentTypeRejectsBadSyntax(t *testing.T) {
	for _, name := range []string{"", "Documentation", "x-", "x--a", "-x", "x-A", "x-a_b"} {
		err := ValidateComponentType(name)
		if err == nil {
			t.Errorf("ValidateComponentType(%q): expected error, got nil", name)
			continue
		}

		var verr *ValidationError
		if !errors.As(err, &verr) {
			t.Errorf("ValidateComponentType(%q): expected *ValidationError, got %T", name, err)
			continue
		}

		if verr.Code != CodeInvalidComponentName {
			t.Errorf("ValidateComponentType(%q): got code %q, want %q", name, verr.Code, CodeInvalidComponentName)
		}
	}
}

func TestIsStandardComponent(t *testing.T) {
	if !IsStandardComponent("changelog") {
		t.Error("expected changelog to be a standard component")
	}

	if IsStandardComponent("x-runbooks") {
		t.Error("did not expect x-runbooks to be a standard component")
	}
}

func TestIsExtensionComponent(t *testing.T) {
	if !IsExtensionComponent("x-runbooks") {
		t.Error("expected x-runbooks to be an extension component")
	}

	if IsExtensionComponent("changelog") {
		t.Error("did not expect changelog to be an extension component")
	}

	if IsExtensionComponent("x-Bad") {
		t.Error("did not expect x-Bad (invalid syntax) to be a valid extension component")
	}
}
