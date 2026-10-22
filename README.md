# JaguarBase 🐆

[🇺🇸 Read in English](README.md) | [🇧🇷 Leia em Português](README.pt-BR.md)

**The Zero-Trust, AI-Native Database Layer for Autonomous Agents.**

JaguarBase is a stateful storage engine designed from the ground up for LLM agents. It acts as a secure, high-performance bridge between your legacy relational databases (PostgreSQL) and your AI context window, ensuring that no PII leaks and no Prompt Injections reach your LLM.

## 🏗 Architecture & Data Flow

JaguarBase operates on a **Zero-Trust Write-Time Governance** model combined with an **"Air Gap"** ingestion strategy. To guarantee sub-millisecond latency and deterministic security, JaguarBase does **not** query transactional databases (like PostgreSQL) in real-time. 

The data lifecycle follows four distinct isolated stages:

1. **The Air Gap (Ingestion):** A background worker (Phase 5) fetches new raw data from transactional sources (e.g., PostgreSQL) via CDC or polling, isolating the vector engine from heavy production loads.
2. **Write-Time Governance:** 
   - **PII Vault:** Deterministically tokenizes sensitive data using Redis as the L2 secure state store.
   - **Scanner:** Runs $O(N)$ RE2 regex to detect and isolate Prompt Injections before they reach the embeddings.
3. **Execution Kernel (Vector Storage):** The sanitized data and high-dimensional embeddings are stored in a standalone `storage.duckdb` file using DuckDB VSS (columnar SIMD-accelerated format).
4. **Multi-Protocol Retrieval:** Serves AI agents directly from DuckDB via high-performance interfaces (gRPC for microservices, JSON-RPC/MCP for LLMs), protected by a Cognitive Paginator and HMAC-SHA256 Zero-Trust Handoff.

## 🚀 Running the Integration Tests

```bash
# 1. Run Core Domain and Security tests
go test -v -race ./internal/security/... ./internal/pagination/... ./internal/dynamic/...

# 2. Run embedded DuckDB Engine & VSS tests (Requires CGO)
CGO_ENABLED=1 go test -v -race ./internal/engine/...
```

## 🛣 Roadmap

- [x] **Phase 1: PoC & Core Domain** - Postgres Adapter, Redis Vault, DuckDB SIMD, Security Handoff validation.
- [x] **Phase 2: Dynamic Schema Configuration** - `jaguardb.yaml` parser, type-agnostic ingestion (`map[string]any`), and removal of static structs.
- [x] **Phase 3: Vector Search (RAG)** - DuckDB vector extensions (VSS), dynamic embedding support (`FLOAT[N]`), and fail-closed Semantic Similarity (Cosine).
- [x] **Phase 4: Multi-Protocol Edge** - High-performance Edge layer (gRPC/Arrow Flight) and MCP Server (for AI Agents) with telemetry endpoints.
- [ ] **Phase 5: Ingestion Pipeline** (PostgreSQL to DuckDB Air-Gap worker, Embedding generation)