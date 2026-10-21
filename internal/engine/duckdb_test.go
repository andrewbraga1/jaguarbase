package engine_test

import (
	"context"
	"testing"
	"time"

	"github.com/andrewbraga1/jaguarbase/internal/config"
	"github.com/andrewbraga1/jaguarbase/internal/engine"
)

func TestDuckDBEngine_DynamicIntegration(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
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
		}

		// 1. Create table dynamically
		if err := dbEngine.InitSchema(ctx, tableName, schema); err != nil {
			t.Fatalf("InitSchema failed: %v", err)
		}

		// 2. Prepare dynamic map data (mimicking the output of the Governance Processor)
		inputData := []map[string]any{
			{"id": 100, "cpf": "vault_mock_111", "message": "clean query", "_is_tainted": false},
			{"id": 101, "cpf": "vault_mock_222", "message": "DROP TABLE", "_is_tainted": true}, // Tainted
			{"id": 102, "cpf": "vault_mock_333", "message": "another clean", "_is_tainted": false},
		}

		// 3. Insert batch
		if err := dbEngine.InsertBatch(ctx, tableName, schema, inputData); err != nil {
			t.Fatalf("InsertBatch failed: %v", err)
		}

		// 4. Query only safe data dynamically
		cleanRecords, err := dbEngine.QueryAllClean(ctx, tableName, schema)
		if err != nil {
			t.Fatalf("QueryAllClean failed: %v", err)
		}

		// 5. Fail-Closed assertions
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
}
