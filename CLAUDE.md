# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

---

# URP: Embodied Agent Protocol

```
AXIOM: context_window ⊂ memory_total
       memory_total = context ∪ graph_db ∪ vector_store
```

## Development Commands

### Build & Test

```bash
# Build URP CLI
cd go && go build -o urp ./cmd/urp

# Run all tests
go test ./...

# Run specific test package
go test ./internal/logging/...

# Run single test
go test -run TestRecoveryHandler_WrapPanic ./internal/logging/

# Run with verbose output
go test -v ./...

# Run with coverage
go test -cover ./...

# Generate coverage report
go test -coverprofile=coverage.out ./...
go tool cover -html=coverage.out
```

### Linting & Code Quality

```bash
# Format code
cd go && go fmt ./...

# Vet (static analysis)
go vet ./...

# Run tests in verbose mode with coverage
go test -v -cover ./internal/<package>/
```

### Infrastructure

```bash
# Start infrastructure services
docker compose up -d memgraph

# Check service health
docker compose ps
docker logs urp-memgraph

# Stop infrastructure
docker compose down
```

### Development Workflow

```bash
# Quick pre-commit check
go vet ./... && go test ./... && go build -o urp ./cmd/urp

# Clean build artifacts
go clean
rm -f urp

# Verify environment
urp doctor
```

## Primitives (PRU)

```
D  Domain     := {File, Function, Class, Container}
τ  Temporal   := sequence(Commit, Event, Command)
Φ  Morphism   := flow(Calls, Data, Energy, ExitCode)
⊆  Inclusion  := hierarchy(File→Func, Class→Method, Net→Container)
⊥  Orthogonal := conflict(DeadCode, Cycles, Errors, Failures)
P  Projective := view(Interface, Implementation)  # future
T  Tensor     := context(Branch, Env, Session)
```

## Protocol

### on_project(any_task):
```python
# MANDATORY: Always use master/worker architecture
urp doctor                           # pre-flight check
if not healthy:
    urp_repair()                     # trigger self-repair

urp launch <project_path>            # start master container
# Master: read-only, orchestrates
# Workers: spawned for write operations

for task in plan.tasks:
    urp spawn                        # ephemeral worker
    worker.execute(task)
    if worker.failed:
        urp_repair(worker.error)     # self-repair on failure
```

### urp_repair(error):
```python
# Self-healing system: launches Claude Code with URP repo access
docker run -it --rm \
    -v ~/.urp-go:/urp:rw \
    -v <urp-cli-repo>:/workspace:rw \
    urp:master claude \
    --prompt "Fix URP error: {error}. Run tests after fix."

# Repair worker has full write access to URP codebase
# Must: diagnose, fix, test, commit
# Then: retry original operation
```

### on_error(red_output):
```python
PAUSE()                              # ¬guess
result = urp think wisdom "<error>"  # query similar
if result.similarity > 0.8:
    apply(result.solution)           # reuse
else:  # PIONEER
    analyze(urp events errors)
    if is_urp_bug(error):
        urp_repair(error)            # self-repair URP itself
    else:
        solve()
```

### on_task(complex):
```python
urp focus <target> -d 2    # load context (D,⊆)
urp sys vitals             # check state (Φ)
urp git history <file>     # temporal (τ)
```

### on_new_code(pattern):
```python
n = urp think novelty "<code>"
match n.level:
    case "safe"     : proceed()           # <30%
    case "moderate" : explain(); proceed() # 30-70%
    case "high"     : STOP(); justify()    # >70%
```

### on_success(user_confirms):
```python
urp think learn "<description>"  # crystallize Solution node
# future wisdom queries will find YOUR solution
```

### on_commit(task_complete):
```python
# MANDATORY: after every commit or completed task
urp think learn "<what_solved>: <key_insight>"
# Examples:
# "Batch queries: UNWIND pattern for N+1 elimination"
# "Memgraph: 'directory' is reserved, use 'workdir'"
# "Cobra: check len(args) before accessing args[0]"
```

## Immune System (⊥)

```python
BLOCK = {
    r"rm -rf /":        "use specific path",
    r"git push --force": "use --force-with-lease",
    r"git add \.env":   "use .gitignore",
    r"DROP DATABASE":   "requires approval",
    r"mkfs":            "filesystem destruction",
}
# on IMMUNE_BLOCK: read reason, use alternative, ¬retry
```

## Commands

```bash
# CODE (D,Φ,⊆)
urp code ingest|deps|impact|dead|cycles|hotspots|stats

# GIT (τ)
urp git ingest|history

# COGNITIVE
urp think wisdom|novelty|learn

# MEMORY (session-scoped)
urp mem add|recall|list|stats|clear

# KNOWLEDGE (persistent, multi-scope)
urp kb store|query|list|reject|promote|stats

# SKILLS (categorized capabilities)
urp skill list|show|run|load|search|add|delete|stats|categories
# Categories: dev, security, content, data, growth, business, core

# OPENCODE (session management)
urp oc session list|new|show|fork|delete
urp oc msg list|add
urp oc usage session|total

# FOCUS
urp focus <target> [-d depth]

# RUNTIME (Φ)
urp sys vitals|topology|health|runtime

# EVENTS (τ+Φ)
urp events run|list|errors

# VECTOR
urp vec stats|search|add

# AUDIT
urp audit status|recent|stats

# BACKUP (knowledge persistence)
urp backup export [-o file] [-t types] [-d desc]
urp backup import <file> [--merge] [-t types]
urp backup list <file>
urp backup stats
# Types: solutions, memories, knowledge, skills, sessions, vectors, all
```

## Memory Architecture

```
┌─────────────────────────────┐
│ SESSION (private, fast)     │  urp mem *
├─────────────────────────────┤
│ KNOWLEDGE (persistent)      │  urp kb *
│  scope ∈ {session,instance, │
│           global}           │
└─────────────────────────────┘

signature := project|branch|env|os
compatible(a,b) := a.project == b.project
```

## Context Profiles (token optimization)

```python
BUG_FIX:   focus(func, depth=1)    # ~100 tokens
REFACTOR:  focus(class, depth=2)   # topology only
FEATURE:   focus(similar) + wisdom # pattern copy
DEBUG:     urp events errors       # causal trace
```

## Graph Schema

```cypher
// Nodes
(:File), (:Function), (:Class), (:Commit), (:Author)
(:Container), (:TerminalEvent), (:Session), (:Solution)
(:Memory), (:Knowledge), (:Conflict)

// Edges
-[:CONTAINS]->  -[:CALLS]->  -[:FLOWS_TO]->
-[:PARENT_OF]-> -[:AUTHORED]-> -[:TOUCHED]->
-[:EXECUTED]->  -[:RESOLVES]-> -[:REJECTED]->
```

## Architecture: Code Organization

### Package Structure

```
go/
├── cmd/urp/                 # CLI entry point (Cobra commands)
│   ├── main.go              # App bootstrap, root command, db connection
│   ├── router.go            # LLM model selection and routing
│   ├── compile.go           # Context Compiler (V2 dynamic prompt generation)
│   ├── cognitive.go         # Wisdom, novelty, learning commands
│   ├── code.go              # Code analysis (ingest, deps, stats, hotspots)
│   ├── git.go               # Git history analysis
│   ├── memory.go            # Session memory management
│   ├── opencode.go          # AI agent execution
│   ├── system.go            # Infrastructure commands (infra, doctor, status)
│   ├── container.go         # Container management (spawn, kill, workers)
│   ├── orchestrate.go       # Master/Worker orchestration
│   ├── audit.go             # Audit logging
│   ├── spec.go              # Spec-driven execution
│   └── ... (other commands)
│
├── internal/
│   ├── alerts/              # Alert & event monitoring
│   │   └── alerting.go      # Alert lifecycle management
│   │
│   ├── audit/               # Audit logging & events
│   │   ├── logger.go        # Structured audit logs
│   │   └── store.go         # Persistent audit store
│   │
│   ├── backup/              # Knowledge base persistence
│   │   └── backup.go        # Export/import functionality
│   │
│   ├── bootstrap/           # Dependency injection & initialization
│   │   └── bootstrap.go     # Wire dependencies (SRP)
│   │
│   ├── cognitive/           # Cognitive skills (Wisdom, Learn, Novelty)
│   │   ├── wisdom.go        # Find similar solutions (vector search)
│   │   ├── novelty.go       # Detect unusual patterns
│   │   ├── learn.go         # Store learned strategies
│   │   ├── evaluator.go     # Quality evaluation
│   │   └── validation_test.go # Validation tests
│   │
│   ├── compiler/            # Context Compiler (V2 feature)
│   │   └── compiler.go      # Dynamic prompt generation from graph state
│   │
│   ├── config/              # Configuration management
│   │   └── models.go        # Config structs
│   │
│   ├── container/           # Container orchestration (Docker/Podman)
│   │   ├── manager.go       # Docker/Podman abstraction
│   │   ├── health.go        # Container health checks
│   │   └── volume.go        # Volume management
│   │
│   ├── domain/              # Domain types & interfaces
│   │   ├── entity.go        # Core domain entities
│   │   └── event.go         # Event types
│   │
│   ├── exec/                # Command execution
│   │   └── executor.go      # Process execution & management
│   │
│   ├── gate/                # Noise Filter LLM
│   │   ├── client.go        # Gate client for filtering/routing
│   │   └── gate.go          # Fast model for pattern classification
│   │
│   ├── graph/               # Graph database (Memgraph)
│   │   ├── driver.go        # Memgraph connection interface
│   │   ├── memgraph.go      # Concrete Memgraph driver
│   │   ├── cache.go         # Graph query caching
│   │   ├── record.go        # Result record handling
│   │   └── *_test.go        # Tests (86+ test files)
│   │
│   ├── harness/             # Testing harness
│   │   └── test.go          # Test infrastructure
│   │
│   ├── ingest/              # Code parsing & ingestion
│   │   ├── parser.go        # AST parsing for code analysis
│   │   └── parser_test.go   # Parser tests
│   │
│   ├── logging/             # Utilities & recovery
│   │   ├── recovery.go      # Panic recovery & error handling
│   │   └── recovery_test.go # Recovery test cases
│   │
│   ├── memory/              # Multi-tier memory system
│   │   ├── session.go       # Ephemeral session memory
│   │   ├── knowledge.go     # Persistent knowledge base
│   │   ├── context.go       # Context window management
│   │   ├── cache.go         # Memory caching
│   │   └── *_test.go        # Multi-tier memory tests
│   │
│   ├── metrics/             # Metrics collection & reporting
│   │   └── metrics.go       # Performance metrics
│   │
│   ├── opencode/            # AI agent system (Claude integration)
│   │   ├── agent/           # Agent executor & learning loop
│   │   │   ├── agent.go
│   │   │   ├── autocorrect.go
│   │   │   ├── message_store.go
│   │   │   ├── prompt.go
│   │   │   └── *_test.go
│   │   ├── provider/        # LLM providers (multi-model support)
│   │   │   ├── anthropic.go # Claude API
│   │   │   ├── openai.go    # OpenAI-compatible APIs
│   │   │   ├── deepseek.go  # DeepSeek models
│   │   │   ├── google.go    # Google Gemini (minimal)
│   │   │   └── factory.go   # Provider routing & selection
│   │   ├── tool/            # Agent tools (file, code, bash ops)
│   │   │   ├── bash.go
│   │   │   ├── read.go
│   │   │   ├── write.go
│   │   │   ├── multiedit.go
│   │   │   ├── codesearch.go
│   │   │   ├── file_grep.go
│   │   │   ├── batch.go
│   │   │   ├── todo.go
│   │   │   └── ...
│   │   ├── model/           # Domain models for agents
│   │   │   └── registry_test.go
│   │   └── session/         # OpenCode session management
│   │
│   ├── orchestrator/        # Master/Worker orchestration
│   │   └── orchestrator.go  # Distributed agent coordination
│   │
│   ├── planning/            # Task planning & decomposition
│   │   └── planner.go       # Plan generation from goals
│   │
│   ├── protocol/            # Protocol definitions (JSON-lines envelope)
│   │   └── protocol.go      # Message protocol handling
│   │
│   ├── query/               # Query utilities
│   │   └── query.go         # Query building & optimization
│   │
│   ├── render/              # Output rendering & formatting
│   │   └── render.go        # Terminal output formatting
│   │
│   ├── runner/              # Task execution runner
│   │   └── runner.go        # Execute tasks with lifecycle
│   │
│   ├── selftest/            # Self-testing & validation
│   │   └── selftest.go      # System self-checks
│   │
│   ├── server/              # Background server mode
│   │   └── server.go        # HTTP/gRPC server
│   │
│   ├── session/             # Session management
│   │   └── session.go       # Session tracking & lifecycle
│   │
│   ├── skills/              # Extensible skills system
│   │   ├── skills.go        # Skill loader & manager
│   │   └── categories.go    # Skill categories
│   │
│   ├── specs/               # Spec-driven execution
│   │   └── spec.go          # Specification parsing & execution
│   │
│   ├── store/               # Data persistence layer
│   │   └── store.go         # Generic store interface
│   │
│   ├── strings/             # String utilities
│   │   └── strings.go       # Text processing helpers
│   │
│   ├── tui/                 # Terminal UI (Bubble Tea)
│   │   ├── agent.go         # Agent TUI
│   │   ├── agent_stream.go  # Streaming output handler
│   │   └── tui.go           # Interactive UI
│   │
│   ├── vector/              # Vector store for embeddings
│   │   ├── store.go         # Vector store interface
│   │   ├── embedder.go      # Embedding generation
│   │   └── lancedb.go       # LanceDB concrete implementation
│   │
│   └── runtime/             # Runtime utilities
│       └── runtime.go       # Runtime environment checks
│
└── docs/                    # Documentation
    ├── ARCHITECTURE.md      # Detailed architecture
    ├── COMMANDS.md          # Command reference
    ├── QUICKSTART.md        # Getting started
    └── progress.md          # Session learnings
```

### Key Architectural Patterns

**V2 Context Compiler:**
- `compiler.go` dynamically generates prompts from graph state
- Adaptive context modes: Full, Focused, Minimal, Delta, Memory
- Automatic token budget management

**Dual-LLM Pipeline:**
- Gate LLM: Fast noise filtering (Qwen/GLM)
- Master LLM: Reasoning and planning (Claude/DeepSeek)
- Provider routing in `opencode/provider/factory.go`

**Master/Worker Architecture:**
- Master: Read-only, orchestrates from `internal/orchestrator/`
- Workers: Ephemeral containers with write access
- Communication: `urp ask` sends instructions to worker's Claude CLI

**Learning Loop (Empiricist):**
- Pre-task: `cognitive/wisdom.go` retrieves similar strategies
- Post-task: Extract and store learnings in vector store
- Used by agents for adaptive behavior

**Graph-Based Perception (PRU):**
- 7 primitives: D (Domain), τ (Temporal), Φ (Morphism), ⊆ (Inclusion), ⊥ (Orthogonal), P (Projective), T (Tensor)
- Cypher queries for code structure, git history, solutions
- Stored in Memgraph (`internal/graph/`)

**Multi-Tier Memory:**
- Context Window: Current conversation (~200k tokens)
- Session Memory: Ephemeral (`internal/memory/session.go`)
- Knowledge Base: Persistent (`internal/memory/knowledge.go`)
- Graph DB: Code structure & history
- Vector Store: Semantic similarity

### Common Development Patterns

**Adding New Commands:**
1. Create file `go/cmd/urp/<feature>.go` with command definition
2. Add to root command in `main.go` via `rootCmd.AddCommand()`
3. Leverage existing services: `db` (Memgraph), `auditLogger`, `store`

**Adding Provider Support:**
1. Implement `opencode/provider.Provider` interface in `opencode/provider/<name>.go`
2. Register in `opencode/provider/factory.go`
3. Environment variable: `URP_<NAME>_API_KEY` and `URP_<NAME>_BASE_URL`

**Extending Agent Tools:**
1. Implement `opencode/tool.Tool` interface
2. Register in agent's tool registry (`opencode/agent/executor.go`)
3. Tools have automatic error recovery and containerization for dangerous operations

**Queries on Graph:**
- Use Cypher queries via `graph.Driver.Query()`
- Common patterns in `internal/graph/memgraph.go`
- Cache frequently-used queries via `graph.Cache`

**Memory Operations:**
- Session: `memory.SessionStore.Add/Recall`
- Knowledge: `memory.KnowledgeBase.Store/Query`
- Both scoped by project/branch/environment

## Build

```bash
cd go && go build -o urp ./cmd/urp && go test ./...
```

## Environment

### Check

```bash
urp doctor       # quick status
urp doctor -v    # full diagnostics
```

### Requirements

- Docker (preferred) or Podman
- urp-network created
- urp-memgraph running
- Images: urp:latest, urp:master, urp:worker

### Variables

```
NEO4J_URI=bolt://localhost:7687
URP_PROJECT=auto
URP_SESSION_ID=auto
```

---

## Orchestration Architecture (Master/Worker)

### Core Principle

```
Master NEVER writes to workspace.
Master sends instructions to Worker's Claude CLI.
Worker executes, reports back to Master.
```

### Flow

```
urp launch [path]
    │
    ├─► Create MASTER container (project:ro)
    ├─► Auto-ingest: urp code ingest && urp git ingest
    ├─► Open Claude CLI (user interacts here)
    │
    └─► User ↔ Claude (master)
            │
            ├─► Master analyzes, plans
            ├─► Master spawns WORKER via `urp spawn`
            │       └─► Worker: daemon mode, project:rw
            │       └─► Worker has its own Claude CLI
            │
            ├─► Master sends instructions via `urp ask`:
            │       urp ask urp-proj-w1 "create branch feature-x"
            │       urp ask urp-proj-w1 "run tests"
            │       urp ask urp-proj-w1 "fix failing tests"
            │       urp ask urp-proj-w1 "commit and push"
            │
            └─► When done: urp kill urp-proj-w1
```

### Communication Pattern

```
┌─────────────────┐                    ┌─────────────────┐
│  MASTER         │                    │  WORKER         │
│  (read-only)    │                    │  (read-write)   │
│                 │                    │                 │
│  Claude CLI ◄───┼── user input       │  Claude CLI     │
│       │         │                    │       ▲         │
│       ▼         │                    │       │         │
│  urp ask ───────┼─► docker exec ─────┼─► claude --print│
│                 │                    │       │         │
│  (reads stdout) │◄────── output ─────┼───────┘         │
└─────────────────┘                    └─────────────────┘
```

### Master Can Optimize Worker

Before sending complex instructions, master can:

```bash
# Write custom CLAUDE.md for worker's task
urp exec urp-proj-w1 "cat > /workspace/.claude/CLAUDE.md << 'EOF'
# Task: Fix authentication bug
Focus: auth.go, middleware.go
Tests: go test ./auth/...
EOF"

# Install tools worker needs
urp exec urp-proj-w1 "apk add --no-cache postgresql-client"
urp exec urp-proj-w1 "pip install pytest-cov"

# Then send the instruction
urp ask urp-proj-w1 "Fix the OAuth token refresh bug. Run tests when done."
```

### Container Topology

```
┌────────────────────────────────────────────────────────────┐
│  HOST                                                      │
│  ~/.urp-go/.env   # ANTHROPIC_API_KEY                      │
└──────────────┬─────────────────────────────────────────────┘
               │
    ┌──────────┴──────────┐
    │                     │
    ▼                     ▼
┌─────────────┐    ┌─────────────────────────────────┐
│ urp-memgraph│    │ urp-master-<project>            │
│ (graph db)  │◄───│  - /workspace:ro                │
│ bolt:7687   │    │  - docker socket (spawn workers)│
└─────────────┘    │  - Claude CLI (user session)    │
                   └──────────────┬──────────────────┘
                                  │ urp spawn
                   ┌──────────────┼──────────────┐
                   ▼              ▼              ▼
            ┌───────────┐  ┌───────────┐  ┌───────────┐
            │ worker-1  │  │ worker-2  │  │ worker-n  │
            │ daemon    │  │ daemon    │  │ daemon    │
            │ :rw       │  │ :rw       │  │ :rw       │
            │ Claude CLI│  │ Claude CLI│  │ Claude CLI│
            └───────────┘  └───────────┘  └───────────┘
```

### Commands

```bash
# START SESSION
urp launch [path]        # Master + ingest + Claude CLI

# WORKER MANAGEMENT
urp spawn [num]          # Create worker (daemon mode)
urp workers              # List active workers
urp kill <name>          # Stop worker

# COMMUNICATION (master → worker)
urp ask <worker> "prompt"    # Send to worker's Claude CLI
urp exec <worker> "cmd"      # Run shell command in worker

# PLANNING
urp plan create "description"  # Create plan in graph
urp plan show                  # View current plan
urp plan add "task"            # Add task to plan
```

### Example Session

```bash
# 1. Launch master
urp launch /path/to/project

# 2. Inside master, spawn worker
urp spawn

# 3. Optimize worker for task
urp exec urp-proj-w1 "pip install black isort"

# 4. Send instructions
urp ask urp-proj-w1 "create branch fix-auth-bug"
urp ask urp-proj-w1 "fix the token expiration bug in auth.go"
urp ask urp-proj-w1 "run tests: go test ./..."
urp ask urp-proj-w1 "format with black and commit"
urp ask urp-proj-w1 "push and create PR"

# 5. Review PR URL from worker output

# 6. Clean up
urp kill urp-proj-w1
```

### GPU/ML Tasks with NeMo

Workers can spawn NeMo containers for GPU-intensive operations:

```bash
# Worker launches NeMo for ML tasks
urp nemo start                    # Starts urp-nemo-<project>
urp nemo exec "pytest tests/"     # Run tests in NeMo (has torch, etc)
urp nemo exec "python train.py"   # Run training
urp nemo stop                     # Clean up

# Full flow from master
urp ask urp-proj-w1 "start nemo, run ML tests, stop when done"
```

NeMo container has:
- Full NVIDIA GPU access (`--gpus all`)
- PyTorch, NeMo, Optuna stack
- 16GB shared memory for training
- Same /workspace mount as worker

### Configuration

```bash
# ~/.urp-go/.env
ANTHROPIC_API_KEY=<key>
ANTHROPIC_BASE_URL=http://100.105.212.98:8317/
NEO4J_URI=bolt://urp-memgraph:7687
```

---

## Key Dependencies & Technologies

| Component | Technology | Location | Purpose |
|-----------|-----------|----------|---------|
| CLI Framework | Cobra | `go/cmd/urp/main.go` | Command routing and argument parsing |
| Graph DB | Memgraph (Neo4j API) | `internal/graph/` | Persistent code structure & history |
| Vector Store | Embeddings via Memgraph (LanceDB planned) | `internal/vector/` | Semantic similarity search |
| Container Runtime | Docker/Podman | `internal/container/` | Master/Worker isolation |
| TUI | Bubble Tea (Charmbracelet) | `internal/tui/` | Interactive terminal interface |
| LLM Providers | Anthropic Claude, OpenAI, DeepSeek, Google | `internal/opencode/provider/` | Multi-provider model support |
| Go Version | 1.24.0 | `go/go.mod` | Target runtime |

## Important Gotchas & Notes

### Memgraph Connection
- Required for most commands (except: `doctor`, `infra`, `version`, `help`, `models`)
- Connection established in `main.go` PersistentPreRun
- Health check: `urp doctor` or `docker logs urp-memgraph`
- If "cannot connect" error: start with `docker compose up -d memgraph`

### Master/Worker Protocol
- Master NEVER writes files directly (read-only mount: `/workspace:ro`)
- All mutations happen through worker containers with write access
- Communication via `urp ask <worker> "<prompt>"` in worker's Claude CLI
- Cleanup critical: `urp kill <worker>` to avoid orphaned containers

### Context Compiler (V2)
- Dynamically generates prompts from graph state, gated logs, and learned strategies
- 5 modes balance detail vs. token cost
- Check output with: `urp compile --goal "description"`

### LLM Provider Routing
- Multiple providers can be configured simultaneously
- Factory pattern in `opencode/provider/factory.go` auto-selects based on env vars
- Gate LLM (fast, noise filtering) separate from Master LLM (reasoning)
- If model not found: verify API key and `URP_<PROVIDER>_MODEL` env vars

### Test Coverage
- 86+ test files across codebase
- Panic recovery tests in `internal/logging/recovery_test.go` (note: intentional panics for testing)
- Run single test: `go test -run TestName ./path/to/package/`
- Coverage reports: `go test -coverprofile=coverage.out ./...` then `go tool cover -html=coverage.out`

### Graph Queries
- Cypher syntax (Neo4j-style)
- Results returned as Record objects in `internal/graph/record.go`
- Cache queries with TTL to avoid repeated network calls
- Common patterns: file lookups, dependency graphs, commit history

### Audit Logging
- All operations logged to Memgraph via `internal/audit/`
- Session-scoped: shared SessionID across Logger and Store
- Queryable via graph for debugging and compliance

### Bootstrapping Services
- Dependency injection via `internal/bootstrap/bootstrap.go`
- Ensures all services initialized in correct order: db → cache → memory → providers
- Add new services here, not scattered in commands

---
