# AGENTS.md - Muamba

Muamba is a Go 1.26.5 CLI for TOFU vendoring, integrity verification, and
package-scoped Go embedding. Approved behavior lives in
`docs/superpowers/specs/2026-08-02-muamba-design.md`; implementation sequence
lives in `docs/superpowers/plans/2026-08-02-muamba-v1-implementation.md`.

## Development rules

- Work in an isolated feature worktree, never directly on `main`.
- Follow test-driven development: add one failing behavior test, observe the
  expected failure, add minimal implementation, then rerun relevant tests.
- Keep `cmd/muamba` thin. Domain behavior belongs in focused `internal/`
  packages defined by the implementation plan.
- Use only the two approved non-standard dependencies: `go.yaml.in/yaml/v3`
  and `github.com/gofrs/flock`, pinned to plan versions.
- Keep Muamba generic. Do not add Goshtoso overlays, attribution models, or
  runtime-specific metadata.
- Use `apply_patch` for authored file edits. Run `gofmt` after Go edits.
- Never log response bodies, authorization values, or secret material.
- Never hand-edit generated `muamba_gen.go`; regenerate and check drift.

## Required gates

```bash
go test ./...
go test -race ./...
go vet ./...
go run ./cmd/muamba verify --strict -f examples/web-assets/muamba.yaml
go run ./cmd/muamba generate-go --strict --check -f examples/web-assets/muamba.yaml --dir assets --output muamba_gen.go
```
