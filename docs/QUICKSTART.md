# URP Quick Start Guide

## Prerequisites

- Go 1.21+
- Docker or Podman
- API key (Anthropic or OpenRouter)

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

Create `~/.urp-go/.env`:

```bash
mkdir -p ~/.urp-go

# Option 1: Anthropic direct
echo "ANTHROPIC_API_KEY=sk-ant-..." > ~/.urp-go/.env

# Option 2: OpenRouter
cat > ~/.urp-go/.env << 'EOF'
OPENAI_API_KEY=sk-or-v1-...
OPENAI_BASE_URL=https://openrouter.ai/api/v1
URP_MODEL=anthropic/claude-sonnet-4
EOF
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

### Bug Fix

```bash
# 1. Launch master
urp launch .

# 2. Inside master, spawn worker
urp spawn

# 3. Send task to worker
urp ask urp-proj-w1 "fix the null pointer in auth.go line 42"

# 4. Review and cleanup
urp kill urp-proj-w1
```

### Code Review

```bash
# Check code quality
urp code ingest .
urp code cycles      # Circular dependencies
urp code dead        # Unused code
urp code hotspots    # High churn files
```

### Learning from Errors

```bash
# After solving a problem
urp think learn "Fixed CORS by adding AllowOrigins header"

# Later, when similar error occurs
urp think wisdom "CORS policy blocked"
# Returns the previous solution
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
