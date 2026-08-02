package lifecycle

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/araihu/muamba/internal/blobcache"
	"github.com/araihu/muamba/internal/integrity"
	"github.com/araihu/muamba/internal/manifest"
	"github.com/araihu/muamba/internal/testrepo"
	"github.com/araihu/muamba/internal/transport"
)

func TestSyncUsesCacheWithoutSecondRequest(t *testing.T) {
	var requests atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests.Add(1)
		_, _ = w.Write([]byte("expected"))
	}))
	repo := testrepo.New(t, lockedManifest(server.URL+"/library-1.0.0.js", sri(t, "expected")))
	cacheDir := filepath.Join(t.TempDir(), "cache")
	engine, err := New(repo.Manifest, Options{CacheDir: cacheDir, Transport: transport.Options{AllowHTTP: true}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := engine.Sync(context.Background(), nil); err != nil {
		t.Fatal(err)
	}
	if requests.Load() != 1 {
		t.Fatalf("requests = %d", requests.Load())
	}
	if err := os.Remove(filepath.Join(repo.Root, "vendor", "library.js")); err != nil {
		t.Fatal(err)
	}
	server.Close()
	if _, err := engine.Sync(context.Background(), nil); err != nil {
		t.Fatal(err)
	}
	if requests.Load() != 1 {
		t.Fatalf("cache sync made request; requests = %d", requests.Load())
	}
	if got, err := os.ReadFile(filepath.Join(repo.Root, "vendor", "library.js")); err != nil || string(got) != "expected" {
		t.Fatalf("restored destination = %q, %v", got, err)
	}
}

func TestSyncSeedsCacheFromVerifiedDestinationWithoutNetwork(t *testing.T) {
	repo := testrepo.New(t, lockedManifest("https://127.0.0.1:1/library-1.0.0.js", sri(t, "expected")))
	target := repo.Write(t, "vendor/library.js", []byte("expected"))
	cacheDir := filepath.Join(t.TempDir(), "cache")
	engine, err := New(repo.Manifest, Options{CacheDir: cacheDir})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := engine.Sync(context.Background(), nil); err != nil {
		t.Fatal(err)
	}
	digest, err := integrity.Parse(sri(t, "expected"))
	if err != nil {
		t.Fatal(err)
	}
	store, err := blobcache.New(cacheDir)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Verify(digest); err != nil {
		t.Fatal(err)
	}
	if got, err := os.ReadFile(target); err != nil || string(got) != "expected" {
		t.Fatalf("destination = %q, %v", got, err)
	}
}

func TestSyncNeverMaterializesCorruptCache(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("upstream drift"))
	}))
	defer server.Close()
	lock := sri(t, "expected")
	repo := testrepo.New(t, lockedManifest(server.URL+"/library-1.0.0.js", lock))
	target := repo.Write(t, "vendor/library.js", []byte("local corrupt"))
	cacheDir := filepath.Join(t.TempDir(), "cache")
	store, err := blobcache.New(cacheDir)
	if err != nil {
		t.Fatal(err)
	}
	digest, _ := integrity.Parse(lock)
	if err := os.MkdirAll(filepath.Dir(store.Path(digest)), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(store.Path(digest), []byte("cache corrupt"), 0o644); err != nil {
		t.Fatal(err)
	}
	engine, err := New(repo.Manifest, Options{CacheDir: cacheDir, Transport: transport.Options{AllowHTTP: true}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := engine.Sync(context.Background(), nil); err == nil {
		t.Fatal("Sync accepted corrupt cache and drifting upstream")
	}
	if got, err := os.ReadFile(target); err != nil || string(got) != "local corrupt" {
		t.Fatalf("failed sync changed destination to %q, %v", got, err)
	}
}

func TestSyncRepairsExecutableModeWithoutNetwork(t *testing.T) {
	repo := testrepo.New(t, `schema: 1
resources:
  tool:
    version: "1.0.0"
    downloads:
      cli:
        url: https://127.0.0.1:1/tool-1.0.0
        path: .tools/tool
        executable: true
        integrity: `+sri(t, "tool")+`
`)
	target := repo.Write(t, ".tools/tool", []byte("tool"))
	if err := os.Chmod(target, 0o644); err != nil {
		t.Fatal(err)
	}
	engine, err := New(repo.Manifest, Options{CacheDir: filepath.Join(t.TempDir(), "cache")})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := engine.Sync(context.Background(), nil); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(target)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o755 {
		t.Fatalf("mode = %o", info.Mode().Perm())
	}
}

func TestSyncUsesManifestSizeUnlessExplicitOverride(t *testing.T) {
	body := strings.Repeat("x", 2048)
	var requests atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests.Add(1)
		_, _ = w.Write([]byte(body))
	}))
	defer server.Close()
	manifestText := fmt.Sprintf(`schema: 1
resources:
  tool:
    version: "1.0.0"
    downloads:
      cli:
        url: %s/tool-1.0.0
        path: .tools/tool
        max_size: 1KiB
        integrity: %s
`, server.URL, sri(t, body))
	repo := testrepo.New(t, manifestText)
	cacheDir := filepath.Join(t.TempDir(), "cache")
	limited, err := New(repo.Manifest, Options{CacheDir: cacheDir, Transport: transport.Options{AllowHTTP: true}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := limited.Sync(context.Background(), nil); err == nil || !strings.Contains(err.Error(), "exceeds 1024 bytes") {
		t.Fatalf("manifest limit error = %v", err)
	}
	overridden, err := New(repo.Manifest, Options{CacheDir: cacheDir, MaxBytes: 4096, MaxBytesSet: true, Transport: transport.Options{AllowHTTP: true}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := overridden.Sync(context.Background(), nil); err != nil {
		t.Fatal(err)
	}
	if requests.Load() != 2 {
		t.Fatalf("requests = %d, want 2", requests.Load())
	}
	got, err := os.ReadFile(filepath.Join(repo.Root, ".tools", "tool"))
	if err != nil || string(got) != body {
		t.Fatalf("destination = %d bytes, %v", len(got), err)
	}
}

func TestVerifyAllPlatformsChecksCacheOffline(t *testing.T) {
	linuxLock := sri(t, "linux")
	darwinLock := sri(t, "darwin")
	repo := testrepo.New(t, `schema: 1
resources:
  tool:
    version: "1.0.0"
    downloads:
      cli:
        path: .tools/tool
        platforms:
          linux/amd64:
            url: https://127.0.0.1:1/tool-1.0.0-linux
            integrity: `+linuxLock+`
          darwin/arm64:
            url: https://127.0.0.1:1/tool-1.0.0-darwin
            integrity: `+darwinLock+`
`)
	cacheDir := filepath.Join(t.TempDir(), "cache")
	store, err := blobcache.New(cacheDir)
	if err != nil {
		t.Fatal(err)
	}
	linuxSource := repo.Write(t, "linux-source", []byte("linux"))
	linuxDigest, _ := integrity.Parse(linuxLock)
	if err := store.Seed(linuxSource, linuxDigest); err != nil {
		t.Fatal(err)
	}
	engine, err := New(repo.Manifest, Options{
		CacheDir: cacheDir,
		Target:   manifest.Target{GOOS: "linux", GOARCH: "amd64"},
	})
	if err != nil {
		t.Fatal(err)
	}
	repo.Write(t, ".tools/tool", []byte("linux"))
	if _, err := engine.Verify(context.Background(), nil, true); err == nil || !strings.Contains(err.Error(), "darwin/arm64") {
		t.Fatalf("Verify(all) error = %v", err)
	}
	darwinSource := repo.Write(t, "darwin-source", []byte("darwin"))
	darwinDigest, _ := integrity.Parse(darwinLock)
	if err := store.Seed(darwinSource, darwinDigest); err != nil {
		t.Fatal(err)
	}
	if _, err := engine.Verify(context.Background(), nil, true); err != nil {
		t.Fatal(err)
	}
	repo.Write(t, ".tools/tool", []byte("corrupt selected target"))
	if _, err := engine.Verify(context.Background(), nil, true); err == nil || !strings.Contains(err.Error(), ".tools/tool") {
		t.Fatalf("Verify(all) accepted corrupt selected target: %v", err)
	}
}

func TestNewRejectsExplicitZeroMaxBytes(t *testing.T) {
	repo := testrepo.New(t, lockedManifest("https://example.com/library-1.0.0.js", sri(t, "expected")))
	if _, err := New(repo.Manifest, Options{CacheDir: t.TempDir(), MaxBytesSet: true}); err == nil || !strings.Contains(err.Error(), "MaxBytes") {
		t.Fatalf("New error = %v", err)
	}
}
