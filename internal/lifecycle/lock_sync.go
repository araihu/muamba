package lifecycle

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

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
	report := Report{}
	err := e.withMutationLock(ctx, func() error {
		report.Warnings = append([]manifest.Warning(nil), e.warnings...)
		selections, selectErr := e.allSelections(selectors)
		if selectErr != nil {
			return selectErr
		}
		client, clientErr := transport.New(e.options.Transport)
		if clientErr != nil {
			return clientErr
		}
		candidate, cloneErr := e.document.Clone()
		if cloneErr != nil {
			return cloneErr
		}
		acquired := make(map[string]struct{})
		var changed []string
		for _, selection := range selections {
			if selection.Integrity != "" {
				continue
			}
			downloaded, downloadErr := e.download(ctx, client, selection, nil)
			if downloadErr != nil {
				return downloadErr
			}
			if seedErr := e.cache.Seed(downloaded.path, downloaded.digest); seedErr != nil {
				_ = os.Remove(downloaded.path)
				return seedErr
			}
			_ = os.Remove(downloaded.path)
			if setErr := candidate.SetIntegrity(selection, downloaded.integrity); setErr != nil {
				return setErr
			}
			acquired[selectionLabel(selection)] = struct{}{}
			changed = append(changed, selectionLabel(selection))
		}
		if len(acquired) == 0 {
			return nil
		}
		warnings, validateErr := candidate.Validate(e.options.Strict)
		if validateErr != nil {
			return validateErr
		}
		selected, selectErr := candidate.SelectTarget(selectors, e.targetOS)
		if selectErr != nil {
			return selectErr
		}
		var staged []stagedDownload
		defer func() { cleanupStaged(staged, e.document.Dir) }()
		for _, selection := range selected {
			if _, ok := acquired[selectionLabel(selection)]; !ok {
				continue
			}
			item, stageErr := e.stageCached(selection)
			if stageErr != nil {
				return stageErr
			}
			staged = append(staged, item)
		}
		manifestBytes, marshalErr := candidate.Marshal()
		if marshalErr != nil {
			return marshalErr
		}
		if commitErr := commitStaged(staged, candidate.Path, manifestBytes); commitErr != nil {
			return commitErr
		}
		report.Changed = append(report.Changed, changed...)
		e.document, e.warnings = candidate, warnings
		return nil
	})
	return sortedReport(report), err
}

func (e *Engine) Sync(ctx context.Context, selectors []string) (Report, error) {
	report := Report{}
	err := e.withMutationLock(ctx, func() error {
		report.Warnings = append([]manifest.Warning(nil), e.warnings...)
		selections, selectErr := e.selections(selectors)
		if selectErr != nil {
			return selectErr
		}
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
