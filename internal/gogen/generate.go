package gogen

import (
	"bytes"
	"fmt"
	"go/format"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/araihu/muamba/internal/integrity"
	"github.com/araihu/muamba/internal/manifest"
	"github.com/araihu/muamba/internal/safepath"
)

type Options struct {
	Dir     string
	Output  string
	Package string
	Check   bool
	Strict  bool
	Target  manifest.Target
}

type embeddedDownload struct {
	selection manifest.Selection
	embedPath string
	hash      string
}

func Generate(document *manifest.Document, options Options) error {
	if _, err := document.Validate(options.Strict); err != nil {
		return err
	}
	if options.Dir == "" {
		return fmt.Errorf("generation directory is required")
	}
	if options.Output == "" || filepath.Base(options.Output) != options.Output {
		return fmt.Errorf("output must be a filename inside generation directory")
	}
	targetDir, err := generationDir(document.Dir, options.Dir)
	if err != nil {
		return err
	}
	packageName := options.Package
	if packageName == "" {
		packageName, err = inferPackage(targetDir, options.Output)
		if err != nil {
			return err
		}
	}
	target := options.Target
	if target == (manifest.Target{}) {
		target = manifest.RuntimeTarget()
	}
	selections, err := selectionsForDirectory(document, targetDir, target)
	if err != nil {
		return err
	}
	var embedded []embeddedDownload
	for _, selection := range selections {
		fullPath, resolveErr := safepath.Resolve(document.Dir, selection.Path)
		if resolveErr != nil {
			return resolveErr
		}
		relative, relativeErr := filepath.Rel(targetDir, fullPath)
		if relativeErr != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
			continue
		}
		if selection.Integrity == "" {
			return fmt.Errorf("%s/%s is unlocked", selection.ResourceName, selection.DownloadName)
		}
		digest, parseErr := integrity.Parse(selection.Integrity)
		if parseErr != nil {
			return parseErr
		}
		file, openErr := os.Open(fullPath)
		if openErr != nil {
			return fmt.Errorf("%s/%s: %w", selection.ResourceName, selection.DownloadName, openErr)
		}
		_, verifyErr := integrity.Verify(file, digest)
		_ = file.Close()
		if verifyErr != nil {
			return fmt.Errorf("%s/%s: %w", selection.ResourceName, selection.DownloadName, verifyErr)
		}
		embedded = append(embedded, embeddedDownload{
			selection: selection,
			embedPath: filepath.ToSlash(relative),
			hash:      integrity.FormatHash(digest),
		})
	}
	if len(embedded) == 0 {
		return fmt.Errorf("no downloads found below package directory %s", options.Dir)
	}
	source, err := render(packageName, embedded)
	if err != nil {
		return err
	}
	outputPath := filepath.Join(targetDir, options.Output)
	if options.Check {
		existing, readErr := os.ReadFile(outputPath)
		if readErr != nil || !bytes.Equal(existing, source) {
			return fmt.Errorf("generated Go file %s is stale", outputPath)
		}
		return nil
	}
	return writeAtomic(outputPath, source)
}

func selectionsForDirectory(document *manifest.Document, targetDir string, target manifest.Target) ([]manifest.Selection, error) {
	allSelections, err := document.SelectAll(nil)
	if err != nil {
		return nil, err
	}
	selectorSet := make(map[string]struct{})
	for _, selection := range allSelections {
		inside, insideErr := pathInsideDirectory(document.Dir, targetDir, selection.Path)
		if insideErr != nil {
			return nil, insideErr
		}
		if inside {
			selectorSet[selection.ResourceName+"/"+selection.DownloadName] = struct{}{}
		}
	}
	selectors := make([]string, 0, len(selectorSet))
	for selector := range selectorSet {
		selectors = append(selectors, selector)
	}
	sort.Strings(selectors)
	selections := make([]manifest.Selection, 0, len(selectors))
	for _, selector := range selectors {
		selected, selectErr := document.SelectTarget([]string{selector}, target)
		if selectErr != nil {
			return nil, selectErr
		}
		selections = append(selections, selected...)
	}
	return selections, nil
}

func pathInsideDirectory(root, targetDir, path string) (bool, error) {
	fullPath, err := safepath.Resolve(root, path)
	if err != nil {
		return false, err
	}
	relative, err := filepath.Rel(targetDir, fullPath)
	if err != nil {
		return false, err
	}
	return relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator)), nil
}

func generationDir(root, relative string) (string, error) {
	if filepath.Clean(relative) == "." {
		return filepath.EvalSymlinks(root)
	}
	return safepath.Resolve(root, relative)
}

func inferPackage(dir, output string) (string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return "", err
	}
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || name == output || strings.HasSuffix(name, "_test.go") || !strings.HasSuffix(name, ".go") {
			continue
		}
		parsed, parseErr := parser.ParseFile(token.NewFileSet(), filepath.Join(dir, name), nil, parser.PackageClauseOnly)
		if parseErr != nil {
			return "", parseErr
		}
		return parsed.Name.Name, nil
	}
	return "", fmt.Errorf("package name required for directory %s with no Go files", dir)
}

func render(packageName string, downloads []embeddedDownload) ([]byte, error) {
	sort.Slice(downloads, func(i, j int) bool {
		left, right := downloads[i].selection, downloads[j].selection
		if left.ResourceName == right.ResourceName {
			return left.DownloadName < right.DownloadName
		}
		return left.ResourceName < right.ResourceName
	})
	identifiers := make(map[string]string)
	for _, download := range downloads {
		for identifier, source := range map[string]string{
			"muambaResource" + goName(download.selection.ResourceName):                                           download.selection.ResourceName,
			"muambaDownload" + goName(download.selection.ResourceName) + goName(download.selection.DownloadName): download.selection.ResourceName + "/" + download.selection.DownloadName,
		} {
			if previous, exists := identifiers[identifier]; exists && previous != source {
				return nil, fmt.Errorf("generated identifier collision %s for %s and %s", identifier, previous, source)
			}
			identifiers[identifier] = source
		}
	}

	var buffer bytes.Buffer
	fmt.Fprintf(&buffer, "// Code generated by muamba. DO NOT EDIT.\n\npackage %s\n\n", packageName)
	buffer.WriteString("import (\n\t\"embed\"\n\t\"fmt\"\n\t\"io/fs\"\n)\n\n")
	embedPaths := make([]string, len(downloads))
	for index, download := range downloads {
		embedPaths[index] = download.embedPath
	}
	sort.Strings(embedPaths)
	buffer.WriteString("//go:embed")
	for _, embedPath := range embedPaths {
		fmt.Fprintf(&buffer, " %s", embedPath)
	}
	buffer.WriteString("\nvar muambaFiles embed.FS\n\nconst (\n")
	identifierNames := make([]string, 0, len(identifiers))
	for identifier := range identifiers {
		identifierNames = append(identifierNames, identifier)
	}
	sort.Strings(identifierNames)
	for _, identifier := range identifierNames {
		value := identifiers[identifier]
		if strings.HasPrefix(identifier, "muambaDownload") {
			value = value[strings.Index(value, "/")+1:]
		}
		fmt.Fprintf(&buffer, "\t%s = %q\n", identifier, value)
	}
	buffer.WriteString(")\n\ntype MuambaResource struct {\n\tName string\n\tVersion string\n\tDownloads []MuambaDownload\n}\n\ntype MuambaDownload struct {\n\tName string\n\tURL string\n\tPath string\n\tIntegrity string\n\tHash string\n}\n\n")
	buffer.WriteString("var muambaResources = []MuambaResource{\n")
	for index := 0; index < len(downloads); {
		resource := downloads[index].selection.ResourceName
		fmt.Fprintf(&buffer, "\t{Name: %q, Version: %q, Downloads: []MuambaDownload{\n", resource, downloads[index].selection.Version)
		for index < len(downloads) && downloads[index].selection.ResourceName == resource {
			selection := downloads[index].selection
			fmt.Fprintf(&buffer, "\t\t{Name: %q, URL: %q, Path: %q, Integrity: %q, Hash: %q},\n", selection.DownloadName, selection.URL, selection.Path, selection.Integrity, downloads[index].hash)
			index++
		}
		buffer.WriteString("\t}},\n")
	}
	buffer.WriteString("}\n\nvar muambaEmbeddedPaths = map[string]string{\n")
	for _, download := range downloads {
		fmt.Fprintf(&buffer, "\t%q: %q,\n", download.selection.ResourceName+"\x00"+download.selection.DownloadName, download.embedPath)
	}
	buffer.WriteString("}\n\nvar muambaHashes = map[string]string{\n")
	for _, download := range downloads {
		fmt.Fprintf(&buffer, "\t%q: %q,\n", download.selection.ResourceName+"\x00"+download.selection.DownloadName, download.hash)
	}
	buffer.WriteString("}\n\nfunc MuambaResources() []MuambaResource {\n\tresult := make([]MuambaResource, len(muambaResources))\n\tcopy(result, muambaResources)\n\tfor index := range result {\n\t\tresult[index].Downloads = append([]MuambaDownload(nil), result[index].Downloads...)\n\t}\n\treturn result\n}\n\n")
	buffer.WriteString("func MuambaResourceByName(name string) (MuambaResource, bool) {\n\tfor _, resource := range MuambaResources() {\n\t\tif resource.Name == name { return resource, true }\n\t}\n\treturn MuambaResource{}, false\n}\n\n")
	buffer.WriteString("func MuambaHash(resource, download string) (string, bool) {\n\thash, ok := muambaHashes[resource+\"\\x00\"+download]\n\treturn hash, ok\n}\n\n")
	buffer.WriteString("func MuambaOpen(resource, download string) (fs.File, error) {\n\tpath, ok := muambaEmbeddedPaths[resource+\"\\x00\"+download]\n\tif !ok { return nil, fmt.Errorf(\"unknown Muamba download %s/%s\", resource, download) }\n\treturn muambaFiles.Open(path)\n}\n")
	formatted, err := format.Source(buffer.Bytes())
	if err != nil {
		return nil, fmt.Errorf("format generated Go: %w\n%s", err, buffer.String())
	}
	return formatted, nil
}

func goName(name string) string {
	var result strings.Builder
	for _, part := range strings.Split(name, "-") {
		if part == "" {
			continue
		}
		result.WriteString(strings.ToUpper(part[:1]))
		result.WriteString(part[1:])
	}
	return result.String()
}

func writeAtomic(path string, contents []byte) error {
	temporary, err := os.CreateTemp(filepath.Dir(path), ".muamba-go-*")
	if err != nil {
		return err
	}
	temporaryPath := temporary.Name()
	defer func() { _ = os.Remove(temporaryPath) }()
	if _, err := temporary.Write(contents); err != nil {
		_ = temporary.Close()
		return err
	}
	if err := temporary.Chmod(0o644); err != nil {
		_ = temporary.Close()
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	return os.Rename(temporaryPath, path)
}
