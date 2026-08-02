package lifecycle

import (
	"context"
	"crypto"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/araihu/muamba/internal/integrity"
	"github.com/araihu/muamba/internal/manifest"
	"github.com/araihu/muamba/internal/transport"
)

type stagedDownload struct {
	selection manifest.Selection
	target    string
	temporary string
	integrity string
}

func (e *Engine) Lock(ctx context.Context, selectors []string) (Report, error) {
	selections, err := e.selections(selectors)
	if err != nil {
		return Report{}, err
	}
	report := Report{Warnings: append([]manifest.Warning(nil), e.warnings...)}
	err = e.withMutationLock(ctx, func() error {
		client, clientErr := transport.New(e.options.Transport)
		if clientErr != nil {
			return clientErr
		}
		var staged []stagedDownload
		defer func() { removeStaged(staged) }()
		for _, selection := range selections {
			if selection.Integrity != "" {
				continue
			}
			item, stageErr := e.stage(ctx, client, selection)
			if stageErr != nil {
				return stageErr
			}
			staged = append(staged, item)
		}
		for _, item := range staged {
			if setErr := e.document.SetIntegrity(item.selection, item.integrity); setErr != nil {
				return setErr
			}
		}
		manifestBytes, marshalErr := e.document.Marshal()
		if marshalErr != nil {
			return marshalErr
		}
		for _, item := range staged {
			if renameErr := os.Rename(item.temporary, item.target); renameErr != nil {
				return renameErr
			}
			report.Changed = append(report.Changed, selectionLabel(item.selection))
		}
		return writeAtomic(e.document.Path, manifestBytes, 0o644)
	})
	return sortedReport(report), err
}

func (e *Engine) Sync(ctx context.Context, selectors []string) (Report, error) {
	selections, err := e.selections(selectors)
	if err != nil {
		return Report{}, err
	}
	report := Report{Warnings: append([]manifest.Warning(nil), e.warnings...)}
	err = e.withMutationLock(ctx, func() error {
		client, clientErr := transport.New(e.options.Transport)
		if clientErr != nil {
			return clientErr
		}
		for _, selection := range selections {
			if selection.Integrity == "" {
				return fmt.Errorf("%s/%s is unlocked", selection.ResourceName, selection.DownloadName)
			}
			changed, restoreErr := e.restoreLocked(ctx, client, selection)
			if restoreErr != nil {
				return restoreErr
			}
			if changed {
				report.Changed = append(report.Changed, selectionLabel(selection))
			} else {
				report.Verified = append(report.Verified, selectionLabel(selection))
			}
		}
		return nil
	})
	return sortedReport(report), err
}

func (e *Engine) stage(ctx context.Context, client *transport.Client, selection manifest.Selection) (stagedDownload, error) {
	target, err := e.target(selection)
	if err != nil {
		return stagedDownload{}, err
	}
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return stagedDownload{}, err
	}
	temporary, err := os.CreateTemp(filepath.Dir(target), ".muamba-download-*")
	if err != nil {
		return stagedDownload{}, err
	}
	temporaryPath := temporary.Name()
	ok := false
	defer func() {
		_ = temporary.Close()
		if !ok {
			_ = os.Remove(temporaryPath)
			removeEmptyParents(filepath.Dir(target), e.document.Dir)
		}
	}()
	if _, err := client.Fetch(ctx, selection.URL, temporary, e.effectiveMaxBytes(selection)); err != nil {
		return stagedDownload{}, fmt.Errorf("%s/%s: %w", selection.ResourceName, selection.DownloadName, err)
	}
	if _, err := temporary.Seek(0, io.SeekStart); err != nil {
		return stagedDownload{}, err
	}
	sum, err := integrity.Compute(temporary, crypto.SHA384)
	if err != nil {
		return stagedDownload{}, err
	}
	if err := temporary.Chmod(selectionMode(selection)); err != nil {
		return stagedDownload{}, err
	}
	if err := temporary.Sync(); err != nil {
		return stagedDownload{}, err
	}
	if err := temporary.Close(); err != nil {
		return stagedDownload{}, err
	}
	digest := integrity.Digest{Algorithm: crypto.SHA384, Sum: sum}
	if err := e.cache.Seed(temporaryPath, digest); err != nil {
		return stagedDownload{}, err
	}
	ok = true
	return stagedDownload{selection: selection, target: target, temporary: temporaryPath, integrity: integrity.FormatSRI(crypto.SHA384, sum)}, nil
}

func removeStaged(items []stagedDownload) {
	for _, item := range items {
		_ = os.Remove(item.temporary)
	}
}

func writeAtomic(path string, contents []byte, mode os.FileMode) error {
	temporary, err := os.CreateTemp(filepath.Dir(path), ".muamba-manifest-*")
	if err != nil {
		return err
	}
	temporaryPath := temporary.Name()
	defer func() { _ = os.Remove(temporaryPath) }()
	if _, err := temporary.Write(contents); err != nil {
		_ = temporary.Close()
		return err
	}
	if err := temporary.Chmod(mode); err != nil {
		_ = temporary.Close()
		return err
	}
	if err := temporary.Sync(); err != nil {
		_ = temporary.Close()
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	return os.Rename(temporaryPath, path)
}
