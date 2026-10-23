package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"math/rand"
	"os"
	"time"

	"github.com/andrewbraga1/jaguarbase/internal/config"
	"github.com/andrewbraga1/jaguarbase/internal/engine"
)

const (
	DBFile    = "chaos_phantom.duckdb"
	TableName = "phantom_tickets"
	MockRows  = 50000 // 50k vectors for a heavy computational load
)

func main() {
	fmt.Println("👻 JaguarBase Chaos Engineering: Phantom Agent (Context Cancellation)")
	fmt.Printf("📂 Database: Local File (%s)\n", DBFile)

	os.Remove(DBFile)
	os.Remove(DBFile + ".wal")

	setupCtx := context.Background()
	db, err := engine.NewDuckDBEngine(setupCtx, DBFile)
	if err != nil {
		log.Fatalf("❌ Failed to start DuckDB: %v", err)
	}
	defer db.Close()

	schema := []config.ColumnDef{
		{Column: "id", Type: "string"},
		{Column: "content", Type: "string"},
		{Column: "metadata", Type: "string"},
		{Column: "embedding", Type: "vector", Dimensions: 1536},
	}
	if err := db.InitSchema(setupCtx, TableName, schema); err != nil {
		log.Fatalf("❌ InitSchema error: %v", err)
	}

	dummyVector := make([]float32, 1536)
	for i := 0; i < 1536; i++ {
		dummyVector[i] = rand.Float32()
	}

	fmt.Printf("⏳ Populating DB with %d vectors (1536-dim) to simulate heavy load...\n", MockRows)
	var rows []map[string]any
	for i := 0; i < MockRows; i++ {
		rows = append(rows, map[string]any{
			"id":          fmt.Sprintf("ticket_%d", i),
			"content":     "Heavy load data",
			"metadata":    `{"status":"pending"}`,
			"embedding":   dummyVector,
			"_is_tainted": false,
		})
	}
	if err := db.InsertBatch(setupCtx, TableName, schema, rows); err != nil {
		log.Fatalf("❌ Failed to populate data: %v", err)
	}
	fmt.Println("✅ Data populated successfully.")
	fmt.Println()

	// ==========================================
	// METRICS PHASE 1: THE BASELINE
	// ==========================================
	fmt.Println("📈 Phase 1: Running Baseline Query (Full Execution)...")
	startBase := time.Now()
	_, err = db.SemanticSearch(context.Background(), TableName, dummyVector, 100)
	if err != nil {
		log.Fatalf("Baseline query failed: %v", err)
	}
	baseDuration := time.Since(startBase)
	fmt.Printf("⏱️  Baseline completed in: %v\n\n", baseDuration)

	// ==========================================
	// METRICS PHASE 2: THE PHANTOM AGENT
	// ==========================================
	fmt.Println("👻 Phase 2: Running Phantom Query (Client Disconnect at 1ms)...")
	queryCtx, cancel := context.WithTimeout(context.Background(), 1*time.Millisecond)
	defer cancel()

	startPhantom := time.Now()
	_, err = db.SemanticSearch(queryCtx, TableName, dummyVector, 100)
	phantomDuration := time.Since(startPhantom)

	// ==========================================
	// FINAL OBSERVABILITY REPORT
	// ==========================================
	fmt.Println("\n📊 === CHAOS TEST METRICS (CONTEXT CANCELLATION) ===")
	fmt.Printf("⏱️  Baseline Compute Time : %v\n", baseDuration)
	fmt.Printf("⏱️  Phantom Abort Time    : %v\n", phantomDuration)

	if err != nil && (errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled)) {
		timeSaved := baseDuration - phantomDuration
		cpuSavedPercent := (float64(timeSaved) / float64(baseDuration)) * 100

		fmt.Println("🛡️  SUCCESS: Context Cancelation respected by DuckDB!")
		fmt.Printf("📄 Error caught          : %v\n", err)
		fmt.Printf("💾 CPU Time Saved        : %v\n", timeSaved)
		fmt.Printf("⚡ Resources Preserved   : %.2f%%\n", cpuSavedPercent)
	} else {
		fmt.Println("❌ FAILURE: Context cancellation was ignored. Resource Leak detected!")
	}

	// Clean up
	db.Close()
	os.Remove(DBFile)
	os.Remove(DBFile + ".wal")
}
