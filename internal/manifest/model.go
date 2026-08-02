package manifest

type Manifest struct {
	Schema    int                  `yaml:"schema"`
	Resources map[string]*Resource `yaml:"resources"`
}

type Resource struct {
	Version   string               `yaml:"version"`
	Downloads map[string]*Download `yaml:"downloads"`
}

type Download struct {
	URL       string `yaml:"url"`
	Path      string `yaml:"path"`
	Integrity string `yaml:"integrity,omitempty"`
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
}
