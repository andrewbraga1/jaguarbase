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
	queryText := "Meu painel travou e só fica girando a tela"

	t.Logf("🧠 Vetorizando a pergunta: '%s'...", queryText)
	queryVector, err := aiAdapter.Generate(ctx, queryText, model)
	if err != nil {
		t.Fatalf("❌ Falha na vetorização: %v", err)
	}

	t.Log("⚡ Conectando ao gRPC do JaguarBase (:50051)...")
	conn, err := grpc.NewClient("localhost:50051", grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatalf("❌ Falha ao conectar no gRPC: %v", err)
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
		t.Fatalf("❌ Erro na chamada gRPC VectorSearch: %v", err)
	}

	// Asserções (O teste só passa se as condições forem atendidas)
	if res.TotalRows == 0 {
		t.Errorf("⚠️ Esperava encontrar resultados, mas retornou 0 linhas.")
	}
	if res.HandoffProof == "" {
		t.Errorf("⚠️ Handoff Proof (HMAC) veio vazio. Falha de segurança Zero-Trust.")
	}
	if len(res.SafePayload) == 0 {
		t.Errorf("⚠️ Payload JSON veio vazio.")
	}

	t.Logf("🎉 Busca concluída em %v", time.Since(startTime))
	t.Logf("📊 Linhas encontradas: %d", res.TotalRows)
	t.Logf("🛡️ HMAC Proof: %s", res.HandoffProof)
	t.Logf("📦 JSON Payload: %s", string(res.SafePayload))
}
