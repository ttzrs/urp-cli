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
# ─────────────────────────────────────────────────────────────────────────────
urp-spawn() {
    local WORKER_NUM="${1:-1}"
    local WORKER_NAME="urp-worker-${PROJECT_NAME}-${WORKER_NUM}"

    if docker ps --format '{{.Names}}' | grep -q "^${WORKER_NAME}$"; then
        echo -e "${YELLOW}Worker ${WORKER_NUM} already running. Attaching...${NC}"
        docker exec -it "$WORKER_NAME" bash
        return 0
    fi

    echo -e "${GREEN}Spawning worker ${WORKER_NUM} for ${PROJECT_NAME}...${NC}"

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
        -e PROJECT_NAME="$PROJECT_NAME" \
        -e CODEBASE_PATH=/codebase \
        -v /var/run/docker.sock:/var/run/docker.sock \
        -v "$PROJECT_DIR:/codebase" \
        -v urp_sessions:/app/sessions \
        -v urp_chroma:/app/chroma \
        -w /codebase \
        urp-cli:latest \
        tail -f /dev/null

    echo -e "${GREEN}✓ Worker ${WORKER_NUM} spawned: ${WORKER_NAME}${NC}"
    echo -e "  Attach with: ${CYAN}urp-attach ${WORKER_NUM}${NC}"
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
# Show available commands on source
# ─────────────────────────────────────────────────────────────────────────────
if [[ "$URP_MASTER" == "1" ]]; then
    echo -e "${MAGENTA}Master commands available:${NC}"
    echo -e "  ${CYAN}urp-spawn [n]${NC}    - Spawn worker n (default: 1)"
    echo -e "  ${CYAN}urp-attach [n]${NC}   - Attach to worker n"
    echo -e "  ${CYAN}urp-exec n cmd${NC}   - Execute cmd in worker n"
    echo -e "  ${CYAN}urp-workers${NC}      - List all workers"
    echo -e "  ${CYAN}urp-kill [n]${NC}     - Kill worker n"
    echo -e "  ${CYAN}urp-kill-all${NC}     - Kill all workers"
    echo ""
fi
