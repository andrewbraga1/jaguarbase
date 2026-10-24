# JaguarBase 🐆

[🇺🇸 Read in English](README.md) | [🇧🇷 Leia em Português](README.pt-BR.md)

![Go Version](https://img.shields.io/badge/Go-1.23-00ADD8?style=flat&logo=go)
![DuckDB](https://img.shields.io/badge/DuckDB-Embedded-FFF000?style=flat)
![Status](https://img.shields.io/badge/Status-Production_Ready-success)
![CI](https://github.com/andrewbraga1/jaguarbase/actions/workflows/ci.yml/badge.svg)

**Um Motor Vetorial Agnóstico, de Alta Performance e Zero-Trust em Go (Acelerado por SIMD via DuckDB C++)**

O JaguarBase é um *appliance* de infraestrutura altamente desacoplado e orientado a eventos, projetado do zero para Agentes Autônomos de IA e IA de Borda (Edge AI). Ele atua como uma ponte segura e de altíssima performance entre seus bancos de dados relacionais legados (PostgreSQL) e a janela de contexto da sua IA, garantindo que nenhum dado sensível (PII) vaze e nenhuma Injeção de Prompt (*Prompt Injection*) alcance o seu LLM.

## 🧠 A Tese Arquitetural: CQRS e Edge Caching

Arquiteturas tradicionais forçam Agentes de IA a realizar cálculos matemáticos vetoriais pesados diretamente no banco de dados relacional primário (ex: PostgreSQL com `pgvector`), gerando picos de uso de CPU e colocando em risco a estabilidade dos sistemas *core* do negócio.

**O JaguarBase inverte esse paradigma usando CQRS (Command Query Responsibility Segregation):**
1. **Sistema de Registro (System of Record):** Mantenha seus dados em segurança no seu PostgreSQL legado.
2. **Sincronização Orientada a Eventos:** O JaguarBase escuta os canais `LISTEN/NOTIFY` do PostgreSQL e extrai/vetoriza os dados de forma assíncrona para o seu motor DuckDB local, sem causar lentidão no seu banco transacional.
3. **Inferência na Borda (Edge):** Agentes de IA consultam o cache local do JaguarBase em milissegundos.
4. **Resultado:** Latência de rede zero para a IA e impacto absoluto de zero na CPU do seu PostgreSQL primário.

## 🚀 Principais Funcionalidades

- **Arquitetura Agnóstica:** Configurado via `jaguardb.yaml`. Você define o *schema*, e o JaguarBase se adapta dinamicamente. Sem *structs* ou colunas hardcoded (engessadas no código).
- **Armazenamento Vetorial In-Process:** Movido a DuckDB + VSS. Sem redes de banco de dados externas. Matemática vetorial ultrarrápida em C++ SIMD dentro de um *wrapper* em Go.
- **Governança Zero-Trust:** Mascaramento nativo de PII e Provas de Handoff HMAC. Prompts corrompidos ou maliciosos são sinalizados (`_is_tainted`) e matematicamente isolados das buscas de IA antes de atingirem a janela de contexto.
- **Protocolo Dual-Plane:**
  - **Plano de Alta Performance (gRPC):** Para microsserviços de backend que exigem latência sub-10ms.
  - **Plano de Agentes de IA (MCP):** JSON-RPC 2.0 via HTTP/REST para integração direta com Agentes de IA (Claude Desktop, Cursor, LLMs customizados).

## 📊 Benchmarks e Engenharia de Caos

Testado em um modesto Dual-Core Intel i7 com 1.000 *workers* concorrentes e simultâneos processando vetores de 1536 dimensões. Veja o relatório completo em [BENCHMARKS.md](docs/BENCHMARKS.md).

| Cenário | Total de Requisições | Throughput (Req/Seg) | Taxa de Sucesso |
| :--- | :--- | :--- | :--- |
| **[gRPC] Health Ping** | 100.000 | **43.584,82 req/s** | 100% |
| **[gRPC] Vector Search** | 10.000 | **1.050,87 req/s** | 100% |
| **[HTTP/MCP] JSON-RPC** | 10.000 | **831,80 req/s** | 100% |

**Resiliência de Hardware (Chaos Engineering):**
- **Load Shedding O(1):** Intercepta "Runaway Loops" (ex: loops infinitos do LangChain) e bloqueia as requisições (HTTP 429) na camada do Go Router em **< 0,5ms**.
- **Zero Vazamento de Memória (Memory Leaks):** Consome **< 10MB de RAM (Go Heap)** enquanto gerencia centenas de milhares de vetores.
- **OS Page Caching:** Transfere inteligentemente os índices vetoriais para o *Buffer Cache* do Kernel do Linux, prevenindo travamentos por OOM (Out Of Memory) no nível da aplicação.

## 💼 Posicionamento de Mercado

- **Precisa indexar 1 Bilhão de vetores na nuvem para uma busca global?** Use um motor distribuído como Qdrant ou Milvus em conjunto com o JaguarGate.
- **Precisa construir um App Local de IA, Ferramenta Desktop robusta ou proteger seu Postgres Corporativo de picos de CPU dos Agentes?** Use o **JaguarBase**.

## 🛠️ Início Rápido (Quick Start)

```bash
# 1. Instale as Dependências CGO (Obrigatório para os bindings C++ do DuckDB)
sudo apt-get install build-essential gcc

# 2. Rode o Servidor JaguarBase
CGO_ENABLED=1 go run cmd/jaguarbase/main.go

# 3. Em outro terminal, rode o Teste de Estresse de Alta Concorrência
go run cmd/stress/main.go
```

## 🛣️ Roteiro (Roadmap)

- [x] **Fase 1: PoC e Domínio Core** - Adaptador Postgres, Cofre Redis (Vault), DuckDB SIMD, Validação de Segurança Handoff.
- [x] **Fase 2: Configuração de Schema Dinâmico** - Parser de `jaguardb.yaml`, ingestão agnóstica a tipos, e remoção de structs fixas.
- [x] **Fase 3: Busca Vetorial (RAG)** - Extensões DuckDB VSS, embeddings dinâmicos e Busca de Similaridade Semântica (fail-closed).
- [x] **Fase 4: Borda Multi-Protocolo (Multi-Protocol Edge)** - Camada de Borda de Alta Performance (gRPC) e Servidor MCP (para Agentes de IA).
- [x] **Fase 5: Engenharia de Caos e Resiliência** - Load Shedding O(1), Cancelamento de Contexto, e limites a nível de hardware (Disco/RAM).
- [ ] **Fase 6: Containerização e CI/CD** - Dockerfile multi-stage (Debian Slim + CGO) e automação de *deploy*.