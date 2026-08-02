# Muamba

<p align="center">
  <a href="https://github.com/araihu/muamba/actions/workflows/ci.yml"><img src="https://github.com/araihu/muamba/actions/workflows/ci.yml/badge.svg" alt="CI" /></a>
  <a href="https://github.com/araihu/muamba/actions/workflows/ci.yml"><img src="https://img.shields.io/badge/coverage-%E2%89%A570%25-brightgreen.svg" alt="Coverage: at least 70%" /></a>
  <a href="https://pkg.go.dev/github.com/araihu/muamba"><img src="https://pkg.go.dev/badge/github.com/araihu/muamba.svg" alt="Go Reference" /></a>
  <a href="https://goreportcard.com/report/github.com/araihu/muamba"><img src="https://goreportcard.com/badge/github.com/araihu/muamba" alt="Go Report Card" /></a>
  <a href="LICENSE"><img src="https://img.shields.io/badge/license-MIT-blue.svg" alt="License: MIT" /></a>
</p>

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

### Platform-aware executables

Downloads are platform-neutral by default (`platform: multi`) and remain
unchanged for JavaScript, CSS, licenses, and similar files. A download known to
work on only one Go target can declare that exact target:

```yaml
resources:
  tool:
    version: "1.2.3"
    downloads:
      cli:
        url: https://example.com/v${version}/tool-linux-x64
        path: .tools/tool
        platform: linux/amd64
        executable: true
        max_size: 128MiB
```

Release executables with several platform assets use an exact platform map and
one shared destination path:

```yaml
resources:
  tailwind:
    version: "4.3.3"
    downloads:
      cli:
        path: .tools/tailwindcss
        executable: true
        max_size: 128MiB
        platforms:
          linux/amd64:
            url: https://github.com/tailwindlabs/tailwindcss/releases/download/v${version}/tailwindcss-linux-x64
            integrity: sha384-...
          darwin/arm64:
            url: https://github.com/tailwindlabs/tailwindcss/releases/download/v${version}/tailwindcss-macos-arm64
            integrity: sha384-...
```

`url` is optional when `platforms` is present. An exact platform entry wins
over a compatible base URL; otherwise a base URL applies when `platform` is
`multi` or matches the target. Target names use exact Go `GOOS/GOARCH` values.
Unsupported targets fail before downloads or destination writes. Muamba does
not infer aliases or libc variants.

`executable` defaults to false. Materialized executables use mode `0755`; other
files use `0644` on POSIX. `max_size` accepts positive binary sizes using
`KiB`, `MiB`, or `GiB`; omission retains the 100 MiB default.

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

`lock` downloads every unlocked base URL and platform variant, caches their
verified bytes, and adds SHA-384 SRI locks to the same YAML atomically. Only
the current runtime target is written to each shared destination; use
`--target GOOS/GOARCH` to choose another target. Commit the manifest and any
tracked downloaded files together.

Later commands do not silently trust different bytes:

```bash
# Offline, read-only integrity check.
go tool muamba verify --strict

# Restore missing or corrupt files only when remote bytes match the lock.
go tool muamba sync --strict

# Restore a CI target explicitly.
go tool muamba sync --strict --target linux/amd64

# Operate on one resource or download.
go tool muamba verify --strict bootstrap/core-css

# Offline verification of every locked cache blob.
go tool muamba verify --strict --all-platforms
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

A grouped or single-download update stages and hashes every declared platform
variant before changing the manifest or visible files. It materializes only
the selected target and refuses to remove an old versioned file if that file
no longer matches its previous lock.

Commands search the current directory and its parents for `muamba.yaml`.
Pass `-f PATH` to select one explicitly. Selectors are `resource` or
`resource/download`; no selector means all downloads.

## Integrity cache and CI

Every locked download can use an integrity-addressed cache, including JS, CSS,
licenses, and executables. Cache identity is the parsed algorithm and digest,
not URL, resource, or version. Muamba verifies cache bytes before every use.
Identical integrity therefore deduplicates safely across declarations.

Cache directory precedence is `--cache-dir`, `MUAMBA_CACHE_DIR`, then the
`muamba` child of the operating system user cache directory. `sync` follows
this order:

1. Verify the destination; seed or repair its cache blob without network.
2. Otherwise verify and copy the cache blob atomically.
3. Otherwise download, verify, cache, and atomically materialize it.

Corrupt cache or remote bytes never replace an existing destination. Muamba
uses regular copies rather than symlinks or hard links.

GitHub Actions can persist a repository-local cache while keeping executable
destinations ignored:

```yaml
- uses: actions/cache@v4
  with:
    path: .cache/muamba
    key: muamba-${{ runner.os }}-${{ runner.arch }}-${{ hashFiles('muamba.yaml') }}

- run: go tool muamba sync --strict --target linux/amd64 --cache-dir .cache/muamba
```

## Generate embedded Go resources

Muamba can generate one registry for each Go package containing vendored files:

```bash
go run ./cmd/muamba generate-go \
  --strict \
  --target linux/amd64 \
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
package directory. Generation resolves the runtime target by default. Projects
committing platform-specific generated bytes should always pass `--target`.

The generated API is deliberately small:

```go
type MuambaDownload struct {
	Name      string
	URL       string
	Path      string
	Integrity string
	Hash      string
}

func MuambaResources() []MuambaResource
func MuambaResourceByName(name string) (MuambaResource, bool)
func MuambaHash(resource, download string) (string, bool)
func MuambaOpen(resource, download string) (fs.File, error)
```

`Integrity` preserves the manifest's original SRI or hexadecimal spelling.
`Hash` is the same full digest normalized as
`sha256|sha384|sha512:<lowercase-hex>`, independent of the manifest encoding.
Consumers can use the field or direct lookup for cache busting:

```go
hash, ok := MuambaHash("bootstrap", "core-css")
if !ok {
	return errors.New("bootstrap/core-css is not embedded")
}
stylesheetURL := "/assets/bootstrap.css?v=" + url.QueryEscape(hash)
```

Muamba exposes the complete digest and leaves URL shape, query names, and cache
headers to the consumer.

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
--target linux/amd64
--cache-dir .cache/muamba
```

An explicit `--max-size` byte count overrides every manifest `max_size` for
that invocation. Otherwise each download's declared limit applies before the
100 MiB default.

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
parallelism, cache inspection/eviction, musl targeting, platform aliases, and
multiple materialization destinations. Muamba remains publisher-neutral and
does not hardcode release knowledge for Tailwind or other projects.

## Development

```bash
go mod tidy
gofmt_files="$(gofmt -l .)" || exit 1
test -z "$gofmt_files"
go vet ./...
golangci-lint run
scripts/check-coverage_test.sh
scripts/check-coverage.sh
go test -race ./...
go run ./cmd/muamba verify --strict -f examples/web-assets/muamba.yaml
go run ./cmd/muamba generate-go --strict --check \
  -f examples/web-assets/muamba.yaml --dir assets --output muamba_gen.go
```

Coverage must remain at or above 70%; CI uploads the raw profile, function
summary, and HTML report. See [CONTRIBUTING.md](CONTRIBUTING.md) before opening a
pull request. Report vulnerabilities privately using [SECURITY.md](SECURITY.md).
