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
			item, itemWarnings, err := validateDownload(resourceName, downloadName, resource, resource.Downloads[downloadName], strict)
			if err != nil {
				return nil, err
			}
			id := resourceName + "/" + downloadName
			if previous, exists := paths[item.Path]; exists {
				return nil, fmt.Errorf("duplicate resolved path %q for %s and %s", item.Path, previous, id)
			}
			paths[item.Path] = id
			warnings = append(warnings, itemWarnings...)
			resolved[id] = item
		}
	}
	d.resolved = resolved
	return warnings, nil
}

func validateDownload(resourceName, downloadName string, resource *Resource, download *Download, strict bool) (resolvedDownload, []Warning, error) {
	if !namePattern.MatchString(downloadName) {
		return resolvedDownload{}, nil, fmt.Errorf("invalid download name %q in resource %q", downloadName, resourceName)
	}
	if download == nil || download.Path == "" {
		return resolvedDownload{}, nil, fmt.Errorf("resource/download %s/%s requires path", resourceName, downloadName)
	}
	if download.URL == "" && len(download.Platforms) == 0 {
		return resolvedDownload{}, nil, fmt.Errorf("resource/download %s/%s requires url or platforms", resourceName, downloadName)
	}
	if download.URL == "" && download.Integrity != "" {
		return resolvedDownload{}, nil, fmt.Errorf("%s/%s integrity requires base url", resourceName, downloadName)
	}
	if download.URL == "" && download.Platform != "" {
		return resolvedDownload{}, nil, fmt.Errorf("%s/%s platform requires base url", resourceName, downloadName)
	}
	platform, err := validateBasePlatform(resourceName, downloadName, download.Platform)
	if err != nil {
		return resolvedDownload{}, nil, err
	}
	var maxBytes int64
	if download.MaxSize != "" {
		maxBytes, err = parseMaxSize(download.MaxSize)
		if err != nil {
			return resolvedDownload{}, nil, fmt.Errorf("%s/%s: %w", resourceName, downloadName, err)
		}
	}
	url, warnings, err := validateURL(resourceName, downloadName, "", download.URL, download.Integrity, resource.Version, strict)
	if err != nil {
		return resolvedDownload{}, nil, err
	}
	path, err := expand(download.Path, resource.Version)
	if err != nil {
		return resolvedDownload{}, nil, fmt.Errorf("%s/%s path: %w", resourceName, downloadName, err)
	}
	clean := filepath.Clean(path)
	if filepath.IsAbs(path) || clean == "." || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return resolvedDownload{}, nil, fmt.Errorf("%s/%s path %q must stay relative to manifest", resourceName, downloadName, path)
	}
	platforms, platformWarnings, err := validatePlatforms(resourceName, downloadName, resource.Version, download.Platforms, strict)
	if err != nil {
		return resolvedDownload{}, nil, err
	}
	warnings = append(warnings, platformWarnings...)
	return resolvedDownload{
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
	}, warnings, nil
}

func validateBasePlatform(resource, download, platform string) (string, error) {
	if platform == "" {
		return "multi", nil
	}
	if platform == "multi" {
		return platform, nil
	}
	if _, err := ParseTarget(platform); err != nil {
		return "", fmt.Errorf("%s/%s platform: %w", resource, download, err)
	}
	return platform, nil
}

func validateURL(resource, download, platform, rawURL, lock, version string, strict bool) (string, []Warning, error) {
	if rawURL == "" {
		return "", nil, nil
	}
	url, err := expand(rawURL, version)
	if err != nil {
		return "", nil, fmt.Errorf("%s/%s url: %w", resource, download, err)
	}
	if err := validateIntegrity(lock); err != nil {
		return "", nil, fmt.Errorf("%s/%s integrity: %w", resource, download, err)
	}
	warning := versionWarning(resource, download, platform, url, version)
	if warning == nil {
		return url, nil, nil
	}
	if strict {
		return "", nil, fmt.Errorf("%s/%s: %s", resource, download, warning.Message)
	}
	return url, []Warning{*warning}, nil
}

func validatePlatforms(resource, download, version string, declared map[string]*PlatformDownload, strict bool) (map[string]PlatformDownload, []Warning, error) {
	platforms := make(map[string]PlatformDownload, len(declared))
	var warnings []Warning
	for _, target := range sortedPlatformNames(declared) {
		entry := declared[target]
		if _, err := ParseTarget(target); err != nil {
			return nil, nil, fmt.Errorf("%s/%s platform %q: %w", resource, download, target, err)
		}
		if entry == nil || entry.URL == "" {
			return nil, nil, fmt.Errorf("%s/%s platform %s requires url", resource, download, target)
		}
		url, entryWarnings, err := validateURL(resource, download, target, entry.URL, entry.Integrity, version, strict)
		if err != nil {
			return nil, nil, fmt.Errorf("%s/%s platform %s: %w", resource, download, target, err)
		}
		warnings = append(warnings, entryWarnings...)
		platforms[target] = PlatformDownload{URL: url, Integrity: entry.Integrity}
	}
	return platforms, warnings, nil
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
