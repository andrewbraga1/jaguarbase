# JaguarBase 🐆

[🇺🇸 Read in English](README.md) | [🇧🇷 Leia em Português](README.pt-BR.md)

**A Camada de Banco de Dados Zero-Trust e AI-Native para Agentes Autônomos.**

O JaguarBase é um motor de armazenamento (storage engine) *stateful* desenhado do zero para agentes LLM. Ele atua como uma ponte segura e de altíssima performance entre seus bancos de dados relacionais legados (PostgreSQL) e a janela de contexto da sua IA, garantindo que nenhum Dado Pessoal Sensível (PII) vaze e que nenhuma injeção de prompt alcance seu LLM.

## 🏗 Arquitetura

O JaguarBase opera sob o modelo de **Governança Zero-Trust em Write-Time**. Em vez de depender do LLM para filtrar dados ruins, o JaguarBase limpa os dados *antes* que eles sejam indexados para busca vetorial.

1. **Origem (PostgreSQL):** Ingestão de dados brutos via CDC ou polling.
2. **Pipeline de Governança (Redis + RE2):** 
   - **PII Vault:** Tokeniza dados sensíveis de forma determinística (ex: `111.222.333-44` vira `vault_cpf_37d6018b`) usando o Redis como cofre L2.
   - **Scanner:** Executa expressões regulares RE2 em tempo linear $O(N)$ para detectar e isolar injeções de prompt.
3. **Kernel de Execução (DuckDB):** Armazena os dados sanitizados em um formato colunar embarcado e acelerado por instruções SIMD.
4. **Recuperação pelo Agente (MCP):** Expõe uma interface JSON-RPC segura para o LLM (ex: LiteLLM/Qwen), protegida por um **Paginador Cognitivo** e validada por uma assinatura **Zero-Trust Handoff (HMAC-SHA256)**.

## ✨ Funcionalidades Principais (PoC Validada)

- **Anti-Injection em Write-Time:** Isola instruções maliciosas durante a ingestão de dados, economizando ciclos de CPU do LLM na leitura.
- **PII Vault Reversível:** Os agentes de IA "enxergam" apenas tokens criptográficos, removendo 100% de PII da janela de contexto.
- **Paginador Cognitivo:** Trunca automaticamente resultados massivos para prevenir alucinações por falta de memória de contexto (OOM) no LLM, retornando um resumo estatístico em seu lugar.
- **Zero-Trust Handoff:** Toda resposta é assinada com um *FrameProof* HMAC-SHA256, provando para o gateway de rede (JaguarGate) que o dado está limpo.

## 🚀 Executando os Testes de Integração

O projeto utiliza a biblioteca nativa de testes do Go com detecção estrita de condições de corrida (`-race`). Alguns testes requerem conexões reais com PostgreSQL e Redis.

```bash
# 1. Rodar os testes do Domínio Core e Segurança (Em memória / Não exige banco externo)
go test -v -race ./internal/security/... ./internal/pagination/...

# 2. Rodar os testes do Motor DuckDB Embarcado (Exige CGO_ENABLED=1)
CGO_ENABLED=1 go test -v -race ./internal/engine/...

# 3. Rodar os testes de Integração do Adapter PostgreSQL
TEST_DATABASE_URL="postgres://usuario:senha@localhost:5432/banco?sslmode=disable" \
go test -v -race ./internal/adapter/...

# 4. Rodar a Pipeline de Governança (Integração Postgres + Redis)
TEST_DATABASE_URL="postgres://usuario:senha@localhost:5432/banco?sslmode=disable" \
TEST_REDIS_URL="redis://localhost:6379" \
go test -v -race ./internal/governance/...
```

## 🛣 Roadmap

- [x] **Fase 1: PoC e Domínio Core** - Validação do Adapter Postgres, PII Vault no Redis, Motor SIMD DuckDB e Handoff de Segurança.
- [ ] **Fase 2: Configuração de Schema Dinâmico** - Parser do `jaguardb.yaml`, ingestão agnóstica a tipos (`map[string]any`) e remoção de structs estáticas.
- [ ] **Fase 3: Busca Vetorial (RAG)** - Extensões vetoriais no DuckDB, integração de embeddings e execução de similaridade semântica.
- [ ] **Fase 4: Servidor MCP e Arrow Flight** - Implementação completa do Model Context Protocol para integração com LiteLLM/Qwen e transporte de rede *Zero-Copy*.