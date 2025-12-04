# URP: Embodied Agent Protocol

**Cognitive infrastructure for AI coding agents.** Graph memory + vector search + container orchestration.

```
AXIOM: context_window ⊂ memory_total
       memory_total = context ∪ graph_db ∪ vector_store
```

## What is URP?

URP extends AI agents with persistent memory and structured perception:

- **Graph Database (Memgraph)**: Code relationships, git history, solutions
- **Vector Store (LanceDB)**: Semantic search over code and memories
- **Container Orchestration**: Master/Worker architecture for safe code execution
- **Spec-Driven Development**: AI generates code from specifications

## Quick Start

```bash
# Build
cd go && go build -o urp ./cmd/urp

# Check environment
./urp doctor

# Start infrastructure
./urp infra start

# Launch interactive session
./urp launch /path/to/project
```

## Architecture

```
┌─────────────────────────────────────────────────────────────────┐
│                        HOST MACHINE                              │
└──────────────────────────────┬──────────────────────────────────┘
                               │
        ┌──────────────────────┼──────────────────────┐
        │                      │                      │
        ▼                      ▼                      ▼
┌───────────────┐    ┌─────────────────┐    ┌───────────────────┐
│  urp-memgraph │    │  urp-master     │    │  urp-worker-N     │
│  (graph db)   │◄───│  (read-only)    │───►│  (read-write)     │
│  bolt:7687    │    │  Claude CLI     │    │  Claude CLI       │
└───────────────┘    └─────────────────┘    └───────────────────┘
```

**Master/Worker Flow:**
1. Master container has read-only access to project
2. Master spawns workers for write operations
3. Workers execute tasks and report back
4. All operations logged to graph database

## Commands

### Infrastructure

```bash
urp doctor              # Check environment health
urp infra start         # Start network, memgraph, volumes
urp infra stop          # Stop all containers
urp infra clean         # Remove all resources
urp status              # Show infrastructure status
```

### Container Orchestration

```bash
urp launch <path>       # Launch master container (interactive)
urp spawn               # Spawn worker from master
urp workers             # List active workers
urp kill <name>         # Kill worker container
urp ask <worker> "msg"  # Send prompt to worker's Claude
urp exec <worker> "cmd" # Execute shell command in worker
urp attach <container>  # Attach to container shell
```

### Code Analysis

```bash
urp code ingest <path>  # Parse code into graph
urp code deps <func>    # Show function dependencies
urp code impact <func>  # Show change impact
urp code dead           # Find unused code
urp code cycles         # Find circular dependencies
urp code hotspots       # High churn files
urp code stats          # Graph statistics
```

### Git History

```bash
urp git ingest <path>   # Load git history into graph
urp git history <file>  # File change timeline
```

### Cognitive Memory

```bash
# Session Memory (ephemeral)
urp mem add <text>      # Remember a note
urp mem recall <query>  # Search memories
urp mem list            # List all memories
urp mem clear           # Clear session

# Knowledge Base (persistent)
urp kb store <text>     # Store knowledge
urp kb query <text>     # Search knowledge
urp kb list             # List all
urp kb promote <id>     # Promote to global scope
urp kb reject <id>      # Mark as not applicable

# Cognitive Skills
urp think wisdom <err>  # Find similar past errors
urp think novelty <code># Check if pattern is unusual
urp think learn <desc>  # Store successful solution
```

### Vector Search

```bash
urp vec stats           # Vector store statistics
urp vec search <query>  # Semantic search
urp vec add <text>      # Add to vector store
```

### OpenCode Sessions

```bash
urp oc session list     # List sessions
urp oc session new      # Create new session
urp oc session show <id># Show session details
urp oc msg list <id>    # List messages in session
urp oc usage total      # Token usage stats
```

### Spec-Driven Development

```bash
urp spec init <name>    # Create spec template
urp spec list           # List available specs
urp spec run <name>     # Execute spec with AI agent
urp spec status <name>  # Check spec status
```

### Skills System

```bash
urp skill list          # List all skills
urp skill show <name>   # Show skill details
urp skill run <name>    # Execute skill
urp skill categories    # List skill categories
urp skill search <q>    # Search skills
```

### Runtime Monitoring

```bash
urp sys vitals          # Container CPU/RAM metrics
urp sys topology        # Network topology
urp sys health          # Container health
urp sys runtime         # Detected runtime (docker/podman)

urp events run <cmd>    # Execute and log command
urp events list         # Recent commands
urp events errors       # Recent errors

urp audit status        # Audit system status
urp audit recent        # Recent audit entries
urp audit stats         # Audit statistics
```

### Backup & Restore

```bash
urp backup export -o backup.json    # Export all knowledge
urp backup import backup.json       # Import knowledge
urp backup list backup.json         # List contents
urp backup stats                    # Backup statistics
```

### Interactive TUI

```bash
urp tui                 # Launch Bubble Tea terminal UI
```

## PRU Primitives

The perception model based on 7 primitives:

| Symbol | Name | Description |
|--------|------|-------------|
| **D** | Domain | Entity existence: File, Function, Class, Container |
| **τ** | Temporal | Sequence: Commits, Events, Commands |
| **Φ** | Morphism | Causal flow: Calls, Data, Energy, ExitCode |
| **⊆** | Inclusion | Hierarchy: File→Func, Class→Method, Net→Container |
| **⊥** | Orthogonal | Conflicts: DeadCode, Cycles, Errors, Failures |
| **P** | Projective | Viewpoint: Interface, Implementation |
| **T** | Tensor | Context: Branch, Env, Session |

## Graph Schema

```cypher
// Nodes
(:File {path, hash, language})
(:Function {name, signature, complexity})
(:Class {name, methods})
(:Commit {hash, message, author, timestamp})
(:Container {name, image, status})
(:Memory {content, type, created_at})
(:Solution {problem, solution, effectiveness})

// Relationships
-[:CONTAINS]->      // File contains Function
-[:CALLS]->         // Function calls Function
-[:FLOWS_TO]->      // Data flow
-[:PARENT_OF]->     // Git parent commit
-[:TOUCHED]->       // Commit touched File
-[:RESOLVES]->      // Solution resolves problem
```

## Environment Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `NEO4J_URI` | `bolt://localhost:7687` | Memgraph connection |
| `URP_PROJECT` | auto-detected | Project name |
| `URP_SESSION_ID` | auto-generated | Session identifier |
| `URP_RUNTIME` | auto-detected | Force docker/podman |
| `ANTHROPIC_API_KEY` | - | Claude API key |
| `OPENAI_API_KEY` | - | OpenAI/OpenRouter key |
| `OPENAI_BASE_URL` | - | OpenAI-compatible endpoint |
| `URP_MODEL` | - | Override model selection |

## Configuration

Store credentials in `~/.urp-go/.env`:

```bash
ANTHROPIC_API_KEY=sk-ant-...
# Or for OpenRouter:
OPENAI_API_KEY=sk-or-v1-...
OPENAI_BASE_URL=https://openrouter.ai/api/v1
URP_MODEL=anthropic/claude-sonnet-4
```

## Safety System

Deterministic pre-execution filter (not AI guessing):

| Blocked | Alternative |
|---------|-------------|
| `rm -rf /` | Use specific path |
| `git push --force` | Use `--force-with-lease` |
| `git add .env` | Add to `.gitignore` |
| `DROP DATABASE` | Requires approval |
| `mkfs` | Filesystem destruction blocked |

## Building

```bash
cd go
go build -o urp ./cmd/urp
go test ./...
```

## Docker Images

- `urp:latest` - Base image for standalone use
- `urp:master` - Master container with docker socket access
- `urp:worker` - Worker container with dev tools

Build images:
```bash
cd docker
./build.sh
```

## Project Structure

```
go/
├── cmd/urp/           # CLI entry point
├── internal/
│   ├── container/     # Docker/Podman orchestration
│   ├── graph/         # Memgraph driver
│   ├── vector/        # LanceDB integration
│   ├── cognitive/     # Wisdom, novelty, learning
│   ├── memory/        # Session + knowledge store
│   ├── opencode/      # AI agent system
│   │   ├── agent/     # Agent executor
│   │   ├── provider/  # LLM providers (Anthropic, OpenAI, Google)
│   │   ├── tool/      # Agent tools (bash, read, write, etc.)
│   │   └── session/   # Session management
│   ├── spec/          # Spec-driven development
│   ├── skill/         # Skills system
│   ├── audit/         # Audit logging
│   └── tui/           # Bubble Tea UI
└── docs/              # Documentation
```

## Performance

| Metric | Value |
|--------|-------|
| Startup | ~6ms |
| Binary size | ~25MB |
| Memory (idle) | ~15MB |

## License

MIT

## Links

- [GitHub](https://github.com/ttzrs/urp-cli)
- [Issues](https://github.com/ttzrs/urp-cli/issues)
