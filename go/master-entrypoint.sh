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

# Launch URP Master Agent
# ─────────────────────────────────────────────────────────────

echo "═══════════════════════════════════════════════════════"
echo " Starting URP Master Agent"
echo "═══════════════════════════════════════════════════════"
echo ""

# Debugging: show the resolved model ID
if [[ -f "/tmp/agent_model.log" ]]; then
    echo "Resolved Agent Model: $(cat /tmp/agent_model.log)"
fi

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
    exec urp ask "$URP_PROJECT" "$URP_PROMPT" # Assuming urp ask is main way to interact
fi

# Check if we have a TTY (using tty command which works in containers)
if tty -s 2>/dev/null; then
    # Interactive mode - run bash shell
    exec "$@"
else
    # Daemon mode (no TTY) - stay alive for urp ask/exec commands
    echo "[DAEMON MODE] Master ready for commands"
    echo "  Use: urp ask urp-opencode-master \"<prompt>\"" # Updated container name
    echo "  Or:  urp exec urp-opencode-master \"<command>\""
    echo ""

    # Stay alive - trap signals for clean shutdown
    trap : SIGTERM SIGINT # Trap signals, but do nothing (sleep will be interrupted)
    sleep infinity # Keep container alive for debugging
fi
