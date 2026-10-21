package pagination_test

import (
	"strings"
	"testing"

	"github.com/andrewbraga1/jaguarbase/internal/engine"
	"github.com/andrewbraga1/jaguarbase/internal/pagination"
)

func TestCognitivePaginator(t *testing.T) {
	secretKey := []byte("test-key")
	mockData := []engine.Ticket{
		{ID: 1, Message: "A"}, {ID: 2, Message: "B"}, {ID: 3, Message: "C"},
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
