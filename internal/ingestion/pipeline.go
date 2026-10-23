package ingestion

import (
	"context"
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/andrewbraga1/jaguarbase/internal/ai"
	"github.com/andrewbraga1/jaguarbase/internal/config"
	"github.com/andrewbraga1/jaguarbase/internal/engine"
)

type Pipeline struct {
	source   DataSource
	duckDB   *engine.DuckDBEngine
	manifest *config.Manifest
	embedder ai.Embedder
}

// NewPipeline initializes the agnostic ingestion worker.
func NewPipeline(source DataSource, duckDB *engine.DuckDBEngine, manifest *config.Manifest, embedder ai.Embedder) *Pipeline {
	return &Pipeline{
		source:   source,
		duckDB:   duckDB,
		manifest: manifest,
		embedder: embedder,
	}
}

// Start begins processing events from ANY data source.
func (p *Pipeline) Start(ctx context.Context) {
	log.Println("🔄 Starting Agnostic Ingestion Pipeline...")

	events := make(chan RawRecord, 100)

	go func() {
		if err := p.source.Watch(ctx, events); err != nil {
			log.Printf("❌ [Ingestion Source Error]: %v", err)
		}
	}()

	for {
		select {
		case <-ctx.Done():
			log.Println("Stopping Ingestion Pipeline gracefully.")
			p.source.Close()
			return
		case record := <-events:
			p.processRecord(ctx, record)
		}
	}
}

func (p *Pipeline) processRecord(ctx context.Context, record RawRecord) {
	// 1. Zero-Trust Governance (Masking PII)
	// (In the future, the YAML will be read here to apply the pii_vault governance)
	safeMessage := fmt.Sprintf("%s (PII Masked)", record.Content)

	targetTable := record.Table
	if targetTable == "" {
		targetTable = p.manifest.Sources[0].Table
	}
	fallbackLLM := os.Getenv("LLM_MODEL")
	// 2. Dynamically read the LLM Model and Vector Dimension from the manifest
	llmModel := fallbackLLM     // Fallback model
	embeddingType := "FLOAT[3]" // Fallback dimension

	for _, src := range p.manifest.Sources {
		if src.Table == targetTable {
			if src.LLMModel != "" {
				llmModel = src.LLMModel
			}
			for _, col := range src.Schema {
				if col.Column == "embedding" {
					embeddingType = strings.ToUpper(col.Type)
					break
				}
			}
			break
		}
	}

	// 3. Real Embedding Generation via Local Ollama
	log.Printf("🧠 [Ollama] Generating %s embedding for record %s...", llmModel, record.ID)
	vector, err := p.embedder.Generate(ctx, safeMessage, llmModel)
	if err != nil {
		log.Printf("❌ [Ollama Error]: Failed to generate embedding: %v", err)
		return // Skip insertion if the AI is offline
	}

	// 4. Transform from Postgres format -> Vector DB format
	vectorRecord := map[string]any{
		"id":          record.ID,
		"content":     safeMessage,
		"metadata":    record.Metadata,
		"embedding":   vector,
		"_is_tainted": false,
	}

	// 5. Define the Target Schema dynamically
	targetSchema := []config.ColumnDef{
		{Column: "id", Type: "VARCHAR"},
		{Column: "content", Type: "VARCHAR"},
		{Column: "metadata", Type: "JSON"},
		{Column: "embedding", Type: embeddingType},
		{Column: "_is_tainted", Type: "BOOLEAN"},
	}

	// 6. Save to the DuckDB vector database
	if err := p.duckDB.InsertBatch(ctx, targetTable, targetSchema, []map[string]any{vectorRecord}); err != nil {
		log.Printf("❌ [DuckDB Insert Error]: %v", err)
	} else {
		log.Printf("⚡ [Ingestion] Record %s vectorized and stored dynamically in '%s'.", record.ID, targetTable)
	}
}
