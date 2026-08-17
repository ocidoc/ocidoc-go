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

## Unreleased

### Added

* `store.Store.Export` for writing stored documents
  as portable `.ocidoc` archives.
* `artifact.PackageReader` for packaging an existing artifact graph.
* Local-store deduplicated blob usage reporting.
* Local-store verification, catalog repair and unreachable-blob pruning.

### Removed

* Document language from the artifact identity,
  manifest annotations and registry selectors.

## [0.1.0][] - 2026-08-14

### Added

* First public release

[0.1.0]: https://github.com/ocidoc/ocidoc-go/v0.1.0

<!--links-->
[Keep a Changelog]: https://keepachangelog.com/en/1.1.0/
[Semantic Versioning]: https://semver.org/spec/v2.0.0.html
