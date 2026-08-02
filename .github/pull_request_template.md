## Summary

<!-- What does this PR do and why? -->

## Related issue

<!-- Example: Closes #123 -->

## Type of change

- [ ] Bug fix
- [ ] Feature
- [ ] Documentation
- [ ] Tests / CI
- [ ] Refactor / chore

## Trust and safety impact

<!-- Describe changes to integrity, paths, redirects, credentials, writes, or recovery. Write "None" when not applicable. -->

## Checklist

- [ ] Added or updated tests before implementation for changed behavior
- [ ] `go mod tidy` leaves `go.mod` and `go.sum` clean
- [ ] `gofmt` and `golangci-lint run` are clean
- [ ] `go vet ./...` passes
- [ ] `scripts/check-coverage.sh` passes (minimum 70%)
- [ ] `go test -race ./...` passes
- [ ] Offline example verification and generated-file drift checks pass
- [ ] No response bodies, authorization values, credentials, or secrets are logged
- [ ] Generated `muamba_gen.go` files were regenerated, not hand-edited
