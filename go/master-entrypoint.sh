#!/bin/bash
# URP Master Container Entrypoint
# ================================
# 1. Setup environment
# 2. Wait for memgraph
# 3. Auto-ingest code + git
# 4. Launch Claude CLI

set -e

# ─────────────────────────────────────────────────────────────
# Docker Socket Permissions (for urp spawn)
# ─────────────────────────────────────────────────────────────

# Match docker group GID to host socket GID
if [[ -S /var/run/docker.sock ]]; then
    DOCKER_GID=$(stat -c '%g' /var/run/docker.sock 2>/dev/null || echo "")
    if [[ -n "$DOCKER_GID" ]]; then
        # Create docker group with matching GID if needed
        if ! getent group docker >/dev/null 2>&1; then
            groupadd -g "$DOCKER_GID" docker 2>/dev/null || true
        fi
        # Add urp user to docker group
        usermod -aG docker urp 2>/dev/null || true
    fi
fi

# ─────────────────────────────────────────────────────────────
# Environment Setup
# ─────────────────────────────────────────────────────────────

# Load .env if mounted
if [[ -f /etc/urp/.env ]]; then
    set -a
    source /etc/urp/.env
    set +a
fi

# ─────────────────────────────────────────────────────────────
# Claude Configuration (hooks for alert injection)
# ─────────────────────────────────────────────────────────────

# Setup Claude config directory
CLAUDE_CONFIG_DIR="/home/urp/.claude"
mkdir -p "$CLAUDE_CONFIG_DIR/hooks"

# Copy settings if not exists
if [[ -f /etc/urp/claude-settings.json ]] && [[ ! -f "$CLAUDE_CONFIG_DIR/settings.json" ]]; then
    cp /etc/urp/claude-settings.json "$CLAUDE_CONFIG_DIR/settings.json"
fi

# Ensure alert hook is accessible
if [[ -f /etc/urp/claude-alert-hook.sh ]]; then
    cp /etc/urp/claude-alert-hook.sh "$CLAUDE_CONFIG_DIR/hooks/"
    chmod +x "$CLAUDE_CONFIG_DIR/hooks/claude-alert-hook.sh"
fi

chown -R urp:urp "$CLAUDE_CONFIG_DIR"

# Set alert directory environment
export URP_ALERT_DIR="/var/lib/urp/alerts"

# Fix git "dubious ownership" for mounted workspace
git config --global --add safe.directory /workspace 2>/dev/null || true

# Generate session ID if not provided
if [[ -z "$URP_SESSION_ID" ]]; then
    export URP_SESSION_ID="s-$(date +%s)-$$"
fi

# Detect project name from workspace
if [[ "$URP_PROJECT" == "unknown" ]] && [[ -d /workspace/.git ]]; then
    URP_PROJECT=$(basename "$(git -C /workspace rev-parse --show-toplevel 2>/dev/null)" 2>/dev/null || echo "unknown")
    export URP_PROJECT
fi

# ─────────────────────────────────────────────────────────────
# Wait for Memgraph
# ─────────────────────────────────────────────────────────────

if [[ -n "$NEO4J_URI" ]]; then
    echo "⏳ Waiting for Memgraph..."
    max_attempts=30
    attempt=0

    while ! urp 2>&1 | grep -q "connected"; do
        attempt=$((attempt + 1))
        if [[ $attempt -ge $max_attempts ]]; then
            echo "⚠️  Warning: Could not connect to Memgraph after $max_attempts attempts"
            break
        fi
        sleep 1
    done

    if [[ $attempt -lt $max_attempts ]]; then
        echo "✓ Memgraph connected"
    fi
fi

# Initialize session in graph
urp session id 2>/dev/null || true

# ─────────────────────────────────────────────────────────────
# Auto-Ingest (Git first, then Code)
# ─────────────────────────────────────────────────────────────

echo ""
echo "═══════════════════════════════════════════════════════"
echo " URP Master - Ingesting Project"
echo "═══════════════════════════════════════════════════════"
echo " Project: $URP_PROJECT"
echo " Session: $URP_SESSION_ID"
echo ""

# Git first - establishes temporal context and file existence
if [[ -d /workspace/.git ]]; then
    echo "📜 Ingesting git history..."
    if urp git ingest /workspace 2>&1; then
        echo "✓ Git history ingested"
    else
        echo "⚠️  Git ingest had issues (continuing)"
    fi
fi

# Code second - parses AST, links to existing files from git
echo "📂 Ingesting code structure..."
if urp code ingest /workspace 2>&1; then
    echo "✓ Code structure ingested"
else
    echo "⚠️  Code ingest had issues (continuing)"
fi

# Show stats
echo ""
urp code stats 2>/dev/null || true
echo ""

# ─────────────────────────────────────────────────────────────
# Launch Claude CLI
# ─────────────────────────────────────────────────────────────

echo "═══════════════════════════════════════════════════════"
echo " Starting Claude Code"
echo "═══════════════════════════════════════════════════════"
echo ""
echo "Commands available:"
echo "  urp spawn           - Create worker for code changes"
echo "  urp workers         - List active workers"
echo "  urp plan show       - View current plan"
echo "  pain                - Recent errors"
echo "  wisdom <error>      - Find similar past errors"
echo "  learn <desc>        - Record successful solution"
echo ""
echo "───────────────────────────────────────────────────────"
echo ""

# Check if we have a prompt to execute (batch mode via env var)
if [[ -n "$URP_PROMPT" ]]; then
    echo "[BATCH MODE] Executing prompt..."
    exec claude --print "$URP_PROMPT"
fi

# Check if we have a TTY (using tty command which works in containers)
if tty -s 2>/dev/null; then
    # Interactive mode - run Claude CLI
    exec "$@"
else
    # Daemon mode (no TTY) - stay alive for urp ask/exec commands
    echo "[DAEMON MODE] Master ready for commands"
    echo "  Use: urp ask urp-master-$URP_PROJECT \"<prompt>\""
    echo "  Or:  urp exec urp-master-$URP_PROJECT \"<command>\""
    echo ""

    # Stay alive - trap signals for clean shutdown
    trap "exit 0" SIGTERM SIGINT
    while true; do
        sleep 86400
    done
fi
