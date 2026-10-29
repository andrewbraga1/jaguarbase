# JaguarBase 🐆

[🇺🇸 Read in English](README.md) | [🇧🇷 Leia em Português](README.pt-BR.md)

![Go Version](https://img.shields.io/badge/Go-1.23-00ADD8?style=flat&logo=go)
![DuckDB](https://img.shields.io/badge/DuckDB-Embedded-FFF000?style=flat)
![Status](https://img.shields.io/badge/Status-Production_Ready-success)
![CI](https://github.com/andrewbraga1/jaguarbase/actions/workflows/ci.yml/badge.svg)

**A Zero-Trust, High-Performance Agnostic Vector Engine in Go (SIMD-accelerated via DuckDB C++)**

JaguarBase is a highly decoupled, event-driven infrastructure appliance designed from the ground up for Autonomous AI Agents and Edge AI. It acts as a secure, high-performance bridge between your legacy relational databases (PostgreSQL) and your AI context window, ensuring that no PII leaks and no Prompt Injections reach your LLM.

## 🧠 The Architectural Thesis: CQRS & Edge Caching

Traditional architectures force AI Agents to perform computationally heavy vector math directly on the primary relational database (e.g., PostgreSQL with `pgvector`), spiking CPU usage and risking the stability of core business systems.

**JaguarBase flips the paradigm using CQRS (Command Query Responsibility Segregation):**
1. **System of Record:** Keep your data safely in your legacy PostgreSQL.
2. **Event-Driven Sync:** JaguarBase listens to PostgreSQL `LISTEN/NOTIFY` channels and asynchronously pulls/vectorizes the data into its local DuckDB engine without slowing down your transactional DB.
3. **Inference at the Edge:** AI Agents query the JaguarBase local cache in milliseconds.
4. **Result:** Zero network latency for the AI, and absolute zero CPU impact on your primary PostgreSQL.

## 🚀 Key Features

- **100% Manifest-Driven Architecture:** Configured strictly via `jaguardb.yaml`. You define the schema, JaguarBase adapts dynamically. No hardcoded structs. It uses `virtual: true` definitions to seamlessly decouple raw transactional columns from the enriched RAG vector schema.
- **In-Process Vector Storage:** Powered by DuckDB + VSS. No external database networks. Extremely fast C++ SIMD vector math inside a Go wrapper.
- **Zero-Trust Governance:** Built-in PII masking and HMAC Handoff Proofs. Corrupted or malicious prompts are flagged (`_is_tainted`) and mathematically isolated from AI searches before they hit the context window.
- **Dual-Plane Protocol:**
  - **High-Performance Plane (gRPC):** For backend microservices requiring sub-10ms latency.
  - **AI Agent Plane (MCP):** JSON-RPC 2.0 via HTTP/REST for direct integration with AI Agents (Claude Desktop, Cursor, Custom LLMs).

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

- **Need to index 1 Billion vectors in the cloud for global search?** Use a distributed engine like Qdrant or Milvus paired with JaguarGate.
- **Need to build a robust Local AI App, Desktop Tool, or protect your Enterprise Postgres from Agentic CPU spikes?** Use **JaguarBase**.

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

## 🛣️ Roadmap

- [x] **Phase 1: PoC & Core Domain** - Postgres Adapter, Redis Vault, DuckDB SIMD, Security Handoff validation.
- [x] **Phase 2: Dynamic Schema Configuration** - `jaguardb.yaml` parser, type-agnostic ingestion, and struct removal.
- [x] **Phase 3: Vector Search (RAG)** - DuckDB VSS extensions, dynamic embeddings, and fail-closed Semantic Similarity.
- [x] **Phase 4: Multi-Protocol Edge** - High-performance Edge layer (gRPC) and MCP Server (for AI Agents).
- [x] **Phase 5: Chaos Engineering & Resilience** - O(1) Load Shedding, Context Cancellation, and hardware-level limits (Disk/RAM).
- [x] **Phase 6: Containerization & CI/CD** - Multi-stage Dockerfile (Debian Slim + CGO) and deployment automation.