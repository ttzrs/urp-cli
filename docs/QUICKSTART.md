# URP Quick Start Guide

## Prerequisites

- Go 1.24.0+
- Docker or Podman
- API key for at least one LLM provider (see Configuration below)

## Installation

```bash
# Clone repository
git clone https://github.com/ttzrs/urp-cli
cd urp-cli

# Build
cd go && go build -o urp ./cmd/urp

# Add to PATH (optional)
sudo ln -s $(pwd)/urp /usr/local/bin/urp
```

## Configuration

Create `~/.urp-go/.env` with your API keys. URP supports multiple LLM providers:

### Supported Providers

| Provider | API Key Environment | Base URL Environment | Models |
|----------|-------------------|----------------------|--------|
| **Anthropic** | `ANTHROPIC_API_KEY` | (none) | claude-opus, claude-sonnet, claude-haiku |
| **OpenAI Direct** | `OPENAI_API_KEY` | (none) | gpt-4o, gpt-4-turbo, gpt-3.5-turbo |
| **OpenRouter** | `OPENAI_API_KEY` | `OPENAI_BASE_URL=https://openrouter.ai/api/v1` | All OpenRouter models |
| **DeepSeek** | `DEEPSEEK_API_KEY` | (none) | deepseek-chat, deepseek-coder |
| **Google Gemini** | `GOOGLE_API_KEY` | (none) | gemini-2.0-flash, gemini-1.5-pro |

### Setup Examples

**Option 1: Anthropic (Recommended)**
```bash
mkdir -p ~/.urp-go
echo "ANTHROPIC_API_KEY=sk-ant-v0-..." > ~/.urp-go/.env
```

**Option 2: OpenRouter (Multi-model)**
```bash
mkdir -p ~/.urp-go
cat > ~/.urp-go/.env << 'EOF'
OPENAI_API_KEY=sk-or-v1-...
OPENAI_BASE_URL=https://openrouter.ai/api/v1
EOF
```

**Option 3: Multiple Providers (Fallback)**
```bash
mkdir -p ~/.urp-go
cat > ~/.urp-go/.env << 'EOF'
# Primary: Anthropic
ANTHROPIC_API_KEY=sk-ant-...

# Fallback: OpenAI
OPENAI_API_KEY=sk-...

# Optional: DeepSeek
DEEPSEEK_API_KEY=sk-...
EOF
```

### Model Selection

Set default model (optional):
```bash
# Add to ~/.urp-go/.env
URP_MODEL=claude-opus-4  # Anthropic
# OR
URP_MODEL=gpt-4o         # OpenAI (requires OPENAI_API_KEY)
```

List available models:
```bash
urp models
urp models --provider anthropic
```

## First Run

```bash
# 1. Check environment
urp doctor

# Expected output:
# ✓ Runtime: docker
# ✓ Network: urp-network exists
# ✓ Memgraph: running
# ✓ API Key: configured

# 2. If not healthy, start infrastructure
urp infra start

# 3. Launch session on your project
urp launch /path/to/your/project
```

## Basic Usage

### Interactive Mode

```bash
# Start interactive Claude session
urp launch .

# Inside the session, Claude has access to:
# - Read-only project files
# - URP cognitive tools
# - Spawn workers for writes
```

### Standalone Commands

```bash
# Analyze code
urp code ingest .
urp code stats
urp code hotspots

# Search memories
urp think wisdom "connection refused"
urp kb query "authentication"

# Monitor
urp sys vitals
urp events errors
```

### Spec-Driven Development

```bash
# Create a spec
mkdir -p specs/my-feature
cat > specs/my-feature/spec.md << 'EOF'
# Specification: My Feature

## Overview
Description of what to build.

## Requirements
1. First requirement
2. Second requirement

## Tests Required
- TestFeature_Basic
- TestFeature_EdgeCase
EOF

# Run the spec (AI generates code)
urp spec run my-feature
```

## Common Workflows

### Master/Worker Architecture

URP uses a **master/worker pattern** for task execution:

- **Master**: Reads code, analyzes, plans (read-only mount)
- **Worker**: Implements changes, runs tests, commits (read-write mount)
- **Communication**: Master sends instructions to worker via `urp ask`

```
┌──────────────────┐
│   Master (RO)    │
│ - Analyzes code  │
│ - Plans tasks    │
│ - Claude CLI     │
└────────┬─────────┘
         │ urp ask
         ▼
┌──────────────────┐
│  Worker (RW)     │
│ - Implements     │
│ - Tests          │
│ - Commits        │
└──────────────────┘
```

### Bug Fix

Complete workflow for fixing a bug:

```bash
# 1. Launch master in read-only mode
urp launch /path/to/project

# Master outputs:
# ✓ Master container: urp-proj-m1
# ✓ Code ingested: 523 files, 42k LOC
# ✓ Git history: 187 commits

# 2. Inside master shell, spawn worker
urp spawn

# Worker created: urp-proj-w1 (read-write access)

# 3. Analyze before fixing
urp code hotspots
urp think wisdom "null pointer exception in auth.go"

# 4. Send task to worker
urp ask urp-proj-w1 "fix the null pointer in auth.go line 42. run tests. commit."

# Worker execution steps:
# - Claude analyzes the bug
# - Creates feature branch
# - Applies fix
# - Runs `go test ./internal/auth/`
# - Commits and reports status

# 5. Review output from worker

# 6. Clean up
urp kill urp-proj-w1
urp workers   # Verify worker is gone
```

### Multi-Task Feature Development

Spawn multiple workers for parallel development:

```bash
# 1. Master mode
urp launch .

# 2. Spawn 3 parallel workers
urp spawn 3

# Creates: urp-proj-w1, urp-proj-w2, urp-proj-w3

# 3. Assign tasks in parallel
urp ask urp-proj-w1 "implement authentication module. run tests."
urp ask urp-proj-w2 "implement API routes. run tests."
urp ask urp-proj-w3 "implement database models. run tests."

# All 3 workers execute in parallel!

# 4. Monitor workers
urp workers

# 5. When done, clean up
urp kill urp-proj-w1 urp-proj-w2 urp-proj-w3
```

### Code Review

```bash
# Check code quality before fixing
urp code ingest .
urp code cycles      # Circular dependencies
urp code dead        # Unused code
urp code hotspots    # High churn files

# Example: Find files to refactor
urp code stats       # Overall metrics
urp code deps        # Dependency graph
```

### Learning from Errors

```bash
# After solving a problem, capture the solution
urp think learn "Fixed CORS by adding AllowOrigins header to middleware"

# Later, when similar error occurs
urp think wisdom "CORS policy blocked"

# Output: Previous solution with context
```

### Spec-Driven Development

Define requirements, let Claude implement:

```bash
# 1. Create specification
mkdir -p specs/auth
cat > specs/auth/spec.md << 'EOF'
# OAuth 2.0 Integration

## Requirements
1. Support Google OAuth
2. Store user tokens securely
3. Refresh expired tokens
4. Add /auth/callback endpoint

## Tests Required
- TestOAuth_GoogleFlow
- TestOAuth_TokenRefresh
- TestOAuth_InvalidToken
EOF

# 2. Spawn worker and execute spec
urp spawn
urp ask urp-proj-w1 "implement the spec in specs/auth/spec.md. run tests."

# Worker:
# 1. Reads spec.md
# 2. Generates code based on requirements
# 3. Runs tests
# 4. Reports compliance

# 3. Review and merge
urp kill urp-proj-w1
```

## Troubleshooting

### "No container runtime found"

```bash
# Install Docker
sudo dnf install docker  # Fedora
sudo apt install docker.io  # Ubuntu

# Or use Podman
sudo dnf install podman
```

### "API key not found"

```bash
# Verify .env file
cat ~/.urp-go/.env

# Should contain one of:
# ANTHROPIC_API_KEY=sk-ant-...
# OPENAI_API_KEY=sk-or-v1-...
```

### "Memgraph not running"

```bash
urp infra start
# Or manually:
docker run -d --name urp-memgraph -p 7687:7687 memgraph/memgraph
```

### "Permission denied on docker socket"

```bash
# Add user to docker group
sudo usermod -aG docker $USER
newgrp docker
```

## Next Steps

- Read [ARCHITECTURE.md](ARCHITECTURE.md) for system design
- Read [COMMANDS.md](COMMANDS.md) for full command reference
- Check [CLAUDE.md](../CLAUDE.md) for AI agent instructions
