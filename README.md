# ocidoc-go

`ocidoc-go` is the reference Go SDK for building, reading,
verifying and publishing [OCIDoc](https://ocidoc.org) documentation artifacts
in OCI registries and OCI Image Layouts.

The current artifact format is `v1beta`. The module is pre-1.0;
public APIs and the format may change before OCIDoc `v1` is frozen.

## Install

```sh
go get github.com/ocidoc/ocidoc-go
```

## Build an archive

`artifact.BuildArchive` reads `ocidoc.yaml` or `ocidoc.json`
from the source tree and writes a deterministic `.ocidoc` archive.

```go
result, err := artifact.BuildArchive(ctx, artifact.BuildArchiveOptions{
    Root: "./project",
    Output: artifact.Destination{Path: "project.ocidoc"},
})
if err != nil {
    return err
}

fmt.Println(result.Output)
```

Use `artifact.BuildReader` when the caller needs a graph reader for committing
to a local store or publishing to a registry without an archive.
Use `artifact.BuildLayout` when the caller needs an unpacked OCI Image Layout.

## Read and verify an artifact

```go
reader, err := artifact.OpenArchive("project.ocidoc")
if err != nil {
    return err
}
defer reader.Close()

verification, err := artifact.Verify(ctx, reader, artifact.VerifyOptions{})
if err != nil {
    return err
}
if !verification.Valid {
    return errors.New("invalid OCIDoc artifact")
}
```

`artifact.OpenLayout` opens an unpacked OCI Image Layout.
The same `Reader` can be inspected, listed, extracted, diffed or published.

## Attach to an OCI subject

```go
reader, err := artifact.OpenArchive("project.ocidoc")
if err != nil {
    return err
}
defer reader.Close()

client := registry.NewClient(registry.ClientOptions{})
attached, err := client.Attach(
    ctx, reader, "registry.example/app:1.2.3", registry.AttachOptions{
    Publication: registry.PublicationBoth,
    })
if err != nil {
    return err
}

fmt.Println(attached.Reference)
```

`PublicationReferrer` is the preferred discovery mechanism.
`PublicationTag` uses the deterministic `.doc` tag;
`PublicationBoth` publishes both views of the same artifact for compatibility.

For explicit registry credentials and transport options,
see the Go package documentation for `registry.ClientOptions`.
The SDK accepts OCI registry references
and does not expose ORAS types in its public API.

## Packages

* `artifact` builds and operates on local artifacts and OCI layouts.
* `registry` publishes, resolves, discovers, copies and removes artifacts.
* `spec` defines format constants, configuration types and validation.
* `store` provides an experimental local artifact store API.

The full format specification,
CLI documentation and generated configuration schema reference
are published at [ocidoc.org](https://ocidoc.org).
