package pipeline

// pipeline defines the structure of a pipeline configuration in YAML format.
type Pipeline struct {
	Name  string `yaml:"name"`
	Image string `yaml:"image"`
	Steps []Step `yaml:"steps"`
}

// step is one command to run inside the container
type Step struct {
	Name string `yaml:"name"`
	Run  string `yaml:"run"`
}
