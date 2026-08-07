package manifest

type Manifest struct {
	Schema    int                  `yaml:"schema"`
	Resources map[string]*Resource `yaml:"resources"`
}

type Resource struct {
	Version     string                `yaml:"version"`
	Downloads   map[string]*Download  `yaml:"downloads,omitempty"`
	Directories map[string]*Directory `yaml:"directories,omitempty"`
}

type Directory struct {
	URL              string   `yaml:"url"`
	Archive          string   `yaml:"archive"`
	Path             string   `yaml:"path"`
	Include          []string `yaml:"include"`
	Exclude          []string `yaml:"exclude,omitempty"`
	StripComponents  int      `yaml:"strip_components,omitempty"`
	MaxSize          string   `yaml:"max_size,omitempty"`
	MaxFiles         int      `yaml:"max_files"`
	MaxUnpackedSize  string   `yaml:"max_unpacked_size"`
	resolvedMaxBytes int64
	resolvedUnpacked int64
}

type PlatformDownload struct {
	URL       string `yaml:"url"`
	Integrity string `yaml:"integrity,omitempty"`
	Size      int64  `yaml:"-"`
}

type Download struct {
	URL        string                       `yaml:"url,omitempty"`
	Path       string                       `yaml:"path"`
	Integrity  string                       `yaml:"integrity,omitempty"`
	Platform   string                       `yaml:"platform,omitempty"`
	Executable bool                         `yaml:"executable,omitempty"`
	MaxSize    string                       `yaml:"max_size,omitempty"`
	Platforms  map[string]*PlatformDownload `yaml:"platforms,omitempty"`
	Size       int64                        `yaml:"-"`
}

type Lock struct {
	Schema      int               `yaml:"schema"`
	Files       []LockedFile      `yaml:"files,omitempty"`
	Directories []LockedDirectory `yaml:"directories,omitempty"`
}

type LockedFile struct {
	ID        string `yaml:"id"`
	URL       string `yaml:"url"`
	Path      string `yaml:"path"`
	Size      int64  `yaml:"size"`
	Integrity string `yaml:"integrity"`
}

type LockedDirectory struct {
	ID        string                `yaml:"id"`
	URL       string                `yaml:"url"`
	Path      string                `yaml:"path"`
	Size      int64                 `yaml:"size"`
	Integrity string                `yaml:"integrity"`
	Files     []LockedDirectoryFile `yaml:"files"`
}

type LockedDirectoryFile struct {
	Source    string `yaml:"source"`
	Path      string `yaml:"path"`
	Size      int64  `yaml:"size"`
	Integrity string `yaml:"integrity"`
}

type DirectorySelection struct {
	ResourceName     string
	DirectoryName    string
	Version          string
	URL              string
	Archive          string
	Path             string
	Include          []string
	Exclude          []string
	StripComponents  int
	MaxBytes         int64
	MaxFiles         int
	MaxUnpackedBytes int64
	Lock             *LockedDirectory
}

func (selection DirectorySelection) ID() string {
	return selection.ResourceName + "/" + selection.DirectoryName
}

type Warning struct {
	Resource string
	Download string
	Message  string
}

type Selection struct {
	ResourceName string
	DownloadName string
	Version      string
	URL          string
	Path         string
	Integrity    string
	Variant      string
	Platform     string
	Executable   bool
	MaxBytes     int64
	Size         int64
}

type resolvedDownload struct {
	ResourceName string
	DownloadName string
	Version      string
	Path         string
	URL          string
	Integrity    string
	Platform     string
	Executable   bool
	MaxBytes     int64
	Size         int64
	Platforms    map[string]PlatformDownload
}
