#!/bin/bash

# Quick test for tool execution fixes
# Run this after setting up environment

echo "🔧 URP Tool Execution Quick Test"
echo "================================"
echo ""

# Set your proxy credentials
export PROXY_API_KEY="${PROXY_API_KEY:-your-key-here}"
export PROXY_BASE_URL="${PROXY_BASE_URL:-http://tizz.win:8317}"

echo "Configuration:"
echo "  PROXY_API_KEY: ${PROXY_API_KEY:0:20}..."
echo "  PROXY_BASE_URL: $PROXY_BASE_URL"
echo ""

# Check if binary exists
if [ ! -f "go/urp" ]; then
    echo "❌ Building URP..."
    cd go && go build -o urp ./cmd/urp && cd ..
fi

# Start Memgraph if not running
if ! docker ps | grep -q urp-memgraph; then
    echo "⏳ Starting Memgraph..."
    docker compose up -d memgraph
    sleep 2
fi

echo "✅ Environment ready"
echo ""
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo ""
echo "Test 1: Check configuration loading"
echo "-----------------------------------"
PROXY_API_KEY="$PROXY_API_KEY" \
PROXY_BASE_URL="$PROXY_BASE_URL" \
URP_MASTER_MODEL_ID="gpt-5.1-codex" \
./go/urp --version 2>&1 | grep -E "\[DEBUG\] Bootstrap|Provider initialized" || echo "(no debug output - check stderr)"

echo ""
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo ""
echo "Test 2: Run compilation test (produces debug output)"
echo "----------------------------------------------------"
echo "Try: urp compile --goal 'list files in /tmp'"
echo ""
echo "You should see:"
echo "  [DEBUG] unified provider: endpoint=..."
echo "  [ERROR] if JSON parsing fails (with raw JSON shown)"
echo "  [WARN] if arguments are missing"
echo ""

echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo ""
echo "✨ Quick test complete!"
echo ""
echo "Next: Run actual tool call in interactive mode"
echo "  URP_MASTER_MODEL_ID='gpt-5.1-codex' ./go/urp --tui 2>&1"
echo ""
echo "Look for [ERROR], [WARN], or [DEBUG] messages in stderr"
