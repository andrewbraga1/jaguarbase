# JaguarBase Performance Benchmarks 🐆

This document details the stress testing and throughput capabilities of the **JaguarBase** Zero-Trust Vector Engine. The benchmarks validate the system's ability to handle high concurrency, mathematical vector operations (DuckDB VSS), and JSON-RPC serialization without dropping connections.

## Test Environment

* **Hardware:** Intel Core i7-7500U (Dual-Core)
* **Concurrency:** 1,000 simultaneous workers (Goroutines)
* **Vector Dimensions:** 1,536 (OpenAI standard)
* **Engine:** DuckDB in-process (CGO_ENABLED=1)
* **Client Tool:** Custom Go stress engine (`cmd/stress/main.go`)

## Benchmark Results

| Scenario | Total Requests | Duration | Throughput (Req/Sec) | Success Rate |
| :--- | :--- | :--- | :--- | :--- |
| **[gRPC] Health Ping** | 100,000 | 2.29s | 43,584.82 req/s | 100% (0 errors) |
| **[gRPC] Vector Search** | 10,000 | 9.51s | 1,050.87 req/s | 100% (0 errors) |
| **[HTTP/MCP] JSON-RPC** | 10,000 | 12.02s | 831.80 req/s | 100% (0 errors) |

## Architectural Insights

### 1. gRPC Multiplexing & Atomics (43k+ req/s)
The HealthCheck scenario bypasses the database to test raw network and application throughput. By leveraging Go's native HTTP/2 gRPC multiplexing and lock-free atomic counters for telemetry, JaguarBase easily saturates local network limits with virtually zero CPU bottleneck, achieving over **43,000 requests per second** with no dropped sockets.

### 2. DuckDB In-Process Vector Math (~1k req/s)
The `[gRPC] Vector Search` scenario forces the engine to calculate cosine similarity across 1536-dimensional vectors for every request. By embedding DuckDB directly into the Go process (`CGO_ENABLED=1`), we eliminate the network hop typically required by external databases (like PgVector or Qdrant). This allows the system to process over **1,000 complex AI context retrievals per second**.

### 3. MCP and JSON Serialization Overhead (831 req/s)
The Model Context Protocol (MCP) relies on HTTP/1.1 and JSON-RPC 2.0. As expected in computer science, parsing text-based JSON (marshaling/unmarshaling strings and arrays) consumes more CPU cycles than gRPC's binary Protobuf. However, the standard `net/http` Go router proved incredibly resilient, handling 1,000 concurrent AI Agent connections without a single `Connection Refused` or timeout, stabilizing at a highly respectable **831 req/s**.

## Conclusion
JaguarBase proves that an architecture built in **pure Go + DuckDB** can deliver enterprise-grade performance for AI Agents locally, consuming a fraction of the resources required by traditional microservices.