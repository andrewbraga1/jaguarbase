# JaguarBase 🐆

[🇺🇸 Read in English](README.md) | [🇧🇷 Leia em Português](README.pt-BR.md)

![Go Version](https://img.shields.io/badge/Go-1.23-00ADD8?style=flat&logo=go)
![DuckDB](https://img.shields.io/badge/DuckDB-Embedded-FFF000?style=flat)
![Status](https://img.shields.io/badge/Status-Production_Ready-success)
![CI](https://github.com/andrewbraga1/jaguarbase/actions/workflows/ci.yml/badge.svg)

**Uma Engine Vetorial Agnóstica de Alta Performance e Zero-Trust em Go (acelerada por SIMD via DuckDB C++)**

O JaguarBase é um appliance de infraestrutura orientado a eventos e altamente desacoplado, projetado do zero para Agentes de IA Autônomos e IA de Borda (*Edge AI*). Ele atua como uma ponte segura e de alta performance entre seus bancos de dados relacionais legados (PostgreSQL) e a janela de contexto da sua IA, garantindo que nenhum vazamento de PII e nenhuma injeção de prompt alcance o seu LLM.

## 🧠 A Tese Arquitetural: CQRS & Edge Caching

Arquiteturas tradicionais forçam os Agentes de IA a realizar cálculos vetoriais pesados diretamente no banco de dados relacional primário (ex: PostgreSQL com `pgvector`), gerando picos de uso de CPU e arriscando a estabilidade dos sistemas centrais do negócio.

**O JaguarBase inverte esse paradigma usando CQRS (Segregação de Responsabilidade de Comando e Consulta):**
1. **Sistema de Registro:** Mantenha seus dados seguros no seu PostgreSQL legado.
2. **Sincronização Orientada a Eventos:** O JaguarBase escuta os canais `LISTEN/NOTIFY` do PostgreSQL e, de forma assíncrona, extrai/vetoriza os dados para sua engine local DuckDB sem onerar o banco de dados transacional.
3. **Inferência na Borda (Edge):** Os Agentes de IA consultam o cache local do JaguarBase em milissegundos.
4. **Resultado:** Zero latência de rede para a IA e impacto zero de CPU no seu PostgreSQL primário.

## 🚀 Principais Funcionalidades

- **Arquitetura 100% Orientada a Manifesto:** Configurado estritamente via `jaguardb.yaml`. Você define o esquema, o JaguarBase se adapta dinamicamente. Zero structs *hardcoded*. Ele utiliza a definição `virtual: true` para desacoplar colunas transacionais brutas do esquema vetorial RAG enriquecido.
- **Armazenamento Vetorial Embutido (In-Process):** Movido por DuckDB + VSS. Sem redes de bancos de dados externos. Cálculos vetoriais em C++ SIMD extremamente rápidos envelopados em Go.
- **Governança Zero-Trust:** Mascaramento de PII embutido e Provas de Autenticidade (HMAC Handoff). Prompts corrompidos ou maliciosos são sinalizados (`_is_tainted`) e matematicamente isolados das buscas de IA antes de chegarem à janela de contexto.
- **Protocolo Dual-Plane:**
  - **Plano de Alta Performance (gRPC):** Para microsserviços de backend que exigem latência sub-10ms.
  - **Plano de Agentes de IA (MCP):** JSON-RPC 2.0 via HTTP/REST para integração direta com Agentes de IA (Claude Desktop, Cursor, LLMs customizados).

## 📊 Benchmarks & Engenharia do Caos

Testado em um Intel i7 Dual-Core com 1.000 workers simultâneos processando vetores de 1536 dimensões. Veja o relatório completo em [BENCHMARKS.md](docs/BENCHMARKS.md).

| Cenário | Total de Requisições | Throughput (Req/Seg) | Taxa de Sucesso |
| :--- | :--- | :--- | :--- |
| **[gRPC] Health Ping** | 100,000 | **43.584,82 req/s** | 100% |
| **[gRPC] Busca Vetorial** | 10,000 | **1.050,87 req/s** | 100% |
| **[HTTP/MCP] JSON-RPC** | 10,000 | **831,80 req/s** | 100% |

**Resiliência de Hardware (Engenharia do Caos):**
- **Load Shedding O(1):** Intercepta loops descontrolados (ex: loops infinitos do LangChain) e bloqueia requisições (HTTP 429) na camada do Roteador Go em **< 0,5ms**.
- **Zero Vazamento de Memória:** Consome **< 10MB de RAM (Go Heap)** enquanto gerencia centenas de milhares de vetores.
- **OS Page Caching:** Transfere inteligentemente os índices vetoriais para o Cache de Buffer do Kernel do Linux, prevenindo quedas da aplicação por falta de memória (OOM).

## 💼 Posicionamento de Mercado

- **Precisa indexar 1 Bilhão de vetores na nuvem para busca global?** Use uma engine distribuída como Qdrant ou Milvus em conjunto com o JaguarGate.
- **Precisa construir um App de IA Local robusto, Ferramenta Desktop, ou proteger seu Postgres Corporativo dos picos de CPU dos Agentes?** Use o **JaguarBase**.

## 🛠️ Início Rápido

Você pode rodar o JaguarBase inteiramente via Docker, injetando sua configuração YAML em tempo de execução:

```bash
# 1. Inicie o JaguarBase no Docker
docker run -d \
  --name jaguarbase-edge \
  --network="host" \
  --env-file .env \
  -v "$(pwd)/jaguardb.yaml:/app/jaguardb.yaml" \
  -v "$(pwd)/data:/app/data" \
  andrewbraga1/jaguarbase:latest

# 2. Visualize os logs de ingestão em tempo real
docker logs -f jaguarbase-edge
```

*Para desenvolvimento local (Requer CGO):*
```bash
sudo apt-get install build-essential gcc
CGO_ENABLED=1 go run cmd/jaguarbase/main.go
```

## 🛣️ Roadmap

- [x] **Fase 1: PoC e Domínio Central** - Adaptador Postgres, Cofre Redis, DuckDB SIMD, Validação de Segurança Handoff.
- [x] **Fase 2: Configuração Dinâmica de Esquema** - Parser do `jaguardb.yaml`, ingestão agnóstica de tipos e remoção de structs.
- [x] **Fase 3: Busca Vetorial (RAG)** - Extensões DuckDB VSS, embeddings dinâmicos e Similaridade Semântica segura por padrão (fail-closed).
- [x] **Fase 4: Borda Multi-Protocolo** - Camada de Borda de alta performance (gRPC) e Servidor MCP (para Agentes de IA).
- [x] **Fase 5: Engenharia do Caos e Resiliência** - Load Shedding O(1), Cancelamento de Contexto e limites de hardware (Disco/RAM).
- [x] **Fase 6: Containerização e CI/CD** - Dockerfile multi-estágio (Debian Slim + CGO) e automação de deploy.