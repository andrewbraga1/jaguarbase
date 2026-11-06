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
      - column: "id"
        type: "string"
        primary_key: true
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
		if source.Schema[1].Governance != "pii_vault" {
			t.Errorf("Expected pii_vault governance, got %s", source.Schema[1].Governance)
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

	t.Run("Fail-Closed: Rejects manifest when content_column is missing from schema", func(t *testing.T) {
		missingContentYAML := `
version: "1.0"
sources:
  - name: "test_db"
    type: "postgres"
    table: "users"
    content_column: "unmapped_body"
    schema:
      - column: "id"
        type: "string"
        primary_key: true
      - column: "cpf"
        type: "string"
`
		tmp, _ := os.CreateTemp("", "missing_content_*.yaml")
		defer os.Remove(tmp.Name())
		tmp.Write([]byte(missingContentYAML))
		tmp.Close()

		_, err := config.LoadManifest(tmp.Name())
		if err == nil {
			t.Fatalf("Expected Fail-Closed error when content_column is not defined in schema, got nil")
		}
	})

	t.Run("Fail-Closed: Rejects manifest without primary key", func(t *testing.T) {
		noPKYAML := `
version: "1.0"
sources:
  - name: "test_db"
    type: "postgres"
    table: "logs"
    schema:
      - column: "message"
        type: "string"
`
		tmp, _ := os.CreateTemp("", "no_pk_*.yaml")
		defer os.Remove(tmp.Name())
		tmp.Write([]byte(noPKYAML))
		tmp.Close()

		_, err := config.LoadManifest(tmp.Name())
		if err == nil {
			t.Fatalf("Expected Fail-Closed error when no primary key is defined, got nil")
		}
	})
}
