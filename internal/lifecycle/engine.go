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

	"github.com/araihu/muamba/internal/manifest"
	"github.com/araihu/muamba/internal/safepath"
	"github.com/araihu/muamba/internal/transport"
	"github.com/gofrs/flock"
)

type Options struct {
	Strict      bool
	Transport   transport.Options
	LockTimeout time.Duration
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
}

func New(manifestPath string, options Options) (*Engine, error) {
	document, err := manifest.Load(manifestPath)
	if err != nil {
		return nil, err
	}
	warnings, err := document.Validate(options.Strict)
	if err != nil {
		return nil, err
	}
	selections, err := document.Select(nil)
	if err != nil {
		return nil, err
	}
	if err := safepath.ValidateUnique(document.Dir, selections); err != nil {
		return nil, err
	}
	if options.LockTimeout <= 0 {
		options.LockTimeout = 5 * time.Second
	}
	return &Engine{document: document, options: options, warnings: warnings}, nil
}

func (e *Engine) selections(selectors []string) ([]manifest.Selection, error) {
	return e.document.Select(selectors)
}

func (e *Engine) target(selection manifest.Selection) (string, error) {
	return safepath.Resolve(e.document.Dir, selection.Path)
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
	return operation()
}

func sortedReport(report Report) Report {
	sort.Strings(report.Changed)
	sort.Strings(report.Verified)
	return report
}
