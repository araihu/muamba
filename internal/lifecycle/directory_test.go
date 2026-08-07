package lifecycle

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	archivepkg "github.com/araihu/muamba/internal/archive"
	"github.com/araihu/muamba/internal/blobcache"
	"github.com/araihu/muamba/internal/manifest"
	"github.com/araihu/muamba/internal/transport"
)

func TestDirectoryLockSyncAndVerifyUseExactGeneratedLock(t *testing.T) {
	archive := directoryArchive(t, map[string]string{
		"icons-v1/icons/z.svg":        "zed",
		"icons-v1/icons/nested/a.svg": "alpha",
		"icons-v1/README.md":          "ignored",
	})
	var requests atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests.Add(1)
		_, _ = w.Write(archive)
	}))
	defer server.Close()
	root := t.TempDir()
	declaration := filepath.Join(root, ".muamba.yaml")
	writeLifecycleFile(t, declaration, `schema: 1
resources:
  icons:
    version: v1
    directories:
      source:
        url: `+server.URL+`/icons-${version}.tar.gz
        archive: tar.gz
        path: vendor/icons/${version}
        include: ["icons/**/*.svg"]
        strip_components: 1
        max_size: 1MiB
        max_files: 10
        max_unpacked_size: 1MiB
`)
	cacheDir := filepath.Join(t.TempDir(), "cache")
	options := Options{Strict: true, CacheDir: cacheDir, Transport: transport.Options{AllowHTTP: true}}
	engine, err := New(declaration, options)
	if err != nil {
		t.Fatal(err)
	}
	report, err := engine.Lock(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Changed) != 2 {
		t.Fatalf("lock report = %#v", report)
	}
	lockBytes, err := os.ReadFile(filepath.Join(root, ".muamba.lock.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	lockText := string(lockBytes)
	for _, want := range []string{
		"id: icons/source", "url: " + server.URL + "/icons-v1.tar.gz", "source: icons/nested/a.svg",
		"path: vendor/icons/v1/icons/nested/a.svg", "size: 5", "integrity: sha384-",
	} {
		if !strings.Contains(lockText, want) {
			t.Fatalf("lock missing %q:\n%s", want, lockText)
		}
	}
	if got, err := os.ReadFile(filepath.Join(root, "vendor/icons/v1/icons/nested/a.svg")); err != nil || string(got) != "alpha" {
		t.Fatalf("materialized a.svg = %q, %v", got, err)
	}
	engine, err = New(declaration, options)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := engine.Verify(context.Background(), nil, false); err != nil {
		t.Fatal(err)
	}
	if err := os.RemoveAll(cacheDir); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(root, "vendor/icons/v1/icons/z.svg")); err != nil {
		t.Fatal(err)
	}
	engine, err = New(declaration, options)
	if err != nil {
		t.Fatal(err)
	}
	report, err = engine.Sync(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Changed) != 1 || requests.Load() != 2 {
		t.Fatalf("sync report = %#v, requests = %d", report, requests.Load())
	}
}

func TestDirectorySyncRejectsArchiveDriftWithoutReplacingTarget(t *testing.T) {
	body := directoryArchive(t, map[string]string{"icons-v1/icons/a.svg": "trusted"})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write(body) }))
	defer server.Close()
	root := t.TempDir()
	declaration := filepath.Join(root, ".muamba.yaml")
	writeLifecycleFile(t, declaration, `schema: 1
resources:
  icons:
    version: v1
    directories:
      source:
        url: `+server.URL+`/icons-${version}.tar.gz
        archive: tar.gz
        path: vendor/icons
        include: ["icons/**/*.svg"]
        strip_components: 1
        max_size: 1MiB
        max_files: 10
        max_unpacked_size: 1MiB
`)
	cacheDir := filepath.Join(t.TempDir(), "cache")
	options := Options{Strict: true, CacheDir: cacheDir, Transport: transport.Options{AllowHTTP: true}}
	engine, err := New(declaration, options)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := engine.Lock(context.Background(), nil); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(root, "vendor/icons/icons/a.svg")
	if err := os.WriteFile(target, []byte("local-corrupt"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.RemoveAll(cacheDir); err != nil {
		t.Fatal(err)
	}
	body = directoryArchive(t, map[string]string{"icons-v1/icons/a.svg": "upstream-drift"})
	engine, err = New(declaration, options)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := engine.Sync(context.Background(), nil); err == nil {
		t.Fatal("sync accepted changed archive")
	}
	if got, err := os.ReadFile(target); err != nil || string(got) != "local-corrupt" {
		t.Fatalf("failed sync replaced target with %q, %v", got, err)
	}
}

func TestUpdateDirectoryRetrustsExactFileSet(t *testing.T) {
	body := directoryArchive(t, map[string]string{"icons-v1/icons/a.svg": "old"})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write(body) }))
	defer server.Close()
	root := t.TempDir()
	declaration := filepath.Join(root, ".muamba.yaml")
	writeLifecycleFile(t, declaration, `schema: 1
resources:
  icons:
    version: v1
    directories:
      source:
        url: `+server.URL+`/icons-${version}.tar.gz
        archive: tar.gz
        path: vendor/icons
        include: ["icons/**/*.svg"]
        strip_components: 1
        max_size: 1MiB
        max_files: 10
        max_unpacked_size: 1MiB
`)
	options := Options{Strict: true, CacheDir: filepath.Join(t.TempDir(), "cache"), Transport: transport.Options{AllowHTTP: true}}
	engine, err := New(declaration, options)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := engine.Lock(context.Background(), nil); err != nil {
		t.Fatal(err)
	}
	body = directoryArchive(t, map[string]string{
		"icons-v1/icons/a.svg": "new",
		"icons-v1/icons/b.svg": "added",
	})
	report, err := engine.UpdateDownload(context.Background(), "icons", "source")
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Changed) != 2 {
		t.Fatalf("report = %#v", report)
	}
	for path, want := range map[string]string{
		"vendor/icons/icons/a.svg": "new",
		"vendor/icons/icons/b.svg": "added",
	} {
		got, readErr := os.ReadFile(filepath.Join(root, path))
		if readErr != nil || string(got) != want {
			t.Fatalf("%s = %q, %v", path, got, readErr)
		}
	}
}

func TestUpdateResourceAtomicallyRewritesSplitDeclarationAndDirectoryLock(t *testing.T) {
	v1Archive := directoryArchive(t, map[string]string{"icons-v1/icons/a.svg": "old"})
	v2Archive := directoryArchive(t, map[string]string{"icons-v2/icons/a.svg": "new"})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		if strings.Contains(request.URL.Path, "v2") {
			_, _ = w.Write(v2Archive)
			return
		}
		_, _ = w.Write(v1Archive)
	}))
	defer server.Close()
	root := t.TempDir()
	declaration := filepath.Join(root, ".muamba.yaml")
	writeLifecycleFile(t, declaration, `schema: 1
resources:
  icons:
    version: v1
    directories:
      source:
        url: `+server.URL+`/icons-${version}.tar.gz
        archive: tar.gz
        path: vendor/icons/${version}
        include: ["icons/**/*.svg"]
        strip_components: 1
        max_size: 1MiB
        max_files: 10
        max_unpacked_size: 1MiB
`)
	options := Options{Strict: true, CacheDir: filepath.Join(t.TempDir(), "cache"), Transport: transport.Options{AllowHTTP: true}}
	engine, err := New(declaration, options)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := engine.Lock(context.Background(), nil); err != nil {
		t.Fatal(err)
	}
	if _, err := engine.UpdateResource(context.Background(), "icons", "v2"); err != nil {
		t.Fatal(err)
	}
	declarationBytes, _ := os.ReadFile(declaration)
	lockBytes, _ := os.ReadFile(filepath.Join(root, ".muamba.lock.yaml"))
	if !strings.Contains(string(declarationBytes), "version: v2") || strings.Contains(string(declarationBytes), "integrity:") {
		t.Fatalf("declaration =\n%s", declarationBytes)
	}
	if !strings.Contains(string(lockBytes), "/icons-v2.tar.gz") || !strings.Contains(string(lockBytes), "vendor/icons/v2/icons/a.svg") {
		t.Fatalf("lock =\n%s", lockBytes)
	}
	if _, err := os.Stat(filepath.Join(root, "vendor/icons/v1/icons/a.svg")); !os.IsNotExist(err) {
		t.Fatalf("old version still exists: %v", err)
	}
	got, err := os.ReadFile(filepath.Join(root, "vendor/icons/v2/icons/a.svg"))
	if err != nil || string(got) != "new" {
		t.Fatalf("new file = %q, %v", got, err)
	}
}

func TestDirectoryHelperFailuresAreClosed(t *testing.T) {
	e := &Engine{options: Options{MaxBytesSet: true, MaxBytes: 99}}
	directory := manifest.DirectorySelection{ResourceName: "icons", DirectoryName: "source", MaxBytes: 10}
	if got := e.effectiveDirectoryMaxBytes(directory); got != 99 {
		t.Fatalf("effective max = %d", got)
	}
	if selections := directoryFileSelections(directory); selections != nil {
		t.Fatalf("unlocked selections = %#v", selections)
	}
	if _, _, err := e.syncDirectory(context.Background(), nil, directory); err == nil {
		t.Fatal("unlocked directory synced")
	}
	directory.Lock = &manifest.LockedDirectory{
		ID:    "icons/source",
		Files: []manifest.LockedDirectoryFile{{Source: "icons/a.svg", Path: "vendor/a.svg", Size: 1, Integrity: sri(t, "a")}},
	}
	if err := e.verifyAndSeedDirectory(directory, nil); err == nil {
		t.Fatal("changed file count accepted")
	}
	unexpected := []archivepkg.File{{Path: "icons/b.svg", Size: 1, Contents: []byte("b")}}
	if err := e.verifyAndSeedDirectory(directory, unexpected); err == nil {
		t.Fatal("unexpected file accepted")
	}
	wrongSize := []archivepkg.File{{Path: "icons/a.svg", Size: 2, Contents: []byte("a")}}
	if err := e.verifyAndSeedDirectory(directory, wrongSize); err == nil {
		t.Fatal("wrong size accepted")
	}
	wrongHash := []archivepkg.File{{Path: "icons/a.svg", Size: 1, Contents: []byte("b")}}
	if err := e.verifyAndSeedDirectory(directory, wrongHash); err == nil {
		t.Fatal("wrong hash accepted")
	}
	store, err := blobcache.New(filepath.Join(t.TempDir(), "cache"))
	if err != nil {
		t.Fatal(err)
	}
	e.cache = store
	valid := []archivepkg.File{{Path: "icons/a.svg", Size: 1, Contents: []byte("a")}}
	if err := e.verifyAndSeedDirectory(directory, valid); err != nil {
		t.Fatalf("valid locked directory: %v", err)
	}
}

func directoryArchive(t *testing.T, entries map[string]string) []byte {
	t.Helper()
	var buffer bytes.Buffer
	gz := gzip.NewWriter(&buffer)
	tw := tar.NewWriter(gz)
	names := make([]string, 0, len(entries))
	for name := range entries {
		names = append(names, name)
	}
	// Map order intentionally unstable; archive extractor and lock must sort.
	for len(names) > 0 {
		name := names[len(names)-1]
		names = names[:len(names)-1]
		contents := entries[name]
		if err := tw.WriteHeader(&tar.Header{Name: name, Typeflag: tar.TypeReg, Mode: 0o644, Size: int64(len(contents))}); err != nil {
			t.Fatal(err)
		}
		if _, err := tw.Write([]byte(contents)); err != nil {
			t.Fatal(err)
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
	return buffer.Bytes()
}

func writeLifecycleFile(t *testing.T, path, contents string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		t.Fatal(err)
	}
}
