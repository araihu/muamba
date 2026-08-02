package manifest

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"go.yaml.in/yaml/v3"
)

type Document struct {
	Path     string
	Dir      string
	Manifest Manifest
	root     yaml.Node
	resolved map[string]resolvedDownload
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
		candidate := filepath.Join(dir, "muamba.yaml")
		if info, statErr := os.Stat(candidate); statErr == nil && !info.IsDir() {
			return candidate, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", fmt.Errorf("muamba.yaml not found from %s", startDir)
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
	return loadBytes(abs, b)
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
	return &Document{Path: path, Dir: filepath.Dir(path), Manifest: typed, root: root}, nil
}

func (d *Document) Clone() (*Document, error) {
	b, err := d.Marshal()
	if err != nil {
		return nil, err
	}
	return loadBytes(d.Path, b)
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

func (d *Document) SetIntegrity(selection Selection, value string) error {
	resource := selection.ResourceName
	download := selection.DownloadName
	r, ok := d.Manifest.Resources[resource]
	if !ok {
		return fmt.Errorf("unknown resource %q", resource)
	}
	item, ok := r.Downloads[download]
	if !ok {
		return fmt.Errorf("unknown download %q in resource %q", download, resource)
	}
	rnode, err := d.resourceNode(resource)
	if err != nil {
		return err
	}
	downloads := mappingValue(rnode, "downloads")
	dnode := mappingValue(downloads, download)
	if dnode == nil {
		return fmt.Errorf("download node %q not found", download)
	}
	typedIntegrity := &item.Integrity
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
		typedIntegrity = &variant.Integrity
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
	*typedIntegrity = value
	d.resolved = nil
	return nil
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
