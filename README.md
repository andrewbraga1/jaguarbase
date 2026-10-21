# JaguarBase 🐆

[🇺🇸 Read in English](README.md) | [🇧🇷 Leia em Português](README.pt-BR.md)

**The Zero-Trust, AI-Native Database Layer for Autonomous Agents.**

JaguarBase is a stateful storage engine designed from the ground up for LLM agents. It acts as a secure, high-performance bridge between your legacy relational databases (PostgreSQL) and your AI context window, ensuring that no PII leaks and no Prompt Injections reach your LLM.

## 🏗 Architecture

JaguarBase operates on a **Zero-Trust Write-Time Governance** model. Instead of relying on the LLM to filter bad data, JaguarBase cleans the data *before* it is indexed for vector search.

1. **Source (PostgreSQL):** Ingests raw data via CDC or polling.
2. **Governance Pipeline (Redis + RE2):** 
   - **PII Vault:** Deterministically tokenizes sensitive data (e.g., `111.222.333-44` becomes `vault_cpf_37d6018b`) using Redis as the L2 secure state store.
   - **Scanner:** Runs $O(N)$ RE2 regex to detect and isolate Prompt Injections.
3. **Execution Kernel (DuckDB):** Stores the sanitized, safe data in an embedded, columnar SIMD-accelerated format.
4. **Agent Retrieval (MCP):** Exposes a secure JSON-RPC interface to the LLM (e.g., LiteLLM/Qwen) protected by a **Cognitive Paginator** and validated by an **HMAC-SHA256 Zero-Trust Handoff** signature.

## ✨ Core Features (PoC Validated)

- **Write-Time Anti-Injection:** Isolates malicious instructions during ingestion, saving LLM CPU cycles during read-time.
- **Reversible PII Vault:** Agents only "see" cryptographic tokens, completely removing PII from the LLM context window.
- **Cognitive Paginator:** Automatically truncates massive result sets to prevent LLM Out-Of-Memory (OOM) hallucinations, returning a smart statistical summary instead.
- **Zero-Trust Handoff:** Every response is signed with an HMAC-SHA256 FrameProof, proving to the network gateway (JaguarGate) that the data is clean.

## 🚀 Running the Integration Tests

The project uses Go's native testing library with strict data race detection. Some tests require real connections to PostgreSQL and Redis.

```bash
# 1. Run Core Domain and Security tests (In-Memory / No external DB required)
go test -v -race ./internal/security/... ./internal/pagination/...

# 2. Run embedded DuckDB Engine tests (Requires CGO)
CGO_ENABLED=1 go test -v -race ./internal/engine/...

# 3. Run Postgres Adapter Integration tests
TEST_DATABASE_URL="postgres://user:pass@localhost:5432/db?sslmode=disable" \
go test -v -race ./internal/adapter/...

# 4. Run Governance Pipeline (Postgres + Redis Integration)
TEST_DATABASE_URL="postgres://user:pass@localhost:5432/db?sslmode=disable" \
TEST_REDIS_URL="redis://localhost:6379" \
go test -v -race ./internal/governance/...
```

## 🛣 Roadmap

- [x] **Phase 1: PoC & Core Domain** - Postgres Adapter, Redis Vault, DuckDB SIMD, Security Handoff validation.
- [ ] **Phase 2: Dynamic Schema Configuration** - `jaguardb.yaml` parser, type-agnostic ingestion (`map[string]any`), and removal of static structs.
- [ ] **Phase 3: Vector Search (RAG)** - DuckDB vector extensions, embedding integration, and semantic similarity execution.
- [ ] **Phase 4: MCP Server & Arrow Flight** - Full Model Context Protocol implementation for LiteLLM/Qwen integration and Zero-Copy network transport.