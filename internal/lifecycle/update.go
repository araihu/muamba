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
		oldSelections, oldDirectories, err := e.preflightResource(resource)
		if err != nil {
			return err
		}
		candidate, warnings, staged, changed, err := e.prepareResourceUpdate(ctx, resource, version)
		if err != nil {
			return err
		}
		defer cleanupStaged(staged, e.document.Dir)
		metadata, marshalErr := metadataWrites(candidate, true)
		if marshalErr != nil {
			return marshalErr
		}
		if err := commitStaged(staged, metadata); err != nil {
			return err
		}
		report.Changed = append(report.Changed, changed...)
		e.document, e.warnings = candidate, warnings
		return e.removeStaleResourceFiles(oldSelections, oldDirectories, staged)
	})
	return sortedReport(report), err
}

func (e *Engine) preflightResource(resource string) ([]manifest.Selection, []manifest.DirectorySelection, error) {
	downloads, err := e.selections([]string{resource})
	if err != nil {
		return nil, nil, err
	}
	for _, selection := range downloads {
		if err := e.verifyFile(selection); err != nil {
			return nil, nil, fmt.Errorf("preflight old %s: %w", selectionLabel(selection), err)
		}
	}
	directories, err := e.directorySelections([]string{resource})
	if err != nil {
		return nil, nil, err
	}
	for _, directory := range directories {
		if directory.Lock == nil {
			return nil, nil, fmt.Errorf("preflight old %s: unlocked", directory.ID())
		}
		for _, selection := range directoryFileSelections(directory) {
			if err := e.verifyFile(selection); err != nil {
				return nil, nil, fmt.Errorf("preflight old %s: %w", selectionLabel(selection), err)
			}
		}
	}
	return downloads, directories, nil
}

func (e *Engine) prepareResourceUpdate(ctx context.Context, resource, version string) (*manifest.Document, []manifest.Warning, []stagedDownload, []string, error) {
	candidate, err := e.document.Clone()
	if err != nil {
		return nil, nil, nil, nil, err
	}
	if err := candidate.SetVersion(resource, version); err != nil {
		return nil, nil, nil, nil, err
	}
	if err := candidate.ClearResourceLocks(resource); err != nil {
		return nil, nil, nil, nil, err
	}
	if _, err := candidate.Validate(e.options.Strict); err != nil {
		return nil, nil, nil, nil, err
	}
	client, err := transport.New(e.options.Transport)
	if err != nil {
		return nil, nil, nil, nil, err
	}
	downloads, changed, err := e.retrustResourceDownloads(ctx, client, candidate, resource)
	if err != nil {
		return nil, nil, nil, nil, err
	}
	directoryFiles, directoryChanges, err := e.retrustResourceDirectories(ctx, client, candidate, resource)
	if err != nil {
		return nil, nil, nil, nil, err
	}
	warnings, err := candidate.Validate(e.options.Strict)
	if err != nil {
		return nil, nil, nil, nil, err
	}
	staged, err := e.stageSelections(append(downloads, directoryFiles...))
	if err != nil {
		return nil, nil, nil, nil, err
	}
	return candidate, warnings, staged, append(changed, directoryChanges...), nil
}

func (e *Engine) retrustResourceDownloads(ctx context.Context, client *transport.Client, candidate *manifest.Document, resource string) ([]manifest.Selection, []string, error) {
	selections, err := candidate.SelectAll([]string{resource})
	if err != nil {
		return nil, nil, err
	}
	trusted, err := e.retrustSelections(ctx, client, selections)
	if err != nil {
		return nil, nil, err
	}
	var changed []string
	for _, selection := range trusted {
		if err := candidate.SetIntegrity(selection, selection.Integrity); err != nil {
			return nil, nil, err
		}
		changed = append(changed, selectionLabel(selection))
	}
	selected, err := candidate.SelectTarget([]string{resource}, e.targetOS)
	return selected, changed, err
}

func (e *Engine) retrustResourceDirectories(ctx context.Context, client *transport.Client, candidate *manifest.Document, resource string) ([]manifest.Selection, []string, error) {
	directories, err := candidate.SelectDirectories([]string{resource})
	if err != nil {
		return nil, nil, err
	}
	var files []manifest.Selection
	var changed []string
	for _, directory := range directories {
		locked, acquired, err := e.acquireDirectory(ctx, client, directory)
		if err != nil {
			return nil, nil, err
		}
		if err := candidate.SetDirectoryLock(locked); err != nil {
			return nil, nil, err
		}
		files = append(files, acquired...)
		for _, selection := range acquired {
			changed = append(changed, selectionLabel(selection))
		}
	}
	return files, changed, nil
}

func (e *Engine) removeStaleResourceFiles(downloads []manifest.Selection, directories []manifest.DirectorySelection, staged []stagedDownload) error {
	retained := make(map[string]struct{}, len(staged))
	for _, item := range staged {
		retained[item.target] = struct{}{}
	}
	old := append([]manifest.Selection(nil), downloads...)
	for _, directory := range directories {
		old = append(old, directoryFileSelections(directory)...)
	}
	for _, selection := range old {
		target, err := e.target(selection)
		if err != nil {
			return err
		}
		if _, ok := retained[target]; ok {
			continue
		}
		if err := os.Remove(target); err != nil && !os.IsNotExist(err) {
			return err
		}
		removeEmptyParents(filepath.Dir(target), e.document.Dir)
	}
	return nil
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
		directories, directoryErr := candidate.SelectDirectories([]string{selector})
		if directoryErr != nil {
			return directoryErr
		}
		if len(directories) == 1 {
			return e.updateDirectoryCandidate(ctx, candidate, directories[0], &report)
		}
		return e.updateFileCandidate(ctx, candidate, selector, &report)
	})
	return sortedReport(report), err
}

func (e *Engine) updateDirectoryCandidate(ctx context.Context, candidate *manifest.Document, old manifest.DirectorySelection, report *Report) error {
	client, err := transport.New(e.options.Transport)
	if err != nil {
		return err
	}
	locked, files, err := e.acquireDirectory(ctx, client, old)
	if err != nil {
		return err
	}
	if err := candidate.SetDirectoryLock(locked); err != nil {
		return err
	}
	warnings, err := candidate.Validate(e.options.Strict)
	if err != nil {
		return err
	}
	staged, err := e.stageSelections(files)
	if err != nil {
		return err
	}
	defer cleanupStaged(staged, e.document.Dir)
	metadata, err := metadataWrites(candidate, false)
	if err != nil {
		return err
	}
	if err := commitStaged(staged, metadata); err != nil {
		return err
	}
	if err := e.removeStaleResourceFiles(nil, []manifest.DirectorySelection{old}, staged); err != nil {
		return err
	}
	for _, selection := range files {
		report.Changed = append(report.Changed, selectionLabel(selection))
	}
	e.document, e.warnings = candidate, warnings
	return nil
}

func (e *Engine) updateFileCandidate(ctx context.Context, candidate *manifest.Document, selector string, report *Report) error {
	allSelections, err := candidate.SelectAll([]string{selector})
	if err != nil {
		return err
	}
	client, err := transport.New(e.options.Transport)
	if err != nil {
		return err
	}
	trusted, err := e.retrustSelections(ctx, client, allSelections)
	if err != nil {
		return err
	}
	for _, selection := range trusted {
		if err := candidate.SetIntegrity(selection, selection.Integrity); err != nil {
			return err
		}
		report.Changed = append(report.Changed, selectionLabel(selection))
	}
	warnings, err := candidate.Validate(e.options.Strict)
	if err != nil {
		return err
	}
	selected, err := candidate.SelectTarget([]string{selector}, e.targetOS)
	if err != nil {
		return err
	}
	staged, err := e.stageSelections(selected)
	if err != nil {
		return err
	}
	defer cleanupStaged(staged, e.document.Dir)
	metadata, err := metadataWrites(candidate, false)
	if err != nil {
		return err
	}
	if err := commitStaged(staged, metadata); err != nil {
		return err
	}
	e.document, e.warnings = candidate, warnings
	return nil
}

type committedFile struct {
	target string
	backup string
}

type metadataWrite struct {
	path     string
	contents []byte
}

func metadataWrites(document *manifest.Document, declarationChanged bool) ([]metadataWrite, error) {
	if document.IsLegacy() {
		contents, err := document.Marshal()
		return []metadataWrite{{path: document.Path, contents: contents}}, err
	}
	lock, err := document.MarshalLock()
	if err != nil {
		return nil, err
	}
	writes := []metadataWrite{{path: document.LockPath, contents: lock}}
	if declarationChanged {
		declaration, marshalErr := document.Marshal()
		if marshalErr != nil {
			return nil, marshalErr
		}
		writes = append([]metadataWrite{{path: document.Path, contents: declaration}}, writes...)
	}
	return writes, nil
}

func commitStaged(items []stagedDownload, metadata []metadataWrite) error {
	type stagedFile struct {
		target    string
		temporary string
	}
	files := make([]stagedFile, 0, len(items)+len(metadata))
	for _, item := range items {
		files = append(files, stagedFile{target: item.target, temporary: item.temporary})
	}
	for _, write := range metadata {
		if err := os.MkdirAll(filepath.Dir(write.path), 0o755); err != nil {
			return err
		}
		temporary, err := os.CreateTemp(filepath.Dir(write.path), ".muamba-metadata-*")
		if err != nil {
			return err
		}
		temporaryPath := temporary.Name()
		defer func() {
			_ = temporary.Close()
			_ = os.Remove(temporaryPath)
		}()
		if _, err := temporary.Write(write.contents); err != nil {
			return err
		}
		if err := temporary.Chmod(0o644); err != nil {
			return err
		}
		if err := temporary.Sync(); err != nil {
			return err
		}
		if err := temporary.Close(); err != nil {
			return err
		}
		files = append(files, stagedFile{target: write.path, temporary: temporaryPath})
	}
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
	for _, item := range files {
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
