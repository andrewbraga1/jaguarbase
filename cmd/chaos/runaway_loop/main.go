package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sync"
	"sync/atomic"
	"time"
)

const (
	MCPAddress    = "http://localhost:8080/mcp"
	TargetTable   = "support_tickets"
	Concurrency   = 1000 // A burst of 1,000 requests all at once
	TotalRequests = 2000
)

func main() {
	fmt.Println("🌀 JaguarBase Chaos Engineering: Runaway Loop (Load Shedding)")
	fmt.Println("⚠️  Make sure JaguarBase is running in another terminal (CGO_ENABLED=1 go run cmd/jaguarbase/main.go)")
	fmt.Println("🚀 Firing a massive burst of HTTP JSON-RPC requests to saturate the port...")

	// Create a dummy vector payload
	dummyVector := make([]float32, 1536)
	for i := 0; i < 1536; i++ {
		dummyVector[i] = 0.1
	}

	payload := map[string]any{
		"jsonrpc": "2.0",
		"id":      1,
		"method":  "tools/call",
		"params": map[string]any{
			"name": "vector_search",
			"arguments": map[string]any{
				"table_name":   TargetTable,
				"query_vector": dummyVector,
				"limit":        1,
			},
		},
	}
	payloadBytes, _ := json.Marshal(payload)

	var successCount, overloadCount, errorCount atomic.Uint64
	var wg sync.WaitGroup

	reqPool := make(chan int, TotalRequests)
	for i := 0; i < TotalRequests; i++ {
		reqPool <- i
	}
	close(reqPool)

	t := http.DefaultTransport.(*http.Transport).Clone()
	t.MaxIdleConns = Concurrency
	t.MaxConnsPerHost = Concurrency
	httpClient := &http.Client{Transport: t, Timeout: 5 * time.Second}

	startTime := time.Now()

	for w := 0; w < Concurrency; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range reqPool {
				req, _ := http.NewRequest(http.MethodPost, MCPAddress, bytes.NewBuffer(payloadBytes))
				req.Header.Set("Content-Type", "application/json")

				resp, err := httpClient.Do(req)
				if err != nil {
					errorCount.Add(1)
					continue
				}

				io.Copy(io.Discard, resp.Body)
				resp.Body.Close()

				switch resp.StatusCode {
				case http.StatusOK:
					successCount.Add(1)
				case http.StatusTooManyRequests:
					overloadCount.Add(1)
				default:
					errorCount.Add(1)
				}
			}
		}()
	}

	wg.Wait()
	duration := time.Since(startTime)

	fmt.Println("\n📊 === CHAOS TEST RESULTS (RUNAWAY LOOP) ===")
	fmt.Printf("⏱️  Test Duration    : %v\n", duration)
	fmt.Printf("✅ Success (200 OK)  : %d (Processed by DuckDB)\n", successCount.Load())
	fmt.Printf("🛡️  Shedded (429)     : %d (Rejected by Go Router before DB)\n", overloadCount.Load())
	fmt.Printf("❌ Failed (Network)  : %d\n", errorCount.Load())

	if overloadCount.Load() > 0 {
		fmt.Println("🏆 VICTORY: The Load Shedding barrier successfully protected the database!")
	} else {
		fmt.Println("⚠️  WARNING: No requests were shed. The database might have absorbed the full hit.")
	}
}
