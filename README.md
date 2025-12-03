# URP-CLI: Universal Repository Perception

Semantic knowledge graph + cognitive layer for AI coding agents.

**Core insight:** Store in graphs, render for LLMs. Raw JSON wastes tokens.

## Quick Start

```bash
# Build the Go binary
cd go && go build -o urp ./cmd/urp

# Run URP (connects to Memgraph if available)
./urp                     # Show status
./urp version             # Show version

# Ingest your codebase
./urp code ingest .       # Parse code into graph
./urp git ingest .        # Load git history

# Query what happened
./urp events errors       # Recent errors (pain)
./urp events list         # Recent commands
./urp sys vitals          # Container health
```

## Architecture

```
┌─────────────────────────────────────────────────────────────────┐
│                         URP CLI (Go)                            │
│                                                                 │
│  cmd/urp/main.go ────────────────────────────────────────────┐ │
│       │                                                       │ │
│       ├── internal/domain/      Entity + Event types         │ │
│       ├── internal/graph/       Memgraph driver              │ │
│       ├── internal/runner/      Command execution + safety   │ │
│       ├── internal/ingest/      Parser + Git loader          │ │
│       ├── internal/query/       Graph queries                │ │
│       ├── internal/cognitive/   Wisdom + Novelty + Learning  │ │
│       ├── internal/memory/      Session + Knowledge + Focus  │ │
│       └── internal/runtime/     Container observation        │ │
│                                                               │ │
└───────────────────────────────────────────────────────────────┘ │
                              │                                   │
                              ▼                                   │
                    ┌─────────────────┐                          │
                    │    Memgraph     │  ← Graph Database        │
                    └─────────────────┘                          │
```

## The 7 PRU Primitives

```
D  (Domain)      → Entity existence: File, Function, Class
τ  (Vector)      → Temporal sequence: Commits, commands
Φ  (Morphism)    → Causal flow: Calls, data flow, CPU/RAM
⊆  (Inclusion)   → Hierarchy: File→Function, Class→Method
⊥  (Orthogonal)  → Conflicts: Errors, dead code
T  (Tensor)      → Context: Branch, environment
P  (Projective)  → Viewpoint (future)
```

## Command Reference

### Code Analysis (D, Φ, ⊆)

```bash
urp code ingest <path>     # Parse code into graph
urp code deps <sig>        # Dependencies of function
urp code impact <sig>      # Impact of changing function
urp code dead              # Find unused code
urp code cycles            # Find circular dependencies
urp code hotspots          # High churn = high risk
urp code stats             # Graph statistics
```

### Git History (τ)

```bash
urp git ingest <path>      # Load git history
urp git history <file>     # File change timeline
```

### Cognitive Skills

```bash
urp think wisdom <error>   # Find similar past errors
urp think novelty <code>   # Check if pattern is unusual
urp think learn <desc>     # Consolidate success into knowledge
```

### Session Memory

```bash
urp mem add <text>         # Remember a note
urp mem recall <query>     # Search memories
urp mem list               # List all memories
urp mem stats              # Memory statistics
urp mem clear              # Clear session memory
```

### Knowledge Base

```bash
urp kb store <text>        # Store knowledge
urp kb query <text>        # Search knowledge
urp kb list                # List all knowledge
urp kb reject <id> <reason># Mark as not applicable
urp kb promote <id>        # Promote to global scope
urp kb stats               # Knowledge statistics
```

### Focus (Context Loading)

```bash
urp focus <target>         # Load focused context
urp focus <target> -d 2    # With depth expansion
```

### Runtime Observation (Φ)

```bash
urp sys vitals             # Container CPU/RAM metrics
urp sys topology           # Network map
urp sys health             # Container health issues
urp sys runtime            # Detected container runtime
```

### Terminal Events (τ + Φ)

```bash
urp events run <cmd>       # Execute and log command
urp events list            # Recent commands
urp events errors          # Recent errors
```

## Packages (Go SOLID Architecture)

| Package | Responsibility |
|---------|---------------|
| `domain/` | Entity types (File, Function, Event) |
| `graph/` | Database interface + Memgraph driver |
| `runner/` | Command execution + safety filter |
| `ingest/` | Code parser + Git loader |
| `query/` | Graph queries (deps, impact, dead) |
| `cognitive/` | Wisdom, novelty, learning |
| `memory/` | Session memory + knowledge store |
| `runtime/` | Container observation |
| `render/` | Output formatting |

## Safety System (Immune System)

Deterministic pre-execution filter (not AI guessing).

| Blocked | Alternative |
|---------|-------------|
| `rm -rf /` | Be specific about paths |
| `git push --force` | Use `--force-with-lease` |
| `git add .env` | Add to `.gitignore` |
| `DROP DATABASE` | Requires user approval |

## Performance

| Metric | Python (legacy) | Go |
|--------|-----------------|-----|
| **Startup** | 616ms | 6ms |
| **10x commands** | 6.9s | 0.04s |
| **Binary** | ~50MB | 12MB |
| **LOC** | 12,000+ | 5,229 |

## Environment Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `NEO4J_URI` | `bolt://localhost:7687` | Graph database URI |
| `URP_PROJECT` | auto | Project name |
| `URP_SESSION_ID` | auto | Session identifier |

## Docker Compose (Optional)

```bash
# Start Memgraph
docker-compose up -d memgraph

# Or use podman
podman run -d -p 7687:7687 memgraph/memgraph
```

## Building

```bash
cd go
go build -o urp ./cmd/urp
go test ./...
```

## License

MIT
