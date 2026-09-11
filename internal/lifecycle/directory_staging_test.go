package lifecycle

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/araihu/muamba/internal/blobcache"
	"github.com/araihu/muamba/internal/integrity"
	"github.com/araihu/muamba/internal/manifest"
	"github.com/araihu/muamba/internal/transport"
)

func TestRejectedDirectoryDoesNotSeedCache(t *testing.T) {
	var archive bytes.Buffer
	gz := gzip.NewWriter(&archive)
	tw := tar.NewWriter(gz)
	for _, name := range []string{"good.svg", "../escape"} {
		if err := tw.WriteHeader(&tar.Header{Name: name, Mode: 0600, Size: 4}); err != nil {
			t.Fatal(err)
		}
		if _, err := tw.Write([]byte("good")); err != nil {
			t.Fatal(err)
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write(archive.Bytes()) }))
	defer server.Close()
	store, err := blobcache.New(filepath.Join(t.TempDir(), "cache"))
	if err != nil {
		t.Fatal(err)
	}
	engine := &Engine{cache: store, document: &manifest.Document{Dir: t.TempDir()}}
	client, err := transport.New(transport.Options{AllowHTTP: true})
	if err != nil {
		t.Fatal(err)
	}
	directory := manifest.DirectorySelection{ResourceName: "icons", DirectoryName: "tree", URL: server.URL, Path: "icons", Include: []string{"*.svg"}, MaxFiles: 10, MaxBytes: 1 << 20, MaxUnpackedBytes: 1 << 20}
	if _, _, err := engine.acquireDirectory(t.Context(), client, directory); err == nil {
		t.Fatal("unsafe archive accepted")
	}
	digest, err := integrity.Parse(sri(t, "good"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(store.Path(digest)); !os.IsNotExist(err) {
		t.Fatalf("rejected archive seeded cache: %v", err)
	}
	archiveDigest, err := digestBytes(archive.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	directory.Lock = &manifest.LockedDirectory{
		Integrity: integrity.FormatSRI(archiveDigest.Algorithm, archiveDigest.Sum),
		Size:      int64(archive.Len()),
		Files:     []manifest.LockedDirectoryFile{{Source: "good.svg", Path: "icons/good.svg", Size: 4, Integrity: sri(t, "good")}},
	}
	for attempt := range 2 {
		if _, _, err := engine.syncDirectory(t.Context(), client, directory); err == nil {
			t.Fatalf("attempt %d accepted rejected archive through cache", attempt)
		}
		if _, err := os.Stat(store.Path(digest)); !os.IsNotExist(err) {
			t.Fatalf("attempt %d seeded cache: %v", attempt, err)
		}
	}
}
