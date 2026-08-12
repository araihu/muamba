# AGENTS.md - Muamba

Muamba is a Go 1.26.5 CLI for TOFU vendoring, integrity verification, and
package-scoped Go embedding. Public behavior lives in `README.md`; executable
contracts live in focused package tests.

## Development rules

- Work in an isolated feature worktree, never directly on `main`.
- Follow test-driven development: add one failing behavior test, observe the
  expected failure, add minimal implementation, then rerun relevant tests.
- Keep `cmd/muamba` thin. Domain behavior belongs in focused `internal/`
  packages.
- Use only the two approved non-standard dependencies: `go.yaml.in/yaml/v3`
  and `github.com/gofrs/flock`, pinned to plan versions.
- Keep Muamba generic. Do not add Goshtoso overlays, attribution models, or
  runtime-specific metadata.
- Use `apply_patch` for authored file edits. Run `gofmt` after Go edits.
- Never log response bodies, authorization values, or secret material.
- Never hand-edit generated `muamba_gen.go`; regenerate and check drift.

## Required gates

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
go run ./cmd/muamba generate-go --strict --check -f examples/web-assets/.muamba.yaml --dir assets --output muamba_gen.go
```

Coverage must remain at or above 70%. CI publishes the raw profile, function
summary, and HTML report as a workflow artifact.
