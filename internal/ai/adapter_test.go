package ai_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/andrewbraga1/jaguarbase/internal/ai"
)

func TestAiAdapter_Generate(t *testing.T) {
	t.Run("Successfully generates embedding vector", func(t *testing.T) {
		mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method != http.MethodPost {
				t.Errorf("Expected POST request, got %s", r.Method)
			}
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"embedding": [0.12, -0.34, 0.56]}`))
		}))
		defer mockServer.Close()

		adapter := ai.NewAiAdapter(mockServer.URL)
		vec, err := adapter.Generate(context.Background(), "test prompt", "qwen2.5-coder:1.5b")
		if err != nil {
			t.Fatalf("Expected success, got error: %v", err)
		}
		if len(vec) != 3 || vec[0] != 0.12 {
			t.Fatalf("Unexpected embedding vector: %v", vec)
		}
		t.Logf("[PASS] AiAdapter generated embedding vector: %v", vec)
	})

	t.Run("Aborts when context times out on slow LLM", func(t *testing.T) {
		slowServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			time.Sleep(200 * time.Millisecond)
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"embedding": [0.1, 0.2, 0.3]}`))
		}))
		defer slowServer.Close()

		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
		defer cancel()

		adapter := ai.NewAiAdapter(slowServer.URL)
		_, err := adapter.Generate(ctx, "slow prompt", "qwen2.5-coder:1.5b")
		if err == nil {
			t.Fatalf("Expected timeout error on hanging LLM, got nil")
		}
		t.Logf("[PASS] Slow LLM request aborted on timeout: %v", err)
	})
}

