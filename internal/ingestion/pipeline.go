package ingestion

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/andrewbraga1/jaguarbase/internal/ai"
	"github.com/andrewbraga1/jaguarbase/internal/config"
	"github.com/andrewbraga1/jaguarbase/internal/dynamic"
	"github.com/andrewbraga1/jaguarbase/internal/engine"
)

// Pipeline orchestrates the agnostic flow of data from raw sources to the vector engine,
// enforcing Zero-Trust governance (PII masking, taint checking) at the edge.
type Pipeline struct {
	source    DataSource
	duckDB    *engine.DuckDBEngine
	manifest  *config.Manifest
	embedder  ai.Embedder
	processor *dynamic.Processor
}

// NewPipeline initializes the agnostic ingestion worker.
func NewPipeline(source DataSource, duckDB *engine.DuckDBEngine, manifest *config.Manifest, embedder ai.Embedder, processor *dynamic.Processor) *Pipeline {
	return &Pipeline{
		source:    source,
		duckDB:    duckDB,
		manifest:  manifest,
		embedder:  embedder,
		processor: processor,
	}
}

// Start begins processing events asynchronously from the configured data source.
// It performs a Historical Backfill first, then transitions to Real-Time Streaming.
func (p *Pipeline) Start(ctx context.Context) {
	log.Println("🔄 Starting Agnostic Ingestion Pipeline...")

	// 1. PHASE 1: Historical Backfill (Carga Inicial)
	if err := p.performBackfill(ctx); err != nil {
		log.Printf("⚠️ [Ingestion Warning]: Backfill encountered an error (skipping to real-time): %v", err)
	}

	// 2. PHASE 2: Real-Time Streaming (CDC / Watch)
	log.Println("📡 Switching to Real-Time Streaming mode...")
	events := make(chan RawRecord, 100)

	go func() {
		if err := p.source.Watch(ctx, events); err != nil {
			log.Printf("❌ [Ingestion Source Error]: Real-time watch failed: %v", err)
		}
	}()

	for {
		select {
		case <-ctx.Done():
			log.Println("🛑 Stopping Ingestion Pipeline gracefully.")
			if err := p.source.Close(); err != nil {
				log.Printf("⚠️ Error closing ingestion source: %v", err)
			}
			return
		case record := <-events:
			p.processRecord(ctx, record)
		}
	}
}

// performBackfill executes the initial sync of legacy data before real-time streaming starts.
func (p *Pipeline) performBackfill(ctx context.Context) error {
	log.Println("🕰️ Initiating Historical Data Sync (Backfill)...")

	// Get configuration for the primary target table
	if len(p.manifest.Sources) == 0 {
		return fmt.Errorf("no sources defined in manifest")
	}
	srcConfig := p.manifest.Sources[0]
	tableName := srcConfig.Table
	contentCol := srcConfig.ContentColumn
	if contentCol == "" {
		contentCol = "content" // Fallback default if not specified
	}
	// Identify primary key and all columns required
	var pkColumn string
	var allColumns []string
	for _, col := range srcConfig.Schema {
		// Ignore the 'embedding' and '_is_tainted' columns since they don't exist in the source DB
		if !col.Virtual {
			allColumns = append(allColumns, col.Column)
		}
		if col.PrimaryKey {
			pkColumn = col.Column
		}
	}

	if pkColumn == "" {
		return fmt.Errorf("no primary key defined for table '%s' in manifest", tableName)
	}

	batchSize := 1000
	totalSynced := 0

	// Retrieve the highest ID currently stored in DuckDB to resume from where we left off
	// (Note: Requires a GetMaxID function in DuckDBEngine. Defaulting to 0 if not implemented yet.)
	lastID := p.duckDB.GetMaxID(ctx, tableName, pkColumn)
	log.Printf("📊 Resuming Backfill for '%s' from ID > %d...", tableName, lastID)

	for {
		// 1. Fetch a batch of historical records dynamically
		records, err := p.source.FetchBatch(ctx, tableName, allColumns, pkColumn, lastID, batchSize)
		if err != nil {
			return fmt.Errorf("FetchBatch failed: %w", err)
		}

		if len(records) == 0 {
			break // No more historical data to sync
		}

		// 2. Process each historical record through the Zero-Trust Governance and Embedding flow
		for _, rawMap := range records {
			// Convert the raw mapped data into the format processRecord expects
			// For backfill, the entire row is treated as Metadata, with Content and ID extracted
			var content string
			var idStr string

			if c, ok := rawMap[contentCol].(string); ok {
				content = c
			} else {
				// Fallback: Marshal the entire row as JSON if content column is missing or not a string
				contentBytes, _ := json.Marshal(rawMap)
				content = string(contentBytes)
			}
			if id, ok := rawMap[pkColumn]; ok {
				idStr = fmt.Sprintf("%v", id)
			}

			// Package as a RawRecord to reuse the battle-tested processRecord pipeline
			// We marshal rawMap to simulate the JSON payload coming from the real-time trigger
			metaBytes, _ := json.Marshal(rawMap)
			record := RawRecord{
				ID:       idStr,
				Table:    tableName,
				Content:  content,
				Metadata: string(metaBytes),
			}

			// Synchronous processing ensures we don't overwhelm the local LLM
			p.processRecord(ctx, record)

			// Update lastID for the next batch iteration
			// Assumes numeric IDs. For UUIDs, pagination logic would differ.
			// Update lastID for the next batch iteration robustly
			if rawID, exists := rawMap[pkColumn]; exists {
				var currentID int
				switch v := rawID.(type) {
				case int64:
					currentID = int(v)
				case int32:
					currentID = int(v)
				case int:
					currentID = v
				case float64: // Sometimes JSON unmarshaling forces numbers to float64
					currentID = int(v)
				}
				if currentID > lastID {
					lastID = currentID
				}
			}
		}

		totalSynced += len(records)
		log.Printf("⏳ Backfill progress: synced %d records so far...", totalSynced)

		// If we fetched less than the limit, we've hit the end of the table
		if len(records) < batchSize {
			break
		}
	}

	log.Printf("✅ Historical Sync Complete: %d legacy records securely vectorized and stored.", totalSynced)
	return nil
}

// processRecord handles the end-to-end flow for a single record: schema resolution,
// governance enforcement, embedding generation, and dynamic storage.
func (p *Pipeline) processRecord(ctx context.Context, record RawRecord) {
	targetTable := record.Table
	if targetTable == "" {
		targetTable = p.manifest.Sources[0].Table
	}

	// 1. Resolve table schema and configurations from the Manifest
	var tableSchema []config.ColumnDef
	llmModel := os.Getenv("LLM_MODEL")
	embeddingType := "FLOAT[3]" // Fallback dimension

	for _, src := range p.manifest.Sources {
		if src.Table == targetTable {
			tableSchema = src.Schema
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

	if len(tableSchema) == 0 {
		log.Printf("❌ [Ingestion Error]: Schema not found for target table '%s'", targetTable)
		return
	}

	// 2. Prepare dynamic map for the Governance Processor
	rawMap := make(map[string]any)

	// Safely unmarshal the raw JSON metadata string into the generic map
	if err := json.Unmarshal([]byte(record.Metadata), &rawMap); err != nil {
		log.Printf("⚠️ [Ingestion Warning]: Failed to parse metadata JSON for record %s: %v", record.ID, err)
	}

	// Ensure core fields are present for governance evaluation
	rawMap["content"] = record.Content
	rawMap["id"] = record.ID

	// 3. ZERO-TRUST GOVERNANCE: Apply PII Masking and Taint Check dynamically
	safeMap, err := p.processor.ProcessRow(ctx, rawMap, tableSchema)
	if err != nil {
		log.Printf("❌ [Governance Error]: Record %s rejected by Fail-Closed rules: %v", record.ID, err)
		return // Abort ingestion to prevent security leaks
	}

	// 4. Prepare the sanitized string for vectorization
	contentToVectorize := record.Content
	if safeContent, ok := safeMap["content"].(string); ok {
		contentToVectorize = safeContent
	}

	// Semantic marker indicating data masking to the LLM
	safeMessage := fmt.Sprintf("%s (PII Masked)", contentToVectorize)

	// 5. Generate Semantic Embeddings via Local LLM (Ollama)
	cleanModel := strings.ReplaceAll(strings.ReplaceAll(llmModel, "\n", ""), "\r", "")
	cleanID := strings.ReplaceAll(strings.ReplaceAll(record.ID, "\n", ""), "\r", "")
	log.Printf("🧠 [Ollama] Generating %s embedding for record %s...", cleanModel, cleanID)
	vector, err := p.embedder.Generate(ctx, safeMessage, llmModel)
	if err != nil {
		log.Printf("❌ [Ollama Error]: Failed to generate embedding for record %s: %v", record.ID, err)
		return
	}

	// 6. Restructure metadata payload for storage (filtering out top-level schema columns)
	metadataOut := make(map[string]any)
	for k, v := range safeMap {
		if k != "id" && k != "content" && k != "embedding" && k != "_is_tainted" && v != nil {
			metadataOut[k] = v
		}
	}

	metadataBytes, err := json.Marshal(metadataOut)
	if err != nil {
		log.Printf("⚠️ [Ingestion Warning]: Failed to marshal sanitized metadata for record %s: %v", record.ID, err)
		metadataBytes = []byte("{}") // Fallback to empty JSON object
	}

	// 7. Assemble the final unified record map matching the DuckDB target schema
	finalRecord := map[string]any{
		"id":          safeMap["id"],
		"content":     safeMessage,
		"metadata":    string(metadataBytes),
		"embedding":   vector,
		"_is_tainted": safeMap["_is_tainted"],
	}

	targetSchemaDef := []config.ColumnDef{
		{Column: "id", Type: "VARCHAR"},
		{Column: "content", Type: "VARCHAR"},
		{Column: "metadata", Type: "JSON"},
		{Column: "embedding", Type: embeddingType},
		{Column: "_is_tainted", Type: "BOOLEAN"},
	}

	// 8. Commit to Vector Storage (DuckDB)
	if err := p.duckDB.InsertBatch(ctx, targetTable, targetSchemaDef, []map[string]any{finalRecord}); err != nil {
		log.Printf("❌ [DuckDB Insert Error]: Failed to store record %s: %v", record.ID, err)
	} else {
		log.Printf("⚡ [Ingestion] Record %s vectorized and securely stored in '%s'.", record.ID, targetTable)
	}
}
