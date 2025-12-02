#!/bin/bash
# ═══════════════════════════════════════════════════════════════════════════════
# URP Container Entrypoint
# ═══════════════════════════════════════════════════════════════════════════════
#
# Initializes the semantic layer before dropping into shell or running commands.

set -e

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# Ensure Python can find app modules from any directory
export PYTHONPATH="/app:${PYTHONPATH}"

# ─────────────────────────────────────────────────────────────────────────────
# 0. Load shared secrets if available
# ─────────────────────────────────────────────────────────────────────────────

SECRETS_FILE="/app/secrets.env"
if [ -f "$SECRETS_FILE" ]; then
    echo -n "Loading secrets..."
    # Export all variables from secrets file (skip comments and empty lines)
    set -a
    while IFS='=' read -r key value || [ -n "$key" ]; do
        # Skip comments and empty lines
        [[ "$key" =~ ^#.*$ ]] && continue
        [[ -z "$key" ]] && continue
        # Remove quotes from value
        value="${value%\"}"
        value="${value#\"}"
        value="${value%\'}"
        value="${value#\'}"
        # Only export if value is not empty
        if [ -n "$value" ]; then
            export "$key=$value"
        fi
    done < "$SECRETS_FILE"
    set +a
    echo -e " ${GREEN}OK${NC}"
fi

echo -e "${GREEN}═══════════════════════════════════════════════════════════════${NC}"
echo -e "${GREEN}  URP: Universal Repository Perception${NC}"
echo -e "${GREEN}═══════════════════════════════════════════════════════════════${NC}"

# ─────────────────────────────────────────────────────────────────────────────
# 1. Wait for Memgraph/Neo4j to be ready
# ─────────────────────────────────────────────────────────────────────────────

wait_for_db() {
    local max_attempts=30
    local attempt=1

    echo -n "Waiting for graph database..."

    while [ $attempt -le $max_attempts ]; do
        if python3 -c "
from database import Database
try:
    db = Database()
    db.execute_query('RETURN 1')
    db.close()
    exit(0)
except Exception as e:
    import sys
    print(f'DB check failed: {e}', file=sys.stderr)
    exit(1)
"; then
            echo -e " ${GREEN}OK${NC}"
            return 0
        fi
        echo -n "."
        sleep 1
        attempt=$((attempt + 1))
    done

    echo -e " ${YELLOW}TIMEOUT${NC}"
    echo -e "${YELLOW}Warning: Graph database not available. Running in degraded mode.${NC}"
    return 1
}

# ─────────────────────────────────────────────────────────────────────────────
# 2. Initialize schema if needed
# ─────────────────────────────────────────────────────────────────────────────

init_schema() {
    echo -n "Initializing graph schema..."

    python3 << 'EOF' 2>/dev/null && echo -e " ${GREEN}OK${NC}" || echo -e " ${YELLOW}SKIP${NC}"
from database import Database
try:
    db = Database()
    # Create indexes for performance
    db.execute_query("CREATE INDEX ON :File(path)")
    db.execute_query("CREATE INDEX ON :Function(signature)")
    db.execute_query("CREATE INDEX ON :Commit(hash)")
    db.execute_query("CREATE INDEX ON :Container(id)")
    db.execute_query("CREATE INDEX ON :TerminalEvent(timestamp)")
    db.close()
except Exception as e:
    # Indexes may already exist
    pass
EOF
}

# ─────────────────────────────────────────────────────────────────────────────
# 3. Auto-ingest codebase if mounted
# ─────────────────────────────────────────────────────────────────────────────

auto_ingest() {
    if [ -d "/codebase" ] && [ "$(ls -A /codebase 2>/dev/null)" ]; then
        echo -n "Auto-ingesting /codebase..."

        # Check if already ingested
        if python3 -c "
from database import Database
db = Database()
result = db.execute_query('MATCH (f:File) WHERE f.path STARTS WITH \"/codebase\" RETURN count(f) as c')
count = result[0]['c'] if result else 0
db.close()
exit(0 if count > 0 else 1)
" 2>/dev/null; then
            echo -e " ${GREEN}Already done${NC}"
        else
            python3 /app/cli.py ingest /codebase 2>/dev/null && echo -e " ${GREEN}OK${NC}" || echo -e " ${YELLOW}SKIP${NC}"

            # Also load git history if available
            if [ -d "/codebase/.git" ]; then
                echo -n "Loading git history..."
                python3 /app/cli.py git /codebase 2>/dev/null && echo -e " ${GREEN}OK${NC}" || echo -e " ${YELLOW}SKIP${NC}"
            fi
        fi
    fi
}

# ─────────────────────────────────────────────────────────────────────────────
# 4. Setup OpenCode config (multi-provider LLM)
# ─────────────────────────────────────────────────────────────────────────────

setup_opencode_config() {
    # OpenCode looks for config in ~/.config/opencode or current dir
    local opencode_dir="/root/.config/opencode"
    mkdir -p "$opencode_dir"

    if [[ -f /app/.opencode/opencode.json ]]; then
        cp /app/.opencode/opencode.json "$opencode_dir/opencode.json"
        echo -n "OpenCode config..."
        echo -e " ${GREEN}OK${NC}"
    fi
}

# ─────────────────────────────────────────────────────────────────────────────
# 5. Setup Claude CLAUDE.md based on role (Master vs Worker)
# ─────────────────────────────────────────────────────────────────────────────

setup_claude_config() {
    # Create .claude directory in codebase if writable, else in /tmp
    local claude_dir="/codebase/.claude"
    if [[ "$URP_MASTER" == "1" ]]; then
        # Master is read-only, use /tmp
        claude_dir="/tmp/.claude"
    fi

    mkdir -p "$claude_dir" 2>/dev/null || claude_dir="/tmp/.claude" && mkdir -p "$claude_dir"

    if [[ "$URP_MASTER" == "1" ]]; then
        echo -n "Setting up Master orchestrator config..."
        if [[ -f /app/.claude/MASTER.md ]]; then
            cp /app/.claude/MASTER.md "$claude_dir/CLAUDE.md"
            echo -e " ${GREEN}OK${NC}"
            echo -e "${YELLOW}  Role: ORCHESTRATOR (read-only)${NC}"
            echo -e "${YELLOW}  Use urp-spawn to create workers${NC}"
        else
            echo -e " ${YELLOW}SKIP (template not found)${NC}"
        fi
    elif [[ "$URP_WORKER" == "1" ]]; then
        echo -n "Setting up Worker config..."
        # Check for task-specific template passed via WORKER_TEMPLATE env
        local template="${WORKER_TEMPLATE:-WORKER_GENERIC}"
        if [[ -f "/app/.claude/templates/${template}.md" ]]; then
            cp "/app/.claude/templates/${template}.md" "$claude_dir/CLAUDE.md"
            echo -e " ${GREEN}OK${NC} (template: $template)"
        elif [[ -f /app/.claude/templates/WORKER_GENERIC.md ]]; then
            cp /app/.claude/templates/WORKER_GENERIC.md "$claude_dir/CLAUDE.md"
            echo -e " ${GREEN}OK${NC} (generic worker)"
        else
            echo -e " ${YELLOW}SKIP${NC}"
        fi
        echo -e "${YELLOW}  Role: WORKER #${WORKER_NUM:-1} (read-write)${NC}"
    fi

    # Copy templates directory for reference
    if [[ -d /app/.claude/templates ]]; then
        cp -r /app/.claude/templates "$claude_dir/" 2>/dev/null || true
    fi

    # Set CLAUDE_CONFIG_DIR for claude CLI
    export CLAUDE_CONFIG_DIR="$claude_dir"
}

# ─────────────────────────────────────────────────────────────────────────────
# 5. Start session
# ─────────────────────────────────────────────────────────────────────────────

start_session() {
    echo -n "Starting session..."
    python3 /app/runner.py session --name "container_$(date +%Y%m%d_%H%M%S)" 2>/dev/null && \
        echo -e " ${GREEN}OK${NC}" || echo -e " ${YELLOW}SKIP${NC}"
}

# ─────────────────────────────────────────────────────────────────────────────
# Main initialization
# ─────────────────────────────────────────────────────────────────────────────

# Source shell hooks for interactive use
export URP_RUNNER="/app/runner.py"
export URP_ENABLED=1

if wait_for_db; then
    init_schema
    auto_ingest
    start_session
fi

# Setup LLM configs
setup_opencode_config
setup_claude_config

# ─────────────────────────────────────────────────────────────────────────────
# 6. Start local proxy for token tracking
# ─────────────────────────────────────────────────────────────────────────────

start_local_proxy() {
    if command -v urp-proxy &>/dev/null; then
        echo -n "Starting local proxy (token tracking)..."
        # Start proxy in background
        UPSTREAM_URL="${ANTHROPIC_UPSTREAM:-http://100.105.212.98:8317}" \
        LISTEN_ADDR=":8318" \
        STATS_DB="/app/sessions/proxy_stats.db" \
        nohup urp-proxy > /tmp/urp-proxy.log 2>&1 &
        echo $! > /tmp/urp-proxy.pid
        sleep 0.5
        if kill -0 $(cat /tmp/urp-proxy.pid) 2>/dev/null; then
            echo -e " ${GREEN}OK${NC} (localhost:8318)"
            # Override ANTHROPIC_BASE_URL to use local proxy
            export ANTHROPIC_BASE_URL="http://localhost:8318"
        else
            echo -e " ${RED}FAILED${NC}"
            echo -e "${YELLOW}Warning: Using direct upstream${NC}"
        fi
    fi
}

start_local_proxy

echo -e "${GREEN}═══════════════════════════════════════════════════════════════${NC}"
echo ""
echo "Commands available:"
echo "  urp-status    - Check URP status"
echo "  urp-init .    - Initialize current directory"
echo "  vitals        - Container health (Φ)"
echo "  pain          - Recent errors (⊥)"
echo "  recent        - Recent commands (τ)"
echo ""
echo "LLM Agents:"
echo "  claude        - Claude Code (Anthropic)"
echo "  oc            - OpenCode (multi-provider)"
echo "  oc-models     - List available models"
echo ""
echo "Token Usage:"
echo "  stats         - Local usage (this container)"
echo "  stats-recent  - Recent requests"
echo "  proxy-stats   - Global usage (all users)"
echo ""

# ─────────────────────────────────────────────────────────────────────────────
# Execute command or drop into shell
# ─────────────────────────────────────────────────────────────────────────────

if [ $# -eq 0 ]; then
    # Interactive mode - drop into bash with hooks
    exec /bin/bash --rcfile /app/shell_hooks.sh -i
else
    # Command mode - source hooks and run
    source /app/shell_hooks.sh
    exec "$@"
fi
