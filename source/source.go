// Package source exposes Muamba's acquisition boundary to Go libraries.
//
// It intentionally has no command installation or manifest discovery. The
// caller supplies both declaration and lock paths, then chooses explicitly
// between first-trust Lock and verified Snapshot operations.
package source

import (
	"context"
	"fmt"

	"github.com/araihu/muamba/internal/lifecycle"
	"github.com/araihu/muamba/internal/transport"
)

// Options identifies one explicit Muamba declaration/lock namespace.
type Options struct {
	ManifestPath string
	LockPath     string
	CacheDir     string
	Strict       bool
	AllowHTTP    bool
}

// Report summarizes a lock or synchronization operation.
type Report struct {
	Changed  []string
	Verified []string
}

// SnapshotFile is a verified in-memory source file. Consumers should use
// Contents, not reopen Path, because Contents are bound to the bytes that
// Muamba verified under its mutation lock.
type SnapshotFile struct {
	ID        string
	Source    string
	Path      string
	Size      int64
	Integrity string
	Contents  []byte
}

// Engine owns one explicit declaration/lock namespace.
type Engine struct {
	inner *lifecycle.Engine
}

// New opens the explicit declaration and lock paths. It never searches for a
// parent .muamba.yaml or muamba.yaml.
func New(options Options) (*Engine, error) {
	if options.ManifestPath == "" {
		return nil, fmt.Errorf("source manifest path is required")
	}
	if options.LockPath == "" {
		return nil, fmt.Errorf("source lock path is required")
	}
	inner, err := lifecycle.NewWithLock(options.ManifestPath, options.LockPath, lifecycle.Options{
		Strict: options.Strict, CacheDir: options.CacheDir,
		Transport: transport.Options{AllowHTTP: options.AllowHTTP},
	})
	if err != nil {
		return nil, err
	}
	return &Engine{inner: inner}, nil
}

// Lock performs explicit first trust for the selected declarations.
func (e *Engine) Lock(ctx context.Context, selectors []string) (Report, error) {
	if e == nil || e.inner == nil {
		return Report{}, fmt.Errorf("source engine is nil")
	}
	report, err := e.inner.Lock(ctx, selectors)
	return Report{Changed: append([]string(nil), report.Changed...), Verified: append([]string(nil), report.Verified...)}, err
}

// Sync verifies and restores all selected files from their locked sources.
func (e *Engine) Sync(ctx context.Context, selectors []string) (Report, error) {
	if e == nil || e.inner == nil {
		return Report{}, fmt.Errorf("source engine is nil")
	}
	report, err := e.inner.Sync(ctx, selectors)
	return Report{Changed: append([]string(nil), report.Changed...), Verified: append([]string(nil), report.Verified...)}, err
}

// Snapshot synchronizes and returns the selected locked files as immutable
// in-memory copies. A missing or changed lock is an error; no implicit trust
// occurs.
func (e *Engine) Snapshot(ctx context.Context, selectors []string) ([]SnapshotFile, error) {
	if e == nil || e.inner == nil {
		return nil, fmt.Errorf("source engine is nil")
	}
	files, err := e.inner.Snapshot(ctx, selectors)
	if err != nil {
		return nil, err
	}
	result := make([]SnapshotFile, len(files))
	for index, file := range files {
		result[index] = SnapshotFile{
			ID: file.ID, Source: file.Source, Path: file.Path, Size: file.Size,
			Integrity: file.Integrity, Contents: append([]byte(nil), file.Contents...),
		}
	}
	return result, nil
}
