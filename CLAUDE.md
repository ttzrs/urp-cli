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

### Configuration

```bash
# ~/.urp-go/.env
ANTHROPIC_API_KEY=<key>
ANTHROPIC_BASE_URL=http://100.105.212.98:8317/
NEO4J_URI=bolt://urp-memgraph:7687
```
