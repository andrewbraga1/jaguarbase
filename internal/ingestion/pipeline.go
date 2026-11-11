package ingestion

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"math"
	"os"
	"strings"
	"time"

	"github.com/andrewbraga1/jaguarbase/internal/ai"
	"github.com/andrewbraga1/jaguarbase/internal/config"
	"github.com/andrewbraga1/jaguarbase/internal/dynamic"
	"github.com/andrewbraga1/jaguarbase/internal/engine"
)

// sanitizeLog strips CRLF characters from string inputs to prevent log injection vulnerabilities.
func sanitizeLog(val string) string {
	return strings.ReplaceAll(strings.ReplaceAll(val, "\n", ""), "\r", "")
}

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
// It starts the real-time event watcher first to avoid event loss during backfill,
// runs the historical backfill, and processes incoming events with idempotent upsert.
func (p *Pipeline) Start(ctx context.Context) {
	log.Println("🔄 Starting Agnostic Ingestion Pipeline...")

	// 1. PHASE 1: Start Real-Time Watcher immediately to eliminate event loss window
	events := make(chan RawRecord, 2048)

	go func() {
		if err := p.source.Watch(ctx, events); err != nil {
			log.Printf("❌ [Ingestion Source Error]: Real-time watch failed: %v", err)
		}
	}()

	// Wait for watcher readiness if the source supports ReadyNotifier to eliminate race window
	if readySource, ok := p.source.(ReadyNotifier); ok {
		log.Println("⏳ Waiting for data source watcher to become ready...")
		select {
		case <-readySource.Ready():
			log.Println("✅ Data source watcher is actively listening.")
		case <-time.After(5 * time.Second):
			log.Println("⚠️ Data source watcher readiness timed out after 5s; proceeding with backfill.")
		case <-ctx.Done():
			return
		}
	}

	// 2. PHASE 2: Historical Backfill (Carga Inicial)
	if err := p.performBackfill(ctx); err != nil {
		log.Printf("⚠️ [Ingestion Warning]: Backfill encountered an error: %v", err)
	}

	// 3. PHASE 3: Real-Time Event Ingestion
	log.Println("📡 Switching to Real-Time Streaming mode...")
	for {
		select {
		case <-ctx.Done():
			log.Println("🛑 Stopping Ingestion Pipeline gracefully.")
			if err := p.source.Close(); err != nil {
				log.Printf("⚠️ Error closing ingestion source: %v", err)
			}
			return
		case record := <-events:
			if err := p.processRecord(ctx, record); err != nil {
				log.Printf("❌ [Real-time Ingestion Error]: Failed to process record %s: %v", sanitizeLog(record.ID), err)
			}
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

	// Retrieve checkpoint from isolated metadata store to decouple backfill from real-time streaming events
	checkpointKey := "backfill_checkpoint_" + tableName
	lastID := 0
	if p.duckDB != nil {
		if cp, err := p.duckDB.GetCheckpoint(ctx, checkpointKey); err == nil && cp > 0 {
			lastID = cp
		}
	}
	log.Printf("📊 Resuming Backfill for '%s' from checkpoint ID > %d...", tableName, lastID)

	for {
		// 1. Fetch a batch of historical records dynamically
		records, err := p.source.FetchBatch(ctx, tableName, allColumns, pkColumn, lastID, batchSize)
		if err != nil {
			return fmt.Errorf("FetchBatch failed: %w", err)
		}

		if len(records) == 0 {
			break // No more historical data to sync
		}

		batchMaxID := lastID

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

			// Synchronous processing with retries ensures we don't overwhelm the local LLM
			var procErr error
			for attempt := 1; attempt <= 3; attempt++ {
				procErr = p.processRecord(ctx, record)
				if procErr == nil {
					break
				}
				log.Printf("⚠️ [Backfill Retry %d/3] Failed to process record %s: %v", attempt, sanitizeLog(record.ID), procErr)
				select {
				case <-ctx.Done():
					return ctx.Err()
				case <-time.After(time.Duration(attempt*200) * time.Millisecond):
				}
			}
			if procErr != nil {
				return fmt.Errorf("backfill halted at record %s (lastID=%d): %w", record.ID, lastID, procErr)
			}

			// Update batchMaxID ONLY after successful processing (with integer overflow defense)
			if rawID, exists := rawMap[pkColumn]; exists {
				var currentID int
				switch v := rawID.(type) {
				case int64:
					if v > math.MaxInt {
						currentID = math.MaxInt
					} else {
						currentID = int(v)
					}
				case int32:
					currentID = int(v)
				case int:
					currentID = v
				case float64:
					if v > float64(math.MaxInt) {
						currentID = math.MaxInt
					} else {
						currentID = int(v)
					}
				}
				if currentID > batchMaxID {
					batchMaxID = currentID
				}
			}
		}

		// Persist durable checkpoint after successful batch processing
		if batchMaxID > lastID {
			lastID = batchMaxID
			if p.duckDB != nil {
				if err := p.duckDB.SetCheckpoint(ctx, checkpointKey, lastID); err != nil {
					log.Printf("⚠️ Failed to persist backfill checkpoint for %s (ID %d): %v", sanitizeLog(tableName), lastID, err)
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

	if p.duckDB != nil {
		completeKey := "backfill_complete_" + tableName
		if err := p.duckDB.SetCheckpoint(ctx, completeKey, 1); err != nil {
			log.Printf("⚠️ Failed to mark backfill complete for %s: %v", sanitizeLog(tableName), err)
		}
	}

	log.Printf("✅ Historical Sync Complete: %d legacy records securely vectorized and stored.", totalSynced)
	return nil
}

// processRecord handles the end-to-end flow for a single record: schema resolution,
// governance enforcement, embedding generation, and dynamic storage.
func (p *Pipeline) processRecord(ctx context.Context, record RawRecord) error {
	targetTable := record.Table
	if targetTable == "" {
		targetTable = p.manifest.Sources[0].Table
	}

	// 0. Handle Deletion / Tombstone event
	if record.IsDeleted {
		log.Printf("🗑️ [Ingestion] Purging deleted record %s from vector cache '%s'...", sanitizeLog(record.ID), sanitizeLog(targetTable))
		if err := p.duckDB.DeleteRecord(ctx, targetTable, "id", record.ID); err != nil {
			return fmt.Errorf("failed to purge record %s from duckdb: %w", record.ID, err)
		}
		log.Printf("✅ [Ingestion] Successfully purged record %s from vector cache '%s'.", sanitizeLog(record.ID), sanitizeLog(targetTable))
		return nil
	}

	// 1. Resolve table schema and configurations from the Manifest
	var tableSchema []config.ColumnDef
	llmModel := os.Getenv("LLM_MODEL")
	embeddingType := "FLOAT[3]" // Fallback dimension
	contentColName := "content" // Fallback content column

	for _, src := range p.manifest.Sources {
		if src.Table == targetTable {
			tableSchema = src.Schema
			if src.ContentColumn != "" {
				contentColName = src.ContentColumn
			}
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
		return fmt.Errorf("schema not found for target table '%s'", targetTable)
	}

	// 2. Prepare dynamic map for the Governance Processor
	rawMap := make(map[string]any)

	// Safely unmarshal the raw JSON metadata string into the generic map
	if err := json.Unmarshal([]byte(record.Metadata), &rawMap); err != nil {
		log.Printf("⚠️ [Ingestion Warning]: Failed to parse metadata JSON for record %s: %v", sanitizeLog(record.ID), err)
	}

	// Ensure core fields are mapped to their canonical schema definitions
	if record.Content != "" {
		rawMap[contentColName] = record.Content
		rawMap["content"] = record.Content
	} else if val, ok := rawMap[contentColName].(string); ok {
		record.Content = val
		rawMap["content"] = val
	}
	rawMap["id"] = record.ID

	// 3. ZERO-TRUST GOVERNANCE: Apply PII Masking and Taint Check dynamically
	safeMap, err := p.processor.ProcessRow(ctx, rawMap, tableSchema)
	if err != nil {
		log.Printf("❌ [Governance Error]: Record %s rejected by Fail-Closed rules: %v", sanitizeLog(record.ID), err)
		return fmt.Errorf("governance failure for record %s: %w", record.ID, err)
	}

	// 4. Prepare the sanitized string for vectorization (Fail-Closed: never fall back to raw uninspected content)
	var contentToVectorize string
	if safeContent, ok := safeMap[contentColName].(string); ok && safeContent != "" {
		contentToVectorize = safeContent
	} else if safeContent, ok := safeMap["content"].(string); ok && safeContent != "" {
		contentToVectorize = safeContent
	} else {
		return fmt.Errorf("fail-closed: content column '%s' missing or empty in governance output for record %s", contentColName, record.ID)
	}

	// Keep canonical content synchronized with the sanitized value
	safeMap["content"] = contentToVectorize
	safeMessage := contentToVectorize

	// 5. Generate Semantic Embeddings via Local LLM (Ollama)
	cleanModel := sanitizeLog(llmModel)
	cleanID := sanitizeLog(record.ID)
	log.Printf("🧠 [Ollama] Generating %s embedding for record %s...", cleanModel, cleanID)
	vector, err := p.embedder.Generate(ctx, safeMessage, llmModel)
	if err != nil {
		log.Printf("❌ [Ollama Error]: Failed to generate embedding for record %s: %v", cleanID, err)
		return fmt.Errorf("embedding generation failed for record %s: %w", record.ID, err)
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
		log.Printf("⚠️ [Ingestion Warning]: Failed to marshal sanitized metadata for record %s: %v", cleanID, err)
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
		{Column: "id", Type: "VARCHAR", PrimaryKey: true},
		{Column: "content", Type: "VARCHAR"},
		{Column: "metadata", Type: "JSON"},
		{Column: "embedding", Type: embeddingType},
		{Column: "_is_tainted", Type: "BOOLEAN"},
	}

	// 8. Commit to Vector Storage (DuckDB with idempotent upsert)
	if err := p.duckDB.InsertBatch(ctx, targetTable, targetSchemaDef, []map[string]any{finalRecord}); err != nil {
		log.Printf("❌ [DuckDB Insert Error]: Failed to store record %s: %v", cleanID, err)
		return fmt.Errorf("duckdb insert failed for record %s: %w", record.ID, err)
	}

	log.Printf("⚡ [Ingestion] Record %s vectorized and securely stored in '%s'.", record.ID, targetTable)
	return nil
}
