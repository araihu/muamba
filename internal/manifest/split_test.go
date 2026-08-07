package manifest

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestFindPrefersSplitDeclarationAndFallsBackToLegacy(t *testing.T) {
	root := t.TempDir()
	nested := filepath.Join(root, "nested")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatal(err)
	}
	legacy := filepath.Join(root, "muamba.yaml")
	if err := os.WriteFile(legacy, []byte("schema: 1\nresources: {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := Find(nested, "")
	if err != nil || got != legacy {
		t.Fatalf("legacy Find() = %q, %v", got, err)
	}
	declaration := filepath.Join(root, ".muamba.yaml")
	if err := os.WriteFile(declaration, []byte("schema: 1\nresources: {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err = Find(nested, "")
	if err != nil || got != declaration {
		t.Fatalf("split Find() = %q, %v", got, err)
	}
}

func TestLoadSplitLockOverlaysExactResolvedFile(t *testing.T) {
	root := t.TempDir()
	declaration := filepath.Join(root, ".muamba.yaml")
	writeTestFile(t, declaration, `schema: 1
resources:
  library:
    version: "1.2.3"
    downloads:
      script:
        url: https://cdn.example/library-${version}.js
        path: vendor/library.js
`)
	writeTestFile(t, filepath.Join(root, ".muamba.lock.yaml"), `schema: 1
files:
  - id: library/script
    url: https://cdn.example/library-1.2.3.js
    path: vendor/library.js
    size: 7
    integrity: sha384-YWJj
`)
	doc, err := Load(declaration)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := doc.Validate(false); err == nil || !strings.Contains(err.Error(), "digest length") {
		t.Fatalf("Validate() error = %v", err)
	}
}

func TestSplitLockRejectsDeclarationDrift(t *testing.T) {
	root := t.TempDir()
	declaration := filepath.Join(root, ".muamba.yaml")
	writeTestFile(t, declaration, `schema: 1
resources:
  library:
    version: "1.2.3"
    downloads:
      script:
        url: https://cdn.example/library-${version}.js
        path: vendor/library.js
`)
	writeTestFile(t, filepath.Join(root, ".muamba.lock.yaml"), `schema: 1
files:
  - id: library/script
    url: https://cdn.example/other-1.2.3.js
    path: vendor/library.js
    size: 7
    integrity: sha384-AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA
`)
	doc, err := Load(declaration)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := doc.Validate(false); err == nil || !strings.Contains(err.Error(), "lock URL") {
		t.Fatalf("Validate() error = %v", err)
	}
}

func TestMarshalLockIsDeterministicAndIncludesExactMetadata(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".muamba.yaml")
	writeTestFile(t, path, `schema: 1
resources:
  zed:
    version: "1"
    downloads:
      beta:
        url: https://example.test/zed-1-beta
        path: vendor/beta
      alpha:
        url: https://example.test/zed-1-alpha
        path: vendor/alpha
`)
	doc, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := doc.Validate(false); err != nil {
		t.Fatal(err)
	}
	selections, err := doc.SelectAll(nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, selection := range selections {
		if err := doc.SetLock(selection, int64(len(selection.DownloadName)), "sha384-AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"); err != nil {
			t.Fatal(err)
		}
	}
	first, err := doc.MarshalLock()
	if err != nil {
		t.Fatal(err)
	}
	second, err := doc.MarshalLock()
	if err != nil {
		t.Fatal(err)
	}
	if string(first) != string(second) {
		t.Fatalf("nondeterministic lock:\n%s\n---\n%s", first, second)
	}
	text := string(first)
	if strings.Index(text, "zed/alpha") > strings.Index(text, "zed/beta") {
		t.Fatalf("lock not sorted:\n%s", text)
	}
	for _, want := range []string{"url: https://example.test/zed-1-alpha", "path: vendor/alpha", "size: 5", "integrity: sha384-"} {
		if !strings.Contains(text, want) {
			t.Fatalf("lock missing %q:\n%s", want, text)
		}
	}
}

func writeTestFile(t *testing.T, path, contents string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		t.Fatal(err)
	}
}
