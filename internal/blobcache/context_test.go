package blobcache

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/gofrs/flock"
)

func TestSeedContextCancelsWhileCacheLockIsHeld(t *testing.T) {
	store, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	source := filepath.Join(t.TempDir(), "source")
	if err := os.WriteFile(source, []byte("payload"), 0600); err != nil {
		t.Fatal(err)
	}
	digest := testDigest(t, "payload")
	target := store.Path(digest)
	if err := os.MkdirAll(filepath.Dir(target), 0700); err != nil {
		t.Fatal(err)
	}
	guard := flock.New(target + ".lock")
	if err := guard.Lock(); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = guard.Unlock(); _ = guard.Close() }()
	ctx, cancel := context.WithTimeout(t.Context(), 50*time.Millisecond)
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- store.SeedContext(ctx, source, digest) }()
	select {
	case err := <-done:
		if !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("seed error: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("seed ignored cancellation while waiting for cache lock")
	}
	if _, err := os.Stat(target); !os.IsNotExist(err) {
		t.Fatalf("cancelled seed published blob: %v", err)
	}
}

func TestSeedContextRejectsAlreadyCancelledContext(t *testing.T) {
	store, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if err := store.SeedContext(ctx, "missing", testDigest(t, "payload")); !errors.Is(err, context.Canceled) {
		t.Fatalf("seed error: %v", err)
	}
}
