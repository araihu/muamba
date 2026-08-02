package lifecycle

import (
	"context"
	"fmt"
	"os"

	"github.com/araihu/muamba/internal/integrity"
	"github.com/araihu/muamba/internal/manifest"
)

func (e *Engine) Verify(_ context.Context, selectors []string, allPlatforms bool) (Report, error) {
	selections, err := e.selections(selectors)
	if err != nil {
		return Report{}, err
	}
	selected := make(map[string]struct{})
	if allPlatforms {
		for _, selection := range selections {
			selected[selectionLabel(selection)] = struct{}{}
		}
		selections, err = e.allSelections(selectors)
		if err != nil {
			return Report{}, err
		}
	}
	report := Report{Warnings: append([]manifest.Warning(nil), e.warnings...)}
	for _, selection := range selections {
		if selection.Integrity == "" {
			return Report{}, fmt.Errorf("%s is unlocked", selectionLabel(selection))
		}
		if allPlatforms {
			digest, parseErr := integrity.Parse(selection.Integrity)
			if parseErr != nil {
				return Report{}, fmt.Errorf("%s: %w", selectionLabel(selection), parseErr)
			}
			if verifyErr := e.cache.Verify(digest); verifyErr != nil {
				return Report{}, fmt.Errorf("%s: %w", selectionLabel(selection), verifyErr)
			}
			if _, materialized := selected[selectionLabel(selection)]; materialized {
				if err := e.verifyFile(selection); err != nil {
					return Report{}, err
				}
			}
		} else if err := e.verifyFile(selection); err != nil {
			return Report{}, err
		}
		report.Verified = append(report.Verified, selectionLabel(selection))
	}
	return sortedReport(report), nil
}

func (e *Engine) verifyFile(selection manifest.Selection) error {
	digest, err := integrity.Parse(selection.Integrity)
	if err != nil {
		return fmt.Errorf("%s: %w", selectionLabel(selection), err)
	}
	target, err := e.target(selection)
	if err != nil {
		return fmt.Errorf("%s: %w", selectionLabel(selection), err)
	}
	file, err := os.Open(target)
	if err != nil {
		return fmt.Errorf("%s at %s: %w", selectionLabel(selection), target, err)
	}
	defer func() { _ = file.Close() }()
	if _, err := integrity.Verify(file, digest); err != nil {
		return fmt.Errorf("%s at %s: %w", selectionLabel(selection), target, err)
	}
	return nil
}
