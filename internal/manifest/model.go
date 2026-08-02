package manifest

type Manifest struct {
	Schema    int                  `yaml:"schema"`
	Resources map[string]*Resource `yaml:"resources"`
}

type Resource struct {
	Version   string               `yaml:"version"`
	Downloads map[string]*Download `yaml:"downloads"`
}

type PlatformDownload struct {
	URL       string `yaml:"url"`
	Integrity string `yaml:"integrity,omitempty"`
}

type Download struct {
	URL        string                       `yaml:"url,omitempty"`
	Path       string                       `yaml:"path"`
	Integrity  string                       `yaml:"integrity,omitempty"`
	Platform   string                       `yaml:"platform,omitempty"`
	Executable bool                         `yaml:"executable,omitempty"`
	MaxSize    string                       `yaml:"max_size,omitempty"`
	Platforms  map[string]*PlatformDownload `yaml:"platforms,omitempty"`
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
	Platforms    map[string]PlatformDownload
}
