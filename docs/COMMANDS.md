# URP Command Reference

Complete reference for all URP commands.

## Global Flags

```bash
--json      # Output as JSON
--pretty    # Pretty print output (default: true)
-h, --help  # Help for command
```

---

## Infrastructure Commands

### `urp doctor`

Check environment health.

```bash
urp doctor      # Quick status
urp doctor -v   # Verbose diagnostics
```

Output:
- Runtime detection (docker/podman)
- Network status
- Memgraph connectivity
- API key configuration
- Image availability

### `urp infra`

Infrastructure management.

```bash
urp infra start   # Create network, volumes, start memgraph
urp infra stop    # Stop all URP containers
urp infra clean   # Remove all URP resources
urp infra logs    # View container logs
urp infra status  # Show infrastructure status
```

### `urp status`

Show infrastructure status (alias for `urp infra status`).

```bash
urp status
```

---

## Container Orchestration

### `urp launch`

Launch master container for a project.

```bash
urp launch                    # Current directory
urp launch /path/to/project   # Specific path
urp launch --worker           # Standalone worker mode
```

Master container features:
- Read-only project mount
- Docker socket access (can spawn workers)
- Interactive Claude CLI session
- Auto-ingests code and git history

### `urp spawn`

Spawn worker container from master.

```bash
urp spawn       # Spawn one worker
urp spawn 3     # Spawn 3 workers
```

Worker containers:
- Read-write project access
- Claude CLI in daemon mode
- Dev tools installed
- Receives instructions via `urp ask`

### `urp workers`

List active worker containers.

```bash
urp workers
urp workers --json
```

### `urp kill`

Kill a worker container.

```bash
urp kill urp-proj-w1    # Kill specific worker
urp kill --all          # Kill all workers
```

### `urp ask`

Send prompt to worker's Claude CLI.

```bash
urp ask urp-proj-w1 "run tests and fix failures"
urp ask urp-proj-w1 "create branch feature-x"
```

### `urp exec`

Execute shell command in container.

```bash
urp exec urp-proj-w1 "pip install pytest"
urp exec urp-proj-w1 "go test ./..."
```

### `urp attach`

Attach to container shell.

```bash
urp attach urp-proj-w1
```

### `urp nemo`

NeMo GPU container control.

```bash
urp nemo start            # Start NeMo container
urp nemo exec "python train.py"
urp nemo stop             # Stop NeMo container
```

---

## Code Analysis

### `urp code ingest`

Parse code into graph.

```bash
urp code ingest .              # Current directory
urp code ingest /path/to/code  # Specific path
urp code ingest . --lang=go    # Filter by language
```

### `urp code deps`

Show function dependencies.

```bash
urp code deps "main.processRequest"
urp code deps "auth.Validate" --depth=2
```

### `urp code impact`

Show impact of changing a function.

```bash
urp code impact "db.Query"
```

### `urp code dead`

Find unused code.

```bash
urp code dead
urp code dead --threshold=0
```

### `urp code cycles`

Find circular dependencies.

```bash
urp code cycles
```

### `urp code hotspots`

Find high-churn files (frequently modified).

```bash
urp code hotspots
urp code hotspots --limit=20
```

### `urp code stats`

Show graph statistics.

```bash
urp code stats
```

---

## Git History

### `urp git ingest`

Load git history into graph.

```bash
urp git ingest .
urp git ingest . --limit=1000
```

### `urp git history`

Show file change timeline.

```bash
urp git history src/main.go
urp git history --author="john"
```

---

## Cognitive Memory

### `urp mem`

Session memory (ephemeral).

```bash
urp mem add "API uses OAuth2 with refresh tokens"
urp mem recall "authentication"
urp mem list
urp mem list --type=fact
urp mem stats
urp mem clear
urp mem clear --type=procedure
```

Memory types:
- `fact` - Static knowledge
- `procedure` - How to do something
- `episodic` - Past events
- `semantic` - Conceptual understanding

### `urp kb`

Knowledge base (persistent).

```bash
urp kb store "Always use context.WithTimeout for DB queries"
urp kb query "database timeout"
urp kb list
urp kb list --scope=global
urp kb reject <id> "outdated"
urp kb promote <id>           # Promote to global scope
urp kb stats
```

Scopes:
- `session` - Current session only
- `instance` - Current project
- `global` - All projects

### `urp think`

Cognitive skills.

```bash
urp think wisdom "connection refused"   # Find similar past solutions
urp think novelty "<code snippet>"      # Check if pattern is unusual
urp think learn "Fixed by adding retry" # Store successful solution
```

---

## Vector Search

### `urp vec`

Vector store operations.

```bash
urp vec stats
urp vec search "authentication middleware"
urp vec search "error handling" --limit=10
urp vec add "important note about caching"
```

---

## Focus (Context Loading)

### `urp focus`

Load focused context around a target.

```bash
urp focus "auth.Validate"          # Focus on function
urp focus "src/auth/"              # Focus on directory
urp focus "auth.Validate" -d 2     # With depth expansion
```

Context profiles by task type:
- `BUG_FIX`: Minimal context (~100 tokens)
- `REFACTOR`: Structure only (~500 tokens)
- `FEATURE`: Patterns + similar code (~1000 tokens)
- `DEBUG`: Errors + vitals (~200 tokens)

---

## OpenCode Sessions

### `urp oc session`

Session management.

```bash
urp oc session list
urp oc session new
urp oc session show <id>
urp oc session fork <id>
urp oc session delete <id>
```

### `urp oc msg`

Message operations.

```bash
urp oc msg list <session-id>
urp oc msg add <session-id> "message content"
```

### `urp oc usage`

Token usage statistics.

```bash
urp oc usage session <id>
urp oc usage total
```

---

## Spec-Driven Development

### `urp spec`

Spec-driven code generation.

```bash
urp spec init <name>      # Create spec template
urp spec list             # List available specs
urp spec show <name>      # Show spec contents
urp spec run <name>       # Execute spec with AI agent
urp spec status <name>    # Check spec status
```

Spec structure:
```
specs/
└── my-feature/
    └── spec.md
```

---

## Skills System

### `urp skill`

Skill management.

```bash
urp skill list
urp skill list --category=dev
urp skill show <name>
urp skill run <name>
urp skill run <name> --args="key=value"
urp skill search "testing"
urp skill add <path>
urp skill delete <name>
urp skill stats
urp skill categories
```

Skill categories:
- `dev` - Development tools
- `security` - Security testing
- `content` - Content generation
- `data` - Data processing
- `growth` - Analytics
- `business` - Business tools
- `core` - Core utilities

---

## Runtime Monitoring

### `urp sys`

System/runtime commands.

```bash
urp sys vitals    # Container CPU/RAM metrics
urp sys topology  # Network topology
urp sys health    # Container health issues
urp sys runtime   # Detected runtime (docker/podman)
```

### `urp events`

Terminal event commands.

```bash
urp events run "go test ./..."  # Execute and log command
urp events list                 # Recent commands
urp events list --limit=50
urp events errors               # Recent errors
urp events errors --since=1h
```

### `urp audit`

Audit logging.

```bash
urp audit status    # Audit system status
urp audit recent    # Recent audit entries
urp audit recent --limit=100
urp audit stats     # Audit statistics
```

### `urp alert`

System alerts.

```bash
urp alert list      # List active alerts
urp alert show <id> # Show alert details
urp alert ack <id>  # Acknowledge alert
urp alert clear     # Clear all alerts
```

---

## Backup & Restore

### `urp backup`

Knowledge backup and restore.

```bash
# Export
urp backup export                     # Export all
urp backup export -o backup.json      # Specify output file
urp backup export -t solutions        # Export only solutions
urp backup export -d "Weekly backup"  # Add description

# Import
urp backup import backup.json
urp backup import backup.json --merge # Merge with existing
urp backup import backup.json -t memories

# Inspect
urp backup list backup.json
urp backup stats
```

Types: `solutions`, `memories`, `knowledge`, `skills`, `sessions`, `vectors`, `all`

---

## Plan/Task Orchestration

### `urp plan`

Plan management.

```bash
urp plan create "Implement user authentication"
urp plan show
urp plan add "Create login endpoint"
urp plan status
urp plan complete <task-id>
```

### `urp orchestrate`

Multi-agent task orchestration.

```bash
urp orchestrate "Refactor auth module with tests"
```

---

## Interactive UI

### `urp tui`

Launch Bubble Tea terminal UI.

```bash
urp tui
```

Features:
- Session list
- Message history
- Tool output
- Token usage
- Keyboard shortcuts

---

## Utility Commands

### `urp version`

Show version.

```bash
urp version
```

### `urp completion`

Generate shell completions.

```bash
urp completion bash > /etc/bash_completion.d/urp
urp completion zsh > ~/.zsh/completions/_urp
urp completion fish > ~/.config/fish/completions/urp.fish
```

### `urp help`

Get help.

```bash
urp help
urp help code
urp code --help
urp code ingest --help
```
