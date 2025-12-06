# Agentic Development Guide

## Build Status (2025-12-06)
- ✅ `make build` - PASS
- ✅ `make test` - 30+ packages PASS
- ✅ SOLID Score: ~90%

## Commands
- **Build**: `make build` (builds `urp` binary)
- **Test All**: `make test` (runs all Go tests)
- **Test Single**: `cd go && go test -v ./internal/<package> -run <TestName>`
- **Lint**: `make lint` (runs `golangci-lint`)
- **Format**: `make fmt` (runs `go fmt` and `goimports`)
- **Eval Loop Smoke Test**: `./scripts/test_improvement_loop.sh` (needs Memgraph + TEI service; optional LLM key)
- **Embeddings Benchmark**: `./scripts/benchmark_embeddings.sh` (switches TEI_MODEL to compare latency)

## Estado Actual (Verificado 2025-12-06)

### Infraestructura
- docker-compose incluye `tei` (HuggingFace Text Embeddings Inference)
- `URP_EMBEDDING_PROVIDER=tei` y `TEI_URL` apuntan al servicio
- `vector.GetDefaultEmbedder` cae a TEI → local hash → OpenAI/Anthropic si hay API key
- Redes/volúmenes: `urp-network`, `memgraph-data`, `urp-vectors`

### Cognitive System
- `urp think context` usa híbrido Vector+Memgraph con spreading activation
- `urp think evaluate [--llm]` agrupa errores del audit log y sugiere fixes
- Señales cognitivas OCP-compliant via `SignalType.Meta()`

### Code Quality (SOLID)
- ✅ EntityType con `GraphLabel()`, `StatKey()` - OCP compliant
- ✅ SignalType con `Meta()` - OCP compliant
- ✅ Store base interface con generics - ISP compliant
- ✅ Agent/Ingester con functional options - DIP compliant
- ✅ TUI split en múltiples archivos - SRP compliant

### Git & Ingest
- `ingest.GitLoader` usa `GitParser` + UNWIND batches (500)
- `urp git link` genera relaciones `CO_CHANGED_WITH` por co-evolución

### Auditoría
- Logger persiste eventos al grafo si hay DB
- SessionID se genera si falta
- Tests cubren backup/config/ingest/opencode/skills/vector

## Code Style & Conventions
- **Language**: Go 1.24+
- **Imports**: Grouped as: Standard Lib (`fmt`, `os`) -> 3rd Party (`github.com/...`) -> Internal (`github.com/joss/urp/...`)
- **Naming**: PascalCase for exported symbols, camelCase for internal. Clear, descriptive names.
- **Structure**: Entry points in `cmd/urp/`, logic in `internal/`. New logic goes to `internal/`.
- **Error Handling**: Explicit check (`if err != nil`). Return errors in `internal/`, use `fatalError` only in `main`.
- **CLI**: Use `cobra` framework. Define commands in `cmd/urp/`.
- **Testing**: Table-driven tests preferred. Use `assert` from `testify` if available.
- **SOLID Patterns**:
  - Use maps instead of switches for OCP (see `domain/entity.go`, `cognitive/signals.go`)
  - Use functional options for DIP (see `agent/agent.go`, `ingest/ingester.go`)
  - Split interfaces for ISP (see `store/store.go` Reader/Writer separation)
- **Agent Rules**: 
  - Read `CLAUDE.md` for high-level axioms.
  - Master/Worker architecture must be respected.
  - Do not introduce cycles in imports.

## Pendientes Clave (P0-P1)

### P0 - Inmediato (4h)
- [ ] Store Interface Compliance: 5 stores necesitan `Ping()/Close()` (2h)
- [ ] Orchestrator DIP: Inyectar `protocol.Master` via interface (2h)

### P1 - Esta semana (8h)
- [ ] Multi-worker paralelo: Completar `SpawnWorker()` + `--parallel N`
- [ ] CLI extraction: Separar business logic de `cmd/urp/audit.go`

### P2 - Backlog
- TUI: scroll suave, colores configurables, panel de estado en tiempo real
- Vector Store: auto-indexación, cache de embeddings, búsqueda híbrida
- Spec-Kit: templates, validación, wizard interactivo
- Observabilidad: Prometheus/Grafana, pruebas E2E/carga
- Distribución: Homebrew, APT/YUM, security scanning

## Archivos Críticos (NO ROMPER)
```
PROTECTED (alta dependencia):
- internal/graph/driver.go
- internal/store/store.go
- internal/opencode/domain/*

OCP COMPLIANT (extiende via maps):
- internal/domain/entity.go
- internal/opencode/cognitive/signals.go

DIP COMPLIANT (functional options):
- internal/opencode/agent/agent.go
- internal/ingest/ingester.go
```
