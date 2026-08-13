// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: Copyright 2026 WoozyMasta
// Source: github.com/ocidoc/ocidoc-go

package markdown

import (
	"reflect"
	"testing"
)

func TestLocalTargetsFindsLinksAndImages(t *testing.T) {
	content := "[Guide](guide.md)\n\n![Diagram](images/diagram.png)\n"

	got := LocalTargets([]byte(content))
	want := []string{"guide.md", "images/diagram.png"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("LocalTargets = %v, want %v", got, want)
	}
}

func TestLocalTargetsSkipsFragmentOnlyAndExternal(t *testing.T) {
	content := "[toc](#section)\n[site](https://example.com/page.md)\n" +
		"[mail](mailto:a@example.com)\n![img](//example.com/x.png)\n"

	if got := LocalTargets([]byte(content)); len(got) != 0 {
		t.Fatalf("LocalTargets = %v, want no local targets", got)
	}
}

func TestLocalTargetsDeduplicatesAndPreservesRelativePaths(t *testing.T) {
	content := "[a](../guide.md) and [b](../guide.md) again\n"

	got := LocalTargets([]byte(content))
	want := []string{"../guide.md"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("LocalTargets = %v, want %v", got, want)
	}
}
