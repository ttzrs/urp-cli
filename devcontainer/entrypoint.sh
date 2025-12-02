#!/bin/bash
# ═══════════════════════════════════════════════════════════════════════════════
# URP Dev Container Entrypoint
# ═══════════════════════════════════════════════════════════════════════════════

set -e

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m'

echo -e "${BLUE}═══════════════════════════════════════════════════════════════${NC}"
echo -e "${BLUE}  URP Dev Container - Full Stack AI Environment${NC}"
echo -e "${BLUE}═══════════════════════════════════════════════════════════════${NC}"

# ─────────────────────────────────────────────────────────────────────────────
# GPU Check
# ─────────────────────────────────────────────────────────────────────────────

check_gpu() {
    if command -v nvidia-smi &> /dev/null; then
        GPU_INFO=$(nvidia-smi --query-gpu=name,memory.total --format=csv,noheader 2>/dev/null || echo "")
        if [ -n "$GPU_INFO" ]; then
            echo -e "GPU: ${GREEN}$GPU_INFO${NC}"
        else
            echo -e "GPU: ${YELLOW}NVIDIA driver loaded but no GPU detected${NC}"
        fi
    else
        echo -e "GPU: ${YELLOW}No NVIDIA support${NC}"
    fi
}

# ─────────────────────────────────────────────────────────────────────────────
# Tool Versions
# ─────────────────────────────────────────────────────────────────────────────

show_versions() {
    echo -e "\n${GREEN}Installed Tools:${NC}"
    echo -n "  Go: "; go version 2>/dev/null | cut -d' ' -f3 || echo "not found"
    echo -n "  Python: "; python3 --version 2>/dev/null | cut -d' ' -f2 || echo "not found"
    echo -n "  Node: "; node --version 2>/dev/null || echo "not found"
    echo -n "  Docker: "; docker --version 2>/dev/null | cut -d' ' -f3 | tr -d ',' || echo "not found"
    echo -n "  Git: "; git --version 2>/dev/null | cut -d' ' -f3 || echo "not found"
    echo -n "  GH: "; gh --version 2>/dev/null | head -1 | cut -d' ' -f3 || echo "not found"
    echo -n "  HF: "; huggingface-cli --version 2>/dev/null || echo "not found"
    echo -n "  Claude: "; claude --version 2>/dev/null || echo "not found"
    check_gpu
}

# ─────────────────────────────────────────────────────────────────────────────
# Database Wait
# ─────────────────────────────────────────────────────────────────────────────

wait_for_db() {
    local max_attempts=30
    local attempt=1

    echo -n "Graph database..."

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
    return 1
}

# ─────────────────────────────────────────────────────────────────────────────
# Shared Knowledge Loader
# ─────────────────────────────────────────────────────────────────────────────

load_shared_knowledge() {
    if [ -d "/shared/knowledge" ] && [ "$(ls -A /shared/knowledge 2>/dev/null)" ]; then
        echo -n "Loading shared knowledge..."

        for f in /shared/knowledge/*.md /shared/knowledge/*.txt; do
            [ -f "$f" ] || continue
            # Ingest into docs RAG
            python3 /app/docs_rag.py ingest "shared" "$f" 2>/dev/null || true
        done

        echo -e " ${GREEN}OK${NC}"
    fi
}

# ─────────────────────────────────────────────────────────────────────────────
# Schema Init
# ─────────────────────────────────────────────────────────────────────────────

init_schema() {
    echo -n "Graph schema..."

    python3 << 'EOF' 2>/dev/null && echo -e " ${GREEN}OK${NC}" || echo -e " ${YELLOW}SKIP${NC}"
from database import Database
try:
    db = Database()
    db.execute_query("CREATE INDEX ON :File(path)")
    db.execute_query("CREATE INDEX ON :Function(signature)")
    db.execute_query("CREATE INDEX ON :Commit(hash)")
    db.execute_query("CREATE INDEX ON :Container(id)")
    db.execute_query("CREATE INDEX ON :TerminalEvent(timestamp)")
    db.execute_query("CREATE INDEX ON :Library(name)")
    db.execute_query("CREATE INDEX ON :DocChunk(id)")
    db.close()
except:
    pass
EOF
}

# ─────────────────────────────────────────────────────────────────────────────
# Auto Ingest
# ─────────────────────────────────────────────────────────────────────────────

auto_ingest() {
    if [ -d "/workspace" ] && [ "$(ls -A /workspace 2>/dev/null)" ]; then
        # Check if already ingested
        if python3 -c "
from database import Database
db = Database()
result = db.execute_query('MATCH (f:File) WHERE f.path STARTS WITH \"/workspace\" RETURN count(f) as c')
count = result[0]['c'] if result else 0
db.close()
exit(0 if count > 0 else 1)
" 2>/dev/null; then
            echo -e "Workspace: ${GREEN}Already ingested${NC}"
        else
            echo -n "Ingesting workspace..."
            python3 /app/cli.py ingest /workspace 2>/dev/null && echo -e " ${GREEN}OK${NC}" || echo -e " ${YELLOW}SKIP${NC}"

            if [ -d "/workspace/.git" ]; then
                echo -n "Loading git history..."
                python3 /app/cli.py git /workspace 2>/dev/null && echo -e " ${GREEN}OK${NC}" || echo -e " ${YELLOW}SKIP${NC}"
            fi
        fi
    fi
}

# ─────────────────────────────────────────────────────────────────────────────
# Session Start
# ─────────────────────────────────────────────────────────────────────────────

start_session() {
    echo -n "Session..."
    python3 /app/runner.py session --name "dev_$(date +%Y%m%d_%H%M%S)" 2>/dev/null && \
        echo -e " ${GREEN}OK${NC}" || echo -e " ${YELLOW}SKIP${NC}"
}

# ─────────────────────────────────────────────────────────────────────────────
# Main
# ─────────────────────────────────────────────────────────────────────────────

export URP_RUNNER="/app/runner.py"
export URP_ENABLED=1

show_versions

echo -e "\n${GREEN}Initializing:${NC}"

if wait_for_db; then
    init_schema
    load_shared_knowledge
    auto_ingest
    start_session
fi

echo -e "\n${BLUE}═══════════════════════════════════════════════════════════════${NC}"
echo ""
echo -e "${GREEN}Commands:${NC}"
echo "  claude            - Start Claude Code (interactive)"
echo "  claude-write      - Claude Code with file writing (uses Bash+heredoc)"
echo "  cw                - Shortcut for claude-write"
echo "  urp-status        - Check URP status"
echo "  vitals            - Container health"
echo "  pain              - Recent errors"
echo ""
echo -e "${GREEN}Shared Memory:${NC}"
echo "  /shared/knowledge - Persistent knowledge (survives container restart)"
echo "  /root/.claude     - Claude config (persistent)"
echo ""
echo -e "${GREEN}Workspace:${NC} /workspace"
echo ""

# ─────────────────────────────────────────────────────────────────────────────
# Execute
# ─────────────────────────────────────────────────────────────────────────────

if [ $# -eq 0 ]; then
    exec /bin/bash --rcfile /app/shell_hooks.sh -i
else
    source /app/shell_hooks.sh
    exec "$@"
fi
