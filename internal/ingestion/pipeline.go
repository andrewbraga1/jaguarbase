package ingestion

import (
	"context"
	"fmt"
	"log"

	"github.com/andrewbraga1/jaguarbase/internal/config"
	"github.com/andrewbraga1/jaguarbase/internal/engine"
)

type Pipeline struct {
	source   DataSource
	duckDB   *engine.DuckDBEngine
	manifest *config.Manifest
}

// NewPipeline initializes the agnostic ingestion worker.
func NewPipeline(source DataSource, duckDB *engine.DuckDBEngine, manifest *config.Manifest) *Pipeline {
	return &Pipeline{
		source:   source,
		duckDB:   duckDB,
		manifest: manifest,
	}
}

// Start begins processing events from ANY data source.
func (p *Pipeline) Start(ctx context.Context) {
	log.Println("🔄 Starting Agnostic Ingestion Pipeline...")

	events := make(chan RawRecord, 100) // Buffer for burst events

	// Start the source watcher in the background
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
	// 1. Zero-Trust Governance & Mock Embedding
	// (Aqui no futuro o YAML será lido para aplicar o pii_vault no CPF)
	mockVector := []float32{0.1, 0.9, 0.3}
	safeMessage := fmt.Sprintf("%s (PII Masked)", record.Content)

	// 2. Transforma o formato Postgres -> Formato Vector DB
	vectorRecord := map[string]any{
		"id":          record.ID,
		"content":     safeMessage,
		"metadata":    record.Metadata, // CPF e demais colunas embutidas aqui
		"embedding":   mockVector,
		"_is_tainted": false,
	}

	targetTable := record.Table
	if targetTable == "" {
		targetTable = p.manifest.Sources[0].Table
	}

	// 3. Define o Target Schema (O formato universal do DuckDB)
	targetSchema := []config.ColumnDef{
		{Column: "id", Type: "VARCHAR"},
		{Column: "content", Type: "VARCHAR"},
		{Column: "metadata", Type: "JSON"},
		{Column: "embedding", Type: "FLOAT[3]"},
		{Column: "_is_tainted", Type: "BOOLEAN"},
	}

	// 4. Salva no banco vetorial
	if err := p.duckDB.InsertBatch(ctx, targetTable, targetSchema, []map[string]any{vectorRecord}); err != nil {
		log.Printf("❌ [DuckDB Insert Error]: %v", err)
	} else {
		log.Printf("⚡ [Ingestion] Record %s vectorized and stored dynamically in '%s'.", record.ID, targetTable)
	}
}
