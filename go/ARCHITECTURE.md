# URP Architecture

## Structure (18.5k LOC)

```
cmd/urp/
├── main.go       # Entry + all commands (1400+ lines - split candidate)
├── backup.go     # Backup commands
├── opencode.go   # OpenCode integration
└── skills.go     # Skills commands

internal/
├── domain/       # Core types: Event, Conflict, Solution, Session
├── graph/        # Memgraph driver + cache
├── ingest/       # Code/git parsing → graph
├── query/        # Cypher queries
├── memory/       # Session memory, focus, knowledge
├── cognitive/    # Wisdom, novelty, learning
├── container/    # Docker/Podman management
├── protocol/     # Master↔Worker JSON Lines
├── orchestrator/ # Plan/Task persistence
├── planning/     # Plan creation/status
├── runner/       # Command execution + safety
├── render/       # Output formatting
├── runtime/      # System observer
├── vector/       # LanceDB + embeddings
├── logging/      # Container event logging + graph persistence
├── metrics/      # Prometheus endpoint
├── audit/        # Observability: metrics collection, anomaly detection, healing
├── skills/       # Skill loading/execution
├── selftest/     # Container health checks
└── opencode/     # LLM agent subsystem (14 subdirs, 33 files)
```

## Package Responsibilities

| Package | Responsibility |
|---------|---------------|
| `logging` | JSON event logging for containers (spawn, health, nemo) |
| `audit` | Full observability: metrics aggregation, anomaly detection, auto-healing |
| `metrics` | Prometheus-compatible HTTP endpoint |

Note: `logging` and `audit` serve different purposes. `logging` is for structured events (container lifecycle), `audit` is for operation tracking and system health.

## SOLID Analysis

### Good Patterns
- Interface segregation: `graph.Driver`, `logging.GraphWriter`
- Single responsibility: `vector/`, `cognitive/`, `planning/`
- Open/closed: Provider pattern in `opencode/provider/`

### Violations to Address
| Location | Issue | Priority |
|----------|-------|----------|
| `main.go` | 1400+ lines, all commands | Medium |
| `logging`→`metrics` | Direct import | Low |

### Event Type Variants (Intentional)
| Package | Purpose |
|---------|---------|
| `domain/event.go` | Terminal/VCS events |
| `opencode/domain/event.go` | LLM streaming events |
| `audit/event.go` | Operation audit trail |

These are not duplicates - different contexts, different fields.

## Future Refactoring

### Split main.go (When needed)
```
cmd/urp/
├── main.go       # Root + global
├── code.go       # urp code *
├── git.go        # urp git *
├── sys.go        # urp sys/infra *
└── ...
```

## Performance

| Operation | Time | Bottleneck |
|-----------|------|------------|
| git ingest | 102ms | Subprocess + parsing |
| infra status | 43ms | Container inspection |
| sys vitals | 25ms | Graph queries |

## Test Coverage

```
internal/audit       ✓
internal/cognitive   ✓
internal/container   ✓
internal/graph       ✓
internal/logging     ✓
internal/memory      ✓
internal/metrics     ✓
internal/planning    ✓
internal/protocol    ✓
internal/query       ✓
internal/runner      ✓
internal/selftest    ✓
internal/vector      ✓
```

## Build

```bash
cd go && go build -o urp ./cmd/urp && go test ./...
```
