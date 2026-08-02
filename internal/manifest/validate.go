package manifest

import (
	"fmt"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/araihu/muamba/internal/integrity"
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
	resolved := make(map[string]resolvedDownload)
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
			if download == nil || download.Path == "" {
				return nil, fmt.Errorf("resource/download %s/%s requires path", resourceName, downloadName)
			}
			if download.URL == "" && len(download.Platforms) == 0 {
				return nil, fmt.Errorf("resource/download %s/%s requires url or platforms", resourceName, downloadName)
			}
			if download.URL == "" && download.Integrity != "" {
				return nil, fmt.Errorf("%s/%s integrity requires base url", resourceName, downloadName)
			}
			platform := download.Platform
			if platform == "" {
				platform = "multi"
			}
			if platform != "multi" {
				if _, err := ParseTarget(platform); err != nil {
					return nil, fmt.Errorf("%s/%s platform: %w", resourceName, downloadName, err)
				}
			}
			var maxBytes int64
			if download.MaxSize != "" {
				var err error
				maxBytes, err = parseMaxSize(download.MaxSize)
				if err != nil {
					return nil, fmt.Errorf("%s/%s: %w", resourceName, downloadName, err)
				}
			}
			var url string
			if download.URL != "" {
				var err error
				url, err = expand(download.URL, resource.Version)
				if err != nil {
					return nil, fmt.Errorf("%s/%s url: %w", resourceName, downloadName, err)
				}
				if err := validateIntegrity(download.Integrity); err != nil {
					return nil, fmt.Errorf("%s/%s integrity: %w", resourceName, downloadName, err)
				}
				warning := versionWarning(resourceName, downloadName, "", url, resource.Version)
				if warning != nil {
					if strict {
						return nil, fmt.Errorf("%s/%s: %s", resourceName, downloadName, warning.Message)
					}
					warnings = append(warnings, *warning)
				}
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
			platforms := make(map[string]PlatformDownload, len(download.Platforms))
			for _, target := range sortedPlatformNames(download.Platforms) {
				entry := download.Platforms[target]
				if _, err := ParseTarget(target); err != nil {
					return nil, fmt.Errorf("%s/%s platform %q: %w", resourceName, downloadName, target, err)
				}
				if entry == nil || entry.URL == "" {
					return nil, fmt.Errorf("%s/%s platform %s requires url", resourceName, downloadName, target)
				}
				resolvedURL, err := expand(entry.URL, resource.Version)
				if err != nil {
					return nil, fmt.Errorf("%s/%s platform %s url: %w", resourceName, downloadName, target, err)
				}
				if err := validateIntegrity(entry.Integrity); err != nil {
					return nil, fmt.Errorf("%s/%s platform %s integrity: %w", resourceName, downloadName, target, err)
				}
				warning := versionWarning(resourceName, downloadName, target, resolvedURL, resource.Version)
				if warning != nil {
					if strict {
						return nil, fmt.Errorf("%s/%s: %s", resourceName, downloadName, warning.Message)
					}
					warnings = append(warnings, *warning)
				}
				platforms[target] = PlatformDownload{URL: resolvedURL, Integrity: entry.Integrity}
			}
			resolved[id] = resolvedDownload{
				ResourceName: resourceName,
				DownloadName: downloadName,
				Version:      resource.Version,
				Path:         clean,
				URL:          url,
				Integrity:    download.Integrity,
				Platform:     platform,
				Executable:   download.Executable,
				MaxBytes:     maxBytes,
				Platforms:    platforms,
			}
		}
	}
	d.resolved = resolved
	return warnings, nil
}

func validateIntegrity(value string) error {
	if value == "" {
		return nil
	}
	_, err := integrity.Parse(value)
	return err
}

func versionWarning(resource, download, platform, url, version string) *Warning {
	if strings.Contains(url, version) {
		return nil
	}
	prefix := ""
	if platform != "" {
		prefix = "platform " + platform + " "
	}
	return &Warning{resource, download, fmt.Sprintf("%sexpanded URL %q does not contain version %q", prefix, url, version)}
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

func sortedPlatformNames(platforms map[string]*PlatformDownload) []string {
	names := make([]string, 0, len(platforms))
	for name := range platforms {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}
