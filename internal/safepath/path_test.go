package safepath

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/araihu/muamba/internal/manifest"
)

func TestResolve(t *testing.T) {
	root := t.TempDir()
	got, err := Resolve(root, "assets/js/app.js")
	if err != nil {
		t.Fatal(err)
	}
	realRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(realRoot, "assets", "js", "app.js")
	if got != want {
		t.Fatalf("Resolve = %q, want %q", got, want)
	}
	for _, path := range []string{"", ".", "../escape", filepath.Join(root, "absolute")} {
		if _, err := Resolve(root, path); err == nil {
			t.Errorf("Resolve(%q) succeeded", path)
		}
	}
}

func TestResolveRejectsSymlinkEscape(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	if err := os.Symlink(outside, filepath.Join(root, "escape")); err != nil {
		t.Fatal(err)
	}
	if _, err := Resolve(root, "escape/file.js"); err == nil {
		t.Fatal("expected symlink escape error")
	}
}

func TestResolveAllowsSymlinkInsideRoot(t *testing.T) {
	root := t.TempDir()
	inside := filepath.Join(root, "real")
	if err := os.Mkdir(inside, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(inside, filepath.Join(root, "alias")); err != nil {
		t.Fatal(err)
	}
	got, err := Resolve(root, "alias/file.js")
	if err != nil {
		t.Fatal(err)
	}
	realInside, err := filepath.EvalSymlinks(inside)
	if err != nil {
		t.Fatal(err)
	}
	if got != filepath.Join(realInside, "file.js") {
		t.Fatalf("Resolve = %q", got)
	}
}

func TestValidateUnique(t *testing.T) {
	root := t.TempDir()
	selections := []manifest.Selection{
		{ResourceName: "a", DownloadName: "js", Path: "assets/app.js"},
		{ResourceName: "b", DownloadName: "js", Path: "assets/app.js"},
	}
	if err := ValidateUnique(root, selections); err == nil {
		t.Fatal("expected duplicate path error")
	}
}
