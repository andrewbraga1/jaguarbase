# JaguarBase 🐆

[🇺🇸 Read in English](README.md) | [🇧🇷 Leia em Português](README.pt-BR.md)

**A Zero-Trust, High-Performance Agnostic Vector Engine in Go (SIMD-accelerated via DuckDB C++)**

JaguarBase is not just another vector database. It is a highly decoupled, event-driven infrastructure appliance designed from the ground up for Autonomous AI Agents. It acts as a secure, high-performance bridge between your legacy relational databases (PostgreSQL) and your AI context window, ensuring that no PII leaks and no Prompt Injections reach your LLM.

## 🚀 Key Features

* **Event-Driven Ingestion (CDC):** Listens to PostgreSQL `LISTEN/NOTIFY` channels. Ingests data asynchronously without slowing down your primary transactional database.
* **Agnostic Architecture:** Configured via `jaguardb.yaml`. You define the schema, JaguarBase adapts dynamically. No hardcoded structs or columns.
* **In-Process Vector Storage:** Powered by DuckDB + VSS. No external database networks. Extremely fast C++ SIMD vector math inside a Go wrapper.
* **Zero-Trust Governance:** Built-in PII masking and HMAC Handoff Proofs. Corrupted or malicious prompts are flagged (`_is_tainted`) and mathematically isolated from AI searches before they hit the context window.
* **Dual-Plane Protocol:**
  * **High-Performance Plane (gRPC):** For backend microservices requiring sub-10ms latency.
  * **AI Agent Plane (MCP):** JSON-RPC 2.0 via HTTP/REST for direct integration with AI Agents (Claude Desktop, Cursor, Custom LLMs).

## 📊 Benchmarks

Tested on a Dual-Core Intel i7 with 1,000 simultaneous concurrent workers processing 1536-dimensional vectors. See the full [BENCHMARKS.md](docs/BENCHMARKS.md) report.

| Scenario | Total Requests | Throughput (Req/Sec) | Success Rate |
| :--- | :--- | :--- | :--- |
| **[gRPC] Health Ping** | 100,000 | **43,584.82 req/s** | 100% |
| **[gRPC] Vector Search** | 10,000 | **1,050.87 req/s** | 100% |
| **[HTTP/MCP] JSON-RPC** | 10,000 | **831.80 req/s** | 100% |

## 🛠️ Quick Start

```bash
# 1. Install CGO Dependencies (Required for DuckDB C++ bindings)
sudo apt-get install build-essential gcc

# 2. Run the JaguarBase Server
CGO_ENABLED=1 go run cmd/jaguarbase/main.go

# 3. In another terminal, run the High-Concurrency Stress Test
go run cmd/stress/main.go
```

## 🛣 Roadmap

- [x] **Phase 1: PoC & Core Domain** - Postgres Adapter, Redis Vault, DuckDB SIMD, Security Handoff validation.
- [x] **Phase 2: Dynamic Schema Configuration** - `jaguardb.yaml` parser, type-agnostic ingestion (`map[string]any`), and removal of static structs.
- [x] **Phase 3: Vector Search (RAG)** - DuckDB vector extensions (VSS), dynamic embedding support (`FLOAT[N]`), and fail-closed Semantic Similarity (Cosine).
- [x] **Phase 4: Multi-Protocol Edge** - High-performance Edge layer (gRPC/Arrow Flight) and MCP Server (for AI Agents) with telemetry endpoints.
- [x] **Phase 5: Ingestion Pipeline** (PostgreSQL to DuckDB Air-Gap worker, Embedding generation)
- [ ] **Phase 6: Ingestion Pipeline** (PostgreSQL to DuckDB Air-Gap worker, Embedding generation)