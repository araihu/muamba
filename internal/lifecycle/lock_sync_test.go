package lifecycle

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

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

func TestConcurrentEnginesReloadManifestAfterMutationLock(t *testing.T) {
	var requests atomic.Int64
	firstRequest := make(chan struct{})
	releaseFirst := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if requests.Add(1) == 1 {
			close(firstRequest)
			<-releaseFirst
		}
		_, _ = w.Write([]byte("trusted"))
	}))
	defer server.Close()
	repo := testrepo.New(t, `schema: 1
resources:
  library:
    version: "1.0.0"
    downloads:
      script:
        url: `+server.URL+`/library-1.0.0.js
        path: vendor/library.js
`)
	options := Options{
		Strict:      true,
		CacheDir:    filepath.Join(t.TempDir(), "cache"),
		LockTimeout: 2 * time.Second,
		Transport:   transport.Options{AllowHTTP: true},
	}
	first, err := New(repo.Manifest, options)
	if err != nil {
		t.Fatal(err)
	}
	second, err := New(repo.Manifest, options)
	if err != nil {
		t.Fatal(err)
	}
	type result struct {
		report Report
		err    error
	}
	firstDone := make(chan result, 1)
	go func() {
		report, lockErr := first.Lock(context.Background(), nil)
		firstDone <- result{report: report, err: lockErr}
	}()
	select {
	case <-firstRequest:
	case <-time.After(time.Second):
		t.Fatal("first lock did not reach upstream")
	}
	secondStarted := make(chan struct{})
	secondDone := make(chan result, 1)
	go func() {
		close(secondStarted)
		report, lockErr := second.Lock(context.Background(), nil)
		secondDone <- result{report: report, err: lockErr}
	}()
	<-secondStarted
	time.Sleep(50 * time.Millisecond)
	close(releaseFirst)
	firstResult := <-firstDone
	secondResult := <-secondDone
	if firstResult.err != nil || secondResult.err != nil {
		t.Fatalf("locks failed: first=%v second=%v", firstResult.err, secondResult.err)
	}
	if requests.Load() != 1 {
		t.Fatalf("upstream requests = %d, want 1", requests.Load())
	}
	if len(firstResult.report.Changed) != 1 || len(secondResult.report.Changed) != 0 {
		t.Fatalf("reports: first=%#v second=%#v", firstResult.report, secondResult.report)
	}
}
