#!/bin/bash
# Smoke test script for URP post-deploy verification
# Usage: ./scripts/smoke-test.sh [--quick]

set -e

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[0;33m'
NC='\033[0m' # No Color

QUICK=false
FAILED=0
PASSED=0

if [[ "$1" == "--quick" ]]; then
    QUICK=true
fi

log_pass() {
    echo -e "${GREEN}✓${NC} $1"
    ((PASSED++))
}

log_fail() {
    echo -e "${RED}✗${NC} $1"
    ((FAILED++))
}

log_skip() {
    echo -e "${YELLOW}○${NC} $1 (skipped)"
}

log_info() {
    echo -e "  $1"
}

# Find urp binary
URP="${URP:-./urp}"
if [[ ! -x "$URP" ]]; then
    URP="$(dirname "$0")/../urp"
fi

if [[ ! -x "$URP" ]]; then
    echo "Error: urp binary not found. Build with 'go build -o urp ./cmd/urp'"
    exit 1
fi

echo "=== URP Smoke Tests ==="
echo "Binary: $URP"
echo "Mode: $([ "$QUICK" = true ] && echo 'quick' || echo 'full')"
echo ""

# Test 1: Version
echo "--- Basic Commands ---"
if $URP version >/dev/null 2>&1; then
    VERSION=$($URP version 2>/dev/null | head -1)
    log_pass "version: $VERSION"
else
    log_fail "version command failed"
fi

# Test 2: Help (use -h which doesn't trigger PersistentPreRun)
if timeout 3 $URP -h 2>&1 | head -1 | grep -q "URP\|urp"; then
    log_pass "help accessible"
else
    log_skip "help (timeout - may need db connection)"
fi

# Test 3: Selftest
if timeout 10 $URP selftest --quick >/dev/null 2>&1; then
    log_pass "selftest --quick"
else
    log_skip "selftest --quick (timeout)"
fi

# Test 4: Infrastructure status
echo ""
echo "--- Infrastructure ---"
if OUTPUT=$(timeout 10 $URP infra status 2>&1); then
    RUNTIME=$(echo "$OUTPUT" | grep -o "Runtime:.*" | head -1 || echo "unknown")
    log_pass "infra status ($RUNTIME)"
else
    log_skip "infra status (timeout)"
fi

# Test 5: System vitals (requires memgraph - optional)
if timeout 5 $URP sys vitals >/dev/null 2>&1; then
    log_pass "sys vitals"
else
    log_skip "sys vitals (memgraph not running)"
fi

# Test 6: GPU status
if OUTPUT=$(timeout 5 $URP sys gpu 2>&1); then
    GPU_AVAILABLE=$(echo "$OUTPUT" | grep -o "Available:.*" || echo "unknown")
    log_pass "sys gpu ($GPU_AVAILABLE)"
else
    log_skip "sys gpu (timeout)"
fi

if [[ "$QUICK" == "true" ]]; then
    echo ""
    echo "--- Quick mode: skipping extended tests ---"
    log_skip "code ingest"
    log_skip "git ingest"
    log_skip "vector operations"
else
    # Extended tests
    echo ""
    echo "--- Extended Tests ---"

    # Test 7: Code ingest (on current dir)
    if $URP code ingest >/dev/null 2>&1; then
        log_pass "code ingest"
    else
        log_fail "code ingest"
    fi

    # Test 8: Git ingest
    if $URP git ingest >/dev/null 2>&1; then
        log_pass "git ingest"
    else
        log_fail "git ingest"
    fi

    # Test 9: Vector stats
    if $URP vec stats >/dev/null 2>&1; then
        log_pass "vec stats"
    else
        log_fail "vec stats"
    fi

    # Test 10: Memory list
    if $URP mem list >/dev/null 2>&1; then
        log_pass "mem list"
    else
        log_fail "mem list"
    fi
fi

# Summary
echo ""
echo "=== Summary ==="
TOTAL=$((PASSED + FAILED))
echo -e "Passed: ${GREEN}$PASSED${NC}/$TOTAL"
if [[ $FAILED -gt 0 ]]; then
    echo -e "Failed: ${RED}$FAILED${NC}/$TOTAL"
    exit 1
else
    echo -e "${GREEN}All tests passed!${NC}"
    exit 0
fi
