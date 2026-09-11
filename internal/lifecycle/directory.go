package lifecycle

import (
	"bytes"
	"context"
	"crypto"
	"fmt"
	"os"
	"path/filepath"
	"sort"

	archivepkg "github.com/araihu/muamba/internal/archive"
	"github.com/araihu/muamba/internal/integrity"
	"github.com/araihu/muamba/internal/manifest"
	"github.com/araihu/muamba/internal/transport"
)

func (e *Engine) directorySelections(selectors []string) ([]manifest.DirectorySelection, error) {
	return e.document.SelectDirectories(selectors)
}

func (e *Engine) effectiveDirectoryMaxBytes(directory manifest.DirectorySelection) int64 {
	if e.options.MaxBytesSet {
		return e.options.MaxBytes
	}
	return directory.MaxBytes
}

func (e *Engine) acquireDirectory(ctx context.Context, client *transport.Client, directory manifest.DirectorySelection) (manifest.LockedDirectory, []manifest.Selection, error) {
	archiveSelection := manifest.Selection{
		ResourceName: directory.ResourceName, DownloadName: directory.DirectoryName,
		URL: directory.URL, MaxBytes: e.effectiveDirectoryMaxBytes(directory), Size: -1,
	}
	downloaded, err := e.download(ctx, client, archiveSelection, nil)
	if err != nil {
		return manifest.LockedDirectory{}, nil, err
	}
	defer func() { _ = os.Remove(downloaded.path) }()
	locked := manifest.LockedDirectory{
		ID: directory.ID(), URL: directory.URL, Path: filepath.ToSlash(directory.Path),
		Size: downloaded.size, Integrity: downloaded.integrity,
	}
	staging, err := newDirectoryStaging()
	if err != nil {
		return manifest.LockedDirectory{}, nil, err
	}
	defer staging.close()
	var selections []manifest.Selection
	err = archivepkg.WalkTarGz(ctx, downloaded.path, archivepkg.Options{
		StripComponents: directory.StripComponents, Include: directory.Include, Exclude: directory.Exclude,
		MaxFiles: directory.MaxFiles, MaxBytes: directory.MaxUnpackedBytes,
	}, func(file archivepkg.File) error {
		digest, err := digestBytes(file.Contents)
		if err != nil {
			return err
		}
		if err := staging.add(file.Contents, digest); err != nil {
			return err
		}
		path := filepath.ToSlash(filepath.Join(directory.Path, filepath.FromSlash(file.Path)))
		lockedFile := manifest.LockedDirectoryFile{
			Source: file.Path, Path: path, Size: file.Size,
			Integrity: integrity.FormatSRI(digest.Algorithm, digest.Sum),
		}
		locked.Files = append(locked.Files, lockedFile)
		selections = append(selections, directoryFileSelection(directory, lockedFile))
		return nil
	})
	if err != nil {
		return manifest.LockedDirectory{}, nil, fmt.Errorf("%s: %w", directory.ID(), err)
	}
	if err := staging.seed(ctx, e.cache); err != nil {
		return manifest.LockedDirectory{}, nil, err
	}
	sort.Slice(locked.Files, func(i, j int) bool { return locked.Files[i].Source < locked.Files[j].Source })
	sort.Slice(selections, func(i, j int) bool { return selections[i].Path < selections[j].Path })
	return locked, selections, nil
}

func directoryFileSelections(directory manifest.DirectorySelection) []manifest.Selection {
	if directory.Lock == nil {
		return nil
	}
	selections := make([]manifest.Selection, 0, len(directory.Lock.Files))
	for _, file := range directory.Lock.Files {
		selections = append(selections, directoryFileSelection(directory, file))
	}
	return selections
}

func directoryFileSelection(directory manifest.DirectorySelection, file manifest.LockedDirectoryFile) manifest.Selection {
	return manifest.Selection{
		ResourceName: directory.ResourceName,
		DownloadName: directory.DirectoryName + ":" + file.Source,
		Version:      directory.Version,
		URL:          directory.URL,
		Path:         filepath.FromSlash(file.Path),
		Integrity:    file.Integrity,
		Platform:     "multi",
		MaxBytes:     directory.MaxBytes,
		Size:         file.Size,
	}
}

func (e *Engine) syncDirectory(ctx context.Context, client *transport.Client, directory manifest.DirectorySelection) ([]string, []string, error) {
	if directory.Lock == nil {
		return nil, nil, fmt.Errorf("%s is unlocked", directory.ID())
	}
	selections := directoryFileSelections(directory)
	missing := make([]manifest.Selection, 0)
	var changed, verified []string
	for _, selection := range selections {
		if err := ctx.Err(); err != nil {
			return nil, nil, err
		}
		if err := e.verifyFile(selection); err == nil {
			digest, _ := integrity.Parse(selection.Integrity)
			target, _ := e.target(selection)
			if cacheErr := e.cache.Verify(digest); cacheErr != nil {
				if seedErr := e.cache.Seed(target, digest); seedErr != nil {
					return nil, nil, seedErr
				}
			}
			verified = append(verified, selectionLabel(selection))
			continue
		}
		digest, parseErr := integrity.Parse(selection.Integrity)
		if parseErr != nil {
			return nil, nil, parseErr
		}
		target, targetErr := e.target(selection)
		if targetErr != nil {
			return nil, nil, targetErr
		}
		if materializeErr := e.cache.Materialize(digest, target, selectionMode(selection)); materializeErr == nil {
			changed = append(changed, selectionLabel(selection))
			continue
		}
		missing = append(missing, selection)
	}
	if len(missing) == 0 {
		return changed, verified, nil
	}
	expectedArchive, err := integrity.Parse(directory.Lock.Integrity)
	if err != nil {
		return nil, nil, fmt.Errorf("%s archive: %w", directory.ID(), err)
	}
	archiveSelection := manifest.Selection{
		ResourceName: directory.ResourceName, DownloadName: directory.DirectoryName,
		URL: directory.URL, MaxBytes: e.effectiveDirectoryMaxBytes(directory), Size: directory.Lock.Size,
	}
	downloaded, err := e.download(ctx, client, archiveSelection, &expectedArchive)
	if err != nil {
		return nil, nil, err
	}
	defer func() { _ = os.Remove(downloaded.path) }()
	if downloaded.size != directory.Lock.Size {
		return nil, nil, fmt.Errorf("%s archive size = %d, want %d", directory.ID(), downloaded.size, directory.Lock.Size)
	}
	locked := make(map[string]manifest.LockedDirectoryFile, len(directory.Lock.Files))
	for _, file := range directory.Lock.Files {
		locked[file.Source] = file
	}
	staging, err := newDirectoryStaging()
	if err != nil {
		return nil, nil, err
	}
	defer staging.close()
	count := 0
	err = archivepkg.WalkTarGz(ctx, downloaded.path, archivepkg.Options{
		StripComponents: directory.StripComponents, Include: directory.Include, Exclude: directory.Exclude,
		MaxFiles: directory.MaxFiles, MaxBytes: directory.MaxUnpackedBytes,
	}, func(file archivepkg.File) error {
		count++
		digest, err := verifyDirectoryFile(directory, locked, file)
		if err != nil {
			return err
		}
		return staging.add(file.Contents, digest)
	})
	if err != nil {
		return nil, nil, fmt.Errorf("%s: %w", directory.ID(), err)
	}
	if count != len(locked) {
		return nil, nil, fmt.Errorf("%s resolved file set changed: got %d files, want %d", directory.ID(), count, len(locked))
	}
	if err := staging.seed(ctx, e.cache); err != nil {
		return nil, nil, err
	}
	for _, selection := range missing {
		if err := ctx.Err(); err != nil {
			return nil, nil, err
		}
		digest, _ := integrity.Parse(selection.Integrity)
		target, _ := e.target(selection)
		if err := e.cache.Materialize(digest, target, selectionMode(selection)); err != nil {
			return nil, nil, err
		}
		changed = append(changed, selectionLabel(selection))
	}
	sort.Strings(changed)
	sort.Strings(verified)
	return changed, verified, nil
}

func (e *Engine) verifyAndSeedDirectory(directory manifest.DirectorySelection, files []archivepkg.File) error {
	if directory.Lock == nil || len(files) != len(directory.Lock.Files) {
		return fmt.Errorf("%s resolved file set changed: got %d files, want %d", directory.ID(), len(files), len(directory.Lock.Files))
	}
	locked := make(map[string]manifest.LockedDirectoryFile, len(directory.Lock.Files))
	for _, file := range directory.Lock.Files {
		locked[file.Source] = file
	}
	for _, file := range files {
		digest, err := verifyDirectoryFile(directory, locked, file)
		if err != nil {
			return err
		}
		if err := e.seedBytes(file.Contents, digest); err != nil {
			return err
		}
	}
	return nil
}

func verifyDirectoryFile(directory manifest.DirectorySelection, locked map[string]manifest.LockedDirectoryFile, file archivepkg.File) (integrity.Digest, error) {
	expected, ok := locked[file.Path]
	if !ok {
		return integrity.Digest{}, fmt.Errorf("%s resolved unexpected file %q", directory.ID(), file.Path)
	}
	if file.Size != expected.Size {
		return integrity.Digest{}, fmt.Errorf("%s file %q size = %d, want %d", directory.ID(), file.Path, file.Size, expected.Size)
	}
	digest, err := digestBytes(file.Contents)
	if err != nil {
		return integrity.Digest{}, err
	}
	if integrity.FormatSRI(digest.Algorithm, digest.Sum) != expected.Integrity {
		return integrity.Digest{}, fmt.Errorf("%s file %q integrity mismatch", directory.ID(), file.Path)
	}
	return digest, nil
}

func digestBytes(contents []byte) (integrity.Digest, error) {
	sum, err := integrity.Compute(bytes.NewReader(contents), crypto.SHA384)
	if err != nil {
		return integrity.Digest{}, err
	}
	return integrity.Digest{Algorithm: crypto.SHA384, Sum: sum}, nil
}

func (e *Engine) seedBytes(contents []byte, digest integrity.Digest) error {
	file, err := os.CreateTemp("", ".muamba-directory-file-*")
	if err != nil {
		return err
	}
	path := file.Name()
	defer func() { _ = os.Remove(path) }()
	if _, err := file.Write(contents); err != nil {
		_ = file.Close()
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	return e.cache.Seed(path, digest)
}
