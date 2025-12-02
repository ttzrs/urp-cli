#!/bin/bash
# ═══════════════════════════════════════════════════════════════════════════════
# Claude Token Hook - Intercepts Claude CLI and tracks token usage with model
# ═══════════════════════════════════════════════════════════════════════════════
#
# This wraps the claude CLI to track token consumption from API responses.
# Tracks: input tokens, output tokens, model used, and calculates cost.

# Original claude binary
CLAUDE_BIN="${CLAUDE_BIN:-/usr/bin/claude}"

# Token monitor script (uses pricing_db.py backend)
TOKEN_MONITOR="/app/token_monitor.py"

# Wrapper function that tracks tokens
claude() {
    local start_time=$(date +%s)
    local temp_log=$(mktemp)
    local exit_code=0

    # Run claude with output capture
    if [[ "$1" == "-p" ]] || [[ "$1" == "--print" ]]; then
        # Non-interactive mode - capture output
        command claude "$@" 2>&1 | tee "$temp_log"
        exit_code=${PIPESTATUS[0]}
    else
        # Interactive mode - just run normally
        command claude "$@"
        exit_code=$?
        rm -f "$temp_log"
        return $exit_code
    fi

    # Try to extract token info and model from output
    local input_tokens=0
    local output_tokens=0
    local model="sonnet"  # Default

    # Pattern 1: JSON response with usage block
    # {"usage":{"input_tokens":1234,"output_tokens":567},"model":"claude-3-5-sonnet-20241022"}
    if grep -q '"usage"' "$temp_log" 2>/dev/null; then
        input_tokens=$(grep -o '"input_tokens":[0-9]*' "$temp_log" | head -1 | grep -o '[0-9]*' || echo 0)
        output_tokens=$(grep -o '"output_tokens":[0-9]*' "$temp_log" | head -1 | grep -o '[0-9]*' || echo 0)

        # Extract model from response
        local model_match=$(grep -o '"model":"[^"]*"' "$temp_log" | head -1 | sed 's/"model":"//;s/"//')
        if [[ -n "$model_match" ]]; then
            model="$model_match"
        fi
    fi

    # Pattern 2: Token summary line from Claude Code
    # e.g., "Tokens: 1234 input, 567 output" or "Model: claude-sonnet-4-5"
    if [[ "$input_tokens" == "0" ]] && grep -qi 'tokens' "$temp_log" 2>/dev/null; then
        local token_line=$(grep -i 'token' "$temp_log" | tail -1)
        input_tokens=$(echo "$token_line" | grep -o '[0-9]*\s*input' | grep -o '[0-9]*' || echo 0)
        output_tokens=$(echo "$token_line" | grep -o '[0-9]*\s*output' | grep -o '[0-9]*' || echo 0)
    fi

    # Try to detect model from various patterns
    if [[ "$model" == "sonnet" ]]; then
        # Pattern: "Using model: X" or "Model: X"
        local model_line=$(grep -i 'model' "$temp_log" | head -1)
        if [[ -n "$model_line" ]]; then
            if echo "$model_line" | grep -qi 'opus'; then
                model="opus"
            elif echo "$model_line" | grep -qi 'haiku'; then
                model="haiku"
            elif echo "$model_line" | grep -qi 'sonnet'; then
                model="sonnet"
            fi
        fi

        # Check for full model ID in output
        local full_model=$(grep -o 'claude-[a-z0-9-]*' "$temp_log" | head -1)
        if [[ -n "$full_model" ]]; then
            model="$full_model"
        fi
    fi

    # If we got token data, track it
    if [[ "$input_tokens" -gt 0 ]] || [[ "$output_tokens" -gt 0 ]]; then
        python3 "$TOKEN_MONITOR" track "$input_tokens" "$output_tokens" "$model" "claude-cli" 2>/dev/null
    else
        # Estimate based on output length (fallback)
        local output_len=$(wc -c < "$temp_log" 2>/dev/null || echo 0)
        if [[ "$output_len" -gt 100 ]]; then
            # Rough estimate: 4 chars per token
            local est_output=$((output_len / 4))
            local est_input=$((est_output / 2))  # Assume input was ~half of output
            python3 "$TOKEN_MONITOR" track "$est_input" "$est_output" "$model" "claude-cli-estimated" 2>/dev/null
        fi
    fi

    rm -f "$temp_log"
    return $exit_code
}

# Claude write helper - track writes
claude-write() {
    local temp_log=$(mktemp)
    command claude "$@" 2>&1 | tee "$temp_log"
    local exit_code=${PIPESTATUS[0]}

    # Track as write operation
    local output_len=$(wc -c < "$temp_log" 2>/dev/null || echo 0)
    if [[ "$output_len" -gt 50 ]]; then
        local est_output=$((output_len / 4))
        local est_input=$((est_output / 3))
        python3 "$TOKEN_MONITOR" track "$est_input" "$est_output" "sonnet" "claude-write" 2>/dev/null
    fi

    rm -f "$temp_log"
    return $exit_code
}

# Export for subshells
export -f claude 2>/dev/null || true
export -f claude-write 2>/dev/null || true
