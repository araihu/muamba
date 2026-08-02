package cli

import (
	"bytes"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

func TestHelp(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := Run([]string{"help"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("Run(help) code = %d, stderr = %q", code, stderr.String())
	}
	for _, command := range []string{"lock", "sync", "verify", "update", "generate-go", "--target", "--cache-dir", "--all-platforms", "MUAMBA_CACHE_DIR"} {
		if !strings.Contains(stdout.String(), command) {
			t.Errorf("help missing %q", command)
		}
	}
	if stderr.Len() != 0 {
		t.Fatalf("stderr = %q, want empty", stderr.String())
	}
}

func TestNoArgsShowsHelp(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if code := Run(nil, &stdout, &stderr); code != 0 {
		t.Fatalf("Run(nil) code = %d", code)
	}
	if !strings.Contains(stdout.String(), "Usage: muamba") {
		t.Fatalf("stdout = %q", stdout.String())
	}
}

func TestUsageAndOperationalExitCodes(t *testing.T) {
	for _, test := range []struct {
		name string
		args []string
		code int
		want string
	}{
		{name: "unknown command", args: []string{"mystery"}, code: 2, want: "unknown command"},
		{name: "unknown flag", args: []string{"verify", "--wat"}, code: 2, want: "flag provided but not defined"},
		{name: "missing manifest", args: []string{"verify", "-f", "not-here.yaml"}, code: 1, want: "manifest"},
		{name: "resource update needs version", args: []string{"update", "lib"}, code: 2, want: "--version"},
		{name: "download update rejects version", args: []string{"update", "lib/core", "--version", "2"}, code: 2, want: "does not accept --version"},
		{name: "generate needs directory", args: []string{"generate-go", "--output", "muamba_gen.go"}, code: 2, want: "--dir"},
	} {
		t.Run(test.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			if code := Run(test.args, &stdout, &stderr); code != test.code {
				t.Fatalf("Run(%q) code = %d, want %d; stderr = %q", test.args, code, test.code, stderr.String())
			}
			if !strings.Contains(stderr.String(), test.want) {
				t.Fatalf("stderr = %q, want %q", stderr.String(), test.want)
			}
		})
	}
}

func TestLifecycleEndToEnd(t *testing.T) {
	var mu sync.RWMutex
	bodies := map[string]string{
		"/lib-1-core.js":     "core one secret body",
		"/lib-1-LICENSE.txt": "license one",
		"/lib-2-core.js":     "core two",
		"/lib-2-LICENSE.txt": "license two",
	}
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		mu.RLock()
		body, ok := bodies[request.URL.Path]
		mu.RUnlock()
		if !ok {
			http.NotFound(writer, request)
			return
		}
		_, _ = writer.Write([]byte(body))
	}))
	defer server.Close()

	root := t.TempDir()
	manifestPath := filepath.Join(root, "muamba.yaml")
	manifestText := fmt.Sprintf(`schema: 1
resources:
  lib:
    version: "1"
    downloads:
      core:
        url: %s/lib-${version}-core.js
        path: assets/vendor/core.js
      license:
        url: %s/lib-${version}-LICENSE.txt
        path: assets/vendor/LICENSE.txt
`, server.URL, server.URL)
	writeTestFile(t, manifestPath, manifestText)
	writeTestFile(t, filepath.Join(root, "assets", "doc.go"), "package assets\n")

	run := func(wantCode int, args ...string) (string, string) {
		t.Helper()
		var stdout, stderr bytes.Buffer
		code := Run(args, &stdout, &stderr)
		if code != wantCode {
			t.Fatalf("Run(%q) code = %d, want %d\nstdout: %s\nstderr: %s", args, code, wantCode, stdout.String(), stderr.String())
		}
		if strings.Contains(stderr.String(), "secret body") {
			t.Fatalf("stderr leaked response body: %q", stderr.String())
		}
		return stdout.String(), stderr.String()
	}
	base := []string{"--strict", "--allow-http", "-f", manifestPath}
	stdout, _ := run(0, append([]string{"lock"}, base...)...)
	if !strings.Contains(stdout, "changed lib/core") || !strings.Contains(stdout, "changed lib/license") {
		t.Fatalf("lock stdout = %q", stdout)
	}
	locked := readTestFile(t, manifestPath)
	if strings.Count(locked, "sha384-") != 2 {
		t.Fatalf("locked manifest =\n%s", locked)
	}
	if got := readTestFile(t, filepath.Join(root, "assets/vendor/core.js")); got != "core one secret body" {
		t.Fatalf("locked core = %q", got)
	}

	stdout, _ = run(0, "verify", "--strict", "-f", manifestPath, "lib/core")
	if !strings.Contains(stdout, "verified lib/core") {
		t.Fatalf("verify stdout = %q", stdout)
	}
	writeTestFile(t, filepath.Join(root, "assets/vendor/core.js"), "corrupt")
	run(1, "verify", "--strict", "-f", manifestPath)
	run(0, "sync", "--strict", "--allow-http", "-f", manifestPath, "lib/core")
	if got := readTestFile(t, filepath.Join(root, "assets/vendor/core.js")); got != "core one secret body" {
		t.Fatalf("synced core = %q", got)
	}

	mu.Lock()
	bodies["/lib-1-core.js"] = "core one refreshed"
	mu.Unlock()
	run(0, "update", "lib/core", "--strict", "--allow-http", "-f", manifestPath)
	if got := readTestFile(t, filepath.Join(root, "assets/vendor/core.js")); got != "core one refreshed" {
		t.Fatalf("single-download update = %q", got)
	}

	run(0, "update", "lib", "--version", "2", "--strict", "--allow-http", "-f", manifestPath)
	if got := readTestFile(t, filepath.Join(root, "assets/vendor/core.js")); got != "core two" {
		t.Fatalf("resource update core = %q", got)
	}
	if !strings.Contains(readTestFile(t, manifestPath), `version: "2"`) {
		t.Fatalf("resource update did not persist version:\n%s", readTestFile(t, manifestPath))
	}

	run(0, "generate-go", "--strict", "-f", manifestPath, "--dir", "assets", "--output", "muamba_gen.go")
	generatedPath := filepath.Join(root, "assets", "muamba_gen.go")
	if generated := readTestFile(t, generatedPath); !strings.Contains(generated, "func MuambaOpen") {
		t.Fatalf("generated source =\n%s", generated)
	}
	run(0, "generate-go", "--strict", "--check", "-f", manifestPath, "--dir", "assets", "--output", "muamba_gen.go")
	writeTestFile(t, generatedPath, "package assets\n// stale\n")
	run(1, "generate-go", "--strict", "--check", "-f", manifestPath, "--dir", "assets", "--output", "muamba_gen.go")
}

func writeTestFile(t *testing.T, path, contents string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		t.Fatal(err)
	}
}

func readTestFile(t *testing.T, path string) string {
	t.Helper()
	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(contents)
}
