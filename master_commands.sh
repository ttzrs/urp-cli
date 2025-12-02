#!/bin/bash
# ═══════════════════════════════════════════════════════════════════════════════
# URP Master Commands - Available inside urp-m container
# ═══════════════════════════════════════════════════════════════════════════════
#
# These commands are sourced in the master container to control workers.
#

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
CYAN='\033[0;36m'
MAGENTA='\033[0;35m'
NC='\033[0m'

# ─────────────────────────────────────────────────────────────────────────────
# urp-spawn: Spawn a worker container with write access
# Usage: urp-spawn [num] [template]
#   num: Worker number (default: 1)
#   template: WORKER_GENERIC, WORKER_BUGFIX, WORKER_FEATURE, WORKER_REFACTOR, WORKER_TEST
# ─────────────────────────────────────────────────────────────────────────────
urp-spawn() {
    local WORKER_NUM="${1:-1}"
    local WORKER_TEMPLATE="${2:-WORKER_GENERIC}"
    local WORKER_NAME="urp-worker-${PROJECT_NAME}-${WORKER_NUM}"

    if docker ps --format '{{.Names}}' | grep -q "^${WORKER_NAME}$"; then
        echo -e "${YELLOW}Worker ${WORKER_NUM} already running. Attaching...${NC}"
        docker exec -it "$WORKER_NAME" bash
        return 0
    fi

    echo -e "${GREEN}Spawning worker ${WORKER_NUM} for ${PROJECT_NAME}...${NC}"
    echo -e "${CYAN}  Template: ${WORKER_TEMPLATE}${NC}"

    docker run -d \
        --name "$WORKER_NAME" \
        --network urp-network \
        --group-add "${DOCKER_GID:-961}" \
        --security-opt label:disable \
        -e NEO4J_URI=bolt://urp-memgraph:7687 \
        -e NEO4J_USER= \
        -e NEO4J_PASSWORD= \
        -e URP_ENABLED=1 \
        -e URP_WORKER=1 \
        -e WORKER_NUM="$WORKER_NUM" \
        -e WORKER_TEMPLATE="$WORKER_TEMPLATE" \
        -e PROJECT_NAME="$PROJECT_NAME" \
        -e CODEBASE_PATH=/codebase \
        -e ANTHROPIC_BASE_URL=http://100.105.212.98:8317 \
        -e ANTHROPIC_AUTH_TOKEN=sk-dummy \
        -v /var/run/docker.sock:/var/run/docker.sock \
        -v "$HOME/.claude:/root/.claude" \
        -v "$PROJECT_DIR:/codebase" \
        -v urp_sessions:/app/sessions \
        -v urp_chroma:/app/chroma \
        -w /codebase \
        urp-cli:latest \
        tail -f /dev/null

    echo -e "${GREEN}✓ Worker ${WORKER_NUM} spawned: ${WORKER_NAME}${NC}"
    echo -e "  Template: ${CYAN}${WORKER_TEMPLATE}${NC}"
    echo -e "  Attach with: ${CYAN}urp-attach ${WORKER_NUM}${NC}"
    echo -e "  Launch Claude: ${CYAN}urp-spawn-claude ${WORKER_NUM}${NC}"
}

# ─────────────────────────────────────────────────────────────────────────────
# Specialized worker spawners
# ─────────────────────────────────────────────────────────────────────────────
urp-spawn-bugfix() {
    local WORKER_NUM="${1:-1}"
    urp-spawn "$WORKER_NUM" "WORKER_BUGFIX"
}

urp-spawn-feature() {
    local WORKER_NUM="${1:-1}"
    urp-spawn "$WORKER_NUM" "WORKER_FEATURE"
}

urp-spawn-refactor() {
    local WORKER_NUM="${1:-1}"
    urp-spawn "$WORKER_NUM" "WORKER_REFACTOR"
}

urp-spawn-test() {
    local WORKER_NUM="${1:-1}"
    urp-spawn "$WORKER_NUM" "WORKER_TEST"
}

# ─────────────────────────────────────────────────────────────────────────────
# urp-spawn-claude: Spawn worker AND launch Claude CLI interactively
# ─────────────────────────────────────────────────────────────────────────────
urp-spawn-claude() {
    local WORKER_NUM="${1:-1}"
    local WORKER_NAME="urp-worker-${PROJECT_NAME}-${WORKER_NUM}"

    # Spawn if not running
    if ! docker ps --format '{{.Names}}' | grep -q "^${WORKER_NAME}$"; then
        urp-spawn "$WORKER_NUM"
        sleep 2  # Wait for container to initialize
    fi

    echo -e "${GREEN}Launching Claude CLI in worker ${WORKER_NUM}...${NC}"
    docker exec -it "$WORKER_NAME" bash -c "source /app/shell_hooks.sh && claude"
}

# ─────────────────────────────────────────────────────────────────────────────
# urp-spawn-oc: Spawn worker AND launch OpenCode interactively
# Usage: urp-spawn-oc [num] [model]
#   model: sonnet, opus, haiku, gpt, codex, gemini, qwen (default: sonnet)
# ─────────────────────────────────────────────────────────────────────────────
urp-spawn-oc() {
    local WORKER_NUM="${1:-1}"
    local MODEL="${2:-}"
    local WORKER_NAME="urp-worker-${PROJECT_NAME}-${WORKER_NUM}"

    # Spawn if not running
    if ! docker ps --format '{{.Names}}' | grep -q "^${WORKER_NAME}$"; then
        urp-spawn "$WORKER_NUM"
        sleep 2  # Wait for container to initialize
    fi

    # Map short names to full model paths
    local MODEL_ARG=""
    case "$MODEL" in
        sonnet|"")
            MODEL_ARG="--model router-for-me/claude-sonnet-4-5"
            echo -e "${GREEN}Launching OpenCode (Claude Sonnet 4.5) in worker ${WORKER_NUM}...${NC}"
            ;;
        opus)
            MODEL_ARG="--model router-for-me/claude-opus-4-1"
            echo -e "${GREEN}Launching OpenCode (Claude Opus 4.1) in worker ${WORKER_NUM}...${NC}"
            ;;
        haiku)
            MODEL_ARG="--model router-for-me/claude-haiku-4-5"
            echo -e "${GREEN}Launching OpenCode (Claude Haiku 4.5) in worker ${WORKER_NUM}...${NC}"
            ;;
        gpt)
            MODEL_ARG="--model router-for-me/gpt-5-1"
            echo -e "${GREEN}Launching OpenCode (GPT-5.1) in worker ${WORKER_NUM}...${NC}"
            ;;
        codex)
            MODEL_ARG="--model router-for-me/gpt-5-1-codex-max"
            echo -e "${GREEN}Launching OpenCode (GPT-5.1 Codex Max) in worker ${WORKER_NUM}...${NC}"
            ;;
        gemini)
            MODEL_ARG="--model router-for-me/gemini-3-pro"
            echo -e "${GREEN}Launching OpenCode (Gemini 3 Pro) in worker ${WORKER_NUM}...${NC}"
            ;;
        qwen)
            MODEL_ARG="--model router-for-me/qwen3-coder"
            echo -e "${GREEN}Launching OpenCode (Qwen3 Coder) in worker ${WORKER_NUM}...${NC}"
            ;;
        *)
            # Allow full model path
            MODEL_ARG="--model $MODEL"
            echo -e "${GREEN}Launching OpenCode ($MODEL) in worker ${WORKER_NUM}...${NC}"
            ;;
    esac

    docker exec -it "$WORKER_NAME" bash -c "source /app/shell_hooks.sh && opencode $MODEL_ARG"
}

# ─────────────────────────────────────────────────────────────────────────────
# Quick aliases for OpenCode with specific models
# ─────────────────────────────────────────────────────────────────────────────
urp-spawn-oc-sonnet() { urp-spawn-oc "${1:-1}" sonnet; }
urp-spawn-oc-opus() { urp-spawn-oc "${1:-1}" opus; }
urp-spawn-oc-haiku() { urp-spawn-oc "${1:-1}" haiku; }
urp-spawn-oc-gpt() { urp-spawn-oc "${1:-1}" gpt; }
urp-spawn-oc-codex() { urp-spawn-oc "${1:-1}" codex; }
urp-spawn-oc-gemini() { urp-spawn-oc "${1:-1}" gemini; }
urp-spawn-oc-qwen() { urp-spawn-oc "${1:-1}" qwen; }

# ─────────────────────────────────────────────────────────────────────────────
# urp-claude: Send a prompt to Claude in a worker (non-interactive)
# ─────────────────────────────────────────────────────────────────────────────
urp-claude() {
    local WORKER_NUM="${1:-1}"
    shift
    local PROMPT="$*"
    local WORKER_NAME="urp-worker-${PROJECT_NAME}-${WORKER_NUM}"

    if [[ -z "$PROMPT" ]]; then
        echo -e "${RED}Usage: urp-claude <worker_num> <prompt>${NC}"
        echo -e "  Example: urp-claude 1 'Fix the type error in main.py'"
        return 1
    fi

    if ! docker ps --format '{{.Names}}' | grep -q "^${WORKER_NAME}$"; then
        echo -e "${YELLOW}Worker ${WORKER_NUM} not running. Spawning...${NC}"
        urp-spawn "$WORKER_NUM"
        sleep 2
    fi

    echo -e "${CYAN}Sending task to Claude in worker ${WORKER_NUM}...${NC}"
    echo -e "${YELLOW}Prompt: ${PROMPT}${NC}"
    echo ""

    docker exec "$WORKER_NAME" bash -c "source /app/shell_hooks.sh && claude -p \"$PROMPT\""
}

# ─────────────────────────────────────────────────────────────────────────────
# urp-claude-file: Send a file with instructions to Claude in worker
# ─────────────────────────────────────────────────────────────────────────────
urp-claude-file() {
    local WORKER_NUM="${1:-1}"
    local TASK_FILE="${2:-}"
    local WORKER_NAME="urp-worker-${PROJECT_NAME}-${WORKER_NUM}"

    if [[ -z "$TASK_FILE" ]] || [[ ! -f "$TASK_FILE" ]]; then
        echo -e "${RED}Usage: urp-claude-file <worker_num> <task_file.md>${NC}"
        return 1
    fi

    if ! docker ps --format '{{.Names}}' | grep -q "^${WORKER_NAME}$"; then
        echo -e "${YELLOW}Worker ${WORKER_NUM} not running. Spawning...${NC}"
        urp-spawn "$WORKER_NUM"
        sleep 2
    fi

    echo -e "${CYAN}Sending task file to Claude in worker ${WORKER_NUM}...${NC}"

    # Copy task file to worker and execute
    docker cp "$TASK_FILE" "${WORKER_NAME}:/tmp/task.md"
    docker exec "$WORKER_NAME" bash -c "source /app/shell_hooks.sh && claude -p \"\$(cat /tmp/task.md)\""
}

# ─────────────────────────────────────────────────────────────────────────────
# urp-attach: Attach to a running worker
# ─────────────────────────────────────────────────────────────────────────────
urp-attach() {
    local WORKER_NUM="${1:-1}"
    local WORKER_NAME="urp-worker-${PROJECT_NAME}-${WORKER_NUM}"

    if ! docker ps --format '{{.Names}}' | grep -q "^${WORKER_NAME}$"; then
        echo -e "${RED}Worker ${WORKER_NUM} not running. Spawn with: urp-spawn ${WORKER_NUM}${NC}"
        return 1
    fi

    echo -e "${CYAN}Attaching to worker ${WORKER_NUM}...${NC}"
    docker exec -it "$WORKER_NAME" bash
}

# ─────────────────────────────────────────────────────────────────────────────
# urp-exec: Execute a command in a worker
# ─────────────────────────────────────────────────────────────────────────────
urp-exec() {
    local WORKER_NUM="${1:-1}"
    shift
    local WORKER_NAME="urp-worker-${PROJECT_NAME}-${WORKER_NUM}"

    if ! docker ps --format '{{.Names}}' | grep -q "^${WORKER_NAME}$"; then
        echo -e "${RED}Worker ${WORKER_NUM} not running.${NC}"
        return 1
    fi

    docker exec "$WORKER_NAME" "$@"
}

# ─────────────────────────────────────────────────────────────────────────────
# urp-workers: List all workers for this project
# ─────────────────────────────────────────────────────────────────────────────
urp-workers() {
    echo -e "${MAGENTA}═══════════════════════════════════════════════════════════${NC}"
    echo -e "${MAGENTA}           Workers for ${PROJECT_NAME}${NC}"
    echo -e "${MAGENTA}═══════════════════════════════════════════════════════════${NC}"
    docker ps --filter "name=urp-worker-${PROJECT_NAME}" --format "table {{.Names}}\t{{.Status}}\t{{.RunningFor}}"
}

# ─────────────────────────────────────────────────────────────────────────────
# urp-kill: Kill a specific worker
# ─────────────────────────────────────────────────────────────────────────────
urp-kill() {
    local WORKER_NUM="${1:-1}"
    local WORKER_NAME="urp-worker-${PROJECT_NAME}-${WORKER_NUM}"

    if docker ps --format '{{.Names}}' | grep -q "^${WORKER_NAME}$"; then
        docker stop "$WORKER_NAME" && docker rm "$WORKER_NAME"
        echo -e "${GREEN}✓ Worker ${WORKER_NUM} killed${NC}"
    else
        echo -e "${YELLOW}Worker ${WORKER_NUM} not running${NC}"
    fi
}

# ─────────────────────────────────────────────────────────────────────────────
# urp-kill-all: Kill all workers for this project
# ─────────────────────────────────────────────────────────────────────────────
urp-kill-all() {
    echo -e "${YELLOW}Killing all workers for ${PROJECT_NAME}...${NC}"
    docker ps -q --filter "name=urp-worker-${PROJECT_NAME}" | xargs -r docker stop
    docker ps -aq --filter "name=urp-worker-${PROJECT_NAME}" | xargs -r docker rm
    echo -e "${GREEN}✓ All workers killed${NC}"
}

# ─────────────────────────────────────────────────────────────────────────────
# urp-tokens: Show master's own token usage (with model breakdown)
# ─────────────────────────────────────────────────────────────────────────────
urp-tokens() {
    python3 /app/token_monitor.py status
}

# ─────────────────────────────────────────────────────────────────────────────
# urp-tokens-all: Show all workers' token usage (master aggregate view)
# ─────────────────────────────────────────────────────────────────────────────
urp-tokens-all() {
    python3 /app/token_monitor.py master
}

# ─────────────────────────────────────────────────────────────────────────────
# urp-tokens-compact: One-liner for all workers
# ─────────────────────────────────────────────────────────────────────────────
urp-tokens-compact() {
    python3 /app/token_monitor.py compact-all
}

# ─────────────────────────────────────────────────────────────────────────────
# urp-tokens-worker: Show specific worker's token usage
# ─────────────────────────────────────────────────────────────────────────────
urp-tokens-worker() {
    local WORKER_NUM="${1:-1}"
    local WORKER_NAME="urp-worker-${PROJECT_NAME}-${WORKER_NUM}"

    if docker ps --format '{{.Names}}' | grep -q "^${WORKER_NAME}$"; then
        docker exec "$WORKER_NAME" python3 /app/token_monitor.py status
    else
        echo -e "${RED}Worker ${WORKER_NUM} not running${NC}"
    fi
}

# ─────────────────────────────────────────────────────────────────────────────
# urp-models: Show available models with pricing
# ─────────────────────────────────────────────────────────────────────────────
urp-models() {
    python3 /app/pricing_db.py models
}

# ─────────────────────────────────────────────────────────────────────────────
# urp-cost: Show actual API cost (calculated from SQLite with model awareness)
# ─────────────────────────────────────────────────────────────────────────────
urp-cost() {
    local data=$(python3 /app/token_monitor.py json 2>/dev/null)

    if [[ -z "$data" ]]; then
        echo "No token data available"
        return 1
    fi

    local total_cost=$(echo "$data" | jq -r '.totals.cost // 0')
    local input=$(echo "$data" | jq -r '.totals.input // 0')
    local output=$(echo "$data" | jq -r '.totals.output // 0')
    local requests=$(echo "$data" | jq -r '.totals.requests // 0')

    echo -e "${MAGENTA}═══════════════════════════════════════════════════════════${NC}"
    echo -e "${MAGENTA}           API COST (Model-Aware Pricing)${NC}"
    echo -e "${MAGENTA}═══════════════════════════════════════════════════════════${NC}"
    echo ""
    printf "  Input tokens:  %'12d\n" "$input"
    printf "  Output tokens: %'12d\n" "$output"
    printf "  Requests:      %'12d\n" "$requests"
    echo "  ─────────────────────────────────────────────"
    printf "  ${GREEN}Total cost: \$%.6f${NC}\n" "$total_cost"
    echo ""
    echo -e "  ${CYAN}Run 'urp-models' to see pricing per model${NC}"
    echo ""
}

# ─────────────────────────────────────────────────────────────────────────────
# urp-cost-estimate: Estimate cost for given tokens
# ─────────────────────────────────────────────────────────────────────────────
urp-cost-estimate() {
    local INPUT="${1:-0}"
    local OUTPUT="${2:-0}"
    local MODEL="${3:-sonnet}"

    python3 /app/pricing_db.py cost "$INPUT" "$OUTPUT" "$MODEL"
}

# ─────────────────────────────────────────────────────────────────────────────
# Show available commands on source
# ─────────────────────────────────────────────────────────────────────────────
if [[ "$URP_MASTER" == "1" ]]; then
    echo -e "${MAGENTA}═══════════════════════════════════════════════════════════${NC}"
    echo -e "${MAGENTA}              MASTER ORCHESTRATOR MODE${NC}"
    echo -e "${MAGENTA}═══════════════════════════════════════════════════════════${NC}"
    echo -e ""
    echo -e "  ${GREEN}Worker Management:${NC}"
    echo -e "  ${CYAN}urp-spawn [n] [template]${NC}  - Spawn worker container"
    echo -e "  ${CYAN}urp-workers${NC}               - List all workers"
    echo -e "  ${CYAN}urp-attach [n]${NC}            - Attach bash to worker"
    echo -e "  ${CYAN}urp-kill [n]${NC}              - Kill worker n"
    echo -e "  ${CYAN}urp-kill-all${NC}              - Kill all workers"
    echo -e ""
    echo -e "  ${GREEN}Claude Code Workers:${NC}"
    echo -e "  ${CYAN}urp-spawn-claude [n]${NC}      - Worker + Claude Code"
    echo -e "  ${CYAN}urp-claude n \"prompt\"${NC}     - Send prompt to Claude"
    echo -e ""
    echo -e "  ${GREEN}OpenCode Workers (multi-model):${NC}"
    echo -e "  ${CYAN}urp-spawn-oc [n] [model]${NC}  - Worker + OpenCode"
    echo -e "  ${CYAN}urp-spawn-oc-sonnet [n]${NC}   - Claude Sonnet 4.5"
    echo -e "  ${CYAN}urp-spawn-oc-opus [n]${NC}     - Claude Opus 4.1"
    echo -e "  ${CYAN}urp-spawn-oc-haiku [n]${NC}    - Claude Haiku 4.5 (fast)"
    echo -e "  ${CYAN}urp-spawn-oc-gpt [n]${NC}      - GPT-5.1"
    echo -e "  ${CYAN}urp-spawn-oc-codex [n]${NC}    - GPT-5.1 Codex Max"
    echo -e "  ${CYAN}urp-spawn-oc-gemini [n]${NC}   - Gemini 3 Pro"
    echo -e "  ${CYAN}urp-spawn-oc-qwen [n]${NC}     - Qwen3 Coder"
    echo -e ""
    echo -e "  ${GREEN}Token Monitoring:${NC}"
    echo -e "  ${CYAN}urp-tokens${NC}                - Master token usage"
    echo -e "  ${CYAN}urp-tokens-all${NC}            - All workers + totals"
    echo -e "  ${CYAN}urp-cost${NC}                  - API cost (\$)"
    echo -e ""
    echo -e "  ${YELLOW}Your role: Analyze → Plan → Delegate → Validate${NC}"
    echo ""
fi
