package manifest

import (
	"strings"
	"testing"
)

const validManifest = `schema: 1
resources:
  alpine:
    version: "3.14.9"
    downloads:
      core-js:
        url: https://cdn.example/alpine@${version}/alpine.js
        path: assets/alpine/${version}/alpine.js
`

func TestValidateExpandsVersion(t *testing.T) {
	doc, err := Load(writeManifest(t, validManifest))
	if err != nil {
		t.Fatal(err)
	}
	warnings, err := doc.Validate(true)
	if err != nil {
		t.Fatal(err)
	}
	if len(warnings) != 0 {
		t.Fatalf("warnings = %#v", warnings)
	}
	selections, err := doc.Select([]string{"alpine/core-js"})
	if err != nil {
		t.Fatal(err)
	}
	got := selections[0]
	if got.URL != "https://cdn.example/alpine@3.14.9/alpine.js" || got.Path != "assets/alpine/3.14.9/alpine.js" {
		t.Fatalf("selection = %#v", got)
	}
}

func TestValidateWarnsWhenURLDoesNotContainVersion(t *testing.T) {
	body := strings.Replace(validManifest, "https://cdn.example/alpine@${version}/alpine.js", "https://cdn.example/alpine/latest.js", 1)
	doc, err := Load(writeManifest(t, body))
	if err != nil {
		t.Fatal(err)
	}
	warnings, err := doc.Validate(false)
	if err != nil {
		t.Fatal(err)
	}
	if len(warnings) != 1 || warnings[0].Resource != "alpine" || warnings[0].Download != "core-js" {
		t.Fatalf("warnings = %#v", warnings)
	}
	if _, err := doc.Validate(true); err == nil || !strings.Contains(err.Error(), "does not contain version") {
		t.Fatalf("strict error = %v", err)
	}
}

func TestValidateRejectsInvalidManifests(t *testing.T) {
	tests := map[string]string{
		"schema":          strings.Replace(validManifest, "schema: 1", "schema: 2", 1),
		"resource name":   strings.Replace(validManifest, "alpine:", "Alpine:", 1),
		"download name":   strings.Replace(validManifest, "core-js:", "core_js:", 1),
		"missing version": strings.Replace(validManifest, "    version: \"3.14.9\"\n", "", 1),
		"unknown token":   strings.Replace(validManifest, "${version}", "${release}", 1),
		"absolute path":   strings.Replace(validManifest, "assets/alpine/${version}/alpine.js", "/tmp/alpine.js", 1),
		"traversal":       strings.Replace(validManifest, "assets/alpine/${version}/alpine.js", "../alpine.js", 1),
	}
	for name, body := range tests {
		t.Run(name, func(t *testing.T) {
			doc, err := Load(writeManifest(t, body))
			if err == nil {
				_, err = doc.Validate(false)
			}
			if err == nil {
				t.Fatal("expected error")
			}
		})
	}
}

func TestValidateRejectsDuplicateResolvedPaths(t *testing.T) {
	body := validManifest + `  htmx:
    version: "2.0.8"
    downloads:
      core-js:
        url: https://cdn.example/htmx@${version}/htmx.js
        path: assets/alpine/3.14.9/alpine.js
`
	doc, err := Load(writeManifest(t, body))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := doc.Validate(false); err == nil || !strings.Contains(err.Error(), "duplicate") {
		t.Fatalf("Validate error = %v", err)
	}
}
