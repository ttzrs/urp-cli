# URP Architecture

## Design Philosophy

```
AXIOM: context_window ⊂ memory_total
       memory_total = context ∪ graph_db ∪ vector_store
```

AI agents have limited context windows. URP extends this with:
- **Graph DB**: Structured relationships (code, git, solutions)
- **Vector Store**: Semantic similarity search
- **Container Isolation**: Safe execution environment

## System Architecture

```
┌─────────────────────────────────────────────────────────────────────┐
│                            HOST MACHINE                              │
│                                                                      │
│  ~/.urp-go/                                                         │
│  ├── .env              # API keys                                   │
│  ├── alerts/           # System alerts                              │
│  └── vector/           # Local vector store                         │
│                                                                      │
└──────────────────────────────┬──────────────────────────────────────┘
                               │
                    urp-<project>-net (isolated network)
                               │
        ┌──────────────────────┼──────────────────────┐
        │                      │                      │
        ▼                      ▼                      ▼
┌───────────────┐    ┌─────────────────┐    ┌───────────────────┐
│ urp-memgraph  │    │  urp-master     │    │  urp-worker-N     │
│               │    │                 │    │                   │
│ Graph DB      │◄───│ /workspace:ro   │───►│ /workspace:rw     │
│ bolt:7687     │    │ Docker socket   │    │ Claude CLI        │
│               │    │ Claude CLI      │    │ Dev tools         │
└───────────────┘    └─────────────────┘    └───────────────────┘
                               │
                               ▼
                     ┌───────────────────┐
                     │  urp-nemo         │  (optional)
                     │  GPU container    │
                     │  PyTorch, NeMo    │
                     └───────────────────┘
```

## Master/Worker Model

### Why Master/Worker?

1. **Safety**: Master never writes to workspace
2. **Isolation**: Each worker is disposable
3. **Parallelism**: Multiple workers for complex tasks
4. **Auditability**: All operations logged

### Flow

```
User ──► urp launch ──► MASTER container
                            │
                            ├── Read-only project access
                            ├── Claude CLI for planning
                            ├── Can spawn workers
                            │
                            ▼
                       urp spawn ──► WORKER container
                                        │
                                        ├── Read-write access
                                        ├── Claude CLI for execution
                                        ├── Dev tools installed
                                        │
                                        ▼
                                   urp ask "task"
                                        │
                                        ▼
                                   Worker executes
                                        │
                                        ▼
                                   urp kill (cleanup)
```

### Communication

```bash
# Master sends instructions to worker
urp ask urp-proj-w1 "create feature branch and implement auth"

# Master can customize worker before task
urp exec urp-proj-w1 "pip install pytest-cov"

# Master reads worker output
urp infra logs urp-proj-w1
```

## Memory Architecture

```
┌─────────────────────────────────────────┐
│            CONTEXT WINDOW               │  ← LLM limit (~200k tokens)
│  (current conversation + recent tools)  │
└─────────────────────────────────────────┘
                    │
                    ▼ urp focus / urp mem recall
┌─────────────────────────────────────────┐
│           SESSION MEMORY                │  ← urp mem *
│  (ephemeral, current task context)      │
└─────────────────────────────────────────┘
                    │
                    ▼ urp kb query
┌─────────────────────────────────────────┐
│          KNOWLEDGE BASE                 │  ← urp kb *
│  scope: session → instance → global     │
│  (persistent, cross-session learning)   │
└─────────────────────────────────────────┘
                    │
                    ▼ urp think wisdom
┌─────────────────────────────────────────┐
│           GRAPH DATABASE                │  ← Memgraph
│  (code structure, git history,          │
│   solutions, relationships)             │
└─────────────────────────────────────────┘
                    │
                    ▼ urp vec search
┌─────────────────────────────────────────┐
│           VECTOR STORE                  │  ← LanceDB
│  (semantic embeddings, similarity)      │
└─────────────────────────────────────────┘
```

## PRU Perception Model

Seven primitives for structured perception:

### D (Domain) - Entity Types

```cypher
(:File {path, hash, language, loc})
(:Function {name, signature, complexity, start_line, end_line})
(:Class {name, methods, properties})
(:Container {name, image, status, cpu, memory})
(:Commit {hash, message, author, timestamp})
(:Memory {id, content, type, created_at})
(:Solution {id, problem, solution, effectiveness})
(:Session {id, project, branch, started_at})
```

### τ (Temporal) - Time Sequences

```cypher
// Git history
(c1:Commit)-[:PARENT_OF]->(c2:Commit)

// Events
(e1:Event)-[:FOLLOWED_BY]->(e2:Event)

// Command execution
(:Command {cmd, exit_code, duration, timestamp})
```

### Φ (Morphism) - Causal Flow

```cypher
// Function calls
(f1:Function)-[:CALLS]->(f2:Function)

// Data flow
(v1:Variable)-[:FLOWS_TO]->(v2:Variable)

// Container resources
(:Container)-[:CONSUMES {cpu, memory}]->(:Resource)

// Error causation
(e1:Error)-[:CAUSED_BY]->(e2:Error)
```

### ⊆ (Inclusion) - Hierarchy

```cypher
// Code structure
(:File)-[:CONTAINS]->(:Function)
(:Class)-[:HAS_METHOD]->(:Function)

// Network topology
(:Network)-[:CONTAINS]->(:Container)

// Project structure
(:Project)-[:HAS_FILE]->(:File)
```

### ⊥ (Orthogonal) - Conflicts

```cypher
// Dead code
(:Function {dead: true})

// Circular dependencies
(f1:Function)-[:CALLS]->(f2:Function)-[:CALLS]->(f1)

// Errors
(:Error)-[:CONFLICTS_WITH]->(:Expectation)

// Test failures
(:Test)-[:FAILED]->(:Assertion)
```

### P (Projective) - Viewpoints

```cypher
// Interface vs Implementation
(:Interface)-[:IMPLEMENTED_BY]->(:Class)

// Public vs Private
(:Function {visibility: 'public'|'private'})
```

### T (Tensor) - Context

```cypher
// Multi-dimensional context
(:Session {
  project: 'urp-cli',
  branch: 'feature-x',
  env: 'development',
  os: 'linux'
})

// Context compatibility
signature := project ∩ branch ∩ env
compatible(a,b) := a.signature ≈ b.signature
```

## Package Structure

```
go/
├── cmd/urp/                 # CLI entry point
│   └── main.go
│
├── internal/
│   ├── container/           # Container orchestration
│   │   ├── manager.go       # Docker/Podman abstraction
│   │   ├── health.go        # Health checks
│   │   └── volume.go        # Volume management
│   │
│   ├── graph/               # Graph database
│   │   ├── driver.go        # Memgraph driver
│   │   └── queries.go       # Cypher queries
│   │
│   ├── vector/              # Vector store
│   │   ├── store.go         # LanceDB interface
│   │   └── embedder.go      # Embedding generation
│   │
│   ├── cognitive/           # Cognitive skills
│   │   ├── wisdom.go        # Find similar solutions
│   │   ├── novelty.go       # Detect unusual patterns
│   │   └── learn.go         # Store learnings
│   │
│   ├── memory/              # Memory systems
│   │   ├── session.go       # Session memory
│   │   ├── knowledge.go     # Knowledge base
│   │   └── focus.go         # Context loading
│   │
│   ├── opencode/            # AI agent system
│   │   ├── agent/           # Agent executor
│   │   ├── provider/        # LLM providers
│   │   │   ├── anthropic.go
│   │   │   ├── openai.go
│   │   │   └── google.go
│   │   ├── tool/            # Agent tools
│   │   │   ├── bash.go
│   │   │   ├── read.go
│   │   │   ├── write.go
│   │   │   └── ...
│   │   ├── session/         # Session management
│   │   └── domain/          # Domain types
│   │
│   ├── spec/                # Spec-driven development
│   │   ├── parser.go        # Spec parser
│   │   └── runner.go        # Spec executor
│   │
│   ├── skill/               # Skills system
│   │   ├── registry.go      # Skill registry
│   │   └── executor.go      # Skill executor
│   │
│   ├── audit/               # Audit logging
│   │   └── logger.go        # Structured audit logs
│   │
│   ├── tui/                 # Terminal UI
│   │   └── tui.go           # Bubble Tea interface
│   │
│   └── logging/             # Logging utilities
│       └── recovery.go      # Panic recovery
│
└── pkg/
    └── llm/                 # LLM abstractions
        └── provider.go
```

## Data Flow

### Code Analysis

```
urp code ingest .
        │
        ▼
    Parse AST ──► Extract entities ──► Store in Memgraph
        │
        ├── Files
        ├── Functions (with signatures, complexity)
        ├── Classes
        ├── Calls relationships
        └── Import dependencies
```

### Cognitive Query

```
urp think wisdom "connection refused"
        │
        ▼
    Embed query ──► Vector search ──► Graph expansion
        │                                   │
        │                                   ▼
        │                            Related solutions
        │                                   │
        └───────────────────────────────────┼
                                            ▼
                                    Ranked results
```

### Spec Execution

```
urp spec run my-feature
        │
        ▼
    Load spec.md ──► Create AI agent ──► Execute with tools
        │                                       │
        │                    ┌──────────────────┼──────────────────┐
        │                    ▼                  ▼                  ▼
        │                [read]            [write]            [bash]
        │                    │                  │                  │
        │                    └──────────────────┼──────────────────┘
        │                                       │
        │                                       ▼
        │                              Autocorrection loop
        │                                       │
        └───────────────────────────────────────┼
                                                ▼
                                         Generated code
```

## Security Model

### Isolation Layers

1. **Container isolation**: Each worker is isolated
2. **Network isolation**: Project-scoped networks
3. **File isolation**: Master read-only, workers write to project only
4. **Command filtering**: Immune system blocks dangerous commands

### Immune System

```go
BLOCKED = {
    "rm -rf /":        "use specific path",
    "git push --force": "use --force-with-lease",
    "git add .env":    "use .gitignore",
    "DROP DATABASE":   "requires approval",
    "mkfs":            "filesystem destruction",
}
```

### Audit Trail

All operations logged:
- Container spawns/kills
- Command executions
- File modifications
- API calls
- Errors and recoveries

## Performance Considerations

### Token Optimization

```python
# Context profiles by task type
BUG_FIX:   focus(func, depth=1)    # ~100 tokens, minimal
REFACTOR:  focus(class, depth=2)   # ~500 tokens, structure only
FEATURE:   focus(similar) + wisdom # ~1000 tokens, patterns
DEBUG:     events errors + vitals  # ~200 tokens, causal
```

### Caching

- Graph queries cached per session
- Vector embeddings cached on disk
- Container status cached with TTL

### Lazy Loading

- Large files streamed, not loaded
- Graph relationships fetched on demand
- Embeddings generated asynchronously
