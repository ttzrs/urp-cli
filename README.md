# URP: Universal Robotic Programmer (V2)

**Cognitive infrastructure for AI coding agents.** Graph memory + vector search + container orchestration.

```
AXIOM: context_window ⊂ memory_total
       memory_total = context ∪ graph_db ∪ vector_store
```

## What is URP?

URP extends AI agents with persistent memory, structured perception, and secure execution:

- **Context as Compiled View (V2)**: Prompts are dynamically compiled from graph state and gated logs.
- **Graph Database (Memgraph)**: Code relationships, git history, solutions.
- **Vector Store (LanceDB)**: Semantic search over code and **Learned Strategies** (End of Cycle).
- **Container Orchestration**: Master/Worker architecture for safe code execution (Docker).
- **Dual-LLM Pipeline**: Cheap/Fast Gate model (e.g., Qwen/GLM) filters noise; Smart Master (e.g., Claude/DeepSeek) reasons.

## Quick Start

```bash
# Build
cd go && go build -o urp ./cmd/urp

# Check environment
./urp doctor

# Start infrastructure
./urp infra start

# Launch interactive session (TUI)
./urp
```

## Architecture V2

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
│  (graph db)   │◄───│  (Context Comp) │───►│  (Remote Exec)    │
│  bolt:7687    │    │  Gate + Agent   │    │  Bash / Sandbox   │
└───────────────┘    └─────────────────┘    └───────────────────┘
```

**Key Components:**
1.  **Context Compiler:** Renders a noise-free prompt using Memgraph state and a "Gate" LLM to filter logs.
2.  **Learning Loop (Empiricist):**
    *   **Pre-Task:** Retrieves similar successful strategies from LanceDB.
    *   **Post-Task:** Extracts and stores the strategy (Success/Failure) for future use.
3.  **Secure Execution:** Dangerous tools (`bash`, `sandbox`) are intercepted by `RemoteExecutor` and run inside ephemeral Docker workers.

## Commands

### Infrastructure

```bash
urp doctor              # Check environment health
urp infra start         # Start network, memgraph, volumes
urp status              # Show infrastructure status
```

### Core Agent

```bash
urp                     # Launch interactive TUI agent (Auto-bootstrap)
urp compile --goal ".." # Debug Context Compiler output
```

### Container Orchestration

```bash
urp spawn               # Spawn worker manually
urp workers             # List active workers
urp kill <name>         # Kill worker container
```

### Code Analysis

```bash
urp code ingest <path>  # Parse code into graph
urp code deps <func>    # Show function dependencies
urp code stats          # Graph statistics
```

### Cognitive Memory

```bash
# Session Memory (ephemeral)
urp mem add <text>      # Remember a note
urp mem recall <query>  # Search memories

# Knowledge Base (persistent)
urp think wisdom <err>  # Find similar past errors
urp think learn <desc>  # Store successful solution manually
```

## Environment Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `NEO4J_URI` | `bolt://localhost:7687` | Memgraph connection |
| `URP_PROJECT` | auto-detected | Project name |
| `ANTHROPIC_API_KEY` | - | Claude API key |
| `OPENAI_API_KEY` | - | OpenAI/OpenRouter key |
| `OPENAI_BASE_URL` | - | OpenAI-compatible endpoint |
| `MODEL_GATE` | `zai-glm-4.6` | Fast model for noise filtering |
| `MODEL_MASTER` | `zai-glm-4.6` | Smart model for reasoning |

## Configuration

Store credentials in `~/.urp-go/.env`:

```bash
OPENAI_API_KEY=sk-or-v1-...
OPENAI_BASE_URL=https://openrouter.ai/api/v1
MODEL_GATE=qwen-turbo
URP_MODEL=anthropic/claude-3.5-sonnet
```

## Building

```bash
cd go
go build -o urp ./cmd/urp
go test ./...
```

## Project Structure

```
go/
├── cmd/urp/           # CLI entry point
├── internal/
│   ├── bootstrap/     # App initialization & wiring (SRP)
│   ├── compiler/      # Context Compiler (V2)
│   ├── gate/          # Noise Filter (LLM-based)
│   ├── orchestrator/  # Master-Worker logic
│   ├── graph/         # Memgraph driver
│   ├── vector/        # LanceDB integration
│   ├── opencode/      # AI agent system
│   │   ├── agent/     # Agent executor & Learning
│   │   ├── provider/  # LLM providers
│   │   ├── tool/      # Tools (Bash, Sandbox, Graph)
│   └── tui/           # Bubble Tea UI
└── docs/              # Documentation
```

## License

MIT

## Links

- [GitHub](https://github.com/ttzrs/urp-cli)