package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	pb "github.com/andrewbraga1/jaguarbase/pkg/api/v1/pb"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

// Config defines the stress test parameters
const (
	TotalRequestsLight = 100000
	TotalRequestsHeavy = 10000
	Concurrency        = 1000
	GRPCAddress        = "localhost:50051"
	MCPAddress         = "http://localhost:8080/mcp"
	TargetTable        = "support_tickets"
)

type TestResult struct {
	Name      string
	Duration  time.Duration
	ReqPerSec float64
	Success   uint64
	Errors    uint64
}

func main() {
	fmt.Println("🐆 JaguarBase Comprehensive Stress Test Suite")
	fmt.Println("==================================================")
	fmt.Printf("Concurrency Level: %d simultaneous workers\n", Concurrency)
	fmt.Println("Initializing benchmarks...")
	fmt.Println()

	// Generate a dummy 1536-dimensional vector for realistic OpenAI-level load
	dummyVector := make([]float32, 1536)
	for i := 0; i < 1536; i++ {
		dummyVector[i] = 0.1
	}

	var results []TestResult

	// 1. gRPC HealthCheck (Network Throughput Focus)
	res1 := runGRPCHealthStress(TotalRequestsLight, Concurrency)
	results = append(results, res1)
	time.Sleep(2 * time.Second) // Cooldown period

	// 2. gRPC Vector Search (DB Compute Focus)
	res2 := runGRPCVectorStress(TotalRequestsHeavy, Concurrency, dummyVector)
	results = append(results, res2)
	time.Sleep(2 * time.Second)

	// 3. MCP / HTTP JSON-RPC (REST/Agent Load Focus)
	res3 := runMCPHttpStress(TotalRequestsHeavy, Concurrency, dummyVector)
	results = append(results, res3)

	// Print Final Report
	fmt.Println("\n📊 === FINAL BENCHMARK REPORT ===")
	fmt.Printf("%-30s | %-12s | %-15s | %-10s | %-10s\n", "SCENARIO", "DURATION", "REQ/SEC", "SUCCESS", "ERRORS")
	fmt.Println("-----------------------------------------------------------------------------------------")
	for _, r := range results {
		fmt.Printf("%-30s | %-12s | %-15.2f | %-10d | %-10d\n", r.Name, r.Duration.Round(time.Millisecond), r.ReqPerSec, r.Success, r.Errors)
	}
	fmt.Println("=========================================================================================")
}

// runGRPCHealthStress tests raw gRPC throughput without DB overhead
func runGRPCHealthStress(total, concurrency int) TestResult {
	fmt.Printf("⏳ Running: [gRPC] HealthCheck (Target: %d reqs)...\n", total)

	conn, err := grpc.NewClient(GRPCAddress, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		log.Fatalf("Failed to connect gRPC: %v", err)
	}
	defer conn.Close()

	client := pb.NewEngineServiceClient(conn)
	reqPool := make(chan int, total)
	for i := 0; i < total; i++ {
		reqPool <- i
	}
	close(reqPool)

	var successCount, errorCount atomic.Uint64
	var wg sync.WaitGroup
	startTime := time.Now()

	for w := 0; w < concurrency; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range reqPool {
				ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
				_, err := client.HealthCheck(ctx, &pb.PingRequest{})
				cancel()

				if err != nil {
					errorCount.Add(1)
				} else {
					successCount.Add(1)
				}
			}
		}()
	}

	wg.Wait()
	duration := time.Since(startTime)

	return TestResult{
		Name:      "[gRPC] Health Ping",
		Duration:  duration,
		ReqPerSec: float64(total) / duration.Seconds(),
		Success:   successCount.Load(),
		Errors:    errorCount.Load(),
	}
}

// runGRPCVectorStress tests complex database reads and math over gRPC
func runGRPCVectorStress(total, concurrency int, queryVector []float32) TestResult {
	fmt.Printf("⏳ Running: [gRPC] Vector Search (Target: %d reqs)...\n", total)

	conn, err := grpc.NewClient(GRPCAddress, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		log.Fatalf("Failed to connect gRPC: %v", err)
	}
	defer conn.Close()

	client := pb.NewEngineServiceClient(conn)
	reqPool := make(chan int, total)
	for i := 0; i < total; i++ {
		reqPool <- i
	}
	close(reqPool)

	reqPayload := &pb.VectorRequest{
		TableName:   TargetTable,
		QueryVector: queryVector,
		Limit:       5,
	}

	var successCount, errorCount atomic.Uint64
	var wg sync.WaitGroup
	startTime := time.Now()

	for w := 0; w < concurrency; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range reqPool {
				ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
				_, err := client.VectorSearch(ctx, reqPayload)
				cancel()

				if err != nil {
					errorCount.Add(1)
				} else {
					successCount.Add(1)
				}
			}
		}()
	}

	wg.Wait()
	duration := time.Since(startTime)

	return TestResult{
		Name:      "[gRPC] Vector Search (Math)",
		Duration:  duration,
		ReqPerSec: float64(total) / duration.Seconds(),
		Success:   successCount.Load(),
		Errors:    errorCount.Load(),
	}
}

// runMCPHttpStress tests JSON-RPC over HTTP simulating AI Agent connections
func runMCPHttpStress(total, concurrency int, queryVector []float32) TestResult {
	fmt.Printf("⏳ Running: [HTTP/MCP] JSON-RPC Vector Search (Target: %d reqs)...\n", total)

	// Optimize HTTP client for high concurrency
	t := http.DefaultTransport.(*http.Transport).Clone()
	t.MaxIdleConns = concurrency
	t.MaxConnsPerHost = concurrency
	t.MaxIdleConnsPerHost = concurrency

	httpClient := &http.Client{
		Transport: t,
		Timeout:   5 * time.Second,
	}

	reqPool := make(chan int, total)
	for i := 0; i < total; i++ {
		reqPool <- i
	}
	close(reqPool)

	// Build the JSON-RPC payload
	payload := map[string]any{
		"jsonrpc": "2.0",
		"id":      1,
		"method":  "tools/call",
		"params": map[string]any{
			"name": "vector_search",
			"arguments": map[string]any{
				"table_name":   TargetTable,
				"query_vector": queryVector,
				"limit":        5,
			},
		},
	}
	payloadBytes, _ := json.Marshal(payload)

	var successCount, errorCount atomic.Uint64
	var wg sync.WaitGroup
	startTime := time.Now()

	for w := 0; w < concurrency; w++ {
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

				// Must read and close body to free sockets
				io.Copy(io.Discard, resp.Body)
				resp.Body.Close()

				if resp.StatusCode == http.StatusOK {
					successCount.Add(1)
				} else {
					errorCount.Add(1)
				}
			}
		}()
	}

	wg.Wait()
	duration := time.Since(startTime)

	return TestResult{
		Name:      "[HTTP/MCP] JSON-RPC Agents",
		Duration:  duration,
		ReqPerSec: float64(total) / duration.Seconds(),
		Success:   successCount.Load(),
		Errors:    errorCount.Load(),
	}
}
