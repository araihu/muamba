package lifecycle

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/araihu/muamba/internal/blobcache"
	"github.com/araihu/muamba/internal/manifest"
	"github.com/araihu/muamba/internal/safepath"
	"github.com/araihu/muamba/internal/transport"
	"github.com/gofrs/flock"
)

type Options struct {
	Strict      bool
	Transport   transport.Options
	LockTimeout time.Duration
	Target      manifest.Target
	CacheDir    string
	MaxBytes    int64
	MaxBytesSet bool
}

type Report struct {
	Changed  []string
	Verified []string
	Warnings []manifest.Warning
}

type Engine struct {
	document *manifest.Document
	options  Options
	warnings []manifest.Warning
	cache    *blobcache.Store
	targetOS manifest.Target
}

func New(manifestPath string, options Options) (*Engine, error) {
	if options.MaxBytesSet && options.MaxBytes <= 0 {
		return nil, fmt.Errorf("MaxBytes must be positive when explicitly set")
	}
	document, err := manifest.Load(manifestPath)
	if err != nil {
		return nil, err
	}
	warnings, err := document.Validate(options.Strict)
	if err != nil {
		return nil, err
	}
	if options.Target == (manifest.Target{}) {
		options.Target = manifest.RuntimeTarget()
	}
	if _, err := manifest.ParseTarget(options.Target.String()); err != nil {
		return nil, err
	}
	selections, err := document.SelectTarget(nil, options.Target)
	if err != nil {
		return nil, err
	}
	if err := safepath.ValidateUnique(document.Dir, selections); err != nil {
		return nil, err
	}
	directories, err := document.SelectDirectories(nil)
	if err != nil {
		return nil, err
	}
	for _, directory := range directories {
		selections = append(selections, directoryFileSelections(directory)...)
	}
	if err := safepath.ValidateUnique(document.Dir, selections); err != nil {
		return nil, err
	}
	if options.LockTimeout <= 0 {
		options.LockTimeout = 5 * time.Second
	}
	cacheDir := options.CacheDir
	if cacheDir == "" {
		cacheDir = os.Getenv("MUAMBA_CACHE_DIR")
	}
	if cacheDir == "" {
		cacheDir, err = blobcache.DefaultRoot()
		if err != nil {
			return nil, err
		}
	}
	cache, err := blobcache.New(cacheDir)
	if err != nil {
		return nil, err
	}
	return &Engine{document: document, options: options, warnings: warnings, cache: cache, targetOS: options.Target}, nil
}

func (e *Engine) selections(selectors []string) ([]manifest.Selection, error) {
	return e.document.SelectTarget(selectors, e.targetOS)
}

func (e *Engine) allSelections(selectors []string) ([]manifest.Selection, error) {
	return e.document.SelectAll(selectors)
}

func (e *Engine) effectiveMaxBytes(selection manifest.Selection) int64 {
	if e.options.MaxBytesSet {
		return e.options.MaxBytes
	}
	return selection.MaxBytes
}

func selectionLabel(selection manifest.Selection) string {
	label := selection.ResourceName + "/" + selection.DownloadName
	if selection.Variant != "" {
		return label + "[" + selection.Variant + "]"
	}
	return label
}

func selectionMode(selection manifest.Selection) os.FileMode {
	if selection.Executable {
		return 0o755
	}
	return 0o644
}

func (e *Engine) target(selection manifest.Selection) (string, error) {
	return safepath.Resolve(e.document.Dir, selection.Path)
}

func (e *Engine) reloadDocument() error {
	document, err := manifest.Load(e.document.Path)
	if err != nil {
		return err
	}
	warnings, err := document.Validate(e.options.Strict)
	if err != nil {
		return err
	}
	selections, err := document.SelectTarget(nil, e.targetOS)
	if err != nil {
		return err
	}
	directories, err := document.SelectDirectories(nil)
	if err != nil {
		return err
	}
	for _, directory := range directories {
		selections = append(selections, directoryFileSelections(directory)...)
	}
	if err := safepath.ValidateUnique(document.Dir, selections); err != nil {
		return err
	}
	e.document, e.warnings = document, warnings
	return nil
}

func (e *Engine) withMutationLock(ctx context.Context, operation func() error) error {
	digest := sha256.Sum256([]byte(e.document.Path))
	lockDir := filepath.Join(os.TempDir(), "muamba-locks")
	if err := os.MkdirAll(lockDir, 0o700); err != nil {
		return err
	}
	lock := flock.New(filepath.Join(lockDir, hex.EncodeToString(digest[:16])+".lock"))
	lockContext, cancel := context.WithTimeout(ctx, e.options.LockTimeout)
	defer cancel()
	locked, err := lock.TryLockContext(lockContext, 25*time.Millisecond)
	if err != nil {
		return fmt.Errorf("lock manifest %s: %w", e.document.Path, err)
	}
	if !locked {
		return fmt.Errorf("lock manifest %s: timed out", e.document.Path)
	}
	defer func() { _ = lock.Unlock() }()
	if err := e.reloadDocument(); err != nil {
		return err
	}
	return operation()
}

func sortedReport(report Report) Report {
	sort.Strings(report.Changed)
	sort.Strings(report.Verified)
	return report
}
