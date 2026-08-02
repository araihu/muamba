package lifecycle

import (
	"context"
	"crypto"
	"fmt"
	"io"
	"os"

	"github.com/araihu/muamba/internal/integrity"
	"github.com/araihu/muamba/internal/manifest"
	"github.com/araihu/muamba/internal/transport"
)

type downloadedFile struct {
	path      string
	digest    integrity.Digest
	integrity string
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
		if err := e.cache.Seed(target, expected); err != nil {
			return false, err
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
