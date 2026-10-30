# JaguarBase 🐆

[🇺🇸 Read in English](README.md) | [🇧🇷 Leia em Português](README.pt-BR.md)

![Go Version](https://img.shields.io/badge/Go-1.26-00ADD8?style=flat&logo=go)
![DuckDB](https://img.shields.io/badge/DuckDB-Embedded-FFF000?style=flat)
![Status](https://img.shields.io/badge/Status-Production_Ready-success)
![AppSec](https://img.shields.io/badge/AppSec-Zero_CVEs-brightgreen)
![CI](https://github.com/andrewbraga1/jaguarbase/actions/workflows/ci.yml/badge.svg)

**Uma Engine Vetorial Agnóstica de Alta Performance e Zero-Trust em Go (Acelerada por SIMD via DuckDB C++)**

O JaguarBase é um appliance de infraestrutura guiado a eventos (event-driven) e altamente desacoplado, desenhado do zero para Agentes IA Autônomos e Edge AI. Ele atua como uma ponte segura e de altíssima performance entre seus bancos de dados transacionais (ex: PostgreSQL, MySQL, streams Kafka) e a janela de contexto da sua Inteligência Artificial, garantindo que nenhum vazamento de PII (Dados Pessoais) e nenhuma Injeção de Prompt cheguem ao seu LLM.

---

## 🧠 A Tese Arquitetural: CQRS & Edge Caching

Arquiteturas tradicionais forçam os Agentes IA a realizarem cálculos matemáticos vetoriais pesados diretamente no banco de dados relacional primário (ex: PostgreSQL com `pgvector`), gerando picos de uso de CPU e arriscando a estabilidade dos sistemas vitais do negócio.

**O JaguarBase inverte esse paradigma usando CQRS (Command Query Responsibility Segregation):**
1. **Sistema de Registro (System of Record):** Mantenha seus dados seguros no seu banco transacional primário.
2. **Sincronização Orientada a Eventos:** O JaguarBase ingere dados através de adaptadores orientados a eventos (ex: `LISTEN/NOTIFY` do PostgreSQL, webhooks de CDC ou streams Kafka) e, de forma assíncrona, puxa e vetoriza os dados para a sua engine local em DuckDB sem onerar o seu banco transacional.
3. **Inferência na Borda (Edge):** Agentes IA consultam o cache local do JaguarBase em milissegundos.
4. **O Resultado:** Latência de rede zero para a IA, e impacto de CPU absolutamente zero nos seus bancos primários.

```mermaid
graph TD
    Source[(Sistema de Registro\nPostgreSQL, MySQL, Kafka...)] -- 1. Sincronização por Eventos --> JB{JaguarBase\nEdge Cache}
    JB -- 2. gRPC Plane\n(Sub-10ms) --> Backend[Microsserviços Backend]
    JB -- 3. MCP Plane\n(JSON-RPC 2.0) --> AI[Agentes IA\nClaude, Cursor, Qwen]
    style JB fill:#f9a03f,stroke:#333,stroke-width:2px,color:#000
    style Source fill:#336791,stroke:#fff,color:#fff
```

## 🚀 Principais Funcionalidades (O Appliance Edge AI)

- **Arquitetura 100% Baseada em Manifesto:** Configurado estritamente via `jaguardb.yaml`. Você define o schema, o JaguarBase se adapta dinamicamente. Sem *structs* fixos (hardcoded). Ele utiliza definições `virtual: true` para desacoplar as colunas transacionais brutas do schema vetorial enriquecido de RAG.
- **Armazenamento Vetorial In-Process:** Alimentado por DuckDB + VSS. Sem redes de banco de dados externas. Matemática vetorial SIMD em C++ extremamente rápida encapsulada em Go.
- **Governança Zero-Trust:** Mascaramento de PII nativo e Provas de Handoff HMAC. Prompts corrompidos ou maliciosos são sinalizados (`_is_tainted`) e isolados matematicamente das buscas da IA antes de atingirem a janela de contexto.
- **Protocolo de Plano Duplo (Dual-Plane Protocol):**
  - **High-Performance Plane (gRPC):** Para microsserviços backend que exigem latência inferior a 10ms.
  - **AI Agent Plane (MCP):** JSON-RPC 2.0 via HTTP/REST para integração direta com Agentes IA (Claude Desktop, Cursor, LLMs Customizados).

### O Pipeline Zero-Trust "Debaixo do Capô"
Todo registro que entra ou sai do JaguarBase passa por um pipeline de segurança rigoroso e *fail-closed* (se houver dúvida, bloqueia).

```mermaid
sequenceDiagram
    participant Source as Banco de Origem / Stream (ex: Postgres, Kafka)
    participant JB as Ingestão JaguarBase
    participant Gov as Governança Zero-Trust
    participant DDB as DuckDB (SIMD)
    participant Agent as Agente IA (MCP)

    Source->>JB: 1. Evento Emitido (CDC / Insert)
    JB->>Gov: 2. Processa Linha
    Gov-->>Gov: 3. Tokeniza PII (Redis) & Identifica Injeções
    Gov->>DDB: 4. Armazena Vetor (Schema Dinâmico)
    Agent->>JB: 5. Busca via MCP (tools/call)
    JB->>DDB: 6. Busca Semântica (WHERE _is_tainted=false)
    DDB-->>JB: 7. Retorna Resultados Seguros
    JB-->>Agent: 8. HMAC Handoff Proof + JSON Limpo
```

---

## 🛡️ Segurança e Hardening de Nível Enterprise

O JaguarBase passou por auditorias rigorosas de AppSec para atender aos padrões corporativos de nuvem:

- **Execução Distroless & Non-Root:** A imagem Docker roda sobre uma base ultra-mínima `gcr.io/distroless/base-nossl-debian12:nonroot` (com runtimes C++ explicitamente copiados para o DuckDB), eliminando por completo vetores de ataque do SO (sem shell, sem binários ociosos e sem pacotes OpenSSL desnecessários).
- **Zero Vulnerabilidades (Trivy & GoSec):** Escaneado continuamente. 0 vulnerabilidades de nível HIGH/CRITICAL tanto nas camadas do sistema operacional quanto nas dependências Go.
- **Tokenização Criptográfica de PII:** A anonimização de dados utiliza **HMAC-SHA256** irreversível atrelado dinamicamente aos nomes das colunas do schema (ex: `vault_cpf_...`, `vault_api_key_...`), imune a ataques de Rainbow Table e colisões.
- **Escudo de API Integrado:** Endpoints MCP suportam autenticação opcional via Bearer Token (`JAGUARBASE_API_KEY`), limite estrito de payload via `MaxBytesReader` (128 KB), limites de conexões CGO no DuckDB e timeouts HTTP para prevenir esgotamento de memória e DoS.

## 🛠️ Início Rápido (Quick Start)

Você pode rodar o JaguarBase inteiramente via Docker, injetando as configurações do seu YAML durante a inicialização. Certifique-se de que as permissões do diretório de dados montado correspondam ao usuário non-root do container (`uid:gid 65532:65532`), ou defina permissão apropriada:

```bash
# Prepara as permissões do diretório de dados para o container nonroot
mkdir -p data && chmod 777 data

# 1. Inicie o JaguarBase no Docker
docker run -d \
  --name jaguarbase-edge \
  --network="host" \
  --env-file .env \
  -v "$(pwd)/jaguardb.yaml:/app/jaguardb.yaml" \
  -v "$(pwd)/data:/app/data" \
  andrewbraga1/jaguarbase:latest

# 2. Acompanhe os logs de ingestão em tempo real
docker logs -f jaguarbase-edge
```

*Para desenvolvimento local (Exige CGO habilitado):*
```bash
sudo apt-get install build-essential gcc
CGO_ENABLED=1 go run cmd/jaguarbase/main.go
```

## 🧪 Suíte Abrangente de Testes

O JaguarBase possui uma taxa de aprovação de testes de 100% com o *Global Race Detector* do Go ativado. 

Para executar os testes unitários internos com detecção de condição de corrida (race detector):
```bash
go test -v -race ./internal/... -count=1
```

Para executar a suíte completa contra serviços reais (requer PostgreSQL, Redis, Ollama e uma instância ativa do JaguarBase rodando na porta `:50051` para os testes ponta a ponta):
```bash
JAGUARBASE_HMAC_SECRET="jaguarbase-super-secret-key-mude-em-prod" \
LLM_API_URL="http://localhost:11434/api/embeddings" \
TEST_DATABASE_URL="postgres://Seu_Usuario:Sua_Senha@localhost:5432/Seu_Banco?sslmode=disable" \
TEST_REDIS_URL="redis://localhost:6379" \
go test -v -race ./... -count=1
```
*Nota: A flag `?sslmode=disable` em `TEST_DATABASE_URL` é opcional, mas altamente recomendada ao testar contra containers PostgreSQL locais.*

## 📊 Benchmarks e Chaos Engineering

Testado em um Dual-Core Intel i7 com 1.000 *workers* concorrentes simultâneos processando vetores de 1536 dimensões. Veja o relatório completo em [BENCHMARKS.md](docs/BENCHMARKS.md).

| Cenário | Total de Requisições | Throughput (Req/Seg) | Taxa de Sucesso |
| :--- | :--- | :--- | :--- |
| **[gRPC] Health Ping** | 100.000 | **43.584,82 req/s** | 100% |
| **[gRPC] Busca Vetorial** | 10.000 | **1.050,87 req/s** | 100% |
| **[HTTP/MCP] JSON-RPC** | 10.000 | **831,80 req/s** | 100% |

**Resiliência de Hardware (Chaos Engineering):**
- **Load Shedding O(1):** Intercepta *Runaway Loops* (ex: loops infinitos do LangChain) e bloqueia as requisições (HTTP 429) na camada do Roteador Go em **< 0.5ms**.
- **Vazamento de Memória Zero (Zero Memory Leaks):** Consome **< 10MB de RAM na Heap do Go** enquanto gerencia centenas de milhares de vetores.
- **OS Page Caching:** Transfere inteligentemente o peso dos índices vetoriais para o *Buffer Cache* do Kernel do Linux, prevenindo travamentos por OOM (Out Of Memory) na aplicação.

## 💼 Posicionamento de Mercado

- **Precisa indexar 1 Bilhão de vetores na nuvem para busca global?** Use uma engine distribuída como Qdrant ou Milvus em conjunto com o [JaguarGate](https://github.com/andrewbraga1/jaguargate).
- **Precisa construir um App de IA Local robusto, uma ferramenta Desktop, ou proteger o seu Postgres Corporativo contra picos de CPU causados por IAs?** Use o **JaguarBase**.

## 🛣️ Roadmap (Próximos Passos)

- [x] **Fase 1: PoC e Domínio Central** - Adaptador Postgres, Cofre Redis, DuckDB SIMD, Validação de Handoff de Segurança.
- [x] **Fase 2: Configuração de Schema Dinâmico** - Parser do `jaguardb.yaml`, ingestão agnóstica de tipos e remoção de structs fixas.
- [x] **Fase 3: Busca Vetorial (RAG)** - Extensões DuckDB VSS, embeddings dinâmicos e Busca Semântica *fail-closed*.
- [x] **Fase 4: Edge Multi-Protocolo** - Camada Edge de alta performance (gRPC) e Servidor MCP (JSON-RPC 2.0).
- [x] **Fase 5: Chaos Engineering & Resiliência** - Load Shedding O(1), Cancelamento de Contexto e limites físicos (Disco/RAM).
- [x] **Fase 6: Containerização e CI/CD** - Multi-stage Dockerfile (Distroless Non-Root + CGO) e automação de deployment.
- [x] **Fase 7: Segurança Enterprise (AppSec)** - Criptografia Zero-Trust, API Shields e Auditorias com Zero CVEs.
