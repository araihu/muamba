package cli

import (
	"bytes"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
)

func TestPlatformFlagsValidateUsage(t *testing.T) {
	for _, test := range []struct {
		args []string
		want string
	}{
		{args: []string{"verify", "--target", "macos/arm64"}, want: "unsupported Go target"},
		{args: []string{"sync", "--all-platforms"}, want: "flag provided but not defined"},
		{args: []string{"verify", "--max-size", "0"}, want: "max-size"},
	} {
		var stdout, stderr bytes.Buffer
		if code := Run(test.args, &stdout, &stderr); code != 2 {
			t.Fatalf("Run(%q) code = %d, stderr = %q", test.args, code, stderr.String())
		}
		if !strings.Contains(stderr.String(), test.want) {
			t.Fatalf("Run(%q) stderr = %q, want %q", test.args, stderr.String(), test.want)
		}
	}
}

func TestPlatformLifecycleUsesCacheAndExplicitTarget(t *testing.T) {
	var requests atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		switch {
		case strings.HasSuffix(r.URL.Path, "/linux"):
			_, _ = w.Write([]byte("linux binary"))
		case strings.HasSuffix(r.URL.Path, "/darwin"):
			_, _ = w.Write([]byte("darwin binary"))
		default:
			http.NotFound(w, r)
		}
	}))
	root := t.TempDir()
	manifestPath := filepath.Join(root, "muamba.yaml")
	writeTestFile(t, manifestPath, fmt.Sprintf(`schema: 1
resources:
  tool:
    version: "1.0.0"
    downloads:
      cli:
        path: .tools/tool
        executable: true
        platforms:
          linux/amd64:
            url: %s/v${version}/linux
          darwin/arm64:
            url: %s/v${version}/darwin
`, server.URL, server.URL))
	cacheDir := filepath.Join(root, ".cache", "muamba")
	run := func(wantCode int, args ...string) (string, string) {
		t.Helper()
		var stdout, stderr bytes.Buffer
		code := Run(args, &stdout, &stderr)
		if code != wantCode {
			t.Fatalf("Run(%q) code = %d, want %d\nstdout: %s\nstderr: %s", args, code, wantCode, stdout.String(), stderr.String())
		}
		return stdout.String(), stderr.String()
	}
	base := []string{"--strict", "--allow-http", "--target", "linux/amd64", "--cache-dir", cacheDir, "-f", manifestPath}
	stdout, _ := run(0, append([]string{"lock"}, base...)...)
	if !strings.Contains(stdout, "changed tool/cli[linux/amd64]") || !strings.Contains(stdout, "changed tool/cli[darwin/arm64]") {
		t.Fatalf("lock stdout = %q", stdout)
	}
	if requests.Load() != 2 {
		t.Fatalf("lock requests = %d", requests.Load())
	}
	target := filepath.Join(root, ".tools", "tool")
	if got, err := os.ReadFile(target); err != nil || string(got) != "linux binary" {
		t.Fatalf("target = %q, %v", got, err)
	}
	if err := os.Remove(target); err != nil {
		t.Fatal(err)
	}
	server.Close()
	run(0, append([]string{"sync"}, base...)...)
	if requests.Load() != 2 {
		t.Fatalf("cached sync requests = %d", requests.Load())
	}
	run(0, "verify", "--strict", "--target", "linux/amd64", "--cache-dir", cacheDir, "-f", manifestPath)
	run(0, "verify", "--strict", "--target", "linux/amd64", "--cache-dir", cacheDir, "--all-platforms", "-f", manifestPath)
}

func TestCacheDirectoryFlagBeatsEnvironment(t *testing.T) {
	t.Setenv("MUAMBA_CACHE_DIR", filepath.Join(t.TempDir(), "environment-cache"))
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("payload"))
	}))
	defer server.Close()
	root := t.TempDir()
	manifestPath := filepath.Join(root, "muamba.yaml")
	writeTestFile(t, manifestPath, fmt.Sprintf(`schema: 1
resources:
  lib:
    version: "1"
    downloads:
      core:
        url: %s/lib-1
        path: vendor/lib
`, server.URL))
	flagCache := filepath.Join(root, "flag-cache")
	var stdout, stderr bytes.Buffer
	code := Run([]string{"lock", "--strict", "--allow-http", "--cache-dir", flagCache, "-f", manifestPath}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("lock code = %d, stderr = %q", code, stderr.String())
	}
	var blobs int
	if err := filepath.WalkDir(flagCache, func(_ string, entry os.DirEntry, err error) error {
		if err == nil && !entry.IsDir() && !strings.HasSuffix(entry.Name(), ".lock") {
			blobs++
		}
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if blobs != 1 {
		t.Fatalf("flag cache blobs = %d", blobs)
	}
}
