# URP-CLI Migration to Go

## AUDIT REPORT

### Current State Analysis

```
VERDICT: MIGRATE - Python has accumulated 12K+ LOC with god classes
DATA:   Graph (Memgraph) + Vectors (ChromaDB) + Files (JSON session state)
REMOVE: runner.py god class (1,449 LOC), duplicate token trackers (4 files)
RISK:   Shell hooks dependency on Python paths
ACTION: Clean SOLID Go architecture → Single binary → Embedded shell hooks
```

---

## 1. ARCHITECTURAL SINS (Current Python)

### A. God Classes

| File | LOC | Responsibilities (Should be 1) | Actual |
|------|-----|-------------------------------|--------|
| `runner.py` | 1,449 | 1 | **7**: Command execution, Graph logging, Working memory, Cognitive skills, Token tracking, Session management, Memory management |
| `context_manager.py` | 1,014 | 1 | **4**: Mode switching, Metrics collection, Noise detection, Recommendations |
| `knowledge_store.py` | 670 | 1 | **3**: ChromaDB ops, Memgraph ops, 3-level search |

### B. Naming Chaos

Current command surface (50+ commands) with inconsistent patterns:

```bash
# Pattern 1: urp-* (cli.py)
urp-ingest, urp-git, urp-impact, urp-deps, urp-history, urp-hotspots, urp-dead, urp-stats

# Pattern 2: bare verbs (runner.py)
wisdom, novelty, focus, learn, surgical, unfocus, clear-context

# Pattern 3: noun-* (aliases)
mem-status, mem-context, tokens-status, tokens-budget

# Pattern 4: k* (knowledge)
kstore, kquery, klist, kreject, kexport

# Pattern 5: cc-* (context)
cc-status, cc-mode, cc-compact, cc-clean

# Pattern 6: local-* / global-* (scope)
local-wisdom, global-wisdom, local-pain, global-pain
```

### C. Dependency Spaghetti

```
runner.py
  ├→ database.py (lazy)
  ├→ immune_system.py (lazy)
  ├→ brain_cortex.py (lazy)
  ├→ brain_render.py (lazy)
  ├→ token_tracker.py (lazy)
  ├→ llm_tools.py (which imports...)
  │   ├→ session_memory.py
  │   ├→ knowledge_store.py (which imports brain_cortex)
  │   └→ context.py
  └→ (circular risk with knowledge_store ↔ brain_cortex)
```

---

## 2. GO SOLID ARCHITECTURE

### Package Structure

```
urp/
├── cmd/                          # Entrypoints
│   └── urp/
│       └── main.go               # Single binary, all commands
│
├── internal/                     # Private packages
│   ├── domain/                   # Domain entities (D primitive)
│   │   ├── entity.go             # File, Function, Class, Container
│   │   ├── event.go              # TerminalEvent, Conflict, Solution
│   │   └── memory.go             # Memory, Knowledge
│   │
│   ├── graph/                    # Graph database layer
│   │   ├── driver.go             # Neo4j/Memgraph driver (Interface)
│   │   ├── memgraph.go           # Memgraph implementation
│   │   └── queries.go            # Cypher query builders
│   │
│   ├── vector/                   # Vector embeddings layer
│   │   ├── store.go              # VectorStore interface
│   │   ├── chroma.go             # ChromaDB implementation
│   │   └── embed.go              # Embedding model wrapper
│   │
│   ├── runner/                   # Command execution (Single Responsibility)
│   │   ├── executor.go           # Execute commands
│   │   ├── logger.go             # Log to graph
│   │   └── safety.go             # Immune system filter
│   │
│   ├── memory/                   # Working memory (Single Responsibility)
│   │   ├── working.go            # Focus/unfocus/clear
│   │   ├── session.go            # Session-private memory
│   │   ├── knowledge.go          # Shared knowledge store
│   │   └── eviction.go           # LRU + importance eviction
│   │
│   ├── cognitive/                # Cognitive skills (Single Responsibility)
│   │   ├── wisdom.go             # Similar error lookup
│   │   ├── novelty.go            # Pattern break detection
│   │   ├── learning.go           # Solution consolidation
│   │   └── focus.go              # Context loading
│   │
│   ├── query/                    # Graph queries (Single Responsibility)
│   │   ├── impact.go             # Impact analysis (Φ inverse)
│   │   ├── deps.go               # Dependency analysis (Φ forward)
│   │   ├── history.go            # File history (τ)
│   │   ├── hotspots.go           # Risk analysis (τ + Φ)
│   │   └── dead.go               # Dead code (⊥)
│   │
│   ├── ingest/                   # Code ingestion (Single Responsibility)
│   │   ├── parser.go             # Parser interface
│   │   ├── python.go             # Python AST
│   │   ├── golang.go             # Go AST
│   │   ├── ingester.go           # Code → Graph
│   │   └── git.go                # Git → Graph
│   │
│   ├── observe/                  # Runtime observation
│   │   ├── container.go          # Docker/Podman vitals
│   │   └── process.go            # Process monitoring
│   │
│   └── render/                   # Output formatting
│       ├── trace.go              # Causal trace format
│       ├── code.go               # Code fragment format
│       └── topology.go           # Network topology
│
├── pkg/                          # Public APIs (for extensions)
│   └── urp/
│       └── client.go             # URP client for other Go programs
│
└── scripts/
    └── hooks.sh                  # Minimal shell hooks (calls urp binary)
```

### SOLID Principles Applied

#### S - Single Responsibility

Each package has ONE job:

| Package | Responsibility |
|---------|----------------|
| `runner` | Execute commands, log to graph |
| `memory` | Manage working memory, sessions, knowledge |
| `cognitive` | Wisdom, novelty, learning, focus |
| `query` | Graph queries for code analysis |
| `ingest` | Parse code, load git, build graph |
| `observe` | Monitor containers and processes |
| `render` | Format output for humans/LLMs |

#### O - Open/Closed

Interfaces for extension:

```go
// graph/driver.go
type GraphDriver interface {
    Execute(query string, params map[string]any) ([]Record, error)
    Close() error
}

// vector/store.go
type VectorStore interface {
    Add(id string, text string, metadata map[string]any) error
    Query(text string, n int) ([]Match, error)
    Delete(id string) error
}

// ingest/parser.go
type Parser interface {
    Extensions() []string
    Parse(path string, content []byte) ([]Entity, error)
}
```

#### L - Liskov Substitution

All implementations are interchangeable:

```go
// Can swap Memgraph for Neo4j without changing code
var db graph.GraphDriver = memgraph.New(uri)
// or
var db graph.GraphDriver = neo4j.New(uri)

// Can swap ChromaDB for Qdrant
var vs vector.VectorStore = chroma.New(uri)
// or
var vs vector.VectorStore = qdrant.New(uri)
```

#### I - Interface Segregation

Small, focused interfaces:

```go
// Only what's needed for wisdom queries
type WisdomStore interface {
    FindSimilarErrors(embedding []float32, threshold float64) ([]Conflict, error)
}

// Only what's needed for focus
type ContextLoader interface {
    LoadSubgraph(target string, depth int) (*Subgraph, error)
}
```

#### D - Dependency Inversion

High-level modules don't depend on low-level:

```go
// cognitive/wisdom.go depends on interface, not implementation
type WisdomService struct {
    store  WisdomStore      // interface
    embed  Embedder         // interface
}

// Injected at startup
func NewWisdomService(store WisdomStore, embed Embedder) *WisdomService {
    return &WisdomService{store: store, embed: embed}
}
```

---

## 3. UNIFIED COMMAND INTERFACE

### Design Principles

1. **One command, one concept**: `urp <noun> <verb> [args]`
2. **Predictable flags**: `--depth`, `--limit`, `--project`, `--json`
3. **Consistent output**: JSON by default, human-friendly with `--pretty`
4. **Scope-aware**: `--local` (project) vs `--global` (all projects)

### New Command Tree

```bash
urp                           # Show status and help
urp version                   # Show version

# ─────────────────────────────────────────────────────────────────────────
# CODE ANALYSIS (replaces urp-* prefix)
# ─────────────────────────────────────────────────────────────────────────
urp code ingest <path>        # Parse code into graph
urp code deps <sig>           # Dependencies of function
urp code impact <sig>         # Impact of changing function
urp code dead                 # Find unused code
urp code cycles               # Find circular dependencies (was: circular)
urp code hotspots             # High churn = high risk
urp code contents <file>      # Entities in file

# ─────────────────────────────────────────────────────────────────────────
# GIT HISTORY (replaces urp-git, urp-history)
# ─────────────────────────────────────────────────────────────────────────
urp git ingest <path>         # Load git history
urp git history <file>        # File change timeline
urp git expert <pattern>      # Who knows this code
urp git reviewers <file>      # Suggested reviewers
urp git recent                # Recently changed files

# ─────────────────────────────────────────────────────────────────────────
# RUNTIME OBSERVATION (replaces vitals, topology, health)
# ─────────────────────────────────────────────────────────────────────────
urp sys vitals                # Container CPU/RAM
urp sys topology              # Network map
urp sys health                # Health issues
urp sys logs <container>      # Container logs

# ─────────────────────────────────────────────────────────────────────────
# TERMINAL EVENTS (replaces recent, pain)
# ─────────────────────────────────────────────────────────────────────────
urp events list               # Recent commands (was: recent)
urp events errors             # Recent errors (was: pain)
urp events run <cmd>          # Execute and log (was: runner.py run)

# ─────────────────────────────────────────────────────────────────────────
# COGNITIVE SKILLS (replaces wisdom, novelty, focus, learn)
# ─────────────────────────────────────────────────────────────────────────
urp think wisdom <error>      # Find similar past errors
urp think novelty <code>      # Check pattern novelty
urp think learn <desc>        # Consolidate success
urp think surgical <target>   # Read only specific function

# ─────────────────────────────────────────────────────────────────────────
# WORKING MEMORY (replaces focus, unfocus, clear-context, mem-*)
# ─────────────────────────────────────────────────────────────────────────
urp focus add <target>        # Add to working memory
urp focus remove <target>     # Remove from working memory
urp focus clear               # Clear all (tabula rasa)
urp focus list                # Show focused items
urp focus context             # Show accumulated context

# ─────────────────────────────────────────────────────────────────────────
# SESSION MEMORY (replaces remember, recall, memories)
# ─────────────────────────────────────────────────────────────────────────
urp mem add <text>            # Remember note
urp mem find <query>          # Search memories (was: recall)
urp mem list                  # List all memories
urp mem export <id>           # Promote to knowledge

# ─────────────────────────────────────────────────────────────────────────
# SHARED KNOWLEDGE (replaces kstore, kquery, klist, kreject, kexport)
# ─────────────────────────────────────────────────────────────────────────
urp kb add <text>             # Store knowledge
urp kb find <query>           # Query knowledge
urp kb list                   # List all knowledge
urp kb reject <id>            # Reject as not applicable

# ─────────────────────────────────────────────────────────────────────────
# CONTEXT OPTIMIZATION (replaces cc-*)
# ─────────────────────────────────────────────────────────────────────────
urp ctx status                # Current context status
urp ctx mode [mode]           # Get/set optimization mode
urp ctx compact               # Run optimization
urp ctx clean                 # Remove noise
urp ctx quality <1-5>         # Record quality feedback

# ─────────────────────────────────────────────────────────────────────────
# TOKEN TRACKING (replaces tokens, api-tokens, pricing)
# ─────────────────────────────────────────────────────────────────────────
urp tokens status             # Current usage
urp tokens budget <n>         # Set budget
urp tokens reset              # Reset hour counter
urp tokens models             # List model prices

# ─────────────────────────────────────────────────────────────────────────
# SESSION / IDENTITY
# ─────────────────────────────────────────────────────────────────────────
urp session new [name]        # Start new session
urp session id                # Show current identity
urp session stats             # Memory/knowledge stats

# ─────────────────────────────────────────────────────────────────────────
# INFRASTRUCTURE (replaces urp-infra, urp-on, urp-off)
# ─────────────────────────────────────────────────────────────────────────
urp infra status              # Show all containers
urp infra start               # Start infrastructure
urp infra stop                # Stop infrastructure
urp infra clean               # Remove containers
urp infra hooks enable        # Enable shell hooks
urp infra hooks disable       # Disable shell hooks
```

### Command Mapping (Old → New)

| Old Command | New Command |
|-------------|-------------|
| `urp-ingest` | `urp code ingest` |
| `urp-git` | `urp git ingest` |
| `urp-impact` | `urp code impact` |
| `urp-deps` | `urp code deps` |
| `urp-history` | `urp git history` |
| `urp-hotspots` | `urp code hotspots` |
| `urp-dead` | `urp code dead` |
| `urp-stats` | `urp session stats` |
| `vitals` | `urp sys vitals` |
| `topology` | `urp sys topology` |
| `health` | `urp sys health` |
| `recent` | `urp events list` |
| `pain` | `urp events errors` |
| `wisdom` | `urp think wisdom` |
| `novelty` | `urp think novelty` |
| `focus` | `urp focus add` |
| `unfocus` | `urp focus remove` |
| `learn` | `urp think learn` |
| `surgical` | `urp think surgical` |
| `clear-context` | `urp focus clear` |
| `mem-status` | `urp focus list` |
| `remember` | `urp mem add` |
| `recall` | `urp mem find` |
| `memories` | `urp mem list` |
| `kstore` | `urp kb add` |
| `kquery` | `urp kb find` |
| `klist` | `urp kb list` |
| `kreject` | `urp kb reject` |
| `kexport` | `urp mem export` |
| `cc-status` | `urp ctx status` |
| `cc-mode` | `urp ctx mode` |
| `cc-compact` | `urp ctx compact` |
| `tokens` | `urp tokens status` |
| `identity` | `urp session id` |
| `memstats` | `urp session stats` |
| `urp-init` | `urp code ingest . && urp git ingest .` |
| `urp-status` | `urp` |
| `urp-on` | `urp infra hooks enable` |
| `urp-off` | `urp infra hooks disable` |
| `urp-topology` | `urp sys topology` |

---

## 4. MIGRATION ROADMAP

### Phase 1: Core Infrastructure (Week 1-2)

```go
// Priority: Get the binary working with basic commands

cmd/urp/main.go               // Cobra CLI setup
internal/graph/driver.go      // Interface
internal/graph/memgraph.go    // Memgraph implementation
internal/domain/entity.go     // Core types
```

Commands implemented:
- `urp` (status)
- `urp version`
- `urp session id`

### Phase 2: Terminal Events (Week 2-3)

```go
internal/runner/executor.go   // Command execution
internal/runner/logger.go     // Graph logging
internal/runner/safety.go     // Immune system
```

Commands implemented:
- `urp events run <cmd>`
- `urp events list`
- `urp events errors`

### Phase 3: Code Analysis (Week 3-4)

```go
internal/ingest/parser.go     // Parser interface
internal/ingest/python.go     // Python AST (use tree-sitter)
internal/ingest/golang.go     // Go AST (native)
internal/ingest/ingester.go   // Code → Graph
internal/ingest/git.go        // Git → Graph
internal/query/*.go           // All query commands
```

Commands implemented:
- `urp code ingest`
- `urp code deps/impact/dead/cycles/hotspots/contents`
- `urp git ingest/history/expert/reviewers/recent`

### Phase 4: Cognitive Skills (Week 4-5)

```go
internal/vector/store.go      // Vector store interface
internal/vector/chroma.go     // ChromaDB client
internal/vector/embed.go      // Embedding model
internal/cognitive/*.go       // Wisdom/novelty/learning/focus
```

Commands implemented:
- `urp think wisdom/novelty/learn/surgical`

### Phase 5: Memory System (Week 5-6)

```go
internal/memory/working.go    // Working memory
internal/memory/session.go    // Session memory
internal/memory/knowledge.go  // Shared knowledge
internal/memory/eviction.go   // LRU + importance
```

Commands implemented:
- `urp focus add/remove/clear/list/context`
- `urp mem add/find/list/export`
- `urp kb add/find/list/reject`

### Phase 6: Context & Tokens (Week 6-7)

```go
internal/context/optimizer.go // Context optimization
internal/tokens/tracker.go    // Token tracking
internal/tokens/budget.go     // Budget management
```

Commands implemented:
- `urp ctx status/mode/compact/clean/quality`
- `urp tokens status/budget/reset/models`

### Phase 7: Runtime & Polish (Week 7-8)

```go
internal/observe/container.go // Docker/Podman
internal/render/*.go          // Output formatting
scripts/hooks.sh              // Minimal shell hooks
```

Commands implemented:
- `urp sys vitals/topology/health/logs`
- `urp infra status/start/stop/clean/hooks`

### Phase 8: Testing & Migration (Week 8-9)

1. Integration tests for all commands
2. Benchmark vs Python (should be 10-100x faster)
3. Migration script for existing data
4. Documentation update
5. Deprecation warnings for old Python commands

---

## 5. SHELL HOOKS (Simplified)

The new shell hooks are minimal - just call the Go binary:

```bash
#!/bin/bash
# hooks.sh - Source this in .bashrc

export URP_ENABLED="${URP_ENABLED:-1}"

_urp_wrap() {
    local cmd="$1"
    shift
    if [[ "$URP_ENABLED" != "1" ]]; then
        command "$cmd" "$@"
        return $?
    fi
    urp events run "$cmd" "$@"
    return $?
}

# Wrap common commands
git() { _urp_wrap git "$@"; }
docker() { _urp_wrap docker "$@"; }
npm() { _urp_wrap npm "$@"; }
pip() { _urp_wrap pip "$@"; }
cargo() { _urp_wrap cargo "$@"; }
go() { _urp_wrap go "$@"; }
make() { _urp_wrap make "$@"; }
pytest() { _urp_wrap pytest "$@"; }

# Enable/disable
alias urp-on='export URP_ENABLED=1'
alias urp-off='export URP_ENABLED=0'
```

---

## 6. BENEFITS OF GO MIGRATION

| Aspect | Python (Current) | Go (Target) |
|--------|------------------|-------------|
| **Binary size** | ~50MB (with deps) | ~15MB (static) |
| **Startup time** | 200-500ms | <10ms |
| **Memory** | 50-100MB | 10-20MB |
| **Concurrency** | GIL limitations | Native goroutines |
| **Distribution** | pip install + deps | Single binary |
| **Shell hooks** | python3 /app/runner.py | urp (native) |
| **Type safety** | Runtime errors | Compile-time |
| **LOC** | 12,000+ | ~5,000 (estimated) |

---

## 7. DEPENDENCIES (Go)

```go
// go.mod
module github.com/urp-cli/urp

require (
    github.com/spf13/cobra v1.8.0           // CLI framework
    github.com/neo4j/neo4j-go-driver/v5     // Memgraph/Neo4j
    github.com/chroma-core/chroma-go        // ChromaDB client
    github.com/smacker/go-tree-sitter       // Tree-sitter for parsing
    github.com/fatih/color                  // Terminal colors
    github.com/olekukonko/tablewriter       // Table output
)
```

---

## 8. BACKWARD COMPATIBILITY

During migration, provide aliases for old commands:

```bash
# In hooks.sh - deprecated aliases
alias urp-ingest='echo "DEPRECATED: Use urp code ingest" && urp code ingest'
alias wisdom='echo "DEPRECATED: Use urp think wisdom" && urp think wisdom'
alias pain='echo "DEPRECATED: Use urp events errors" && urp events errors'
# ... etc
```

After 3 months, remove deprecated aliases.

---

## SUMMARY

```
CURRENT:  12K+ LOC Python, 50+ inconsistent commands, god classes
TARGET:   5K LOC Go, ~40 hierarchical commands, SOLID architecture
BENEFIT:  10x faster, single binary, type-safe, maintainable
TIMELINE: 8-9 weeks for full migration
```
