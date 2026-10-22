# JaguarBase 🐆

[🇺🇸 Read in English](README.md) | [🇧🇷 Leia em Português](README.pt-BR.md)

**A Camada de Banco de Dados Zero-Trust e AI-Native para Agentes Autônomos.**

O JaguarBase é um motor de armazenamento (storage engine) *stateful* desenhado do zero para agentes LLM. Ele atua como uma ponte segura e de altíssima performance entre seus bancos de dados relacionais legados (PostgreSQL) e a janela de contexto da sua IA, garantindo que nenhum Dado Pessoal Sensível (PII) vaze e que nenhuma injeção de prompt alcance seu LLM.

## 🏗 Arquitetura e Fluxo de Dados

O JaguarBase opera em um modelo de **Governança Zero-Trust no Tempo de Escrita (Write-Time)** combinado com uma estratégia de ingestão **"Air Gap"**. Para garantir latência de submilisegundos e segurança determinística, o JaguarBase **não** consulta bancos transacionais (como PostgreSQL) em tempo real.

O ciclo de vida dos dados segue quatro estágios isolados:

1. **O Air Gap (Ingestão):** Um *worker* em background (Fase 5) busca novos dados brutos de fontes transacionais (ex: PostgreSQL) via CDC ou *polling*, isolando o motor vetorial das cargas pesadas de produção.
2. **Governança no Tempo de Escrita:**
   - **Cofre de PII (PII Vault):** Tokeniza dados sensíveis deterministicamente usando Redis como loja de estado seguro L2.
   - **Scanner:** Executa regex RE2 em $O(N)$ para detectar e isolar Prompt Injections antes que cheguem aos embeddings.
3. **Kernel de Execução (Armazenamento Vetorial):** Os dados higienizados e os embeddings de alta dimensão são armazenados em um arquivo autossuficiente `storage.duckdb` usando DuckDB VSS (formato colunar acelerado via SIMD).
4. **Recuperação Multi-Protocolo:** Serve agentes de IA diretamente do DuckDB via interfaces de alta performance (gRPC para microsserviços, JSON-RPC/MCP para LLMs), protegidos por um Paginador Cognitivo e Handoff Zero-Trust via HMAC-SHA256.

## 🚀 Executando os Testes de Integração

```bash
# 1. Rodar os testes do Domínio Core e Segurança
go test -v -race ./internal/security/... ./internal/pagination/... ./internal/dynamic/...

# 2. Rodar os testes do Motor DuckDB e VSS (Exige CGO_ENABLED=1)
CGO_ENABLED=1 go test -v -race ./internal/engine/...
```

## 🛣 Roadmap

- [x] **Fase 1: PoC e Domínio Core** - Validação do Adapter Postgres, PII Vault no Redis, Motor SIMD DuckDB e Handoff de Segurança.
- [x] **Fase 2: Configuração de Schema Dinâmico** - Parser do `jaguardb.yaml`, ingestão agnóstica a tipos (`map[string]any`) e remoção de structs estáticas.
- [x] **Fase 3: Busca Vetorial (RAG)** - Extensões vetoriais no DuckDB (VSS), suporte a embeddings dinâmicos (`FLOAT[N]`) e similaridade semântica (Cosseno) Fail-Closed.
- [x] **Fase 4: Borda Multi-Protocolo** - Camada de borda de alta performance (gRPC/Arrow Flight) e Servidor MCP (para Agentes de IA), junto com endpoints de telemetria.
- [ ] **Fase 5: Pipeline de Ingestão** (Worker Air-Gap do PostgreSQL para DuckDB, Geração de Embeddings)