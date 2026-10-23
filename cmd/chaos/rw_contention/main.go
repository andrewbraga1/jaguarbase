package main

import (
	"context"
	"fmt"
	"log"
	"math/rand"
	"os"
	"sync"
	"sync/atomic"
	"time"

	"github.com/andrewbraga1/jaguarbase/internal/config"
	"github.com/andrewbraga1/jaguarbase/internal/engine"
)

const (
	DBFile      = "chaos_test.duckdb"
	TableName   = "chaos_tickets"
	ReadWorkers = 100 // 100 AI Agents reading concurrently
	WriteFreq   = 50  // Milliseconds between each Batch Insert (CDC simulation)
	TestTime    = 10  // Test duration in seconds
)

func main() {
	fmt.Println("🌪️  JaguarBase Chaos Engineering: Read/Write Contention")
	fmt.Printf("📂 Database: Local File (%s)\n", DBFile)

	// Clean up previous test database if it exists
	os.Remove(DBFile)
	os.Remove(DBFile + ".wal") // Remove Write-Ahead Log just in case

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// 1. Initialize Engine on Physical Disk
	db, err := engine.NewDuckDBEngine(ctx, DBFile)
	if err != nil {
		log.Fatalf("❌ Failed to start DuckDB: %v", err)
	}
	defer db.Close()

	// 2. Initialize Agnostic Schema
	schema := []config.ColumnDef{
		{Column: "id", Type: "string"},
		{Column: "content", Type: "string"},
		{Column: "metadata", Type: "string"},
		{Column: "embedding", Type: "vector", Dimensions: 1536},
	}
	if err := db.InitSchema(ctx, TableName, schema); err != nil {
		log.Fatalf("❌ InitSchema error: %v", err)
	}

	var readCount, readErrors, writeCount, writeErrors atomic.Uint64
	var wg sync.WaitGroup

	// Create a dummy 1536-dimensional vector
	dummyVector := make([]float32, 1536)
	for i := 0; i < 1536; i++ {
		dummyVector[i] = rand.Float32()
	}

	fmt.Println("🚀 Starting simultaneous Read/Write bombardment...")
	startTime := time.Now()

	// ==========================================
	// WRITER THREAD (Simulating CDC Ingestion)
	// ==========================================
	wg.Add(1)
	go func() {
		defer wg.Done()
		ticker := time.NewTicker(time.Duration(WriteFreq) * time.Millisecond)
		defer ticker.Stop()
		idCounter := 0

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				idCounter++
				rows := []map[string]any{
					{
						"id":          fmt.Sprintf("ticket_%d", idCounter),
						"content":     "Chaos Content",
						"metadata":    `{"source":"chaos"}`,
						"embedding":   dummyVector,
						"_is_tainted": false,
					},
				}
				err := db.InsertBatch(ctx, TableName, schema, rows)
				if err != nil {
					writeErrors.Add(1)
				} else {
					writeCount.Add(1)
				}
			}
		}
	}()

	// ==========================================
	// READER THREADS (Simulating AI Agents via MCP)
	// ==========================================
	for i := 0; i < ReadWorkers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-ctx.Done():
					return
				default:
					// Fire semantic searches relentlessly
					_, err := db.SemanticSearch(ctx, TableName, dummyVector, 5)
					if err != nil {
						readErrors.Add(1)
					} else {
						readCount.Add(1)
					}
				}
			}
		}()
	}

	// Allow Chaos to run for the specified time
	time.Sleep(TestTime * time.Second)
	cancel() // Send kill signal to all goroutines
	wg.Wait()

	duration := time.Since(startTime)

	fmt.Println("\n📊 === CHAOS TEST RESULTS (R/W CONTENTION) ===")
	fmt.Printf("⏱️  Test Duration: %v\n", duration)
	fmt.Printf("📝 Batches Written (SSD): %d (Errors: %d)\n", writeCount.Load(), writeErrors.Load())
	fmt.Printf("🔍 Semantic Searches Read: %d (Errors: %d)\n", readCount.Load(), readErrors.Load())

	readsPerSec := float64(readCount.Load()) / duration.Seconds()
	fmt.Printf("⚡ Read Throughput under stress: %.2f req/s\n", readsPerSec)

	// Clean up test files
	db.Close()
	os.Remove(DBFile)
	os.Remove(DBFile + ".wal")
}
