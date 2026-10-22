package server_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/andrewbraga1/jaguarbase/internal/config"
	"github.com/andrewbraga1/jaguarbase/internal/engine"
	"github.com/andrewbraga1/jaguarbase/internal/server"
)

func TestMCPServer_VectorSearch(t *testing.T) {
	ctx := context.Background()

	// 1. Setup Embedded DB & Schema
	dbEngine, err := engine.NewDuckDBEngine(ctx, "?access_mode=read_write")
	if err != nil {
		t.Fatalf("Failed to init DuckDB: %v", err)
	}
	defer dbEngine.Close()

	schema := []config.ColumnDef{
		{Column: "id", Type: "integer"},
		{Column: "content", Type: "string"},
		{Column: "embedding", Type: "vector", Dimensions: 3},
	}
	tableName := "agent_knowledge"

	if err := dbEngine.InitSchema(ctx, tableName, schema); err != nil {
		t.Fatalf("InitSchema failed: %v", err)
	}

	inputData := []map[string]any{
		{"id": 1, "content": "Safe Agent Data", "embedding": []float32{1.0, 0.0, 0.0}, "_is_tainted": false},
		{"id": 2, "content": "Tainted Agent Data", "embedding": []float32{1.0, 0.1, 0.0}, "_is_tainted": true},
	}
	_ = dbEngine.InsertBatch(ctx, tableName, schema, inputData)

	// 2. Setup MCP Server
	manifest := &config.Manifest{
		Sources: []config.SourceDef{{Table: tableName, Schema: schema}},
	}
	mcpServer := server.NewMCPServer(dbEngine, manifest, []byte("mcp-secret"))

	t.Run("Executes valid JSON-RPC 2.0 tool call", func(t *testing.T) {
		// Mock a payload coming from an LLM
		reqPayload := `{
			"jsonrpc": "2.0",
			"id": 1,
			"method": "tools/call",
			"params": {
				"name": "vector_search",
				"arguments": {
					"table_name": "agent_knowledge",
					"query_vector": [1.0, 0.0, 0.0],
					"limit": 2
				}
			}
		}`

		req := httptest.NewRequest(http.MethodPost, "/mcp", bytes.NewBufferString(reqPayload))
		req.Header.Set("Content-Type", "application/json")

		rec := httptest.NewRecorder()
		mcpServer.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Errorf("Expected 200 OK, got %d. Body: %s", rec.Code, rec.Body.String())
		}

		var res server.MCPResponse
		if err := json.Unmarshal(rec.Body.Bytes(), &res); err != nil {
			t.Fatalf("Failed to parse response: %v", err)
		}

		if res.Result == nil {
			t.Fatalf("Expected valid result, got error: %v", res.Error)
		}

		if len(res.Result.Content) != 1 {
			t.Errorf("Expected 1 clean row, got %d", len(res.Result.Content))
		}

		contentStr := fmt.Sprintf("%v", res.Result.Content)
		if strings.Contains(contentStr, "Tainted") {
			t.Errorf("Security Leak: Tainted data reached the MCP response!")
		}

		t.Logf("[PASS] MCP VectorSearch successful. Proof: %s...", res.Result.Proof[:15])
	})
}
