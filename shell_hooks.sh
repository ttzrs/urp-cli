#!/bin/bash
# ═══════════════════════════════════════════════════════════════════════════════
# URP Shell Hooks - Transparent command interception for semantic logging
# ═══════════════════════════════════════════════════════════════════════════════
#
# This file wraps common commands to log execution to the knowledge graph.
# Source this in .bashrc or .zshrc: source /app/shell_hooks.sh
#
# Commands are executed normally - wrapper only adds observability.
# If the graph DB is down, commands still work (silent fallback).

# Path to the runner script
export URP_RUNNER="${URP_RUNNER:-/app/runner.py}"
export URP_ENABLED="${URP_ENABLED:-1}"

# Load master commands if in master mode
if [[ "$URP_MASTER" == "1" ]] && [[ -f /app/master_commands.sh ]]; then
    source /app/master_commands.sh
fi

# ═══════════════════════════════════════════════════════════════════════════════
# Core wrapper function
# ═══════════════════════════════════════════════════════════════════════════════

_urp_wrap() {
    local cmd_name="$1"
    shift

    if [[ "$URP_ENABLED" != "1" ]] || [[ ! -f "$URP_RUNNER" ]]; then
        # Fallback to real command
        command "$cmd_name" "$@"
        return $?
    fi

    # Use the runner to execute and log
    python3 "$URP_RUNNER" run "$cmd_name" "$@"
    return $?
}

# ═══════════════════════════════════════════════════════════════════════════════
# VCS Commands (τ - Temporal primitives)
# ═══════════════════════════════════════════════════════════════════════════════

git() { _urp_wrap git "$@"; }

# ═══════════════════════════════════════════════════════════════════════════════
# Container Commands (D - Domain, Φ - Energy)
# ═══════════════════════════════════════════════════════════════════════════════

docker() { _urp_wrap docker "$@"; }
podman() { _urp_wrap podman "$@"; }
kubectl() { _urp_wrap kubectl "$@"; }
k3s() { _urp_wrap k3s "$@"; }

# ═══════════════════════════════════════════════════════════════════════════════
# Build/Package Commands (Φ - Build energy)
# ═══════════════════════════════════════════════════════════════════════════════

npm() { _urp_wrap npm "$@"; }
npx() { _urp_wrap npx "$@"; }
yarn() { _urp_wrap yarn "$@"; }
pnpm() { _urp_wrap pnpm "$@"; }
pip() { _urp_wrap pip "$@"; }
pip3() { _urp_wrap pip3 "$@"; }
cargo() { _urp_wrap cargo "$@"; }
go() { _urp_wrap go "$@"; }
make() { _urp_wrap make "$@"; }
mvn() { _urp_wrap mvn "$@"; }
gradle() { _urp_wrap gradle "$@"; }
cmake() { _urp_wrap cmake "$@"; }

# ═══════════════════════════════════════════════════════════════════════════════
# Test Commands (⊥ - Validation/Conflict detection)
# ═══════════════════════════════════════════════════════════════════════════════

pytest() { _urp_wrap pytest "$@"; }
jest() { _urp_wrap jest "$@"; }
mocha() { _urp_wrap mocha "$@"; }
vitest() { _urp_wrap vitest "$@"; }

# ═══════════════════════════════════════════════════════════════════════════════
# Direct URP commands (aliases for convenience)
# ═══════════════════════════════════════════════════════════════════════════════

# Brain queries (read-only, no wrapping needed)
alias urp='python3 /app/cli.py'
alias urp-ingest='python3 /app/cli.py ingest'
alias urp-git='python3 /app/cli.py git'
alias urp-impact='python3 /app/cli.py impact'
alias urp-deps='python3 /app/cli.py deps'
alias urp-history='python3 /app/cli.py history'
alias urp-hotspots='python3 /app/cli.py hotspots'
alias urp-dead='python3 /app/cli.py dead'
alias urp-stats='python3 /app/cli.py stats'

# Runtime observation
alias vitals='python3 /app/cli.py vitals'
alias topology='python3 /app/cli.py topology'
alias health='python3 /app/cli.py health'

# Terminal flow queries
alias recent='python3 /app/runner.py recent'
alias pain='python3 /app/runner.py pain'

# Session management
alias session='python3 /app/runner.py session'

# Cognitive skills
alias wisdom='python3 /app/runner.py wisdom'
alias novelty='python3 /app/runner.py novelty'
alias focus='python3 /app/runner.py focus'
alias learn='python3 /app/runner.py learn'
alias surgical='python3 /app/runner.py surgical'

# Working memory (token economy)
alias unfocus='python3 /app/runner.py unfocus'
alias clear-context='python3 /app/runner.py clear'
alias mem-status='python3 /app/runner.py status'
alias mem-context='python3 /app/runner.py context'

# External documentation
alias docs='python3 /app/docs_rag.py'
alias docs-ingest='python3 /app/docs_rag.py ingest'
alias docs-query='python3 /app/docs_rag.py query'
alias docs-list='python3 /app/docs_rag.py list'

# Shared knowledge
alias knowledge='python3 /app/knowledge.py'
alias knowledge-upload='python3 /app/knowledge.py upload'
alias knowledge-list='python3 /app/knowledge.py list'
alias knowledge-sync='python3 /app/knowledge.py sync'

# Token tracking
alias tokens='python3 /app/token_tracker.py'
alias tokens-status='python3 /app/token_tracker.py status'
alias tokens-compact='python3 /app/token_tracker.py compact'
alias tokens-stats='python3 /app/token_tracker.py stats'
alias tokens-budget='python3 /app/token_tracker.py budget'
alias tokens-reset='python3 /app/token_tracker.py reset'

# ═══════════════════════════════════════════════════════════════════════════════
# Session Memory & Knowledge (Multi-session cognitive architecture)
# ═══════════════════════════════════════════════════════════════════════════════

# Session memory (private per session)
alias remember='python3 /app/runner.py remember'
alias recall='python3 /app/runner.py recall'
alias memories='python3 /app/runner.py memories'

# Shared knowledge (global KB)
alias kstore='python3 /app/runner.py knowledge store'
alias kquery='python3 /app/runner.py knowledge query'
alias klist='python3 /app/runner.py knowledge list'
alias kreject='python3 /app/runner.py knowledge reject'
alias kexport='python3 /app/runner.py knowledge export'

# Memory stats & identity
alias memstats='python3 /app/runner.py memstats'
alias identity='python3 /app/runner.py identity'

# Metacognitive evaluation
alias should-save='python3 /app/runner.py should save'
alias should-promote='python3 /app/runner.py should promote'
alias should-reject='python3 /app/runner.py should reject'

# LLM Tools direct access
alias llm-tools='python3 /app/llm_tools.py'

# Claude Code wrappers
alias cw='claude-write'  # Shortcut for claude-write

# ═══════════════════════════════════════════════════════════════════════════════
# Utility functions
# ═══════════════════════════════════════════════════════════════════════════════

# Disable URP wrapping temporarily
urp-off() {
    export URP_ENABLED=0
    echo "URP wrapping disabled. Commands run directly."
}

# Re-enable URP wrapping
urp-on() {
    export URP_ENABLED=1
    echo "URP wrapping enabled."
}

# Check URP status
urp-status() {
    echo "URP_ENABLED=$URP_ENABLED"
    echo "URP_RUNNER=$URP_RUNNER"
    if [[ -f "$URP_RUNNER" ]]; then
        echo "Runner: OK"
    else
        echo "Runner: NOT FOUND"
    fi
    python3 /app/cli.py stats 2>/dev/null || echo "Graph: NOT CONNECTED"
    echo ""
    echo "Token Usage:"
    python3 /app/token_tracker.py compact 2>/dev/null || echo "Token tracker: NOT AVAILABLE"
}

# Initialize the codebase (run on first use)
urp-init() {
    local path="${1:-.}"
    echo "Initializing URP for: $path"
    python3 /app/cli.py ingest "$path"
    if [[ -d "$path/.git" ]]; then
        python3 /app/cli.py git "$path"
    fi
    python3 /app/runner.py session --name "init_$(date +%s)"
    echo "Done. Use 'urp-status' to check."
}

# ═══════════════════════════════════════════════════════════════════════════════
# Query Scope Commands (project-local vs global)
# ═══════════════════════════════════════════════════════════════════════════════

# Project-local queries (only current project)
alias local-wisdom='python3 /app/runner.py wisdom --project "$PROJECT_NAME"'
alias local-pain='python3 /app/runner.py pain --project "$PROJECT_NAME"'
alias local-recent='python3 /app/runner.py recent --project "$PROJECT_NAME"'
alias local-history='python3 /app/cli.py history --project "$PROJECT_NAME"'
alias local-hotspots='python3 /app/cli.py hotspots --project "$PROJECT_NAME"'

# Global queries (all projects - cross-project learning)
alias global-wisdom='python3 /app/runner.py wisdom --all'
alias global-pain='python3 /app/runner.py pain --all'
alias global-recent='python3 /app/runner.py recent --all'

# Show connection topology
urp-topology() {
    echo -e "\033[0;36m"
    echo "═══════════════════════════════════════════════════════════════"
    echo "                    URP Network Topology                        "
    echo "═══════════════════════════════════════════════════════════════"
    echo -e "\033[0m"
    echo ""
    echo "  ┌─────────────────────────────────────────────────────────┐"
    echo "  │                    urp-network                          │"
    echo "  │                                                         │"
    echo "  │   ┌─────────────┐      ┌─────────────┐                 │"
    echo "  │   │ urp-memgraph│◄────►│  urp-chroma │                 │"
    echo "  │   │  :7687 (db) │      │  (vectors)  │                 │"
    echo "  │   └──────┬──────┘      └─────────────┘                 │"
    echo "  │          │                                              │"
    echo "  │          │ bolt://urp-memgraph:7687                     │"
    echo "  │          │                                              │"

    # List connected project containers
    local containers=$(docker ps --filter "network=urp-network" --filter "name=urp-" --format "{{.Names}}" 2>/dev/null | grep -v memgraph | grep -v chroma | grep -v lab)

    if [[ -n "$containers" ]]; then
        for c in $containers; do
            local is_current=""
            if [[ "$c" == "urp-${PROJECT_NAME}" ]] || [[ "$c" == "urp-master-${PROJECT_NAME}" ]]; then
                is_current=" ◄── YOU"
            fi
            printf "  │   ┌─────────────┐                                       │\n"
            printf "  │   │ %-11s │%s                          │\n" "$c" "$is_current"
            printf "  │   └─────────────┘                                       │\n"
        done
    else
        echo "  │   (no project containers running)                       │"
    fi

    echo "  │                                                         │"
    echo "  └─────────────────────────────────────────────────────────┘"
    echo ""
    echo -e "\033[0;33mProject: ${PROJECT_NAME:-unknown}\033[0m"
    echo -e "\033[0;33mScope:   local-* (this project) | global-* (all projects)\033[0m"
    echo ""
}

# ═══════════════════════════════════════════════════════════════════════════════
# Startup message
# ═══════════════════════════════════════════════════════════════════════════════

if [[ "$URP_ENABLED" == "1" ]] && [[ -f "$URP_RUNNER" ]]; then
    # Show topology on startup
    urp-topology
    echo -e "\033[0;32m🧠 URP active.\033[0m Commands: urp-topology, local-wisdom, global-wisdom"
fi
