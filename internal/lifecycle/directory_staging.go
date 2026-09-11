package lifecycle

import (
	"context"
	"os"

	"github.com/araihu/muamba/internal/blobcache"
	"github.com/araihu/muamba/internal/integrity"
)

// directoryStaging keeps extracted contents private until the entire archive
// and locked inventory have passed validation. Failed attempts cannot make a
// later Sync bypass archive validation through partially seeded cache blobs.
type directoryStaging struct {
	root  string
	files []stagedDirectoryFile
}

type stagedDirectoryFile struct {
	path   string
	digest integrity.Digest
}

func newDirectoryStaging() (*directoryStaging, error) {
	root, err := os.MkdirTemp("", "muamba-directory-*")
	if err != nil {
		return nil, err
	}
	return &directoryStaging{root: root}, nil
}

func (s *directoryStaging) close() { _ = os.RemoveAll(s.root) }

func (s *directoryStaging) add(contents []byte, digest integrity.Digest) error {
	file, err := os.CreateTemp(s.root, "file-*")
	if err != nil {
		return err
	}
	if _, err := file.Write(contents); err != nil {
		_ = file.Close()
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	s.files = append(s.files, stagedDirectoryFile{file.Name(), digest})
	return nil
}

func (s *directoryStaging) seed(ctx context.Context, cache *blobcache.Store) error {
	for _, file := range s.files {
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := cache.SeedContext(ctx, file.path, file.digest); err != nil {
			return err
		}
	}
	return ctx.Err()
}
