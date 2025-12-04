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

# Load Claude token hook for API usage tracking
if [[ -f /app/claude_token_hook.sh ]]; then
    source /app/claude_token_hook.sh
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

# Token tracking (file reads - legacy)
alias tokens='python3 /app/token_tracker.py'
alias tokens-status='python3 /app/token_tracker.py status'
alias tokens-compact='python3 /app/token_tracker.py compact'
alias tokens-stats='python3 /app/token_tracker.py stats'
alias tokens-budget='python3 /app/token_tracker.py budget'
alias tokens-reset='python3 /app/token_tracker.py reset'

# API Token monitoring (Claude API usage with model-aware pricing)
alias api-tokens='python3 /app/token_monitor.py'
alias api-tokens-status='python3 /app/token_monitor.py status'
alias api-tokens-compact='python3 /app/token_monitor.py compact'
alias api-tokens-json='python3 /app/token_monitor.py json'
alias api-tokens-track='python3 /app/token_monitor.py track'
alias api-tokens-models='python3 /app/token_monitor.py models'
alias api-tokens-cost='python3 /app/token_monitor.py cost'

# Pricing database direct access
alias pricing='python3 /app/pricing_db.py'
alias pricing-models='python3 /app/pricing_db.py models'
alias pricing-cost='python3 /app/pricing_db.py cost'

# Local proxy statistics (this container only)
alias stats='python3 /app/local_stats.py'
alias stats-recent='python3 /app/local_stats.py recent'
alias stats-json='python3 /app/local_stats.py json'

# Global proxy statistics (CLIProxyAPI - ALL users)
alias proxy-stats='python3 /app/proxy_stats.py'
alias proxy-usage='python3 /app/proxy_stats.py status'
alias proxy-models='python3 /app/proxy_stats.py models'
alias proxy-compact='python3 /app/proxy_stats.py compact'

# Secrets management (inside container)
secrets-list() {
    if [ -f /app/secrets.env ]; then
        echo "Loaded secrets (keys only):"
        grep -v '^#' /app/secrets.env | grep -v '^$' | cut -d'=' -f1 | sort
    else
        echo "No secrets file mounted"
    fi
}
alias secrets='secrets-list'

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
# OpenCode (via router-for.me)
# ═══════════════════════════════════════════════════════════════════════════════

alias oc='opencode'
alias oc-run='opencode run'

# Claude models
alias oc-sonnet='opencode --model router-for-me/claude-sonnet-4-5'
alias oc-opus='opencode --model router-for-me/claude-opus-4-1'
alias oc-haiku='opencode --model router-for-me/claude-haiku-4-5'

# GPT models
alias oc-gpt='opencode --model router-for-me/gpt-5-1'
alias oc-codex='opencode --model router-for-me/gpt-5-1-codex-max'

# Other models
alias oc-gemini='opencode --model router-for-me/gemini-3-pro'
alias oc-qwen='opencode --model router-for-me/qwen3-coder'

# DeepSeek (direct API - NOT through proxy)
alias oc-d3.2s='opencode --model deepseek/deepseek-v3-2-special'
alias oc-deepseek='opencode --model deepseek/deepseek-chat'
alias oc-r1='opencode --model deepseek/deepseek-reasoner'

# List available models
oc-models() {
    echo "OpenCode Models:"
    echo ""
    echo "  Claude (via proxy):"
    echo "    oc         - Claude Sonnet 4.5 (default)"
    echo "    oc-sonnet  - Claude Sonnet 4.5"
    echo "    oc-opus    - Claude Opus 4.1"
    echo "    oc-haiku   - Claude Haiku 4.5 (fast)"
    echo ""
    echo "  OpenAI (via proxy):"
    echo "    oc-gpt     - GPT-5.1"
    echo "    oc-codex   - GPT-5.1 Codex Max (best for code)"
    echo ""
    echo "  DeepSeek (direct API):"
    echo "    oc-d3.2s    - DeepSeek V3.2 Special (latest)"
    echo "    oc-deepseek - DeepSeek V3"
    echo "    oc-r1       - DeepSeek R1 (reasoning)"
    echo ""
    echo "  Other (via proxy):"
    echo "    oc-gemini  - Gemini 3 Pro Preview"
    echo "    oc-qwen    - Qwen3 Coder Plus"
}

# ═══════════════════════════════════════════════════════════════════════════════
# Context Optimization (cc-* commands)
# ═══════════════════════════════════════════════════════════════════════════════

# Status and diagnostics
alias cc-status='python3 /app/context_manager.py status'
alias cc-noise='python3 /app/context_manager.py detect-noise'
alias cc-stats='python3 /app/context_manager.py stats'

# Optimization actions
alias cc-compact='python3 /app/context_manager.py compact'
alias cc-clean='python3 /app/context_manager.py clean'

# Mode control (A/B testing)
alias cc-mode='python3 /app/context_manager.py mode'
alias cc-none='python3 /app/context_manager.py mode none'
alias cc-semi='python3 /app/context_manager.py mode semi'
alias cc-auto='python3 /app/context_manager.py mode auto'
alias cc-smart='python3 /app/context_manager.py mode hybrid'

# Quality feedback (for A/B testing)
alias cc-quality='python3 /app/context_manager.py quality'
alias cc-error='python3 /app/context_manager.py error'

# Recommendations
alias cc-recommend='python3 /app/context_manager.py recommend'

# ═══════════════════════════════════════════════════════════════════════════════
# A/B Testing Orchestrator (ab-* commands)
# ═══════════════════════════════════════════════════════════════════════════════

# Start parallel A/B test with 4 containers (none, semi, auto, hybrid)
alias ab-start='python3 /app/ab_orchestrator.py start'

# Show aggregated A/B statistics from Memgraph
alias ab-stats='python3 /app/ab_orchestrator.py stats'

# Get best mode recommendation based on collected data
alias ab-recommend='python3 /app/ab_orchestrator.py recommend'

# List recent A/B testing sessions
alias ab-list='python3 /app/ab_orchestrator.py list'

# Quick A/B test for current directory
ab-test() {
    local task="${1:-Evaluate optimization modes}"
    local script="${2:-echo 'Test task'}"
    python3 /app/ab_orchestrator.py start "$(pwd)" --task "$task" --script "$script"
}

# Initialize Memgraph schema for A/B testing
ab-init-schema() {
    cat /app/ab_schema.cypher | docker exec -i urp-memgraph mgconsole
    echo "A/B testing schema initialized in Memgraph"
}

# ═══════════════════════════════════════════════════════════════════════════════
# PCx - Performance Comparison eXperiment
# ═══════════════════════════════════════════════════════════════════════════════

# Run PCx experiment
alias pcx='python3 /app/pcx/pcx_runner.py'
alias pcx-simple='python3 /app/pcx/pcx_runner.py run simple'
alias pcx-medium='python3 /app/pcx/pcx_runner.py run medium'
alias pcx-complex='python3 /app/pcx/pcx_runner.py run complex'
alias pcx-all='python3 /app/pcx/pcx_runner.py run all'

# PCx analysis
alias pcx-results='python3 /app/pcx/pcx_runner.py results'
alias pcx-compare='python3 /app/pcx/pcx_runner.py compare'
alias pcx-analyze='python3 /app/pcx/pcx_runner.py analyze'
alias pcx-export='python3 /app/pcx/pcx_runner.py export'

# Initialize PCx schema in Memgraph
pcx-init() {
    cat /app/pcx/pcx_schema.cypher | docker exec -i urp-memgraph mgconsole
    echo "PCx schema initialized in Memgraph"
}

# ═══════════════════════════════════════════════════════════════════════════════
# Tutorial interactivo
# ═══════════════════════════════════════════════════════════════════════════════

alias urp-tutorial='python3 /app/tutorial/tutorial.py'
alias urp-tutorial-md='cat /app/tutorial/TUTORIAL.md | less'

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
