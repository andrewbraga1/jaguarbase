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

	"github.com/andrewbraga1/jaguarbase/internal/config"
	"github.com/andrewbraga1/jaguarbase/internal/engine"
	"github.com/andrewbraga1/jaguarbase/internal/server"
	pb "github.com/andrewbraga1/jaguarbase/pkg/api/v1/pb"
	"google.golang.org/grpc"
)

func main() {
	start := time.Now() // ⏱️ Start cold start timer
	log.Println("Starting JaguarBase 🐆...")

	// 1. Load Configuration (Manifest)
	manifest, err := config.LoadManifest("jaguardb.example.yaml")
	if err != nil {
		log.Fatalf("Failed to load manifest: %v", err)
	}

	// 2. Initialize the Engine (DuckDB)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	dbEngine, err := engine.NewDuckDBEngine(ctx, "?access_mode=read_write")
	if err != nil {
		log.Fatalf("Failed to initialize engine: %v", err)
	}
	defer dbEngine.Close()

	// 📊 Metric 1: Disk and Memory load time
	log.Printf("✅ Engine and Manifest loaded in %v", time.Since(start))

	// Secret key for Zero-Trust Signature (In production, load from env var)
	hmacSecret := []byte("jaguarbase-super-secret-key")

	// 3. Start gRPC Server (Microservices Plane - Port 50051)
	grpcListener, err := net.Listen("tcp", ":50051")
	if err != nil {
		log.Fatalf("Failed to listen for gRPC: %v", err)
	}
	grpcSrv := grpc.NewServer()
	pb.RegisterEngineServiceServer(grpcSrv, server.NewGRPCEngineServer(dbEngine, manifest, hmacSecret))

	// Log before the goroutine to prevent race conditions in terminal output
	log.Println("🚀 gRPC Server running on :50051 (High-Performance Plane)")
	go func() {
		if err := grpcSrv.Serve(grpcListener); err != nil {
			log.Fatalf("gRPC server failed: %v", err)
		}
	}()

	// 4. Start MCP / REST Server (Agent Plane - Port 8080)
	mcpSrv := server.NewMCPServer(dbEngine, manifest, hmacSecret)
	httpMux := http.NewServeMux()
	httpMux.Handle("/mcp", mcpSrv)

	httpServer := &http.Server{
		Addr:    ":8080",
		Handler: httpMux,
	}

	// Log before the goroutine to prevent race conditions in terminal output
	log.Println("🧠 MCP Server running on :8080/mcp (AI Agent Plane)")
	go func() {
		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("HTTP server failed: %v", err)
		}
	}()

	// 📊 Metric 2: Total Cold Start
	log.Printf("⚡ All systems online and ready to accept traffic in %v", time.Since(start))

	// 5. Graceful Shutdown (Safely terminate connections on Ctrl+C)
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	<-stop

	log.Println("\nShutting down JaguarBase gracefully...")
	grpcSrv.GracefulStop()

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer shutdownCancel()
	if err := httpServer.Shutdown(shutdownCtx); err != nil {
		log.Printf("HTTP shutdown error: %v", err)
	}

	log.Println("Shutdown complete.")
}
