package pagination_test

import (
	"strings"
	"testing"

	"github.com/andrewbraga1/jaguarbase/internal/pagination"
)

func TestCognitivePaginator(t *testing.T) {
	secretKey := []byte("test-key")
	mockData := []map[string]any{
		{"id": 1, "message": "A"},
		{"id": 2, "message": "B"},
		{"id": 3, "message": "C"},
	}

	t.Run("Protects LLM Context (OOM Prevented)", func(t *testing.T) {
		// Budget is 2, Data is 3 -> Should block and sample
		output, err := pagination.BuildResponse(mockData, 2, secretKey)
		if err != nil {
			t.Fatalf("Unexpected error: %v", err)
		}

		jsonStr := string(output)
		if !strings.Contains(jsonStr, "COGNITIVE_OOM_PREVENTED") {
			t.Errorf("Expected OOM prevention, got: %s", jsonStr)
		}
		if !strings.Contains(jsonStr, "handoff_proof") {
			t.Errorf("Missing HMAC proof in response")
		}

		t.Logf("[PASS] Budget exceeded, safely truncated.")
	})
}
