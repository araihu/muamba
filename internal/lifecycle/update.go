package lifecycle

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/araihu/muamba/internal/manifest"
	"github.com/araihu/muamba/internal/transport"
)

func (e *Engine) UpdateResource(ctx context.Context, resource, version string) (Report, error) {
	if version == "" {
		return Report{}, fmt.Errorf("version must not be empty")
	}
	report := Report{}
	err := e.withMutationLock(ctx, func() error {
		report.Warnings = append([]manifest.Warning(nil), e.warnings...)
		oldSelections, selectErr := e.selections([]string{resource})
		if selectErr != nil {
			return selectErr
		}
		for _, selection := range oldSelections {
			if err := e.verifyFile(selection); err != nil {
				return fmt.Errorf("preflight old %s: %w", selectionLabel(selection), err)
			}
		}
		candidate, cloneErr := e.document.Clone()
		if cloneErr != nil {
			return cloneErr
		}
		if err := candidate.SetVersion(resource, version); err != nil {
			return err
		}
		_, validateErr := candidate.Validate(e.options.Strict)
		if validateErr != nil {
			return validateErr
		}
		allSelections, selectErr := candidate.SelectAll([]string{resource})
		if selectErr != nil {
			return selectErr
		}
		client, clientErr := transport.New(e.options.Transport)
		if clientErr != nil {
			return clientErr
		}
		trusted, trustErr := e.retrustSelections(ctx, client, allSelections)
		if trustErr != nil {
			return trustErr
		}
		var changed []string
		for _, selection := range trusted {
			if err := candidate.SetIntegrity(selection, selection.Integrity); err != nil {
				return err
			}
			changed = append(changed, selectionLabel(selection))
		}
		warnings, validateErr := candidate.Validate(e.options.Strict)
		if validateErr != nil {
			return validateErr
		}
		selected, selectErr := candidate.SelectTarget([]string{resource}, e.targetOS)
		if selectErr != nil {
			return selectErr
		}
		staged, stageErr := e.stageSelections(selected)
		if stageErr != nil {
			return stageErr
		}
		defer cleanupStaged(staged, e.document.Dir)
		manifestBytes, marshalErr := candidate.Marshal()
		if marshalErr != nil {
			return marshalErr
		}
		if err := commitStaged(staged, candidate.Path, manifestBytes); err != nil {
			return err
		}
		report.Changed = append(report.Changed, changed...)
		e.document, e.warnings = candidate, warnings
		newTargets := make(map[string]struct{}, len(staged))
		for _, item := range staged {
			newTargets[item.target] = struct{}{}
		}
		for _, old := range oldSelections {
			target, targetErr := e.target(old)
			if targetErr != nil {
				return targetErr
			}
			if _, retained := newTargets[target]; retained {
				continue
			}
			if err := os.Remove(target); err != nil && !os.IsNotExist(err) {
				return err
			}
			removeEmptyParents(filepath.Dir(target), e.document.Dir)
		}
		return nil
	})
	return sortedReport(report), err
}

func (e *Engine) UpdateDownload(ctx context.Context, resource, download string) (Report, error) {
	selector := resource + "/" + download
	report := Report{}
	err := e.withMutationLock(ctx, func() error {
		report.Warnings = append([]manifest.Warning(nil), e.warnings...)
		candidate, cloneErr := e.document.Clone()
		if cloneErr != nil {
			return cloneErr
		}
		_, validateErr := candidate.Validate(e.options.Strict)
		if validateErr != nil {
			return validateErr
		}
		allSelections, selectErr := candidate.SelectAll([]string{selector})
		if selectErr != nil {
			return selectErr
		}
		client, clientErr := transport.New(e.options.Transport)
		if clientErr != nil {
			return clientErr
		}
		trusted, trustErr := e.retrustSelections(ctx, client, allSelections)
		if trustErr != nil {
			return trustErr
		}
		var changed []string
		for _, selection := range trusted {
			if err := candidate.SetIntegrity(selection, selection.Integrity); err != nil {
				return err
			}
			changed = append(changed, selectionLabel(selection))
		}
		warnings, validateErr := candidate.Validate(e.options.Strict)
		if validateErr != nil {
			return validateErr
		}
		selected, selectErr := candidate.SelectTarget([]string{selector}, e.targetOS)
		if selectErr != nil {
			return selectErr
		}
		staged, stageErr := e.stageSelections(selected)
		if stageErr != nil {
			return stageErr
		}
		defer cleanupStaged(staged, e.document.Dir)
		manifestBytes, marshalErr := candidate.Marshal()
		if marshalErr != nil {
			return marshalErr
		}
		if err := commitStaged(staged, candidate.Path, manifestBytes); err != nil {
			return err
		}
		report.Changed = append(report.Changed, changed...)
		e.document, e.warnings = candidate, warnings
		return nil
	})
	return sortedReport(report), err
}

type committedFile struct {
	target string
	backup string
}

func commitStaged(items []stagedDownload, manifestPath string, manifestBytes []byte) error {
	var committed []committedFile
	rollback := func() {
		for index := len(committed) - 1; index >= 0; index-- {
			item := committed[index]
			_ = os.Remove(item.target)
			if item.backup != "" {
				_ = os.Rename(item.backup, item.target)
			}
		}
	}
	for _, item := range items {
		entry := committedFile{target: item.target}
		if _, err := os.Stat(item.target); err == nil {
			backup, createErr := os.CreateTemp(filepath.Dir(item.target), ".muamba-backup-*")
			if createErr != nil {
				rollback()
				return createErr
			}
			entry.backup = backup.Name()
			_ = backup.Close()
			_ = os.Remove(entry.backup)
			if err := os.Rename(item.target, entry.backup); err != nil {
				rollback()
				return err
			}
		}
		if err := os.Rename(item.temporary, item.target); err != nil {
			if entry.backup != "" {
				_ = os.Rename(entry.backup, entry.target)
			}
			rollback()
			return err
		}
		committed = append(committed, entry)
	}
	if err := writeAtomic(manifestPath, manifestBytes, 0o644); err != nil {
		rollback()
		return err
	}
	for _, item := range committed {
		if item.backup != "" {
			_ = os.Remove(item.backup)
		}
	}
	return nil
}

func cleanupStaged(items []stagedDownload, root string) {
	for _, item := range items {
		_ = os.Remove(item.temporary)
		removeEmptyParents(filepath.Dir(item.target), root)
	}
}

func removeEmptyParents(start, root string) {
	canonicalRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		return
	}
	for current := start; current != canonicalRoot; current = filepath.Dir(current) {
		relative, err := filepath.Rel(canonicalRoot, current)
		if err != nil || relative == ".." || len(relative) >= 3 && relative[:3] == ".."+string(filepath.Separator) {
			return
		}
		if err := os.Remove(current); err != nil {
			return
		}
	}
}
