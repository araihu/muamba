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

	"github.com/araihu/muamba/internal/testrepo"
	"github.com/araihu/muamba/internal/transport"
)

func groupedManifest(baseURL string, scriptDigest, licenseDigest string) string {
	return fmt.Sprintf(`schema: 1
resources:
  library:
    version: "1.0.0" # shared
    downloads:
      script:
        url: %s/library-${version}.js
        path: vendor/library/${version}/library.js
        integrity: %s
      license:
        url: %s/library-${version}.LICENSE
        path: vendor/library/${version}/LICENSE
        integrity: %s
`, baseURL, scriptDigest, baseURL, licenseDigest)
}

func TestUpdateResourceStagesAllDownloadsAndCleansOldVersion(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, ".LICENSE") {
			_, _ = w.Write([]byte("new license"))
			return
		}
		_, _ = w.Write([]byte("new script"))
	}))
	defer server.Close()
	repo := testrepo.New(t, groupedManifest(server.URL, sri(t, "old script"), sri(t, "old license")))
	repo.Write(t, "vendor/library/1.0.0/library.js", []byte("old script"))
	repo.Write(t, "vendor/library/1.0.0/LICENSE", []byte("old license"))
	engine, err := New(repo.Manifest, Options{Strict: true, Transport: transport.Options{AllowHTTP: true}})
	if err != nil {
		t.Fatal(err)
	}
	report, err := engine.UpdateResource(context.Background(), "library", "1.1.0")
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Changed) != 2 {
		t.Fatalf("report = %#v", report)
	}
	manifestBytes, _ := os.ReadFile(repo.Manifest)
	if !strings.Contains(string(manifestBytes), `version: "1.1.0" # shared`) {
		t.Fatalf("manifest =\n%s", manifestBytes)
	}
	if _, err := os.Stat(filepath.Join(repo.Root, "vendor/library/1.0.0")); !os.IsNotExist(err) {
		t.Fatalf("old version remains: %v; tree=%v", err, repositoryTree(t, repo.Root))
	}
	for path, want := range map[string]string{"library.js": "new script", "LICENSE": "new license"} {
		got, _ := os.ReadFile(filepath.Join(repo.Root, "vendor/library/1.1.0", path))
		if string(got) != want {
			t.Errorf("%s = %q", path, got)
		}
	}
}

func TestUpdateResourceFailureLeavesOldState(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, ".LICENSE") {
			w.WriteHeader(http.StatusBadGateway)
			return
		}
		_, _ = w.Write([]byte("new script"))
	}))
	defer server.Close()
	repo := testrepo.New(t, groupedManifest(server.URL, sri(t, "old script"), sri(t, "old license")))
	repo.Write(t, "vendor/library/1.0.0/library.js", []byte("old script"))
	repo.Write(t, "vendor/library/1.0.0/LICENSE", []byte("old license"))
	before, _ := os.ReadFile(repo.Manifest)
	engine, _ := New(repo.Manifest, Options{Strict: true, Transport: transport.Options{AllowHTTP: true}})
	if _, err := engine.UpdateResource(context.Background(), "library", "1.1.0"); err == nil {
		t.Fatal("expected update failure")
	}
	after, _ := os.ReadFile(repo.Manifest)
	if string(after) != string(before) {
		t.Fatal("failed update changed manifest")
	}
	if _, err := os.Stat(filepath.Join(repo.Root, "vendor/library/1.1.0")); !os.IsNotExist(err) {
		t.Fatalf("failed update left new tree: %v; tree=%v", err, repositoryTree(t, repo.Root))
	}
}

func repositoryTree(t *testing.T, root string) []string {
	t.Helper()
	var paths []string
	if err := filepath.WalkDir(root, func(path string, _ os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		relative, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		paths = append(paths, relative)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	return paths
}

func TestUpdateResourceRejectsModifiedOldFileBeforeNetwork(t *testing.T) {
	var requests atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests.Add(1)
		_, _ = w.Write([]byte("new"))
	}))
	defer server.Close()
	repo := testrepo.New(t, groupedManifest(server.URL, sri(t, "old script"), sri(t, "old license")))
	repo.Write(t, "vendor/library/1.0.0/library.js", []byte("locally modified"))
	repo.Write(t, "vendor/library/1.0.0/LICENSE", []byte("old license"))
	engine, _ := New(repo.Manifest, Options{Strict: true, Transport: transport.Options{AllowHTTP: true}})
	if _, err := engine.UpdateResource(context.Background(), "library", "1.1.0"); err == nil {
		t.Fatal("expected modified file failure")
	}
	if requests.Load() != 0 {
		t.Fatalf("network requests = %d", requests.Load())
	}
}

func TestUpdateDownloadRetrustsOnlySelectedArtifact(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, ".LICENSE") {
			_, _ = w.Write([]byte("old license"))
			return
		}
		_, _ = w.Write([]byte("replacement script"))
	}))
	defer server.Close()
	repo := testrepo.New(t, groupedManifest(server.URL, sri(t, "old script"), sri(t, "old license")))
	repo.Write(t, "vendor/library/1.0.0/library.js", []byte("old script"))
	repo.Write(t, "vendor/library/1.0.0/LICENSE", []byte("old license"))
	engine, _ := New(repo.Manifest, Options{Strict: true, Transport: transport.Options{AllowHTTP: true}})
	if _, err := engine.UpdateDownload(context.Background(), "library", "script"); err != nil {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(filepath.Join(repo.Root, "vendor/library/1.0.0/library.js"))
	if string(got) != "replacement script" {
		t.Fatalf("script = %q", got)
	}
	license, _ := os.ReadFile(filepath.Join(repo.Root, "vendor/library/1.0.0/LICENSE"))
	if string(license) != "old license" {
		t.Fatalf("license = %q", license)
	}
}
