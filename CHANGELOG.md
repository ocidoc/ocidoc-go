<!-- markdownlint-disable MD024 -->
# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog][],
and this project adheres to [Semantic Versioning][].

<!--
## Unreleased

### Added
### Changed
### Removed
-->

## [0.3.0][] - 2026-09-17

### Added

* Component-local locale configuration with per-locale path rules,
  resolved bundle files, entrypoints and fallback markers.
* Context cancellation is now propagated through archive extraction
  and build planning.
* `artifact.Diff` reporting for root annotation,
  manifest subject and locale metadata changes.
* `store.Store.ExportFile` for atomic export to a filesystem path.

### Changed

* Component configuration entries now use structured objects with `paths`,
  `entrypoint` and optional `locales` instead of raw path-rule arrays.
* Local-store commits and metadata updates now coordinate concurrent handles
  and publish files atomically;
  catalog state is rebuilt from the OCI index when needed.
* Generated configuration schemas now include canonical IDs,
  dynamic component constraints and realistic structured examples.
* Schema documentation now reflects effective compression-level normalization.

### Fixed

* Hardened archive extraction and component streams by draining
  and verifying trailing data
  and removing partial files after failed extraction.
* Local-store pruning now preserves blobs shared by a reachable document,
  and oversized OCI index metadata is rejected before store initialization.
* zstd component streams now enforce explicit decoder memory
  and window limits with single-flight decoding.

[0.3.0]: https://github.com/ocidoc/ocidoc-go/compare/v0.2.0...v0.3.0

## [0.2.0][] - 2026-08-17

### Added

* `artifact.BuildReader` for building an OCI graph
  without creating an intermediate `.ocidoc` archive.
* `registry.Client.Pull` for opening a registry graph directly as a Reader.
* Bounded tar scanning for `artifact.List`, `Diff`
  and `Extract` collision preflight,
  including full stream draining and digest verification.
* Compression defaults and level normalization: omitted type resolves to gzip,
  gzip levels are capped at 9 and zstd levels at 19.
* Deep-copy metadata returned by all built-in artifact Readers.
* Metadata trust-boundary validation for registry push,
  local-store commit and attached publication.
* Raw OCI blob access for preserving byte-exact manifest and config content
  across store, registry and `.ocidoc` transfers.
* `store.Store.Export` for writing stored documents
  as portable `.ocidoc` archives.
* `artifact.PackageReader` for packaging an existing artifact graph.
* Local-store deduplicated blob usage reporting.
* Local-store verification, catalog repair and unreachable-blob pruning.

### Changed

* `artifact.Diff` now detects file replacements with unchanged sizes
  by comparing streaming SHA-256 digests.
* Artifact graph transfers no longer re-serialize manifest or config JSON;
  they copy and verify the original OCI blobs.
* Archive-producing operations are named `BuildArchive` and `PullArchive`;
  graph-first operations use `BuildReader` and `Pull`.
* `store.Remove` no longer performs implicit blob garbage collection;
  unreachable content is reclaimed only by explicit store pruning.

### Removed

* Document language from the artifact identity,
  manifest annotations and registry selectors.

[0.2.0]: https://github.com/ocidoc/ocidoc-go/compare/v0.1.0...v0.2.0

## [0.1.0][] - 2026-08-14

### Added

* First public release

[0.1.0]: https://github.com/ocidoc/ocidoc-go/v0.1.0

<!--links-->
[Keep a Changelog]: https://keepachangelog.com/en/1.1.0/
[Semantic Versioning]: https://semver.org/spec/v2.0.0.html
