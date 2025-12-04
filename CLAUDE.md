# URP: Embodied Agent Protocol

```
AXIOM: context_window ⊂ memory_total
       memory_total = context ∪ graph_db ∪ vector_store
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

### on_error(red_output):
```python
PAUSE()                              # ¬guess
result = urp think wisdom "<error>"  # query similar
if result.similarity > 0.8:
    apply(result.solution)           # reuse
else:  # PIONEER
    analyze(urp events errors)
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

## Build

```bash
cd go && go build -o urp ./cmd/urp && go test ./...
```

## Env

```
NEO4J_URI=bolt://localhost:7687
URP_PROJECT=auto
URP_SESSION_ID=auto
```

---

## Orchestration Architecture (Master/Worker)

### Flow

```
urp launch [path]
    │
    ├─► Create MASTER container (project:ro, SELinux :z)
    ├─► Auto-ingest: urp code ingest && urp git ingest
    ├─► Open Claude CLI (interactive, user controls)
    │
    └─► User ↔ Claude (master)
            │
            ├─► Claude analyzes, plans → graph (:Plan)-[:HAS_TASK]->(:Task)
            ├─► Claude spawns WORKERS via `urp spawn`
            │       └─► Worker: ephemeral (docker --rm), project:rw
            │       └─► Worker runs Claude CLI with instructions from master
            │       └─► Worker can install tools, modify code, debug
            ├─► Master monitors workers via graph + envelope protocol
            └─► On task complete: worker dies, result in graph
```

### Container Topology

```
┌─────────────────────────────────────────────────────────────┐
│  HOST (SELinux, X11)                                        │
│  ~/.urp-go/                                                 │
│    ├── .env          # ANTHROPIC_API_KEY, ANTHROPIC_BASE_URL│
│    ├── data/         # persistent data                      │
│    └── config/       # claude cli config                    │
└──────────────┬──────────────────────────────────────────────┘
               │
    ┌──────────┴──────────┐
    │                     │
    ▼                     ▼
┌─────────────┐    ┌─────────────────────────────────┐
│ urp-memgraph│    │ urp-master-<project>            │
│ (persistent)│◄───│  - /workspace:ro:z              │
│ bolt:7687   │    │  - ~/.urp-go/.env:ro:Z          │
└─────────────┘    │  - Claude CLI (user interactive)│
                   │  - Can spawn workers            │
                   │  - Reads worker output          │
                   └──────────────┬──────────────────┘
                                  │ docker socket
                   ┌──────────────┼──────────────┐
                   ▼              ▼              ▼
            ┌───────────┐  ┌───────────┐  ┌───────────┐
            │ worker-1  │  │ worker-2  │  │ worker-n  │
            │ --rm      │  │ --rm      │  │ --rm      │
            │ rw:z      │  │ rw:z      │  │ rw:z      │
            │ Claude CLI│  │ Claude CLI│  │ Claude CLI│
            └───────────┘  └───────────┘  └───────────┘
                 │              │              │
                 └──────────────┴──────────────┘
                          results → graph
```

### Envelope Protocol (Master ↔ Worker)

JSON-lines over stdin/stdout:

```json
// Master → Worker (instruction)
{"id":"t-001","type":"instruction","task":"Fix bug in auth.go","context":{"files":["auth.go"]}}

// Worker → Master (status)
{"id":"t-001","type":"status","state":"running","progress":0.3}

// Worker → Master (result)
{"id":"t-001","type":"result","success":true,"changes":["auth.go:42"],"summary":"Fixed null check"}

// Worker → Master (error)
{"id":"t-001","type":"error","code":"BLOCKED","reason":"Immune system blocked rm -rf"}
```

Envelope wrapper intercepts Claude CLI stdin/stdout, parses, logs to graph.

### Graph Schema (Orchestration)

```cypher
// Plan nodes
(:Plan {id, project, created_at, status})
(:Task {id, description, status, worker_id, started_at, completed_at})
(:TaskResult {id, success, changes, summary, error})

// Relationships
(:Plan)-[:HAS_TASK]->(:Task)
(:Task)-[:EXECUTED_BY]->(:Container)
(:Task)-[:PRODUCED]->(:TaskResult)
(:Task)-[:DEPENDS_ON]->(:Task)
(:TaskResult)-[:MODIFIED]->(:File)

// Learning from failures
(:Task)-[:FAILED_WITH]->(:Error)
(:Error)-[:RESOLVED_BY]->(:Solution)
```

### X11 Browser Worker

On-demand when master needs visual testing:

```bash
# Master requests browser
urp spawn --type=browser

# Spawns worker with X11 forwarding
docker run --rm \
  -e DISPLAY=$DISPLAY \
  -v /tmp/.X11-unix:/tmp/.X11-unix:z \
  -v ~/.urp-go/.env:/etc/urp/.env:ro:Z \
  --network urp-network \
  urp:browser firefox
```

### Configuration

```bash
# ~/.urp-go/.env
ANTHROPIC_API_KEY=<key>
ANTHROPIC_BASE_URL=http://100.105.212.98:8317/
NEO4J_URI=bolt://urp-memgraph:7687
URP_ENVELOPE_MODE=json  # json | delimiter | raw
```

### Commands (Extended)

```bash
# ORCHESTRATION
urp launch [path]      # Create master, ingest, open Claude
urp spawn [--type=T]   # Spawn ephemeral worker (default|browser|test)
urp workers            # List active workers
urp plan show          # Show current plan from graph
urp plan status        # Task completion status

# ENVELOPE (internal, used by wrapper)
urp envelope send <worker> <json>   # Send instruction to worker
urp envelope recv <worker>          # Read worker output
urp envelope log <json>             # Log envelope to graph
```

### Implementation Priority

```
P1: urp launch (master + auto-ingest + claude)
P2: urp spawn (ephemeral workers with envelope)
P3: Graph schema for Plan/Task/Result
P4: Envelope wrapper for Claude CLI
P5: X11 browser worker type
```
