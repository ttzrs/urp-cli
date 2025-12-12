#!/bin/bash

# Test script for URP Tool Execution Fixes
# Usage: ./TEST_TOOL_FIXES.sh

set -e

echo "================================================"
echo "URP Tool Execution Diagnostic Test"
echo "================================================"
echo ""

# Check if binary exists
if [ ! -f "go/urp" ]; then
    echo "❌ Binary not found. Building..."
    cd go && go build -o urp ./cmd/urp && cd ..
fi

echo "✓ Binary ready: go/urp"
echo ""

# Test 1: Check proxy configuration
echo "Test #1: Verify Proxy Configuration"
echo "-----------------------------------------"
if [ -z "$PROXY_API_KEY" ]; then
    echo "⚠️  PROXY_API_KEY not set"
else
    echo "✓ PROXY_API_KEY is set (length: ${#PROXY_API_KEY})"
fi

if [ -z "$PROXY_BASE_URL" ]; then
    echo "⚠️  PROXY_BASE_URL not set"
else
    echo "✓ PROXY_BASE_URL: $PROXY_BASE_URL"
fi

if [ -z "$OPENAI_API_KEY" ]; then
    echo "⚠️  OPENAI_API_KEY not set (good if using proxy)"
else
    echo "⚠️  OPENAI_API_KEY is set - might override PROXY_API_KEY!"
fi

echo ""

# Test 2: Check Memgraph connectivity
echo "Test #2: Check Memgraph Connectivity"
echo "-----------------------------------------"
if ./go/urp doctor 2>&1 | grep -q "Memgraph.*running"; then
    echo "✓ Memgraph is running"
else
    echo "⚠️  Memgraph might not be running. Start with: docker compose up -d memgraph"
fi

echo ""

# Test 3: Run with debug output to see provider initialization
echo "Test #3: Provider Initialization (with DEBUG output)"
echo "-----------------------------------------"
echo "Running: urp --version 2>&1 | grep -E '\[DEBUG\]|Provider'"
echo ""

# Capture output
OUTPUT=$(cd go && URP_MASTER_MODEL_ID="gpt-5.1-codex" ./urp --version 2>&1 || true)

if echo "$OUTPUT" | grep -q "\[DEBUG\] Bootstrap"; then
    echo "✓ Bootstrap debug output detected:"
    echo "$OUTPUT" | grep "\[DEBUG\] Bootstrap" || true
    echo "$OUTPUT" | grep "\[DEBUG\] Initialize" | head -3 || true
else
    echo "⚠️  No bootstrap debug output. Check if running in proper mode."
fi

echo ""

# Test 4: Quick functional test with simple command
echo "Test #4: Simple Tool Execution Test"
echo "-----------------------------------------"
echo "Testing 'urp code stats' (quick operation)..."
echo ""

if timeout 10 ./go/urp code stats /etc 2>&1 | head -20; then
    echo ""
    echo "✓ Basic command executed"
else
    echo "⚠️  Command might have failed or timed out"
fi

echo ""
echo "================================================"
echo "Diagnostic Summary"
echo "================================================"
echo ""
echo "After applying fixes FIX #1-#4, you should see:"
echo ""
echo "1. [DEBUG] unified provider: endpoint=... messages"
echo "   - Shows actual endpoint being used"
echo ""
echo "2. [ERROR] unified: Failed to parse tool arguments..."
echo "   - If JSON arguments are malformed"
echo "   - Shows raw JSON that failed"
echo ""
echo "3. [WARN] Tool call #X (...) has no arguments"
echo "   - If streaming failed to accumulate arguments"
echo ""
echo "4. [DEBUG] Bootstrap: Using PROXY_API_KEY..."
echo "   - Confirms proxy credentials are being used"
echo ""
echo "Next Steps:"
echo "-----------"
echo "1. Run your typical failing operation and capture stderr:"
echo "   urp --tui 2>&1 | tee tool_test.log"
echo ""
echo "2. Share the error messages starting with [ERROR]"
echo "3. Check if endpoint looks correct for your proxy"
echo ""
echo "================================================"
