# JaguarBase 🐆

[🇺🇸 Read in English](README.md) | [🇧🇷 Leia em Português](README.pt-BR.md)

**A Camada de Banco de Dados Zero-Trust e AI-Native para Agentes Autônomos.**

O JaguarBase é um motor de armazenamento (storage engine) *stateful* desenhado do zero para agentes LLM. Ele atua como uma ponte segura e de altíssima performance entre seus bancos de dados relacionais legados (PostgreSQL) e a janela de contexto da sua IA, garantindo que nenhum Dado Pessoal Sensível (PII) vaze e que nenhuma injeção de prompt alcance seu LLM.

## 🏗 Arquitetura

O JaguarBase opera sob o modelo de **Governança Zero-Trust em Write-Time**. Em vez de depender do LLM para filtrar dados ruins, o JaguarBase limpa os dados *antes* que eles sejam indexados para busca vetorial.

1. **Origem (PostgreSQL):** Ingestão de dados brutos via CDC ou polling.
2. **Pipeline de Governança (Redis + RE2):** 
   - **PII Vault:** Tokeniza dados sensíveis de forma determinística usando o Redis como cofre L2.
   - **Scanner:** Executa expressões regulares RE2 em tempo linear $O(N)$ para detectar e isolar injeções de prompt.
3. **Kernel de Execução (DuckDB VSS):** Armazena os dados sanitizados e embeddings de alta dimensão em um formato colunar embarcado e acelerado por instruções SIMD.
4. **Recuperação Multi-Protocolo:** Expõe interfaces de altíssima performance para backends tradicionais e uma interface MCP (Model Context Protocol) para LLMs, protegida por um **Paginador Cognitivo** e assinada por um **Zero-Trust Handoff (HMAC-SHA256)**.

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
- [ ] **Fase 4: Borda Multi-Protocolo** - Camada de borda de alta performance (gRPC/Arrow Flight) e Servidor MCP (para Agentes de IA), junto com endpoints de telemetria.