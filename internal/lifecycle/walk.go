package lifecycle

import (
	"context"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"

	"github.com/araihu/muamba/internal/integrity"
	"github.com/araihu/muamba/internal/manifest"
)

// File describes one verified source without retaining its contents.
type File struct {
	ID, Source, Path, Integrity string
	Size                        int64
}

// Walk holds the mutation lock while synchronizing and visiting files in path
// order. Each reader is a private disk snapshot of the bytes verified, valid
// only during its callback. Callback failure stops iteration and cleans staging.
func (e *Engine) Walk(ctx context.Context, selectors []string, visit func(File, io.Reader) error) error {
	if visit == nil {
		return fmt.Errorf("source visitor is nil")
	}
	return e.withMutationLock(ctx, func() error {
		var report Report
		if err := e.syncLocked(ctx, selectors, &report); err != nil {
			return err
		}
		selections, err := e.selections(selectors)
		if err != nil {
			return err
		}
		directories, err := e.directorySelections(selectors)
		if err != nil {
			return err
		}
		for _, directory := range directories {
			if directory.Lock == nil {
				return fmt.Errorf("%s is unlocked", directory.ID())
			}
			selections = append(selections, directoryFileSelections(directory)...)
		}
		sort.Slice(selections, func(i, j int) bool { return selections[i].Path < selections[j].Path })
		staging, err := os.MkdirTemp("", "muamba-walk-*")
		if err != nil {
			return err
		}
		defer func() { _ = os.RemoveAll(staging) }()
		for _, selection := range selections {
			if err := ctx.Err(); err != nil {
				return err
			}
			if err := e.visitFile(ctx, staging, selection, visit); err != nil {
				return fmt.Errorf("%s: %w", selectionLabel(selection), err)
			}
		}
		return ctx.Err()
	})
}

func (e *Engine) visitFile(ctx context.Context, staging string, selection manifest.Selection, visit func(File, io.Reader) error) error {
	digest, err := integrity.Parse(selection.Integrity)
	if err != nil {
		return err
	}
	target, err := e.target(selection)
	if err != nil {
		return err
	}
	source, err := os.Open(target)
	if err != nil {
		return err
	}
	defer func() { _ = source.Close() }()
	info, err := source.Stat()
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("source is not a regular file")
	}
	snapshot, err := os.CreateTemp(staging, "file-*")
	if err != nil {
		return err
	}
	defer func() { _ = os.Remove(snapshot.Name()) }()
	defer func() { _ = snapshot.Close() }()
	// Verify exactly the stream written to private staging, never reopen the
	// materialized path after verification (including same-inode rewrites).
	reader := io.Reader(contextReader{ctx, source})
	if selection.Size >= 0 {
		reader = io.LimitReader(reader, selection.Size+1)
	}
	if _, err := integrity.Verify(io.TeeReader(reader, snapshot), digest); err != nil {
		return err
	}
	size, err := snapshot.Seek(0, io.SeekCurrent)
	if err != nil {
		return err
	}
	if selection.Size >= 0 && size != selection.Size {
		return fmt.Errorf("size = %d, want %d", size, selection.Size)
	}
	if _, err := snapshot.Seek(0, io.SeekStart); err != nil {
		return err
	}
	name := selection.DownloadName
	if index := strings.IndexByte(name, ':'); index >= 0 {
		name = name[index+1:]
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	return visit(File{ID: selectionLabel(selection), Source: name, Path: selection.Path, Integrity: selection.Integrity, Size: size}, contextReader{ctx, snapshot})
}

type contextReader struct {
	ctx    context.Context
	reader io.Reader
}

func (r contextReader) Read(p []byte) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	return r.reader.Read(p)
}
