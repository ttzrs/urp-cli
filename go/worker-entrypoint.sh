#!/bin/bash
# URP Worker Container Entrypoint
# ================================
# Worker communicates with master via stdin/stdout protocol.
# Receives tasks, executes them, reports progress.

set -e

# ─────────────────────────────────────────────────────────────
# Docker Socket Permissions (for NeMo control)
# ─────────────────────────────────────────────────────────────

# Export health status for downstream checks
export URP_DOCKER_HEALTHY=false
export URP_DOCKER_MODE="none"

if [[ -S /var/run/docker.sock ]]; then
    DOCKER_GID=$(stat -c '%g' /var/run/docker.sock 2>/dev/null || echo "")
    if [[ -n "$DOCKER_GID" ]]; then
        # Create docker group with matching GID if needed
        if ! getent group docker >/dev/null 2>&1; then
            if ! groupadd -g "$DOCKER_GID" docker 2>/dev/null; then
                echo "[worker] WARN: Failed to create docker group with GID $DOCKER_GID" >&2
            fi
        fi
        # Add urp user to docker group
        if ! usermod -aG docker urp 2>/dev/null; then
            echo "[worker] WARN: Failed to add urp to docker group" >&2
        fi

        # VERIFY docker access actually works
        if docker ps >/dev/null 2>&1; then
            export URP_DOCKER_HEALTHY=true
            export URP_DOCKER_MODE="host"
            echo "[worker] Docker access: OK (host socket)" >&2
        else
            echo "[worker] WARN: Docker socket mounted but not accessible (SELinux?)" >&2
            echo "[worker] HINT: Try --security-opt label=disable or use :Z mount" >&2
        fi
    fi
else
    echo "[worker] INFO: No docker socket mounted (NeMo unavailable)" >&2
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
[[ "$URP_DOCKER_HEALTHY" == "true" ]] && CAPS="$CAPS,docker,nemo"

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
# Stay alive for master instructions via urp ask
# ─────────────────────────────────────────────────────────────

# Check if we have a TTY
if tty -s 2>/dev/null; then
    # Interactive mode - run command
    exec "$@"
else
    # Daemon mode - stay alive for urp ask commands
    log "Ready for instructions (daemon mode)"
    log "  Use: urp ask urp-$URP_PROJECT-w$URP_WORKER_ID \"<instruction>\""

    # Stay alive
    trap "exit 0" SIGTERM SIGINT
    while true; do
        sleep 86400
    done
fi
