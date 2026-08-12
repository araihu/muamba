package cicontract

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestFreshnessAndReleaseSupplyChainContracts(t *testing.T) {
	t.Parallel()
	path := filepath.Join("..", "..", "main.go")
	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	module := string(contents)

	requireContains(t, module,
		`cosignImage  = "ghcr.io/sigstore/cosign/cosign:v3.1.2@sha256:`,
		`File("/ko-app/cosign")`,
		`WithFile("/usr/bin/cosign"`,
		`cosign version | grep -F 'v3.1.2'`,
		`cachepolicy.ValidateRelease(trustDomain, runNonce)`,
		`WithEnvVariable("CI_RUN_NONCE", runNonce).`,
		`dag.CacheVolume(cachepolicy.Volume(trustDomain, "go-mod"))`,
		`dag.CacheVolume(cachepolicy.Volume(trustDomain, "go-build"))`,
		`.coverage/test-exit-code.txt`,
		`sha256sum -c checksums.txt`,
	)
	requireAbsent(t, module, `cachepolicy.PersistentAllowed`, `sha256sum --check`)

	for _, signature := range []string{
		"func (m *Muamba) Check(",
		"func (m *Muamba) CoverageReport(",
		"func (m *Muamba) ReleaseTest(",
		"func (m *Muamba) PublishRelease(",
	} {
		index := strings.Index(module, signature)
		if index < 0 {
			t.Fatalf("module missing %q", signature)
		}
		start := index - 160
		if start < 0 {
			start = 0
		}
		if !strings.Contains(module[start:index], `+cache="never"`) {
			t.Errorf("%s missing cache=never annotation", signature)
		}
	}
}
