# Agentic Development Guide

## Build Status (2025-12-07)
- ✅ `make build` - PASS
- ✅ `make test` - 43 packages, ALL PASS
- ✅ SOLID Score: 95%
- ✅ `urp doctor` - HEALTHY (requires Docker + Memgraph)

### Test Coverage Progress
| Package | Status | Notes |
|---------|--------|-------|
| internal/exec | ✅ NEW | Runner interface + mock tests |
| internal/render | ✅ NEW | Output formatting tests |
| internal/server | ✅ NEW | AgentService interface + tests |
| internal/runtime | ✅ NEW | Runtime interface + Observer tests (37 tests) |
| internal/tui | ✅ NEW | Commands, Brain, DebugPanel tests (32 tests) |
| opencode/session | ✅ NEW | Manager + MockStore tests (20 tests) |
| opencode/storage | ✅ NEW | SQLite storage full coverage (23 tests) |
| opencode/command | ✅ NEW | Parse, Registry, Commands + MockAgent (17 tests) |
| opencode/config | ✅ NEW | Load, Save, paths, env overrides (14 tests) |
| pkg/llm | ✅ NEW | Provider interface, Registry + MockProvider (13 tests) |

## Commands
- **Build**: `make build` (builds `urp` binary)
- **Test Unit**: `make test` (runs unit tests, no infra required)
- **Test Integration**: `make test-integration` (requires Docker + Memgraph)
- **Test E2E**: `make test-e2e` (creates real containers)
- **Preflight**: `make preflight` (check Docker + Memgraph ready)
- **Test Single**: `cd go && go test -v ./internal/<package> -run <TestName>`
- **Lint**: `make lint` (runs `golangci-lint`)
- **Format**: `make fmt` (runs `go fmt` and `goimports`)
- **Infra Up**: `make infra-up` (starts Memgraph + TEI)
- **Infra Down**: `make infra-down` (stops infrastructure)

## Estado Actual (Verificado 2025-12-07)

### Infraestructura (ACTUALIZADO)
- **Docker ONLY** - Podman eliminado completamente
- **Red unificada**: `urp-network` (siempre, sin variantes por proyecto)
- **Memgraph REQUERIDO** - Sin fallbacks, error explícito si no está disponible
- **Embeddings REQUIEREN configuración** - `URP_EMBEDDING_PROVIDER=tei` o `openai`
- docker-compose incluye `tei` (HuggingFace Text Embeddings Inference)
- Volúmenes: `urp-memgraph-data`, `urp-vectors`

### Cambios Recientes (2025-12-07)
- ❌ **Eliminado**: Soporte Podman (44 referencias removidas)
- ❌ **Eliminado**: LocalEmbedder fallback (hash-based)
- ❌ **Eliminado**: Graceful degradation sin Memgraph
- ✅ **Agregado**: Tests de integración con `//go:build integration`
- ✅ **Agregado**: `make preflight` para verificar prerequisitos
- ✅ **Agregado**: Mensajes de error explícitos con instrucciones

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
- Logger persiste eventos al grafo (REQUIERE Memgraph)
- SessionID se genera si falta
- Tests cubren backup/config/ingest/opencode/skills/vector

## Code Style & Conventions
- **Language**: Go 1.24+
- **Imports**: Grouped as: Standard Lib (`fmt`, `os`) -> 3rd Party (`github.com/...`) -> Internal (`github.com/joss/urp/...`)
- **Naming**: PascalCase for exported symbols, camelCase for internal. Clear, descriptive names.
- **Structure**: Entry points in `cmd/urp/`, logic in `internal/`. New logic goes to `internal/`.
- **Error Handling**: Explicit check (`if err != nil`). **NO FALLBACKS SILENCIOSOS**. Errores deben ser visibles.
- **CLI**: Use `cobra` framework. Define commands in `cmd/urp/`.
- **Testing**: Table-driven tests preferred. Use `assert` from `testify` if available.
- **Integration Tests**: Use `//go:build integration` tag. Run with `go test -tags=integration`
- **SOLID Patterns**:
  - Use maps instead of switches for OCP (see `domain/entity.go`, `cognitive/signals.go`)
  - Use functional options for DIP (see `agent/agent.go`, `ingest/ingester.go`)
  - Split interfaces for ISP (see `store/store.go` Reader/Writer separation)
- **Agent Rules**:
  - Read `CLAUDE.md` for high-level axioms.
  - Master/Worker architecture must be respected.
  - Do not introduce cycles in imports.
  - **Docker only** - No Podman references.

## Final Version Rules (NO FALLBACKS)

### Mocks Policy
- **Test Mocks (`*_test.go`)**: KEEP - Essential for unit testing without infra
- **Production Fallbacks**: REMOVE - All errors must be explicit

### Error Visibility Rules
| Pattern | Action | Example |
|---------|--------|---------|
| `return nil // silent` | **FIX**: Return error | `return ErrVectorStoreRequired` |
| `continue // ignore` | **FIX**: Return error | `return fmt.Errorf("...")` |
| `_ = err` | **FIX**: Handle or log | `log.Printf("WARN: %v", err)` |
| GPU/optional features | **KEEP**: Acceptable fallback | NeMo GPU detection |
| Provider switching | **KEEP**: Valid fallback | OpenAI if Anthropic fails |

### Fixed Silent Failures (2025-12-07)
- `tui/tui.go`: Memgraph now required, returns explicit error
- `tui/agent_run.go`: Memgraph now required, returns explicit error
- `tui/commands.go`: Logs warning on command file read failure
- `cognitive/novelty.go`: `IndexCode` returns `ErrVectorStoreRequired`
- `cognitive/wisdom.go`: `IndexError` returns `ErrVectorStoreRequired`
- `vector/store.go`: `load()` returns error on corruption
- `permission/permission.go`: Logs warnings instead of silent fail
- `specs/engine.go`: Logs neighborhood errors with count
- `audit/graph_persistence.go`: Logs warning when graph not configured
- `skills/loader.go`: Logs skill/category load errors with count
- `session/share.go`: `Export()` returns error on usage failure

### Mocks in Production Code
- **NONE** - All mocks are in `*_test.go` files only (verified)

## Prerequisitos

```bash
# 1. Docker instalado y corriendo
docker info

# 2. Infraestructura levantada
docker compose up -d memgraph tei

# 3. Variables de entorno
export URP_EMBEDDING_PROVIDER=tei
export TEI_URL=http://localhost:8080

# 4. Verificar
make preflight
```

## Completed (P0-P1)

### P0 - DONE
- [x] Store Interface Compliance: All stores have `Ping()/Close()`
- [x] Orchestrator DIP: `MasterProtocol` interface injection
- [x] Container Service: `internal/container/service.go` (CLI → Service → Manager)
- [x] **Docker Only**: Eliminado soporte Podman
- [x] **Red unificada**: `urp-network` siempre
- [x] **Sin fallbacks**: Errores explícitos sin Memgraph/Embeddings

### P1 - DONE
- [x] Multi-worker parallel: `SpawnWorker()` + `--parallel N`
- [x] CLI extraction: `cmd/urp/audit.go` thin CLI layer
- [x] Integration tests: `docker_integration_test.go`, `memgraph_integration_test.go`
- [x] **exec package tests**: Runner interface with OSRunner + MockRunner
- [x] **render package tests**: Output formatting (Events, Errors, Status)
- [x] **server package**: AgentService interface + ContextTracker (DIP)

### P2 - Backlog
- TUI: scroll suave, colores configurables, panel de estado en tiempo real
- Vector Store: auto-indexación, cache de embeddings, búsqueda híbrida
- Spec-Kit: templates, validación, wizard interactivo
- Observabilidad: Prometheus/Grafana, pruebas E2E/carga
- Distribución: Homebrew, APT/YUM, security scanning
- Provider Factory: Unify provider creation

## Archivos Críticos (NO ROMPER)
```
PROTECTED (alta dependencia):
- internal/graph/driver.go
- internal/graph/memgraph.go (ConnectWithRetry ahora retorna error)
- internal/store/store.go
- internal/opencode/domain/*
- internal/protocol/master.go

OCP COMPLIANT (extiende via maps):
- internal/domain/entity.go
- internal/opencode/cognitive/signals.go

DIP COMPLIANT (functional options):
- internal/opencode/agent/agent.go
- internal/ingest/ingester.go
- internal/orchestrator/orchestrator.go (MasterProtocol)
- internal/container/service.go (Service layer)
- internal/server/agent_service.go (AgentService interface)
- internal/runtime/observer.go (Runtime interface via WithRuntime)

ISP COMPLIANT (interface segregation):
- internal/exec/runner.go (Runner interface)
- internal/server/agent_service.go (AgentService, FocusService, ContextTracker)
- internal/runtime/runtime.go (Runtime interface)

INTEGRATION TESTS:
- internal/container/docker_integration_test.go
- internal/graph/memgraph_integration_test.go
```

## Errores Comunes y Soluciones

| Error | Causa | Solución |
|-------|-------|----------|
| `Docker not found` | Docker no instalado/corriendo | `systemctl start docker` |
| `urp-network not found` | Infra no levantada | `docker compose up -d memgraph` |
| `Memgraph connection failed` | Memgraph no corriendo | `docker compose up -d memgraph` |
| `URP_EMBEDDING_PROVIDER not set` | Config faltante | `export URP_EMBEDDING_PROVIDER=tei` |
| `TEI_URL not set` | TEI no configurado | `export TEI_URL=http://localhost:8080` |
