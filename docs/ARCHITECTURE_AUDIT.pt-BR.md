# JaguarBase: Relatório Técnico de Auditoria Arquitetural & Segurança

[🇺🇸 Read in English](ARCHITECTURE_AUDIT.md) | [🇧🇷 Leia em Português](ARCHITECTURE_AUDIT.pt-BR.md)

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

