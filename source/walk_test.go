package source

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

func walkFixture(t *testing.T) (*Engine, string) {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, strings.Repeat(r.URL.Path, 1024))
	}))
	t.Cleanup(server.Close)
	root := t.TempDir()
	declaration := "schema: 1\nresources:\n  demo:\n    version: v1\n    downloads:\n"
	for _, name := range []string{"b", "a"} {
		declaration += "      " + name + ":\n        url: " + server.URL + "/v1/" + name + "\n        path: sources/" + name + "\n        max_size: 1MiB\n"
	}
	engine, err := New(Options{ManifestPath: filepath.Join(root, "source.yaml"), ManifestBytes: []byte(declaration), LockPath: filepath.Join(root, "lock.yaml"), CacheDir: filepath.Join(root, "cache"), AllowHTTP: true, Strict: true})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := engine.Lock(t.Context(), nil); err != nil {
		t.Fatal(err)
	}
	return engine, root
}

func TestWalkIntegrityOrderingCancellationAndCleanup(t *testing.T) {
	for _, mode := range []string{"ordered", "rewrite", "replace", "cancel", "callback", "lock"} {
		t.Run(mode, func(t *testing.T) {
			engine, root := walkFixture(t)
			staging := t.TempDir()
			t.Setenv("TMPDIR", staging)
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			sentinel := errors.New("callback failed")
			var paths []string
			err := engine.Walk(ctx, nil, func(file File, r io.Reader) error {
				paths = append(paths, file.Path)
				if file.Path == "sources/a" {
					target := filepath.Join(root, "sources/b")
					switch mode {
					case "rewrite", "replace":
						if mode == "replace" {
							if err := os.Remove(target); err != nil {
								return err
							}
						}
						if err := os.WriteFile(target, []byte(strings.Repeat("!", 5120)), 0600); err != nil {
							return err
						}
					case "cancel":
						cancel()
					case "callback":
						return sentinel
					case "lock":
						blocked, stop := context.WithTimeout(ctx, 25*time.Millisecond)
						defer stop()
						if _, err := engine.Sync(blocked, nil); err == nil {
							t.Fatal("mutation lock released during callback")
						}
					}
				}
				_, err := io.Copy(io.Discard, r)
				return err
			})
			switch mode {
			case "ordered", "lock":
				if err != nil || !reflect.DeepEqual(paths, []string{"sources/a", "sources/b"}) {
					t.Fatalf("paths=%v err=%v", paths, err)
				}
			case "cancel":
				if !errors.Is(err, context.Canceled) {
					t.Fatalf("error=%v", err)
				}
			case "callback":
				if !errors.Is(err, sentinel) {
					t.Fatalf("error=%v", err)
				}
			default:
				if err == nil || !strings.Contains(err.Error(), "integrity mismatch") || len(paths) != 1 {
					t.Fatalf("unverified file visited: paths=%v err=%v", paths, err)
				}
			}
			entries, err := os.ReadDir(staging)
			if err != nil {
				t.Fatalf("staging leaked: %v %v", entries, err)
			}
			assertNoWalkStaging(t, entries)
			// Failure must release the namespace lock as well as temporary files.
			if _, err := engine.Sync(t.Context(), nil); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func assertNoWalkStaging(t *testing.T, entries []os.DirEntry) {
	t.Helper()
	for _, entry := range entries {
		if entry.Name() != "muamba-locks" {
			t.Fatalf("staging leaked: %s", entry.Name())
		}
	}
}

func TestWalkRejectsInvalidVisitorsAndCancelledContext(t *testing.T) {
	if err := (*Engine)(nil).Walk(t.Context(), nil, nil); err == nil {
		t.Fatal("nil engine accepted")
	}
	engine, _ := walkFixture(t)
	if err := engine.Walk(t.Context(), nil, nil); err == nil {
		t.Fatal("nil visitor accepted")
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if err := engine.Walk(ctx, nil, func(File, io.Reader) error { t.Fatal("cancelled visitor called"); return nil }); err == nil {
		t.Fatal("cancelled walk succeeded")
	}
}
