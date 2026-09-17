# OCIDoc JSON Schemas

## Context

These Draft 2020-12 schemas are generated from the Go types
that are the actual source of truth (`spec.BuildConfig`, `spec.ArtifactConfig`),
via `schemadoc build schemadoc.build.yaml`
(see the root `Makefile`'s `schema` target).
Do not hand-edit them; edit the Go field/type comments
and `jsonschema` struct tags in `spec/config.go` and regenerate instead.

* `build-config-v1beta.json` describes `ocidoc.yaml`/`ocidoc.json` writer input.
* `artifact-config-v1beta.json` describes the config blob referenced
  by an OCIDoc OCI manifest.

Their canonical identifiers are:

```text
https://ocidoc.org/schema/build-config-v1beta.json
https://ocidoc.org/schema/artifact-config-v1beta.json
```

The same values are exported by `spec.BuildConfigSchemaID`
and `spec.ArtifactConfigSchemaID`.

Corresponding generated Markdown reference documentation
and example configs live under `../docs/`.

## Schema overlays

The generated base schemas come from the Go types. The merge patches under
`patches/` add schema-only metadata that reflection cannot derive from dynamic
maps, including component-name constraints and structured examples.

Canonical root `$id` values are supplied directly by `mod2schema.root_id` in
`schemadoc.build.yaml`; they are not maintained in the patches.

## Purpose

These schemas exist for IDE/editor completion and validation,
third-party implementers, and published machine-readable reference.
They are not part of OCIDoc's own runtime validation path:
`ocidoc.yaml`/`ocidoc.json` loading and artifact config reading both
go through typed Go decoding and `Validate()`, never through these files.
A gap between what the schema expresses and what Go validation actually
enforces is not itself a bug in either direction -
the schema favors IDE-friendly structural constraints,
while `Validate()` remains authoritative for semantics requiring context
the schema alone cannot express (for example, an entrypoint belonging
to its component's actually-matched files).
