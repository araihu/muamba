package testrepo

import (
	"os"
	"path/filepath"
	"testing"
)

type Repo struct {
	Root     string
	Manifest string
}

func New(t testing.TB, manifest string) Repo {
	t.Helper()
	root := t.TempDir()
	path := filepath.Join(root, "muamba.yaml")
	if err := os.WriteFile(path, []byte(manifest), 0o644); err != nil {
		t.Fatal(err)
	}
	return Repo{Root: root, Manifest: path}
}

func (r Repo) Write(t testing.TB, path string, contents []byte) string {
	t.Helper()
	full := filepath.Join(r.Root, filepath.FromSlash(path))
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(full, contents, 0o644); err != nil {
		t.Fatal(err)
	}
	return full
}
