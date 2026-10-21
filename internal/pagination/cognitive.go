package pagination

import (
	"encoding/json"
	"fmt"

	"github.com/andrewbraga1/jaguarbase/internal/engine"
	"github.com/andrewbraga1/jaguarbase/internal/security"
)

// CognitiveResponse is the standard Zero-Trust payload sent to the LLM agent via MCP.
type CognitiveResponse struct {
	Status       string          `json:"status"`
	TotalRows    int             `json:"total_rows"`
	Message      string          `json:"message,omitempty"`
	Data         []engine.Ticket `json:"data,omitempty"`
	SampleItems  []engine.Ticket `json:"sample_items,omitempty"`
	HandoffProof string          `json:"handoff_proof,omitempty"`
}

// BuildResponse evaluates the token budget and signs the output payload.
func BuildResponse(tickets []engine.Ticket, budget int, secretKey []byte) ([]byte, error) {
	var resp CognitiveResponse

	if len(tickets) > budget {
		// Truncate to protect LLM context window
		sampleSize := 2
		if len(tickets) < 2 {
			sampleSize = len(tickets)
		}

		resp = CognitiveResponse{
			Status:      "COGNITIVE_OOM_PREVENTED",
			TotalRows:   len(tickets),
			Message:     "Payload exceeds cognitive budget. Please refine your query.",
			SampleItems: tickets[:sampleSize],
		}
	} else {
		// Full safe payload
		resp = CognitiveResponse{
			Status:    "SUCCESS",
			TotalRows: len(tickets),
			Data:      tickets,
		}
	}

	// Sign the payload (excluding the proof field itself)
	rawJSON, err := json.Marshal(resp)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal base response: %w", err)
	}

	resp.HandoffProof = security.GenerateFrameProof(secretKey, rawJSON)

	// Final marshal with the proof attached
	finalJSON, err := json.Marshal(resp)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal final response: %w", err)
	}

	return finalJSON, nil
}
