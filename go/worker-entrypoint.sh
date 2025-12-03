#!/bin/bash
# URP Worker Container Entrypoint
# ================================
# Worker communicates with master via stdin/stdout protocol.
# Receives tasks, executes them, reports progress.

set -e

# ─────────────────────────────────────────────────────────────
# Environment Setup
# ─────────────────────────────────────────────────────────────

# Load .env if mounted
if [[ -f /etc/urp/.env ]]; then
    set -a
    source /etc/urp/.env
    set +a
fi

# Fix git "dubious ownership" for mounted workspace
git config --global --add safe.directory /workspace 2>/dev/null || true

# Worker ID from environment or hostname
export URP_WORKER_ID="${URP_WORKER_ID:-$(hostname)}"

# Generate session ID
if [[ -z "$URP_SESSION_ID" ]]; then
    export URP_SESSION_ID="w-$(date +%s)-$$"
fi

# Detect project name from workspace
if [[ "$URP_PROJECT" == "unknown" ]] && [[ -d /workspace/.git ]]; then
    URP_PROJECT=$(basename "$(git -C /workspace rev-parse --show-toplevel 2>/dev/null)" 2>/dev/null || echo "unknown")
    export URP_PROJECT
fi

# ─────────────────────────────────────────────────────────────
# Wait for Memgraph (if configured)
# ─────────────────────────────────────────────────────────────

if [[ -n "$NEO4J_URI" ]]; then
    max_attempts=15
    attempt=0

    while ! urp 2>&1 | grep -q "connected"; do
        attempt=$((attempt + 1))
        if [[ $attempt -ge $max_attempts ]]; then
            break
        fi
        sleep 1
    done
fi

# ─────────────────────────────────────────────────────────────
# Determine Capabilities
# ─────────────────────────────────────────────────────────────

CAPS=""

# Check for common tools
command -v go >/dev/null 2>&1 && CAPS="$CAPS,go"
command -v python3 >/dev/null 2>&1 && CAPS="$CAPS,python"
command -v node >/dev/null 2>&1 && CAPS="$CAPS,node"
command -v npm >/dev/null 2>&1 && CAPS="$CAPS,npm"
command -v make >/dev/null 2>&1 && CAPS="$CAPS,make"
command -v gcc >/dev/null 2>&1 && CAPS="$CAPS,gcc"
command -v git >/dev/null 2>&1 && CAPS="$CAPS,git"
command -v gh >/dev/null 2>&1 && CAPS="$CAPS,gh"

# Remove leading comma
CAPS="${CAPS#,}"
export URP_WORKER_CAPS="$CAPS"

# ─────────────────────────────────────────────────────────────
# Log to stderr (stdout is for protocol)
# ─────────────────────────────────────────────────────────────

log() {
    echo "[worker:$URP_WORKER_ID] $*" >&2
}

log "Starting worker"
log "  ID:      $URP_WORKER_ID"
log "  Project: $URP_PROJECT"
log "  Caps:    $CAPS"

# ─────────────────────────────────────────────────────────────
# Execute worker process
# ─────────────────────────────────────────────────────────────

# Worker run command reads from stdin, writes to stdout
# Master sends assign_task, worker responds with task_started, progress, complete
exec "$@"
