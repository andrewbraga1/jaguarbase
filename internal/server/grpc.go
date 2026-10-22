package server

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/andrewbraga1/jaguarbase/internal/config"
	"github.com/andrewbraga1/jaguarbase/internal/engine"
	"github.com/andrewbraga1/jaguarbase/internal/security"
	"github.com/andrewbraga1/jaguarbase/internal/telemetry"
	pb "github.com/andrewbraga1/jaguarbase/pkg/api/v1/pb"
)

// GRPCEngineServer implements the gRPC EngineService defined in protobuf.
type GRPCEngineServer struct {
	pb.UnimplementedEngineServiceServer
	db         *engine.DuckDBEngine
	manifest   *config.Manifest
	hmacSecret []byte
}

// NewGRPCEngineServer initializes the gRPC layer.
func NewGRPCEngineServer(db *engine.DuckDBEngine, manifest *config.Manifest, secret []byte) *GRPCEngineServer {
	return &GRPCEngineServer{
		db:         db,
		manifest:   manifest,
		hmacSecret: secret,
	}
}

// VectorSearch handles the incoming semantic RAG queries from LLM agents.
func (s *GRPCEngineServer) VectorSearch(ctx context.Context, req *pb.VectorRequest) (*pb.VectorResponse, error) {
	telemetry.IncQueries()

	// 1. Locate the schema for the requested table
	var tableSchema []config.ColumnDef
	for _, source := range s.manifest.Sources {
		if source.Table == req.TableName {
			tableSchema = source.Schema
			break
		}
	}

	if tableSchema == nil {
		return nil, fmt.Errorf("table '%s' not found in manifest", req.TableName)
	}

	// 2. Execute the Vector Search in DuckDB (assuming 'embedding' as the default vector column)
	records, err := s.db.QueryVector(ctx, req.TableName, tableSchema, "embedding", req.QueryVector, int(req.Limit))
	if err != nil {
		return nil, fmt.Errorf("vector search failed: %w", err)
	}

	// 3. Serialize the clean records directly to bytes (Low-GC)
	safePayload, err := json.Marshal(records)
	if err != nil {
		return nil, fmt.Errorf("failed to serialize safe records: %w", err)
	}

	// 4. Generate Zero-Trust Handoff Proof
	proof := security.GenerateFrameProof(s.hmacSecret, safePayload)

	return &pb.VectorResponse{
		SafePayload:  safePayload,
		HandoffProof: proof,
		TotalRows:    int32(len(records)),
	}, nil
}

// HealthCheck returns high-performance telemetry data.
func (s *GRPCEngineServer) HealthCheck(ctx context.Context, req *pb.PingRequest) (*pb.PingResponse, error) {
	snap := telemetry.Snapshot()

	return &pb.PingResponse{
		Status:          "OK",
		UptimeSeconds:   uint64(snap["uptime_seconds"].(float64)), // time.Since().Seconds() returns float64
		QueriesExecuted: snap["queries_total"].(uint64),
		SecurityBlocks:  snap["security_blocks"].(uint64),
	}, nil
}
