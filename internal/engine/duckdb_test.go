package engine_test

import (
	"context"
	"testing"
	"time"

	"github.com/andrewbraga1/jaguarbase/internal/config"
	"github.com/andrewbraga1/jaguarbase/internal/engine"
)

func TestDuckDBEngine_DynamicIntegration(t *testing.T) {
	// Extended timeout to allow DuckDB to download VSS extension if needed
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	dbEngine, err := engine.NewDuckDBEngine(ctx, "?access_mode=read_write")
	if err != nil {
		t.Fatalf("Failed to initialize DuckDB: %v", err)
	}
	defer dbEngine.Close()

	t.Run("Dynamic Schema Creation and Insertion", func(t *testing.T) {
		tableName := "dynamic_logs"
		schema := []config.ColumnDef{
			{Column: "id", Type: "integer"},
			{Column: "cpf", Type: "string"},
			{Column: "message", Type: "string"},
			{Column: "_is_tainted", Type: "BOOLEAN"},
		}

		if err := dbEngine.InitSchema(ctx, tableName, schema); err != nil {
			t.Fatalf("InitSchema failed: %v", err)
		}

		inputData := []map[string]any{
			{"id": 100, "cpf": "vault_mock_111", "message": "clean query", "_is_tainted": false},
			{"id": 101, "cpf": "vault_mock_222", "message": "DROP TABLE", "_is_tainted": true},
			{"id": 102, "cpf": "vault_mock_333", "message": "another clean", "_is_tainted": false},
		}

		if err := dbEngine.InsertBatch(ctx, tableName, schema, inputData); err != nil {
			t.Fatalf("InsertBatch failed: %v", err)
		}

		cleanRecords, err := dbEngine.QueryAllClean(ctx, tableName, schema)
		if err != nil {
			t.Fatalf("QueryAllClean failed: %v", err)
		}

		if len(cleanRecords) != 2 {
			t.Errorf("Expected 2 clean records, got %d", len(cleanRecords))
		}

		for _, record := range cleanRecords {
			if record["_is_tainted"] == true {
				t.Errorf("Security Breach: Tainted record leaked!")
			}
			t.Logf("[PASS] Dynamic Safe Record: ID=%v | CPF=%v", record["id"], record["cpf"])
		}
	})

	t.Run("Idempotent Upsert and Primary Key deduplication", func(t *testing.T) {
		tableName := "upsert_test"
		schema := []config.ColumnDef{
			{Column: "id", Type: "VARCHAR", PrimaryKey: true},
			{Column: "content", Type: "string"},
			{Column: "_is_tainted", Type: "BOOLEAN"},
		}

		if err := dbEngine.InitSchema(ctx, tableName, schema); err != nil {
			t.Fatalf("InitSchema failed: %v", err)
		}

		// Initial insert
		initial := []map[string]any{
			{"id": "doc-1", "content": "original content", "_is_tainted": false},
		}
		if err := dbEngine.InsertBatch(ctx, tableName, schema, initial); err != nil {
			t.Fatalf("Initial InsertBatch failed: %v", err)
		}

		// Re-inserting the same ID with updated content must replace, NOT duplicate
		updated := []map[string]any{
			{"id": "doc-1", "content": "updated content", "_is_tainted": false},
		}
		if err := dbEngine.InsertBatch(ctx, tableName, schema, updated); err != nil {
			t.Fatalf("Idempotent re-insert failed: %v", err)
		}

		records, err := dbEngine.QueryAllClean(ctx, tableName, schema)
		if err != nil {
			t.Fatalf("QueryAllClean failed: %v", err)
		}

		if len(records) != 1 {
			t.Fatalf("Expected exactly 1 record after upsert, got %d (duplicate created!)", len(records))
		}

		if records[0]["content"] != "updated content" {
			t.Errorf("Expected content 'updated content', got '%v'", records[0]["content"])
		}
	})
}
