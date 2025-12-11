# URP: Universal Robotic Programmer (V2)

**Cognitive infrastructure for AI coding agents.** Graph memory + vector search + container orchestration + adaptive learning.

```
AXIOM: context_window ⊂ memory_total
       memory_total = context ∪ graph_db ∪ vector_store ∪ learned_strategies
```

## What is URP?

URP extends AI agents with persistent memory, structured perception, adaptive learning, and secure execution:

-   **Context as Compiled View (V2)**: Prompts are dynamically compiled from graph state and gated logs with noise filtering.
-   **Adaptive Context Optimization**: 5 context modes (Full, Focused, Minimal, Delta, Memory) with automatic token budget management.
-   **Graph Database (Memgraph)**: Code relationships, git history, solutions, and task dependencies.
-   **Vector Store (LanceDB)**: Semantic search over code and **Learned Strategies** (End of Cycle).
-   **Container Orchestration**: Master/Worker architecture for safe code execution (Docker).
-   **Dual-LLM Pipeline**: Cheap/Fast Gate LLM (e.g., Qwen/GLM) filters noise; Smart Master LLM (e.g., Claude/DeepSeek) reasons.
-   **Learning Loop**: Agents learn from successful strategies and adapt behavior for future tasks.
-   **Advanced TUI**: Bubble Tea powered interface with cognitive state visualization, debugging panel, and Vim-style navigation.

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

## Architecture V2: Context Compiler Architecture

```
┌─────────────────────────────────────────────────────────────────────────┐
│                          HOST MACHINE                                   │
└─────────────────────────────────────────────────────────────────────────┘
                                  │
        ┌─────────────────────────┼─────────────────────────┐
        │                         │                         │
        ▼                         ▼                         ▼
┌─────────────────┐    ┌──────────────────────┐    ┌─────────────────────┐
│  urp-memgraph   │    │  urp-master (TUI)    │    │  urp-worker-N       │
│  (Graph DB)     │◄───│  ┌─────────────────┐ │───►│  (Remote Exec)      │
│  bolt:7687      │    │  │ Context         │ │    │  ┌─────────────────┐│
│  - Code rels    │    │  │ Compiler        │ │    │  │ Bash            ││
│  - Git history  │    │  │ Gate LLM        │ │    │  │ Sandbox         ││
│  - Solutions    │    │  │ Learning Agent  │ │    │  │ + Custom tools  ││
└─────────────────┘    │  └─────────────────┘ │    │  └─────────────────┘│
                       └───────────────────────┘    └─────────────────────┘
```

**Key Components:**
1.  **Context Compiler:** Dynamically compiles prompts from graph state, gated logs, and learned strategies. Creates optimized context views based on task requirements.
2.  **Adaptive Context Optimization:** 5 context modes with automatic token budget management:
    *   **ModeFull:** Full context for exploration
    *   **ModeFocused:** Current file + dependencies
    *   **ModeMinimal:** Minimal function context
    *   **ModeDelta:** Changes + surrounding context
    *   **ModeMemory:** Rely on compressed state
3.  **Dual LLM Pipeline:**
    *   **Gate LLM:** Fast noise filtering (Qwen/GLM)
    *   **Master LLM:** Reasoning and decision making (Claude/DeepSeek)
4.  **Learning Loop (Empiricist):**
    *   **Pre-Task:** Retrieves similar successful strategies from LanceDB and Vector Store.
    *   **Post-Task:** Extracts, rates, and stores strategies for future use with similarity embeddings.
5.  **Cognitive TUI:** Advanced Bubble Tea interface with:
    *   Brain Monitor: Real-time cognitive state visualization
    *   Debug Panel: LLM calls, tool usage, permissions
    *   Vim-style navigation and search
    *   Tool call visualization and expansion
6.  **Secure Execution:** Dangerous tools (`bash`, `sandbox`) are intercepted by `RemoteExecutor` and run inside ephemeral Docker workers.

## Commands

### Interactive Interface
```bash
urp                    # Start interactive TUI session
urp tui                # Launch advanced TUI interface
urp --tui              # Alternative TUI launch
```

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

## Environment Variables for LLM Configuration

URP's LLMs are configured via environment variables. For a full template and detailed explanations, refer to the `.env.example` file.

| Variable | Description | Default (if not set) |
|---|---|---|
| `ANTHROPIC_API_KEY` | API key for Anthropic Claude models. | - |
| `OPENAI_API_KEY` | API key for OpenAI-compatible services (e.g., GPT, OpenRouter for Gemini/GLM). | - |
| `DEEPSEEK_API_KEY` | API key for DeepSeek models. | - |
| `ANTHROPIC_BASE_URL` | Custom base URL for Anthropic API. | `https://api.anthropic.com/v1` |
| `OPENAI_BASE_URL` | Custom base URL for OpenAI-compatible API. | `https://api.openai.com/v1` |
| `URP_MASTER_MODEL` | Specifies the LLM for the Master (reasoning/planning) role. | `anthropic/claude-sonnet-4-5-20250929` |
| `URP_GATE_MODEL` | Specifies the LLM for the Gate (noise filtering) role. | `gpt-4o-mini` |
| `NEO4J_URI` | Memgraph connection URI. | `bolt://localhost:7687` |
| `NEO4J_USER` | Memgraph username. | - |
| `NEO4J_PASSWORD` | Memgraph password. | - |
| `URP_PROJECT` | Current project name. | auto-detected |
| `URP_SESSION_ID` | Current session identifier. | - |
| `URP_THINKING` | Thinking budget (e.g., in tokens or turns). | - |

## Configuration Example

Store your credentials and preferred models in `~/.urp-go/.env`:

```
# Example ~/.urp-go/.env content
OPENAI_API_KEY=sk-or-v1-...
OPENAI_BASE_URL=https://openrouter.ai/api/v1
URP_MASTER_MODEL=zai-glm-4.6 # Master LLM via OpenRouter
URP_GATE_MODEL=qwen3-coder-flash # Gate LLM via OpenRouter
ANTHROPIC_API_KEY=sk-ant-...
NEO4J_URI=bolt://localhost:7687
```

URP will automatically load `~/.urp-go/.env` if it exists.

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
