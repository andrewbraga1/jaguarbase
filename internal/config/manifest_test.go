package config_test

import (
	"os"
	"testing"

	"github.com/andrewbraga1/jaguarbase/internal/config"
)

func TestLoadManifest(t *testing.T) {
	// Create a temporary valid yaml file for testing
	validYAML := `
version: "1.0"
sources:
  - name: "test_db"
    type: "postgres"
    table: "users"
    schema:
      - column: "cpf"
        type: "string"
        governance: "pii_vault"
`
	tmpFile, err := os.CreateTemp("", "jaguardb_test_*.yaml")
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}
	defer os.Remove(tmpFile.Name())

	if _, err := tmpFile.Write([]byte(validYAML)); err != nil {
		t.Fatalf("Failed to write temp file: %v", err)
	}
	tmpFile.Close()

	t.Run("Successfully loads valid manifest", func(t *testing.T) {
		manifest, err := config.LoadManifest(tmpFile.Name())
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		if manifest.Version != "1.0" {
			t.Errorf("Expected version 1.0, got %s", manifest.Version)
		}
		if len(manifest.Sources) != 1 {
			t.Fatalf("Expected 1 source, got %d", len(manifest.Sources))
		}

		source := manifest.Sources[0]
		if source.Name != "test_db" || source.Table != "users" {
			t.Errorf("Source mapping failed")
		}
		if source.Schema[0].Governance != "pii_vault" {
			t.Errorf("Expected pii_vault governance, got %s", source.Schema[0].Governance)
		}
	})

	t.Run("Fails gracefully on missing file", func(t *testing.T) {
		_, err := config.LoadManifest("non_existent_file.yaml")
		if err == nil {
			t.Errorf("Expected error for missing file, got nil")
		}
	})

	t.Run("Fails gracefully on invalid YAML content", func(t *testing.T) {
		badFile, _ := os.CreateTemp("", "bad_*.yaml")
		defer os.Remove(badFile.Name())
		badFile.Write([]byte(`version: "1.0" \n sources: [invalid_yaml_format`))
		badFile.Close()

		_, err := config.LoadManifest(badFile.Name())
		if err == nil {
			t.Errorf("Expected error for invalid yaml, got nil")
		}
	})
}
