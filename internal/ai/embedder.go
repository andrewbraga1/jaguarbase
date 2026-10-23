package ai

import "context"

type Embedder interface {
	Generate(ctx context.Context, text string, model string) ([]float32, error)
}
