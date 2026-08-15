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
		`releasepolicy.ValidateRepository(repository)`,
		`for f in go.mod go.sum; do cmp -s "$f" "/tmp/$f"`,
		`go mod tidy drift in %s:\n`,
		`diff -u "/tmp/$f" "$f"`,
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

func TestDependencySecurityPins(t *testing.T) {
	t.Parallel()
	path := filepath.Join("..", "..", "go.mod")
	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	module := string(contents)
	for _, dependency := range []struct {
		module  string
		version string
	}{
		{module: "go.opentelemetry.io/otel/sdk", version: "v1.43.0"},
		{module: "go.opentelemetry.io/otel/exporters/otlp/otlplog/otlploggrpc", version: "v0.19.0"},
		{module: "go.opentelemetry.io/otel/exporters/otlp/otlplog/otlploghttp", version: "v0.19.0"},
		{module: "go.opentelemetry.io/otel/log", version: "v0.19.0"},
		{module: "go.opentelemetry.io/otel/sdk/log", version: "v0.19.0"},
		{module: "go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetricgrpc", version: "v1.43.0"},
		{module: "go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetrichttp", version: "v1.43.0"},
		{module: "go.opentelemetry.io/otel/exporters/otlp/otlptrace", version: "v1.43.0"},
		{module: "go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc", version: "v1.43.0"},
		{module: "go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp", version: "v1.43.0"},
		{module: "google.golang.org/grpc", version: "v1.82.1"},
		{module: "golang.org/x/net", version: "v0.56.0"},
		{module: "golang.org/x/sys", version: "v0.46.0"},
		{module: "golang.org/x/text", version: "v0.39.0"},
	} {
		requireModuleVersion(t, module, dependency.module, dependency.version)
		requireNoModuleReplacement(t, module, dependency.module)
	}
}

func requireModuleVersion(t *testing.T, contents, module, version string) {
	t.Helper()
	for _, line := range strings.Split(contents, "\n") {
		fields := strings.Fields(strings.SplitN(line, "//", 2)[0])
		if strings.Contains(strings.Join(fields, " "), "=>") {
			continue
		}
		if len(fields) > 0 && fields[0] == "require" {
			fields = fields[1:]
		}
		if len(fields) >= 2 && fields[0] == module && fields[1] == version {
			return
		}
	}
	t.Errorf("go.mod missing %s %s", module, version)
}

func requireNoModuleReplacement(t *testing.T, contents, module string) {
	t.Helper()
	for _, line := range strings.Split(contents, "\n") {
		fields := strings.Fields(strings.SplitN(line, "//", 2)[0])
		if len(fields) > 0 && fields[0] == "replace" {
			fields = fields[1:]
		}
		for index, field := range fields {
			if field == "=>" && index > 0 && fields[0] == module {
				t.Errorf("go.mod unexpectedly replaces %s: %s", module, strings.TrimSpace(line))
				return
			}
		}
	}
}
