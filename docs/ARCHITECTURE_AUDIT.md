# JaguarBase: Relatório Técnico de Auditoria Arquitetural & Segurança

**Status da Versão:** Beta (v1.0.0)  
**Escopo:** Revisão Estática de Código, Análise de Fluxo de Dados, Protocolos e Governança  
**Objetivo:** Mapeamento de dívida técnica, vulnerabilidades lógicas e plano de ação para a versão v1.1.0  

---

## 📌 Sumário Executivo

O JaguarBase apresenta fundamentos sólidos de engenharia de sistemas em Go: segregação em camadas (*clean architecture*), isolamento estrito de contêineres (*distroless non-root*), zero alertas de SAST no GoSec, integração nativa CGO com DuckDB e mecanismos eficientes de proteção na borda (*Load Shedding* O(1) e timeouts HTTP).

Contudo, a auditoria funcional identificou **desconexões semânticas críticas entre os módulos**, particularmente no desacoplamento entre manifesto, pipeline de ingestão e ciclo de vida de dados no cache. Este documento consolida todos os achados priorizados, fornecendo a base técnica para a evolução imediata da plataforma.

---

## 🔴 Achados Prioritários (P1: Integridade e Corretude Funcional)

### 1. Desvio no Mapeamento de Coluna de Conteúdo (`content_column`)
* **Localização:** `internal/ingestion/postgres_adapter.go`, `internal/ingestion/pipeline.go`, `internal/dynamic/processor.go`
* **Descrição do Problema:** No adaptador PostgreSQL em tempo real, colunas nomeadas `message` ou `content` são extraídas para `RawRecord.Content` e removidas do mapa de metadados genéricos. No `pipeline.go`, o mapa `rawMap` recebe `content`, mas não repõe a chave configurada no manifesto (ex.: `message`). Ao executar `ProcessRow`, a coluna declarada no YAML com governança `untrusted_input` é avaliada como ausente (`nil`).
* **Impacto:** O scanner anti-injeção não é disparado na coluna customizada de mensagem em tempo real, permitindo que payloads maliciosos sejam inseridos com `_is_tainted = false`.
* **Remediação Recomendada:**
  - Definir uma resolução canônica única de schema na ingestão.
  - O `pipeline.go` deve mapear o `record.Content` diretamente sob o nome de coluna declarado em `content_column` dentro de `rawMap`.
  - Garantir que a mesma string inspecionada e sanitizada pelo `processor` seja aquela submetida ao vetorizador e à persistência.

---

### 2. Janela de Perda de Eventos entre Backfill e Streaming Real-Time
* **Localização:** `internal/ingestion/pipeline.go` (`Start` e `performBackfill`)
* **Descrição do Problema:** O método `Start` executa o `performBackfill` de forma totalmente síncrona antes de iniciar a goroutine do `source.Watch` (que executa o `LISTEN`).
* **Impacto:** Quaisquer eventos de `INSERT`/`UPDATE` que ocorram no PostgreSQL durante a execução do backfill geram notificações `NOTIFY` que são descartadas, pois o cliente ainda não abriu a conexão de escuta.
* **Remediação Recomendada:**
  - Iniciar a escuta (`LISTEN`) ou enfileiramento concorrente de notificações antes ou paralelamente ao início da leitura de snapshot.
  - Implementar estratégia de reconciliação ou adotar padrão de replicação lógica (CDC durável/tabela de outbox com commit transacional).

---

### 3. Falhas Parciais no Backfill Avançando o Ponteiro de Checkpoint
* **Localização:** `internal/ingestion/pipeline.go` (`performBackfill`)
* **Descrição do Problema:** `processRecord` não propaga erros para o laço de paginação do backfill. Se uma etapa intermediária falhar (ex.: timeout na geração de embeddings via Ollama ou falha de escrita), o loop continua e o `lastID` avança normalmente.
* **Impacto:** Registros com falha são ignorados permanentemente. Se a aplicação reiniciar, a busca por `MAX(id)` começará a partir do maior ID processado com sucesso, deixando lacunas não sincronizadas no DuckDB.
* **Remediação Recomendada:**
  - Fazer `processRecord` retornar `error`.
  - Implementar política de retentativa finita (*exponential backoff*).
  - Em caso de esgotamento de retentativas, persistir a falha em fila de quarentena durável ou interromper o avanço do checkpoint (`lastID`).

---

### 4. Ausência de Upsert Idempotente e Propagação de Exclusões (Deletes)
* **Localização:** `internal/engine/duckdb.go` (`AutoMigrate`, `InsertBatch`), `internal/ingestion/postgres_adapter.go`
* **Descrição do Problema:** A tabela vetorial criada no DuckDB não possui chave primária explícita (`PRIMARY KEY (id)`). O método `InsertBatch` realiza `INSERT` direto. O fluxo de eventos de streaming também não diferencia eventos de exclusão (`DELETE`).
* **Impacto:** Atualizações no banco relacional resultam em registros duplicados com o mesmo ID no DuckDB. Registros apagados no banco primário permanecem indefinidamente no cache de busca semântica.
* **Remediação Recomendada:**
  - Definir `PRIMARY KEY (id)` na tabela DuckDB.
  - Implementar semântica de *Upsert* (ou `DELETE WHERE id = ?` transacional prévio à inserção do lote).
  - Tratar eventos de `DELETE` no adaptador de ingestão para remoção de vetores correspondentes.

---

### 5. Incompatibilidade com o Ciclo de Vida do Protocolo MCP
* **Localização:** `internal/server/mcp.go`
* **Descrição do Problema:** O endpoint HTTP aceita exclusivamente os métodos `tools/list` e `tools/call`. A especificação oficial do *Model Context Protocol* (MCP) determina que toda conexão de cliente inicie obrigatoriamente com o handshake `initialize`, seguido de `notifications/initialized`.
* **Impacto:** Clientes MCP oficiais padrão (como Claude Desktop ou Cursor) falham imediatamente na conexão inicial com o erro `-32601 Method not found`.
* **Remediação Recomendada:**
  - Implementar handlers para `initialize` (retornando capabilities do servidor e protocolo suportado) e `notifications/initialized`.
  - Tipar as respostas em blocos de conteúdo compatíveis com o padrão MCP (`TextContent`).

---

## 🟡 Achados Secundários (P2: Robustez, Governança e Realismo Técnico)

### 6. Heurística Anti-Injeção Baseada em Assinaturas Fixas
* **Localização:** `internal/governance/scanner.go`
* **Análise:** O scanner utiliza uma expressão regular RE2 compilada contendo 3 padrões estáticos (`ignore...instructions`, `acesso...root`).
* **Diretriz de Engenharia:** Tratar explicitamente o módulo como um **filtro heurístico de primeira linha**, e não uma garantia matemática contra *Prompt Injections*. O modelo de ameaças deve orientar os consumidores downstream a tratar o contexto recuperado como entrada não confiável.

---

### 7. Ciclo de Vida e Entropia do Cofre de PII (Redis Vault)
* **Localização:** `internal/governance/vault.go`
* **Análise:**
  - O cofre armazena o mapeamento reverso `token -> valor original` no Redis com TTL de 24 horas. Trata-se, portanto, de uma **pseudonimização reversível com estado**, e não anonimização irreversível.
  - O hash HMAC-SHA256 é truncado em 16 caracteres hexadecimais (64 bits). Embora proteja contra rainbow tables, 64 bits de entropia apresentam possibilidade estatística de colisão em corpora massivos sem um resolvedor de conflito.
* **Diretriz de Engenharia:** Documentar formalmente o cofre como pseudonimização reversível de sessão. Empregar no mínimo 128 bits de hash ou adicionar mecanismo de fallback em caso de colisão no Redis.

---

### 8. Busca Vetorial Exata (SIMD) vs. Índice Aproximado (ANN/HNSW)
* **Localização:** `internal/engine/duckdb.go`
* **Análise:** O DuckDB executa `array_cosine_distance` via força bruta paralelizada em C++ (SIMD). Embora a extensão VSS seja carregada (`INSTALL vss; LOAD vss;`), nenhum comando `CREATE INDEX ... USING HNSW` é executado na tabela.
* **Diretriz de Engenharia:** Esclarecer na documentação que o mecanismo atual realiza busca exata (*Exact k-NN* via SIMD), ideal para dezenas/centenas de milhares de registros locais com precisão de 100%, e planejar a ativação de índices HNSW para corpora na casa dos milhões.

---

### 9. Fronteiras de Segurança e Autenticação no Transporte gRPC
* **Localização:** `cmd/jaguarbase/main.go`, `internal/server/grpc.go`
* **Análise:** O gRPC escuta na porta `:50051` sem interceptor de autenticação (a variável `JAGUARBASE_API_KEY` protege exclusivamente o HTTP/MCP).
* **Diretriz de Engenharia:** Em arquiteturas de microsserviços onde a porta não estiver restrita a localhost ou a um service mesh mTLS, permitir a injeção opcional de um interceptor de metadata (`Authorization: Bearer <token>`) idêntico ao plano HTTP.

---

### 10. Metodologia de Métricas de Memória em Chaos Tests
* **Localização:** `cmd/chaos/breaking_point/main.go`, `docs/BENCHMARKS.md`
* **Análise:** A função `getProcessMemoryMB` utiliza `runtime.MemStats.Alloc`, medindo estritamente a Heap gerenciada pelo Go (<10MB). Como o DuckDB aloca memória nativa C++ fora da heap do Go através de CGO, essa métrica não reflete o RSS (*Resident Set Size*) real do processo.
* **Diretriz de Engenharia:** Coletar métricas via `/proc/self/statm` ou cgroups para reportar o consumo real de memória do sistema operacional em relatórios futuros.

---

## 📋 Plano de Ação & Roadmap de Correções (v1.1.0)

| ID | Item | Arquivo Afetado | Prioridade | Status |
| :--- | :--- | :--- | :--- | :--- |
| **P1-1** | Mapeamento canônico `content_column` no pipeline | `pipeline.go` & `postgres_adapter.go` | **Crítico** | Planejado |
| **P1-2** | Handshake de inicialização do protocolo MCP | `internal/server/mcp.go` | **Crítico** | Planejado |
| **P1-3** | Primary Key e Upsert transacional no DuckDB | `internal/engine/duckdb.go` | **Crítico** | Planejado |
| **P1-4** | Propagação de erros e checkpoint resiliente no backfill | `internal/ingestion/pipeline.go` | **Crítico** | Planejado |
| **P2-1** | Interceptor de autenticação opcional no gRPC | `internal/server/grpc.go` | Médio | Planejado |
| **P2-2** | Documentação e testes de regressão dos fluxos reais | `tests/` e `README.md` | Médio | Concluído (Docs) |

# JaguarBase: Technical Architecture & Security Audit Report

[🇺🇸 Read in English](ARCHITECTURE_AUDIT.md) | [🇧🇷 Leia em Português](ARCHITECTURE_AUDIT.pt-BR.md)

**Release Status:** Beta (v1.0.0)  
**Scope:** Static Code Analysis, Data Flow Verification, Protocol Conformance & Governance Lifecycle  
**Objective:** Technical Debt Mapping, Logical Vulnerability Remediation & v1.1.0 Roadmap  

---

## 📌 Executive Summary

JaguarBase exhibits solid systems engineering foundations in Go: clean layered architecture, minimal attack surface via distroless non-root containers, a zero-warning GoSec clean build, native CGO DuckDB integration, and edge-resilience mechanisms including O(1) Load Shedding and strict HTTP timeouts.

However, an in-depth semantic audit identified **critical architectural misalignments between subsystems**, specifically in how schemas are dynamically mapped across the ingestion pipeline, cache lifecycle handling, and protocol compliance. This document consolidates all prioritized findings to serve as the definitive specification for the platform's v1.1.0 hardening.

---

## 🔴 Priority 1 Findings (Critical Correctness, Data Loss & Security)

### 1. Ingestion Payload Mapping Bypass (`content_column`)
* **Target Components:** `internal/ingestion/postgres_adapter.go`, `internal/ingestion/pipeline.go`, `internal/dynamic/processor.go`
* **Root Cause:** In the real-time PostgreSQL adapter, columns named `message` or `content` are extracted into `RawRecord.Content` and omitted from the generic metadata map. During pipeline ingestion (`pipeline.go`), `rawMap` is populated with `content`, but the column name configured in `jaguardb.yaml` (e.g., `message`) is not restored. When `ProcessRow` executes, it evaluates the declared `message` column as `nil`.
* **Impact:** The `untrusted_input` scanner is bypassed for real-time events on custom column names, allowing tainted payloads to be persisted with `_is_tainted = false`.
* **Required Remediation:**
  - Establish a single canonical schema resolver across adapters.
  - The pipeline must map `record.Content` into `rawMap` under the exact identifier specified by `content_column`.
  - Ensure the exact string inspected and sanitized by the governance processor is the one delivered to the embedder and DuckDB storage.

---

### 2. Event Loss Race Window Between Historical Backfill and Real-Time Stream
* **Target Components:** `internal/ingestion/pipeline.go` (`Start` and `performBackfill`)
* **Root Cause:** The `Start` function completes `performBackfill` synchronously before spawning the real-time `source.Watch` listener (which registers the PostgreSQL `LISTEN` command).
* **Impact:** Any `INSERT` or `UPDATE` occurring on the primary PostgreSQL database during backfill emits a `NOTIFY` that is permanently dropped, as no listener has subscribed yet. Standard `LISTEN/NOTIFY` provides no durable replay for offline clients.
* **Required Remediation:**
  - Subscribe to `LISTEN` concurrently before or during snapshot backfill, buffering events in a bounded channel.
  - Plan long-term transition to a durable CDC model (PostgreSQL Logical Replication / transactional outbox table).

---

### 3. Partial Batch Ingestion Failures Advancing Checkpoint Pointers
* **Target Components:** `internal/ingestion/pipeline.go` (`performBackfill`)
* **Root Cause:** `processRecord` returns `void` without propagating errors back to the backfill loop. If an intermediate step fails (e.g., Ollama embedding timeout or database write error), the failure is merely logged while `totalSynced` increments and `lastID` advances.
* **Impact:** Records that fail to ingest are skipped permanently. On restart, DuckDB's `GetMaxID` queries the highest recorded ID, leaving permanent silent data gaps in the local cache.
* **Required Remediation:**
  - Refactor `processRecord` to return explicit `error`.
  - Implement bounded retries with exponential backoff for transient failures (e.g., LLM/network).
  - On permanent failure, route the record to a durable dead-letter queue (DLQ) or halt checkpoint advancement.

---

### 4. Cache Desynchronization: Lack of Primary Key Constraints, Upserts, and Deletions
* **Target Components:** `internal/engine/duckdb.go` (`AutoMigrate`, `InsertBatch`), `internal/ingestion/postgres_adapter.go`
* **Root Cause:** The DuckDB canonical table schema lacks a `PRIMARY KEY (id)` constraint. `InsertBatch` executes standard `INSERT` statements. Furthermore, the streaming event pipeline does not handle `DELETE` operations or tombstones.
* **Impact:** Updates on the primary database create duplicate rows with the same ID in DuckDB. Rows deleted from the system of record persist indefinitely in the vector cache, violating data retention and GDPR/LGPD requirements.
* **Required Remediation:**
  - Enforce `PRIMARY KEY (id)` in DuckDB's `AutoMigrate`.
  - Implement idempotent upsert semantics (e.g., transactional `DELETE WHERE id = ?` prior to batch insert).
  - Add explicit `DELETE` event routing to purge stale vectors from DuckDB upon primary deletion.

---

### 5. Model Context Protocol (MCP) Lifecycle Non-Conformance
* **Target Components:** `internal/server/mcp.go`
* **Root Cause:** The HTTP JSON-RPC router only accepts `tools/list` and `tools/call`. Official MCP client specifications (Anthropic Model Context Protocol) mandate an initial handshake: `initialize` followed by `notifications/initialized`.
* **Impact:** Standard MCP clients (Claude Desktop, Cursor MCP connector) fail during handshake with `-32601 Method not found`.
* **Required Remediation:**
  - Implement the `initialize` method returning server capabilities, protocol version, and server metadata.
  - Accept `notifications/initialized`.
  - Return typed MCP content blocks (`TextContent`) in tool responses.

---

## 🟡 Priority 2 Findings (Hardening, Governance & Technical Realism)

### 6. Heuristic Anti-Injection Scanner Boundaries
* **Target Components:** `internal/governance/scanner.go`
* **Analysis:** The detector relies on an RE2 regex matching 3 static evasion patterns (`ignore...instructions`, `acesso...root`).
* **Engineering Directive:** Formally document this component as a **first-line heuristic filter**, not a cryptographic or mathematical injection firewall. Consumers must be instructed to treat retrieved context as untrusted input.

---

### 7. PII Vault Lifecycle, Entropy, and Scope
* **Target Components:** `internal/governance/vault.go`
* **Analysis:**
  - Sensitive plaintext is stored reversibly in Redis under a 24-hour TTL (`vault_{token}`). This is **stateful session pseudonymization**, not irreversible anonymization.
  - Truncating HMAC-SHA256 to 16 hex characters yields 64 bits of entropy. While safe against precomputation, 64-bit truncated hashes are not collision-immune at high scale.
* **Engineering Directive:** Document the vault as a reversible tokenization cache. Increase hash length to 128 bits (32 hex characters) and implement collision mitigation.

---

### 8. Exact Cosine Distance (SIMD) vs. Approximate Vector Indexing (ANN/HNSW)
* **Target Components:** `internal/engine/duckdb.go`
* **Analysis:** The engine computes exact brute-force cosine distance (`array_cosine_distance`) accelerated by DuckDB's C++ SIMD instructions. Although the VSS extension is loaded (`INSTALL vss; LOAD vss;`), no `CREATE INDEX ... USING HNSW` is created on the vector column.
* **Engineering Directive:** Clarify in documentation that the engine currently provides **Exact k-NN via SIMD**, which guarantees 100% recall for tens/hundreds of thousands of vectors. Plan true HNSW index creation for larger-scale workloads.

---

### 9. gRPC Transport Security Boundaries
* **Target Components:** `cmd/jaguarbase/main.go`, `internal/server/grpc.go`
* **Analysis:** The gRPC server listens on `:50051` without transport credentials or metadata authentication interceptors (`JAGUARBASE_API_KEY` only shields HTTP/MCP).
* **Engineering Directive:** Provide an optional metadata interceptor (`Authorization: Bearer <token>`) for deployments where the gRPC port is exposed beyond trusted localhost/service mesh perimeters.

---

### 10. Memory Accounting Methodology in Benchmarks
* **Target Components:** `cmd/chaos/breaking_point/main.go`, `docs/BENCHMARKS.md`
* **Analysis:** `getProcessMemoryMB` relies on `runtime.MemStats.Alloc`, which only tracks the Go runtime managed heap (<10MB). Because DuckDB allocates native C++ memory outside the Go runtime via CGO, this does not represent the process's true Resident Set Size (RSS).
* **Engineering Directive:** Collect metrics from `/proc/self/statm` or OS cgroups to report accurate total physical memory consumption in future benchmark publications.

---

## 📋 Remediation Roadmap (v1.1.0)

| ID | Issue | Target Component | Priority | Status |
| :--- | :--- | :--- | :--- | :--- |
| **P1-1** | Canonical `content_column` mapping in pipeline | `pipeline.go` & `postgres_adapter.go` | **Critical** | Planned |
| **P1-2** | MCP `initialize` & `notifications/initialized` handshake | `internal/server/mcp.go` | **Critical** | Planned |
| **P1-3** | DuckDB Primary Key constraint & Idempotent Upsert | `internal/engine/duckdb.go` | **Critical** | Planned |
| **P1-4** | Error propagation & resilient backfill checkpointing | `internal/ingestion/pipeline.go` | **Critical** | Planned |
| **P2-1** | Optional Bearer auth interceptor for gRPC plane | `internal/server/grpc.go` | Medium | Planned |
| **P2-2** | Transparent positioning & heuristic disclaimer in docs | `README.md` & `README.pt-BR.md` | Medium | Completed |

