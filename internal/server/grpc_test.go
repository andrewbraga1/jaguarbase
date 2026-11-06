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
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
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
		{Column: "_is_tainted", Type: "boolean"},
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

	// 5. Test UnaryAuthInterceptor enforcement
	t.Run("UnaryAuthInterceptor enforces API Key authentication", func(t *testing.T) {
		apiKey := []byte("edge-grpc-secret-999")
		interceptor := server.UnaryAuthInterceptor(apiKey)

		dummyHandler := func(ctx context.Context, req any) (any, error) {
			return "success", nil
		}
		info := &grpc.UnaryServerInfo{FullMethod: "/EngineService/VectorSearch"}

		// Case A: Missing metadata -> Unauthenticated
		_, err := interceptor(context.Background(), nil, info, dummyHandler)
		if status.Code(err) != codes.Unauthenticated {
			t.Fatalf("Expected Unauthenticated for missing metadata, got: %v", err)
		}

		// Case B: Invalid Bearer key -> Unauthenticated
		badCtx := metadata.NewIncomingContext(context.Background(), metadata.Pairs("authorization", "Bearer wrong-key"))
		_, err = interceptor(badCtx, nil, info, dummyHandler)
		if status.Code(err) != codes.Unauthenticated {
			t.Fatalf("Expected Unauthenticated for invalid key, got: %v", err)
		}

		// Case C: Valid Bearer key -> Success
		goodBearerCtx := metadata.NewIncomingContext(context.Background(), metadata.Pairs("authorization", "Bearer edge-grpc-secret-999"))
		res, err := interceptor(goodBearerCtx, nil, info, dummyHandler)
		if err != nil || res != "success" {
			t.Fatalf("Expected success with valid Bearer key, got: %v, res: %v", err, res)
		}

		// Case D: Valid X-API-Key -> Success
		goodKeyCtx := metadata.NewIncomingContext(context.Background(), metadata.Pairs("x-api-key", "edge-grpc-secret-999"))
		res, err = interceptor(goodKeyCtx, nil, info, dummyHandler)
		if err != nil || res != "success" {
			t.Fatalf("Expected success with valid X-API-Key, got: %v, res: %v", err, res)
		}

		t.Logf("[PASS] UnaryAuthInterceptor correctly validates Bearer and X-API-Key headers.")
	})
}
