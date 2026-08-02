package lifecycle

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/araihu/muamba/internal/testrepo"
	"github.com/araihu/muamba/internal/transport"
)

func TestLockTrustsMissingIntegrityAndPreservesComments(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte("trusted")) }))
	defer server.Close()
	repo := testrepo.New(t, `schema: 1
resources:
  library:
    version: "1.0.0" # release
    downloads:
      script:
        url: `+server.URL+`/library-1.0.0.js
        path: vendor/library.js # destination
`)
	cacheDir := filepath.Join(t.TempDir(), "cache")
	engine, err := New(repo.Manifest, Options{Strict: true, CacheDir: cacheDir, Transport: transport.Options{AllowHTTP: true}})
	if err != nil {
		t.Fatal(err)
	}
	report, err := engine.Lock(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Changed) != 1 || report.Changed[0] != "library/script" {
		t.Fatalf("report = %#v", report)
	}
	got, _ := os.ReadFile(filepath.Join(repo.Root, "vendor", "library.js"))
	if string(got) != "trusted" {
		t.Fatalf("file = %q", got)
	}
	manifestBytes, _ := os.ReadFile(repo.Manifest)
	manifestText := string(manifestBytes)
	if !strings.Contains(manifestText, "integrity: sha384-") || !strings.Contains(manifestText, "# release") || !strings.Contains(manifestText, "# destination") {
		t.Fatalf("manifest =\n%s", manifestText)
	}
}

func TestSyncRepairsOnlyAfterRemoteMatchesLock(t *testing.T) {
	body := "expected"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte(body)) }))
	defer server.Close()
	repo := testrepo.New(t, lockedManifest(server.URL+"/library-1.0.0.js", sri(t, "expected")))
	target := repo.Write(t, "vendor/library.js", []byte("corrupt"))
	cacheDir := filepath.Join(t.TempDir(), "cache")
	engine, err := New(repo.Manifest, Options{Strict: true, CacheDir: cacheDir, Transport: transport.Options{AllowHTTP: true}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := engine.Sync(context.Background(), nil); err != nil {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(target)
	if string(got) != "expected" {
		t.Fatalf("file = %q", got)
	}

	body = "upstream drift"
	if err := os.WriteFile(target, []byte("local corrupt"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.RemoveAll(cacheDir); err != nil {
		t.Fatal(err)
	}
	if _, err := engine.Sync(context.Background(), nil); err == nil {
		t.Fatal("expected remote integrity failure")
	}
	got, _ = os.ReadFile(target)
	if string(got) != "local corrupt" {
		t.Fatalf("failed sync replaced target with %q", got)
	}
}
