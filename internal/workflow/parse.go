package workflow

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// ParseWorkflow reads a YAML file and returns a validated Workflow.
func ParseWorkflow(path string) (*Workflow, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading workflow file: %w", err)
	}
	return ParseWorkflowBytes(data)
}

// ParseWorkflowBytes parses YAML bytes into a validated Workflow.
func ParseWorkflowBytes(data []byte) (*Workflow, error) {
	var w Workflow
	if err := yaml.Unmarshal(data, &w); err != nil {
		return nil, fmt.Errorf("parsing workflow YAML: %w", err)
	}
	if err := Validate(&w); err != nil {
		return nil, err
	}
	return &w, nil
}

// ParseFarmConfig reads a YAML file and returns a FarmConfig.
func ParseFarmConfig(path string) (*FarmConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading farm config: %w", err)
	}
	var cfg FarmConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parsing farm config YAML: %w", err)
	}
	if err := ValidateFarmConfig(&cfg); err != nil {
		return nil, err
	}
	return &cfg, nil
}
