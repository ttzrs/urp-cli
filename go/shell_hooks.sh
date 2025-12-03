#!/bin/bash
# URP Shell Hooks
# ===============
# Transparent command interception for terminal flow capture.
# Commands are logged to the graph database via urp events run.

# Skip if URP hooks are disabled
[[ -n "$URP_HOOKS_DISABLED" ]] && return

# Colors
export URP_COLOR_INFO='\033[0;34m'
export URP_COLOR_WARN='\033[1;33m'
export URP_COLOR_ERROR='\033[0;31m'
export URP_COLOR_SUCCESS='\033[0;32m'
export URP_COLOR_RESET='\033[0m'

# Wrapped commands - these get logged to the graph
URP_WRAPPED_COMMANDS=(
    git docker podman kubectl
    npm pnpm yarn bun
    pip pip3 pipx poetry
    cargo go make cmake
    pytest jest vitest
    terraform ansible
)

# Create wrapper function for a command
_urp_wrap() {
    local cmd="$1"

    # Define wrapper function
    eval "
    $cmd() {
        urp events run $cmd \"\$@\"
    }
    "
}

# Install wrappers for all commands
for cmd in "${URP_WRAPPED_COMMANDS[@]}"; do
    if command -v "$cmd" &>/dev/null; then
        _urp_wrap "$cmd"
    fi
done

# Quick aliases for common URP operations
alias pain='urp events errors'
alias recent='urp events list'
alias vitals='urp sys vitals'
alias topology='urp sys topology'
alias health='urp sys health'
alias wisdom='urp think wisdom'
alias novelty='urp think novelty'
alias learn='urp think learn'
alias remember='urp mem add'
alias recall='urp mem recall'
alias memories='urp mem list'
alias focus='urp focus'

# Status indicator in prompt
_urp_prompt() {
    local exit_code=$?
    local graph_status=""

    # Check graph connection (cached for 60s)
    if [[ -z "$_URP_GRAPH_CHECK" ]] || (( $(date +%s) - _URP_GRAPH_CHECK > 60 )); then
        if urp 2>&1 | grep -q "Graph:.*connected"; then
            _URP_GRAPH_STATUS="+"
        else
            _URP_GRAPH_STATUS="-"
        fi
        _URP_GRAPH_CHECK=$(date +%s)
    fi

    echo -n "[urp:${_URP_GRAPH_STATUS}]"
}

# Add to PS1 if interactive
if [[ $- == *i* ]]; then
    PS1='$(_urp_prompt) \w \$ '
fi

# Control functions
urp-on() {
    unset URP_HOOKS_DISABLED
    source /etc/profile.d/urp-hooks.sh
    echo -e "${URP_COLOR_SUCCESS}URP hooks enabled${URP_COLOR_RESET}"
}

urp-off() {
    export URP_HOOKS_DISABLED=1
    # Unset all wrappers
    for cmd in "${URP_WRAPPED_COMMANDS[@]}"; do
        unset -f "$cmd" 2>/dev/null
    done
    echo -e "${URP_COLOR_WARN}URP hooks disabled${URP_COLOR_RESET}"
}

urp-status() {
    if [[ -n "$URP_HOOKS_DISABLED" ]]; then
        echo -e "${URP_COLOR_WARN}URP hooks: DISABLED${URP_COLOR_RESET}"
    else
        echo -e "${URP_COLOR_SUCCESS}URP hooks: ENABLED${URP_COLOR_RESET}"
    fi

    echo "Wrapped commands:"
    for cmd in "${URP_WRAPPED_COMMANDS[@]}"; do
        if type -t "$cmd" | grep -q function; then
            echo "  + $cmd (wrapped)"
        elif command -v "$cmd" &>/dev/null; then
            echo "  - $cmd (available but not wrapped)"
        fi
    done
}

# Master-specific commands (only available in master containers)
if [[ "$URP_MASTER" == "1" ]]; then
    alias urp-spawn='urp spawn'
    alias urp-workers='urp workers'
    alias urp-attach='urp attach'
    alias urp-kill='urp kill'
    alias urp-kill-all='urp kill --all'

    echo -e "${URP_COLOR_INFO}URP Master Mode${URP_COLOR_RESET}"
    echo "  urp-spawn [n]    - Spawn worker n"
    echo "  urp-workers      - List workers"
    echo "  urp-attach <n>   - Attach to worker"
    echo "  urp-kill <n>     - Kill worker"
fi

# Read-only warning
if [[ "$URP_READ_ONLY" == "true" ]]; then
    echo -e "${URP_COLOR_WARN}Read-only mode - workspace is mounted read-only${URP_COLOR_RESET}"
fi

# Show session info on startup
echo -e "${URP_COLOR_INFO}URP Session: ${URP_PROJECT}${URP_COLOR_RESET}"
