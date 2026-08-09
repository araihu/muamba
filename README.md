# Muamba

<p align="center">
  <a href="https://github.com/araihu/muamba/actions/workflows/ci.yml"><img src="https://github.com/araihu/muamba/actions/workflows/ci.yml/badge.svg" alt="CI" /></a>
  <a href="https://github.com/araihu/muamba/actions/workflows/ci.yml"><img src="https://img.shields.io/badge/coverage-%E2%89%A570%25-brightgreen.svg" alt="Coverage: at least 70%" /></a>
  <a href="https://pkg.go.dev/github.com/araihu/muamba"><img src="https://pkg.go.dev/badge/github.com/araihu/muamba.svg" alt="Go Reference" /></a>
  <a href="https://goreportcard.com/report/github.com/araihu/muamba"><img src="https://goreportcard.com/badge/github.com/araihu/muamba" alt="Go Report Card" /></a>
  <a href="LICENSE"><img src="https://img.shields.io/badge/license-MIT-blue.svg" alt="License: MIT" /></a>
</p>

Muamba vendors remote files using trust on first use (TOFU). Running `lock`
accepts the first response returned by each reviewed URL and records its
exact URL, destination, size, and SHA-384 digest in `.muamba.lock.yaml`.
Declarations stay in `.muamba.yaml`. The digest detects later changes; it does not
authenticate the publisher or content. `verify` checks materialized files
offline. `sync` restores missing or corrupt files only when available bytes
match the lock.

Muamba treats every download as opaque bytes. It works with JavaScript, CSS,
licenses, notices, source maps, executables, and other release files without
package-manager rules. Once you commit the manifest and files, builds and tests
need no network access.

## Installation

### Prebuilt archives

Prebuilt archives require no Go installation. Each release publishes Muamba
for macOS (`darwin_amd64`, `darwin_arm64`), Linux (`linux_amd64`,
`linux_arm64`), and Windows (`windows_amd64`). Download the matching archive,
`checksums.txt`, and `checksums.txt.sigstore.json` from the
[latest GitHub release](https://github.com/araihu/muamba/releases/latest).

Verify the signed checksum list with the release version you downloaded, then
verify the archive before placing `muamba` on your `PATH`:

```bash
VERSION=v0.0.5 # Latest public release; set this to the tag you downloaded.
ARCHIVE="muamba_${VERSION#v}_linux_amd64.tar.gz"
cosign verify-blob \
  --bundle checksums.txt.sigstore.json \
  --certificate-identity "https://github.com/araihu/muamba/.github/workflows/release.yml@refs/tags/${VERSION}" \
  --certificate-oidc-issuer https://token.actions.githubusercontent.com \
  checksums.txt
grep -F -- "  ${ARCHIVE}" checksums.txt | sha256sum --check
muamba version
```

Set `ARCHIVE` to the file you downloaded. Use `shasum -a 256 -c` instead of
`sha256sum --check` on macOS.

On Windows, calculate the archive digest with PowerShell and compare it with
the matching archive record in `checksums.txt`:

```powershell
Get-FileHash .\muamba_0.0.5_windows_amd64.zip -Algorithm SHA256
Select-String "muamba_0.0.5_windows_amd64.zip" .\checksums.txt
```

### Pinned Go tool

Go projects can pin Muamba in the consumer module. This installation path
requires Go 1.26.5 or later:

```bash
# Latest public release:
go get -tool github.com/araihu/muamba/cmd/muamba@v0.0.5
go tool muamba help
```

The examples below use the standalone `muamba` command. Prefix commands with
`go tool` when using the module-pinned tool. Use `go run ./cmd/muamba` when
working in this repository.

### Library integrations

Go libraries can provide a generated declaration in memory while keeping a
separate lock namespace. `ManifestPath` is only the logical materialization
root and mutation-lock identity; Muamba does not read or write that path when
`ManifestBytes` is set:

```go
engine, err := source.New(source.Options{
	ManifestPath:  filepath.Join(root, ".iconpack-engine"),
	ManifestBytes: declarationBytes,
	LockPath:      filepath.Join(root, ".iconpack.lock.yaml"),
	CacheDir:      filepath.Join(root, ".iconpack-cache"),
})
```

## Declaration and lock

Each resource groups related downloads under one version. Every download names
a destination path and either a base URL or platform-specific URLs. Put this
reviewable source declaration in `.muamba.yaml`:

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
warns when an expanded URL omits the exact declared version. With `--strict`,
that warning stops the command before any download or write.

The manifest records download mechanics, not generic metadata. Keep
consumer-specific roles, licensing relationships, attribution, and overlays
outside Muamba.

### Platform-aware executables

Downloads default to `platform: multi`, which suits JavaScript, CSS, licenses,
and similar files. Declare an exact Go target when a download works on only one
platform:

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

Use an exact platform map when a release publishes one executable per target.
All variants share one destination path:

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
          darwin/arm64:
            url: https://github.com/tailwindlabs/tailwindcss/releases/download/v${version}/tailwindcss-macos-arm64
```

`url` is optional when `platforms` is present. An exact platform entry takes
precedence over a compatible base URL. Otherwise, a base URL applies when
`platform` is `multi` or matches the target. Target names use exact Go
`GOOS/GOARCH` values. Unsupported targets fail before downloads or destination
writes. Muamba does not infer aliases or libc variants.

`executable` defaults to false. Materialized executables use mode `0755`; other
files use `0644` on POSIX. `max_size` accepts positive binary sizes using
`KiB`, `MiB`, or `GiB`; omission retains the 100 MiB default.

`lock` generates `.muamba.lock.yaml`. Each locked file records its stable ID,
expanded URL, resolved destination path, exact byte size, and SHA-384 SRI.
Generated entries and nested directory files are sorted by ID and path. Do not
put `integrity` in `.muamba.yaml`; split declarations reject inline locks.

### Archive directories and globs

Use one bounded HTTPS `tar.gz` source to vendor a directory tree without one
download declaration per file:

```yaml
schema: 1
resources:
  heroicons:
    version: v2.2.0
    directories:
      optimized:
        url: https://github.com/tailwindlabs/heroicons/archive/${version}.tar.gz
        archive: tar.gz
        path: vendor/heroicons/${version}
        include:
          - optimized/**/*.svg
          - LICENSE
        exclude:
          - optimized/**/experimental-*.svg
        strip_components: 1
        max_size: 32MiB
        max_files: 4096
        max_unpacked_size: 128MiB
```

`**` matches zero or more path segments; other segments use Go glob syntax.
`strip_components` applies before include/exclude matching. `max_size` bounds
the compressed response; `max_size`, `max_files`, and `max_unpacked_size` are
mandatory archive-expansion bounds.

Muamba accepts only regular files and directories. Archive paths are normalized
before validation; absolute paths, paths that still escape the extraction root
after normalization, backslashes, symlinks, hard links, devices, FIFOs,
duplicate resolved paths, empty matches, and bound overruns fail before
destination or lock replacement.
The lock pins the archive URL, size, digest, and exact sorted file set; every
resolved file also records source path, destination path, size, and digest.

## Trust and materialization workflow

Check the committed example in this repository without network access:

```bash
go run ./cmd/muamba verify --strict -f examples/web-assets/.muamba.yaml
```

For a new or partially unlocked manifest, review every URL before you establish
first trust:

```bash
muamba lock --strict
```

`lock` downloads every unlocked base URL, platform variant, and directory
archive; caches verified file bytes; and generates `.muamba.lock.yaml`
using staged publication with rollback if a publish step returns an error. It
writes only the selected target to each shared destination. Use
`--target GOOS/GOARCH` to choose another target. Commit `.muamba.yaml`,
`.muamba.lock.yaml`, and all materialized files together. Review changes to the
archive digest and exact member list before accepting first trust or an update.

Directory locks record transport origin, archive/member paths, sizes, and
digests. They do not authenticate the publisher or determine licensing. Include
every applicable `LICENSE`, `NOTICE`, and attribution file, then preserve
consumer-specific attribution outside Muamba.

Later commands reject different bytes:

```bash
# Offline, read-only integrity check.
muamba verify --strict

# Restore missing or corrupt files only when remote bytes match the lock.
muamba sync --strict

# Restore a CI target explicitly.
muamba sync --strict --target linux/amd64

# Operate on one resource or download.
muamba verify --strict bootstrap/core-css

# Offline verification of every locked platform variant, directory member, and materialized target.
muamba verify --strict --all-platforms
```

Trust changes only through an explicit update. Move every download in a grouped
resource together to a new logical version:

```bash
muamba update bootstrap --version 5.3.9 --strict
```

Or re-trust one artifact whose bytes changed at the same URL:

```bash
muamba update bootstrap/core-css --strict
```

A grouped or single-download update stages and hashes every declared platform
variant or directory before it changes declaration, lock, or visible files. It
materializes only the selected target. Publication uses per-path renames and
rolls back paths when a publish step returns an error; a process crash can still
leave different files at different generations, so consumers should run
`verify` at their build or deployment boundary. If an old versioned file no
longer matches its previous lock, Muamba leaves it in place and fails the
update.

Commands search the current directory and its parents for `.muamba.yaml`, then
fall back to legacy `muamba.yaml` when no split declaration exists.
Pass `-f PATH` to select one explicitly. Selectors are `resource` or
`resource/download-or-directory`; no selector means all declarations.

Legacy `muamba.yaml` files with inline integrity remain readable and keep their
existing mutation behavior. New directory sources require split files. To
migrate, create `.muamba.yaml` from the legacy declarations with every inline
`integrity` removed, keep reviewed URLs and paths unchanged, then run
`muamba lock --strict`. Review the generated exact sizes and resolved URLs,
verify offline, and commit `.muamba.yaml`, `.muamba.lock.yaml`, and vendored
files together. Presence of `.muamba.yaml` makes discovery ignore the legacy
file, preventing an ambiguous mixed mode.

## Library integrations

Go libraries that need Muamba's acquisition boundary can import
`github.com/araihu/muamba/source`. The package deliberately requires explicit
declaration and lock paths, so a library can own a namespaced lock such as
`.iconpack.lock.yaml` without discovering or modifying a consumer's
`.muamba.yaml`:

```go
engine, err := source.New(source.Options{
    ManifestPath: ".iconpack.engine.yaml",
    LockPath:     ".iconpack.lock.yaml",
    CacheDir:     ".iconpack-cache",
})
if err != nil {
    return err
}
if _, err := engine.Lock(ctx, nil); err != nil { // explicit first trust
    return err
}
files, err := engine.Snapshot(ctx, nil)
```

`Snapshot` synchronizes locked inputs while holding Muamba's mutation lock and
returns verified in-memory bytes. Consumers should process `SnapshotFile.Contents`
instead of reopening `SnapshotFile.Path`; that preserves the verify-then-use
binding. `Lock` is the only first-trust operation, and later `Snapshot` calls
fail on a changed or unavailable source.

## Integrity cache and CI

Every locked download can use the integrity cache, including JavaScript, CSS,
licenses, and executables. The cache key is the parsed algorithm plus digest.
URL, resource name, and version do not affect identity, so identical locked
bytes share one blob. Muamba verifies cached bytes before every use.

Muamba resolves the cache directory from `--cache-dir`, then
`MUAMBA_CACHE_DIR`, then the `muamba` child of the operating system user cache
directory. `sync` follows this order:

1. Verify the destination; seed or repair its cache blob without network.
2. Otherwise verify and copy the cache blob with an atomic per-file replacement.
3. Otherwise download, verify, cache, and atomically materialize the file.

Corrupt cached or remote bytes never replace an existing destination. Muamba
uses regular copies instead of symlinks or hard links.

GitHub Actions can persist a repository-local cache while keeping executable
destinations ignored:

```yaml
- uses: actions/cache@v4
  with:
    path: .cache/muamba
    key: muamba-${{ runner.os }}-${{ runner.arch }}-${{ hashFiles('.muamba.yaml', '.muamba.lock.yaml') }}

- run: go tool muamba sync --strict --target linux/amd64 --cache-dir .cache/muamba
```

## Generate embedded Go resources

Run `generate-go` once for each Go package that owns vendored files:

```bash
go run ./cmd/muamba generate-go \
  --strict \
  --target linux/amd64 \
  -f examples/web-assets/.muamba.yaml \
  --dir assets \
  --output muamba_gen.go

go run ./cmd/muamba generate-go \
  --strict --check \
  -f examples/web-assets/.muamba.yaml \
  --dir assets \
  --output muamba_gen.go
```

`--dir` is relative to the declaration. Generation selects named locked downloads under
that package directory, verifies their local bytes, emits explicit `//go:embed`
paths, and formats deterministic Go. It infers the package from an existing
non-test `.go` file. Use `--package NAME` for an otherwise empty package
directory. Directory/glob sources are materialized and verified by lifecycle
commands but intentionally do not synthesize public download names. Before
running an iconpack or other consumer generator, run
`muamba verify --strict RESOURCE/DIRECTORY`. The consumer reads the verified
directory path and owns generated APIs, attribution, and source-version
metadata. Commit consumer output with the declaration, lock, license files,
and materialized tree. Generation uses the runtime target by default. Projects that commit
platform-specific generated bytes should always pass `--target`.

The generated API exposes resource and download types plus four lookup functions:

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

`Integrity` preserves the lock file's original SRI or hexadecimal spelling.
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

Muamba exposes the complete digest. The consumer owns URL shape, query names,
and cache headers.

Generated resource/download constants and the embedded filesystem stay
private. Projects needing several package directories run `generate-go` once
per package. See [`examples/web-assets`](examples/web-assets) for a committed
JS, CSS, and license example.

## Integrity and transport policy

Muamba accepts SRI Base64 and prefixed hexadecimal integrity values using
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

Allowing HTTP does not disable TLS verification for HTTPS. Muamba checks each
redirect target, caps redirects at ten, and never includes response bodies in
errors.

## Scope and roadmap

Muamba v1 excludes authentication, SSH/Git sources, arbitrary archive formats,
release discovery, overlays, attribution models, and automatic network access
during build or test.

Planned source access includes HTTP Basic authentication, bearer/JWT tokens,
OAuth 2.0 client credentials, and SSH. Credential values will come from
environment variables, keychains, agents, or helper processes. They will not
live in `.muamba.yaml` or `.muamba.lock.yaml`. Other roadmap candidates include protected credential
redirects, Git/release discovery, additional safe archive formats, resumable downloads,
bounded parallelism, cache inspection and eviction, musl targeting, platform
aliases, and multiple materialization destinations. Muamba remains
publisher-neutral and does not hardcode release knowledge for Tailwind or
other projects.

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
go run ./cmd/muamba verify --strict -f examples/web-assets/.muamba.yaml
go run ./cmd/muamba generate-go --strict --check \
  -f examples/web-assets/.muamba.yaml --dir assets --output muamba_gen.go
```

Coverage must remain at or above 70%; CI uploads the raw profile, function
summary, and HTML report. See [CONTRIBUTING.md](CONTRIBUTING.md) before opening a
pull request. Report vulnerabilities privately using [SECURITY.md](SECURITY.md).
