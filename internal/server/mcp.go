package server

import (
	"crypto/subtle"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"sync/atomic"

	"github.com/andrewbraga1/jaguarbase/internal/config"
	"github.com/andrewbraga1/jaguarbase/internal/engine"
	"github.com/andrewbraga1/jaguarbase/internal/security"
	"github.com/andrewbraga1/jaguarbase/internal/telemetry"
)

// MCPRequest represents a generic JSON-RPC 2.0 request.
// Params is raw so we can parse it dynamically based on the Method.
type MCPRequest struct {
	Jsonrpc string          `json:"jsonrpc"`
	ID      any             `json:"id"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

// MCPResponse represents a JSON-RPC 2.0 success or error response.
// Result is 'any' to support both Tool Lists and Tool Call results.
type MCPResponse struct {
	Jsonrpc string    `json:"jsonrpc"`
	ID      any       `json:"id"`
	Result  any       `json:"result,omitempty"`
	Error   *RPCError `json:"error,omitempty"`
}

type RPCError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// CallToolParams is the specific payload for 'tools/call'
type CallToolParams struct {
	Name      string `json:"name"`
	Arguments struct {
		TableName   string    `json:"table_name"`
		QueryVector []float32 `json:"query_vector"`
		Limit       int       `json:"limit"`
	} `json:"arguments"`
}

type MCPServer struct {
	db              *engine.DuckDBEngine
	manifest        *config.Manifest
	hmacSecret      []byte
	apiKey          []byte
	inFlightCounter atomic.Int32
	inFlightLimit   int32
}

func NewMCPServer(db *engine.DuckDBEngine, manifest *config.Manifest, secret []byte) *MCPServer {
	limit := int32(manifest.Security.MCPInFlightLimit)
	if limit <= 0 {
		limit = 100
	}

	return &MCPServer{
		db:            db,
		manifest:      manifest,
		hmacSecret:    secret,
		apiKey:        []byte(os.Getenv("JAGUARBASE_API_KEY")),
		inFlightLimit: limit,
	}
}

// ServeHTTP implements the standard Go http.Handler interface.
func (s *MCPServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Only POST is allowed", http.StatusMethodNotAllowed)
		return
	}

	// 🔐 Optional Edge API Key Validation (active when JAGUARBASE_API_KEY is configured)
	if len(s.apiKey) > 0 {
		token := strings.TrimSpace(strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer"))
		if token == "" {
			token = r.Header.Get("X-API-Key")
		}
		if subtle.ConstantTimeCompare([]byte(token), s.apiKey) != 1 {
			http.Error(w, `{"jsonrpc": "2.0", "error": {"code": -32001, "message": "Unauthorized: Invalid API Key"}}`, http.StatusUnauthorized)
			return
		}
	}

	// 🛡️ O(1) Load Shedding / Chaos Engineering Resilience
	current := s.inFlightCounter.Add(1)
	defer s.inFlightCounter.Add(-1)

	if current > s.inFlightLimit {
		http.Error(w, `{"jsonrpc": "2.0", "error": {"code": -32000, "message": "Server overloaded (Runaway Loop Protected)"}}`, http.StatusTooManyRequests)
		return
	}

	// Enforce 128 KB max payload to protect JSON decoder memory under load
	r.Body = http.MaxBytesReader(w, r.Body, 128<<10)

	var req MCPRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.sendError(w, req.ID, -32700, "Parse error")
		return
	}

	// 🚦 MCP Protocol Router
	switch req.Method {
	case "tools/list":
		s.handleToolsList(w, req)
	case "tools/call":
		s.handleToolsCall(w, r, req)
	default:
		s.sendError(w, req.ID, -32601, "Method not found")
	}
}

// Answer the AI Agent which tools are available in JaguarBase
func (s *MCPServer) handleToolsList(w http.ResponseWriter, req MCPRequest) {
	tools := []map[string]any{
		{
			"name":        "vector_search",
			"description": "Executes a semantic vector search securely across the database.",
			"inputSchema": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"table_name":   map[string]any{"type": "string"},
					"query_vector": map[string]any{"type": "array", "items": map[string]any{"type": "number"}},
					"limit":        map[string]any{"type": "integer"},
				},
				"required": []string{"table_name", "query_vector"},
			},
		},
	}

	res := MCPResponse{
		Jsonrpc: "2.0",
		ID:      req.ID,
		Result:  map[string]any{"tools": tools},
	}
	s.writeJSON(w, res)
}

// Execute the requested tool
func (s *MCPServer) handleToolsCall(w http.ResponseWriter, r *http.Request, req MCPRequest) {
	var params CallToolParams
	if err := json.Unmarshal(req.Params, &params); err != nil {
		s.sendError(w, req.ID, -32602, "Invalid params")
		return
	}

	if params.Name != "vector_search" {
		s.sendError(w, req.ID, -32601, "Unsupported tool")
		return
	}

	telemetry.IncQueries()

	// 1. Locate schema
	var tableSchema []config.ColumnDef
	for _, source := range s.manifest.Sources {
		if source.Table == params.Arguments.TableName {
			tableSchema = source.Schema
			break
		}
	}

	if tableSchema == nil {
		s.sendError(w, req.ID, -32602, fmt.Sprintf("Table '%s' not found in manifest", params.Arguments.TableName))
		return
	}

	// 2. Validate limit and execute Vector Search
	const maxSearchLimit = 1000
	limit := params.Arguments.Limit
	if limit <= 0 || limit > maxSearchLimit {
		limit = 10
	}

	records, err := s.db.SemanticSearch(
		// Safe Context Creation: Creates a secure context without redundancy
		r.Context(), // Uses 'r' (*http.Request) passed to ServeHTTP
		params.Arguments.TableName,
		params.Arguments.QueryVector,
		limit,
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
		Result: map[string]any{
			"content":       records,
			"handoff_proof": proof,
		},
	}
	s.writeJSON(w, res)
}

func (s *MCPServer) sendError(w http.ResponseWriter, id any, code int, message string) {
	res := MCPResponse{
		Jsonrpc: "2.0",
		ID:      id,
		Error: &RPCError{
			Code:    code,
			Message: message,
		},
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusBadRequest)
	json.NewEncoder(w).Encode(res)
}

func (s *MCPServer) writeJSON(w http.ResponseWriter, res MCPResponse) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(res)
}
