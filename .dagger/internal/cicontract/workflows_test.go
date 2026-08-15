package cicontract

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

func TestRequiredCIAdapter(t *testing.T) {
	t.Parallel()
	workflow := readWorkflow(t, "ci.yml")

	requireContains(t, workflow,
		"pull_request:",
		"push:",
		"branches: [main]",
		"contents: read",
		"DAGGER_VERSION: '0.21.8'",
		"MIN_COVERAGE: '70.0'",
		"RUN_NONCE: ${{ github.run_id }}-${{ github.run_attempt }}",
		"github.event.pull_request.head.repo.fork && 'fork'",
		"github.event_name == 'pull_request' && 'internal'",
		"'main'",
		"github.event.pull_request.base.repo.full_name == github.repository",
		"github.event.pull_request.head.repo.full_name == github.repository",
		"github.event.pull_request.head.repo.fork == false",
		"github.event_name == 'push' &&",
		"||\n        'ubuntu-24.04'",
		`fromJSON('["self-hosted","Linux","X64","hostinger-vps-pr"]')`,
		`fromJSON('["self-hosted","Linux","X64","hostinger-vps-trusted"]')`,
		"Install pinned Dagger on GitHub-hosted runner",
		"if: runner.environment == 'github-hosted'",
		"version: ${{ env.DAGGER_VERSION }}",
		"id: verify-dagger",
		`expected="v${DAGGER_VERSION}"`,
		`actual="$(dagger version | awk 'NR == 1 { print $2 }')"`,
		`if [[ "$actual" != "$expected" ]]`,
		`echo "exact=true" >> "$GITHUB_OUTPUT"`,
		"dagger call check",
		`--minimum-coverage="${MIN_COVERAGE}"`,
		`--trust-domain="${TRUST_DOMAIN}"`,
		`--run-nonce="${RUN_NONCE}"`,
		"if: always()",
		"steps.verify-dagger.outputs.exact == 'true'",
		"dagger call coverage-report",
		"coverage-report",
		"export --path=.coverage",
		"include-hidden-files: true",
		"retention-days: 14",
		"if-no-files-found: error",
	)
	requireAbsent(t, workflow, "coderabbit", "setup-go", "golangci-lint-action", "goreleaser-action", "pull_request_target", "contents: write", "pull-requests: write", "id-token: write")
	requirePinnedActions(t, workflow)
	requireAbsent(t, workflow, `"hostinger-vps"]`)
	requireCount(t, workflow, "uses: dagger/dagger-for-github@", 1)
	requireCount(t, workflow, "dagger call ", 2)
	requireInOrder(t, workflow,
		"Install pinned Dagger on GitHub-hosted runner",
		"Verify exact Dagger version",
		"dagger call check",
		"dagger call coverage-report",
	)
	requireInOrder(t, workflow,
		`fromJSON('["self-hosted","Linux","X64","hostinger-vps-pr"]')`,
		"github.event_name == 'push' &&",
		`fromJSON('["self-hosted","Linux","X64","hostinger-vps-trusted"]')`,
		"||\n        'ubuntu-24.04'",
	)
}

func TestReleaseAdapter(t *testing.T) {
	t.Parallel()
	workflow := readWorkflow(t, "release.yml")

	requireContains(t, workflow,
		"tags:",
		"- 'v*'",
		"cancel-in-progress: false",
		"if: github.repository == 'araihu/muamba'",
		"contents: write",
		"id-token: write",
		"RUN_NONCE: ${{ github.run_id }}-${{ github.run_attempt }}",
		"TRUST_DOMAIN: release",
		`runs-on: ${{ fromJSON('["self-hosted","Linux","X64","hostinger-vps-trusted"]') }}`,
		"if: runner.environment == 'github-hosted'",
		"version: ${{ env.DAGGER_VERSION }}",
		`expected="v${DAGGER_VERSION}"`,
		`actual="$(dagger version | awk 'NR == 1 { print $2 }')"`,
		`if [[ "$actual" != "$expected" ]]`,
		`echo "::error::expected Dagger $expected, found $actual"`,
		"exit 1",
		`echo "exact=true" >> "$GITHUB_OUTPUT"`,
		"dagger call release-test",
		"dagger call publish-release",
		"--github-token=env://GITHUB_TOKEN",
		"--oidc-token=env://ACTIONS_ID_TOKEN_REQUEST_TOKEN",
		"--oidc-url=env://ACTIONS_ID_TOKEN_REQUEST_URL",
		`--tag="${GITHUB_REF_NAME}"`,
		`--commit="${GITHUB_SHA}"`,
		"--repository=https://github.com/araihu/muamba.git",
		`--trust-domain="${TRUST_DOMAIN}"`,
		`--run-nonce="${RUN_NONCE}"`,
		"GITHUB_TOKEN: ${{ secrets.GITHUB_TOKEN }}",
	)
	requireAbsent(t, workflow, "coderabbit", "setup-go", "cosign-installer", "goreleaser-action")
	requirePinnedActions(t, workflow)
	requireAbsent(t, workflow, `"hostinger-vps"]`)
	requireCount(t, workflow, "uses: dagger/dagger-for-github@", 1)
	requireCount(t, workflow, "dagger call ", 2)
	requireInOrder(t, workflow,
		"Install pinned Dagger on GitHub-hosted runner",
		"Verify exact Dagger version",
		"dagger call release-test",
		"dagger call publish-release",
	)
}

func readWorkflow(t *testing.T, name string) string {
	t.Helper()
	path := filepath.Join("..", "..", "..", ".github", "workflows", name)
	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(contents)
}

func requireContains(t *testing.T, value string, fragments ...string) {
	t.Helper()
	for _, fragment := range fragments {
		if !strings.Contains(value, fragment) {
			t.Errorf("workflow missing %q", fragment)
		}
	}
}

func requireAbsent(t *testing.T, value string, fragments ...string) {
	t.Helper()
	lower := strings.ToLower(value)
	for _, fragment := range fragments {
		if strings.Contains(lower, strings.ToLower(fragment)) {
			t.Errorf("workflow unexpectedly contains %q", fragment)
		}
	}
}

func requireCount(t *testing.T, value, fragment string, want int) {
	t.Helper()
	if got := strings.Count(value, fragment); got != want {
		t.Errorf("workflow count of %q = %d, want %d", fragment, got, want)
	}
}

var usesPin = regexp.MustCompile(`(?m)^\s*(?:-\s*)?uses:\s*([^\s@]+)@([^\s#]+)`)

func requirePinnedActions(t *testing.T, value string) {
	t.Helper()
	sha := regexp.MustCompile(`^[0-9a-f]{40}$`)
	matches := usesPin.FindAllStringSubmatch(value, -1)
	if len(matches) == 0 {
		t.Fatal("workflow contains no actions")
	}
	for _, match := range matches {
		if !sha.MatchString(match[2]) {
			t.Errorf("action %s is not pinned to a commit SHA: %s", match[1], match[2])
		}
	}
}

func requireInOrder(t *testing.T, value string, fragments ...string) {
	t.Helper()
	offset := 0
	for _, fragment := range fragments {
		index := strings.Index(value[offset:], fragment)
		if index < 0 {
			t.Errorf("workflow missing ordered fragment %q", fragment)
			return
		}
		offset += index + len(fragment)
	}
}
