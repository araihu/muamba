package lifecycle

import (
	"context"
	"crypto"
	"os"
	"strings"
	"testing"

	"github.com/araihu/muamba/internal/integrity"
	"github.com/araihu/muamba/internal/testrepo"
)

func sri(t *testing.T, contents string) string {
	t.Helper()
	sum, err := integrity.Compute(strings.NewReader(contents), crypto.SHA384)
	if err != nil {
		t.Fatal(err)
	}
	return integrity.FormatSRI(crypto.SHA384, sum)
}

func lockedManifest(url, digest string) string {
	return `schema: 1
resources:
  library:
    version: "1.0.0"
    downloads:
      script:
        url: ` + url + `
        path: vendor/library.js
        integrity: ` + digest + "\n"
}

func TestVerifyIsOfflineAndReadOnly(t *testing.T) {
	repo := testrepo.New(t, lockedManifest("https://127.0.0.1:1/library-1.0.0.js", sri(t, "locked")))
	repo.Write(t, "vendor/library.js", []byte("locked"))
	before, _ := os.ReadFile(repo.Manifest)
	engine, err := New(repo.Manifest, Options{Strict: true})
	if err != nil {
		t.Fatal(err)
	}
	report, err := engine.Verify(context.Background(), nil, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Verified) != 1 || report.Verified[0] != "library/script" {
		t.Fatalf("report = %#v", report)
	}
	after, _ := os.ReadFile(repo.Manifest)
	if string(after) != string(before) {
		t.Fatal("verify changed manifest")
	}
}

func TestVerifyRejectsMissingMismatchedAndUnlocked(t *testing.T) {
	tests := map[string]struct {
		integrity string
		file      *string
	}{
		"missing":    {integrity: sri(t, "locked")},
		"mismatched": {integrity: sri(t, "locked"), file: ptr("wrong")},
		"unlocked":   {file: ptr("bytes")},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			repo := testrepo.New(t, lockedManifest("https://example.com/library-1.0.0.js", test.integrity))
			if test.file != nil {
				repo.Write(t, "vendor/library.js", []byte(*test.file))
			}
			engine, err := New(repo.Manifest, Options{Strict: true})
			if err != nil {
				t.Fatal(err)
			}
			if _, err := engine.Verify(context.Background(), nil, false); err == nil {
				t.Fatal("expected verify failure")
			}
		})
	}
}

func ptr(value string) *string { return &value }
