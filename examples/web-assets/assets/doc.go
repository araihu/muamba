// Package assets demonstrates a package-scoped Muamba embed registry.
package assets

//go:generate go run ../../../cmd/muamba sync --strict -f ../.muamba.yaml
//go:generate go run ../../../cmd/muamba generate-go --strict -f ../.muamba.yaml --dir assets --output muamba_gen.go
