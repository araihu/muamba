package manifest

import (
	"strings"
	"testing"
)

func TestParseTargetRequiresExactGoPair(t *testing.T) {
	got, err := ParseTarget("linux/amd64")
	if err != nil {
		t.Fatal(err)
	}
	if got.GOOS != "linux" || got.GOARCH != "amd64" || got.String() != "linux/amd64" {
		t.Fatalf("ParseTarget = %#v", got)
	}

	for _, value := range []string{"", "multi", "x64", "macos/arm64", "linux/", "/amd64", "linux/amd64/gnu", "Linux/amd64", "linux/amd_64"} {
		t.Run(value, func(t *testing.T) {
			if _, err := ParseTarget(value); err == nil {
				t.Fatalf("ParseTarget(%q) succeeded", value)
			}
		})
	}
}

func TestRuntimeTargetIsValid(t *testing.T) {
	target := RuntimeTarget()
	if target.GOOS == "" || target.GOARCH == "" {
		t.Fatalf("RuntimeTarget = %#v", target)
	}
	if parsed, err := ParseTarget(target.String()); err != nil || parsed != target {
		t.Fatalf("ParseTarget(RuntimeTarget) = %#v, %v", parsed, err)
	}
}

func TestSelectTargetPrefersExactPlatformAndSelectAllIsStable(t *testing.T) {
	doc, err := Load(writeManifest(t, `schema: 1
resources:
  tailwind:
    version: "4.3.3"
    downloads:
      cli:
        path: .tools/tailwindcss
        executable: true
        max_size: 128MiB
        platforms:
          linux/amd64:
            url: https://example.com/v${version}/tailwindcss-linux-x64
          darwin/arm64:
            url: https://example.com/v${version}/tailwindcss-macos-arm64
`))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := doc.Validate(true); err != nil {
		t.Fatal(err)
	}

	selected, err := doc.SelectTarget([]string{"tailwind/cli"}, Target{GOOS: "linux", GOARCH: "amd64"})
	if err != nil {
		t.Fatal(err)
	}
	if len(selected) != 1 {
		t.Fatalf("SelectTarget count = %d", len(selected))
	}
	got := selected[0]
	if got.Variant != "linux/amd64" || got.Platform != "linux/amd64" || !got.Executable || got.MaxBytes != 128<<20 {
		t.Fatalf("selection = %#v", got)
	}
	if !strings.HasSuffix(got.URL, "/v4.3.3/tailwindcss-linux-x64") {
		t.Fatalf("URL = %q", got.URL)
	}

	all, err := doc.SelectAll([]string{"tailwind/cli"})
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 2 || all[0].Variant != "darwin/arm64" || all[1].Variant != "linux/amd64" {
		t.Fatalf("SelectAll = %#v", all)
	}
}

func TestSelectTargetUsesBaseFallbackAndExactOverride(t *testing.T) {
	doc, err := Load(writeManifest(t, `schema: 1
resources:
  tool:
    version: "1.2.3"
    downloads:
      cli:
        url: https://example.com/${version}/portable
        path: .tools/tool
        platforms:
          linux/amd64:
            url: https://example.com/${version}/linux-x64
`))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := doc.Validate(true); err != nil {
		t.Fatal(err)
	}

	linux, err := doc.SelectTarget(nil, Target{GOOS: "linux", GOARCH: "amd64"})
	if err != nil {
		t.Fatal(err)
	}
	if linux[0].Variant != "linux/amd64" || !strings.HasSuffix(linux[0].URL, "/linux-x64") {
		t.Fatalf("linux selection = %#v", linux[0])
	}

	darwin, err := doc.SelectTarget(nil, Target{GOOS: "darwin", GOARCH: "arm64"})
	if err != nil {
		t.Fatal(err)
	}
	if darwin[0].Variant != "" || darwin[0].Platform != "darwin/arm64" || !strings.HasSuffix(darwin[0].URL, "/portable") {
		t.Fatalf("darwin selection = %#v", darwin[0])
	}
}

func TestSelectTargetRejectsUnsupportedDownloadPlatform(t *testing.T) {
	doc, err := Load(writeManifest(t, `schema: 1
resources:
  tool:
    version: "1.2.3"
    downloads:
      cli:
        path: .tools/tool
        platforms:
          linux/amd64:
            url: https://example.com/${version}/linux-x64
`))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := doc.Validate(true); err != nil {
		t.Fatal(err)
	}
	_, err = doc.SelectTarget(nil, Target{GOOS: "windows", GOARCH: "amd64"})
	if err == nil || !strings.Contains(err.Error(), "tool/cli") || !strings.Contains(err.Error(), "windows/amd64") {
		t.Fatalf("SelectTarget error = %v", err)
	}
}

func TestLegacyDownloadDefaultsToMulti(t *testing.T) {
	doc, err := Load(writeManifest(t, validManifest))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := doc.Validate(true); err != nil {
		t.Fatal(err)
	}
	selected, err := doc.SelectTarget(nil, Target{GOOS: "windows", GOARCH: "amd64"})
	if err != nil {
		t.Fatal(err)
	}
	if selected[0].Platform != "windows/amd64" || selected[0].Executable || selected[0].MaxBytes != 0 {
		t.Fatalf("legacy selection = %#v", selected[0])
	}
}

func TestValidateRejectsInvalidPlatformDownloads(t *testing.T) {
	base := `schema: 1
resources:
  tool:
    version: "1.2.3"
    downloads:
      cli:
        path: .tools/tool
        platforms:
          linux/amd64:
            url: https://example.com/${version}/linux-x64
`
	tests := map[string]string{
		"missing acquisition": strings.Replace(base, "        platforms:\n          linux/amd64:\n            url: https://example.com/${version}/linux-x64\n", "", 1),
		"invalid platform":    strings.Replace(base, "linux/amd64", "macos/arm64", 1),
		"missing variant URL": strings.Replace(base, "            url: https://example.com/${version}/linux-x64\n", "", 1),
		"invalid max size":    strings.Replace(base, "        platforms:\n", "        max_size: 128MB\n        platforms:\n", 1),
	}
	for name, body := range tests {
		t.Run(name, func(t *testing.T) {
			doc, err := Load(writeManifest(t, body))
			if err == nil {
				_, err = doc.Validate(false)
			}
			if err == nil {
				t.Fatal("expected validation error")
			}
		})
	}
}

func TestValidateChecksVersionInEveryPlatformURL(t *testing.T) {
	doc, err := Load(writeManifest(t, `schema: 1
resources:
  tool:
    version: "1.2.3"
    downloads:
      cli:
        path: .tools/tool
        platforms:
          linux/amd64:
            url: https://example.com/latest/linux-x64
          darwin/arm64:
            url: https://example.com/latest/macos-arm64
`))
	if err != nil {
		t.Fatal(err)
	}
	warnings, err := doc.Validate(false)
	if err != nil {
		t.Fatal(err)
	}
	if len(warnings) != 2 {
		t.Fatalf("warnings = %#v", warnings)
	}
	if _, err := doc.Validate(true); err == nil || !strings.Contains(err.Error(), "does not contain version") {
		t.Fatalf("strict error = %v", err)
	}
}
