package main

import (
	"context"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/andrewbraga1/jaguarbase/internal/ai"
	"github.com/andrewbraga1/jaguarbase/internal/config"
	"github.com/andrewbraga1/jaguarbase/internal/engine"
	"github.com/andrewbraga1/jaguarbase/internal/ingestion"
	"github.com/andrewbraga1/jaguarbase/internal/server"
	pb "github.com/andrewbraga1/jaguarbase/pkg/api/v1/pb"
	"github.com/joho/godotenv"
	"google.golang.org/grpc"
)

func main() {
	start := time.Now() // ⏱️ Start cold start timer
	log.Println("Starting JaguarBase 🐆...")

	// 0. Load Environment Variables (12-Factor App)
	// Ignores error if .env doesn't exist (e.g., when running in Docker/K8s with native env vars)
	_ = godotenv.Load()

	// 1. Load Configuration (Manifest)
	manifest, err := config.LoadManifest("jaguardb.example.yaml")
	if err != nil {
		log.Fatalf("Failed to load manifest: %v", err)
	}

	// Context for graceful shutdown propagation to workers (like the Ingestion Pipeline)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// 2. Initialize the Engine (DuckDB)
	dbEngine, err := engine.NewDuckDBEngine(ctx, "?access_mode=read_write")
	if err != nil {
		log.Fatalf("Failed to initialize engine: %v", err)
	}
	defer dbEngine.Close()

	if err := dbEngine.AutoMigrate(ctx, manifest); err != nil {
		log.Fatalf("Failed to auto-migrate DuckDB schema: %v", err)
	}

	// 📊 Metric 1: Disk and Memory load time
	log.Printf("✅ Engine and Manifest loaded in %v", time.Since(start))

	// --- PHASE 5: Start Agnostic Ingestion Pipeline (Air Gap) ---
	pluggedDSN := os.Getenv("JAGUARBASE_DB_PLUGGED")
	tableName := manifest.Sources[0].Table

	llmHost := os.Getenv("LLM_API_URL")
	aiAdapter := ai.NewAiAdapter(llmHost)
	if pluggedDSN == "" {
		log.Println("⚠️ JAGUARBASE_DB_PLUGGED not set. Ingestion pipeline bypassed.")
	} else {
		// Factory Pattern: Dynamically resolves the correct DB adapter
		adapter, err := ingestion.NewAdapter(pluggedDSN, tableName)
		if err != nil {
			log.Printf("⚠️ Warning: Failed to plug database adapter: %v", err)
		} else {
			pipeline := ingestion.NewPipeline(adapter, dbEngine, manifest, aiAdapter)
			go pipeline.Start(ctx) // Runs asynchronously
		}
	}
	// ------------------------------------------------------------

	// Secret key for Zero-Trust Signature
	hmacEnv := os.Getenv("JAGUARBASE_HMAC_SECRET")
	if hmacEnv == "" {
		log.Fatalf("❌ FATAL: JAGUARBASE_HMAC_SECRET environment variable is strictly required for Zero-Trust governance.")
	}
	hmacSecret := []byte(hmacEnv)

	// 3. Start gRPC Server (Microservices Plane - Port 50051)
	grpcListener, err := net.Listen("tcp", ":50051")
	if err != nil {
		log.Fatalf("Failed to listen for gRPC: %v", err)
	}
	grpcSrv := grpc.NewServer()
	pb.RegisterEngineServiceServer(grpcSrv, server.NewGRPCEngineServer(dbEngine, manifest, hmacSecret))

	log.Println("🚀 gRPC Server running on :50051 (High-Performance Plane)")
	go func() {
		if err := grpcSrv.Serve(grpcListener); err != nil {
			log.Fatalf("gRPC server failed: %v", err)
		}
	}()

	// 4. Start MCP / JSON-RPC Server (Agent Plane - Port 8080)
	mcpSrv := server.NewMCPServer(dbEngine, manifest, hmacSecret)
	httpMux := http.NewServeMux()
	httpMux.Handle("/mcp", mcpSrv)

	httpServer := &http.Server{
		Addr:    ":8080",
		Handler: httpMux,
	}

	log.Println("🧠 MCP Server running on :8080/mcp (AI Agent Plane)")
	go func() {
		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("HTTP server failed: %v", err)
		}
	}()

	// 📊 Metric 2: Total Cold Start
	log.Printf("⚡ All systems online and ready to accept traffic in %v", time.Since(start))

	// 5. Graceful Shutdown (Safely terminate connections and workers on Ctrl+C)
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	<-stop

	log.Println("\nShutting down JaguarBase gracefully...")

	// Broadcast cancellation to all background workers (Ingestion Pipeline)
	cancel()

	grpcSrv.GracefulStop()

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer shutdownCancel()
	if err := httpServer.Shutdown(shutdownCtx); err != nil {
		log.Printf("HTTP shutdown error: %v", err)
	}

	log.Println("Shutdown complete.")
}
