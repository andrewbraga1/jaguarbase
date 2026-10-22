# JaguarBase 🐆

[🇺🇸 Read in English](README.md) | [🇧🇷 Leia em Português](README.pt-BR.md)

**The Zero-Trust, AI-Native Database Layer for Autonomous Agents.**

JaguarBase is a stateful storage engine designed from the ground up for LLM agents. It acts as a secure, high-performance bridge between your legacy relational databases (PostgreSQL) and your AI context window, ensuring that no PII leaks and no Prompt Injections reach your LLM.

## 🏗 Architecture

JaguarBase operates on a **Zero-Trust Write-Time Governance** model. Instead of relying on the LLM to filter bad data, JaguarBase cleans the data *before* it is indexed for vector search.

1. **Source (PostgreSQL):** Ingests raw data via CDC or polling.
2. **Governance Pipeline (Redis + RE2):** 
   - **PII Vault:** Deterministically tokenizes sensitive data using Redis as the L2 secure state store.
   - **Scanner:** Runs $O(N)$ RE2 regex to detect and isolate Prompt Injections.
3. **Execution Kernel (DuckDB VSS):** Stores the sanitized, safe data and high-dimensional embeddings in an embedded, columnar SIMD-accelerated format.
4. **Multi-Protocol Retrieval:** Exposes high-performance interfaces for standard backends and a secure MCP (Model Context Protocol) interface for LLMs, protected by a **Cognitive Paginator** and **HMAC-SHA256 Zero-Trust Handoff**.

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
- [ ] **Phase 4: Multi-Protocol Edge** - High-performance Edge layer (gRPC/Arrow Flight) and MCP Server (for AI Agents) with telemetry endpoints.