package server_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/andrewbraga1/jaguarbase/internal/config"
	"github.com/andrewbraga1/jaguarbase/internal/engine"
	"github.com/andrewbraga1/jaguarbase/internal/server"
	pb "github.com/andrewbraga1/jaguarbase/pkg/api/v1/pb"
)

func TestGRPCEngineServer_Integration(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// 1. Setup Embedded DB & Schema
	dbEngine, err := engine.NewDuckDBEngine(ctx, "?access_mode=read_write")
	if err != nil {
		t.Fatalf("Failed to init DuckDB: %v", err)
	}
	defer dbEngine.Close()

	schema := []config.ColumnDef{
		{Column: "id", Type: "integer"},
		{Column: "content", Type: "string"},
		{Column: "metadata", Type: "string"},
		{Column: "embedding", Type: "vector", Dimensions: 3},
	}
	tableName := "test_knowledge"

	if err := dbEngine.InitSchema(ctx, tableName, schema); err != nil {
		t.Fatalf("InitSchema failed: %v", err)
	}

	inputData := []map[string]any{
		{"id": 1, "content": "Safe Data", "embedding": []float32{1.0, 0.0, 0.0}, "_is_tainted": false},
		{"id": 2, "content": "Tainted Data", "embedding": []float32{1.0, 0.1, 0.0}, "_is_tainted": true},
	}
	_ = dbEngine.InsertBatch(ctx, tableName, schema, inputData)

	// 2. Setup Server
	manifest := &config.Manifest{
		Sources: []config.SourceDef{{Table: tableName, Schema: schema}},
	}
	secret := []byte("super-secret-key")
	grpcServer := server.NewGRPCEngineServer(dbEngine, manifest, secret)

	// 3. Test HealthCheck (Telemetry)
	t.Run("HealthCheck returns telemetry", func(t *testing.T) {
		res, err := grpcServer.HealthCheck(ctx, &pb.PingRequest{})
		if err != nil {
			t.Fatalf("HealthCheck failed: %v", err)
		}
		if res.Status != "OK" {
			t.Errorf("Expected status OK, got %s", res.Status)
		}
		t.Logf("[PASS] HealthCheck: %v queries executed", res.QueriesExecuted)
	})

	// 4. Test VectorSearch via gRPC Handler
	t.Run("VectorSearch enforces Zero-Trust", func(t *testing.T) {
		req := &pb.VectorRequest{
			TableName:   tableName,
			QueryVector: []float32{1.0, 0.0, 0.0},
			Limit:       2,
		}

		res, err := grpcServer.VectorSearch(ctx, req)
		if err != nil {
			t.Fatalf("VectorSearch failed: %v", err)
		}

		if res.TotalRows != 1 {
			t.Errorf("Expected 1 row (tainted filtered out), got %d", res.TotalRows)
		}

		payloadStr := string(res.SafePayload)
		if strings.Contains(payloadStr, "Tainted") {
			t.Errorf("Security Leak: Tainted data reached the gRPC response!")
		}
		if res.HandoffProof == "" {
			t.Errorf("Missing HMAC Handoff Proof")
		}

		t.Logf("[PASS] gRPC VectorSearch successful. Proof: %s...", res.HandoffProof[:15])
	})
}
