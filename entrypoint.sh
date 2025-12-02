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
except:
    exit(1)
" 2>/dev/null; then
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
# 4. Start session
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

echo -e "${GREEN}═══════════════════════════════════════════════════════════════${NC}"
echo ""
echo "Commands available:"
echo "  urp-status    - Check URP status"
echo "  urp-init .    - Initialize current directory"
echo "  vitals        - Container health (Φ)"
echo "  pain          - Recent errors (⊥)"
echo "  recent        - Recent commands (τ)"
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
