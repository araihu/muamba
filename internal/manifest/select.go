package manifest

import (
	"fmt"
	"sort"
	"strings"
)

func (d *Document) Select(selectors []string) ([]Selection, error) {
	return d.SelectTarget(selectors, RuntimeTarget())
}

func (d *Document) SelectTarget(selectors []string, target Target) ([]Selection, error) {
	if target == (Target{}) {
		target = RuntimeTarget()
	}
	if _, err := ParseTarget(target.String()); err != nil {
		return nil, err
	}
	items, err := d.selectedDownloads(selectors)
	if err != nil {
		return nil, err
	}
	result := make([]Selection, 0, len(items))
	for _, item := range items {
		platform := target.String()
		if entry, ok := item.Platforms[platform]; ok {
			result = append(result, item.selection(entry.URL, entry.Integrity, platform, platform))
			continue
		}
		if item.URL != "" && (item.Platform == "multi" || item.Platform == platform) {
			result = append(result, item.selection(item.URL, item.Integrity, "", platform))
			continue
		}
		return nil, fmt.Errorf("%s/%s does not support target %s", item.ResourceName, item.DownloadName, platform)
	}
	return result, nil
}

func (d *Document) SelectAll(selectors []string) ([]Selection, error) {
	items, err := d.selectedDownloads(selectors)
	if err != nil {
		return nil, err
	}
	var result []Selection
	for _, item := range items {
		if item.URL != "" {
			result = append(result, item.selection(item.URL, item.Integrity, "", item.Platform))
		}
		platforms := make([]string, 0, len(item.Platforms))
		for platform := range item.Platforms {
			platforms = append(platforms, platform)
		}
		sort.Strings(platforms)
		for _, platform := range platforms {
			entry := item.Platforms[platform]
			result = append(result, item.selection(entry.URL, entry.Integrity, platform, platform))
		}
	}
	return result, nil
}

func (d *Document) selectedDownloads(selectors []string) ([]resolvedDownload, error) {
	if d.resolved == nil {
		if _, err := d.Validate(false); err != nil {
			return nil, err
		}
	}
	selected := make(map[string]resolvedDownload)
	if len(selectors) == 0 {
		for id, item := range d.resolved {
			selected[id] = item
		}
	} else {
		for _, selector := range selectors {
			parts := strings.Split(selector, "/")
			if len(parts) > 2 || parts[0] == "" || len(parts) == 2 && parts[1] == "" {
				return nil, fmt.Errorf("invalid selector %q", selector)
			}
			resource, ok := d.Manifest.Resources[parts[0]]
			if !ok {
				return nil, fmt.Errorf("unknown resource %q", parts[0])
			}
			if len(parts) == 2 {
				id := parts[0] + "/" + parts[1]
				item, ok := d.resolved[id]
				if !ok {
					if _, directory := resource.Directories[parts[1]]; directory {
						continue
					}
					return nil, fmt.Errorf("unknown download %q in resource %q", parts[1], parts[0])
				}
				selected[id] = item
				continue
			}
			for downloadName := range resource.Downloads {
				id := parts[0] + "/" + downloadName
				selected[id] = d.resolved[id]
			}
		}
	}
	ids := make([]string, 0, len(selected))
	for id := range selected {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	result := make([]resolvedDownload, 0, len(ids))
	for _, id := range ids {
		result = append(result, selected[id])
	}
	return result, nil
}

func (d *Document) SelectDirectories(selectors []string) ([]DirectorySelection, error) {
	if d.resolved == nil {
		if _, err := d.Validate(false); err != nil {
			return nil, err
		}
	}
	selected := make(map[string]DirectorySelection)
	addResource := func(resourceName string, resource *Resource) {
		for _, directoryName := range sortedDirectoryNames(resource.Directories) {
			directory := resource.Directories[directoryName]
			selection := directorySelection(resourceName, directoryName, resource.Version, directory)
			if locked, ok := d.directoryLocks[selection.ID()]; ok {
				copy := cloneLockedDirectory(locked)
				selection.Lock = &copy
			}
			selected[selection.ID()] = selection
		}
	}
	if len(selectors) == 0 {
		for _, resourceName := range sortedResourceNames(d.Manifest.Resources) {
			addResource(resourceName, d.Manifest.Resources[resourceName])
		}
	} else {
		for _, selector := range selectors {
			parts := strings.Split(selector, "/")
			if len(parts) > 2 || parts[0] == "" || len(parts) == 2 && parts[1] == "" {
				return nil, fmt.Errorf("invalid selector %q", selector)
			}
			resource, ok := d.Manifest.Resources[parts[0]]
			if !ok {
				return nil, fmt.Errorf("unknown resource %q", parts[0])
			}
			if len(parts) == 1 {
				addResource(parts[0], resource)
				continue
			}
			directory, ok := resource.Directories[parts[1]]
			if !ok {
				if _, download := resource.Downloads[parts[1]]; download {
					continue
				}
				return nil, fmt.Errorf("unknown download or directory %q in resource %q", parts[1], parts[0])
			}
			selection := directorySelection(parts[0], parts[1], resource.Version, directory)
			if locked, ok := d.directoryLocks[selection.ID()]; ok {
				copy := cloneLockedDirectory(locked)
				selection.Lock = &copy
			}
			selected[selection.ID()] = selection
		}
	}
	ids := make([]string, 0, len(selected))
	for id := range selected {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	result := make([]DirectorySelection, 0, len(ids))
	for _, id := range ids {
		result = append(result, selected[id])
	}
	return result, nil
}

func directorySelection(resourceName, directoryName, version string, directory *Directory) DirectorySelection {
	return DirectorySelection{
		ResourceName: resourceName, DirectoryName: directoryName, Version: version,
		URL: directory.URL, Archive: directory.Archive, Path: directory.Path,
		Include: append([]string(nil), directory.Include...), Exclude: append([]string(nil), directory.Exclude...),
		StripComponents: directory.StripComponents, MaxBytes: directory.resolvedMaxBytes,
		MaxFiles: directory.MaxFiles, MaxUnpackedBytes: directory.resolvedUnpacked,
	}
}

func (item resolvedDownload) selection(url, lock, variant, platform string) Selection {
	size := item.Size
	if variant != "" {
		size = item.Platforms[variant].Size
	}
	return Selection{
		ResourceName: item.ResourceName,
		DownloadName: item.DownloadName,
		Version:      item.Version,
		URL:          url,
		Path:         item.Path,
		Integrity:    lock,
		Variant:      variant,
		Platform:     platform,
		Executable:   item.Executable,
		MaxBytes:     item.MaxBytes,
		Size:         size,
	}
}

func (selection Selection) ID() string {
	id := selection.ResourceName + "/" + selection.DownloadName
	if selection.Variant != "" {
		return id + "[" + selection.Variant + "]"
	}
	return id
}
