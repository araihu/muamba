package blobcache

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/araihu/muamba/internal/integrity"
	"github.com/gofrs/flock"
)

type Store struct {
	root string
}

func DefaultRoot() (string, error) {
	root, err := os.UserCacheDir()
	if err != nil {
		return "", fmt.Errorf("resolve user cache directory: %w", err)
	}
	return filepath.Join(root, "muamba"), nil
}

func New(root string) (*Store, error) {
	if root == "" {
		return nil, fmt.Errorf("cache directory must not be empty")
	}
	abs, err := filepath.Abs(root)
	if err != nil {
		return nil, fmt.Errorf("resolve cache directory: %w", err)
	}
	return &Store{root: abs}, nil
}

func (s *Store) Path(expected integrity.Digest) string {
	formatted := integrity.FormatHash(expected)
	algorithm, sum, _ := strings.Cut(formatted, ":")
	return filepath.Join(s.root, algorithm, sum)
}

func (s *Store) Verify(expected integrity.Digest) error {
	path := s.Path(expected)
	if err := verifyFile(path, expected); err != nil {
		return fmt.Errorf("verify cache blob %s: %w", path, err)
	}
	return nil
}

func (s *Store) Seed(source string, expected integrity.Digest) error {
	return s.SeedContext(context.Background(), source, expected)
}

// SeedContext verifies and atomically seeds a blob, observing cancellation
// while waiting for its lock, reading, copying, and before publication.
func (s *Store) SeedContext(ctx context.Context, source string, expected integrity.Digest) error {
	if err := verifyFileContext(ctx, source, expected); err != nil {
		return fmt.Errorf("verify cache source %s: %w", source, err)
	}
	target := s.Path(expected)
	if err := os.MkdirAll(filepath.Dir(target), 0o700); err != nil {
		return err
	}
	lock := flock.New(target + ".lock")
	defer func() { _ = lock.Close() }()
	locked, err := lock.TryLockContext(ctx, 25*time.Millisecond)
	if err != nil {
		return fmt.Errorf("lock cache blob: %w", err)
	}
	defer func() { _ = lock.Unlock() }()
	if !locked {
		return fmt.Errorf("cache lock was not acquired")
	}
	if err := verifyFileContext(ctx, target, expected); err == nil {
		return ctx.Err()
	}
	temporary, err := copyTemporaryContext(ctx, source, filepath.Dir(target), 0o644)
	if err != nil {
		return err
	}
	defer func() { _ = os.Remove(temporary) }()
	if err := verifyFileContext(ctx, temporary, expected); err != nil {
		return fmt.Errorf("verify staged cache blob: %w", err)
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := os.Rename(temporary, target); err != nil {
		return fmt.Errorf("publish cache blob: %w", err)
	}
	return nil
}

func (s *Store) Materialize(expected integrity.Digest, destination string, mode os.FileMode) error {
	source := s.Path(expected)
	if err := s.Verify(expected); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(destination), 0o755); err != nil {
		return err
	}
	temporary, err := copyTemporary(source, filepath.Dir(destination), mode)
	if err != nil {
		return err
	}
	defer func() { _ = os.Remove(temporary) }()
	if err := verifyFile(temporary, expected); err != nil {
		return fmt.Errorf("verify staged destination: %w", err)
	}
	if err := os.Rename(temporary, destination); err != nil {
		return fmt.Errorf("publish destination: %w", err)
	}
	return nil
}

func verifyFile(path string, expected integrity.Digest) error {
	return verifyFileContext(context.Background(), path, expected)
}

func verifyFileContext(ctx context.Context, path string, expected integrity.Digest) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer func() { _ = file.Close() }()
	_, err = integrity.Verify(contextReader{ctx, file}, expected)
	return err
}

func copyTemporary(source, directory string, mode os.FileMode) (string, error) {
	return copyTemporaryContext(context.Background(), source, directory, mode)
}

func copyTemporaryContext(ctx context.Context, source, directory string, mode os.FileMode) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	input, err := os.Open(source)
	if err != nil {
		return "", err
	}
	defer func() { _ = input.Close() }()
	output, err := os.CreateTemp(directory, ".muamba-blob-*")
	if err != nil {
		return "", err
	}
	path := output.Name()
	ok := false
	defer func() {
		_ = output.Close()
		if !ok {
			_ = os.Remove(path)
		}
	}()
	if _, err := io.Copy(output, contextReader{ctx, input}); err != nil {
		return "", err
	}
	if err := output.Chmod(mode.Perm()); err != nil {
		return "", err
	}
	if err := ctx.Err(); err != nil {
		return "", err
	}
	if err := output.Sync(); err != nil {
		return "", err
	}
	if err := output.Close(); err != nil {
		return "", err
	}
	if err := ctx.Err(); err != nil {
		return "", err
	}
	ok = true
	return path, nil
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
