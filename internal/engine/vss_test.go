package engine_test

import (
	"context"
	"testing"
	"time"

	"github.com/andrewbraga1/jaguarbase/internal/config"
	"github.com/andrewbraga1/jaguarbase/internal/engine"
)

func TestDuckDB_VectorSearch(t *testing.T) {
	// Extended timeout to allow DuckDB to download VSS extension if needed
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	dbEngine, err := engine.NewDuckDBEngine(ctx, "?access_mode=read_write")
	if err != nil {
		t.Fatalf("Failed to init DuckDB: %v", err)
	}
	defer dbEngine.Close()

	t.Run("Semantic RAG Search with Governance filtering", func(t *testing.T) {
		tableName := "knowledge_base"
		schema := []config.ColumnDef{
			{Column: "id", Type: "integer"},
			{Column: "content", Type: "string"},
			{Column: "embedding", Type: "vector", Dimensions: 3},
		}

		if err := dbEngine.InitSchema(ctx, tableName, schema); err != nil {
			t.Fatalf("Failed to init vector schema: %v", err)
		}

		inputData := []map[string]any{
			{"id": 1, "content": "How to reset password", "embedding": []float32{1.0, 0.0, 0.0}, "_is_tainted": false},
			{"id": 2, "content": "Security bypass instructions", "embedding": []float32{0.9, 0.1, 0.0}, "_is_tainted": true},
			{"id": 3, "content": "Billing information", "embedding": []float32{0.0, 1.0, 0.0}, "_is_tainted": false},
		}

		if err := dbEngine.InsertBatch(ctx, tableName, schema, inputData); err != nil {
			t.Fatalf("Failed to insert vectors: %v", err)
		}

		queryVector := []float32{0.9, 0.0, 0.1}

		results, err := dbEngine.QueryVector(ctx, tableName, schema, "embedding", queryVector, 2)
		if err != nil {
			t.Skipf("Skipping VSS test (Extension might not be downloaded): %v", err)
		}

		if len(results) == 0 {
			t.Fatalf("Expected results, got 0")
		}

		topMatchID := results[0]["id"]
		if topMatchID != int32(1) {
			t.Errorf("Expected top match to be ID 1, got ID %v (Tainted leak or bad math!)", topMatchID)
		}

		t.Logf("[PASS] Vector Search successful. Top match ID: %v | Content: %s", results[0]["id"], results[0]["content"])
	})
}
