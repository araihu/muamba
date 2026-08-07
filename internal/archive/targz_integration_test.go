//go:build integration

package archive

import (
	"context"
	"crypto/sha256"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestExtractPinnedGitHubArchives(t *testing.T) {
	tests := []struct {
		name     string
		url      string
		sha256   string
		include  []string
		expected int
	}{
		{
			name:     "heroicons-v2.2.0",
			url:      "https://github.com/tailwindlabs/heroicons/archive/0435d4ca364a608cc75e2f8683d374e55abbae26.tar.gz",
			sha256:   "0b66712da6b739a7c0c2ad6534e1351fb36659c3153b8d1995dd0957589eaf4b",
			include:  []string{"optimized/**/*.svg"},
			expected: 1288,
		},
		{
			name:     "developer-icons-v7.0.1",
			url:      "https://github.com/xandemon/developer-icons/archive/28b895aba6a4984b8c43714336fafa5fa832f08f.tar.gz",
			sha256:   "429f7504ad95ce092bb2bff23cef33124a4f6c1abc2144dd7cd1e2b9ae3d46b3",
			include:  []string{"icons/*.svg"},
			expected: 319,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			archive := downloadPinnedArchive(t, test.url, test.sha256)
			files, err := ExtractTarGz(archive, Options{
				StripComponents: 1,
				Include:         test.include,
				MaxFiles:        20_000,
				MaxBytes:        512 << 20,
			})
			if err != nil {
				t.Fatal(err)
			}
			if len(files) != test.expected {
				t.Fatalf("matched files = %d, want %d", len(files), test.expected)
			}
			t.Logf("matched files = %d", len(files))
		})
	}
}

func downloadPinnedArchive(t *testing.T, archiveURL, expectedSHA256 string) string {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, archiveURL, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !allowedGitHubArchiveURL(request.URL) {
		t.Fatalf("unsafe archive URL %q", request.URL)
	}
	client := &http.Client{CheckRedirect: func(request *http.Request, via []*http.Request) error {
		if len(via) >= 10 {
			return fmt.Errorf("too many redirects")
		}
		if !allowedGitHubArchiveURL(request.URL) {
			return fmt.Errorf("unsafe archive redirect to %q", request.URL)
		}
		return nil
	}}
	response, err := client.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("GET %s: status %d", archiveURL, response.StatusCode)
	}
	const maxArchiveBytes = int64(64 << 20)
	contents, err := io.ReadAll(io.LimitReader(response.Body, maxArchiveBytes+1))
	if err != nil {
		t.Fatal(err)
	}
	if int64(len(contents)) > maxArchiveBytes {
		t.Fatalf("archive exceeds %d bytes", maxArchiveBytes)
	}
	digest := fmt.Sprintf("%x", sha256.Sum256(contents))
	if digest != expectedSHA256 {
		t.Fatalf("archive sha256 = %s, want %s", digest, expectedSHA256)
	}
	path := filepath.Join(t.TempDir(), "source.tar.gz")
	if err := os.WriteFile(path, contents, 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func allowedGitHubArchiveURL(value *url.URL) bool {
	return value.Scheme == "https" && (value.Hostname() == "github.com" || value.Hostname() == "codeload.github.com")
}
