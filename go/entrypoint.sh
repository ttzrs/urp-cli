#!/bin/bash
# URP Container Entrypoint
# ========================

set -e

# Fix git "dubious ownership" for mounted workspace
git config --global --add safe.directory /workspace 2>/dev/null || true

# Generate session ID if not provided
if [[ -z "$URP_SESSION_ID" ]]; then
    export URP_SESSION_ID="s-$(date +%s)-$$"
fi

# Detect project name from workspace if not set
if [[ "$URP_PROJECT" == "unknown" ]] && [[ -d /workspace/.git ]]; then
    URP_PROJECT=$(basename "$(git -C /workspace rev-parse --show-toplevel 2>/dev/null)" 2>/dev/null || echo "unknown")
    export URP_PROJECT
fi

# Wait for memgraph to be ready (if NEO4J_URI is set)
if [[ -n "$NEO4J_URI" ]]; then
    echo "Waiting for Memgraph..."
    max_attempts=30
    attempt=0

    while ! urp 2>&1 | grep -q "connected"; do
        attempt=$((attempt + 1))
        if [[ $attempt -ge $max_attempts ]]; then
            echo "Warning: Could not connect to Memgraph after $max_attempts attempts"
            break
        fi
        sleep 1
    done

    if [[ $attempt -lt $max_attempts ]]; then
        echo "Memgraph connected"
    fi
fi

# Initialize session in graph
urp session id 2>/dev/null || true

# Display startup info
echo ""
echo "═══════════════════════════════════════════════════════"
echo " URP Container Ready"
echo "═══════════════════════════════════════════════════════"
echo " Project:    $URP_PROJECT"
echo " Session:    $URP_SESSION_ID"
echo " Read-only:  $URP_READ_ONLY"
echo " Master:     ${URP_MASTER:-0}"
echo "═══════════════════════════════════════════════════════"
echo ""
echo "Commands:"
echo "  urp              - Show status"
echo "  pain             - Recent errors"
echo "  vitals           - Container metrics"
echo "  wisdom <error>   - Find similar past errors"
echo "  learn <desc>     - Consolidate success"
echo ""

# Execute the provided command
exec "$@"
