# URP Tool Execution Fixes - Applied

## Summary of Changes

Applied 4 critical fixes to resolve tool execution failures with OpenAI-compatible providers (GPT-5.1-CODEX via proxy).

---

## Fixes Applied

### ✅ FIX #1: Silent JSON Parse Error Handling
**File:** `go/internal/opencode/provider/unified.go:413-423`

**Problem:** Tool arguments were failing to parse silently, causing nil args to be passed to executor
- `json.Unmarshal([]byte(tc.Function.Arguments), &args)` had no error checking
- If parsing failed, `args` was nil but execution continued
- Tool executed with empty arguments, causing validation failures

**Solution:** Added error handling and logging
```go
if err := json.Unmarshal([]byte(tc.Function.Arguments), &args); err != nil {
    fmt.Fprintf(os.Stderr, "[ERROR] unified: Failed to parse tool arguments for '%s': %v\nRaw JSON: %s\n",
        tc.Function.Name, err, tc.Function.Arguments)
    args = map[string]any{
        "_parse_error": fmt.Sprintf("JSON unmarshal failed: %v", err),
        "_raw_json":    tc.Function.Arguments,
    }
}
```

**Benefit:** You will NOW SEE the actual JSON error instead of silent failure

---

### ✅ FIX #2: Tool Arguments Validation Before Execution
**File:** `go/internal/opencode/agent/agent.go:627-643`

**Problem:** Empty or nil arguments were not caught before tool execution
- Executor was called even with `Args == nil`
- Tool validation failures were unclear
- No warning that streaming failed

**Solution:** Added pre-execution validation
```go
for i, tc := range toolCalls {
    if tc.Args == nil || len(tc.Args) == 0 {
        fmt.Fprintf(os.Stderr,
            "[WARN] Tool call #%d (%s) has no arguments. Check provider streaming.\n",
            i, tc.Name)
    }
    // ... check Name and ToolID too
}
```

**Benefit:** You'll see a warning if arguments are missing, making the problem obvious

---

### ✅ FIX #3: Provider Configuration Priority (Proxy Credentials)
**File:** `go/internal/bootstrap/bootstrap.go:69-122`

**Problem:** ConfigOption overwrites instead of merging
- Multiple `WithAPIKey()` calls meant last one wins
- Proxy credentials were being overwritten by other API keys
- Bootstrap didn't even include proxy options in original code

**Solution:** Build provider options with proper precedence
```go
// Priority: Proxy > DeepSeek > OpenAI > Anthropic
// We pass them in reverse priority so proxy overrides others
providerOpts := []provider.ConfigOption{}

// Lowest priority: Anthropic
if key := os.Getenv("ANTHROPIC_API_KEY"); key != "" {
    providerOpts = append(providerOpts, provider.WithAPIKey(key))
}
// ... (OpenAI, DeepSeek)

// Proxy (HIGHEST priority - overrides everything if set)
if key := os.Getenv("PROXY_API_KEY"); key != "" {
    providerOpts = append(providerOpts, provider.WithAPIKey(key))
    fmt.Printf("[DEBUG] Bootstrap: Using PROXY_API_KEY for authentication\n")
}
// ... etc
```

**Benefit:** Proxy credentials will now be used correctly and you'll see debug output confirming it

---

### ✅ FIX #4: Provider Endpoint Logging
**File:** `go/internal/opencode/provider/unified.go:56-57`

**Problem:** No visibility into which endpoint was actually being used
- Endpoint normalization was happening silently
- If proxy used different path format, nobody would know

**Solution:** Added debug logging of actual endpoint
```go
// FIX #4: Log the actual endpoint being used (for debugging tool failures)
fmt.Fprintf(os.Stderr, "[DEBUG] unified provider: endpoint=%s\n", baseURL)
```

**Benefit:** You'll see `[DEBUG] unified provider: endpoint=http://tizz.win:8317/v1/chat/completions` (or actual URL)

---

## Build Status

✅ **Compilation:** Successful
✅ **Tests:** Passing (provider tests)
✅ **Binary:** `go/urp` (ready to use)

```bash
cd go && go build -o urp ./cmd/urp
go test ./...
```

---

## Testing Instructions

### Step 1: Configure Proxy Environment

Set your proxy credentials:
```bash
# For HTTP proxy gateway
export PROXY_API_KEY="your-api-key"
export PROXY_BASE_URL="http://tizz.win:8317"

# Or if using OpenRouter style
export OPENAI_API_KEY="sk-or-v1-..."
export OPENAI_BASE_URL="https://openrouter.ai/api/v1"
```

### Step 2: Start Infrastructure

```bash
# Ensure Memgraph is running
docker compose up -d memgraph

# Verify it's healthy
docker ps | grep memgraph
urp doctor
```

### Step 3: Run Diagnostic Test

```bash
./TEST_TOOL_FIXES.sh
```

This will:
1. Check proxy configuration
2. Verify Memgraph connectivity
3. Show provider initialization debug output
4. Run a quick sanity test

### Step 4: Test Tool Execution

Run your failing scenario and look for error messages:

```bash
# Option A: Interactive mode
urp --tui 2>&1 | tee urp_debug.log

# Option B: Specific test
urp compile --goal "read /etc/hostname" 2>&1 | tee urp_debug.log
```

### Step 5: Analyze Debug Output

Look for these new messages in stderr:

```
[DEBUG] Bootstrap: Using PROXY_API_KEY for authentication
[DEBUG] Bootstrap: Using PROXY_BASE_URL endpoint
[DEBUG] unified provider: endpoint=http://tizz.win:8317/v1/chat/completions
[DEBUG] Initialize: Provider initialized with resolved model ID 'gpt-5.1-codex' (Provider: unified)
```

If tool call fails:
```
[ERROR] unified: Failed to parse tool arguments for 'bash': ...
Raw JSON: {...incomplete...}
```

Or:
```
[WARN] Tool call #0 (bash) has no arguments. Check provider streaming.
```

---

## Expected Behavior Changes

### Before Fixes
```
$ urp --tui
[Using gpt-5.1-codex]
> read /etc/hostname
[Silent failure - command hangs or returns nothing]
```

### After Fixes
```
$ urp --tui 2>&1
[DEBUG] Bootstrap: Using PROXY_API_KEY for authentication
[DEBUG] Bootstrap: Using PROXY_BASE_URL endpoint
[DEBUG] Initialize: Provider initialized with resolved model ID 'gpt-5.1-codex' (Provider: unified)
[DEBUG] unified provider: endpoint=http://tizz.win:8317/v1/chat/completions

> read /etc/hostname
[If streaming fails]
[ERROR] unified: Failed to parse tool arguments for 'bash': unexpected end of JSON input
Raw JSON: {"command": "read /etc/hostname"

[If everything works]
hostname
```

---

## Troubleshooting

### Problem: Still seeing tool failures

**Check 1: Proxy Endpoint**
```bash
echo "Your endpoint should normalize to:"
echo "http://tizz.win:8317/v1/chat/completions"
# Verify it matches what you see in [DEBUG] output
```

**Check 2: API Key is set**
```bash
echo "PROXY_API_KEY length: ${#PROXY_API_KEY}"
echo "UNIFIED_API_KEY length: ${#UNIFIED_API_KEY}"
# At least one should be non-zero
```

**Check 3: Model is registered**
```bash
# List available models in registry
go/urp models list 2>&1 | grep "gpt-5.1" || echo "Model not found!"
```

**Check 4: Argument Parsing Error**
```bash
# If you see:
# [ERROR] unified: Failed to parse tool arguments
# The raw JSON shows what proxy is returning
# This tells us proxy is streaming JSON incorrectly
```

### Problem: "Provider initialized with resolved model ID 'gpt-5.1-codex' (Provider: anthropic)"

**This means:** Provider detection fell back to Anthropic (wrong provider)

**Fix:**
1. Ensure `PROXY_BASE_URL` is set
2. Ensure `PROXY_API_KEY` is set (or `UNIFIED_*` variants)
3. Restart the app

---

## Files Modified

1. ✅ `go/internal/opencode/provider/unified.go` (Lines 413-423, 56-57)
   - Added error handling for JSON parse failures
   - Added endpoint logging

2. ✅ `go/internal/opencode/agent/agent.go` (Lines 627-643, import os)
   - Added tool argument validation before execution
   - Added import for stderr logging

3. ✅ `go/internal/bootstrap/bootstrap.go` (Lines 69-122)
   - Fixed provider option priority
   - Added PROXY_API_KEY and UNIFIED_API_KEY support
   - Added debug logging for configuration

---

## Additional Documentation

See also:
- `TOOL_FAILURE_DIAGNOSIS.md` - Detailed analysis of the issues
- `CLAUDE.md` - Architecture guide for future development
- `docs/ARCHITECTURE.md` - System architecture overview

---

## Commit Recommendations

These fixes should be committed as:

```
fix: add error handling and logging for tool argument parsing

- unified.go: Add error capture for JSON unmarshal failures
  Tool arguments that fail to parse now show actual error + raw JSON

- agent.go: Add validation that tool arguments are present before execution
  Catches streaming failures early with clear warnings

- bootstrap.go: Fix provider credential merging
  PROXY_API_KEY and UNIFIED_API_KEY now properly override other keys
  Added debug logging to confirm which credentials are being used

Fixes tool execution failures when using OpenAI-compatible providers
through HTTP proxies (e.g., GPT-5.1-CODEX via http://tizz.win:8317/)
```

---

## Questions?

If tool execution still fails after applying these fixes:

1. Share the `[ERROR]` or `[WARN]` messages from stderr
2. Show output of `urp doctor`
3. Confirm `PROXY_API_KEY` and `PROXY_BASE_URL` are set correctly
4. Check that Memgraph is running and healthy

The error messages will now be VISIBLE instead of silent failures, making it much easier to diagnose the actual problem.
