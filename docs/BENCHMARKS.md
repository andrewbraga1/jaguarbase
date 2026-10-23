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

## Competitive Landscape: Why JaguarBase?

How does **~1,000 req/s (gRPC)** and **~831 req/s (HTTP)** on a standard dual-core laptop compare to industry standards? We cross-reference our local tests with industry architectural baselines:

### 1. vs. Python-based Vector DBs (e.g., ChromaDB)
* **The Architecture:** Relies on Python (FastAPI/Uvicorn) wrapping C++ libraries. 
* **The Bottleneck:** Python's Global Interpreter Lock (GIL) and HTTP parsing overhead.
* **Comparison:** While the underlying C++ math is fast, the Python HTTP layer caps throughput on constrained hardware. JaguarBase utilizes Go's native HTTP multiplexer, bypassing the GIL entirely.
* **Validation Source:** [TechEmpower Web Framework Benchmarks](https://www.techempower.com/benchmarks/) historically demonstrates Go's `net/http` outperforming Python's FastAPI by multiples of 3x to 5x in JSON serialization and routing throughput.

### 2. vs. pgvector (PostgreSQL)
* **The Architecture:** A C extension running inside PostgreSQL.
* **The Bottleneck:** TCP Network hops and Postgres connection pooling (PgBouncer) overhead.
* **Comparison:** Querying `pgvector` requires network I/O and connection instantiation. JaguarBase embeds DuckDB **in-process**, eliminating the database network hop entirely and allowing direct memory access for the Go runtime.
* **Validation Source:** Supabase's [pgvector Performance Benchmark](https://supabase.com/blog/pgvector-performance) details the impact of connection scaling and network I/O on vector retrieval throughput, highlighting the overhead that in-process databases (like DuckDB) avoid.

### 3. vs. Heavyweight Clusters (Qdrant / Milvus)
* **The Architecture:** Distributed, scalable engines built for billion-scale vectors using HNSW indexing.
* **The Bottleneck:** Memory footprint (RAM).
* **Comparison:** Distributed systems require gigabytes of RAM just to idle and maintain cluster consensus. JaguarBase is built for the **Agentic Scale** (thousands to millions of context vectors), delivering comparable retrieval speeds in a single lightweight Go binary without the massive RAM overhead.
* **Validation Source:** Official sizing guides, such as the [Milvus Prerequisite Calculator](https://milvus.io/tools/sizing), which recommends 8GB+ of RAM for basic production deployments, whereas JaguarBase idles in megabytes.

> **Methodology Note:** To reproduce these comparisons manually, benchmark a raw FastAPI endpoint returning a 1536-dim JSON vector against a Go `net/http` endpoint on the same hardware. The delta represents the architectural overhead that JaguarBase reclaims.

## Conclusion
JaguarBase proves that an architecture built in **Go (SIMD-accelerated via DuckDB)** can deliver enterprise-grade performance for AI Agents locally, consuming a fraction of the resources required by traditional microservices.