package safepath

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/araihu/muamba/internal/manifest"
)

func Resolve(root, relative string) (string, error) {
	rootAbs, err := filepath.Abs(root)
	if err != nil {
		return "", err
	}
	rootReal, err := filepath.EvalSymlinks(rootAbs)
	if err != nil {
		return "", fmt.Errorf("resolve root %s: %w", root, err)
	}
	clean := filepath.Clean(relative)
	if relative == "" || clean == "." || filepath.IsAbs(relative) || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("path %q must stay below root", relative)
	}

	current := rootReal
	parts := strings.Split(clean, string(filepath.Separator))
	for index, part := range parts {
		next := filepath.Join(current, part)
		_, statErr := os.Lstat(next)
		if os.IsNotExist(statErr) {
			current = filepath.Join(current, filepath.Join(parts[index:]...))
			break
		}
		if statErr != nil {
			return "", statErr
		}
		resolved, resolveErr := filepath.EvalSymlinks(next)
		if resolveErr != nil {
			return "", resolveErr
		}
		if err := ensureWithin(rootReal, resolved); err != nil {
			return "", err
		}
		current = resolved
	}
	if err := ensureWithin(rootReal, current); err != nil {
		return "", err
	}
	return current, nil
}

func ValidateUnique(root string, selections []manifest.Selection) error {
	seen := make(map[string]string)
	for _, selection := range selections {
		resolved, err := Resolve(root, selection.Path)
		if err != nil {
			return fmt.Errorf("%s/%s: %w", selection.ResourceName, selection.DownloadName, err)
		}
		id := selection.ResourceName + "/" + selection.DownloadName
		if previous, ok := seen[resolved]; ok {
			return fmt.Errorf("duplicate destination %q for %s and %s", resolved, previous, id)
		}
		seen[resolved] = id
	}
	return nil
}

func ensureWithin(root, path string) error {
	relative, err := filepath.Rel(root, path)
	if err != nil {
		return err
	}
	if relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return fmt.Errorf("resolved path %q escapes root %q", path, root)
	}
	return nil
}
