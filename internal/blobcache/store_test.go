package blobcache

import (
	"crypto"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/araihu/muamba/internal/integrity"
)

func testDigest(t *testing.T, value string) integrity.Digest {
	t.Helper()
	sum, err := integrity.Compute(strings.NewReader(value), crypto.SHA384)
	if err != nil {
		t.Fatal(err)
	}
	return integrity.Digest{Algorithm: crypto.SHA384, Sum: sum}
}

func TestStoreSeedsVerifiesAndMaterializes(t *testing.T) {
	root := t.TempDir()
	store, err := New(root)
	if err != nil {
		t.Fatal(err)
	}
	source := filepath.Join(t.TempDir(), "source")
	if err := os.WriteFile(source, []byte("payload"), 0o600); err != nil {
		t.Fatal(err)
	}
	digest := testDigest(t, "payload")
	wantPath := filepath.Join(root, "sha384", hex.EncodeToString(digest.Sum))
	if got := store.Path(digest); got != wantPath {
		t.Fatalf("Path = %q, want %q", got, wantPath)
	}
	if err := store.Seed(source, digest); err != nil {
		t.Fatal(err)
	}
	if err := store.Verify(digest); err != nil {
		t.Fatal(err)
	}

	destination := filepath.Join(t.TempDir(), "bin", "tool")
	if err := store.Materialize(digest, destination, 0o755); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(destination)
	if err != nil || string(got) != "payload" {
		t.Fatalf("materialized = %q, %v", got, err)
	}
	info, err := os.Stat(destination)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o755 {
		t.Fatalf("mode = %v", info.Mode().Perm())
	}
}

func TestStoreRepairsCorruptBlobFromVerifiedSource(t *testing.T) {
	store, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	source := filepath.Join(t.TempDir(), "source")
	if err := os.WriteFile(source, []byte("expected"), 0o600); err != nil {
		t.Fatal(err)
	}
	digest := testDigest(t, "expected")
	if err := store.Seed(source, digest); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(store.Path(digest), []byte("corrupt"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := store.Seed(source, digest); err != nil {
		t.Fatal(err)
	}
	if err := store.Verify(digest); err != nil {
		t.Fatal(err)
	}
}

func TestStoreRejectsCorruptSourceAndMissingBlob(t *testing.T) {
	store, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	digest := testDigest(t, "expected")
	source := filepath.Join(t.TempDir(), "source")
	if err := os.WriteFile(source, []byte("wrong"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := store.Seed(source, digest); err == nil {
		t.Fatal("Seed accepted corrupt source")
	}
	if _, err := os.Stat(store.Path(digest)); !os.IsNotExist(err) {
		t.Fatalf("cache blob exists: %v", err)
	}
	if err := store.Verify(digest); err == nil {
		t.Fatal("Verify accepted missing blob")
	}
	if err := store.Materialize(digest, filepath.Join(t.TempDir(), "tool"), 0o755); err == nil {
		t.Fatal("Materialize accepted missing blob")
	}
}

func TestStoreConcurrentSeedPublishesOneCompleteBlob(t *testing.T) {
	store, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	source := filepath.Join(t.TempDir(), "source")
	contents := strings.Repeat("concurrent payload", 1024)
	if err := os.WriteFile(source, []byte(contents), 0o600); err != nil {
		t.Fatal(err)
	}
	digest := testDigest(t, contents)

	var wait sync.WaitGroup
	errors := make(chan error, 16)
	for range 16 {
		wait.Add(1)
		go func() {
			defer wait.Done()
			errors <- store.Seed(source, digest)
		}()
	}
	wait.Wait()
	close(errors)
	for err := range errors {
		if err != nil {
			t.Errorf("Seed = %v", err)
		}
	}
	if err := store.Verify(digest); err != nil {
		t.Fatal(err)
	}
	entries, err := os.ReadDir(filepath.Dir(store.Path(digest)))
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), ".muamba-") {
			t.Errorf("temporary file remains: %s", entry.Name())
		}
	}
}

func TestStoreRepairNeverMakesBlobPathDisappear(t *testing.T) {
	store, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	contents := strings.Repeat("verified payload", 8192)
	source := filepath.Join(t.TempDir(), "source")
	if err := os.WriteFile(source, []byte(contents), 0o600); err != nil {
		t.Fatal(err)
	}
	digest := testDigest(t, contents)
	if err := store.Seed(source, digest); err != nil {
		t.Fatal(err)
	}
	for range 20 {
		if err := os.WriteFile(store.Path(digest), []byte("corrupt"), 0o644); err != nil {
			t.Fatal(err)
		}
		done := make(chan error, 1)
		go func() { done <- store.Seed(source, digest) }()
		for {
			select {
			case err := <-done:
				if err != nil {
					t.Fatal(err)
				}
				if err := store.Verify(digest); err != nil {
					t.Fatal(err)
				}
				goto repaired
			default:
				if err := store.Verify(digest); errors.Is(err, os.ErrNotExist) {
					t.Fatalf("cache path disappeared during repair: %v", err)
				}
			}
		}
	repaired:
	}
}

func TestNewRejectsEmptyRoot(t *testing.T) {
	if _, err := New(""); err == nil {
		t.Fatal("New accepted empty root")
	}
}
