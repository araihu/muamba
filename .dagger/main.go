// Muamba's portable build, test, and release automation.
package main

import (
	"context"
	"fmt"
	"strings"

	"dagger/muamba/internal/cachepolicy"
	"dagger/muamba/internal/dagger"
	"dagger/muamba/internal/releasepolicy"
)

const (
	goImage      = "golang:1.27.0-bookworm@sha256:484ef6066fa69acb059fdfeda7ba2b8f7391f2ef6abc6f9b8411e669ebd56466"
	lintImage    = "golangci/golangci-lint:v2.13.1@sha256:d371321370bf2907bd13a8f6f8baff0e0ca7438d76fdf636b281eadf7e2305e3"
	releaseImage = "goreleaser/goreleaser:v2.18.0@sha256:a7609141326e383370858ab3ca2572e96e00fb212fe3fd5610cd4de434652faa"
	cosignImage  = "ghcr.io/sigstore/cosign/cosign:v3.1.2@sha256:d91bc4e7e95e8d2f549c747a72dc174f90579e410a1695f57f686674f84ce849"

	workdir = "/src"
)

type Muamba struct{}

// Check runs every required, side-effect-free CI gate. Its nonce forces fresh
// test/effect layers for each CI attempt while dependency caches remain useful.
// +cache="never"
func (m *Muamba) Check(
	ctx context.Context,
	// Muamba repository source.
	// +defaultPath="/"
	// +ignore=[".worktrees", "dist", ".coverage", "site/node_modules"]
	source *dagger.Directory,
	// Minimum total coverage percentage.
	// +optional
	// +default="70.0"
	minimumCoverage string,
	// Trusted cache partition derived by the CI adapter.
	trustDomain string,
	// Unique GitHub run and attempt identifier.
	runNonce string,
) (string, error) {
	if err := cachepolicy.Validate(trustDomain, runNonce); err != nil {
		return "", err
	}
	if _, err := m.module(ctx, source, trustDomain, runNonce); err != nil {
		return "", err
	}
	if _, err := m.format(ctx, source, trustDomain, runNonce); err != nil {
		return "", err
	}
	if _, err := m.vet(ctx, source, trustDomain, runNonce); err != nil {
		return "", err
	}
	if _, err := m.lint(ctx, source, trustDomain, runNonce); err != nil {
		return "", err
	}
	if _, err := m.build(ctx, source, trustDomain, runNonce); err != nil {
		return "", err
	}
	if _, err := m.releaseSnapshot(ctx, source, trustDomain, runNonce); err != nil {
		return "", err
	}
	if _, err := m.coverage(ctx, source, minimumCoverage, trustDomain, runNonce); err != nil {
		return "", err
	}
	if _, err := m.race(ctx, source, trustDomain, runNonce); err != nil {
		return "", err
	}
	if _, err := m.examples(ctx, source, trustDomain, runNonce); err != nil {
		return "", err
	}
	return "all required checks passed", nil
}

// Module checks go.mod and go.sum for tidy drift.
func (m *Muamba) Module(ctx context.Context, source *dagger.Directory, trustDomain string) (string, error) {
	if err := validateTrustDomain(trustDomain); err != nil {
		return "", err
	}
	return m.module(ctx, source, trustDomain, "")
}

func (m *Muamba) module(ctx context.Context, source *dagger.Directory, trustDomain, runNonce string) (string, error) {
	return fresh(m.goContainer(source, trustDomain), runNonce).
		WithExec([]string{"bash", "-euo", "pipefail", "-c", `cp go.mod /tmp/go.mod; cp go.sum /tmp/go.sum; go mod tidy; for f in go.mod go.sum; do cmp -s "$f" "/tmp/$f" || { printf 'go mod tidy drift in %s:\n' "$f" >&2; diff -u "/tmp/$f" "$f" >&2 || true; exit 1; }; done`}).
		WithExec([]string{"go", "version"}).
		Stdout(ctx)
}

// Format rejects gofmt drift without editing host files.
func (m *Muamba) Format(ctx context.Context, source *dagger.Directory, trustDomain string) (string, error) {
	if err := validateTrustDomain(trustDomain); err != nil {
		return "", err
	}
	return m.format(ctx, source, trustDomain, "")
}

func (m *Muamba) format(ctx context.Context, source *dagger.Directory, trustDomain, runNonce string) (string, error) {
	return fresh(m.goContainer(source, trustDomain), runNonce).
		WithExec([]string{"bash", "-euo", "pipefail", "-c", `out="$(gofmt -l .)"; test -z "$out" || { printf 'gofmt drift in:\n%s\n' "$out" >&2; exit 1; }`}).
		WithExec([]string{"printf", "format clean\n"}).
		Stdout(ctx)
}

// Vet runs Go's static analyzer.
func (m *Muamba) Vet(ctx context.Context, source *dagger.Directory, trustDomain string) (string, error) {
	if err := validateTrustDomain(trustDomain); err != nil {
		return "", err
	}
	return m.vet(ctx, source, trustDomain, "")
}

func (m *Muamba) vet(ctx context.Context, source *dagger.Directory, trustDomain, runNonce string) (string, error) {
	return fresh(m.goContainer(source, trustDomain), runNonce).
		WithExec([]string{"go", "vet", "./..."}).
		WithExec([]string{"printf", "vet passed\n"}).
		Stdout(ctx)
}

// Lint runs the repository-pinned golangci-lint release.
func (m *Muamba) Lint(ctx context.Context, source *dagger.Directory, trustDomain string) (string, error) {
	if err := validateTrustDomain(trustDomain); err != nil {
		return "", err
	}
	return m.lint(ctx, source, trustDomain, "")
}

func (m *Muamba) lint(ctx context.Context, source *dagger.Directory, trustDomain, runNonce string) (string, error) {
	return fresh(m.toolContainer(source, lintImage, trustDomain), runNonce).
		WithExec([]string{"golangci-lint", "run", "--timeout=5m"}).
		WithExec([]string{"printf", "lint passed\n"}).
		Stdout(ctx)
}

// Build compiles the Muamba CLI.
func (m *Muamba) Build(ctx context.Context, source *dagger.Directory, trustDomain string) (*dagger.File, error) {
	if err := validateTrustDomain(trustDomain); err != nil {
		return nil, err
	}
	return m.build(ctx, source, trustDomain, "")
}

func (m *Muamba) build(ctx context.Context, source *dagger.Directory, trustDomain, runNonce string) (*dagger.File, error) {
	container := fresh(m.goContainer(source, trustDomain), runNonce).
		WithExec([]string{"go", "build", "-trimpath", "-o", "/out/muamba", "./cmd/muamba"})
	if _, err := container.File("/out/muamba").Sync(ctx); err != nil {
		return nil, err
	}
	return container.File("/out/muamba"), nil
}

// ReleaseSnapshot builds and smoke-tests the exact GoReleaser snapshot used in CI.
func (m *Muamba) ReleaseSnapshot(ctx context.Context, source *dagger.Directory, trustDomain string) (*dagger.Directory, error) {
	if err := validateTrustDomain(trustDomain); err != nil {
		return nil, err
	}
	return m.releaseSnapshot(ctx, source, trustDomain, "")
}

func (m *Muamba) releaseSnapshot(ctx context.Context, source *dagger.Directory, trustDomain, runNonce string) (*dagger.Directory, error) {
	container := fresh(m.releaseContainer(source, trustDomain), runNonce).
		WithExec([]string{"goreleaser", "release", "--snapshot", "--clean", "--skip=sign"}).
		WithExec([]string{"bash", "-euo", "pipefail", "-c", `archive="$(find dist -maxdepth 1 -name 'muamba_*_linux_amd64.tar.gz' -print -quit)"; test -n "$archive"; mkdir -p /tmp/smoke; tar -xzf "$archive" -C /tmp/smoke; version_output="$(/tmp/smoke/muamba version)"; grep -E 'muamba v.+-SNAPSHOT-.+linux/amd64' <<<"$version_output"; grep -F "commit $(git rev-parse HEAD)" <<<"$version_output"; (cd dist && sha256sum -c checksums.txt)`})
	distPath := workdir + "/dist"
	if _, err := container.Directory(distPath).Sync(ctx); err != nil {
		return nil, err
	}
	return container.Directory(distPath), nil
}

// Coverage applies test and minimum-percentage gates, returning all reports on success.
func (m *Muamba) Coverage(ctx context.Context, source *dagger.Directory, minimumCoverage, trustDomain string) (*dagger.Directory, error) {
	if err := validateTrustDomain(trustDomain); err != nil {
		return nil, err
	}
	return m.coverage(ctx, source, minimumCoverage, trustDomain, "")
}

func (m *Muamba) coverage(ctx context.Context, source *dagger.Directory, minimumCoverage, trustDomain, runNonce string) (*dagger.Directory, error) {
	container := fresh(m.goContainer(source, trustDomain).
		WithEnvVariable("MIN_COVERAGE", minimumCoverage), runNonce).
		WithExec([]string{"scripts/check-coverage_test.sh"}).
		WithExec([]string{"scripts/check-coverage.sh"})
	coveragePath := workdir + "/.coverage"
	if _, err := container.Directory(coveragePath).Sync(ctx); err != nil {
		return nil, err
	}
	return container.Directory(coveragePath), nil
}

// CoverageReport always returns an artifact directory and records test exit
// status. It never applies the threshold; Check remains the required gate.
// +cache="never"
func (m *Muamba) CoverageReport(
	ctx context.Context,
	source *dagger.Directory,
	trustDomain string,
	runNonce string,
) (*dagger.Directory, error) {
	if err := cachepolicy.Validate(trustDomain, runNonce); err != nil {
		return nil, err
	}
	container := fresh(m.goContainer(source, trustDomain), runNonce).
		WithExec([]string{"bash", "-euo", "pipefail", "-c", `mkdir -p .coverage; status=0; go test -count=1 -covermode=atomic -coverprofile=.coverage/coverage.out ./... || status=$?; printf '%s\n' "$status" > .coverage/test-exit-code.txt; if test -s .coverage/coverage.out; then go tool cover -func=.coverage/coverage.out > .coverage/coverage.txt || printf 'coverage summary unavailable\n' > .coverage/coverage.txt; go tool cover -html=.coverage/coverage.out -o .coverage/coverage.html || printf '<html><body>coverage HTML unavailable</body></html>\n' > .coverage/coverage.html; else : > .coverage/coverage.out; printf 'coverage unavailable; go test exited %s\n' "$status" > .coverage/coverage.txt; printf '<html><body>coverage unavailable; go test exited %s</body></html>\n' "$status" > .coverage/coverage.html; fi`})
	coveragePath := workdir + "/.coverage"
	if _, err := container.Directory(coveragePath).Sync(ctx); err != nil {
		return nil, err
	}
	return container.Directory(coveragePath), nil
}

// Race runs every Go test with the race detector and no test-result reuse.
func (m *Muamba) Race(ctx context.Context, source *dagger.Directory, trustDomain string) (string, error) {
	if err := validateTrustDomain(trustDomain); err != nil {
		return "", err
	}
	return m.race(ctx, source, trustDomain, "")
}

func (m *Muamba) race(ctx context.Context, source *dagger.Directory, trustDomain, runNonce string) (string, error) {
	return fresh(m.goContainer(source, trustDomain), runNonce).
		WithExec([]string{"go", "test", "-race", "-count=1", "./..."}).
		WithExec([]string{"printf", "race tests passed\n"}).
		Stdout(ctx)
}

// Examples verifies vendored assets offline and rejects generated drift.
func (m *Muamba) Examples(ctx context.Context, source *dagger.Directory, trustDomain string) (string, error) {
	if err := validateTrustDomain(trustDomain); err != nil {
		return "", err
	}
	return m.examples(ctx, source, trustDomain, "")
}

func (m *Muamba) examples(ctx context.Context, source *dagger.Directory, trustDomain, runNonce string) (string, error) {
	return fresh(m.goContainer(source, trustDomain), runNonce).
		WithExec([]string{"go", "run", "./cmd/muamba", "verify", "--strict", "-f", "examples/web-assets/.muamba.yaml"}).
		WithExec([]string{"go", "run", "./cmd/muamba", "generate-go", "--strict", "--check", "-f", "examples/web-assets/.muamba.yaml", "--dir", "assets", "--output", "muamba_gen.go"}).
		WithExec([]string{"printf", "examples verified\n"}).
		Stdout(ctx)
}

// ReleaseTest runs fresh tests and vet in the isolated release trust domain.
// +cache="never"
func (m *Muamba) ReleaseTest(ctx context.Context, source *dagger.Directory, trustDomain, runNonce string) (string, error) {
	if err := cachepolicy.ValidateRelease(trustDomain, runNonce); err != nil {
		return "", err
	}
	return fresh(m.goContainer(source, trustDomain), runNonce).
		WithExec([]string{"go", "test", "-count=1", "./..."}).
		WithExec([]string{"go", "vet", "./..."}).
		WithExec([]string{"printf", "release tests passed\n"}).
		Stdout(ctx)
}

// PublishRelease validates immutable identity, confirms ancestry on origin/main,
// then signs and publishes with GoReleaser. External publication is explicit.
// +cache="never"
func (m *Muamba) PublishRelease(
	ctx context.Context,
	// Exact release-tag source with full Git history.
	// +ignore=[".worktrees", "dist", ".coverage", "site/node_modules"]
	source *dagger.Directory,
	githubToken *dagger.Secret,
	oidcToken *dagger.Secret,
	oidcURL *dagger.Secret,
	tag string,
	commit string,
	repository string,
	trustDomain string,
	runNonce string,
) (string, error) {
	if err := cachepolicy.ValidateRelease(trustDomain, runNonce); err != nil {
		return "", err
	}
	if err := releasepolicy.ValidateIdentity(tag, commit); err != nil {
		return "", err
	}
	if err := releasepolicy.ValidateRepository(repository); err != nil {
		return "", err
	}

	repo := dag.Git(repository, dagger.GitOpts{KeepGitDir: true})
	mainCommit, err := repo.Branch("main").Commit(ctx)
	if err != nil {
		return "", fmt.Errorf("resolve origin/main: %w", err)
	}
	ancestor, err := repo.Commit(commit).CommonAncestor(repo.Commit(mainCommit)).Commit(ctx)
	if err != nil {
		return "", fmt.Errorf("verify release ancestry: %w", err)
	}
	if ancestor != commit {
		return "", fmt.Errorf("release tag commit %s must belong to origin/main", commit)
	}

	output, err := m.releaseContainer(source, trustDomain).
		WithSecretVariable("GITHUB_TOKEN", githubToken).
		WithSecretVariable("ACTIONS_ID_TOKEN_REQUEST_TOKEN", oidcToken).
		WithSecretVariable("ACTIONS_ID_TOKEN_REQUEST_URL", oidcURL).
		WithEnvVariable("GORELEASER_CURRENT_TAG", tag).
		WithEnvVariable("GORELEASER_CURRENT_COMMIT", commit).
		WithEnvVariable("CI_RUN_NONCE", runNonce).
		WithExec([]string{"goreleaser", "release", "--clean"}).
		Stdout(ctx)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(output), nil
}

func validateTrustDomain(trustDomain string) error {
	return cachepolicy.ValidateTrustDomain(trustDomain)
}

func fresh(container *dagger.Container, runNonce string) *dagger.Container {
	if runNonce == "" {
		return container
	}
	return container.WithEnvVariable("CI_RUN_NONCE", runNonce)
}

func (m *Muamba) goContainer(source *dagger.Directory, trustDomain string) *dagger.Container {
	return m.toolContainer(source, goImage, trustDomain)
}

func (m *Muamba) releaseContainer(source *dagger.Directory, trustDomain string) *dagger.Container {
	cosign := dag.Container().From(cosignImage).File("/ko-app/cosign")
	return m.toolContainer(source, releaseImage, trustDomain).
		WithoutEntrypoint().
		WithFile("/usr/bin/cosign", cosign, dagger.ContainerWithFileOpts{Permissions: 0o755}).
		WithExec([]string{"sh", "-c", "cosign version | grep -F 'v3.1.2'"})
}

func (m *Muamba) toolContainer(source *dagger.Directory, image, trustDomain string) *dagger.Container {
	container := dag.Container().
		From(image).
		WithDirectory(workdir, source).
		WithWorkdir(workdir).
		WithEnvVariable("GOMODCACHE", "/go/pkg/mod").
		WithEnvVariable("GOCACHE", "/root/.cache/go-build")
	// Host-owned runner labels isolate PR and trusted Engines, sockets, and data
	// roots. Only reusable Go dependency/build state is mounted here; source,
	// coverage, artifacts, release state, and secrets never enter these volumes.
	return container.
		WithMountedCache("/go/pkg/mod", dag.CacheVolume(cachepolicy.Volume(trustDomain, "go-mod")), dagger.ContainerWithMountedCacheOpts{Sharing: dagger.CacheSharingModeShared}).
		WithMountedCache("/root/.cache/go-build", dag.CacheVolume(cachepolicy.Volume(trustDomain, "go-build")), dagger.ContainerWithMountedCacheOpts{Sharing: dagger.CacheSharingModeShared})
}
