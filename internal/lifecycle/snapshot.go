package lifecycle

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/araihu/muamba/internal/integrity"
)

// SnapshotFile is a verified in-memory copy of one materialized source file.
// Callers should consume Contents instead of reopening Path: the bytes are
// the same bytes that Muamba verified while holding its mutation lock.
type SnapshotFile struct {
	ID        string
	Source    string
	Path      string
	Size      int64
	Integrity string
	Contents  []byte
}

// Snapshot synchronizes locked inputs and returns their verified bytes while
// the Muamba mutation lock is held. Returning memory-backed contents closes
// the verify-then-reopen gap for library consumers.
func (e *Engine) Snapshot(ctx context.Context, selectors []string) ([]SnapshotFile, error) {
	var snapshot []SnapshotFile
	err := e.withMutationLock(ctx, func() error {
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
		for _, selection := range selections {
			if selection.Integrity == "" {
				return fmt.Errorf("%s is unlocked", selectionLabel(selection))
			}
			digest, err := integrity.Parse(selection.Integrity)
			if err != nil {
				return fmt.Errorf("%s: %w", selectionLabel(selection), err)
			}
			target, err := e.target(selection)
			if err != nil {
				return fmt.Errorf("%s: %w", selectionLabel(selection), err)
			}
			contents, err := os.ReadFile(target)
			if err != nil {
				return fmt.Errorf("%s at %s: %w", selectionLabel(selection), target, err)
			}
			if selection.Size >= 0 && int64(len(contents)) != selection.Size {
				return fmt.Errorf("%s at %s: size = %d, want %d", selectionLabel(selection), target, len(contents), selection.Size)
			}
			if _, err := integrity.Verify(bytes.NewReader(contents), digest); err != nil {
				return fmt.Errorf("%s at %s: %w", selectionLabel(selection), target, err)
			}
			source := selection.DownloadName
			if index := strings.IndexByte(source, ':'); index >= 0 {
				source = source[index+1:]
			}
			snapshot = append(snapshot, SnapshotFile{
				ID: selectionLabel(selection), Source: source, Path: selection.Path,
				Size: int64(len(contents)), Integrity: selection.Integrity,
				Contents: append([]byte(nil), contents...),
			})
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Slice(snapshot, func(i, j int) bool { return snapshot[i].Path < snapshot[j].Path })
	return snapshot, nil
}
