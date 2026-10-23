# JaguarBase 🐆

[🇺🇸 Read in English](README.md) | [🇧🇷 Leia em Português](README.pt-BR.md)

**Um Motor Vetorial Agnóstico de Alta Performance e Zero-Trust em Go (acelerado via DuckDB C++)**

O JaguarBase não é apenas mais um banco de dados vetorial. É um *appliance* de infraestrutura orientado a eventos e altamente desacoplado, projetado do zero para Agentes Autônomos de IA. Ele atua como uma ponte segura e extremamente rápida entre seus bancos de dados relacionais legados (PostgreSQL) e o contexto da sua IA, garantindo que nenhuma informação sensível (PII) vaze e que nenhum *Prompt Injection* alcance o seu LLM.

## 🚀 Principais Funcionalidades

* **Ingestão Orientada a Eventos (CDC):** Escuta canais `LISTEN/NOTIFY` do PostgreSQL. Ingere dados de forma assíncrona sem onerar o banco de dados transacional primário.
* **Arquitetura Agnóstica:** Configurado via `jaguardb.yaml`. Você define o *schema* e o JaguarBase se adapta dinamicamente sem *structs* hardcoded.
* **Armazenamento Vetorial *In-Process*:** Empoderado pelo DuckDB + VSS. Sem saltos de rede para bancos externos. Matemática vetorial SIMD em C++ envelopada em Go.
* **Governança Zero-Trust:** Mascaramento nativo de PII e Provas de Autenticidade (HMAC Handoff). Prompts maliciosos ou corrompidos são marcados (`_is_tainted`) e isolados matematicamente das buscas antes de chegarem à IA.
* **Protocolo de Dupla Camada (Dual-Plane):**
  * **High-Performance Plane (gRPC):** Para microsserviços backend que exigem latência sub-10ms.
  * **AI Agent Plane (MCP):** JSON-RPC 2.0 via HTTP para integração nativa com Agentes IA (Claude Desktop, Cursor, LLMs customizados).

## 📊 Benchmarks

Testado em um Intel i7 Dual-Core com 1.000 workers concorrentes simultâneos processando vetores de 1536 dimensões. Veja o relatório completo em [BENCHMARKS.md](docs/BENCHMARKS.md).

| Cenário | Total de Reqs | Throughput (Req/Seg) | Taxa de Sucesso |
| :--- | :--- | :--- | :--- |
| **[gRPC] Health Ping** | 100.000 | **43.584,82 req/s** | 100% |
| **[gRPC] Vector Search** | 10.000 | **1.050,87 req/s** | 100% |
| **[HTTP/MCP] JSON-RPC** | 10.000 | **831,80 req/s** | 100% |

## 🛠️ Quick Start

```bash
# 1. Instale as dependências CGO (Obrigatório para os bindings C++ do DuckDB)
sudo apt-get install build-essential gcc

# 2. Inicie o Servidor JaguarBase
CGO_ENABLED=1 go run cmd/jaguarbase/main.go

# 3. Em outro terminal, rode o Teste de Estresse de Alta Concorrência
go run cmd/stress/main.go
```

## 🛣 Roadmap

- [x] **Fase 1: PoC e Domínio Core** - Validação do Adapter Postgres, PII Vault no Redis, Motor SIMD DuckDB e Handoff de Segurança.
- [x] **Fase 2: Configuração de Schema Dinâmico** - Parser do `jaguardb.yaml`, ingestão agnóstica a tipos (`map[string]any`) e remoção de structs estáticas.
- [x] **Fase 3: Busca Vetorial (RAG)** - Extensões vetoriais no DuckDB (VSS), suporte a embeddings dinâmicos (`FLOAT[N]`) e similaridade semântica (Cosseno) Fail-Closed.
- [x] **Fase 4: Borda Multi-Protocolo** - Camada de borda de alta performance (gRPC/Arrow Flight) e Servidor MCP (para Agentes de IA), junto com endpoints de telemetria.
- [x] **Fase 5: Pipeline de Ingestão** (Worker Air-Gap do PostgreSQL para DuckDB, Geração de Embeddings)