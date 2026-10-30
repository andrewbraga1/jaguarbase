package ai

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"time"
)

// AiAdapter implementa a interface Embedder de forma agnóstica via chamadas REST.
type AiAdapter struct {
	APIUrl string
	client *http.Client
}

// NewAiAdapter cria o adapter injetando a URL base da API.
func NewAiAdapter(apiUrl string) *AiAdapter {
	llmHost := os.Getenv("LLM_API_URL")
	if apiUrl == "" {
		apiUrl = llmHost
	}
	return &AiAdapter{
		APIUrl: apiUrl,
		client: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// Generate dispara o payload JSON para a URL configurada no .env
func (a *AiAdapter) Generate(ctx context.Context, text string, model string) ([]float32, error) {
	reqBody := map[string]any{
		"model":  model,
		"prompt": text,
	}
	bodyBytes, _ := json.Marshal(reqBody)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, a.APIUrl, bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, fmt.Errorf("failed to create HTTP request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	httpClient := a.client
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 30 * time.Second}
	}

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to reach AI API at %s: %w", a.APIUrl, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("AI API returned status code: %d", resp.StatusCode)
	}

	var result struct {
		Embedding []float32 `json:"embedding"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode JSON response: %w", err)
	}

	return result.Embedding, nil
}
