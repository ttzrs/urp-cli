# URP-CLI Session Summary

**Date:** 2025-12-02
**Sessions:** 6 (Initial + Cognitive Layer + Safety & Learning + Background Maintenance + Working Memory + Dev Container)

---

## What We Built

URP-CLI (Universal Repository Perception) - A semantic knowledge graph + cognitive layer for AI coding agents.

**Core insights:**
1. Store in graphs, render for LLMs. Raw JSON wastes tokens.
2. Safety first: deterministic filters before AI reasoning.
3. Learn from success: consolidate winning sequences.

## The 7 PRU Primitives

```
D  (Domain)      → Entity existence
τ  (Vector)      → Temporal sequence
Φ  (Morphism)    → Causal flow / energy
⊆  (Inclusion)   → Hierarchy
⊥  (Orthogonal)  → Conflicts
T  (Tensor)      → Context / Norms
P  (Projective)  → Viewpoint (future)
```

## Files

```
# Core
parser.py          → Multi-language AST (Python, Go)
ingester.py        → Code → Graph nodes
git_loader.py      → Git commits → Graph
observer.py        → Docker containers → Graph
querier.py         → PRU-based queries
runner.py          → Terminal wrapper + cognitive skills + learning + surgical read
cli.py             → Main CLI interface

# Cognitive Layer (Session 2)
brain_cortex.py    → Embeddings (sentence-transformers)
brain_render.py    → Graph → LLM-friendly formats

# Safety & Learning (Session 3)
immune_system.py   → Pre-execution safety filter (⊥)

# Background Maintenance (Session 4)
dreamer.py         → Background daemon ("REM sleep")
docs_rag.py        → External documentation RAG

# Infrastructure
shell_hooks.sh     → Bash function wrappers
entrypoint.sh      → Container init
Dockerfile         → Dev container
docker-compose.yml → Full stack (+ dreamer service)
```

## Token Economy (Session 2 Focus)

**Problem:** Agent reads 500 lines to fix 3 → 2000 tokens wasted.
**Solution:** Surgical context via `focus` → 80 tokens of signal.

### Cognitive Skills

| Skill | Command | Purpose |
|-------|---------|---------|
| Wisdom | `wisdom "error"` | Find similar past errors (embeddings) |
| Novelty | `novelty "code"` | Check if pattern breaks conventions |
| Focus | `focus target` | Load surgical subgraph context |
| Learn | `learn "description"` | Consolidate success into knowledge |
| Surgical | `surgical function` | Read only specific function/class (Session 4) |

### Rendering Modes

| Mode | Format | Use Case |
|------|--------|----------|
| `code` | Pseudo-TypeScript | Structure/deps |
| `trace` | Inverse chronological | Errors/timeline |
| `minimal` | Single line | Quick lookups |

### Context Profiles

| Profile | Task | Tokens |
|---------|------|--------|
| BUG FIX | Surgical | ~100 |
| REFACTOR | Structural | ~200 |
| DEBUG | Trace | ~50 |

## Usage

```bash
# Start
docker-compose up -d
docker-compose exec urp bash

# Cognitive commands
focus MyClass --depth 2     # Surgical context
wisdom "Connection refused" # Find similar past errors
novelty "new pattern code"  # Check if too novel
pain --minutes 10           # Causal error trace

# Graph queries
urp impact X                # What breaks if X changes?
urp deps X                  # What does X depend on?
vitals                      # Container CPU/RAM
```

## Output Examples

**Before (JSON noise):**
```json
[{"id": 1, "name": "process"}, {"start": 1, "end": 2}...]
```

**After (LLM-friendly):**
```
module 'payments/processor.py' {
  @CALLS(Database.connect)
  def process_payment(amount: int) { ... }
}
```

## Architecture

```
                    ┌─────────────────────┐
                    │   brain_cortex.py   │
                    │   (embeddings)      │
                    └──────────┬──────────┘
                               │
Terminal ──► runner.py ────────┼──► Memgraph ──► brain_render.py ──► LLM
                               │
Code ──────► ingester.py ──────┤
                               │
Git ───────► git_loader.py ────┘
```

## Safety System (Session 3)

**Immune System** - Deterministic pre-execution filter (not AI guessing).

| Blocked | Alternative |
|---------|-------------|
| `rm -rf /` | Be specific about paths |
| `git push --force` | Use `--force-with-lease` |
| `git add .env` | Add to `.gitignore` |
| `DROP DATABASE` | Requires user approval |
| Edit `immune_system.py` | Don't touch the brain |

## Learning System (Session 3)

```
Error → Agent solves → User says "works" → learn "description"
                                                ↓
                                        Creates :Solution node
                                        Links successful commands
                                        Links resolved :Conflict
                                                ↓
                                        wisdom finds YOUR solution next time
```

## Session 4: Background Maintenance & Surgical Context

### Dreamer (Background Daemon)

Runs when CPU idle. Performs maintenance without blocking work.

| Task | What it does |
|------|--------------|
| reingest | Re-parse modified files |
| orphans | Delete nodes with no edges |
| embeddings | Generate missing vectors |
| prune | Delete old events (7 days) |
| patterns | Find successful command patterns |

```bash
docker-compose --profile full up -d  # Enable dreamer
python3 dreamer.py --once            # Manual run
```

### Surgical Read

```bash
surgical process_payment   # 60 tokens vs 2000 for whole file
```

Uses graph metadata (`start_line`, `end_line`) to extract exact code.

### External Docs RAG

```bash
docs-ingest requests /path/to/docs.md  # Add library docs
docs-query requests "set timeout"       # Semantic search
docs-list                               # Show ingested libs
```

Creates `:Library` and `:DocChunk` nodes with embeddings.

## Session 5: Working Memory Management

### Problem

Agent uses `focus` multiple times → context accumulates → token waste.
No way to selectively remove items or enforce budget.

### Solution: Working Memory Manager

Persistent state in `.session_context.json` with budget enforcement.

```bash
# Add to working memory
focus AuthService         # +200 tokens, persists

# Check usage
mem-status                # 200/4000 tokens (5%)

# Selective removal
unfocus AuthService       # -200 tokens

# Nuclear option
clear-context             # Tabula rasa

# See injection payload
mem-context               # What LLM receives
```

### Budget Enforcement

- `URP_MAX_CONTEXT_TOKENS=4000` (configurable)
- FIFO eviction when over budget
- ~4 chars = 1 token estimation

### New Commands

| Command | Purpose |
|---------|---------|
| `unfocus X` | Remove specific target |
| `clear-context` | Clear all |
| `mem-status` | Show usage |
| `mem-context` | Show injection |

---

## Result

The agent now has:
- **Memory** (code/git in graph)
- **Proprioception** (container state)
- **Timeline** (command history)
- **Conflict detection** (errors, dead code)
- **Episodic memory** (wisdom - similar past errors)
- **Novelty detection** (pattern deviation)
- **Selective attention** (focus - surgical context)
- **Safety instincts** (immune system - deterministic blocks)
- **Reinforcement learning** (learn - consolidate success)
- **Surgical precision** (surgical - read only what you need)
- **External knowledge** (docs RAG - library APIs)
- **Self-maintenance** (dreamer - graph stays fresh)
- **Working memory control** (unfocus/clear - token hygiene)

All rendered in formats LLMs actually understand.

---

## Session 6: Code Audit & Dev Container

### Code Audit

Full audit of 13 Python files + infrastructure. Found 2 bugs:

| File | Line | Bug | Fix |
|------|------|-----|-----|
| `parser.py` | 177 | Go type signature used `node.start_point` | Changed to `file_path` |
| `dreamer.py` | 142 | Called non-existent `ingester.ingest_file()` | Changed to `ingester._ingest_file()` |

Created `.gitignore` and `README.md`.

### Dev Container (Full Stack)

Created production-ready development container with:

- **Base:** `nvidia/cuda:12.4.0-base-ubuntu22.04` (~1.5GB vs ~8GB devel)
- **Languages:** Go 1.22.5, Python 3.11, Node 20
- **Tools:** Docker CLI, GH CLI, Claude CLI, HuggingFace CLI
- **GPU:** NVIDIA runtime with CUDA 12.4
- **Database:** Memgraph (graph DB)
- **SELinux:** All volumes use `:z` for proper labeling

### Network Architecture

Custom Docker network (no localhost port conflicts):

```
172.28.0.0/16
├── memgraph: 172.28.0.10
├── lab:      172.28.0.11 (optional UI)
├── urp:      172.28.0.20
└── dreamer:  172.28.0.21 (optional)
```

### Shared Memory

Persistent volumes across sessions:

| Volume | Mount | Purpose |
|--------|-------|---------|
| `urp_claude_config` | `/root/.claude` | Claude CLI config |
| `urp_shared_knowledge` | `/shared/knowledge` | Uploaded knowledge |
| `urp_sessions` | `/shared/sessions` | Session persistence |

### Knowledge Management

```bash
knowledge-upload file.md --tag api   # Upload to shared memory
knowledge-list                        # List uploaded knowledge
knowledge-sync                        # Sync to RAG system
```

### Launcher Script

```bash
urp-dev                    # Launch in current directory
urp-dev /path/to/project   # Launch in specific directory
urp-dev --upload docs.md   # Upload knowledge before launch
urp-dev --claude           # Run Claude directly
urp-dev --ui               # Include Memgraph Lab UI
urp-dev --full             # Include dreamer daemon
urp-dev --build            # Force rebuild
```

### Files Created

```
devcontainer/
├── Dockerfile           # Full stack container
├── docker-compose.yml   # Stack with fixed IPs
├── entrypoint.sh        # Container init
├── knowledge.py         # Shared memory manager
└── requirements.txt     # Python deps

urp-dev                  # Launcher script
.gitignore               # Git ignore patterns
README.md                # Project documentation
```

### Current Capabilities

The agent now has a complete development environment:

1. **Semantic Memory** - Code/git in Memgraph graph
2. **Proprioception** - Container CPU/RAM/network monitoring
3. **Safety** - Immune system blocks dangerous commands
4. **Learning** - Consolidate successful solutions
5. **Token Economy** - Surgical context loading
6. **Working Memory** - Explicit context management
7. **External Knowledge** - RAG for library docs
8. **Self-Maintenance** - Dreamer daemon
9. **GPU Acceleration** - NVIDIA CUDA support
10. **SELinux Compatible** - Works on Fedora/RHEL
11. **Persistent Knowledge** - Shared volumes across sessions
