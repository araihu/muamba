# muamba

TOFU vendoring and integrity locks for remote assets.

## Requirements

- Go 1.26.5

## Build and run

```bash
go mod tidy

go build ./cmd/muamba
```

## Usage

```bash
muamba
muamba help
muamba lock
muamba sync
muamba verify
muamba update
muamba generate-go
```

Implementation follows the approved design under `docs/superpowers/specs/`.

## CI

GitHub Actions runs `go test ./...` and `go vet ./...` using Go 1.26.5.
