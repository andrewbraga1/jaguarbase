# JaguarBase 🐆

[🇺🇸 Read in English](README.md) | [🇧🇷 Leia em Português](README.pt-BR.md)

![Go Version](https://img.shields.io/badge/Go-1.26-00ADD8?style=flat&logo=go)
![DuckDB](https://img.shields.io/badge/DuckDB-Embedded-FFF000?style=flat)
![Status](https://img.shields.io/badge/Status-Beta_/_v1.0.0-orange)
![AppSec](https://img.shields.io/badge/AppSec-Zero_CVEs-brightgreen)
![License](https://img.shields.io/badge/License-Apache_2.0-blue.svg)
![CI](https://github.com/andrewbraga1/jaguarbase/actions/workflows/ci.yml/badge.svg)

**An Open-Source High-Performance Edge Vector Cache for PostgreSQL in Go & DuckDB (SIMD-Accelerated)**

JaguarBase is an event-driven edge vector cache designed from the ground up for Autonomous AI Agents and Edge AI. It acts as a decoupled bridge between your primary transactional database (PostgreSQL) and your AI context window, offloading compute-intensive vector similarity search to an embedded DuckDB engine while applying stateful PII pseudonymization and heuristic prompt-injection filtering.

---

## 🧠 The Architectural Thesis: CQRS & Edge Caching

Traditional architectures force AI Agents to perform computationally heavy vector math directly on the primary relational database (e.g., PostgreSQL with `pgvector`), spiking CPU usage and risking the stability of core business systems.

**JaguarBase flips the paradigm using CQRS (Command Query Responsibility Segregation):**
1. **System of Record:** Keep your operational data safe in your primary transactional database.
2. **Event-Driven Sync:** JaguarBase receives change events (via PostgreSQL `LISTEN/NOTIFY`) and asynchronously embeds records into an embedded DuckDB instance.
3. **Inference at the Edge:** AI Agents query the JaguarBase local cache in milliseconds.
4. **Result:** Sub-millisecond local query plane and dramatic reduction of CPU overhead on your primary database.

```mermaid
graph TD
    Source[(PostgreSQL\nSystem of Record)] -- 1. Event-Driven Sync --> JB{JaguarBase\nEdge Cache}
    JB -- 2. gRPC Plane\n(Sub-10ms) --> Backend[Backend Microservices]
    JB -- 3. MCP Plane\n(JSON-RPC 2.0) --> AI[AI Agents\nClaude, Cursor, Local LLMs]
    style JB fill:#f9a03f,stroke:#333,stroke-width:2px,color:#000
    style Source fill:#336791,stroke:#fff,color:#fff
```

## 🚀 Key Features (The Edge AI Appliance)

- **100% Manifest-Driven Architecture:** Configured strictly via `jaguardb.yaml`. You define the schema, JaguarBase adapts dynamically without hardcoded Go structs, decoupling transactional columns from the vector representation.
- **In-Process Vector Storage:** Powered by DuckDB C++ with SIMD-accelerated exact cosine distance (`array_cosine_distance`) inside a Go wrapper.
- **Zero-Trust Governance Pipeline:**
  - **Stateful PII Pseudonymization:** Sensitive fields are tokenized via HMAC-SHA256 (`vault_{column}_{hash}`) and cached in Redis with a 24-hour TTL, ensuring plain text is never persisted at rest in DuckDB.
  - **Heuristic Anti-Injection:** Incoming payloads matching known evasion patterns are flagged (`_is_tainted=true`) and excluded from semantic search (`WHERE _is_tainted=false`).
  - **HMAC Handoff Proofs:** Responses are signed with HMAC-SHA256 to ensure data tampering is detected downstream.
- **Dual-Plane Protocol:**
  - **High-Performance Plane (gRPC):** Internal low-latency plane for backend services.
  - **AI Agent Plane (MCP / JSON-RPC 2.0):** HTTP endpoint for semantic tool calling (`tools/list`, `tools/call`).

### The Zero-Trust Pipeline Under the Hood
Every record that enters or leaves JaguarBase passes through a defensive, fail-closed security pipeline:

```mermaid
sequenceDiagram
    participant Source as PostgreSQL (System of Record)
    participant JB as JaguarBase Ingestion
    participant Gov as Zero-Trust Governance
    participant DDB as DuckDB (SIMD)
    participant Agent as AI Agent (MCP / gRPC)

    Source->>JB: 1. Event Emitted (LISTEN/NOTIFY)
    JB->>Gov: 2. Process Row
    Gov-->>Gov: 3. Tokenize PII (Redis) & Scan Injections
    Gov->>DDB: 4. Store Vector (Dynamic Schema)
    Agent->>JB: 5. Search Query (tools/call)
    JB->>DDB: 6. Semantic Search (WHERE _is_tainted=false)
    DDB-->>JB: 7. Return Safe Records
    JB-->>Agent: 8. HMAC Handoff Proof + Clean JSON
```

---

## 🛡️ Enterprise-Grade Security & Hardening

JaguarBase has undergone rigorous AppSec auditing to ensure a clean build and minimal attack surface:

- **Distroless & Non-Root Execution:** The Docker image runs on `gcr.io/distroless/base-nossl-debian12:nonroot` with explicitly copied C++ runtime libraries (`libstdc++.so.6`, `libgcc_s.so.1`), eliminating shells, package managers, and unused OpenSSL packages.
- **Zero Vulnerabilities (Trivy & GoSec):** 0 HIGH/CRITICAL CVEs in OS layers and Go dependencies; Clean Build (0 issues) in GoSec SAST.
- **API Shield & Bounds Checking:** Protected with constant-time token comparison (`subtle.ConstantTimeCompare`), payload caps (`http.MaxBytesReader` 128 KB), bounded DuckDB connection pools, and strict HTTP server timeouts against Slowloris attacks.

---

## ⚠️ Current Stage & Known Limitations (Beta v1.0.0)

To maintain absolute engineering transparency, the current release has the following design boundaries:
* **Event Ingestion:** Relies on PostgreSQL `LISTEN/NOTIFY` (with watcher readiness synchronization to prevent race conditions during startup) and durable batch backfilling. Additional CDC connectors via Postgres Logical Replication / Outbox table are on the roadmap.
* **Cache Updates:** Full idempotent upserts (`INSERT OR REPLACE`) and tombstone propagation for record deletions are implemented and validated. Backfill checkpoints are isolated in an internal metadata table to prevent resume skips.
* **Vector Indexing:** Uses DuckDB's in-process SIMD-accelerated cosine distance (`array_cosine_similarity`). Approximate Nearest Neighbor (ANN/HNSW) indexing via VSS is supported for offline and embedded setups.
* **MCP Protocol:** Compliant with Model Context Protocol specification (protocol versions 2024-11-05 and 2025-06-18), featuring full `initialize` handshake, capabilities negotiation, ping, `tools/list`, and typed Content Blocks in `tools/call` for direct integration with Claude Desktop and MCP-compatible AI clients.

## 🛠️ Quick Start

You can run JaguarBase entirely via Docker, injecting your YAML configuration at runtime. Ensure host permissions for the mounted data directory match the container's non-root user (`uid:gid 65532:65532`), or specify `--user`:

```bash
# Prepare data directory permissions for nonroot container
mkdir -p data && chmod 777 data

# 1. Start JaguarBase in Docker
docker run -d \
  --name jaguarbase-edge \
  --network="host" \
  --env-file .env \
  -v "$(pwd)/jaguardb.yaml:/app/jaguardb.yaml" \
  -v "$(pwd)/data:/app/data" \
  andrewbraga1/jaguarbase:latest

# 2. View live ingestion logs
docker logs -f jaguarbase-edge
```

*For local development (Requires CGO):*
```bash
sudo apt-get install build-essential gcc
CGO_ENABLED=1 go run cmd/jaguarbase/main.go
```

## 🧪 Comprehensive Testing Suite

JaguarBase has a 100% unit and integration test pass rate with the Go Global Race Detector enabled. 

To run internal unit tests with race detection:
```bash
go test -v -race ./internal/... -count=1
```

To run the full suite against live services (requires PostgreSQL, Redis, Ollama, and an active JaguarBase instance running on `:50051` for end-to-end tests):
```bash
JAGUARBASE_HMAC_SECRET="jaguarbase-super-secret-key-mude-em-prod" \
LLM_API_URL="http://localhost:11434/api/embeddings" \
TEST_DATABASE_URL="postgres://Your_User:Your_PassWord@localhost:5432/Your_DataBase?sslmode=disable" \
TEST_REDIS_URL="redis://localhost:6379" \
go test -v -race ./... -count=1
```
*Note: The `?sslmode=disable` flag in `TEST_DATABASE_URL` is optional but highly recommended when testing against local PostgreSQL containers.*

## 📊 Benchmarks & Chaos Engineering

Tested on a Dual-Core Intel i7 with 1,000 simultaneous concurrent workers processing 1536-dimensional vectors. See the full [BENCHMARKS.md](docs/BENCHMARKS.md) report.

| Scenario | Total Requests | Throughput (Req/Sec) | Success Rate |
| :--- | :--- | :--- | :--- |
| **[gRPC] Health Ping** | 100,000 | **43,584.82 req/s** | 100% |
| **[gRPC] Vector Search** | 10,000 | **1,050.87 req/s** | 100% |
| **[HTTP/MCP] JSON-RPC** | 10,000 | **831.80 req/s** | 100% |

**Hardware Resilience (Chaos Engineering):**
- **O(1) Load Shedding:** Intercepts Runaway Loops (e.g., LangChain infinite loops) and blocks requests (HTTP 429) at the Go Router layer in **< 0.5ms**.
- **Predictable Memory Footprint:** The Go service layer maintains a lean heap footprint (**< 10MB of Go Heap RAM**), while embedded DuckDB off-heap C++ allocations are managed by DuckDB buffer pooling and offloaded to Linux Kernel page caching, preventing application-level OOM crashes.
- **OS Page Caching:** Intelligently offloads vector indexes to the Linux Kernel Buffer Cache, ensuring high-throughput local vector queries without memory bloat.

## 💼 Market Positioning

- **Need to index 1 Billion vectors in the cloud for global search?** Use a distributed engine like Qdrant or Milvus paired with [JaguarGate](https://github.com/andrewbraga1/jaguargate).
- **Need to build a robust Local AI App, Desktop Tool, or protect your Enterprise Postgres from Agentic CPU spikes?** Use **JaguarBase**.

## 🛣️ Roadmap

- [x] **Phase 1: PoC & Core Domain** Postgres Adapter, Redis Vault, DuckDB SIMD, Security Handoff validation.
- [x] **Phase 2: Dynamic Schema Configuration** `jaguardb.yaml` parser, type-agnostic ingestion, and struct removal.
- [x] **Phase 3: Vector Search (RAG)** DuckDB VSS extensions, dynamic embeddings, and fail-closed Semantic Similarity.
- [x] **Phase 4: Multi-Protocol Edge** High-performance Edge layer (gRPC) and MCP Server (JSON-RPC 2.0).
- [x] **Phase 5: Chaos Engineering & Resilience** O(1) Load Shedding, Context Cancellation, and hardware-level limits.
- [x] **Phase 6: Containerization & CI/CD** Multi-stage Dockerfile (Distroless Non-Root + CGO) and deployment automation.
- [x] **Phase 7: Enterprise Security (AppSec)** Zero-Trust Cryptography, API Shields, and Zero-CVE audits.
