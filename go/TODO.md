# URP Go Module TODO

## Build Status (2025-12-06)
- ✅ `go build ./cmd/urp` - PASS
- ✅ `go test ./...` - 446 tests, 34 packages PASS
- ✅ SOLID Score: 93%
- ✅ `urp doctor` - HEALTHY

---

## Completed (P0 - Reliability)

- [x] P0.1: Panic recovery middleware (`internal/logging/recovery.go`)
- [x] P0.2: Graceful shutdown handler (`internal/runtime/shutdown.go`)
- [x] P0.3: Log rotation para alerts (`internal/alerts/alerts.go`)
- [x] P0.4: EntityType OCP methods (`internal/domain/entity.go`)
- [x] P0.5: SignalType OCP methods (`internal/opencode/cognitive/signals.go`)
- [x] P0.6: Store base interface (`internal/store/store.go`)
- [x] P0.7: Agent functional options (`internal/opencode/agent/agent.go`)
- [x] P0.8: Ingester functional options (`internal/ingest/ingester.go`)

## Completed (P0 - SOLID Compliance)

- [x] P0.9: Store Interface Compliance - 5 stores with `Ping()/Close()`
- [x] P0.10: Orchestrator DIP - `MasterProtocol` interface + injection
- [x] P1.1: Multi-worker parallel spawn - `SpawnWorker()` + pool
- [x] P1.2: CLI extraction - `cmd/urp/audit.go` → thin CLI
- [x] P1.3: Container Service layer - `internal/container/service.go` (DIP)

---

## Pending - Orchestration System

### P2: `urp launch` - Master Container
- [x] Create master container with project:ro mount
- [x] Auto-ingest: `urp code ingest && urp git ingest`
- [x] Open Claude CLI (interactive)
- [ ] SELinux :z labels for volumes (optional)

### P2: `urp spawn` - Ephemeral Workers
- [x] Spawn worker containers (docker --rm)
- [x] Workers have project:rw access
- [x] Workers can install tools, modify code
- [x] `--parallel N` flag for pool
- [ ] Worker types: browser, test (future)

### P3: Graph Schema Plan/Task/Result
- [ ] `:Plan` node with status, project
- [ ] `:Task` node with description, worker_id
- [ ] `:TaskResult` with success, changes, summary
- [ ] Relationships: HAS_TASK, EXECUTED_BY, PRODUCED, DEPENDS_ON

### P4: Envelope Wrapper
- [x] JSON-lines protocol over stdin/stdout
- [x] Message types: instruction, status, result, error
- [ ] Intercept Claude CLI I/O
- [ ] Log envelopes to graph

### P5: X11 Browser Worker
- [ ] X11 forwarding setup
- [ ] Firefox/Chrome in container
- [ ] Visual testing support

---

## Completed - CLI Extraction

- [x] `audit.go` - thin CLI, logic in render/service
- [x] `container.go` (615 LOC) → `internal/container/service.go` (284 LOC)
- [x] `orchestrate.go` - uses orchestrator package

---

## Implemented Features

### Commands (35+)
- `urp` - Interactive agent session
- `urp doctor` - Environment health check
- `urp infra start|stop|clean|logs|status`
- `urp launch|spawn|workers|attach|exec|kill|ask`
- `urp code ingest|deps|impact|dead|cycles|hotspots|stats`
- `urp git ingest|history|link`
- `urp think wisdom|novelty|learn|context|evaluate`
- `urp mem add|recall|list|stats|clear`
- `urp kb store|query|list|reject|promote|stats`
- `urp focus <target> [-d depth]`
- `urp sys vitals|topology|health|runtime`
- `urp events run|list|errors`
- `urp vec stats|search|add`
- `urp alert send|list|resolve|active`
- `urp oc session list|new|show`
- `urp spec init|list|run|status`
- `urp skill list|show|run|categories|search`
- `urp backup export|import|list|stats`
- `urp audit status|recent|stats`
- `urp tui` - Bubble Tea interface

### Core Systems
- Alerts system with Claude hooks injection
- Vector store with TEI/local embeddings
- Cognitive engine (signals, reflex, hygiene)
- Agent with functional options (DIP)
- TUI split into multiple files (SRP)
- OCP-compliant EntityType and SignalType

---

**Last updated:** 2025-12-06
**Verified:** Build ✅ | Tests ✅
