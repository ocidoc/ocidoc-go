// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: Copyright 2026 WoozyMasta
// Source: github.com/ocidoc/ocidoc-go

/*
Package artifact implements local OCIDoc artifact operations:
parsing build config, building artifacts from a source tree,
and opening, inspecting, listing, extracting, verifying
and diffing existing artifacts (OCI layout or ".ocidoc" archive).

It has no network dependency; remote operations live in the registry package.
*/
package artifact
