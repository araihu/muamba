package archive

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"os"
	"path"
	"sort"
	"strings"
)

type Options struct {
	StripComponents int
	Include         []string
	Exclude         []string
	MaxFiles        int
	MaxBytes        int64
}

type File struct {
	Path     string
	Size     int64
	Contents []byte
}

func ExtractTarGz(filename string, options Options) ([]File, error) {
	var files []File
	err := WalkTarGz(context.Background(), filename, options, func(file File) error {
		files = append(files, file)
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Slice(files, func(i, j int) bool { return files[i].Path < files[j].Path })
	return files, nil
}

// WalkTarGz validates all entries and visits selected files one at a time.
// Contents are owned by the callback; do not retain them for bounded memory.
func WalkTarGz(ctx context.Context, filename string, options Options, visit func(File) error) error {
	if visit == nil {
		return fmt.Errorf("archive visitor is nil")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := validateOptions(options); err != nil {
		return err
	}
	input, err := os.Open(filename)
	if err != nil {
		return err
	}
	defer func() { _ = input.Close() }()
	gz, err := gzip.NewReader(cancelReader{ctx, input})
	if err != nil {
		return fmt.Errorf("open tar.gz: %w", err)
	}
	defer func() { _ = gz.Close() }()
	reader := tar.NewReader(gz)
	seen := make(map[string]struct{})
	matched := 0
	var unpacked int64
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		header, nextErr := reader.Next()
		if nextErr == io.EOF {
			break
		}
		if nextErr != nil {
			return fmt.Errorf("read tar.gz: %w", nextErr)
		}
		file, entryErr := extractEntry(reader, header, options, &unpacked, seen, matched)
		if entryErr != nil {
			return entryErr
		}
		if file != nil {
			matched++
			if err := visit(*file); err != nil {
				return err
			}
		}
	}
	if matched == 0 {
		return fmt.Errorf("include globs matched no files")
	}
	return ctx.Err()
}

func validateOptions(options Options) error {
	if options.StripComponents < 0 {
		return fmt.Errorf("strip components must not be negative")
	}
	if len(options.Include) == 0 {
		return fmt.Errorf("at least one include glob is required")
	}
	if options.MaxFiles <= 0 || options.MaxBytes <= 0 {
		return fmt.Errorf("positive max files and unpacked bytes are required")
	}
	for _, pattern := range append(append([]string(nil), options.Include...), options.Exclude...) {
		if err := validatePattern(pattern); err != nil {
			return err
		}
	}
	return nil
}

func extractEntry(reader io.Reader, header *tar.Header, options Options, unpacked *int64, seen map[string]struct{}, matched int) (*File, error) {
	// PAX global and extended headers carry metadata only. archive/tar applies
	// extended records to the following file header before returning it.
	if header.Typeflag == tar.TypeXGlobalHeader || header.Typeflag == tar.TypeXHeader {
		return nil, nil
	}
	clean, err := safeArchivePath(header.Name)
	if err != nil {
		return nil, err
	}
	if header.Typeflag == tar.TypeDir {
		return nil, nil
	}
	if header.Typeflag != tar.TypeReg && header.Typeflag != 0 {
		return nil, fmt.Errorf("archive entry %q has unsafe type %d", header.Name, header.Typeflag)
	}
	if header.Size < 0 || *unpacked > options.MaxBytes-header.Size {
		return nil, fmt.Errorf("archive exceeds %d unpacked bytes", options.MaxBytes)
	}
	*unpacked += header.Size
	stripped, ok := stripPath(clean, options.StripComponents)
	if !ok || !matchesAny(options.Include, stripped) || matchesAny(options.Exclude, stripped) {
		return nil, nil
	}
	if _, duplicate := seen[stripped]; duplicate {
		return nil, fmt.Errorf("archive resolves duplicate path %q", stripped)
	}
	if matched >= options.MaxFiles {
		return nil, fmt.Errorf("archive exceeds %d matched files", options.MaxFiles)
	}
	contents, err := io.ReadAll(reader)
	if err != nil {
		return nil, fmt.Errorf("read archive entry %q: %w", header.Name, err)
	}
	if int64(len(contents)) != header.Size {
		return nil, fmt.Errorf("archive entry %q size = %d, want %d", header.Name, len(contents), header.Size)
	}
	seen[stripped] = struct{}{}
	return &File{Path: stripped, Size: header.Size, Contents: contents}, nil
}

func safeArchivePath(raw string) (string, error) {
	if raw == "" || strings.Contains(raw, "\\") || strings.HasPrefix(raw, "/") {
		return "", fmt.Errorf("archive entry has unsafe path %q", raw)
	}
	clean := path.Clean(raw)
	if clean == "." || clean == ".." || strings.HasPrefix(clean, "../") {
		return "", fmt.Errorf("archive entry has unsafe path %q", raw)
	}
	return clean, nil
}

func stripPath(value string, count int) (string, bool) {
	parts := strings.Split(value, "/")
	if len(parts) <= count {
		return "", false
	}
	return strings.Join(parts[count:], "/"), true
}

func validatePattern(pattern string) error {
	if pattern == "" || strings.HasPrefix(pattern, "/") || strings.Contains(pattern, "\\") {
		return fmt.Errorf("invalid archive glob %q", pattern)
	}
	for _, segment := range strings.Split(pattern, "/") {
		if segment == "**" {
			continue
		}
		if _, err := path.Match(segment, "probe"); err != nil {
			return fmt.Errorf("invalid archive glob %q: %w", pattern, err)
		}
	}
	return nil
}

func matchesAny(patterns []string, value string) bool {
	for _, pattern := range patterns {
		if matchSegments(strings.Split(pattern, "/"), strings.Split(value, "/")) {
			return true
		}
	}
	return false
}

func Match(patterns []string, value string) (bool, error) {
	for _, pattern := range patterns {
		if err := validatePattern(pattern); err != nil {
			return false, err
		}
	}
	return matchesAny(patterns, value), nil
}

func matchSegments(pattern, value []string) bool {
	if len(pattern) == 0 {
		return len(value) == 0
	}
	if pattern[0] == "**" {
		return matchSegments(pattern[1:], value) || len(value) > 0 && matchSegments(pattern, value[1:])
	}
	if len(value) == 0 {
		return false
	}
	matched, err := path.Match(pattern[0], value[0])
	return err == nil && matched && matchSegments(pattern[1:], value[1:])
}

// cancelReader also covers skipped tar entries, which tar.Reader drains internally.
type cancelReader struct {
	ctx    context.Context
	reader io.Reader
}

func (r cancelReader) Read(p []byte) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	return r.reader.Read(p)
}
