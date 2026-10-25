package pipeline

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"

	"gopkg.in/yaml.v3"
)

//this file contains the parser for the pipeline
//configuration in yaml format

// Parse reads YAML bytes into a validated Pipeline.

func Parse(data []byte) (*Pipeline, error) {
	var p Pipeline

	if len(bytes.TrimSpace(data)) == 0 {
		return nil, errors.New("pipeline file is empty")
	}
	decoder := yaml.NewDecoder(bytes.NewReader(data))
	decoder.KnownFields(true)
	if err := decoder.Decode(&p); err != nil {
		if errors.Is(err, io.EOF) {
			return nil, errors.New("pipeline file is empty")
		}
		return nil, fmt.Errorf("pipeline field is invalid: %w", err)
	}
	if err := p.Validate(); err != nil {
		return nil, err
	}
	return &p, nil
}

// ParseFile reads a file from disk and parses it..
func ParseFile(path string) (*Pipeline, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read pipeline file: %w", err)
	}
	return Parse(data)
}

// validate function here
func (p Pipeline) Validate() error {
	if p.Image == "" {
		return errors.New("pipeline image is required")
	}
	if len(p.Steps) == 0 {
		return errors.New("pipeline steps must contain at least one step")
	}
	names := make(map[string]struct{}, len(p.Steps))
	for i, step := range p.Steps {
		if step.Name == "" {
			return fmt.Errorf("pipeline steps[%d].name is required", i)
		}
		if _, exists := names[step.Name]; exists {
			return fmt.Errorf("pipeline steps[%d].name %q is duplicated", i, step.Name)
		}
		names[step.Name] = struct{}{}
	}
	return nil
}
