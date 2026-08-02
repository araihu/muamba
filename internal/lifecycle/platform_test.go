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

func platformManifest(baseURL, version, linuxLock, darwinLock string) string {
	lockLine := func(lock string) string {
		if lock == "" {
			return ""
		}
		return "\n            integrity: " + lock
	}
	return fmt.Sprintf(`schema: 1
resources:
  tailwind:
    version: %q # shared version
    downloads:
      cli:
        path: .tools/tailwindcss
        executable: true
        max_size: 128MiB
        platforms:
          linux/amd64:
            url: %s/v${version}/linux%s
          darwin/arm64:
            url: %s/v${version}/darwin%s
`, version, baseURL, lockLine(linuxLock), baseURL, lockLine(darwinLock))
}

func TestLockAllPlatformsCachesBothAndMaterializesSelectedTarget(t *testing.T) {
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
	defer server.Close()
	repo := testrepo.New(t, platformManifest(server.URL, "4.3.3", "", ""))
	cacheDir := filepath.Join(t.TempDir(), "cache")
	engine, err := New(repo.Manifest, Options{
		Strict:    true,
		CacheDir:  cacheDir,
		Target:    manifest.Target{GOOS: "linux", GOARCH: "amd64"},
		Transport: transport.Options{AllowHTTP: true},
	})
	if err != nil {
		t.Fatal(err)
	}
	report, err := engine.Lock(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if requests.Load() != 2 {
		t.Fatalf("requests = %d, want 2", requests.Load())
	}
	if len(report.Changed) != 2 || report.Changed[0] != "tailwind/cli[darwin/arm64]" || report.Changed[1] != "tailwind/cli[linux/amd64]" {
		t.Fatalf("report = %#v", report)
	}
	target := filepath.Join(repo.Root, ".tools", "tailwindcss")
	got, err := os.ReadFile(target)
	if err != nil || string(got) != "linux binary" {
		t.Fatalf("target = %q, %v", got, err)
	}
	info, err := os.Stat(target)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o755 {
		t.Fatalf("target mode = %v", info.Mode().Perm())
	}
	document, err := manifest.Load(repo.Manifest)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := document.Validate(true); err != nil {
		t.Fatal(err)
	}
	all, err := document.SelectAll(nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 2 || all[0].Integrity == "" || all[1].Integrity == "" {
		t.Fatalf("locked selections = %#v", all)
	}
	assertCachedSelections(t, cacheDir, all)
	manifestBytes, _ := os.ReadFile(repo.Manifest)
	if strings.Count(string(manifestBytes), "integrity: sha384-") != 2 || !strings.Contains(string(manifestBytes), "# shared version") {
		t.Fatalf("manifest =\n%s", manifestBytes)
	}
}

func assertCachedSelections(t *testing.T, cacheDir string, selections []manifest.Selection) {
	t.Helper()
	store, err := blobcache.New(cacheDir)
	if err != nil {
		t.Fatal(err)
	}
	for _, selection := range selections {
		digest, err := integrity.Parse(selection.Integrity)
		if err != nil {
			t.Fatal(err)
		}
		if err := store.Verify(digest); err != nil {
			t.Fatalf("cache %s: %v", selection.Variant, err)
		}
	}
}

func TestLockPlatformFailurePreservesManifestAndDestination(t *testing.T) {
	downloadTemp := t.TempDir()
	t.Setenv("TMPDIR", downloadTemp)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/linux") {
			http.Error(w, "failed", http.StatusBadGateway)
			return
		}
		_, _ = w.Write([]byte("linux binary"))
	}))
	defer server.Close()
	repo := testrepo.New(t, platformManifest(server.URL, "4.3.3", "", ""))
	target := repo.Write(t, ".tools/tailwindcss", []byte("old local"))
	before, _ := os.ReadFile(repo.Manifest)
	engine, err := New(repo.Manifest, Options{
		Strict:    true,
		CacheDir:  filepath.Join(t.TempDir(), "cache"),
		Target:    manifest.Target{GOOS: "linux", GOARCH: "amd64"},
		Transport: transport.Options{AllowHTTP: true},
	})
	if err != nil {
		t.Fatal(err)
	}
	report, err := engine.Lock(context.Background(), nil)
	if err == nil {
		t.Fatal("Lock succeeded")
	}
	if len(report.Changed) != 0 {
		t.Fatalf("failed lock report = %#v", report)
	}
	after, _ := os.ReadFile(repo.Manifest)
	if string(after) != string(before) {
		t.Fatal("failed lock changed manifest")
	}
	got, err := os.ReadFile(target)
	if err != nil || string(got) != "old local" {
		t.Fatalf("failed lock changed target to %q, %v", got, err)
	}
	assertNoMuambaTemporaryFiles(t, repo.Root, downloadTemp)
}

func TestUpdateResourceRelocksAllPlatformsAndMaterializesSelectedTarget(t *testing.T) {
	var requests atomic.Int64
	requestPaths := make(chan string, 8)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		select {
		case requestPaths <- r.URL.Path:
		default:
		}
		switch {
		case strings.HasSuffix(r.URL.Path, "/linux"):
			_, _ = w.Write([]byte("linux new"))
		case strings.HasSuffix(r.URL.Path, "/darwin"):
			_, _ = w.Write([]byte("darwin new"))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	repo := testrepo.New(t, platformManifest(server.URL, "1.0.0", sri(t, "linux old"), sri(t, "darwin old")))
	target := repo.Write(t, ".tools/tailwindcss", []byte("linux old"))
	engine, err := New(repo.Manifest, Options{
		Strict:    true,
		CacheDir:  filepath.Join(t.TempDir(), "cache"),
		Target:    manifest.Target{GOOS: "linux", GOARCH: "amd64"},
		Transport: transport.Options{AllowHTTP: true},
	})
	if err != nil {
		t.Fatal(err)
	}
	report, err := engine.UpdateResource(context.Background(), "tailwind", "2.0.0")
	if err != nil {
		t.Fatal(err)
	}
	if requests.Load() != 2 {
		t.Fatalf("requests = %d, want 2", requests.Load())
	}
	for range 2 {
		path := <-requestPaths
		if !strings.Contains(path, "/v2.0.0/") {
			t.Errorf("update request path = %q", path)
		}
	}
	if len(report.Changed) != 2 {
		t.Fatalf("report = %#v", report)
	}
	got, err := os.ReadFile(target)
	if err != nil || string(got) != "linux new" {
		t.Fatalf("target = %q, %v", got, err)
	}
	document, err := manifest.Load(repo.Manifest)
	if err != nil {
		t.Fatal(err)
	}
	if document.Manifest.Resources["tailwind"].Version != "2.0.0" {
		t.Fatalf("version = %q", document.Manifest.Resources["tailwind"].Version)
	}
	if _, err := document.Validate(true); err != nil {
		t.Fatal(err)
	}
	all, err := document.SelectAll(nil)
	if err != nil {
		t.Fatal(err)
	}
	wants := map[string]string{"darwin/arm64": "darwin new", "linux/amd64": "linux new"}
	for _, selection := range all {
		digest, err := integrity.Parse(selection.Integrity)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := integrity.Verify(strings.NewReader(wants[selection.Variant]), digest); err != nil {
			t.Fatalf("lock %s: %v", selection.Variant, err)
		}
	}
}

func TestUpdateResourcePlatformFailurePreservesOldState(t *testing.T) {
	downloadTemp := t.TempDir()
	t.Setenv("TMPDIR", downloadTemp)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/linux") {
			http.Error(w, "failed", http.StatusBadGateway)
			return
		}
		_, _ = w.Write([]byte("linux new"))
	}))
	defer server.Close()
	repo := testrepo.New(t, platformManifest(server.URL, "1.0.0", sri(t, "linux old"), sri(t, "darwin old")))
	target := repo.Write(t, ".tools/tailwindcss", []byte("linux old"))
	before, _ := os.ReadFile(repo.Manifest)
	engine, err := New(repo.Manifest, Options{
		Strict:    true,
		CacheDir:  filepath.Join(t.TempDir(), "cache"),
		Target:    manifest.Target{GOOS: "linux", GOARCH: "amd64"},
		Transport: transport.Options{AllowHTTP: true},
	})
	if err != nil {
		t.Fatal(err)
	}
	report, err := engine.UpdateResource(context.Background(), "tailwind", "2.0.0")
	if err == nil {
		t.Fatal("UpdateResource succeeded")
	}
	if len(report.Changed) != 0 {
		t.Fatalf("failed update report = %#v", report)
	}
	after, _ := os.ReadFile(repo.Manifest)
	if string(after) != string(before) {
		t.Fatal("failed update changed manifest")
	}
	got, err := os.ReadFile(target)
	if err != nil || string(got) != "linux old" {
		t.Fatalf("failed update changed target to %q, %v", got, err)
	}
	assertNoMuambaTemporaryFiles(t, repo.Root, downloadTemp)
}

func TestUpdateDownloadRetrustsAllPlatformsOnly(t *testing.T) {
	var requests atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		switch {
		case strings.HasSuffix(r.URL.Path, "/linux"):
			_, _ = w.Write([]byte("linux replacement"))
		case strings.HasSuffix(r.URL.Path, "/darwin"):
			_, _ = w.Write([]byte("darwin replacement"))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	manifestText := strings.TrimSuffix(platformManifest(server.URL, "1.0.0", sri(t, "linux old"), sri(t, "darwin old")), "\n") + `
      notice:
        url: ` + server.URL + `/v${version}/NOTICE
        path: NOTICE
        integrity: ` + sri(t, "notice old") + "\n"
	repo := testrepo.New(t, manifestText)
	target := repo.Write(t, ".tools/tailwindcss", []byte("linux old"))
	repo.Write(t, "NOTICE", []byte("notice old"))
	engine, err := New(repo.Manifest, Options{
		Strict:    true,
		CacheDir:  filepath.Join(t.TempDir(), "cache"),
		Target:    manifest.Target{GOOS: "linux", GOARCH: "amd64"},
		Transport: transport.Options{AllowHTTP: true},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := engine.UpdateDownload(context.Background(), "tailwind", "cli"); err != nil {
		t.Fatal(err)
	}
	if requests.Load() != 2 {
		t.Fatalf("requests = %d, want 2", requests.Load())
	}
	got, err := os.ReadFile(target)
	if err != nil || string(got) != "linux replacement" {
		t.Fatalf("target = %q, %v", got, err)
	}
	if notice, err := os.ReadFile(filepath.Join(repo.Root, "NOTICE")); err != nil || string(notice) != "notice old" {
		t.Fatalf("notice = %q, %v", notice, err)
	}
	document, err := manifest.Load(repo.Manifest)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := document.Validate(true); err != nil {
		t.Fatal(err)
	}
	all, err := document.SelectAll([]string{"tailwind/cli"})
	if err != nil {
		t.Fatal(err)
	}
	wants := map[string]string{"darwin/arm64": "darwin replacement", "linux/amd64": "linux replacement"}
	for _, selection := range all {
		digest, err := integrity.Parse(selection.Integrity)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := integrity.Verify(strings.NewReader(wants[selection.Variant]), digest); err != nil {
			t.Fatalf("platform %s lock: %v", selection.Variant, err)
		}
	}
}

func TestUpdateDownloadStagingFailureReportsNoChanges(t *testing.T) {
	downloadTemp := t.TempDir()
	t.Setenv("TMPDIR", downloadTemp)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/linux") {
			_, _ = w.Write([]byte("linux replacement"))
			return
		}
		_, _ = w.Write([]byte("darwin replacement"))
	}))
	defer server.Close()
	repo := testrepo.New(t, platformManifest(server.URL, "1.0.0", sri(t, "linux old"), sri(t, "darwin old")))
	before, _ := os.ReadFile(repo.Manifest)
	engine, err := New(repo.Manifest, Options{
		Strict:    true,
		CacheDir:  filepath.Join(t.TempDir(), "cache"),
		Target:    manifest.Target{GOOS: "linux", GOARCH: "amd64"},
		Transport: transport.Options{AllowHTTP: true},
	})
	if err != nil {
		t.Fatal(err)
	}
	repo.Write(t, ".tools", []byte("blocks destination directory"))
	report, err := engine.UpdateDownload(context.Background(), "tailwind", "cli")
	if err == nil {
		t.Fatal("UpdateDownload succeeded")
	}
	if len(report.Changed) != 0 {
		t.Fatalf("failed update report = %#v", report)
	}
	after, _ := os.ReadFile(repo.Manifest)
	if string(after) != string(before) {
		t.Fatal("failed update changed manifest")
	}
	assertNoMuambaTemporaryFiles(t, repo.Root, downloadTemp)
}

func TestCommitStagedRollsBackEarlierDestination(t *testing.T) {
	root := t.TempDir()
	manifestPath := filepath.Join(root, "muamba.yaml")
	firstTarget := filepath.Join(root, "first")
	firstTemporary := filepath.Join(root, ".muamba-stage-first")
	secondTemporary := filepath.Join(root, ".muamba-stage-second")
	for path, contents := range map[string]string{
		manifestPath:    "old manifest",
		firstTarget:     "old first",
		firstTemporary:  "new first",
		secondTemporary: "new second",
	} {
		if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	items := []stagedDownload{
		{target: firstTarget, temporary: firstTemporary},
		{target: filepath.Join(root, "missing", "second"), temporary: secondTemporary},
	}
	if err := commitStaged(items, manifestPath, []byte("new manifest")); err == nil {
		t.Fatal("commitStaged succeeded")
	}
	cleanupStaged(items, root)
	for path, want := range map[string]string{firstTarget: "old first", manifestPath: "old manifest"} {
		got, err := os.ReadFile(path)
		if err != nil || string(got) != want {
			t.Fatalf("%s = %q, %v; want %q", path, got, err, want)
		}
	}
	assertNoMuambaTemporaryFiles(t, root)
}

func assertNoMuambaTemporaryFiles(t *testing.T, roots ...string) {
	t.Helper()
	for _, root := range roots {
		if err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if !entry.IsDir() && strings.HasPrefix(entry.Name(), ".muamba-") {
				t.Errorf("temporary file remains: %s", path)
			}
			return nil
		}); err != nil {
			t.Fatal(err)
		}
	}
}
