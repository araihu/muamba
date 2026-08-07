package manifest

import (
	"crypto"
	"path/filepath"
	"strings"
	"testing"

	"github.com/araihu/muamba/internal/integrity"
)

func TestDirectoryDeclarationLoadsExactLock(t *testing.T) {
	root := t.TempDir()
	declaration := filepath.Join(root, ".muamba.yaml")
	writeTestFile(t, declaration, `schema: 1
resources:
  heroicons:
    version: v2.2.0
    directories:
      optimized:
        url: https://github.com/tailwindlabs/heroicons/archive/${version}.tar.gz
        archive: tar.gz
        path: vendor/heroicons/${version}
        include:
          - optimized/**/*.svg
        exclude:
          - optimized/**/experimental-*.svg
        strip_components: 1
        max_size: 32MiB
        max_files: 4096
        max_unpacked_size: 128MiB
`)
	writeTestFile(t, filepath.Join(root, ".muamba.lock.yaml"), `schema: 1
directories:
  - id: heroicons/optimized
    url: https://github.com/tailwindlabs/heroicons/archive/v2.2.0.tar.gz
    path: vendor/heroicons/v2.2.0
    size: 100
    integrity: sha384-AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA
    files:
      - source: optimized/24/outline/a.svg
        path: vendor/heroicons/v2.2.0/optimized/24/outline/a.svg
        size: 5
        integrity: sha384-AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA
`)
	doc, err := Load(declaration)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := doc.Validate(true); err != nil {
		t.Fatal(err)
	}
	directories, err := doc.SelectDirectories(nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(directories) != 1 || directories[0].Lock == nil || len(directories[0].Lock.Files) != 1 {
		t.Fatalf("directories = %#v", directories)
	}
	if directories[0].MaxFiles != 4096 || directories[0].MaxUnpackedBytes != 128<<20 {
		t.Fatalf("bounds = %#v", directories[0])
	}
}

func TestDirectoryLockCloneSelectAndClear(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".muamba.yaml")
	writeTestFile(t, path, `schema: 1
resources:
  icons:
    version: v1
    downloads:
      license:
        url: https://example.test/icons-v1/LICENSE
        path: vendor/LICENSE
    directories:
      source:
        url: https://example.test/icons-v1.tar.gz
        archive: tar.gz
        path: vendor/icons
        include: ["icons/**/*.svg"]
        strip_components: 1
        max_size: 1MiB
        max_files: 10
        max_unpacked_size: 1MiB
`)
	doc, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if doc.IsLegacy() {
		t.Fatal("split declaration marked legacy")
	}
	if _, err := doc.Validate(true); err != nil {
		t.Fatal(err)
	}
	digest, err := integrity.Compute(strings.NewReader("x"), crypto.SHA384)
	if err != nil {
		t.Fatal(err)
	}
	sri := integrity.FormatSRI(crypto.SHA384, digest)
	locked := LockedDirectory{
		ID: "icons/source", URL: "https://example.test/icons-v1.tar.gz", Path: "vendor/icons",
		Size: 42, Integrity: sri,
		Files: []LockedDirectoryFile{{Source: "icons/a.svg", Path: "vendor/icons/icons/a.svg", Size: 1, Integrity: sri}},
	}
	if err := doc.SetDirectoryLock(locked); err != nil {
		t.Fatal(err)
	}
	files, err := doc.SelectAll([]string{"icons/license"})
	if err != nil || len(files) != 1 {
		t.Fatalf("files = %#v, %v", files, err)
	}
	files[0].Size = 7
	if err := doc.SetLock(files[0], 7, sri); err != nil {
		t.Fatal(err)
	}
	if _, err := doc.Validate(true); err != nil {
		t.Fatal(err)
	}
	clone, err := doc.Clone()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := clone.Validate(true); err != nil {
		t.Fatal(err)
	}
	for _, selectors := range [][]string{nil, {"icons"}, {"icons/source"}, {"icons/license"}} {
		if _, err := clone.SelectDirectories(selectors); err != nil {
			t.Fatalf("SelectDirectories(%v): %v", selectors, err)
		}
	}
	if _, err := clone.SelectDirectories([]string{"icons/missing"}); err == nil {
		t.Fatal("unknown directory accepted")
	}
	if _, err := clone.SelectDirectories([]string{"bad/selector/shape"}); err == nil {
		t.Fatal("invalid selector accepted")
	}
	if err := clone.ClearResourceLocks("icons"); err != nil {
		t.Fatal(err)
	}
	if err := clone.ClearResourceLocks("missing"); err == nil {
		t.Fatal("unknown resource locks cleared")
	}
	if _, err := clone.Validate(true); err != nil {
		t.Fatal(err)
	}
	directories, err := clone.SelectDirectories(nil)
	if err != nil || len(directories) != 1 || directories[0].Lock != nil {
		t.Fatalf("cleared directories = %#v, %v", directories, err)
	}
}

func TestDirectoryDeclarationValidationFailures(t *testing.T) {
	base := `schema: 1
resources:
  icons:
    version: v1
    directories:
      source:
        url: https://example.test/icons-v1.tar.gz
        archive: tar.gz
        path: vendor/icons
        include: ["icons/**/*.svg"]
        strip_components: 1
        max_size: 1MiB
        max_files: 10
        max_unpacked_size: 1MiB
`
	tests := map[string]struct{ old, new string }{
		"archive":        {"archive: tar.gz", "archive: zip"},
		"include":        {`include: ["icons/**/*.svg"]`, "include: []"},
		"strip":          {"strip_components: 1", "strip_components: -1"},
		"files":          {"max_files: 10", "max_files: 0"},
		"response bound": {"max_size: 1MiB", "max_size: nope"},
		"unpacked bound": {"max_unpacked_size: 1MiB", "max_unpacked_size: nope"},
		"path":           {"path: vendor/icons", "path: ../escape"},
		"glob":           {`include: ["icons/**/*.svg"]`, `include: ["../escape"]`},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), ".muamba.yaml")
			writeTestFile(t, path, strings.Replace(base, test.old, test.new, 1))
			doc, err := Load(path)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := doc.Validate(true); err == nil {
				t.Fatal("invalid directory accepted")
			}
		})
	}
}

func TestValidateBasePlatformBranches(t *testing.T) {
	for _, platform := range []string{"", "multi", "linux/amd64"} {
		if got, err := validateBasePlatform("tool", "cli", platform); err != nil || got == "" {
			t.Fatalf("validateBasePlatform(%q) = %q, %v", platform, got, err)
		}
	}
	if _, err := validateBasePlatform("tool", "cli", "invalid"); err == nil {
		t.Fatal("invalid platform accepted")
	}
}

func TestDirectoryDeclarationRequiresExplicitSafetyBounds(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".muamba.yaml")
	writeTestFile(t, path, `schema: 1
resources:
  icons:
    version: v1
    directories:
      source:
        url: https://example.test/icons-v1.tar.gz
        archive: tar.gz
        path: vendor/icons
        include: ["**/*.svg"]
`)
	doc, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := doc.Validate(true); err == nil {
		t.Fatal("directory without explicit bounds accepted")
	}
}
