package main

import (
	"context"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/andrewbraga1/jaguarbase/internal/ai"
	"github.com/andrewbraga1/jaguarbase/internal/config"
	"github.com/andrewbraga1/jaguarbase/internal/dynamic"
	"github.com/andrewbraga1/jaguarbase/internal/engine"
	"github.com/andrewbraga1/jaguarbase/internal/governance"
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
	_ = godotenv.Load()

	// 1. Load Configuration (Manifest)
	configPath := os.Getenv("JAGUARBASE_CONFIG_PATH")
	if configPath == "" {
		configPath = "jaguardb.yaml"
	}
	manifest, err := config.LoadManifest(configPath)
	if err != nil {
		cleanConfigPath := strings.ReplaceAll(strings.ReplaceAll(configPath, "\n", ""), "\r", "")
		log.Fatalf("Failed to load manifest from %s: %v", cleanConfigPath, err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// 2. Initialize the Engine (DuckDB)
	dbEngine, err := engine.NewDuckDBEngine(ctx, os.Getenv("DUCKDB_PATH"))

	if err != nil {
		log.Fatalf("Failed to initialize engine: %v", err)
	}
	defer dbEngine.Close()

	if err := dbEngine.AutoMigrate(ctx, manifest); err != nil {
		log.Fatalf("Failed to auto-migrate DuckDB schema: %v", err)
	}

	log.Printf("✅ Engine and Manifest loaded in %v", time.Since(start))

	// --- PHASE 3: Initialize Zero-Trust Governance (Redis Vault & Taint Scanner) ---
	redisURL := os.Getenv("REDIS_URL")
	if redisURL == "" {
		redisURL = "redis://localhost:6379" // Container host
	}

	vault, err := governance.NewVault(ctx, redisURL)
	if err != nil {
		log.Fatalf("❌ FATAL: Failed to connect to Redis Vault: %v", err)
	}
	defer vault.Close()

	scanner := governance.NewScanner()
	processor := dynamic.NewProcessor(vault, scanner)
	// -------------------------------------------------------------------------------

	// --- PHASE 4: Start Agnostic Ingestion Pipeline (Air Gap) ---
	pluggedDSN := os.Getenv("JAGUARBASE_DB_PLUGGED")
	tableName := manifest.Sources[0].Table

	llmHost := os.Getenv("LLM_API_URL")
	aiAdapter := ai.NewAiAdapter(llmHost)
	if pluggedDSN == "" {
		log.Println("⚠️ JAGUARBASE_DB_PLUGGED not set. Ingestion pipeline bypassed.")
	} else {
		adapter, err := ingestion.NewAdapter(pluggedDSN, tableName)
		if err != nil {
			log.Printf("⚠️ Warning: Failed to plug database adapter: %v", err)
		} else {
			pipeline := ingestion.NewPipeline(adapter, dbEngine, manifest, aiAdapter, processor)
			go pipeline.Start(ctx)
		}
	}
	// ------------------------------------------------------------

	// Secret key for Zero-Trust Signature
	hmacEnv := os.Getenv("JAGUARBASE_HMAC_SECRET")
	if hmacEnv == "" {
		log.Fatalf("❌ FATAL: JAGUARBASE_HMAC_SECRET environment variable is strictly required for Zero-Trust governance.")
	}
	hmacSecret := []byte(hmacEnv)

	// 5. Network configuration: Default to 127.0.0.1 for secure local-only binding
	host := os.Getenv("JAGUARBASE_HOST")
	if host == "" {
		host = "127.0.0.1"
	}
	grpcPort := os.Getenv("JAGUARBASE_GRPC_PORT")
	if grpcPort == "" {
		grpcPort = "50051"
	}
	httpPort := os.Getenv("JAGUARBASE_HTTP_PORT")
	if httpPort == "" {
		httpPort = "8080"
	}

	grpcAddr := net.JoinHostPort(host, grpcPort)
	httpAddr := net.JoinHostPort(host, httpPort)

	apiKey := []byte(os.Getenv("JAGUARBASE_API_KEY"))
	if len(apiKey) == 0 {
		if host == "0.0.0.0" {
			log.Printf("⚠️ [SECURITY WARNING] Server bound to 0.0.0.0 without JAGUARBASE_API_KEY! Set JAGUARBASE_API_KEY to prevent unauthorized network access.")
		} else {
			log.Printf("⚠️ [Security Notice] Running in unauthenticated development mode bound to localhost (%s).", host)
		}
	} else {
		log.Printf("🔐 Enforcing API Key authentication (bound to %s)", host)
	}

	// #nosec G102 -- Configurable host binding with 127.0.0.1 secure default
	grpcListener, err := net.Listen("tcp", grpcAddr)
	if err != nil {
		log.Fatalf("Failed to listen for gRPC on %s: %v", grpcAddr, err)
	}

	var grpcOpts []grpc.ServerOption
	if len(apiKey) > 0 {
		grpcOpts = append(grpcOpts, grpc.UnaryInterceptor(server.UnaryAuthInterceptor(apiKey)))
	}

	grpcSrv := grpc.NewServer(grpcOpts...)
	pb.RegisterEngineServiceServer(grpcSrv, server.NewGRPCEngineServer(dbEngine, manifest, hmacSecret))

	log.Printf("🚀 gRPC Server running on %s (High-Performance Plane)", grpcAddr)
	go func() {
		if err := grpcSrv.Serve(grpcListener); err != nil {
			log.Fatalf("gRPC server failed: %v", err)
		}
	}()

	// 6. Start MCP / JSON-RPC Server
	mcpSrv := server.NewMCPServer(dbEngine, manifest, hmacSecret)
	httpMux := http.NewServeMux()
	httpMux.Handle("/mcp", mcpSrv)

	httpServer := &http.Server{
		Addr:              httpAddr,
		Handler:           httpMux,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	log.Printf("🧠 MCP Server running on http://%s/mcp (AI Agent Plane)", httpAddr)
	go func() {
		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("HTTP server failed: %v", err)
		}
	}()

	log.Printf("⚡ All systems online and ready to accept traffic in %v", time.Since(start))

	// Graceful Shutdown
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	<-stop

	log.Println("\nShutting down JaguarBase gracefully...")
	cancel()
	grpcSrv.GracefulStop()

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer shutdownCancel()
	if err := httpServer.Shutdown(shutdownCtx); err != nil {
		log.Printf("HTTP shutdown error: %v", err)
	}

	log.Println("Shutdown complete.")
}
