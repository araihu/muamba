package archive

import (
	"archive/tar"
	"compress/gzip"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type tarEntry struct {
	name     string
	body     string
	typeflag byte
	linkname string
	pax      map[string]string
}

func TestExtractTarGzAppliesStripIncludeExcludeAndSort(t *testing.T) {
	path := writeTarGz(t, []tarEntry{
		{name: "release/icons/z.svg", body: "z"},
		{name: "release/icons/nested/a.svg", body: "a"},
		{name: "release/icons/ignore.svg", body: "ignore"},
		{name: "release/README.md", body: "readme"},
	})
	files, err := ExtractTarGz(path, Options{
		StripComponents: 1,
		Include:         []string{"icons/**/*.svg"},
		Exclude:         []string{"**/ignore.svg"},
		MaxFiles:        10,
		MaxBytes:        1024,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 2 || files[0].Path != "icons/nested/a.svg" || files[1].Path != "icons/z.svg" {
		t.Fatalf("files = %#v", files)
	}
	if string(files[0].Contents) != "a" || files[0].Size != 1 {
		t.Fatalf("first file = %#v", files[0])
	}
}

func TestExtractTarGzRejectsUnsafeEntriesEvenWhenGlobWouldIgnoreThem(t *testing.T) {
	tests := map[string][]tarEntry{
		"traversal": {
			{name: "release/icons/ok.svg", body: "ok"},
			{name: "../escape", body: "bad"},
		},
		"symlink": {
			{name: "release/icons/ok.svg", body: "ok"},
			{name: "release/link", typeflag: tar.TypeSymlink, linkname: "icons/ok.svg"},
		},
		"hardlink": {
			{name: "release/icons/ok.svg", body: "ok"},
			{name: "release/hardlink", typeflag: tar.TypeLink, linkname: "release/icons/ok.svg"},
		},
		"special-type": {
			{name: "release/icons/ok.svg", body: "ok"},
			{name: "release/device", typeflag: tar.TypeChar},
		},
		"duplicate": {
			{name: "release/icons/ok.svg", body: "one"},
			{name: "release/icons/ok.svg", body: "two"},
		},
	}
	for name, entries := range tests {
		t.Run(name, func(t *testing.T) {
			path := writeTarGz(t, entries)
			_, err := ExtractTarGz(path, Options{StripComponents: 1, Include: []string{"icons/**/*.svg"}, MaxFiles: 10, MaxBytes: 1024})
			if err == nil {
				t.Fatal("unsafe archive accepted")
			}
			if name == "traversal" && !strings.Contains(err.Error(), "unsafe path") {
				t.Fatalf("error = %v", err)
			}
		})
	}
}

func TestExtractTarGzIgnoresPAXMetadataHeaders(t *testing.T) {
	path := writeTarGz(t, []tarEntry{
		{name: "pax_global_header", typeflag: tar.TypeXGlobalHeader, pax: map[string]string{"comment": "GitHub archive metadata"}},
		{name: "release/icons/a.svg", body: "a", pax: map[string]string{"mtime": "1700000000.0"}},
	})
	files, err := ExtractTarGz(path, Options{
		StripComponents: 1,
		Include:         []string{"icons/**/*.svg"},
		MaxFiles:        10,
		MaxBytes:        1024,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 1 || files[0].Path != "icons/a.svg" || string(files[0].Contents) != "a" {
		t.Fatalf("files = %#v", files)
	}
}

func TestExtractTarGzEnforcesBoundsAndRequiresMatch(t *testing.T) {
	path := writeTarGz(t, []tarEntry{{name: "release/icons/a.svg", body: "12345"}})
	if _, err := ExtractTarGz(path, Options{StripComponents: 1, Include: []string{"icons/**/*.svg"}, MaxFiles: 1, MaxBytes: 4}); err == nil || !strings.Contains(err.Error(), "unpacked bytes") {
		t.Fatalf("byte bound error = %v", err)
	}
	if _, err := ExtractTarGz(path, Options{StripComponents: 1, Include: []string{"missing/**"}, MaxFiles: 1, MaxBytes: 100}); err == nil || !strings.Contains(err.Error(), "matched no files") {
		t.Fatalf("no-match error = %v", err)
	}
}

func TestArchiveOptionsAndMatchFailures(t *testing.T) {
	if matched, err := Match([]string{"icons/**/*.svg"}, "icons/a.svg"); err != nil || !matched {
		t.Fatalf("Match() = %v, %v", matched, err)
	}
	if matched, err := Match([]string{"icons/**/*.svg"}, "README.md"); err != nil || matched {
		t.Fatalf("Match(nonmatch) = %v, %v", matched, err)
	}
	if _, err := Match([]string{"["}, "file"); err == nil {
		t.Fatal("invalid glob accepted")
	}
	valid := writeTarGz(t, []tarEntry{{name: "root/a.svg", body: "a"}, {name: "root/b.svg", body: "b"}})
	tests := []Options{
		{StripComponents: -1, Include: []string{"**"}, MaxFiles: 1, MaxBytes: 1},
		{Include: nil, MaxFiles: 1, MaxBytes: 1},
		{Include: []string{"**"}, MaxFiles: 0, MaxBytes: 1},
		{Include: []string{"["}, MaxFiles: 1, MaxBytes: 1},
		{StripComponents: 1, Include: []string{"*.svg"}, MaxFiles: 1, MaxBytes: 10},
	}
	for index, options := range tests {
		if _, err := ExtractTarGz(valid, options); err == nil {
			t.Fatalf("invalid options %d accepted", index)
		}
	}
	invalid := filepath.Join(t.TempDir(), "invalid.tar.gz")
	if err := os.WriteFile(invalid, []byte("not gzip"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := ExtractTarGz(invalid, Options{Include: []string{"**"}, MaxFiles: 1, MaxBytes: 1}); err == nil {
		t.Fatal("invalid gzip accepted")
	}
}

func writeTarGz(t *testing.T, entries []tarEntry) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "source.tar.gz")
	file, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	gz := gzip.NewWriter(file)
	tw := tar.NewWriter(gz)
	for _, entry := range entries {
		typeflag := entry.typeflag
		if typeflag == 0 {
			typeflag = tar.TypeReg
		}
		header := &tar.Header{Name: entry.name, Typeflag: typeflag, Linkname: entry.linkname, Mode: 0o644, Size: int64(len(entry.body)), PAXRecords: entry.pax}
		if typeflag == tar.TypeXGlobalHeader {
			header = &tar.Header{Typeflag: typeflag, PAXRecords: entry.pax}
		}
		if len(entry.pax) != 0 {
			header.Format = tar.FormatPAX
		}
		if err := tw.WriteHeader(header); err != nil {
			t.Fatal(err)
		}
		if entry.body != "" {
			if _, err := tw.Write([]byte(entry.body)); err != nil {
				t.Fatal(err)
			}
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	return path
}
