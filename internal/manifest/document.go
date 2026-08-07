package manifest

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"go.yaml.in/yaml/v3"
)

type Document struct {
	Path           string
	LockPath       string
	Dir            string
	Manifest       Manifest
	root           yaml.Node
	resolved       map[string]resolvedDownload
	locks          map[string]LockedFile
	directoryLocks map[string]LockedDirectory
	legacy         bool
}

func Find(startDir, explicit string) (string, error) {
	if explicit != "" {
		path := explicit
		if !filepath.IsAbs(path) {
			path = filepath.Join(startDir, path)
		}
		path, err := filepath.Abs(path)
		if err != nil {
			return "", err
		}
		if _, err := os.Stat(path); err != nil {
			return "", fmt.Errorf("manifest %s: %w", path, err)
		}
		return path, nil
	}

	dir, err := filepath.Abs(startDir)
	if err != nil {
		return "", err
	}
	for {
		for _, name := range []string{".muamba.yaml", "muamba.yaml"} {
			candidate := filepath.Join(dir, name)
			if info, statErr := os.Stat(candidate); statErr == nil && !info.IsDir() {
				return candidate, nil
			}
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", fmt.Errorf(".muamba.yaml or legacy muamba.yaml not found from %s", startDir)
		}
		dir = parent
	}
}

func Load(path string) (*Document, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}
	b, err := os.ReadFile(abs)
	if err != nil {
		return nil, fmt.Errorf("read manifest %s: %w", abs, err)
	}
	document, err := loadBytes(abs, b)
	if err != nil {
		return nil, err
	}
	if filepath.Base(abs) != ".muamba.yaml" {
		document.legacy = true
		document.markLegacySizes()
		return document, nil
	}
	document.LockPath = filepath.Join(filepath.Dir(abs), ".muamba.lock.yaml")
	if err := document.rejectInlineLocks(); err != nil {
		return nil, err
	}
	if err := document.loadLock(); err != nil {
		return nil, err
	}
	return document, nil
}

func (d *Document) markLegacySizes() {
	for _, resource := range d.Manifest.Resources {
		for _, download := range resource.Downloads {
			if download != nil && download.Integrity != "" {
				download.Size = -1
			}
			for _, platform := range download.Platforms {
				if platform != nil && platform.Integrity != "" {
					platform.Size = -1
				}
			}
		}
	}
}

func loadBytes(path string, b []byte) (*Document, error) {
	var typed Manifest
	decoder := yaml.NewDecoder(bytes.NewReader(b))
	decoder.KnownFields(true)
	if err := decoder.Decode(&typed); err != nil {
		return nil, fmt.Errorf("parse manifest %s: %w", path, err)
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		if err == nil {
			return nil, fmt.Errorf("parse manifest %s: multiple YAML documents", path)
		}
		return nil, fmt.Errorf("parse manifest %s: %w", path, err)
	}
	var root yaml.Node
	if err := yaml.Unmarshal(b, &root); err != nil {
		return nil, fmt.Errorf("parse manifest nodes %s: %w", path, err)
	}
	return &Document{
		Path: path, Dir: filepath.Dir(path), Manifest: typed, root: root,
		locks: make(map[string]LockedFile), directoryLocks: make(map[string]LockedDirectory),
	}, nil
}

func (d *Document) Clone() (*Document, error) {
	b, err := d.Marshal()
	if err != nil {
		return nil, err
	}
	clone, err := loadBytes(d.Path, b)
	if err != nil {
		return nil, err
	}
	clone.LockPath, clone.legacy = d.LockPath, d.legacy
	clone.locks = make(map[string]LockedFile, len(d.locks))
	for id, locked := range d.locks {
		clone.locks[id] = locked
		if err := clone.applyLock(locked); err != nil {
			return nil, err
		}
	}
	clone.directoryLocks = make(map[string]LockedDirectory, len(d.directoryLocks))
	for id, locked := range d.directoryLocks {
		clone.directoryLocks[id] = cloneLockedDirectory(locked)
	}
	return clone, nil
}

func (d *Document) Marshal() ([]byte, error) {
	var out bytes.Buffer
	encoder := yaml.NewEncoder(&out)
	encoder.SetIndent(2)
	if err := encoder.Encode(&d.root); err != nil {
		return nil, err
	}
	if err := encoder.Close(); err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}

func (d *Document) MarshalLock() ([]byte, error) {
	files := make([]LockedFile, 0, len(d.locks))
	for _, locked := range d.locks {
		files = append(files, locked)
	}
	sort.Slice(files, func(i, j int) bool { return files[i].ID < files[j].ID })
	var out bytes.Buffer
	encoder := yaml.NewEncoder(&out)
	encoder.SetIndent(2)
	directories := make([]LockedDirectory, 0, len(d.directoryLocks))
	for _, locked := range d.directoryLocks {
		locked = cloneLockedDirectory(locked)
		sort.Slice(locked.Files, func(i, j int) bool { return locked.Files[i].Path < locked.Files[j].Path })
		directories = append(directories, locked)
	}
	sort.Slice(directories, func(i, j int) bool { return directories[i].ID < directories[j].ID })
	if err := encoder.Encode(Lock{Schema: 1, Files: files, Directories: directories}); err != nil {
		return nil, err
	}
	if err := encoder.Close(); err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}

func (d *Document) SetDirectoryLock(locked LockedDirectory) error {
	if locked.ID == "" {
		return fmt.Errorf("directory lock id must not be empty")
	}
	if d.directoryLocks == nil {
		d.directoryLocks = make(map[string]LockedDirectory)
	}
	d.directoryLocks[locked.ID] = cloneLockedDirectory(locked)
	return nil
}

func (d *Document) IsLegacy() bool { return d.legacy }

func (d *Document) SetLock(selection Selection, size int64, value string) error {
	if size < 0 {
		return fmt.Errorf("lock size must not be negative")
	}
	if err := d.setTypedIntegrity(selection, value); err != nil {
		return err
	}
	if d.locks == nil {
		d.locks = make(map[string]LockedFile)
	}
	d.locks[selection.ID()] = LockedFile{
		ID: selection.ID(), URL: selection.URL, Path: selection.Path, Size: size, Integrity: value,
	}
	d.resolved = nil
	return nil
}

func (d *Document) SetVersion(resource, value string) error {
	r, ok := d.Manifest.Resources[resource]
	if !ok {
		return fmt.Errorf("unknown resource %q", resource)
	}
	node, err := d.resourceNode(resource)
	if err != nil {
		return err
	}
	valueNode := mappingValue(node, "version")
	if valueNode == nil {
		return fmt.Errorf("resource %q has no version node", resource)
	}
	valueNode.Value, valueNode.Tag = value, "!!str"
	r.Version = value
	d.resolved = nil
	return nil
}

func (d *Document) ClearResourceLocks(resourceName string) error {
	resource, ok := d.Manifest.Resources[resourceName]
	if !ok {
		return fmt.Errorf("unknown resource %q", resourceName)
	}
	for _, download := range resource.Downloads {
		download.Integrity, download.Size = "", 0
		for _, platform := range download.Platforms {
			if platform != nil {
				platform.Integrity, platform.Size = "", 0
			}
		}
	}
	prefix := resourceName + "/"
	for id := range d.locks {
		if strings.HasPrefix(id, prefix) {
			delete(d.locks, id)
		}
	}
	for id := range d.directoryLocks {
		if strings.HasPrefix(id, prefix) {
			delete(d.directoryLocks, id)
		}
	}
	d.resolved = nil
	return nil
}

func (d *Document) SetIntegrity(selection Selection, value string) error {
	if !d.legacy {
		return d.SetLock(selection, selection.Size, value)
	}
	if err := d.setTypedIntegrity(selection, value); err != nil {
		return err
	}
	resource := selection.ResourceName
	download := selection.DownloadName
	r := d.Manifest.Resources[resource]
	item := r.Downloads[download]
	rnode, err := d.resourceNode(resource)
	if err != nil {
		return err
	}
	downloads := mappingValue(rnode, "downloads")
	dnode := mappingValue(downloads, download)
	if dnode == nil {
		return fmt.Errorf("download node %q not found", download)
	}
	integrityNode := dnode
	if selection.Variant != "" {
		variant, ok := item.Platforms[selection.Variant]
		if !ok || variant == nil {
			return fmt.Errorf("unknown platform %q in %s/%s", selection.Variant, resource, download)
		}
		platforms := mappingValue(dnode, "platforms")
		integrityNode = mappingValue(platforms, selection.Variant)
		if integrityNode == nil {
			return fmt.Errorf("platform node %q not found in %s/%s", selection.Variant, resource, download)
		}
	}
	valueNode := mappingValue(integrityNode, "integrity")
	if valueNode == nil {
		integrityNode.Content = append(integrityNode.Content,
			&yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: "integrity"},
			&yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: value},
		)
	} else {
		valueNode.Value, valueNode.Tag = value, "!!str"
	}
	return nil
}

func (d *Document) setTypedIntegrity(selection Selection, value string) error {
	resource, ok := d.Manifest.Resources[selection.ResourceName]
	if !ok {
		return fmt.Errorf("unknown resource %q", selection.ResourceName)
	}
	download, ok := resource.Downloads[selection.DownloadName]
	if !ok {
		return fmt.Errorf("unknown download %q in resource %q", selection.DownloadName, selection.ResourceName)
	}
	if selection.Variant == "" {
		download.Integrity, download.Size = value, selection.Size
	} else {
		platform, ok := download.Platforms[selection.Variant]
		if !ok || platform == nil {
			return fmt.Errorf("unknown platform %q in %s/%s", selection.Variant, selection.ResourceName, selection.DownloadName)
		}
		platform.Integrity, platform.Size = value, selection.Size
	}
	d.resolved = nil
	return nil
}

func (d *Document) rejectInlineLocks() error {
	for resourceName, resource := range d.Manifest.Resources {
		for downloadName, download := range resource.Downloads {
			if download.Integrity != "" {
				return fmt.Errorf("%s/%s: integrity belongs in .muamba.lock.yaml", resourceName, downloadName)
			}
			for platform, entry := range download.Platforms {
				if entry != nil && entry.Integrity != "" {
					return fmt.Errorf("%s/%s[%s]: integrity belongs in .muamba.lock.yaml", resourceName, downloadName, platform)
				}
			}
		}
	}
	return nil
}

func (d *Document) loadLock() error {
	b, err := os.ReadFile(d.LockPath)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("read lock %s: %w", d.LockPath, err)
	}
	var lock Lock
	decoder := yaml.NewDecoder(bytes.NewReader(b))
	decoder.KnownFields(true)
	if err := decoder.Decode(&lock); err != nil {
		return fmt.Errorf("parse lock %s: %w", d.LockPath, err)
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		if err == nil {
			return fmt.Errorf("parse lock %s: multiple YAML documents", d.LockPath)
		}
		return fmt.Errorf("parse lock %s: %w", d.LockPath, err)
	}
	if lock.Schema != 1 {
		return fmt.Errorf("lock schema = %d, want 1", lock.Schema)
	}
	for _, locked := range lock.Files {
		if locked.ID == "" {
			return fmt.Errorf("parse lock %s: file id must not be empty", d.LockPath)
		}
		if _, duplicate := d.locks[locked.ID]; duplicate {
			return fmt.Errorf("parse lock %s: duplicate file id %q", d.LockPath, locked.ID)
		}
		if locked.Size < 0 {
			return fmt.Errorf("parse lock %s: %s size must not be negative", d.LockPath, locked.ID)
		}
		d.locks[locked.ID] = locked
		if err := d.applyLock(locked); err != nil {
			return fmt.Errorf("parse lock %s: %w", d.LockPath, err)
		}
	}
	for _, locked := range lock.Directories {
		if locked.ID == "" {
			return fmt.Errorf("parse lock %s: directory id must not be empty", d.LockPath)
		}
		if _, duplicate := d.directoryLocks[locked.ID]; duplicate {
			return fmt.Errorf("parse lock %s: duplicate directory id %q", d.LockPath, locked.ID)
		}
		d.directoryLocks[locked.ID] = cloneLockedDirectory(locked)
	}
	return nil
}

func cloneLockedDirectory(locked LockedDirectory) LockedDirectory {
	locked.Files = append([]LockedDirectoryFile(nil), locked.Files...)
	return locked
}

func (d *Document) applyLock(locked LockedFile) error {
	selection, err := parseSelectionID(locked.ID)
	if err != nil {
		return err
	}
	selection.Size = locked.Size
	return d.setTypedIntegrity(selection, locked.Integrity)
}

func parseSelectionID(id string) (Selection, error) {
	variant := ""
	base := id
	if index := strings.LastIndex(id, "["); index >= 0 && strings.HasSuffix(id, "]") {
		base, variant = id[:index], id[index+1:len(id)-1]
	}
	parts := strings.Split(base, "/")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return Selection{}, fmt.Errorf("invalid lock file id %q", id)
	}
	return Selection{ResourceName: parts[0], DownloadName: parts[1], Variant: variant}, nil
}

func (d *Document) resourceNode(name string) (*yaml.Node, error) {
	if len(d.root.Content) == 0 {
		return nil, errors.New("empty YAML document")
	}
	resources := mappingValue(d.root.Content[0], "resources")
	if resources == nil {
		return nil, errors.New("resources node missing")
	}
	node := mappingValue(resources, name)
	if node == nil {
		return nil, fmt.Errorf("resource node %q missing", name)
	}
	return node, nil
}

func mappingValue(mapping *yaml.Node, key string) *yaml.Node {
	if mapping == nil || mapping.Kind != yaml.MappingNode {
		return nil
	}
	for index := 0; index+1 < len(mapping.Content); index += 2 {
		if mapping.Content[index].Value == key {
			return mapping.Content[index+1]
		}
	}
	return nil
}
