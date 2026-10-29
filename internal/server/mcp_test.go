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
	"time"

	"github.com/andrewbraga1/jaguarbase/internal/config"
	"github.com/andrewbraga1/jaguarbase/internal/engine"
	"github.com/andrewbraga1/jaguarbase/internal/server"
)

func TestMCPServer_VectorSearch(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// 1. Setup Embedded DB & Schema
	dbEngine, err := engine.NewDuckDBEngine(ctx, "?access_mode=read_write")
	if err != nil {
		t.Fatalf("Failed to init DuckDB: %v", err)
	}
	defer dbEngine.Close()

	schema := []config.ColumnDef{
		{Column: "id", Type: "string"},
		{Column: "content", Type: "string"},
		{Column: "metadata", Type: "string"},
		{Column: "_is_tainted", Type: "boolean"},
		{Column: "embedding", Type: "vector", Dimensions: 3},
	}
	tableName := "agent_knowledge"

	if err := dbEngine.InitSchema(ctx, tableName, schema); err != nil {
		t.Fatalf("InitSchema failed: %v", err)
	}

	// Inserting data with metadata column
	inputData := []map[string]any{
		{"id": "1", "content": "Safe Agent Data", "metadata": `{"role":"agent"}`, "embedding": []float32{1.0, 0.0, 0.0}, "_is_tainted": false},
		{"id": "2", "content": "Tainted Agent Data", "metadata": `{"role":"hacker"}`, "embedding": []float32{1.0, 0.1, 0.0}, "_is_tainted": true},
	}
	_ = dbEngine.InsertBatch(ctx, tableName, schema, inputData)

	// 2. Setup MCP Server
	manifest := &config.Manifest{
		Sources: []config.SourceDef{{Table: tableName, Schema: schema}},
	}
	mcpServer := server.NewMCPServer(dbEngine, manifest, []byte("mcp-secret"))

	// Helper to send HTTP requests to the MCP handler
	sendRequest := func(reqPayload string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodPost, "/mcp", bytes.NewBufferString(reqPayload))
		req.Header.Set("Content-Type", "application/json")
		rr := httptest.NewRecorder()
		mcpServer.ServeHTTP(rr, req)
		return rr
	}

	// 3. Validates the required MCP 'tools/list' protocol discovery phase
	t.Run("Executes tools/list correctly", func(t *testing.T) {
		reqPayload := `{
			"jsonrpc": "2.0",
			"id": 1,
			"method": "tools/list"
		}`

		rec := sendRequest(reqPayload)
		if rec.Code != http.StatusOK {
			t.Errorf("Expected 200 OK, got %d", rec.Code)
		}

		var res server.MCPResponse
		json.Unmarshal(rec.Body.Bytes(), &res)

		if res.Error != nil {
			t.Fatalf("Unexpected error from tools/list: %v", res.Error)
		}

		// Validate if the tool is dynamically exposed in the list
		resultMap, ok := res.Result.(map[string]any)
		if !ok || resultMap["tools"] == nil {
			t.Fatalf("Failed to parse tools list result")
		}

		t.Logf("[PASS] MCP tools/list successful. Tools exposed: %v", len(resultMap["tools"].([]any)))
	})

	// 4. Original valid vector_search tool call test (Zero-Trust enforcement)
	t.Run("Executes valid JSON-RPC 2.0 tool call", func(t *testing.T) {
		// Mock a payload coming from an LLM
		reqPayload := `{
			"jsonrpc": "2.0",
			"id": 2,
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

		rec := sendRequest(reqPayload)
		if rec.Code != http.StatusOK {
			t.Errorf("Expected 200 OK, got %d. Body: %s", rec.Code, rec.Body.String())
		}

		var res server.MCPResponse
		if err := json.Unmarshal(rec.Body.Bytes(), &res); err != nil {
			t.Fatalf("Failed to parse response: %v", err)
		}

		if res.Error != nil {
			t.Fatalf("Expected valid result, got error: %v", res.Error)
		}

		// Safely cast the generic Result (any) to map to access Content and Proof
		resultMap, ok := res.Result.(map[string]any)
		if !ok {
			t.Fatalf("Failed to parse tool call result map")
		}

		content, ok := resultMap["content"].([]any)
		if !ok {
			t.Fatalf("Result 'content' is missing or not an array")
		}

		if len(content) != 1 {
			t.Errorf("Expected 1 clean row, got %d", len(content))
		}

		contentStr := fmt.Sprintf("%v", content)
		if strings.Contains(contentStr, "Tainted") {
			t.Errorf("Security Leak: Tainted data reached the MCP response!")
		}

		proof, _ := resultMap["handoff_proof"].(string)
		if proof == "" {
			t.Errorf("Missing HMAC Handoff Proof")
		}

		t.Logf("[PASS] MCP VectorSearch successful. Proof: %s...", proof[:15])
	})
}
