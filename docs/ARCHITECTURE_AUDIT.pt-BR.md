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
* **Descrição do Problema:** No adaptador PostgreSQL em tempo real, colunas nomeadas `message` ou `content` eram extraídas para `RawRecord.Content` e removidas do mapa de metadados genéricos. No `pipeline.go`, o mapa `rawMap` recebia `content`, mas não repunha a chave configurada no manifesto (ex.: `message`). Ao executar `ProcessRow`, a coluna declarada no YAML com governança `untrusted_input` era avaliada como ausente (`nil`).
* **Impacto:** O scanner anti-injeção não era disparado na coluna customizada de mensagem em tempo real, permitindo que payloads maliciosos fossem inseridos com `_is_tainted = false`. Além disso, tokens do cofre de PII eram gravados na coluna customizada enquanto o `Content` sem máscara vazava para o vetorizador.
* **Remediação Implementada (`fix/gringo-audit-remediation`):**
  - Mapeamento de schema canônico único implementado em `postgres_adapter.go` e `pipeline.go`.
  - `record.Content` é mapeado tanto sob o `content_column` canônico quanto sob o fallback `content`.
  - A string exata inspecionada e sanitizada pelo processador de governança é sincronizada com o vetorizador e o armazenamento DuckDB.
  - Testes de regressão adicionados em `internal/ingestion/pipeline_test.go`.

---

### 2. Janela de Perda de Eventos entre Backfill e Streaming Real-Time
* **Localização:** `internal/ingestion/pipeline.go` (`Start` e `performBackfill`)
* **Descrição do Problema:** O método `Start` executava o `performBackfill` de forma totalmente síncrona antes de iniciar a goroutine do `source.Watch` (que executa o `LISTEN`).
* **Impacto:** Quaisquer eventos de `INSERT`/`UPDATE` ocorridos no PostgreSQL durante a execução do backfill geravam notificações `NOTIFY` descartadas, pois o cliente ainda não havia aberto a conexão de escuta.
* **Remediação Implementada (`fix/gringo-audit-remediation`):**
  - O watcher em tempo real `source.Watch` inicia imediatamente em `Start` com buffer assíncrono de 2048 registros antes do início da carga histórica.
  - O backfill histórico executa com reconciliação.
  - A camada de upsert idempotente no DuckDB reconcilia qualquer sobreposição sem duplicar registros ou corromper o estado do cache vetorial.

---

### 3. Falhas Parciais no Backfill Avançando o Ponteiro de Checkpoint
* **Localização:** `internal/ingestion/pipeline.go` (`performBackfill`, `processRecord`)
* **Descrição do Problema:** `processRecord` retornava `void` sem propagar erros para o laço de paginação do backfill. Se uma etapa intermediária falhasse (ex.: timeout na geração de embeddings via Ollama ou falha de escrita), o loop continuava e o `lastID` avançava normalmente.
* **Impacto:** Registros com falha eram ignorados permanentemente. Se a aplicação reiniciasse, a busca por `MAX(id)` começava a partir do maior ID processado, deixando lacunas não sincronizadas ("buracos silenciosos") no DuckDB.
* **Remediação Implementada (`fix/gringo-audit-remediation`):**
  - Assinatura de `processRecord` refatorada para retornar `error` explícito.
  - Política de retentativa finita (3 tentativas com *exponential backoff*) implementada no backfill.
  - Em caso de esgotamento de retentativas, o backfill aborta e retorna erro **sem avançar** o `lastID` além do registro com falha, garantindo retomada íntegra.
  - Testes unitários adicionados em `internal/ingestion/pipeline_test.go`.

---

### 4. Ausência de Upsert Idempotente e Chave Primária no DuckDB
* **Localização:** `internal/engine/duckdb.go` (`AutoMigrate`, `InsertBatch`), `internal/ingestion/pipeline.go`
* **Descrição do Problema:** A tabela vetorial criada no DuckDB não possuía chave primária explícita (`PRIMARY KEY (id)`). O método `InsertBatch` realizava `INSERT INTO` direto sem suporte a substituição.
* **Impacto:** Atualizações no banco relacional ou reprocessamentos resultavam em registros duplicados com o mesmo ID no DuckDB, retendo vetores desatualizados.
* **Remediação Implementada (`fix/gringo-audit-remediation`):**
  - Declarado `id VARCHAR PRIMARY KEY` em `AutoMigrate` e suporte a `PRIMARY KEY` em `InitSchema`.
  - Implementada semântica de upsert idempotente dinâmico (`INSERT OR REPLACE INTO`) no `InsertBatch` para tabelas com chave primária.
  - Testes de integração adicionados em `internal/engine/duckdb_test.go`.

---

### 5. Incompatibilidade com o Ciclo de Vida do Protocolo MCP
* **Localização:** `internal/server/mcp.go`
* **Descrição do Problema:** O endpoint HTTP aceitava exclusivamente os métodos `tools/list` e `tools/call`. A especificação oficial do *Model Context Protocol* (MCP) determina que toda conexão de cliente inicie obrigatoriamente com o handshake `initialize`, seguido de `notifications/initialized`.
* **Impacto:** Clientes MCP oficiais padrão (como Claude Desktop ou Cursor) falhavam imediatamente na conexão inicial com o erro `-32601 Method not found`.
* **Remediação Implementada (`fix/gringo-audit-remediation`):**
  - Implementados os métodos `initialize` (retornando capabilities do servidor, versão de protocolo `2024-11-05` e metadados `jaguarbase v1.0.0`) e `notifications/initialized` (semântica 204 No Content).
  - Implementado suporte ao ping de keepalive (`ping`).
  - Testes unitários adicionados em `internal/server/mcp_test.go`.

---

## 🟡 Achados Secundários (P2: Robustez, Governança e Realismo Técnico)

### 6. Heurística Anti-Injeção Baseada em Assinaturas Fixas
* **Localização:** `internal/governance/scanner.go`
* **Análise:** O scanner utiliza uma expressão regular RE2 compilada contendo 3 padrões estáticos (`ignore...instructions`, `acesso...root`).
* **Diretriz de Engenharia:** Documentado explicitamente no `README.md` e `README.pt-BR.md` como um **filtro heurístico de primeira linha**, e não uma garantia matemática contra *Prompt Injections*. O modelo de ameaças orienta os consumidores downstream a tratar o contexto recuperado como entrada não confiável.

---

### 7. Ciclo de Vida e Entropia do Cofre de PII (Redis Vault)
* **Localização:** `internal/governance/vault.go`
* **Análise:**
  - O cofre armazena o mapeamento reverso `token -> valor original` no Redis com TTL de 24 horas. Trata-se, portanto, de uma **pseudonimização reversível com estado**, e não anonimização irreversível.
  - O hash HMAC-SHA256 é truncado em 16 caracteres hexadecimais (64 bits). Embora proteja contra rainbow tables, 64 bits de entropia apresentam possibilidade estatística de colisão em corpora massivos sem um resolvedor de conflito.
* **Diretriz de Engenharia:** Documentado formalmente o cofre como pseudonimização reversível de sessão. O roadmap prevê elevar a entropia para 128 bits (32 caracteres hexadecimais) com tratamento de colisão.

---

### 8. Busca Vetorial Exata (SIMD) vs. Índice Aproximado (ANN/HNSW)
* **Localização:** `internal/engine/duckdb.go`
* **Análise:** O DuckDB executa `array_cosine_distance` via força bruta paralelizada em C++ (SIMD). Embora a extensão VSS seja carregada (`INSTALL vss; LOAD vss;`), nenhum comando `CREATE INDEX ... USING HNSW` é executado na tabela.
* **Diretriz de Engenharia:** Esclarecido na documentação que o mecanismo atual realiza busca exata (*Exact k-NN* via SIMD), ideal para dezenas/centenas de milhares de registros locais com precisão de 100%, com roadmap para índices HNSW em corpora maiores.

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
| **P1-1** | Mapeamento canônico `content_column` no pipeline | `pipeline.go` & `postgres_adapter.go` | **Crítico** | **Concluído (`fix/gringo-audit-remediation`)** |
| **P1-2** | Subscrição e buffer de eventos pré-backfill | `internal/ingestion/pipeline.go` | **Crítico** | **Concluído (`fix/gringo-audit-remediation`)** |
| **P1-3** | Propagação de erros e checkpoint resiliente no backfill | `internal/ingestion/pipeline.go` | **Crítico** | **Concluído (`fix/gringo-audit-remediation`)** |
| **P1-4** | Primary Key e Upsert transacional no DuckDB | `internal/engine/duckdb.go` | **Crítico** | **Concluído (`fix/gringo-audit-remediation`)** |
| **P1-5** | Handshake de inicialização do protocolo MCP | `internal/server/mcp.go` | **Crítico** | **Concluído (`fix/gringo-audit-remediation`)** |
| **P2-1** | Interceptor de autenticação Bearer & API Key no gRPC | `internal/server/grpc.go` | Médio | **Concluído (`fix/gringo-audit-remediation`)** |
| **P2-2** | Entropia 128-bit do Vault e propagação de Deletes CDC | `vault.go`, `duckdb.go`, `pipeline.go` | Médio | **Concluído (`fix/gringo-audit-remediation`)** |
| **P2-3** | Documentação e testes de regressão dos fluxos reais | `README.md` & `README.pt-BR.md` | Médio | **Concluído** |

---

## 🛡️ Rodada 2 de Hardening: Consistência de Cache, Conformidade de Protocolo e Defesas Zero-Warning (`fix/audit-hardening-round-2`)

### 11. Checkpointing Isolado em Tabela de Metadados
* **Localização:** `internal/engine/duckdb.go` (`InitMetadata`, `GetCheckpoint`, `SetCheckpoint`), `internal/ingestion/pipeline.go` (`performBackfill`)
* **Descrição do Problema:** A retomada do backfill após falha consultava `SELECT MAX(id) FROM table`. Se o streaming em tempo real inserisse IDs maiores (ex.: ID 1000) enquanto o backfill falhava no ID 20, ao reiniciar o backfill pulava permanentemente os registros de 21 a 999.
* **Remediação Implementada:**
  - Criada tabela interna de metadados `_jaguarbase_metadata(key VARCHAR PRIMARY KEY, value VARCHAR)` no DuckDB.
  - O progresso do backfill é persistido sob a chave `backfill_checkpoint_<table>` exclusivamente após o processamento bem-sucedido de cada lote.
  - Ao reiniciar, o backfill retoma estritamente do checkpoint durável, eliminando qualquer pulo de registros históricos.

### 12. Sinal de Sincronização de Prontidão do Watcher
* **Localização:** `internal/ingestion/source.go` (`ReadyNotifier`), `internal/ingestion/postgres_adapter.go`, `internal/ingestion/pipeline.go` (`Start`)
* **Descrição do Problema:** Disparar `go source.Watch(...)` concorrentemente com `performBackfill` criava uma janela de corrida (race window) em microssegundos onde a leitura da carga inicial podia começar antes que o comando `LISTEN` do PostgreSQL fosse confirmado pelo banco.
* **Remediação Implementada:**
  - Definida interface `ReadyNotifier` com método `Ready() <-chan struct{}`.
  - O `PostgresAdapter` desbloqueia o canal `ready` apenas após `listener.Listen(channel)` registrar a escuta ativa no banco.
  - O `pipeline.Start` aguarda a confirmação de prontidão antes de iniciar o snapshot de dados legados.

### 13. Blocos Tipados de Conteúdo e Negociação de Versão MCP
* **Localização:** `internal/server/mcp.go`, `internal/server/mcp_test.go`
* **Descrição do Problema:** O método `tools/call` retornava os registros do banco diretamente em `content`, violando as especificações oficiais MCP (2024-11-05 e 2025-06-18) que exigem blocos de conteúdo tipados (`[{"type": "text", "text": "..."}]`), o que quebrava clientes como Claude Desktop. Além disso, o handshake `initialize` aceitava qualquer versão de protocolo arbitrariamente.
* **Remediação Implementada:**
  - Resposta do `tools/call` formatada com Content Blocks oficiais (`content: [{"type": "text", "text": jsonString}]`), campo adicional `structuredContent` e `isError: false`.
  - Adicionada negociação estrita de versão de protocolo (`2024-11-05` e `2025-06-18`), rejeitando versões incompatíveis com erro JSON-RPC `-32602`.

### 14. Ingestão Fail-Closed e Validação Rigorosa de Manifesto
* **Localização:** `internal/config/manifest.go`, `internal/ingestion/pipeline.go`
* **Descrição do Problema:** O manifesto YAML podia declarar um `content_column` inexistente no schema ou omitir chaves primárias. No `pipeline.go`, existia um fallback para o `record.Content` bruto não inspecionado caso a saída da governança não contivesse a coluna de conteúdo.
* **Remediação Implementada:**
  - O parser de manifesto valida obrigatoriamente se `content_column` existe no `schema` e se ao menos uma coluna é chave primária (`primary_key: true`).
  - O pipeline de ingestão opera em modo fail-closed: se a coluna de conteúdo sanitizada estiver ausente na saída da governança, o registro é rejeitado com erro em vez de vetorizar o texto bruto.

### 15. Segurança de Rede Localhost-by-Default
* **Localização:** `cmd/jaguarbase/main.go`
* **Descrição do Problema:** Os servidores gRPC e HTTP realizavam bind padrão em `0.0.0.0`, aceitando conexões não autenticadas em todas as interfaces de rede quando executados em ambiente de desenvolvimento sem API key configurada.
* **Remediação Implementada:**
  - Adicionada variável de configuração `JAGUARBASE_HOST` com padrão seguro `127.0.0.1`.
  - Alerta explícito de segurança registrado em log se houver bind em `0.0.0.0` sem `JAGUARBASE_API_KEY`.

### 16. Rigor em Testes e Determinismo em Engenharia do Caos
* **Localização:** `internal/engine/vss_test.go`, `cmd/chaos/runaway_loop/main.go`
* **Descrição do Problema:** O teste `vss_test.go` engolia qualquer erro de consulta via `t.Skipf`, mascarando possíveis falhas de sintaxe SQL ou do binder. O teste de caos `runaway_loop` finalizava com código 0 mesmo quando 0 requisições eram descartadas.
* **Remediação Implementada:**
  - O teste `vss_test.go` restringe `t.Skipf` exclusivamente à ausência real da extensão VSS em ambientes de teste offline; qualquer outro erro de consulta falha imediatamente (`t.Fatalf`).
  - O teste `cmd/chaos/runaway_loop/main.go` encerra com `os.Exit(1)` caso o rate limit / load shedding não descarte requisições conforme esperado ou caso o alvo esteja inacessível.

### 17. Precisão na Métricas de Memória
* **Localização:** `README.md`, `README.pt-BR.md`
* **Remediação Implementada:** Documentada claramente a fronteira entre a Heap gerenciada pelo Go (<10MB) e a memória nativa C++ do DuckDB alocada fora da heap via CGO, detalhando os limites do buffer pool do DuckDB e o cache de páginas do kernel Linux.

---

## 🛡️ Rodada 3 de Hardening: Diretrizes de Engenharia, Limites de Payload e Cifragem AES-256-GCM

### 18. Leitor de Manifesto Delimitado com Detecção de Overflow `+1 Byte`
* **Localização:** `internal/config/manifest.go`, `internal/config/manifest_test.go`
* **Remediação Implementada:** Substituído o uso de `os.ReadFile` por `io.LimitReader(file, MaxManifestSizeBytes + 1)` (teto de 1 MB), prevenindo exaustão de memória no boot e detectando arquivos que ultrapassem o limite estipulado.

### 19. Retorno Estrito de HTTP 413 e JSON-RPC -32002 no Servidor MCP
* **Localização:** `internal/server/mcp.go`, `internal/server/mcp_test.go`
* **Remediação Implementada:** Tratamento formal de `*http.MaxBytesError` disparado pelo `http.MaxBytesReader`, emitindo status HTTP `413 Payload Too Large` e código JSON-RPC `-32002` em vez de erro genérico de parsing.

### 20. Delimitação de Pool de Conexões em Adaptadores Externos
* **Localização:** `internal/ingestion/factory.go`
* **Remediação Implementada:** Configurado o pool de conexões do PostgreSQL com `SetMaxOpenConns(25)`, `SetMaxIdleConns(25)` e `SetConnMaxLifetime(5 * time.Minute)`, espelhando a disciplina aplicada ao DuckDB contra esgotamento de descritores de arquivo e conexões.

### 21. Normalização Unicode e Defesa Anti-Evasão por Caracteres Invisíveis
* **Localização:** `internal/governance/scanner.go`, `internal/governance/governance_test.go`
* **Remediação Implementada:** Higienização prévia de payloads para remoção de zero-width spaces (`\u200B`, `\u200C`, `\u200D`, `\uFEFF`), hífens suaves (`\u00AD`) e null bytes (`\x00`), impedindo bypasses de regex com caracteres ocultos.

### 22. Governança Fail-Closed para Payloads Estruturados e Aninhados
* **Localização:** `internal/dynamic/processor.go`, `internal/dynamic/processor_test.go`
* **Remediação Implementada:** Fechado o bypass estrutural em que campos JSON aninhados ou fatias ignoravam a varredura de `untrusted_input`. Valores estruturados são serializados deterministicamente em JSON e inspecionados com segurança fail-closed.

### 23. Sanitização Universal de Logs contra CRLF Injection
* **Localização:** `internal/ingestion/pipeline.go`, `internal/ingestion/postgres_adapter.go`
* **Remediação Implementada:** Sanitização de todas as saídas de log contendo dados externos (`record.ID`, `tableName`, `model`) com remoção de `\r` e `\n` para prevenir falsificação e injeção de logs.

### 24. Criptografia em Repouso no Cofre de PII com AES-256-GCM
* **Localização:** `internal/governance/vault.go`, `internal/governance/vault_test.go`
* **Remediação Implementada:** Os dados em claro de PII são criptografados com **AES-256-GCM** autenticado antes de serem gravados no Redis (usando chave derivada de `JAGUARBASE_HMAC_SECRET`), assegurando que nenhum dado sensível resida em claro em repouso.

### 25. Quality Gate de Segurança no CI sem Supressão de Erro
* **Localização:** `audit.sh`, `.github/workflows/ci.yml`
* **Remediação Implementada:** Removidos operadores `|| true` das varreduras de Trivy, GoSec e Govulncheck, alinhado o nome da tabela do manifesto de teste de carga (`support_tickets`) e integrado o Govulncheck na esteira de CI.

### 26. Quality Gate Zero-CVE em Dependências (Remediação de GO-2026-6865)
* **Localização:** `go.mod`, `go.sum`
* **Remediação Implementada:** A dependência transitiva `github.com/klauspost/compress@v1.18.7` (trazida pelo driver DuckDB) possuía vulnerabilidade de pânico/DoS (GO-2026-6865). O módulo foi atualizado explicitamente para `v1.20.2` no `go.mod`, zerando os apontamentos do Govulncheck no pipeline estrito de CI.

### 27. Quality Gate Zero-CVE na Biblioteca Padrão e Defesa Concorrente de MaxBytesReader (Remediação de GO-2026-6859)
* **Localização:** `go.mod`, `Dockerfile`, `internal/server/mcp_test.go`
* **Remediação Implementada:** A biblioteca padrão do Go (`net/http@go1.26.10`) continha vulnerabilidade de race condition em cabeçalhos sob `http.MaxBytesReader` causando crash no servidor (GO-2026-6859). A toolchain base foi atualizada de `1.26.10` para a versão corrigida `1.26.11` no `go.mod` e `Dockerfile`, e a suíte de testes foi expandida com o teste concorrente `TestMCPServer_ConcurrentMaxBytesReaderRace` sob o detector de data races (`-race`).


