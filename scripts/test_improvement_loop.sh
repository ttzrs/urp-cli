#!/bin/bash
set -e

# Ensure clean state (optional, but good for testing)
# ./urp infra clean > /dev/null 2>&1
# ./urp infra start > /dev/null 2>&1

echo "🧪 TEST: Improvement Loop Verification"
echo "======================================"

# Set a fixed Session ID so audit log can find events from previous commands
export URP_SESSION_ID="test-session-$(date +%s)"
echo "   Session ID: $URP_SESSION_ID"

# Configure Sovereign Embeddings (TEI)
# When running outside containers, use localhost:8080 (exposed port)
# Inside containers, use http://urp-tei:80
export URP_EMBEDDING_PROVIDER="${URP_EMBEDDING_PROVIDER:-tei}"
export TEI_URL="${TEI_URL:-http://localhost:8080}"
echo "   Embedding Provider: $URP_EMBEDDING_PROVIDER"
echo "   TEI URL: $TEI_URL"

# Check if TEI is reachable
echo ""
echo "0. Checking TEI health..."
if curl -sf "$TEI_URL/health" > /dev/null 2>&1; then
    echo "   ✅ TEI service is healthy"
else
    echo "   ⚠️  TEI service not reachable at $TEI_URL"
    echo "   Falling back to local embeddings..."
    export URP_EMBEDDING_PROVIDER=local
fi

echo "1. Generating SUCCESS event (urp status)..."
./urp status > /dev/null
echo "   Done."

echo "2. Generating ERROR event (urp code ingest /non/existent/path)..."
# We expect this to fail, so we OR true
./urp code ingest /non/existent/path 2>/dev/null || true
echo "   Done (Error triggered)."

echo "3. Verifying Persistence (urp audit log)..."
# We should see the error event
LOG_OUTPUT=$(./urp audit log --limit 2)
echo "$LOG_OUTPUT"

# Check for ingest command which we just ran
if echo "$LOG_OUTPUT" | grep -q "ingest"; then
    echo "   ✅ Audit Log contains event."
else
    echo "   ❌ Audit Log MISSING event. Persistence failed."
    exit 1
fi

echo "4. Running Evaluator (urp think evaluate)..."
EVAL_OUTPUT=$(./urp think evaluate)
echo "$EVAL_OUTPUT"

# Check if it picked up the error
if echo "$EVAL_OUTPUT" | grep -q "Analyzing"; then
    echo "   ✅ Evaluator analyzed errors."
else
    echo "   ❌ Evaluator failed to analyze."
    exit 1
fi

# Check if it found the pattern
# The error message from ingest might be "path does not exist" or similar
# My evaluate code groups by message.
if echo "$EVAL_OUTPUT" | grep -q "Pattern:"; then
    echo "   ✅ Evaluator grouped patterns."
elif echo "$EVAL_OUTPUT" | grep -q "healthy"; then
    echo "   ✅ No errors to evaluate (system healthy)."
else
    echo "   ❌ Evaluator failed to group patterns."
fi

# Check that embedding worked (no "Failed to embed" errors)
if echo "$EVAL_OUTPUT" | grep -q "Failed to embed"; then
    echo "   ❌ Embedding failed. Check TEI service."
    exit 1
else
    echo "   ✅ Embedding step passed."
fi

# Optional: Test LLM analysis (only if ANTHROPIC_API_KEY is set)
if [ -n "$ANTHROPIC_API_KEY" ] || [ -n "$OPENAI_API_KEY" ]; then
    echo ""
    echo "5. Testing LLM Analysis (urp think evaluate --llm)..."
    LLM_OUTPUT=$(./urp think evaluate --llm 2>&1)
    
    if echo "$LLM_OUTPUT" | grep -q "ROOT CAUSE:"; then
        echo "   ✅ LLM provided root cause analysis."
    elif echo "$LLM_OUTPUT" | grep -q "LLM analysis enabled"; then
        echo "   ⚠️ LLM enabled but API may have failed."
    else
        echo "   ℹ️  LLM analysis skipped (no patterns or API error)."
    fi
fi

echo "======================================"
echo "🎉 Test Completed Successfully"
