# JaguarBase 🐆

[🇺🇸 Read in English](README.md) | [🇧🇷 Leia em Português](README.pt-BR.md)

![Go Version](https://img.shields.io/badge/Go-1.26-00ADD8?style=flat&logo=go)
![DuckDB](https://img.shields.io/badge/DuckDB-Embedded-FFF000?style=flat)
![Status](https://img.shields.io/badge/Status-Production_Ready-success)
![AppSec](https://img.shields.io/badge/AppSec-Zero_CVEs-brightgreen)
![CI](https://github.com/andrewbraga1/jaguarbase/actions/workflows/ci.yml/badge.svg)

**A Zero-Trust, High-Performance Agnostic Vector Engine in Go (SIMD-accelerated via DuckDB C++)**

JaguarBase is a highly decoupled, event-driven infrastructure appliance designed from the ground up for Autonomous AI Agents and Edge AI. It acts as a secure, high-performance bridge between your transactional databases (e.g., PostgreSQL, MySQL, Kafka streams) and your AI context window, ensuring that no PII leaks and no Prompt Injections reach your LLM.

---

## 🧠 The Architectural Thesis: CQRS & Edge Caching

Traditional architectures force AI Agents to perform computationally heavy vector math directly on the primary relational database (e.g., PostgreSQL with `pgvector`), spiking CPU usage and risking the stability of core business systems.

**JaguarBase flips the paradigm using CQRS (Command Query Responsibility Segregation):**
1. **System of Record:** Keep your data safely in your primary transactional database.
2. **Event-Driven Sync:** JaguarBase ingests data via event-driven adapters (e.g., PostgreSQL `LISTEN/NOTIFY`, CDC webhooks, or Kafka streams) and asynchronously pulls/vectorizes records into its local DuckDB engine without slowing down your transactional DB.
3. **Inference at the Edge:** AI Agents query the JaguarBase local cache in milliseconds.
4. **Result:** Zero network latency for the AI, and absolute zero CPU impact on your primary databases.

```mermaid
graph TD
    Source[(System of Record\nPostgreSQL, MySQL, Kafka...)] -- 1. Event-Driven Sync --> JB{JaguarBase\nEdge Cache}
    JB -- 2. gRPC Plane\n(Sub-10ms) --> Backend[Backend Microservices]
    JB -- 3. MCP Plane\n(JSON-RPC 2.0) --> AI[AI Agents\nClaude, Cursor, Qwen]
    style JB fill:#f9a03f,stroke:#333,stroke-width:2px,color:#000
    style Source fill:#336791,stroke:#fff,color:#fff
```

## 🚀 Key Features (The Edge AI Appliance)

- **100% Manifest-Driven Architecture:** Configured strictly via `jaguardb.yaml`. You define the schema, JaguarBase adapts dynamically. No hardcoded structs. It uses `virtual: true` definitions to seamlessly decouple raw transactional columns from the enriched RAG vector schema.
- **In-Process Vector Storage:** Powered by DuckDB + VSS. No external database networks. Extremely fast C++ SIMD vector math inside a Go wrapper.
- **Zero-Trust Governance:** Built-in PII masking and HMAC Handoff Proofs. Corrupted or malicious prompts are flagged (`_is_tainted`) and mathematically isolated from AI searches before they hit the context window.
- **Dual-Plane Protocol:**
  - **High-Performance Plane (gRPC):** For backend microservices requiring sub-10ms latency.
  - **AI Agent Plane (MCP):** JSON-RPC 2.0 via HTTP/REST for direct integration with AI Agents (Claude Desktop, Cursor, Custom LLMs).

### The Zero-Trust Pipeline Under the Hood
Every record that enters or leaves JaguarBase passes through a rigorous, fail-closed security pipeline.

```mermaid
sequenceDiagram
    participant Source as Source DB / Stream (e.g. Postgres, Kafka)
    participant JB as JaguarBase Ingestion
    participant Gov as Zero-Trust Governance
    participant DDB as DuckDB (SIMD)
    participant Agent as AI Agent (MCP)

    Source->>JB: 1. Event Emitted (CDC / Insert)
    JB->>Gov: 2. Process Row
    Gov-->>Gov: 3. Tokenize PII (Redis) & Flag Injections
    Gov->>DDB: 4. Store Vector (Dynamic Schema)
    Agent->>JB: 5. Search Query via MCP (tools/call)
    JB->>DDB: 6. Semantic Search (WHERE _is_tainted=false)
    DDB-->>JB: 7. Return Safe Results
    JB-->>Agent: 8. HMAC Handoff Proof + Clean JSON
```

---

## 🛡️ Enterprise-Grade Security & Hardening

JaguarBase has undergone rigorous AppSec auditing to ensure it meets Enterprise cloud standards:

- **Distroless & Non-Root Execution:** The Docker image runs on an ultra-minimal `gcr.io/distroless/base-nossl-debian12:nonroot` base (with explicitly copied C++ runtimes for DuckDB), completely eliminating unused OS libraries, shells, and unnecessary SSL packages.
- **Zero Vulnerabilities (Trivy & GoSec):** Continuously scanned. 0 HIGH/CRITICAL vulnerabilities in both the OS layers and Go dependencies.
- **Cryptographic PII Tokenization:** Data anonymization uses irreversible **HMAC-SHA256** tied dynamically to schema column names (e.g., `vault_cpf_...`, `vault_api_key_...`), immune to Rainbow Table and collision attacks.
- **Built-In API Shield:** MCP endpoints support optional Bearer Token authentication (`JAGUARBASE_API_KEY`), strict `MaxBytesReader` (128 KB) payload limits, bounded DuckDB CGO connection pools, and HTTP timeouts to prevent memory exhaustion and DoS.

## 🛠️ Quick Start

You can run JaguarBase entirely via Docker, injecting your YAML configuration at runtime:

```bash
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

JaguarBase has a 100% test pass rate with the Go Global Race Detector enabled. To run the full integration and unit test suite (including PostgreSQL adapter, Redis Vault, and MCP routing), use the following command:

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
- **Zero Memory Leaks:** Consumes **< 10MB of Go Heap RAM** while managing hundreds of thousands of vectors.
- **OS Page Caching:** Intelligently offloads vector indexes to the Linux Kernel Buffer Cache, preventing application-level OOM crashes.

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
