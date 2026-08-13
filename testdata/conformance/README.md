# Internal conformance fixtures

## Context

These fixtures keep the `v1beta` Go validators and published JSON Schemas in
sync. They are repository test data, not a stable importable conformance API.

Directories named `valid` must pass both the corresponding JSON Schema and SDK
semantic validator. Directories named `invalid` must fail both. An
`invalid-schema` directory contains shape failures that disappear after normal
Go unmarshalling, such as unknown JSON properties, and therefore applies only
to schema validation.

Every new validation rule or discovered bypass should add a focused fixture.
