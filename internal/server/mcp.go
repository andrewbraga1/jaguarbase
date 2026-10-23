package server

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/andrewbraga1/jaguarbase/internal/config"
	"github.com/andrewbraga1/jaguarbase/internal/engine"
	"github.com/andrewbraga1/jaguarbase/internal/security"
	"github.com/andrewbraga1/jaguarbase/internal/telemetry"
)

// MCPRequest represents a standard JSON-RPC 2.0 tool call from an AI Agent.
type MCPRequest struct {
	Jsonrpc string `json:"jsonrpc"`
	ID      any    `json:"id"`
	Method  string `json:"method"`
	Params  struct {
		Name      string `json:"name"`
		Arguments struct {
			TableName   string    `json:"table_name"`
			QueryVector []float32 `json:"query_vector"`
			Limit       int       `json:"limit"`
		} `json:"arguments"`
	} `json:"params"`
}

// MCPResponse represents a JSON-RPC 2.0 success or error response.
type MCPResponse struct {
	Jsonrpc string `json:"jsonrpc"`
	ID      any    `json:"id"`
	Result  *struct {
		Content []map[string]any `json:"content"`
		Proof   string           `json:"handoff_proof"`
	} `json:"result,omitempty"`
	Error *struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

// MCPServer exposes the database to AI agents via Model Context Protocol.
type MCPServer struct {
	db         *engine.DuckDBEngine
	manifest   *config.Manifest
	hmacSecret []byte
}

// NewMCPServer initializes the JSON-RPC interface for LLMs.
func NewMCPServer(db *engine.DuckDBEngine, manifest *config.Manifest, secret []byte) *MCPServer {
	return &MCPServer{
		db:         db,
		manifest:   manifest,
		hmacSecret: secret,
	}
}

// ServeHTTP implements the standard Go http.Handler interface for the MCP endpoint.
func (s *MCPServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Only POST is allowed", http.StatusMethodNotAllowed)
		return
	}

	var req MCPRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.sendError(w, req.ID, -32700, "Parse error")
		return
	}

	if req.Method != "tools/call" || req.Params.Name != "vector_search" {
		s.sendError(w, req.ID, -32601, "Method not found or unsupported tool")
		return
	}

	telemetry.IncQueries()

	// 1. Locate schema
	var tableSchema []config.ColumnDef
	for _, source := range s.manifest.Sources {
		if source.Table == req.Params.Arguments.TableName {
			tableSchema = source.Schema
			break
		}
	}

	if tableSchema == nil {
		s.sendError(w, req.ID, -32602, fmt.Sprintf("Table '%s' not found", req.Params.Arguments.TableName))
		return
	}

	// 2. Execute Vector Search
	// records, err := s.db.QueryVector(
	// 	context.Background(),
	// 	req.Params.Arguments.TableName,
	// 	tableSchema,
	// 	"embedding",
	// 	req.Params.Arguments.QueryVector,
	// 	req.Params.Arguments.Limit,
	// )
	records, err := s.db.SemanticSearch(
		r.Context(), // Usando o contexto do request HTTP
		req.Params.Arguments.TableName,
		req.Params.Arguments.QueryVector,
		req.Params.Arguments.Limit,
	)
	if err != nil {
		s.sendError(w, req.ID, -32000, fmt.Sprintf("Internal error: %v", err))
		return
	}

	// 3. Generate HMAC Proof (Zero-Trust)
	payloadBytes, _ := json.Marshal(records)
	proof := security.GenerateFrameProof(s.hmacSecret, payloadBytes)

	// 4. Send successful MCP Response
	res := MCPResponse{
		Jsonrpc: "2.0",
		ID:      req.ID,
		Result: &struct {
			Content []map[string]any `json:"content"`
			Proof   string           `json:"handoff_proof"`
		}{
			Content: records,
			Proof:   proof,
		},
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(res)
}

func (s *MCPServer) sendError(w http.ResponseWriter, id any, code int, message string) {
	res := MCPResponse{
		Jsonrpc: "2.0",
		ID:      id,
		Error: &struct {
			Code    int    `json:"code"`
			Message string `json:"message"`
		}{
			Code:    code,
			Message: message,
		},
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusBadRequest)
	json.NewEncoder(w).Encode(res)
}
