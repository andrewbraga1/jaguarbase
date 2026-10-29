package main

import (
	"context"
	"fmt"
	"log"
	"math/rand"
	"os"
	"runtime"
	"time"

	"github.com/andrewbraga1/jaguarbase/internal/config"
	"github.com/andrewbraga1/jaguarbase/internal/engine"
)

const (
	DBFile          = "chaos_breaking.duckdb"
	TableName       = "breaking_tickets"
	BatchSize       = 10000
	TargetMax       = 1000000 // 1 Million vectors
	MaxTestDuration = 5 * time.Minute
	MaxDiskMB       = 2000.0 // Fail-Safe: Stop if DB reaches 2GB
	MaxMemoryMB     = 1024.0 // Fail-Safe: Stop if Go process uses > 1GB RAM
)

// getFileSizeMB returns the physical disk size of the database
func getFileSizeMB(filename string) float64 {
	info, err := os.Stat(filename)
	if err != nil {
		return 0
	}
	return float64(info.Size()) / 1024 / 1024
}

// getProcessMemoryMB returns the current RAM allocated by the Go process
func getProcessMemoryMB() float64 {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	return float64(m.Alloc) / 1024 / 1024
}

func main() {
	fmt.Println("🔥 JaguarBase Chaos Engineering: Breaking Point (OOM / Disk Limits)")
	fmt.Printf("📂 Database: Local File (%s)\n", DBFile)

	os.Remove(DBFile)
	os.Remove(DBFile + ".wal")

	// 1. Guardrail: TTL Context (Time-To-Live)
	ctx, cancel := context.WithTimeout(context.Background(), MaxTestDuration)
	defer cancel()

	db, err := engine.NewDuckDBEngine(ctx, DBFile)
	if err != nil {
		log.Fatalf("❌ Failed to start DuckDB: %v", err)
	}
	defer db.Close()

	schema := []config.ColumnDef{
		{Column: "id", Type: "string"},
		{Column: "content", Type: "string"},
		{Column: "metadata", Type: "string"},
		{Column: "_is_tainted", Type: "BOOLEAN"},
		{Column: "embedding", Type: "vector", Dimensions: 1536},
	}
	if err := db.InitSchema(ctx, TableName, schema); err != nil {
		log.Fatalf("❌ InitSchema error: %v", err)
	}

	dummyVector := make([]float32, 1536)
	for i := 0; i < 1536; i++ {
		dummyVector[i] = rand.Float32()
	}

	fmt.Printf("🚀 Target: %d vectors (Batches of %d)\n", TargetMax, BatchSize)
	fmt.Println("🛡️  Automated Safety Guardrails ENABLED:")
	fmt.Printf("   - TTL Limit : %v\n", MaxTestDuration)
	fmt.Printf("   - Disk Limit: %.2f MB\n", MaxDiskMB)
	fmt.Printf("   - RAM Limit : %.2f MB\n", MaxMemoryMB)
	fmt.Println("---------------------------------------------------------")

	startTime := time.Now()
	totalInserted := 0
	stopReason := "Target Successfully Reached"

IngestionLoop:
	for totalInserted < TargetMax {
		// Check TTL Guardrail
		select {
		case <-ctx.Done():
			stopReason = "Safety Abort: TTL Expired"
			break IngestionLoop
		default:
		}

		// Check Memory Guardrail
		currentMem := getProcessMemoryMB()
		if currentMem > MaxMemoryMB {
			stopReason = fmt.Sprintf("Safety Abort: Memory limit exceeded (%.2f MB)", currentMem)
			break IngestionLoop
		}

		// Check Disk Guardrail
		currentDisk := getFileSizeMB(DBFile)
		if currentDisk > MaxDiskMB {
			stopReason = fmt.Sprintf("Safety Abort: Disk limit exceeded (%.2f MB)", currentDisk)
			break IngestionLoop
		}

		batchStart := time.Now()
		var rows []map[string]any

		for i := 0; i < BatchSize; i++ {
			rows = append(rows, map[string]any{
				"id":          fmt.Sprintf("ticket_%d", totalInserted+i),
				"content":     "Breaking point stress test data",
				"metadata":    `{"status":"stress"}`,
				"embedding":   dummyVector,
				"_is_tainted": false,
			})
		}

		err := db.InsertBatch(ctx, TableName, schema, rows)
		if err != nil {
			stopReason = fmt.Sprintf("Engine Crash / DuckDB Error: %v", err)
			break IngestionLoop
		}

		totalInserted += BatchSize
		batchDuration := time.Since(batchStart)

		fmt.Printf("✅ Inserted: %7d / %d | Batch Time: %6v | RAM: %6.2f MB | Disk: %7.2f MB\n",
			totalInserted, TargetMax, batchDuration.Round(time.Millisecond), getProcessMemoryMB(), getFileSizeMB(DBFile))
	}

	duration := time.Since(startTime)

	fmt.Println("\n📊 === CHAOS TEST RESULTS (BREAKING POINT) ===")
	fmt.Printf("⏱️  Total Duration : %v\n", duration)
	fmt.Printf("📈 Total Vectors  : %d\n", totalInserted)
	fmt.Printf("🧠 Final RAM Usage: %.2f MB\n", getProcessMemoryMB())
	fmt.Printf("💾 Final Disk Size: %.2f MB\n", getFileSizeMB(DBFile))
	fmt.Printf("🛑 Stop Reason    : %s\n", stopReason)

	// Clean up automatically
	db.Close()
	os.Remove(DBFile)
	os.Remove(DBFile + ".wal")
	fmt.Println("🧹 Cleanup complete. Disk space restored.")
}
