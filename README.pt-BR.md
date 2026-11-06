# JaguarBase 🐆

[🇺🇸 Read in English](README.md) | [🇧🇷 Leia em Português](README.pt-BR.md)

![Go Version](https://img.shields.io/badge/Go-1.26-00ADD8?style=flat&logo=go)
![DuckDB](https://img.shields.io/badge/DuckDB-Embedded-FFF000?style=flat)
![Status](https://img.shields.io/badge/Status-Beta_/_v1.0.0-orange)
![AppSec](https://img.shields.io/badge/AppSec-Zero_CVEs-brightgreen)
![Licença](https://img.shields.io/badge/Licen%C3%A7a-Apache_2.0-blue.svg)
![CI](https://github.com/andrewbraga1/jaguarbase/actions/workflows/ci.yml/badge.svg)

**Um Cache Vetorial de Borda e Alta Performance para PostgreSQL em Go & DuckDB (Acelerado por SIMD)**

JaguarBase é um cache vetorial na borda orientado a eventos, projetado do zero para Agentes Autônomos de IA e Edge AI. Ele atua como uma ponte desacoplada entre seu banco de dados transacional primário (PostgreSQL) e a janela de contexto da sua IA, descarregando as consultas vetoriais pesadas para um motor embarcado em DuckDB, enquanto aplica pseudonimização de PII em cofre e filtragem heurística de injeção de prompt.

---

## 🧠 A Tese Arquitetural: CQRS e Cache na Borda

Arquiteturas tradicionais forçam os Agentes de IA a executar cálculos vetoriais pesados diretamente no banco de dados relacional primário (ex: PostgreSQL com `pgvector`), gerando picos de uso de CPU e arriscando a estabilidade dos sistemas centrais de negócios.

**O JaguarBase inverte o paradigma usando CQRS (Segregação de Responsabilidade de Comando e Consulta):**
1. **Sistema de Registro:** Mantenha seus dados seguros no seu banco transacional primário.
2. **Sincronização por Eventos:** O JaguarBase recebe eventos de alteração (via PostgreSQL `LISTEN/NOTIFY`) e gera vetores assincronamente em uma instância local do DuckDB.
3. **Inferência na Borda (Edge):** Agentes de IA consultam o cache local do JaguarBase em milissegundos.
4. **Resultado:** Plano de consulta local sub-milissegundo e redução drástica da sobrecarga de CPU no seu banco de dados primário.

```mermaid
graph TD
    Source[(PostgreSQL\nSistema de Registro)] -- 1. Sincronização por Eventos --> JB{JaguarBase\nEdge Cache}
    JB -- 2. Plano gRPC\n(Sub-10ms) --> Backend[Microsserviços de Backend]
    JB -- 3. Plano MCP\n(JSON-RPC 2.0) --> AI[Agentes IA\nClaude, Cursor, LLMs Locais]
    style JB fill:#f9a03f,stroke:#333,stroke-width:2px,color:#000
    style Source fill:#336791,stroke:#fff,color:#fff
```

## 🚀 Principais Funcionalidades (O Appliance Edge AI)

- **Arquitetura 100% Dirigida por Manifesto:** Configurado estritamente via `jaguardb.yaml`. Você define o schema, o JaguarBase se adapta dinamicamente sem *structs* fixos em Go, desacoplando as colunas transacionais do schema vetorial.
- **Armazenamento Vetorial In-Process:** Movido por DuckDB C++ com distância de cosseno exata acelerada por SIMD (`array_cosine_distance`) dentro de um wrapper Go.
- **Pipeline de Governança Zero-Trust:**
  - **Pseudonimização com Cofre Redis:** Campos sensíveis são tokenizados via HMAC-SHA256 (`vault_{coluna}_{hash}`) e armazenados temporariamente no Redis com TTL de 24 horas, garantindo que o texto original não persista em repouso no DuckDB.
  - **Filtro Heurístico Anti-Injeção:** Registros contendo padrões de evasão conhecidos são sinalizados (`_is_tainted=true`) durante a ingestão e excluídos da busca semântica (`WHERE _is_tainted=false`).
  - **HMAC Handoff Proofs:** Respostas são assinadas com HMAC-SHA256 para detecção de adulteração downstream.
- **Protocolo de Plano Duplo:**
  - **Plano de Alta Performance (gRPC):** Plano interno de baixa latência para microsserviços.
  - **Plano de Agentes IA (MCP / JSON-RPC 2.0):** Endpoint HTTP para chamadas semânticas de ferramentas (`tools/list`, `tools/call`).

### O Pipeline Zero-Trust por Baixo do Capô
Todo registro que entra ou sai do JaguarBase passa por um pipeline defensivo e seguro por padrão (fail-closed):

```mermaid
sequenceDiagram
    participant Source as PostgreSQL (Sistema de Registro)
    participant JB as Ingestão JaguarBase
    participant Gov as Governança Zero-Trust
    participant DDB as DuckDB (SIMD)
    participant Agent as Agente IA (MCP / gRPC)

    Source->>JB: 1. Evento Emitido (LISTEN/NOTIFY)
    JB->>Gov: 2. Processar Linha
    Gov-->>Gov: 3. Pseudonimizar PII (Redis) & Varrer Injeções
    Gov->>DDB: 4. Armazenar Vetor (Schema Dinâmico)
    Agent->>JB: 5. Consulta via MCP (tools/call)
    JB->>DDB: 6. Busca Semântica (WHERE _is_tainted=false)
    DDB-->>JB: 7. Retornar Registros Seguros
    JB-->>Agent: 8. HMAC Handoff Proof + JSON Limpo
```

---

## 🛡️ Segurança de Nível Enterprise e Hardening

O JaguarBase passou por auditorias rigorosas de AppSec para garantir um build limpo e superfície de ataque mínima:

- **Execução Distroless e Non-Root:** A imagem Docker roda na base `gcr.io/distroless/base-nossl-debian12:nonroot` com bibliotecas de runtime C++ copiadas explicitamente (`libstdc++.so.6`, `libgcc_s.so.1`), eliminando shells, gerenciadores de pacotes e pacotes OpenSSL desnecessários.
- **Zero Vulnerabilidades (Trivy e GoSec):** 0 CVEs de nível ALTO/CRÍTICO nas camadas de SO e dependências Go; Clean Build (0 issues) no SAST do GoSec.
- **API Shield e Checagem de Limites:** Autenticação em tempo constante (`subtle.ConstantTimeCompare`), limites estritos de payload (`http.MaxBytesReader` 128 KB), pool de conexões DuckDB limitado e timeouts HTTP contra ataques Slowloris.

---

## ⚠️ Estágio Atual e Limitações Conhecidas (Beta v1.0.0)

Para manter total transparência e rigor de engenharia, a versão atual possui os seguintes limites de design:
* **Ingestão de Eventos:** Baseada em PostgreSQL `LISTEN/NOTIFY` (com sincronização de prontidão do watcher para eliminar condições de corrida na inicialização) e carga histórica resiliente. Conectores adicionais via Replicação Lógica / tabela Outbox estão no roadmap.
* **Atualizações de Cache:** Upserts totalmente idempotentes (`INSERT OR REPLACE`) e propagação de exclusões por tombstone implementados e validados. Checkpoints de backfill são isolados em tabela interna de metadados para evitar pulos na retomada.
* **Indexação Vetorial:** Utiliza cálculo exato por força bruta acelerado por SIMD no DuckDB (`array_cosine_similarity`). Suporte à extensão VSS para indexação vetorial ANN/HNSW em ambientes locais e embarcados.
* **Protocolo MCP:** Em estrita conformidade com a especificação do Model Context Protocol (versões 2024-11-05 e 2025-06-18), com handshake completo `initialize`, negociação de capabilities, ping, `tools/list` e Content Blocks tipados em `tools/call` para integração direta com Claude Desktop e clientes compatíveis com MCP.

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
- **Pegada de Memória Previsível:** A camada de serviço em Go mantém uma pegada de heap enxuta (**< 10MB de RAM Go Heap**), enquanto as alocações C++ fora da heap do DuckDB embarcado são geridas pelo pool de buffers do DuckDB e descarregadas para o cache de páginas do kernel Linux, prevenindo travamentos por OOM.
- **Cache de Páginas do SO:** Descarrega inteligentemente os índices vetoriais para o Buffer Cache do Kernel Linux, garantindo alto throughput em consultas locais sem inchaço de memória.

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
