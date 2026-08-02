# Muamba design

**Status:** Approved
**Date:** 2026-08-02  
**Module:** `github.com/araihu/muamba`

## Purpose

Muamba is a small Go tool for reproducibly vendoring remote files. Its first
consumer is Goshtoso, replacing Goshtoso's built-in JavaScript downloader and
generated integrity constants. Manja and Goshtoso Charts are expected early
consumers too.

Muamba uses trust on first use (TOFU). A human explicitly locks remote bytes
once. Later commands accept only bytes matching the committed integrity value.
Muamba primarily targets JavaScript, CSS, licenses, notices, and source maps,
but treats every download as opaque bytes.

Muamba owns acquisition, integrity, materialization, and package-scoped Go
embedding. It does not own Goshtoso runtime roles, attribution models, license
relationships, HTML behavior, or YAML overlays.

## Goals

- Keep one concise `muamba.yaml` as source and lock manifest.
- Share one version across all downloads belonging to a logical dependency.
- Require explicit first trust and explicit replacement trust.
- Accept integrity values commonly published by CDNs and GitHub releases.
- Default to HTTPS and safe filesystem behavior.
- Materialize downloads deterministically and verify them offline.
- Generate package-scoped Go code containing an embedded filesystem and a
  generic resource registry.
- Work as a pinned Go tool and through optional `go generate` directives.

## Non-goals for v1

- Authentication or secret storage.
- Git or SSH sources.
- Archive extraction.
- Remote release discovery.
- Generic metadata, overlays, attribution, or license-domain modeling.
- A public Go library for lifecycle operations.
- Build-time or test-time automatic network access.
- Parallel or resumable downloads.

## Manifest

`muamba.yaml` has a schema marker and a map of logical resources. Each resource
has one opaque version string and a keyed map of downloads.

```yaml
schema: 1

resources:
  alpine:
    version: "3.14.9"
    downloads:
      core-js:
        url: https://unpkg.com/alpinejs@${version}/dist/cdn.min.js
        path: assets/vendor/alpine/${version}/alpine.min.js
        integrity: sha384-NArNwzWsUSF+kY2lgW4YriEkjLqi+J+za6HrENUn/3nZqkBnWbxV22kCJEK5Uu6n

      collapse-js:
        url: https://unpkg.com/@alpinejs/collapse@${version}/dist/cdn.min.js
        path: assets/vendor/alpine/${version}/collapse.min.js

      license:
        url: https://unpkg.com/alpinejs@${version}/LICENSE
        path: assets/vendor/alpine/${version}/LICENSE
```

This example shows a partially locked resource: `core-js` is locked while
`collapse-js` and `license` still require explicit `lock`.

### Schema rules

- Root `schema` is required and must equal `1`.
- `resources` is required and must not be empty.
- Resource and download names use strict kebab-case: lowercase ASCII letters,
  digits, and single hyphen separators.
- Resource names and download names are unique within their maps.
- `version` is a required non-empty string. Muamba treats it as opaque; it does
  not require semantic versioning.
- `downloads` is required and must not be empty.
- Each download requires `url` and `path`.
- `integrity` may be absent only before first lock. Commands other than `lock`
  reject selected unlocked downloads.
- Unknown schema keys are errors. The base manifest has no metadata key.
- Muamba preserves YAML comments and ordering when changing version or
  integrity fields.

### Interpolation

`${version}` is the only v1 interpolation token. It may appear in `url` and
`path`. Muamba expands it from the containing resource's version.

Missing values and unknown `${...}` tokens are validation errors. Muamba does
not execute Go templates, shell expansion, environment interpolation, or
template functions.

After expansion, every download URL is checked for an exact, case-sensitive
occurrence of the declared version. A URL containing `${version}` naturally
passes. A literal URL containing the same version also passes, including a URL
such as `/v3.14.9/` for version `3.14.9`.

A missing version occurrence emits a warning by default. `--strict` converts
the warning into a validation error before network or filesystem mutation.
The diagnostic includes resource, download, declared version, and expanded
URL. CI uses strict mode.

### Paths

Download paths are relative to the directory containing `muamba.yaml`.
Absolute paths, empty paths, parent traversal, duplicate resolved destinations,
and destinations escaping through symlinks are rejected.

Files may be spread across multiple directories. Go embedding remains
package-scoped: each generated Go file embeds only downloads beneath its target
package directory.

## Integrity

Muamba accepts one explicitly identified digest per download:

- SRI Base64: `sha256-BASE64`, `sha384-BASE64`, or `sha512-BASE64`.
- Prefixed hexadecimal: `sha256:HEX`, `sha384:HEX`, or `sha512:HEX`.

Bare hashes and unsupported algorithms are errors. Verification decodes either
representation and compares the digest in constant time. Muamba preserves a
manually supplied representation instead of rewriting it.

`lock` and `update` write SHA-384 SRI by default. This matches the primary
JavaScript/CSS workload and lets web consumers reuse the integrity value
directly. GitHub release asset digests such as `sha256:HEX` remain valid manual
inputs.

## Command model

Muamba ships `github.com/araihu/muamba/cmd/muamba`. A consumer pins and invokes
it as a Go tool:

```bash
go get -tool github.com/araihu/muamba/cmd/muamba@v0.1.0
go tool muamba verify --strict
```

Every command searches the current directory and its parents for the nearest
`muamba.yaml`. `-f PATH` selects a manifest explicitly.

Selectors use `resource` or `resource/download`. With no selector, a command
operates on all applicable downloads.

### `lock [selectors...]`

`lock` establishes first trust. It downloads selected entries missing
integrity, writes the destination files, and adds SHA-384 SRI values to the
same manifest. Already locked selections are unchanged.

First trust is never implicit in `sync`, `verify`, or Go generation.

### `sync [selectors...]`

`sync` materializes selected locked downloads. A missing or mismatched local
file is replaced only after downloaded bytes match the committed integrity.
The command never changes the manifest and never performs broad pruning.

### `verify [selectors...]`

`verify` is offline and read-only. It checks manifest validity, strict-version
policy when requested, file presence, and file integrity. It performs no
network requests and no writes.

### `update RESOURCE --version VERSION`

A resource update explicitly trusts a new logical version. Muamba expands the
new version across every download, fetches and hashes all downloads, and stages
all results before committing any new version or integrity value.

If any download fails, the resource version does not commit. When versioned
paths change, old files are removed only if they still match their previous
locks. A locally modified old file fails preflight and blocks automatic
cleanup.

### `update RESOURCE/DOWNLOAD`

A download update explicitly re-trusts one artifact at the current resource
version. This supports an upstream that replaced bytes at an unchanged URL.
Only that download's integrity and materialized file change.

### `generate-go`

`generate-go` emits a package-scoped embedded filesystem and registry:

```bash
go tool muamba generate-go \
  --dir assets \
  --output muamba_gen.go
```

`--dir` is relative to the manifest directory. The output file is placed in
that directory. Muamba infers the package name from existing non-test Go files;
`--package` is required when the directory contains no Go source.

`--check` generates in memory and fails if committed output differs. Generation
also verifies selected materialized files against the manifest, preventing
incorrect bytes from entering a Go binary.

Typical optional integration:

```go
//go:generate go tool muamba sync
//go:generate go tool muamba generate-go --dir assets --output muamba_gen.go
```

`go generate` remains explicit. `go build` and `go test` never run Muamba or
perform network requests automatically.

## Generated Go contract

Generation selects downloads whose resolved destinations are beneath the
target package directory. A logical resource may appear with only the subset
embedded by that package.

Generated public registry API:

```go
type MuambaResource struct {
	Name      string
	Version   string
	Downloads []MuambaDownload
}

type MuambaDownload struct {
	Name      string
	URL       string
	Path      string
	Integrity string
}

func MuambaResources() []MuambaResource
func MuambaResourceByName(name string) (MuambaResource, bool)
func MuambaOpen(resource, download string) (fs.File, error)
```

`Path` remains manifest-relative. `MuambaOpen` maps the resource/download pair
to the package-relative embedded path internally. Returned resource slices are
caller-owned copies; callers cannot mutate later results.

Generated implementation details remain private:

```go
const (
	muambaResourceAlpine        = "alpine"
	muambaDownloadAlpineCoreJs  = "core-js"
	muambaDownloadAlpineLicense = "license"
)

var muambaFiles embed.FS
```

Private constants permit same-package generated integrations without making
manifest names permanent exported API. Go identifier conversion is
deterministic: split a kebab-case name on hyphens, uppercase the first ASCII
letter of each segment, and preserve the remaining lowercase letters. Thus
`core-js` becomes `CoreJs`. Collisions fail generation. Explicit `//go:embed`
paths, resources, and downloads are sorted before formatting with `gofmt`.

The `Muamba` prefix limits collisions in existing packages. Custom public
naming is deferred until a concrete consumer needs it.

Multiple package directories use separate generation invocations. A generated
file cannot embed a parent or sibling directory because Go forbids `..` in
`//go:embed` patterns.

## Architecture

The repository uses a thin command and focused internal packages:

```text
cmd/muamba/          CLI parsing and exit behavior
internal/manifest/   YAML nodes, validation, interpolation, selectors
internal/integrity/  digest parsing, generation, and verification
internal/transport/  HTTPS policy, redirects, limits, downloads
internal/lifecycle/  lock, sync, verify, update, cleanup, process locking
internal/gogen/      package discovery, embed selection, deterministic Go
```

No lifecycle package is public in v1. A public library may be introduced after
a real import-based consumer establishes the required API.

## Transport policy

- HTTPS is allowed by default.
- HTTP requires `--allow-http`.
- Invalid or untrusted TLS certificates require
  `--insecure-skip-tls-verify`.
- `--ca-file` adds a private CA without disabling certificate verification.
- HTTP allowance and disabled TLS verification are separate flags.
- Redirects are limited to ten and transport policy is rechecked at every hop.
- HTTPS-to-HTTP downgrade requires `--allow-http`.
- Default request timeout is 60 seconds.
- Default maximum response size is 100 MiB per download and is configurable.
- Response bodies are never included in errors.

Authentication headers and credential forwarding are absent in v1.

## Filesystem and mutation safety

Mutating commands hold a manifest-scoped process lock. Downloads and manifest
updates are written to temporary files beside their destinations, then replaced
atomically at the individual-file level.

A logical multi-download update is staged completely before visible changes.
The command attempts rollback on ordinary commit failures. No portable atomic
transaction spans several files and the manifest, so a process or machine crash
may leave an extra new file or a file/manifest mismatch. Such state is always
detectable by `verify` and repairable by `sync` or another explicit `update`;
Muamba never reports a false valid lock.

Old path cleanup happens only after new state commits. Empty directories may be
removed upward only as far as the directory containing `muamba.yaml`; that
manifest directory itself is never removed.

## Diagnostics

Errors identify the command, resource, download, destination, and relevant URL
host. Integrity errors show expected and actual digests. Errors do not print
downloaded response bodies or future credential values.

Warnings are stable enough for CI logs but are not a machine-readable API in
v1. Commands return non-zero on validation, transport, integrity, generation,
or mutation failure.

## Verification strategy

Unit tests cover:

- Schema parsing, unknown keys, kebab-case names, and YAML preservation.
- `${version}` expansion and exact URL-version validation.
- Selector resolution and duplicate destination detection.
- SRI Base64 and prefixed hexadecimal SHA-256/384/512 parsing.
- Default SHA-384 SRI generation.
- Path traversal, absolute paths, and symlink escape rejection.
- Deterministic identifier conversion and collision detection.

Transport tests use local HTTP and TLS servers to cover HTTPS defaults, HTTP
opt-in, invalid certificates, custom CAs, redirect downgrade, redirect limits,
timeouts, and response-size limits.

Lifecycle tests use temporary repositories to cover first lock, partial lock,
sync repair, offline verification, single-download retrust, grouped version
update, all-download staging, modified-old-file protection, safe cleanup,
process locking, and detectable interrupted states.

Go-generation tests compare golden output, run `--check`, compile generated
fixtures, open every embedded artifact, and prove caller mutations cannot alter
later registry results.

Repository gates:

```bash
go test ./...
go test -race ./...
go vet ./...
```

## Roadmap

- HTTP Basic authentication.
- Bearer tokens, including JWTs.
- OAuth 2.0 client credentials.
- SSH sources for private repositories and services.
- Credential sources backed by environment variables, keychains, agents, or
  external helper processes. Secrets never belong in `muamba.yaml`.
- Cross-origin redirect protections for future credentials.
- A public Go library after concrete import use appears.
- Exported or configurable generated identifiers and cross-package access.
- Archive extraction with explicit path and size protections.
- Git and release-asset discovery.
- Resumable downloads and bounded parallelism.

## Goshtoso follow-up

Goshtoso-specific overlays remain a separate follow-up design. Planned shape:

- Public `assetmeta` package inside the existing Goshtoso module.
- `cmd/goshtoso-assets` for parsing typed overlay YAML and generating usable Go.
- Resource-level attribution and license relationships.
- Download-level runtime roles and behavior.
- Generated asset-handler integration and attribution/license page data.
- Replacement of Goshtoso's current `versions.json`, `vendorgen`, and duplicated
  runtime manifest without teaching Muamba Goshtoso semantics.
- Later adoption by Manja and Goshtoso Charts through their existing Goshtoso
  dependency.

This follow-up starts after Muamba's specification and implementation plan are
reviewed. Muamba remains independently useful without Goshtoso.
