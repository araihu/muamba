# Contributing to Muamba

Muamba is an alpha-stage, MIT-licensed Go CLI for trust-on-first-use vendoring,
integrity verification, and package-scoped Go embedding. Bug reports, focused
features, documentation, and tests are welcome.

## Code of Conduct

This project ships a [Code of Conduct](CODE_OF_CONDUCT.md). By participating,
you agree to uphold it.

## Prerequisites

- Go 1.27.0
- golangci-lint v2.12.2
- govulncheck v1.7.0
- Git

## Getting started

```bash
git clone https://github.com/araihu/muamba
cd muamba
git fetch origin
git worktree add -b feature/<name> ../muamba-feature origin/main
cd ../muamba-feature
go test ./...
```

Read [AGENTS.md](AGENTS.md) for repository rules and [README.md](README.md) for
the public behavior contract before changing behavior. Work in an isolated
feature worktree based on `origin/main`; never work directly on `main`.

## Development workflow

All CI logic is portable through Dagger 0.21.8. Run the same required checks
locally with:

```bash
dagger call check --source=. --minimum-coverage=70.0 --trust-domain=internal --run-nonce=1-1
dagger call coverage-report --source=. --trust-domain=internal --run-nonce=1-1 export --path=.coverage
```

`publish-release` is reserved for the protected tag workflow because it writes
to GitHub and uses keyless signing credentials.

Use test-driven development: add one failing behavior test, confirm the expected
failure, make the smallest implementation change, then rerun focused and full
gates. Keep `cmd/muamba` thin and domain behavior in focused `internal/`
packages.

Muamba intentionally has a narrow dependency policy. Do not add a dependency
without explicit design approval. Keep manifests generic; application overlays,
attribution models, and runtime-specific metadata belong in consumers.

Never hand-edit generated `muamba_gen.go` files. Regenerate them with
`generate-go`, then use `--check` to prove that committed output is current.

## Before opening a pull request

Run `dagger call check` above. If Dagger is unavailable, run these equivalent
local gates. This list omits the GoReleaser snapshot smoke test that `check`
runs:

```bash
go mod tidy -diff
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

Coverage must remain at or above 70%. CI publishes `coverage.out`, a function
summary, and an HTML report as a workflow artifact.

Run the pinned vulnerability scan from the Dagger module:

```bash
scan_dir="$(mktemp -d "${TMPDIR:-/tmp}/muamba-govulncheck.XXXXXX")"
GOBIN="$scan_dir" GOTOOLCHAIN=local go install golang.org/x/vuln/cmd/govulncheck@v1.7.0
(cd .dagger && GOWORK=off "$scan_dir/govulncheck" ./...)
```

## Pull requests

1. Create an isolated feature worktree from current `origin/main`.
2. Keep one logical change per pull request.
3. Fill out the pull request template, including trust and safety impact.
4. Resolve required CI and validated review findings before merge.

## Reporting bugs and vulnerabilities

Use the issue templates for public bugs and feature requests. Never include
credentials, private URLs, authorization headers, or sensitive response data.
Report security issues privately using [SECURITY.md](SECURITY.md).

## License

By contributing, you agree that your contributions are licensed under the
[MIT License](LICENSE).
