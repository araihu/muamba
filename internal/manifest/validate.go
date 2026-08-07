package manifest

import (
	"fmt"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	archivepkg "github.com/araihu/muamba/internal/archive"
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
		if len(resource.Downloads) == 0 && len(resource.Directories) == 0 {
			return nil, fmt.Errorf("resource %q downloads and directories must not both be empty", resourceName)
		}
		if d.legacy && len(resource.Directories) != 0 {
			return nil, fmt.Errorf("resource %q directories require .muamba.yaml and .muamba.lock.yaml", resourceName)
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
		for _, directoryName := range sortedDirectoryNames(resource.Directories) {
			if _, conflict := resource.Downloads[directoryName]; conflict {
				return nil, fmt.Errorf("resource %q uses %q as both download and directory", resourceName, directoryName)
			}
			directoryWarnings, err := validateDirectory(resourceName, directoryName, resource.Version, resource.Directories[directoryName], strict)
			if err != nil {
				return nil, err
			}
			warnings = append(warnings, directoryWarnings...)
		}
	}
	d.resolved = resolved
	if err := d.validateLocks(); err != nil {
		return nil, err
	}
	return warnings, nil
}

func validateDirectory(resourceName, directoryName, version string, directory *Directory, strict bool) ([]Warning, error) {
	if !namePattern.MatchString(directoryName) {
		return nil, fmt.Errorf("invalid directory name %q in resource %q", directoryName, resourceName)
	}
	if directory == nil || directory.URL == "" || directory.Path == "" {
		return nil, fmt.Errorf("resource/directory %s/%s requires url and path", resourceName, directoryName)
	}
	if directory.Archive != "tar.gz" {
		return nil, fmt.Errorf("%s/%s archive %q is unsupported; want tar.gz", resourceName, directoryName, directory.Archive)
	}
	if len(directory.Include) == 0 {
		return nil, fmt.Errorf("%s/%s requires at least one include glob", resourceName, directoryName)
	}
	if directory.StripComponents < 0 {
		return nil, fmt.Errorf("%s/%s strip_components must not be negative", resourceName, directoryName)
	}
	if directory.MaxFiles <= 0 {
		return nil, fmt.Errorf("%s/%s max_files must be positive", resourceName, directoryName)
	}
	maxBytes, err := parseMaxSize(directory.MaxSize)
	if err != nil {
		return nil, fmt.Errorf("%s/%s: %w", resourceName, directoryName, err)
	}
	maxUnpacked, err := parseMaxSize(directory.MaxUnpackedSize)
	if err != nil {
		return nil, fmt.Errorf("%s/%s max_unpacked_size: %w", resourceName, directoryName, err)
	}
	if maxUnpacked <= 0 {
		return nil, fmt.Errorf("%s/%s max_unpacked_size must be positive", resourceName, directoryName)
	}
	directory.resolvedMaxBytes, directory.resolvedUnpacked = maxBytes, maxUnpacked
	url, warnings, err := validateURL(resourceName, directoryName, "", directory.URL, "", version, strict)
	if err != nil {
		return nil, err
	}
	directory.URL = url
	expandedPath, err := expand(directory.Path, version)
	if err != nil {
		return nil, fmt.Errorf("%s/%s path: %w", resourceName, directoryName, err)
	}
	clean := filepath.Clean(expandedPath)
	if filepath.IsAbs(expandedPath) || clean == "." || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return nil, fmt.Errorf("%s/%s path %q must stay relative to manifest", resourceName, directoryName, expandedPath)
	}
	directory.Path = clean
	for _, pattern := range append(append([]string(nil), directory.Include...), directory.Exclude...) {
		if pattern == "" || strings.HasPrefix(pattern, "/") || strings.Contains(pattern, "\\") || path.Clean(pattern) == ".." || strings.HasPrefix(path.Clean(pattern), "../") {
			return nil, fmt.Errorf("%s/%s has unsafe glob %q", resourceName, directoryName, pattern)
		}
	}
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
		Size:         download.Size,
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
		platforms[target] = PlatformDownload{URL: url, Integrity: entry.Integrity, Size: entry.Size}
	}
	return platforms, warnings, nil
}

func (d *Document) validateLocks() error {
	if d.legacy {
		return nil
	}
	paths, err := d.validateFileLocks()
	if err != nil {
		return err
	}
	return d.validateDirectoryLocks(paths)
}

func (d *Document) validateFileLocks() (map[string]string, error) {
	resolved := make(map[string]Selection)
	lockedPaths := make(map[string]string)
	for _, item := range d.resolved {
		if item.URL != "" {
			selection := item.selection(item.URL, item.Integrity, "", item.Platform)
			resolved[selection.ID()] = selection
			lockedPaths[filepath.ToSlash(selection.Path)] = selection.ID()
		}
		for platform, entry := range item.Platforms {
			selection := item.selection(entry.URL, entry.Integrity, platform, platform)
			resolved[selection.ID()] = selection
			lockedPaths[filepath.ToSlash(selection.Path)] = selection.ID()
		}
	}
	for id, locked := range d.locks {
		selection, ok := resolved[id]
		if !ok {
			return nil, fmt.Errorf("lock file %s has no declaration", id)
		}
		if locked.URL != selection.URL {
			return nil, fmt.Errorf("%s lock URL %q does not match declaration %q", id, locked.URL, selection.URL)
		}
		if locked.Path != selection.Path {
			return nil, fmt.Errorf("%s lock path %q does not match declaration %q", id, locked.Path, selection.Path)
		}
		if locked.Integrity == "" {
			return nil, fmt.Errorf("%s lock integrity must not be empty", id)
		}
	}
	return lockedPaths, nil
}

func (d *Document) validateDirectoryLocks(lockedPaths map[string]string) error {
	declaredDirectories := make(map[string]DirectorySelection)
	for resourceName, resource := range d.Manifest.Resources {
		for directoryName, directory := range resource.Directories {
			selection := directorySelection(resourceName, directoryName, resource.Version, directory)
			declaredDirectories[selection.ID()] = selection
		}
	}
	for id, locked := range d.directoryLocks {
		declaration, ok := declaredDirectories[id]
		if !ok {
			return fmt.Errorf("lock directory %s has no declaration", id)
		}
		if err := validateDirectoryLock(id, declaration, locked, lockedPaths); err != nil {
			return err
		}
	}
	return nil
}

func validateDirectoryLock(id string, declaration DirectorySelection, locked LockedDirectory, lockedPaths map[string]string) error {
	if locked.URL != declaration.URL || locked.Path != filepath.ToSlash(declaration.Path) {
		return fmt.Errorf("%s lock source does not match declaration", id)
	}
	if locked.Size < 0 || locked.Integrity == "" {
		return fmt.Errorf("%s archive lock requires non-negative size and integrity", id)
	}
	if err := validateIntegrity(locked.Integrity); err != nil {
		return fmt.Errorf("%s archive integrity: %w", id, err)
	}
	if len(locked.Files) == 0 {
		return fmt.Errorf("%s lock has no files", id)
	}
	seen := make(map[string]struct{}, len(locked.Files))
	previous := ""
	for _, file := range locked.Files {
		if err := validateDirectoryLockFile(id, declaration, file, previous, seen, lockedPaths); err != nil {
			return err
		}
		seen[file.Path], previous = struct{}{}, file.Path
	}
	return nil
}

func validateDirectoryLockFile(id string, declaration DirectorySelection, file LockedDirectoryFile, previous string, seen map[string]struct{}, lockedPaths map[string]string) error {
	cleanSource := path.Clean(file.Source)
	if file.Source == "" || cleanSource == "." || cleanSource == ".." || strings.HasPrefix(cleanSource, "../") || strings.HasPrefix(file.Source, "/") || strings.Contains(file.Source, "\\") {
		return fmt.Errorf("%s lock has unsafe source path %q", id, file.Source)
	}
	expectedPath := filepath.ToSlash(filepath.Join(declaration.Path, filepath.FromSlash(cleanSource)))
	if file.Path != expectedPath || file.Size < 0 || file.Integrity == "" {
		return fmt.Errorf("%s lock file %q has invalid path, size, or integrity", id, file.Path)
	}
	if err := validateIntegrity(file.Integrity); err != nil {
		return fmt.Errorf("%s lock file %q integrity: %w", id, file.Path, err)
	}
	included, err := archivepkg.Match(declaration.Include, cleanSource)
	if err != nil {
		return fmt.Errorf("%s include glob: %w", id, err)
	}
	excluded, err := archivepkg.Match(declaration.Exclude, cleanSource)
	if err != nil {
		return fmt.Errorf("%s exclude glob: %w", id, err)
	}
	if !included || excluded {
		return fmt.Errorf("%s lock file %q no longer matches declaration globs", id, cleanSource)
	}
	if owner, duplicate := lockedPaths[file.Path]; duplicate {
		return fmt.Errorf("duplicate resolved path %q for %s and %s", file.Path, owner, id)
	}
	if _, duplicate := seen[file.Path]; duplicate {
		return fmt.Errorf("%s lock has duplicate file path %q", id, file.Path)
	}
	if previous != "" && file.Path < previous {
		return fmt.Errorf("%s lock files are not sorted by path", id)
	}
	lockedPaths[file.Path] = id
	return nil
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

func sortedDirectoryNames(directories map[string]*Directory) []string {
	names := make([]string, 0, len(directories))
	for name := range directories {
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
