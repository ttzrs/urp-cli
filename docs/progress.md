# URP-CLI Development Progress

## Session: 2025-12-02 - Initial Implementation

### Summary

Implemented the complete URP-CLI (Universal Repository Perception) system - a semantic knowledge graph interface for AI coding agents based on PRU (Primitive Relational Units).

### PRU Primitives Implemented

| Primitive | Symbol | Domain | Implementation |
|-----------|--------|--------|----------------|
| Domain | D | Entity existence | File, Function, Class, Container nodes |
| Vector | τ | Temporal sequence | Commits, terminal commands, log events |
| Morphism | Φ | Causal flow | Calls, deps, CPU/RAM energy, exit codes |
| Inclusion | ⊆ | Hierarchy | CONTAINS edges, network topology |
| Orthogonal | ⊥ | Conflicts | Dead code, circular deps, failed commands |
| Tensor | T | Context | Branch, environment, session |
| Projective | P | Viewpoint | (Future: interface vs implementation) |

### Files Created

| File | Lines | Purpose |
|------|-------|---------|
| `parser.py` | 263 | Multi-language AST (Python + Go, extensible registry) |
| `ingester.py` | 227 | Code → Graph (D, ⊆, Φ primitives) |
| `git_loader.py` | 165 | Git history → Graph (τ, T primitives) |
| `observer.py` | 240 | Docker/Process → Graph (Φ energy, τ events, ⊥ health) |
| `querier.py` | 303 | PRU-based queries across all primitives |
| `runner.py` | 220 | Transparent command wrapper (terminal flow capture) |
| `cli.py` | 320 | 16+ commands for agent interface |
| `shell_hooks.sh` | 120 | Bash function wrappers for git, npm, docker, etc. |
| `entrypoint.sh` | 90 | Container init script |
| `Dockerfile` | 78 | Full dev container |
| `docker-compose.yml` | 100 | Memgraph + URP stack |
| `requirements.txt` | 12 | Dependencies |
| `CLAUDE.md` | 156 | Agent instructions |

### Architecture

```
┌─────────────────────────────────────────────────────────────┐
│                    Terminal (bash/zsh)                       │
│  git, npm, docker, pytest → shell_hooks.sh → runner.py      │
└──────────────────────────────┬──────────────────────────────┘
                               │
                               ▼
┌─────────────────────────────────────────────────────────────┐
│                         cli.py                               │
│  ingest, git, impact, deps, vitals, pain, recent...         │
└──────────────────────────────┬──────────────────────────────┘
                               │
        ┌──────────────────────┼──────────────────────┐
        ▼                      ▼                      ▼
┌───────────────┐    ┌─────────────────┐    ┌───────────────┐
│   parser.py   │    │  git_loader.py  │    │  observer.py  │
│ (tree-sitter) │    │   (GitPython)   │    │   (Docker)    │
│   D, ⊆, Φ     │    │      τ, T       │    │   Φ, τ, ⊥     │
└───────┬───────┘    └────────┬────────┘    └───────┬───────┘
        │                     │                     │
        └─────────────────────┼─────────────────────┘
                              ▼
┌─────────────────────────────────────────────────────────────┐
│                    database.py → Memgraph                    │
│                                                              │
│  Nodes: File, Function, Class, Commit, Author, Container,   │
│         TerminalEvent, Session, LogEvent                     │
│                                                              │
│  Edges: CONTAINS, CALLS, FLOWS_TO, PARENT_OF, AUTHORED,     │
│         TOUCHED, CONNECTED_TO, EMITTED, EXECUTED            │
└─────────────────────────────────────────────────────────────┘
```

### Terminal Flow Capture

Shell commands transparently intercepted via bash functions:

```
User types: npm install express
     │
     ▼
shell_hooks.sh: npm() { _urp_wrap npm "$@"; }
     │
     ▼
runner.py: run_transparent(['npm', 'install', 'express'])
     │
     ├─► subprocess.run() → real npm executes (colors preserved)
     │
     └─► log_to_graph() → CREATE (:TerminalEvent:BuildEvent {...})
     │
     ▼
Exit code returned to shell
```

**Wrapped commands:** git, docker, podman, kubectl, npm, yarn, pip, cargo, go, make, mvn, gradle, pytest, jest, mocha

### Key Commands

```bash
# Graph building
urp ingest <path>        # Parse code
urp git <path>           # Load history

# Causal queries (Φ)
urp impact <sig>         # What breaks if I change this?
urp deps <sig>           # What does this depend on?

# Temporal queries (τ)
recent                   # Recent terminal commands
recent --errors          # Only failures
urp history <file>       # File change history
urp hotspots             # High churn files

# Conflict detection (⊥)
pain                     # Recent errors
urp dead                 # Uncalled functions
urp circular             # Dependency cycles
health                   # Container issues

# Runtime observation (Φ energy)
vitals                   # Container CPU/RAM
topology                 # Network map
```

### Next Steps

- [ ] Add more language parsers (Rust, TypeScript, Java)
- [ ] Implement P (Projective) primitive for interface analysis
- [ ] Add MAGE graph algorithms for advanced queries
- [ ] Stream mode for continuous observation
- [ ] Integration with Claude Code hooks system

---

## Session: 2025-12-02 - Cognitive Layer & Token Economy

### Summary

Added cognitive skills and LLM-optimized rendering. Focus shifted from infrastructure to **token economy** - giving agents surgical context instead of whole files.

### Problem Solved

**Before:** Agent reads 500-line file to fix 3 lines → 2000 tokens of noise.
**After:** Agent uses `focus function_name` → 80 tokens of signal.

### New Files

| File | Lines | Purpose |
|------|-------|---------|
| `brain_cortex.py` | 95 | Embedding model for semantic similarity |
| `brain_render.py` | 230 | Graph → LLM-friendly formats |

### Updated Files

| File | Changes |
|------|---------|
| `runner.py` | +180 lines: wisdom, novelty, focus cognitive skills |
| `shell_hooks.sh` | +3 aliases for cognitive skills |
| `CLAUDE.md` | Cognitive protocols + context profiles |
| `requirements.txt` | +sentence-transformers, numpy |
| `Dockerfile` | Model preload during build |

### Cognitive Skills Added

| Skill | Command | Purpose |
|-------|---------|---------|
| Wisdom | `wisdom "error msg"` | Find similar past errors via embeddings |
| Novelty | `novelty "code"` | Check if pattern breaks conventions |
| Focus | `focus target` | Load surgical subgraph context |

### Rendering Modes (brain_render.py)

| Mode | Output Format | Use Case |
|------|---------------|----------|
| `code` | Pseudo-TypeScript with `@CALLS()` | Structure/deps |
| `trace` | Inverse chronological | Errors/timeline |
| `minimal` | Single line | Quick lookups |
| `markdown` | Headers/lists | Reports |

### Context Profiles

| Profile | Task | Depth | Token Cost |
|---------|------|-------|------------|
| BUG FIX | Surgical repair | 1 | ~100 |
| REFACTOR | Structural | 2 | ~200 |
| FEATURE | Pattern copy | 1 | ~150 |
| DEBUG | Error trace | - | ~50 |

### Key Insight

> Graph stores relationships. Renderer translates to formats LLMs are trained on.
> Raw JSON wastes tokens. Pseudo-code maximizes comprehension.

### Example Output Comparison

**Before (JSON):**
```json
[{"id": 1, "name": "process", "type": "Function"},
 {"start": 1, "end": 2, "type": "CALLS"}...]
```

**After (Rendered):**
```
module 'payments/processor.py' {
  @CALLS(Database.connect)
  def process_payment(amount: int) { ... }
}
```

### Architecture Update

```
                    ┌─────────────────────┐
                    │   brain_cortex.py   │
                    │   (embeddings)      │
                    └──────────┬──────────┘
                               │
┌──────────────┐    ┌──────────▼──────────┐    ┌──────────────┐
│  Memgraph    │───►│   brain_render.py   │───►│     LLM      │
│  (raw graph) │    │   (serialize)       │    │ (understands)│
└──────────────┘    └─────────────────────┘    └──────────────┘

Render modes:
  - code: topology as pseudo-TypeScript
  - trace: events as causal narrative
  - minimal: bare facts, one line
```

### Next Steps

- [ ] Add surgical code reading (read only lines X-Y from file)
- [x] Implement RESOLVED_BY edges for wisdom learning
- [ ] Add attention scoring to prioritize context
- [ ] Continuous embedding updates on code changes

---

## Session: 2025-12-02 - Safety & Learning Systems

### Summary

Added two critical systems: **Immune System** (pre-execution safety filter) and **Consolidation** (reinforcement learning for successful command sequences).

### Problem Solved

1. **Agent destroying things**: LLMs hallucinate dangerous commands (`rm -rf /`, `git push --force`, `git add .env`)
2. **Not learning from success**: Agent solves problem but forgets; next time starts from zero

### New Files

| File | Lines | Purpose |
|------|-------|---------|
| `immune_system.py` | 180 | Deterministic pre-execution safety filter |

### Updated Files

| File | Changes |
|------|---------|
| `runner.py` | +90 lines: `consolidate_learning()`, immune system integration |
| `shell_hooks.sh` | +1 alias: `learn` |
| `CLAUDE.md` | Safety protocol + learning protocol docs |

### Immune System Rules

| Category | Blocked | Alternative |
|----------|---------|-------------|
| Filesystem | `rm -rf /`, `mkfs`, `dd if=` | Be specific, use `/tmp/trash` |
| Database | `DROP DATABASE`, `DELETE` w/o WHERE | Add WHERE clause |
| Git | `push --force`, `rm -rf .git` | `--force-with-lease` |
| Credentials | `git add .env`, `cat id_rsa` | Use `.gitignore`, env vars |
| Self | Edit `immune_system.py`, `runner.py` | Don't touch the brain |

### Learning System

```
Error occurs → Agent solves → User confirms "works"
                                    ↓
                            learn "Fixed X by doing Y"
                                    ↓
                            Creates :Solution node
                            Links successful commands
                            Links resolved :Conflict
                                    ↓
                            Next time: wisdom finds YOUR solution
```

### New Graph Schema

```
(:Conflict)-[:RESOLVES]->(:Solution)<-[:CONTRIBUTED_TO]-(:TerminalEvent)
```

### Architecture Update

```
Command Input
     │
     ▼
┌──────────────────┐
│ immune_system.py │ ──► BLOCK (exit 126) if dangerous
└────────┬─────────┘
         │ SAFE
         ▼
┌──────────────────┐
│   runner.py      │ ──► Execute + log to graph
└────────┬─────────┘
         │ On success
         ▼
┌──────────────────┐
│ consolidate()    │ ──► Create :Solution, link events
└──────────────────┘
```

### Next Steps

- [x] Background "dreamer" process for offline maintenance
- [x] RAG for external documentation (library APIs)
- [x] Surgical code reading
- [ ] Attention scoring for context prioritization

---

## Session: 2025-12-02 - Background Maintenance & Surgical Context

### Summary

Implemented three systems: **Dreamer** (background maintenance), **Surgical Read** (extract only specific lines), and **Docs RAG** (external documentation).

### Problem Solved

1. **Graph gets stale**: Files change, embeddings missing, orphan nodes accumulate
2. **Token waste on reads**: Agent reads whole file when it needs 10 lines
3. **Unknown libraries**: Agent hallucinates API when library not in training data

### New Files

| File | Lines | Purpose |
|------|-------|---------|
| `dreamer.py` | 250 | Background maintenance daemon ("REM sleep") |
| `docs_rag.py` | 280 | External documentation RAG system |

### Updated Files

| File | Changes |
|------|---------|
| `runner.py` | +60 lines: `surgical_read()` for targeted code extraction |
| `shell_hooks.sh` | +5 aliases: surgical, docs, docs-ingest, docs-query, docs-list |
| `docker-compose.yml` | +dreamer service (profile: full) |

### Dreamer Tasks

| Task | What it does | When |
|------|--------------|------|
| reingest | Re-parse modified files | After code changes |
| orphans | Delete nodes with no edges | Cleanup |
| embeddings | Generate missing vectors | Background |
| prune | Delete old terminal events | After 7 days |
| patterns | Find successful command patterns | Analysis |

Runs only when CPU < 15% for 3 consecutive checks.

### Surgical Read

**Before:**
```bash
# Agent reads whole file
cat payments/processor.py  # 500 lines = 2000 tokens
```

**After:**
```bash
# Agent reads only the function
surgical process_payment   # 15 lines = 60 tokens
```

Uses graph metadata (`start_line`, `end_line`) to extract exact code.

### Docs RAG

```bash
# Ingest library docs
docs-ingest requests /path/to/requests-docs.md

# Query when stuck
docs-query requests "how to set timeout"
```

Creates `:Library` and `:DocChunk` nodes with embeddings for semantic search.

### Architecture Update

```
┌──────────────────────────────────────────────────────────────────┐
│                         Active Session                            │
│  wisdom, novelty, focus, surgical, pain, learn                   │
└─────────────────────────────┬────────────────────────────────────┘
                              │
                              ▼
┌──────────────────────────────────────────────────────────────────┐
│                          Memgraph                                 │
│  Code nodes + Git nodes + Terminal events + Solutions + Docs     │
└─────────────────────────────┬────────────────────────────────────┘
                              │
                              ▼
┌──────────────────────────────────────────────────────────────────┐
│                         Dreamer (idle)                            │
│  reingest, cleanup, embeddings, prune, patterns                  │
│  "The unexamined graph is not worth querying."                   │
└──────────────────────────────────────────────────────────────────┘
```

### New Commands

```bash
# Surgical read (token economy)
surgical function_name       # Read only that function's code

# External docs
docs-ingest lib path         # Add library documentation
docs-query lib "question"    # Search documentation
docs-list                    # List ingested libraries

# Dreamer (manual)
python3 dreamer.py --once    # Run one maintenance cycle
python3 dreamer.py --task embeddings  # Run specific task
```

### Docker Profiles

| Profile | Services | Use Case |
|---------|----------|----------|
| (default) | memgraph, urp | Basic development |
| `ui` | + lab | Visual graph exploration |
| `dev` | + urp-dev | Writable codebase |
| `full` | + dreamer | Background maintenance |

```bash
docker-compose --profile full up -d
```

### Next Steps

- [ ] Attention scoring for context prioritization
- [ ] Stream mode for continuous file watching
- [ ] More language parsers (Rust, TypeScript)
- [ ] Integration with Claude Code hooks

---

## Session: 2025-12-02 - Working Memory Management (Token Economy)

### Summary

Implemented **Working Memory Manager** - a persistent context accumulator with budget enforcement and selective clearing.

### Problem Solved

**Token Accumulation**: Agent uses `focus` multiple times → context grows → tokens waste.
**No control**: Can't remove specific items, only clear everything or nothing.

### New Functionality in runner.py

| Function | Purpose |
|----------|---------|
| `load_working_memory()` | Load from `.session_context.json` |
| `save_working_memory()` | Persist state |
| `add_to_focus()` | Add target + auto-evict if over budget |
| `remove_from_focus()` | Remove specific target |
| `clear_working_memory()` | Tabula rasa |
| `get_working_memory_status()` | Token usage + items |
| `get_accumulated_context()` | Full injection string |

### New Commands

```bash
# Add to working memory (persists between commands)
focus AuthService         # +200 tokens

# Check what's accumulated
mem-status                # 200/4000 tokens (5%)

# Remove specific item
unfocus AuthService       # -200 tokens

# Clear everything
clear-context             # Tabula rasa

# See what would be injected
mem-context               # Full accumulated context
```

### Token Budget Enforcement

- Default: `URP_MAX_CONTEXT_TOKENS=4000`
- Auto-eviction: FIFO (oldest removed first)
- Estimation: ~4 chars = 1 token

### Flow Example

```bash
focus LoginController     # +150 tokens
focus DatabaseService     # +200 tokens
mem-status                # 350/4000 (8.7%)

# Done with DB
unfocus DatabaseService   # -200 tokens

# New task
clear-context             # 0/4000 tokens
focus PaymentService      # Fresh context
```

### Updated Files

| File | Changes |
|------|---------|
| `runner.py` | +170 lines: Working memory manager + 4 new CLI commands |
| `shell_hooks.sh` | +4 aliases: unfocus, clear-context, mem-status, mem-context |

### Architecture Update

```
┌─────────────────────────────────────────────────────────────────┐
│                     Working Memory Manager                       │
│  .session_context.json → persistent state between commands      │
└──────────────────────────────┬──────────────────────────────────┘
                               │
     ┌─────────────────────────┼─────────────────────────┐
     │                         │                         │
     ▼                         ▼                         ▼
┌─────────┐             ┌─────────────┐          ┌──────────────┐
│ focus X │             │ unfocus X   │          │ clear-context│
│ +tokens │             │ -tokens     │          │ tabula rasa  │
│ FIFO    │             │ selective   │          │ 0 tokens     │
└─────────┘             └─────────────┘          └──────────────┘
                               │
                               ▼
                        ┌─────────────┐
                        │ mem-context │
                        │ (injection) │
                        └─────────────┘
```

### Next Steps

- [ ] Attention scoring for context prioritization
- [ ] LRU eviction (not just FIFO)
- [ ] Per-item TTL (auto-expire old focus items)
- [ ] Integration with Claude Code hooks

---

## Session: 2025-12-02 - Code Audit & Dev Container

### Summary

Full codebase audit, bug fixes, and creation of a **full-stack dev container** with Go, Python, Node, Docker, Claude CLI, GH, HF, NVIDIA support, and SELinux compatibility.

### Bugs Fixed

| File | Line | Issue | Fix |
|------|------|-------|-----|
| `parser.py` | 177 | Go type signature used `node.start_point` (tuple) instead of `file_path` | Changed to `file_path` |
| `dreamer.py` | 142 | Called `ingester.ingest_file()` which doesn't exist | Changed to `_ingest_file()` |

### New Files

| File | Purpose |
|------|---------|
| `.gitignore` | Ignore `__pycache__`, `.session_context.json`, `.env`, etc. |
| `README.md` | Project documentation |
| `devcontainer/Dockerfile` | Full-stack dev container (minimal NVIDIA base) |
| `devcontainer/docker-compose.yml` | Stack with fixed IPs, no port mapping |
| `devcontainer/entrypoint.sh` | Container init with GPU check |
| `devcontainer/knowledge.py` | Shared memory knowledge manager |
| `urp-dev` | Launcher script |

### Dev Container Stack

| Component | Version/Image |
|-----------|---------------|
| Base | `nvidia/cuda:12.4.0-base-ubuntu22.04` (~1.5GB) |
| Go | 1.22.5 |
| Python | 3.11 |
| Node | 20.x |
| Docker CLI | Latest |
| Claude CLI | `@anthropic-ai/claude-code` |
| Git | Latest |
| GH CLI | Latest |
| HF CLI | Latest |
| PyTorch | CUDA 12.4 |

### Network Configuration (No Port Mapping)

```
Network: 172.28.0.0/16

┌─────────────┬──────────────┐
│ Service     │ IP           │
├─────────────┼──────────────┤
│ memgraph    │ 172.28.0.10  │
│ lab         │ 172.28.0.11  │
│ urp         │ 172.28.0.20  │
│ dreamer     │ 172.28.0.21  │
└─────────────┴──────────────┘
```

SELinux: All volumes use `:z` for proper labeling.

### Shared Memory System

```bash
# Upload knowledge (persists across restarts)
./urp-dev --upload docs/api.md --tag api

# Inside container
knowledge list                    # List all knowledge
knowledge sync                    # Sync to graph RAG

# Persistent directories
/shared/knowledge   # Knowledge files
/root/.claude       # Claude config
```

### Usage

```bash
# Launch in current directory
./urp-dev

# Launch in specific project
./urp-dev /path/to/project

# Upload knowledge and launch
./urp-dev --upload docs/api.md --tag api

# Include Memgraph Lab UI
./urp-dev --ui

# Include dreamer daemon
./urp-dev --full

# Run claude directly
./urp-dev --claude

# Force rebuild
./urp-dev --build
```

### Image Size Optimization

| Image | Size |
|-------|------|
| `nvidia/cuda:12.4.0-devel` | ~8 GB |
| `nvidia/cuda:12.4.0-runtime` | ~3 GB |
| `nvidia/cuda:12.4.0-base` | **~1.5 GB** |

PyTorch bundles its own CUDA libs, so `base` is sufficient.

### Architecture

```
┌─────────────────────────────────────────────────────────────────┐
│                         Host (Fedora/SELinux)                    │
│  ./urp-dev /path/to/project                                     │
└──────────────────────────────┬──────────────────────────────────┘
                               │
                               ▼
┌─────────────────────────────────────────────────────────────────┐
│                     Docker Network: 172.28.0.0/16                │
│                                                                  │
│  ┌──────────────┐    ┌──────────────┐    ┌──────────────┐       │
│  │  memgraph    │    │     urp      │    │   dreamer    │       │
│  │ 172.28.0.10  │◄───│ 172.28.0.20  │    │ 172.28.0.21  │       │
│  │  (graph DB)  │    │ (dev shell)  │    │ (background) │       │
│  └──────────────┘    └──────────────┘    └──────────────┘       │
│                              │                                   │
│                              ▼                                   │
│                      ┌──────────────┐                            │
│                      │  /workspace  │ ← project mounted :z       │
│                      │  /shared     │ ← persistent volumes       │
│                      │  /root/.claude│                           │
│                      └──────────────┘                            │
└─────────────────────────────────────────────────────────────────┘
```

### Next Steps

- [ ] Integration with Claude Code hooks
- [ ] Automated knowledge sync on file changes
- [ ] Multi-project workspace support
