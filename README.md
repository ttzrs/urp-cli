# URP-CLI: Universal Repository Perception

Semantic knowledge graph + cognitive layer for AI coding agents.

**Core insight:** Store in graphs, render for LLMs. Raw JSON wastes tokens.

## Quick Start

```bash
# Start the stack
docker-compose up -d

# Enter the dev container
docker-compose exec urp bash

# Inside container - commands are automatically logged to graph
git status      # Logged
npm install     # Logged
pytest          # Logged

# Query what happened
pain            # Recent errors
recent          # Recent commands
vitals          # Container health
```

## Architecture

```
Terminal ──► runner.py ────────┬──► Memgraph ──► brain_render.py ──► LLM
                               │
Code ──────► ingester.py ──────┤
                               │
Git ───────► git_loader.py ────┘
                               │
            brain_cortex.py ───┘
            (embeddings)
```

## The 7 PRU Primitives

```
D  (Domain)      → Entity existence: File, Function, Class
τ  (Vector)      → Temporal sequence: Commits, commands
Φ  (Morphism)    → Causal flow: Calls, data flow
⊆  (Inclusion)   → Hierarchy: File→Function, Class→Method
⊥  (Orthogonal)  → Conflicts: Errors, dead code
T  (Tensor)      → Context: Branch, environment
P  (Projective)  → Viewpoint (future)
```

## Files

| File | Purpose |
|------|---------|
| `cli.py` | Main CLI interface |
| `runner.py` | Command wrapper + cognitive skills |
| `parser.py` | Multi-language AST (Python, Go) |
| `ingester.py` | Code → Graph nodes |
| `git_loader.py` | Git commits → Graph |
| `observer.py` | Docker containers → Graph |
| `querier.py` | PRU-based queries |
| `brain_cortex.py` | Embeddings (sentence-transformers) |
| `brain_render.py` | Graph → LLM-friendly formats |
| `immune_system.py` | Pre-execution safety filter |
| `dreamer.py` | Background maintenance daemon |
| `docs_rag.py` | External documentation RAG |

## Cognitive Commands

| Command | Purpose |
|---------|---------|
| `wisdom "error"` | Find similar past errors |
| `novelty "code"` | Check if pattern breaks conventions |
| `focus target` | Load surgical subgraph context |
| `learn "desc"` | Consolidate success into knowledge |
| `surgical fn` | Read only specific function |

## Working Memory

```bash
focus AuthService         # Add to working memory
mem-status                # Show token usage
unfocus AuthService       # Remove from memory
clear-context             # Clear all
```

## Safety System

Deterministic pre-execution filter (not AI guessing).

| Blocked | Alternative |
|---------|-------------|
| `rm -rf /` | Be specific about paths |
| `git push --force` | Use `--force-with-lease` |
| `git add .env` | Add to `.gitignore` |
| `DROP DATABASE` | Requires user approval |

## Docker Compose Profiles

```bash
docker-compose up -d                      # Basic: memgraph + urp
docker-compose --profile ui up -d         # + Memgraph Lab UI
docker-compose --profile dev up -d        # + Writable codebase
docker-compose --profile full up -d       # + Dreamer daemon
```

## Environment Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `NEO4J_URI` | `bolt://memgraph:7687` | Graph database URI |
| `URP_ENABLED` | `1` | Enable/disable command wrapping |
| `URP_MAX_CONTEXT_TOKENS` | `4000` | Working memory budget |
| `DREAMER_CPU_THRESHOLD` | `15` | CPU% to activate dreamer |
| `DREAMER_PRUNE_DAYS` | `7` | Delete events older than N days |

## Output Formats

**Raw JSON (wasteful):**
```json
[{"id": 1, "name": "process"}, {"start": 1, "end": 2}...]
```

**LLM-friendly:**
```
module 'payments/processor.py' {
  @CALLS(Database.connect)
  def process_payment(amount: int) { ... }
}
```

## License

MIT
