package lifecycle

import (
	"context"
	"fmt"
	"os"

	"github.com/araihu/muamba/internal/integrity"
	"github.com/araihu/muamba/internal/manifest"
)

func (e *Engine) Verify(_ context.Context, selectors []string) (Report, error) {
	selections, err := e.selections(selectors)
	if err != nil {
		return Report{}, err
	}
	report := Report{Warnings: append([]manifest.Warning(nil), e.warnings...)}
	for _, selection := range selections {
		if selection.Integrity == "" {
			return Report{}, fmt.Errorf("%s/%s is unlocked", selection.ResourceName, selection.DownloadName)
		}
		if err := e.verifyFile(selection); err != nil {
			return Report{}, err
		}
		report.Verified = append(report.Verified, selection.ResourceName+"/"+selection.DownloadName)
	}
	return sortedReport(report), nil
}

func (e *Engine) verifyFile(selection manifest.Selection) error {
	digest, err := integrity.Parse(selection.Integrity)
	if err != nil {
		return fmt.Errorf("%s/%s: %w", selection.ResourceName, selection.DownloadName, err)
	}
	target, err := e.target(selection)
	if err != nil {
		return fmt.Errorf("%s/%s: %w", selection.ResourceName, selection.DownloadName, err)
	}
	file, err := os.Open(target)
	if err != nil {
		return fmt.Errorf("%s/%s at %s: %w", selection.ResourceName, selection.DownloadName, target, err)
	}
	defer func() { _ = file.Close() }()
	if _, err := integrity.Verify(file, digest); err != nil {
		return fmt.Errorf("%s/%s at %s: %w", selection.ResourceName, selection.DownloadName, target, err)
	}
	return nil
}
