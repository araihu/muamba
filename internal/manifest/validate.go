package manifest

import (
	"fmt"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

var (
	namePattern  = regexp.MustCompile(`^[a-z][a-z0-9]*(?:-[a-z0-9]+)*$`)
	tokenPattern = regexp.MustCompile(`\$\{([^}]+)\}`)
)

func (d *Document) Validate(strict bool) ([]Warning, error) {
	if d.Manifest.Schema != 1 {
		return nil, fmt.Errorf("schema = %d, want 1", d.Manifest.Schema)
	}
	if len(d.Manifest.Resources) == 0 {
		return nil, fmt.Errorf("resources must not be empty")
	}

	resourceNames := sortedResourceNames(d.Manifest.Resources)
	resolved := make(map[string]Selection)
	paths := make(map[string]string)
	var warnings []Warning
	for _, resourceName := range resourceNames {
		resource := d.Manifest.Resources[resourceName]
		if !namePattern.MatchString(resourceName) {
			return nil, fmt.Errorf("invalid resource name %q", resourceName)
		}
		if resource == nil || resource.Version == "" {
			return nil, fmt.Errorf("resource %q version must not be empty", resourceName)
		}
		if len(resource.Downloads) == 0 {
			return nil, fmt.Errorf("resource %q downloads must not be empty", resourceName)
		}
		for _, downloadName := range sortedDownloadNames(resource.Downloads) {
			download := resource.Downloads[downloadName]
			if !namePattern.MatchString(downloadName) {
				return nil, fmt.Errorf("invalid download name %q in resource %q", downloadName, resourceName)
			}
			if download == nil || download.URL == "" || download.Path == "" {
				return nil, fmt.Errorf("resource/download %s/%s requires url and path", resourceName, downloadName)
			}
			url, err := expand(download.URL, resource.Version)
			if err != nil {
				return nil, fmt.Errorf("%s/%s url: %w", resourceName, downloadName, err)
			}
			path, err := expand(download.Path, resource.Version)
			if err != nil {
				return nil, fmt.Errorf("%s/%s path: %w", resourceName, downloadName, err)
			}
			clean := filepath.Clean(path)
			if filepath.IsAbs(path) || clean == "." || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
				return nil, fmt.Errorf("%s/%s path %q must stay relative to manifest", resourceName, downloadName, path)
			}
			id := resourceName + "/" + downloadName
			if previous, exists := paths[clean]; exists {
				return nil, fmt.Errorf("duplicate resolved path %q for %s and %s", clean, previous, id)
			}
			paths[clean] = id
			selection := Selection{resourceName, downloadName, resource.Version, url, clean, download.Integrity}
			resolved[id] = selection
			if !strings.Contains(url, resource.Version) {
				warning := Warning{resourceName, downloadName, fmt.Sprintf("expanded URL %q does not contain version %q", url, resource.Version)}
				if strict {
					return nil, fmt.Errorf("%s/%s: %s", resourceName, downloadName, warning.Message)
				}
				warnings = append(warnings, warning)
			}
		}
	}
	d.resolved = resolved
	return warnings, nil
}

func expand(value, version string) (string, error) {
	var unknown string
	result := tokenPattern.ReplaceAllStringFunc(value, func(token string) string {
		name := tokenPattern.FindStringSubmatch(token)[1]
		if name != "version" {
			unknown = name
			return token
		}
		return version
	})
	if unknown != "" {
		return "", fmt.Errorf("unknown interpolation token ${%s}", unknown)
	}
	if strings.Contains(result, "${") {
		return "", fmt.Errorf("malformed interpolation token")
	}
	return result, nil
}

func sortedResourceNames(resources map[string]*Resource) []string {
	names := make([]string, 0, len(resources))
	for name := range resources {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func sortedDownloadNames(downloads map[string]*Download) []string {
	names := make([]string, 0, len(downloads))
	for name := range downloads {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}
