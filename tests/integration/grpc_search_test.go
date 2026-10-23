package integration_test

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/andrewbraga1/jaguarbase/internal/ai"
	pb "github.com/andrewbraga1/jaguarbase/pkg/api/v1/pb"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

func TestGRPCSemanticSearch(t *testing.T) {
	// Skip this test in CI/CD environments since Ollama and the DB are not running
	if os.Getenv("CI") != "" {
		t.Skip("Skipping integration test in CI: Local Ollama daemon and DB are not available in the GitHub Actions runner.")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	apiUrl := os.Getenv("LLM_API_URL")
	if apiUrl == "" {
		apiUrl = "http://localhost:11434/api/embeddings"
	}
	model := os.Getenv("LLM_MODEL")
	if model == "" {
		model = "qwen2.5-coder:1.5b"
	}

	aiAdapter := ai.NewAiAdapter(apiUrl)
	queryText := "My dashboard froze and the screen just keeps spinning"

	t.Logf("🧠 Vectorizing query: '%s'...", queryText)
	queryVector, err := aiAdapter.Generate(ctx, queryText, model)
	if err != nil {
		t.Fatalf("❌ Vectorization failed: %v", err)
	}

	t.Log("⚡ Connecting to JaguarBase gRPC (:50051)...")
	conn, err := grpc.NewClient("localhost:50051", grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatalf("❌ Failed to connect to gRPC: %v", err)
	}
	defer conn.Close()

	client := pb.NewEngineServiceClient(conn)

	req := &pb.VectorRequest{
		TableName:   "support_tickets",
		QueryVector: queryVector,
		Limit:       3,
	}

	startTime := time.Now()
	res, err := client.VectorSearch(ctx, req)
	if err != nil {
		t.Fatalf("❌ gRPC VectorSearch call failed: %v", err)
	}

	// Assertions to guarantee Zero-Trust compliance and data delivery
	if res.TotalRows == 0 {
		t.Errorf("⚠️ Expected to find results, but returned 0 rows.")
	}
	if res.HandoffProof == "" {
		t.Errorf("⚠️ Handoff Proof (HMAC) is empty. Zero-Trust security validation failed.")
	}
	if len(res.SafePayload) == 0 {
		t.Errorf("⚠️ JSON Payload is empty.")
	}

	t.Logf("🎉 Search completed in %v", time.Since(startTime))
	t.Logf("📊 Rows found: %d", res.TotalRows)
	t.Logf("🛡️ HMAC Proof: %s", res.HandoffProof)
	t.Logf("📦 JSON Payload: %s", string(res.SafePayload))
}
