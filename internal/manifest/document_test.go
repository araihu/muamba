package manifest

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeManifest(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "muamba.yaml")
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestLoadGroupedManifest(t *testing.T) {
	path := writeManifest(t, `schema: 1
resources:
  alpine:
    version: "3.14.9"
    downloads:
      core-js:
        url: https://cdn.example/alpine@${version}/alpine.js
        path: assets/alpine/${version}/alpine.js
`)
	doc, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	got := doc.Manifest.Resources["alpine"].Downloads["core-js"]
	if got.URL != "https://cdn.example/alpine@${version}/alpine.js" {
		t.Fatalf("URL = %q", got.URL)
	}
}

func TestLoadRejectsUnknownKey(t *testing.T) {
	path := writeManifest(t, "schema: 1\nresources: {}\nsurprise: true\n")
	if _, err := Load(path); err == nil || !strings.Contains(err.Error(), "surprise") {
		t.Fatalf("Load error = %v", err)
	}
}

func TestFindWalksParents(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "muamba.yaml")
	if err := os.WriteFile(path, []byte("schema: 1\nresources: {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	start := filepath.Join(root, "assets", "js")
	if err := os.MkdirAll(start, 0o755); err != nil {
		t.Fatal(err)
	}
	got, err := Find(start, "")
	if err != nil {
		t.Fatal(err)
	}
	if got != path {
		t.Fatalf("Find = %q, want %q", got, path)
	}
}

func TestMutationPreservesCommentsAndOrder(t *testing.T) {
	path := writeManifest(t, `schema: 1
resources:
  alpine:
    version: "3.14.9" # shared version
    downloads:
      core-js:
        url: https://cdn.example/alpine@${version}/alpine.js # upstream
        path: assets/alpine/${version}/alpine.js
`)
	doc, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := doc.SetVersion("alpine", "3.15.0"); err != nil {
		t.Fatal(err)
	}
	if err := doc.SetIntegrity("alpine", "core-js", "sha384-abc"); err != nil {
		t.Fatal(err)
	}
	b, err := doc.Marshal()
	if err != nil {
		t.Fatal(err)
	}
	got := string(b)
	for _, want := range []string{`version: "3.15.0" # shared version`, "# upstream", "path:", "integrity: sha384-abc"} {
		if !strings.Contains(got, want) {
			t.Errorf("marshaled YAML missing %q:\n%s", want, got)
		}
	}
	if strings.Index(got, "path:") > strings.Index(got, "integrity:") {
		t.Fatalf("integrity inserted before existing path:\n%s", got)
	}
}
