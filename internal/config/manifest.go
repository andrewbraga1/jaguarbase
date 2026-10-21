package config

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// ColumnDef defines the governance rules for a specific column.
type ColumnDef struct {
	Column     string `yaml:"column"`
	Type       string `yaml:"type"`
	Governance string `yaml:"governance"`
}

// SourceDef represents a data source (e.g., a PostgreSQL table).
type SourceDef struct {
	Name   string      `yaml:"name"`
	Type   string      `yaml:"type"`
	Table  string      `yaml:"table"`
	Schema []ColumnDef `yaml:"schema"`
}

// Manifest represents the entire jaguardb.yaml configuration.
type Manifest struct {
	Version string      `yaml:"version"`
	Sources []SourceDef `yaml:"sources"`
}

// LoadManifest reads and parses the YAML configuration file.
func LoadManifest(path string) (*Manifest, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read manifest file: %w", err)
	}

	var manifest Manifest
	if err := yaml.Unmarshal(data, &manifest); err != nil {
		return nil, fmt.Errorf("failed to parse manifest yaml: %w", err)
	}

	// Fail-Closed: Basic validation at boot time
	if manifest.Version == "" {
		return nil, fmt.Errorf("invalid manifest: missing version")
	}
	if len(manifest.Sources) == 0 {
		return nil, fmt.Errorf("invalid manifest: no sources defined")
	}

	return &manifest, nil
}
