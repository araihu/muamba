package gogen

import (
	"crypto"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/araihu/muamba/internal/integrity"
	"github.com/araihu/muamba/internal/manifest"
	"github.com/araihu/muamba/internal/testrepo"
)

func generatorFixture(t *testing.T) (*manifest.Document, string) {
	t.Helper()
	sum, err := integrity.Compute(strings.NewReader("script"), crypto.SHA384)
	if err != nil {
		t.Fatal(err)
	}
	digest := integrity.FormatSRI(crypto.SHA384, sum)
	repo := testrepo.New(t, `schema: 1
resources:
  web-lib:
    version: "1.2.3"
    downloads:
      core-js:
        url: https://cdn.example/web-lib@${version}/core.js
        path: assets/vendor/core.js
        integrity: `+digest+`
      package-copy:
        url: https://cdn.example/web-lib@${version}/copy.js
        path: assets/vendor/copy.js
        integrity: `+digest+`
      site-copy:
        url: https://cdn.example/web-lib@${version}/site.js
        path: site/static/site.js
        integrity: `+digest+`
`)
	repo.Write(t, "assets/vendor/core.js", []byte("script"))
	repo.Write(t, "assets/vendor/copy.js", []byte("script"))
	repo.Write(t, "site/static/site.js", []byte("script"))
	repo.Write(t, "assets/doc.go", []byte("package assets\n"))
	doc, err := manifest.Load(repo.Manifest)
	if err != nil {
		t.Fatal(err)
	}
	return doc, repo.Root
}

func TestGenerateSelectsPackageDownloadsAndIsDeterministic(t *testing.T) {
	doc, root := generatorFixture(t)
	options := Options{Dir: "assets", Output: "muamba_gen.go", Strict: true}
	if err := Generate(doc, options); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "assets", "muamba_gen.go")
	first, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	text := string(first)
	for _, want := range []string{
		"package assets",
		"//go:embed vendor/copy.js vendor/core.js",
		"muambaResourceWebLib",
		"muambaDownloadWebLibCoreJs",
		"Hash      string",
		`Hash: "sha384:eae7b977641a3f095a30684d56763e8f876e8367e79ab0f8cf127170bd86ffea27fe0c1150c579b75e9b2a5635b3c08a"`,
		"func MuambaHash",
		"func MuambaOpen",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("generated source missing %q:\n%s", want, text)
		}
	}
	if strings.Contains(text, "site-copy") {
		t.Fatalf("generated source contains package-external download:\n%s", text)
	}
	if err := Generate(doc, options); err != nil {
		t.Fatal(err)
	}
	second, _ := os.ReadFile(path)
	if string(second) != string(first) {
		t.Fatal("generation is not deterministic")
	}
}

func TestGenerateCheckDetectsDriftWithoutWriting(t *testing.T) {
	doc, root := generatorFixture(t)
	path := filepath.Join(root, "assets", "muamba_gen.go")
	if err := os.WriteFile(path, []byte("package assets\n// stale\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	err := Generate(doc, Options{Dir: "assets", Output: "muamba_gen.go", Check: true, Strict: true})
	if err == nil || !strings.Contains(err.Error(), "stale") {
		t.Fatalf("Generate(check) error = %v", err)
	}
	got, _ := os.ReadFile(path)
	if string(got) != "package assets\n// stale\n" {
		t.Fatal("check mode changed output")
	}
}

func TestGeneratedPackageCompilesAndReturnsCallerOwnedSlices(t *testing.T) {
	doc, root := generatorFixture(t)
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module fixture\n\ngo 1.26.5\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := Generate(doc, Options{Dir: "assets", Output: "muamba_gen.go", Strict: true}); err != nil {
		t.Fatal(err)
	}
	testSource := `package assets
import (
  "io"
  "testing"
)
func TestGenerated(t *testing.T) {
  first := MuambaResources()
  const integrity = "sha384-6ue5d2QaPwlaMGhNVnY+j4dug2fnmrD4zxJxcL2G/+on/gwRUMV5t16bKlY1s8CK"
  const hash = "sha384:eae7b977641a3f095a30684d56763e8f876e8367e79ab0f8cf127170bd86ffea27fe0c1150c579b75e9b2a5635b3c08a"
  if first[0].Downloads[0].Integrity != integrity { t.Fatalf("integrity = %q", first[0].Downloads[0].Integrity) }
  if first[0].Downloads[0].Hash != hash { t.Fatalf("hash = %q", first[0].Downloads[0].Hash) }
  if got, ok := MuambaHash("web-lib", "core-js"); !ok || got != hash { t.Fatalf("MuambaHash = %q, %v", got, ok) }
  if got, ok := MuambaHash("missing", "asset"); ok || got != "" { t.Fatalf("missing MuambaHash = %q, %v", got, ok) }
  first[0].Downloads[0].Name = "mutated"
  if MuambaResources()[0].Downloads[0].Name != "core-js" { t.Fatal("registry mutated") }
  file, err := MuambaOpen("web-lib", "core-js")
  if err != nil { t.Fatal(err) }
  defer file.Close()
  body, _ := io.ReadAll(file)
  if string(body) != "script" { t.Fatalf("body = %q", body) }
}
`
	if err := os.WriteFile(filepath.Join(root, "assets", "generated_test.go"), []byte(testSource), 0o644); err != nil {
		t.Fatal(err)
	}
	command := exec.Command("go", "test", "./assets")
	command.Dir = root
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("generated package: %v\n%s", err, output)
	}
}

func TestGenerateRequiresPackageForEmptyDirectory(t *testing.T) {
	doc, _ := generatorFixture(t)
	if err := Generate(doc, Options{Dir: "site/static", Output: "muamba_gen.go"}); err == nil || !strings.Contains(err.Error(), "package") {
		t.Fatalf("Generate error = %v", err)
	}
}

func TestGenerateSelectsExplicitPlatformTarget(t *testing.T) {
	linuxIntegrity := digestFor(t, "linux bytes")
	darwinIntegrity := digestFor(t, "darwin bytes")
	repo := testrepo.New(t, `schema: 1
resources:
  tool:
    version: "1.0.0"
    downloads:
      cli:
        path: assets/vendor/tool
        platforms:
          linux/amd64:
            url: https://example.com/${version}/linux
            integrity: `+linuxIntegrity+`
          darwin/arm64:
            url: https://example.com/${version}/darwin
            integrity: `+darwinIntegrity+`
`)
	repo.Write(t, "assets/vendor/tool", []byte("linux bytes"))
	repo.Write(t, "assets/doc.go", []byte("package assets\n"))
	doc, err := manifest.Load(repo.Manifest)
	if err != nil {
		t.Fatal(err)
	}
	options := Options{
		Dir:    "assets",
		Output: "muamba_gen.go",
		Strict: true,
		Target: manifest.Target{GOOS: "linux", GOARCH: "amd64"},
	}
	if err := Generate(doc, options); err != nil {
		t.Fatal(err)
	}
	generated, err := os.ReadFile(filepath.Join(repo.Root, "assets", "muamba_gen.go"))
	if err != nil {
		t.Fatal(err)
	}
	text := string(generated)
	if !strings.Contains(text, `/1.0.0/linux`) || !strings.Contains(text, linuxIntegrity) {
		t.Fatalf("generated Linux registry =\n%s", text)
	}
	options.Target = manifest.Target{GOOS: "darwin", GOARCH: "arm64"}
	if err := Generate(doc, options); err == nil || !strings.Contains(err.Error(), "integrity mismatch") {
		t.Fatalf("Darwin generation error = %v", err)
	}
}

func TestGenerateSelectsRuntimeTargetByDefault(t *testing.T) {
	target := manifest.RuntimeTarget()
	contents := "runtime bytes"
	digest := digestFor(t, contents)
	repo := testrepo.New(t, fmt.Sprintf(`schema: 1
resources:
  tool:
    version: "1.0.0"
    downloads:
      cli:
        path: assets/vendor/tool
        platforms:
          %s:
            url: https://example.com/${version}/runtime
            integrity: %s
`, target.String(), digest))
	repo.Write(t, "assets/vendor/tool", []byte(contents))
	repo.Write(t, "assets/doc.go", []byte("package assets\n"))
	doc, err := manifest.Load(repo.Manifest)
	if err != nil {
		t.Fatal(err)
	}
	if err := Generate(doc, Options{Dir: "assets", Output: "muamba_gen.go", Strict: true}); err != nil {
		t.Fatal(err)
	}
	generated, err := os.ReadFile(filepath.Join(repo.Root, "assets", "muamba_gen.go"))
	if err != nil {
		t.Fatal(err)
	}
	text := string(generated)
	if !strings.Contains(text, `/1.0.0/runtime`) || !strings.Contains(text, digest) {
		t.Fatalf("generated runtime registry =\n%s", text)
	}
}

func digestFor(t *testing.T, contents string) string {
	t.Helper()
	sum, err := integrity.Compute(strings.NewReader(contents), crypto.SHA384)
	if err != nil {
		t.Fatal(err)
	}
	return integrity.FormatSRI(crypto.SHA384, sum)
}
