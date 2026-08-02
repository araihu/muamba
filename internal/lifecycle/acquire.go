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

type downloadedFile struct {
	path      string
	digest    integrity.Digest
	integrity string
}

func (e *Engine) stageCached(selection manifest.Selection) (stagedDownload, error) {
	expected, err := integrity.Parse(selection.Integrity)
	if err != nil {
		return stagedDownload{}, fmt.Errorf("%s: %w", selectionLabel(selection), err)
	}
	target, err := e.target(selection)
	if err != nil {
		return stagedDownload{}, err
	}
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return stagedDownload{}, err
	}
	temporary, err := os.CreateTemp(filepath.Dir(target), ".muamba-stage-*")
	if err != nil {
		return stagedDownload{}, err
	}
	temporaryPath := temporary.Name()
	if err := temporary.Close(); err != nil {
		_ = os.Remove(temporaryPath)
		return stagedDownload{}, err
	}
	if err := os.Remove(temporaryPath); err != nil {
		return stagedDownload{}, err
	}
	if err := e.cache.Materialize(expected, temporaryPath, selectionMode(selection)); err != nil {
		_ = os.Remove(temporaryPath)
		return stagedDownload{}, err
	}
	return stagedDownload{selection: selection, target: target, temporary: temporaryPath, integrity: selection.Integrity}, nil
}

func (e *Engine) download(ctx context.Context, client *transport.Client, selection manifest.Selection, expected *integrity.Digest) (downloadedFile, error) {
	file, err := os.CreateTemp("", ".muamba-download-*")
	if err != nil {
		return downloadedFile{}, err
	}
	path := file.Name()
	ok := false
	defer func() {
		_ = file.Close()
		if !ok {
			_ = os.Remove(path)
		}
	}()
	if _, err := client.Fetch(ctx, selection.URL, file, e.effectiveMaxBytes(selection)); err != nil {
		return downloadedFile{}, fmt.Errorf("%s: %w", selectionLabel(selection), err)
	}
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return downloadedFile{}, err
	}
	var digest integrity.Digest
	if expected == nil {
		sum, err := integrity.Compute(file, crypto.SHA384)
		if err != nil {
			return downloadedFile{}, err
		}
		digest = integrity.Digest{Algorithm: crypto.SHA384, Sum: sum}
	} else {
		if _, err := integrity.Verify(file, *expected); err != nil {
			return downloadedFile{}, fmt.Errorf("%s remote bytes: %w", selectionLabel(selection), err)
		}
		digest = *expected
	}
	if err := file.Chmod(0o600); err != nil {
		return downloadedFile{}, err
	}
	if err := file.Sync(); err != nil {
		return downloadedFile{}, err
	}
	if err := file.Close(); err != nil {
		return downloadedFile{}, err
	}
	ok = true
	return downloadedFile{path: path, digest: digest, integrity: integrity.FormatSRI(digest.Algorithm, digest.Sum)}, nil
}

func (e *Engine) retrustSelections(ctx context.Context, client *transport.Client, selections []manifest.Selection) ([]manifest.Selection, error) {
	trusted := make([]manifest.Selection, 0, len(selections))
	for _, selection := range selections {
		downloaded, err := e.download(ctx, client, selection, nil)
		if err != nil {
			return nil, err
		}
		if err := e.cache.Seed(downloaded.path, downloaded.digest); err != nil {
			_ = os.Remove(downloaded.path)
			return nil, err
		}
		_ = os.Remove(downloaded.path)
		selection.Integrity = downloaded.integrity
		trusted = append(trusted, selection)
	}
	return trusted, nil
}

func (e *Engine) stageSelections(selections []manifest.Selection) ([]stagedDownload, error) {
	staged := make([]stagedDownload, 0, len(selections))
	for _, selection := range selections {
		item, err := e.stageCached(selection)
		if err != nil {
			cleanupStaged(staged, e.document.Dir)
			return nil, err
		}
		staged = append(staged, item)
	}
	return staged, nil
}

func (e *Engine) restoreLocked(ctx context.Context, client *transport.Client, selection manifest.Selection) (bool, error) {
	expected, err := integrity.Parse(selection.Integrity)
	if err != nil {
		return false, fmt.Errorf("%s: %w", selectionLabel(selection), err)
	}
	target, err := e.target(selection)
	if err != nil {
		return false, err
	}
	if err := e.verifyFile(selection); err == nil {
		if err := os.Chmod(target, selectionMode(selection)); err != nil {
			return false, err
		}
		if err := e.cache.Verify(expected); err != nil {
			if err := e.cache.Seed(target, expected); err != nil {
				return false, err
			}
		}
		return false, nil
	}
	if err := e.cache.Materialize(expected, target, selectionMode(selection)); err == nil {
		return true, nil
	}
	downloaded, err := e.download(ctx, client, selection, &expected)
	if err != nil {
		return false, err
	}
	defer func() { _ = os.Remove(downloaded.path) }()
	if err := e.cache.Seed(downloaded.path, expected); err != nil {
		return false, err
	}
	if err := e.cache.Materialize(expected, target, selectionMode(selection)); err != nil {
		return false, err
	}
	return true, nil
}
