#!/bin/bash

# Complete URP Tool Execution Testing with Real Proxy
# This script walks you through all necessary steps

set -e

echo "╔════════════════════════════════════════════════════════════════╗"
echo "║     URP Tool Execution - Real Proxy Testing                   ║"
echo "║     Status: Ready to diagnose gpt-5.1-codex failures          ║"
echo "╚════════════════════════════════════════════════════════════════╝"
echo ""

# Step 1: Collect configuration from user
echo "STEP 1: Configure Your Proxy"
echo "────────────────────────────────────────────────────────────────"
echo ""

read -p "Enter your PROXY_API_KEY (or press Enter if using environment variable): " USER_API_KEY
if [ -n "$USER_API_KEY" ]; then
    export PROXY_API_KEY="$USER_API_KEY"
fi

read -p "Enter your PROXY_BASE_URL (e.g., http://tizz.win:8317): " USER_BASE_URL
if [ -n "$USER_BASE_URL" ]; then
    export PROXY_BASE_URL="$USER_BASE_URL"
fi

echo ""
echo "Configuration Summary:"
echo "  PROXY_API_KEY: ${PROXY_API_KEY:0:20}***"
echo "  PROXY_BASE_URL: $PROXY_BASE_URL"
echo ""

# Step 2: Verify prerequisites
echo "STEP 2: Verify Prerequisites"
echo "────────────────────────────────────────────────────────────────"
echo ""

# Check if binary exists
if [ ! -f "go/urp" ]; then
    echo "❌ Binary not found, rebuilding..."
    cd go && go build -o urp ./cmd/urp && cd ..
    echo "✅ Binary built"
else
    echo "✅ Binary exists"
fi

# Check Memgraph
if docker ps | grep -q "urp-memgraph"; then
    echo "✅ Memgraph is running"
else
    echo "⏳ Starting Memgraph..."
    docker compose up -d memgraph
    sleep 3
    if docker ps | grep -q "urp-memgraph"; then
        echo "✅ Memgraph started successfully"
    else
        echo "❌ Failed to start Memgraph"
        exit 1
    fi
fi

# Check API key is set
if [ -z "$PROXY_API_KEY" ]; then
    echo "⚠️  WARNING: PROXY_API_KEY not set!"
    echo "   The tests may fail if proxy requires authentication"
fi

echo ""
echo "STEP 3: Run Diagnostic Test"
echo "────────────────────────────────────────────────────────────────"
echo ""
echo "This will show if bootstrap is loading proxy configuration correctly..."
echo ""

# Run diagnostic with captured output
DIAG_OUTPUT=$(PROXY_API_KEY="$PROXY_API_KEY" \
              PROXY_BASE_URL="$PROXY_BASE_URL" \
              URP_MASTER_MODEL_ID="gpt-5.1-codex" \
              ./go/urp version 2>&1 || true)

echo "$DIAG_OUTPUT"
echo ""

# Step 4: Interactive test
echo "STEP 4: Interactive Testing"
echo "────────────────────────────────────────────────────────────────"
echo ""
echo "Starting interactive TUI with proxy configuration..."
echo ""
echo "⚠️  IMPORTANT INSTRUCTIONS:"
echo "  1. When the TUI opens, you're in the interactive mode"
echo "  2. Try these tool commands:"
echo "     - 'read /etc/hostname'"
echo "     - 'list files in /tmp'"
echo "     - Any command that uses bash tool"
echo ""
echo "  3. OBSERVE the error output:"
echo "     - [DEBUG] messages show configuration loaded"
echo "     - [ERROR] shows JSON parsing failures"
echo "     - [WARN] shows missing arguments"
echo ""
echo "  4. Exit with Ctrl+C when done"
echo ""
echo "─────────────────────────────────────────────────────────────────"
echo ""

# Launch interactive TUI with logging
TUI_LOG="/tmp/urp_interactive_test_$(date +%s).log"
echo "Output will be saved to: $TUI_LOG"
echo ""

PROXY_API_KEY="$PROXY_API_KEY" \
PROXY_BASE_URL="$PROXY_BASE_URL" \
URP_MASTER_MODEL_ID="gpt-5.1-codex" \
./go/urp --tui 2>&1 | tee "$TUI_LOG"

echo ""
echo "STEP 5: Analyze Results"
echo "────────────────────────────────────────────────────────────────"
echo ""

# Check for debug messages
if grep -q "Bootstrap.*PROXY" "$TUI_LOG"; then
    echo "✅ Proxy credentials detected in bootstrap"
else
    echo "⚠️  No proxy configuration messages found"
fi

if grep -q "unified provider: endpoint" "$TUI_LOG"; then
    echo "✅ Provider endpoint was logged"
    echo "   Endpoint: $(grep 'unified provider: endpoint' "$TUI_LOG" | head -1 | sed 's/.*endpoint=//')"
else
    echo "⚠️  No provider endpoint logged"
fi

if grep -q "\[ERROR\] unified" "$TUI_LOG"; then
    echo "❌ JSON parsing errors detected:"
    grep "\[ERROR\] unified" "$TUI_LOG" | head -3
else
    echo "✅ No JSON parsing errors detected"
fi

if grep -q "\[WARN\].*no arguments" "$TUI_LOG"; then
    echo "⚠️  Tools executed with missing arguments"
else
    echo "✅ Tool arguments present"
fi

echo ""
echo "════════════════════════════════════════════════════════════════"
echo "Test Complete!"
echo "════════════════════════════════════════════════════════════════"
echo ""
echo "Log file saved to: $TUI_LOG"
echo ""
echo "Next steps based on results:"
echo ""
echo "✅ If tools worked:"
echo "   Congratulations! gpt-5.1-codex is now working with your proxy."
echo ""
echo "❌ If you see [ERROR] messages:"
echo "   1. Check the error message - it shows what JSON failed"
echo "   2. Review TOOL_FAILURE_DIAGNOSIS.md for guidance"
echo "   3. Verify PROXY_BASE_URL format matches proxy expectations"
echo ""
echo "⚠️  If you see [WARN] messages:"
echo "   1. Tool streaming is incomplete"
echo "   2. Check proxy logs for streaming issues"
echo "   3. Proxy might be rate-limiting or timing out"
echo ""
echo "📊 Useful commands for analysis:"
echo ""
echo "   # See all debug messages"
echo "   grep DEBUG $TUI_LOG"
echo ""
echo "   # See all errors"
echo "   grep ERROR $TUI_LOG"
echo ""
echo "   # See proxy configuration messages"
echo "   grep 'PROXY_API_KEY\|PROXY_BASE_URL\|endpoint' $TUI_LOG"
echo ""
echo "════════════════════════════════════════════════════════════════"
