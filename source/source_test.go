package source

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestExplicitNamespaceLocksAndSnapshotsWithoutMuambaDiscovery(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("icon bytes"))
	}))
	defer server.Close()

	root := t.TempDir()
	manifest := filepath.Join(root, ".iconpack.engine.yaml")
	lock := filepath.Join(root, ".iconpack.lock.yaml")
	if err := os.WriteFile(filepath.Join(root, "muamba.yaml"), []byte("not a Muamba manifest\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	contents := "schema: 1\nresources:\n  demo:\n    version: v1\n    downloads:\n      icon:\n        url: " + server.URL + "/${version}/icon.svg\n        path: sources/demo.svg\n        max_size: 1MiB\n"
	if err := os.WriteFile(manifest, []byte(contents), 0o644); err != nil {
		t.Fatal(err)
	}

	engine, err := New(Options{
		ManifestPath: manifest,
		LockPath:     lock,
		CacheDir:     filepath.Join(root, "cache"),
		Strict:       true,
		AllowHTTP:    true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := engine.Lock(context.Background(), nil); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(lock); err != nil {
		t.Fatalf("explicit lock was not written: %v", err)
	}

	files, err := engine.Snapshot(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 1 || string(files[0].Contents) != "icon bytes" {
		t.Fatalf("snapshot = %#v", files)
	}
	if files[0].Path != "sources/demo.svg" || files[0].Integrity == "" {
		t.Fatalf("snapshot metadata = %#v", files[0])
	}
	if strings.Contains(string(files[0].Contents), "Muamba manifest") {
		t.Fatal("snapshot used the discovered parent manifest")
	}
	var streamed []byte
	if err := engine.Walk(context.Background(), nil, func(file File, r io.Reader) error {
		if file.Path != files[0].Path || file.Integrity != files[0].Integrity || file.Size != files[0].Size {
			t.Fatalf("stream metadata = %#v", file)
		}
		// Replacing the materialized source cannot change the verified reader.
		if err := os.WriteFile(filepath.Join(root, file.Path), []byte("tampered"), 0600); err != nil {
			return err
		}
		var err error
		streamed, err = io.ReadAll(r)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if string(streamed) != "icon bytes" {
		t.Fatalf("stream = %q", streamed)
	}
}

func TestInMemoryDeclarationUsesLogicalPathWithoutWritingAdapter(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("icon bytes"))
	}))
	defer server.Close()

	root := t.TempDir()
	manifest := filepath.Join(root, ".iconpack-engine")
	lock := filepath.Join(root, ".iconpack.lock.yaml")
	contents := []byte("schema: 1\nresources:\n  demo:\n    version: v1\n    downloads:\n      icon:\n        url: " + server.URL + "/${version}/icon.svg\n        path: sources/demo.svg\n        max_size: 1MiB\n")

	engine, err := New(Options{
		ManifestPath: manifest, ManifestBytes: contents, LockPath: lock,
		CacheDir: filepath.Join(root, "cache"), Strict: true, AllowHTTP: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(manifest); !os.IsNotExist(err) {
		t.Fatalf("in-memory declaration path exists before use: %v", err)
	}
	if _, err := engine.Lock(context.Background(), nil); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(manifest); !os.IsNotExist(err) {
		t.Fatalf("in-memory declaration path was written: %v", err)
	}
	if _, err := engine.Snapshot(context.Background(), nil); err != nil {
		t.Fatal(err)
	}
}

func TestNewRequiresExplicitDeclarationAndLockPaths(t *testing.T) {
	if _, err := New(Options{LockPath: ".iconpack.lock.yaml"}); err == nil {
		t.Fatal("missing declaration path was accepted")
	}
	if _, err := New(Options{ManifestPath: ".iconpack.engine.yaml"}); err == nil {
		t.Fatal("missing lock path was accepted")
	}
}
