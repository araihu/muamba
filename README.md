# Muamba

Muamba vendors remote files with explicit trust on first use (TOFU). One
`muamba.yaml` groups every artifact for a dependency under one version, stores
its integrity lock, and can generate a package-scoped Go embed registry.

It is aimed at JavaScript, CSS, licenses, notices, source maps, and release
artifacts, but treats every download as opaque bytes. Builds and tests never
need network access after the files and manifest are committed.

## Requirements and installation

Muamba requires Go 1.26.5. Pin it in a consumer module as a Go tool:

```bash
go get -tool github.com/araihu/muamba/cmd/muamba@<version>
go tool muamba help
```

Use `go run ./cmd/muamba` when working in this repository.

## Manifest

Each resource declares one opaque version and one or more named downloads:

```yaml
schema: 1

resources:
  bootstrap:
    version: "5.3.8"
    downloads:
      bundle-js:
        url: https://unpkg.com/bootstrap@${version}/dist/js/bootstrap.bundle.min.js
        path: assets/vendor/bootstrap/${version}/bootstrap.bundle.min.js
      core-css:
        url: https://unpkg.com/bootstrap@${version}/dist/css/bootstrap.min.css
        path: assets/vendor/bootstrap/${version}/bootstrap.min.css
      license:
        url: https://unpkg.com/bootstrap@${version}/LICENSE
        path: assets/vendor/bootstrap/${version}/LICENSE
```

`${version}` is the only template token. It works in `url` and `path`. Muamba
warns when an expanded URL does not contain the exact declared version;
`--strict` makes that warning fatal before any download or write.

The base manifest intentionally has no generic metadata. Consumer-specific
roles, licensing relationships, attribution, and overlays belong outside
Muamba.

## Trust and materialization workflow

From this repository, the committed example can be checked offline:

```bash
go run ./cmd/muamba verify --strict -f examples/web-assets/muamba.yaml
```

For a new or partially unlocked manifest, review every URL and then establish
first trust explicitly:

```bash
go tool muamba lock --strict
```

`lock` downloads only entries without `integrity`, writes the files, and adds
SHA-384 SRI locks to the same YAML. Commit the manifest and downloaded files
together.

Later commands do not silently trust different bytes:

```bash
# Offline, read-only integrity check.
go tool muamba verify --strict

# Restore missing or corrupt files only when remote bytes match the lock.
go tool muamba sync --strict

# Operate on one resource or download.
go tool muamba verify --strict bootstrap/core-css
```

Changing trust is always explicit. Update every download in a grouped resource
atomically to a new logical version:

```bash
go tool muamba update bootstrap --version 5.3.9 --strict
```

Or re-trust one artifact whose bytes changed at the same URL:

```bash
go tool muamba update bootstrap/core-css --strict
```

A grouped update stages and hashes all new downloads before changing the
manifest or visible files. It refuses to remove an old versioned file if that
file no longer matches its previous lock.

Commands search the current directory and its parents for `muamba.yaml`.
Pass `-f PATH` to select one explicitly. Selectors are `resource` or
`resource/download`; no selector means all downloads.

## Generate embedded Go resources

Muamba can generate one registry for each Go package containing vendored files:

```bash
go run ./cmd/muamba generate-go \
  --strict \
  -f examples/web-assets/muamba.yaml \
  --dir assets \
  --output muamba_gen.go

go run ./cmd/muamba generate-go \
  --strict --check \
  -f examples/web-assets/muamba.yaml \
  --dir assets \
  --output muamba_gen.go
```

`--dir` is relative to the manifest. Generation selects only locked downloads
beneath that package directory, verifies their local bytes, emits explicit
`//go:embed` paths, and formats deterministic Go. It infers the package from an
existing non-test `.go` file; use `--package NAME` for an otherwise empty
package directory.

The generated API is deliberately small:

```go
func MuambaResources() []MuambaResource
func MuambaResourceByName(name string) (MuambaResource, bool)
func MuambaOpen(resource, download string) (fs.File, error)
```

Generated resource/download constants and the embedded filesystem stay
private. Projects needing several package directories run `generate-go` once
per package. See [`examples/web-assets`](examples/web-assets) for a committed
JS, CSS, and license example.

## Integrity and transport policy

Accepted integrity forms are SRI Base64 and prefixed hexadecimal using
SHA-256, SHA-384, or SHA-512:

```text
sha384-BASE64
sha256:HEX
```

New locks use SHA-384 SRI. HTTPS is the default. Riskier transport requires an
explicit option:

```bash
--allow-http
--insecure-skip-tls-verify
--ca-file private-ca.pem
--timeout 30s
--max-size 10485760
```

HTTP remains separate from disabled TLS verification, redirect targets are
checked again, redirects are capped at ten, and response bodies are never
included in errors.

## Scope and roadmap

Muamba v1 does not implement authentication, SSH/Git sources, archive
extraction, release discovery, overlays, attribution models, or automatic
network access during build/test.

Planned source access includes HTTP Basic authentication, bearer/JWT tokens,
OAuth 2.0 client credentials, and SSH. Credential values will come from
environment variables, keychains, agents, or helper processes—not from
`muamba.yaml`. Other roadmap candidates include protected credential redirects,
Git/release discovery, safe archive extraction, resumable downloads, bounded
parallelism, and a public Go package after a concrete import use appears.

The complete approved behavior and tradeoffs are in
[`docs/superpowers/specs/2026-08-02-muamba-design.md`](docs/superpowers/specs/2026-08-02-muamba-design.md).

## Development

```bash
go test ./...
go test -race ./...
go vet ./...
go run ./cmd/muamba verify --strict -f examples/web-assets/muamba.yaml
go run ./cmd/muamba generate-go --strict --check \
  -f examples/web-assets/muamba.yaml --dir assets --output muamba_gen.go
```
