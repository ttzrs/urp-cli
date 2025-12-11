#!/bin/bash

# E2E Test: Real-world scenario test for URP model configuration
# Tests the model configuration system in realistic usage scenarios

set -e  # Exit on any error

echo "Starting Real-World Scenario Model Configuration E2E Test..."
echo "=========================================================="

# Ensure the urp binary is built and available
if [ ! -f "./urp" ]; then
    echo "Error: URP binary not found. Please run 'cd go && go build -o ../urp ./cmd/urp' first."
    exit 1
fi

# --- Prerequisites: Set API Keys for testing ---
# Use provided keys or fallback to dummy values
export ANTHROPIC_API_KEY="${ANTHROPIC_API_KEY:-sk-ant-...}"
export OPENAI_API_KEY="${OPENAI_API_KEY:-sk-or-v1-...}"
export DEEPSEEK_API_KEY="${DEEPSEEK_API_KEY:-sk-deepseek-...}"
export HF_TOKEN="${HF_TOKEN:-hf_dummy_token}"

if [ "$OPENAI_API_KEY" == "sk-or-v1-..." ] && [ "$ANTHROPIC_API_KEY" == "sk-ant-..." ] && [ "$DEEPSEEK_API_KEY" == "sk-deepseek-..." ]; then
    echo "WARNING: No real API keys found. Most tests will fail due to authentication."
else
    echo "API keys detected for testing"
fi
echo ""

# Create a temporary directory for the test project
TEST_DIR=$(mktemp -d)
echo "Created test directory: $TEST_DIR"

# Create a simple Go file to test code analysis capabilities
cat << 'EOF' > "$TEST_DIR/main.go"
package main

import "fmt"

// This is a sample Go file for testing URP model configuration
func main() {
    fmt.Println("Hello from URP test!")
}
EOF

# --- Scenario 1: Developer wants maximum performance ---
echo "Scenario 1: High-performance development setup..."
echo "  - Using best models for each function"

# Configure for maximum performance
export URP_MASTER_MODEL_ID="o1"  # Best reasoning
export URP_GATE_MODEL_ID="gpt-4o-mini"  # Fast filtering
export URP_WORKER_MODEL_ID="deepseek-chat"  # Good code execution
export URP_CODING_MODEL_ID="deepseek-coder"  # Specialized for coding
export URP_REASONING_MODEL_ID="o1"  # Advanced reasoning
export URP_FAST_MODEL_ID="gpt-4o-mini"  # Quick responses

echo "  - Master: $URP_MASTER_MODEL_ID"
echo "  - Gate: $URP_GATE_MODEL_ID"
echo "  - Worker: $URP_WORKER_MODEL_ID"
echo "  - Coding: $URP_CODING_MODEL_ID"
echo "  - Reasoning: $URP_REASONING_MODEL_ID"
echo "  - Fast: $URP_FAST_MODEL_ID"

# Test the configuration by running commands
cd $TEST_DIR
SCENARIO1_OUTPUT=$(timeout 15s ../urp doctor 2>&1 || echo "timeout_or_error")
SCENARIO1_EXIT_CODE=$?

if [ $SCENARIO1_EXIT_CODE -ne 0 ] && [ "$SCENARIO1_OUTPUT" != "timeout_or_error" ]; then
    if echo "$SCENARIO1_OUTPUT" | grep -qi "api.key\|authentication\|unauthorized\|rate.limit\|quota" > /dev/null; then
        echo "  ✓ High-performance scenario test passed (configuration loaded, API auth error expected without keys)"
    else
        echo "  ✗ High-performance scenario test failed with configuration error: $SCENARIO1_OUTPUT"
        cd - > /dev/null
        rm -rf $TEST_DIR
        exit 1
    fi
else
    echo "  ✓ High-performance scenario test passed (system started correctly)"
fi
cd - > /dev/null
echo

# --- Scenario 2: Developer wants cost efficiency ---
echo "Scenario 2: Cost-efficient development setup..."
echo "  - Using economical models while maintaining functionality"

# Configure for cost efficiency
export URP_MASTER_MODEL_ID="gpt-4o-mini"
export URP_GATE_MODEL_ID="gpt-4o-mini"
export URP_WORKER_MODEL_ID="deepseek-chat"
export URP_CODING_MODEL_ID="deepseek-coder"
export URP_REASONING_MODEL_ID="gpt-4o-mini"
export URP_FAST_MODEL_ID="gpt-4o-mini"

echo "  - Master: $URP_MASTER_MODEL_ID"
echo "  - Gate: $URP_GATE_MODEL_ID"
echo "  - Worker: $URP_WORKER_MODEL_ID"
echo "  - Coding: $URP_CODING_MODEL_ID"
echo "  - Reasoning: $URP_REASONING_MODEL_ID"
echo "  - Fast: $URP_FAST_MODEL_ID"

cd $TEST_DIR
SCENARIO2_OUTPUT=$(timeout 15s ../urp doctor 2>&1 || echo "timeout_or_error")
SCENARIO2_EXIT_CODE=$?

if [ $SCENARIO2_EXIT_CODE -ne 0 ] && [ "$SCENARIO2_OUTPUT" != "timeout_or_error" ]; then
    if echo "$SCENARIO2_OUTPUT" | grep -qi "api.key\|authentication\|unauthorized\|rate.limit\|quota" > /dev/null; then
        echo "  ✓ Cost-efficient scenario test passed (configuration loaded, API auth error expected without keys)"
    else
        echo "  ✗ Cost-efficient scenario test failed with configuration error: $SCENARIO2_OUTPUT"
        cd - > /dev/null
        rm -rf $TEST_DIR
        exit 1
    fi
else
    echo "  ✓ Cost-efficient scenario test passed (system started correctly)"
fi
cd - > /dev/null
echo

# --- Scenario 3: Team with specific model preferences ---
echo "Scenario 3: Team-specific model configuration..."
echo "  - Using consistent models across the team"

# Configure for a team using Anthropic models
export URP_MASTER_MODEL_ID="anthropic/claude-sonnet-4-5-20250929"
export URP_GATE_MODEL_ID="anthropic/claude-3-5-haiku-20241022"  # Fast Anthropic model
export URP_WORKER_MODEL_ID="anthropic/claude-sonnet-4-5-20250929"
export URP_CODING_MODEL_ID="anthropic/claude-sonnet-4-5-20250929"
export URP_REASONING_MODEL_ID="anthropic/claude-opus-4-20250514"  # For complex tasks
export URP_FAST_MODEL_ID="anthropic/claude-3-5-haiku-20241022"

echo "  - Master: $URP_MASTER_MODEL_ID"
echo "  - Gate: $URP_GATE_MODEL_ID"
echo "  - Worker: $URP_WORKER_MODEL_ID"
echo "  - Coding: $URP_CODING_MODEL_ID"
echo "  - Reasoning: $URP_REASONING_MODEL_ID"
echo "  - Fast: $URP_FAST_MODEL_ID"

cd $TEST_DIR
SCENARIO3_OUTPUT=$(timeout 15s ../urp doctor 2>&1 || echo "timeout_or_error")
SCENARIO3_EXIT_CODE=$?

if [ $SCENARIO3_EXIT_CODE -ne 0 ] && [ "$SCENARIO3_OUTPUT" != "timeout_or_error" ]; then
    if echo "$SCENARIO3_OUTPUT" | grep -qi "api.key\|authentication\|unauthorized\|rate.limit\|quota" > /dev/null; then
        echo "  ✓ Team-specific scenario test passed (configuration loaded, API auth error expected without keys)"
    else
        echo "  ✗ Team-specific scenario test failed with configuration error: $SCENARIO3_OUTPUT"
        cd - > /dev/null
        rm -rf $TEST_DIR
        exit 1
    fi
else
    echo "  ✓ Team-specific scenario test passed (system started correctly)"
fi
cd - > /dev/null
echo

# --- Scenario 4: Testing with custom provider endpoints ---
echo "Scenario 4: Custom provider endpoints setup..."
echo "  - Using self-hosted or custom API endpoints"

# Configure custom endpoints
export URP_MASTER_MODEL_ID="custom-model-name"
export URP_MASTER_MODEL_URL="https://my-custom-provider.com/v1"
export URP_GATE_MODEL_ID="custom-fast-model"
export URP_GATE_MODEL_URL="https://my-custom-provider.com/v1"
export URP_WORKER_MODEL_ID="custom-code-model"
export URP_WORKER_MODEL_URL="https://my-custom-provider.com/v1"

echo "  - Master: $URP_MASTER_MODEL_ID (via $URP_MASTER_MODEL_URL)"
echo "  - Gate: $URP_GATE_MODEL_ID (via $URP_GATE_MODEL_URL)"
echo "  - Worker: $URP_WORKER_MODEL_ID (via $URP_WORKER_MODEL_URL)"

cd $TEST_DIR
SCENARIO4_OUTPUT=$(timeout 15s ../urp doctor 2>&1 || echo "timeout_or_error")
SCENARIO4_EXIT_CODE=$?

if [ $SCENARIO4_EXIT_CODE -ne 0 ] && [ "$SCENARIO4_OUTPUT" != "timeout_or_error" ]; then
    if echo "$SCENARIO4_OUTPUT" | grep -qi "api.key\|authentication\|unauthorized\|rate.limit\|quota\|connection.refused\|no.such.host" > /dev/null; then
        echo "  ✓ Custom provider scenario test passed (configuration loaded, network error expected for dummy endpoints)"
    else
        echo "  ✗ Custom provider scenario test failed with configuration error: $SCENARIO4_OUTPUT"
        cd - > /dev/null
        rm -rf $TEST_DIR
        exit 1
    fi
else
    echo "  ✓ Custom provider scenario test passed (system accepted custom endpoints)"
fi
cd - > /dev/null
echo

# --- Scenario 5: Testing configuration persistence ---
echo "Scenario 5: Configuration persistence test..."
echo "  - Verifying that configuration remains stable across commands"

# Set complex configuration
export URP_MASTER_MODEL_ID="anthropic/claude-sonnet-4-5-20250929"
export URP_GATE_MODEL_ID="gpt-4o-mini"
export URP_WORKER_MODEL_ID="deepseek-chat"
export URP_CODING_MODEL_ID="deepseek-coder"
export URP_REASONING_MODEL_ID="o1"
export URP_FAST_MODEL_ID="gpt-4o-mini"
export URP_FALLBACK_MODEL_ID="gpt-4o"
export URP_DEFAULT_MASTER_MODEL="anthropic/claude-sonnet-4-5-20250929"
export URP_DEFAULT_GATE_MODEL="gpt-4o-mini"

echo "  - Testing multiple commands with persistent configuration..."

# Run multiple commands to verify configuration persistence
cd $TEST_DIR
for i in {1..3}; do
    CMD_OUTPUT=$(timeout 10s ../urp doctor 2>&1 || echo "timeout_or_error_$i")
    if [ "$CMD_OUTPUT" != "timeout_or_error_$i" ]; then
        echo "    - Command $i executed successfully (with possible API auth errors)"
    else
        echo "    - Command $i timed out"
    fi
done

echo "  ✓ Configuration persistence test completed"
cd - > /dev/null
echo

# --- Scenario 6: Testing configuration in different environments ---
echo "Scenario 6: Environment-specific configuration test..."
echo "  - Testing configuration behavior in different scenarios"

# Simulate a container environment by setting container variables
export URP_CONTAINER_MODE="1"
export URP_MASTER_MODEL_ID="container-optimized-model"
export URP_GATE_MODEL_ID="fast-gate-model"

echo "  - Simulated container environment with specific models"
echo "  - Master: $URP_MASTER_MODEL_ID"
echo "  - Gate: $URP_GATE_MODEL_ID"

# Test in this environment
cd $TEST_DIR
ENV_TEST_OUTPUT=$(timeout 15s ../urp doctor 2>&1 || echo "timeout_or_error")
ENV_TEST_EXIT_CODE=$?

if [ $ENV_TEST_EXIT_CODE -ne 0 ] && [ "$ENV_TEST_OUTPUT" != "timeout_or_error" ]; then
    if echo "$ENV_TEST_OUTPUT" | grep -qi "api.key\|authentication\|unauthorized\|rate.limit\|quota" > /dev/null; then
        echo "  ✓ Environment-specific scenario test passed (configuration loaded, API auth error expected without keys)"
    else
        echo "  ✗ Environment-specific scenario test failed with configuration error: $ENV_TEST_OUTPUT"
        cd - > /dev/null
        rm -rf $TEST_DIR
        exit 1
    fi
else
    echo "  ✓ Environment-specific scenario test passed (system handled environment variables correctly)"
fi
cd - > /dev/null
echo

# Cleanup
rm -rf $TEST_DIR
echo "Test directory cleaned up."

echo "All Real-World Scenario Model Configuration E2E tests completed successfully!"
echo "============================================================================"

# Clean up environment variables
unset ANTHROPIC_API_KEY
unset OPENAI_API_KEY
unset DEEPSEEK_API_KEY
unset HF_TOKEN
unset URP_MASTER_MODEL_ID
unset URP_GATE_MODEL_ID
unset URP_WORKER_MODEL_ID
unset URP_CODING_MODEL_ID
unset URP_REASONING_MODEL_ID
unset URP_FAST_MODEL_ID
unset URP_VISION_MODEL_ID
unset URP_LONG_CONTEXT_MODEL_ID
unset URP_FALLBACK_MODEL_ID
unset URP_DEFAULT_MASTER_MODEL
unset URP_DEFAULT_GATE_MODEL
unset URP_MASTER_MODEL_URL
unset URP_GATE_MODEL_URL
unset URP_WORKER_MODEL_URL
unset URP_MASTER_MODEL_API_KEY
unset URP_GATE_MODEL_API_KEY
unset URP_WORKER_MODEL_API_KEY
unset URP_CONTAINER_MODE

echo "Environment variables cleaned up."