# JaguarBase 🐆

[🇺🇸 Read in English](README.md) | [🇧🇷 Leia em Português](README.pt-BR.md)

![Go Version](https://img.shields.io/badge/Go-1.26-00ADD8?style=flat&logo=go)
![DuckDB](https://img.shields.io/badge/DuckDB-Embedded-FFF000?style=flat)
![Status](https://img.shields.io/badge/Status-Production_Ready-success)
![AppSec](https://img.shields.io/badge/AppSec-Zero_CVEs-brightgreen)
![Licença](https://img.shields.io/badge/Licen%C3%A7a-Apache_2.0-blue.svg)
![CI](https://github.com/andrewbraga1/jaguarbase/actions/workflows/ci.yml/badge.svg)

**Um Motor Vetorial Agnóstico de Zero-Trust e Alta Performance em Go (Aceleração SIMD via DuckDB C++)**

JaguarBase é um *appliance* de infraestrutura altamente desacoplado e orientado a eventos, projetado do zero para Agentes Autônomos de IA e Edge AI. Ele atua como uma ponte segura e de alta performance entre seus bancos de dados transacionais (ex: PostgreSQL, MySQL, streams do Kafka) e a janela de contexto da sua IA, garantindo que PII sensível seja deterministicamente anonimizada **em repouso** (Data at Rest) e que dados envenenados sejam isolados da busca semântica, garantindo uma janela de contexto permanentemente limpa para o seu LLM.

---

## 🧠 A Tese Arquitetural: CQRS e Cache na Borda

Arquiteturas tradicionais forçam os Agentes de IA a executar cálculos vetoriais pesados diretamente no banco de dados relacional primário (ex: PostgreSQL com `pgvector`), gerando picos de uso de CPU e arriscando a estabilidade dos sistemas centrais de negócios.

**O JaguarBase inverte o paradigma usando CQRS (Segregação de Responsabilidade de Comando e Consulta):**
1. **Sistema de Registro:** Mantenha seus dados a salvo em seu banco transacional primário.
2. **Sincronização por Eventos:** O JaguarBase ingere dados via adaptadores orientados a eventos (ex: `LISTEN/NOTIFY` do PostgreSQL, webhooks CDC, ou streams do Kafka) e vetoriza os registros assincronamente para seu motor local DuckDB, sem sobrecarregar seu banco transacional.
3. **Inferência na Borda (Edge):** Agentes de IA consultam o cache local do JaguarBase em milissegundos.
4. **Resultado:** Latência de rede zero para a IA, e zero impacto na CPU dos seus bancos de dados primários.

```mermaid
graph TD
    Source[(Sistema de Registro\nPostgreSQL, MySQL, Kafka)] -- 1. Sincronização por Eventos --> JB{JaguarBase\nEdge Cache}
    JB -- 2. Plano gRPC\n(Sub-10ms) --> Backend[Microsserviços de Backend]
    JB -- 3. Plano MCP\n(JSON-RPC 2.0) --> AI[Agentes IA\nClaude, Cursor, Qwen]
    style JB fill:#f9a03f,stroke:#333,stroke-width:2px,color:#000
    style Source fill:#336791,stroke:#fff,color:#fff
```

## 🚀 Principais Funcionalidades (O Appliance Edge AI)

- **Arquitetura 100% Dirigida por Manifesto:** Configurado estritamente via `jaguardb.yaml`. Você define o schema, o JaguarBase se adapta dinamicamente. Sem `structs` hardcoded. Usa definições `virtual: true` para desacoplar as colunas transacionais brutas do schema vetorial enriquecido (RAG).
- **Armazenamento Vetorial In-Process:** Movido por DuckDB + VSS. Sem redes de banco de dados externas. Matemática vetorial SIMD em C++ extremamente rápida dentro de um wrapper Go.
- **Governança Zero-Trust:** Anonimização (masking) de PII embutida e HMAC Handoff Proofs. Prompts corrompidos ou maliciosos são sinalizados (`_is_tainted`) durante a ingestão e excluídos em nível de motor (`WHERE _is_tainted=false`), protegendo a janela de contexto da IA.
- **Protocolo de Plano Duplo:**
  - **Plano de Alta Performance (gRPC):** Para microsserviços de backend que exigem latência sub-10ms.
  - **Plano de Agentes IA (MCP):** JSON-RPC 2.0 via HTTP/REST para integração direta com Agentes de IA (Claude Desktop, Cursor, LLMs Customizados).

### O Pipeline Zero-Trust por Baixo do Capô
Todo registro que entra ou sai do JaguarBase passa por um pipeline de segurança rigoroso e seguro por padrão (fail-closed).

```mermaid
sequenceDiagram
    participant Source as Fonte (Postgres, Kafka)
    participant JB as Ingestão JaguarBase
    participant Gov as Governança Zero-Trust
    participant DDB as DuckDB (SIMD)
    participant Agent as Agente IA (MCP)

    Source->>JB: 1. Evento Emitido (CDC / Insert)
    JB->>Gov: 2. Processar Linha
    Gov-->>Gov: 3. Anonimizar PII (Redis) & Identificar Injeções
    Gov->>DDB: 4. Armazenar Vetor (Schema Dinâmico)
    Agent->>JB: 5. Consulta via MCP (tools/call)
    JB->>DDB: 6. Busca Semântica (WHERE _is_tainted=false)
    DDB-->>JB: 7. Retornar Resultados Seguros
    JB-->>Agent: 8. HMAC Handoff Proof + JSON Limpo
```

---

## 🛡️ Segurança de Nível Enterprise e Hardening

O JaguarBase passou por auditorias rigorosas de AppSec para garantir os padrões de nuvem Enterprise:

- **Execução Distroless e Non-Root:** A imagem Docker roda em uma base ultra-minimalista `gcr.io/distroless/base-nossl-debian12:nonroot` (com runtimes C++ do DuckDB copiados explicitamente), eliminando bibliotecas de SO ociosas, shells e pacotes SSL desnecessários.
- **Zero Vulnerabilidades (Trivy e GoSec):** Verificação contínua. 0 vulnerabilidades ALTA/CRÍTICA nas camadas de SO e nas dependências Go.
- **Anonimização Criptográfica de PII:** O processo utiliza **HMAC-SHA256** irreversível vinculado dinamicamente aos nomes das colunas (ex: `vault_cpf_...`, `vault_api_key_...`), imune a ataques de Rainbow Table e colisão.
- **API Shield Embutido:** Endpoints MCP suportam autenticação Bearer Token opcional (`JAGUARBASE_API_KEY`), limites estritos de payload `MaxBytesReader` (128 KB), pool de conexões DuckDB CGO limitados e timeouts HTTP para prevenir exaustão de memória e DoS.

## 🛠️ Início Rápido

Você pode rodar o JaguarBase inteiramente via Docker, injetando sua configuração YAML em runtime. Certifique-se de que as permissões do diretório host montado correspondam ao usuário non-root do container (`uid:gid 65532:65532`), ou especifique `--user`:

```bash
# Preparar permissões do diretório de dados para container nonroot
mkdir -p data && chmod 777 data

# 1. Iniciar o JaguarBase no Docker
docker run -d \
  --name jaguarbase-edge \
  --network="host" \
  --env-file .env \
  -v "$(pwd)/jaguardb.yaml:/app/jaguardb.yaml" \
  -v "$(pwd)/data:/app/data" \
  andrewbraga1/jaguarbase:latest

# 2. Visualizar os logs de ingestão em tempo real
docker logs -f jaguarbase-edge
```

*Para desenvolvimento local (Requer CGO):*
```bash
sudo apt-get install build-essential gcc
CGO_ENABLED=1 go run cmd/jaguarbase/main.go
```

## 🧪 Suíte de Testes Abrangente

O JaguarBase possui uma taxa de aprovação de 100% em testes unitários e de integração com o Go Global Race Detector ativado.

Para rodar os testes unitários internos com detector de corrida (race detector):
```bash
go test -v -race ./internal/... -count=1
```

Para rodar a suíte completa contra serviços reais (requer PostgreSQL, Redis, Ollama e uma instância ativa do JaguarBase na porta `:50051` para os testes End-to-End):
```bash
JAGUARBASE_HMAC_SECRET="jaguarbase-super-secret-key-mude-em-prod" \
LLM_API_URL="http://localhost:11434/api/embeddings" \
TEST_DATABASE_URL="postgres://Seu_Usuario:Sua_Senha@localhost:5432/Seu_Banco?sslmode=disable" \
TEST_REDIS_URL="redis://localhost:6379" \
go test -v -race ./... -count=1
```
*Nota: A flag `?sslmode=disable` na `TEST_DATABASE_URL` é opcional, mas altamente recomendada ao testar contra containers locais do PostgreSQL.*

## 📊 Benchmarks e Engenharia do Caos

Testado em um Intel i7 Dual-Core com 1.000 workers simultâneos processando vetores de 1536 dimensões. Veja o relatório completo em [BENCHMARKS.md](docs/BENCHMARKS.md).

| Cenário | Requisições Totais | Taxa de Transf. (Req/Seg) | Sucesso |
| :--- | :--- | :--- | :--- |
| **[gRPC] Health Ping** | 100.000 | **43.584,82 req/s** | 100% |
| **[gRPC] Busca Vetorial** | 10.000 | **1.050,87 req/s** | 100% |
| **[HTTP/MCP] JSON-RPC** | 10.000 | **831,80 req/s** | 100% |

**Resiliência de Hardware (Engenharia do Caos):**
- **Load Shedding O(1):** Intercepta loops fora de controle (ex: loops infinitos do LangChain) e bloqueia as requisições (HTTP 429) na camada do Roteador Go em **< 0.5ms**.
- **Vazamentos de Memória Zero:** Consome **< 10MB de RAM (Go Heap)** enquanto gerencia centenas de milhares de vetores.
- **Cache de Páginas do SO:** Descarrega inteligentemente os índices vetoriais para o Buffer Cache do Kernel Linux, evitando travamentos por OOM (Out-of-Memory) no nível da aplicação.

## 💼 Posicionamento de Mercado

- **Precisa indexar 1 Bilhão de vetores na nuvem para busca global?** Use um motor distribuído como Qdrant ou Milvus em conjunto com o [JaguarGate](https://github.com/andrewbraga1/jaguargate).
- **Precisa construir um Aplicativo Local de IA robusto, uma Ferramenta Desktop, ou proteger seu Postgres Enterprise contra picos de CPU dos Agentes?** Use o **JaguarBase**.

## 🛣️ Roadmap (Próximos Passos)

- [x] **Fase 1: PoC e Domínio Central** Adaptador Postgres, Cofre Redis, SIMD DuckDB, Validação de Handoff Seguro.
- [x] **Fase 2: Configuração de Schema Dinâmico** Parser do `jaguardb.yaml`, ingestão agnóstica de tipos e remoção de structs fixas.
- [x] **Fase 3: Busca Vetorial (RAG)** Extensões VSS do DuckDB, embeddings dinâmicos e Similaridade Semântica (fail-closed).
- [x] **Fase 4: Borda Multi-Protocolo** Camada Edge de alta performance (gRPC) e Servidor MCP (JSON-RPC 2.0).
- [x] **Fase 5: Engenharia do Caos e Resiliência** Load Shedding O(1), Cancelamento de Contexto e limites em nível de hardware.
- [x] **Fase 6: Containerização e CI/CD** Dockerfile Multi-stage (Distroless Non-Root + CGO) e automação de deploy.
- [x] **Fase 7: Segurança Enterprise (AppSec)** Criptografia Zero-Trust, API Shields e Auditorias com Zero CVEs.
