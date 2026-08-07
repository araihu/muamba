package lifecycle

import (
	"context"
	"fmt"
	"os"

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
		if err := e.lockDownloads(ctx, client, candidate, selections, acquired, &changed); err != nil {
			return err
		}
		directories, directoryErr := e.directorySelections(selectors)
		if directoryErr != nil {
			return directoryErr
		}
		directoryStaged, directoryErr := e.lockDirectories(ctx, client, candidate, directories, acquired, &changed)
		if directoryErr != nil {
			return directoryErr
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
		staged, stageErr := e.stageAcquired(selected, directoryStaged, acquired)
		if stageErr != nil {
			return stageErr
		}
		defer func() { cleanupStaged(staged, e.document.Dir) }()
		metadata, marshalErr := metadataWrites(candidate, false)
		if marshalErr != nil {
			return marshalErr
		}
		if commitErr := commitStaged(staged, metadata); commitErr != nil {
			return commitErr
		}
		report.Changed = append(report.Changed, changed...)
		e.document, e.warnings = candidate, warnings
		return nil
	})
	return sortedReport(report), err
}

func (e *Engine) lockDownloads(ctx context.Context, client *transport.Client, candidate *manifest.Document, selections []manifest.Selection, acquired map[string]struct{}, changed *[]string) error {
	for _, selection := range selections {
		if selection.Integrity != "" {
			continue
		}
		downloaded, err := e.download(ctx, client, selection, nil)
		if err != nil {
			return err
		}
		if err := e.cache.Seed(downloaded.path, downloaded.digest); err != nil {
			_ = os.Remove(downloaded.path)
			return err
		}
		_ = os.Remove(downloaded.path)
		selection.Size = downloaded.size
		if candidate.IsLegacy() {
			err = candidate.SetIntegrity(selection, downloaded.integrity)
		} else {
			err = candidate.SetLock(selection, downloaded.size, downloaded.integrity)
		}
		if err != nil {
			return err
		}
		label := selectionLabel(selection)
		acquired[label] = struct{}{}
		*changed = append(*changed, label)
	}
	return nil
}

func (e *Engine) lockDirectories(ctx context.Context, client *transport.Client, candidate *manifest.Document, directories []manifest.DirectorySelection, acquired map[string]struct{}, changed *[]string) ([]manifest.Selection, error) {
	var staged []manifest.Selection
	for _, directory := range directories {
		if directory.Lock != nil {
			continue
		}
		locked, files, err := e.acquireDirectory(ctx, client, directory)
		if err != nil {
			return nil, err
		}
		if err := candidate.SetDirectoryLock(locked); err != nil {
			return nil, err
		}
		staged = append(staged, files...)
		for _, selection := range files {
			label := selectionLabel(selection)
			acquired[label] = struct{}{}
			*changed = append(*changed, label)
		}
	}
	return staged, nil
}

func (e *Engine) stageAcquired(downloads, directories []manifest.Selection, acquired map[string]struct{}) ([]stagedDownload, error) {
	selected := make([]manifest.Selection, 0, len(downloads)+len(directories))
	for _, selection := range downloads {
		if _, ok := acquired[selectionLabel(selection)]; ok {
			selected = append(selected, selection)
		}
	}
	selected = append(selected, directories...)
	return e.stageSelections(selected)
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
		directories, directoryErr := e.directorySelections(selectors)
		if directoryErr != nil {
			return directoryErr
		}
		for _, directory := range directories {
			changed, verified, syncErr := e.syncDirectory(ctx, client, directory)
			if syncErr != nil {
				return syncErr
			}
			report.Changed = append(report.Changed, changed...)
			report.Verified = append(report.Verified, verified...)
		}
		return nil
	})
	return sortedReport(report), err
}
