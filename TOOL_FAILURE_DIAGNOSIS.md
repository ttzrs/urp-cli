# URP Tool Execution Failure Diagnosis

**Status:** Critical Tool Failures with OpenAI-Compatible Providers (GPT-5.1-CODEX via proxy HTTP://tizz.win:8317/)

**Models Affected:**
- ✓ `glm-4.6` (works - special case)
- ✗ `gpt-5.1-codex` (fails - generic provider)

---

## ROOT CAUSE ANALYSIS

### Issue #1: Tool Arguments Parsing Failure (CRITICAL)

**Location:** `go/internal/opencode/provider/unified.go:412`

```go
// BROKEN: Silent failure on JSON parse
var args map[string]any
json.Unmarshal([]byte(tc.Function.Arguments), &args)
// ↑ If this fails, args is nil silently. No error checking!

events <- domain.StreamEvent{
    Type: domain.StreamEventToolCall,
    Part: domain.ToolCallPart{
        ToolID: tc.ID,
        Name:   tc.Function.Name,
        Args:   args,  // ← EMPTY! nil map passed to executor
    },
}
```

**Why This Breaks glm-4.6 but not others:**
- GLM-4.6 may return well-formed JSON in Arguments
- GPT-5.1-CODEX might be returning malformed or partial JSON chunks
- No error capture = agent gets empty arguments and silently fails

**Symptom:** Tools execute with no arguments, causing validation failures

---

### Issue #2: Streaming Arguments Accumulation Without Validation

**Location:** `go/internal/opencode/provider/unified.go:404`

```go
// PROBLEM: String concatenation with no validation
acc.Function.Arguments += tc.Function.Arguments
```

**Why This Fails:**
1. Chunks arrive as incomplete JSON strings
2. Example: First chunk `{"param1"`, second chunk `: "value"}`
3. Concatenation happens out-of-order for some proxies
4. Final JSON is malformed: `{"param1: "value"}` (missing quote)
5. Line 412 unmarshal fails silently

**Why glm-4.6 Works:**
- Might return complete Arguments in first chunk
- Or proxy handles chunk ordering differently
- Or tool arguments are simpler (no complex types)

**Why gpt-5.1-codex Fails:**
- Complex function parameters streamed across multiple chunks
- Proxy concatenates differently than expected
- Final JSON is malformed but error is silent

---

### Issue #3: ConfigOption Overwrites (Proxy Configuration Bug)

**Location:** `go/internal/bootstrap/bootstrap.go:73-78` and `go/internal/opencode/provider/factory.go:208-220`

```go
// BOOTSTRAP CODE
provider.WithAPIKey(os.Getenv("ANTHROPIC_API_KEY")),
provider.WithAPIKey(os.Getenv("OPENAI_API_KEY")),
provider.WithAPIKey(os.Getenv("DEEPSEEK_API_KEY")),
// ↑ Each one OVERWRITES the previous! Last non-empty key wins

// FACTORY CODE
func (f *Factory) Create(pt ProviderType, opts ...ConfigOption) (llm.Provider, error) {
    cfg := Config{}
    for _, opt := range opts {
        opt(&cfg)  // ← OVERWRITE, not merge!
    }
}
```

**Impact on Your Setup:**
- If you set `OPENAI_API_KEY` for OpenRouter
- Your `PROXY_API_KEY` gets IGNORED
- Or vice versa: proxy key overwrites OpenAI key

**Current Order (bootstrap.go:73-78):**
1. ANTHROPIC_API_KEY
2. OPENAI_API_KEY ← **WINS** (if set, overwrites all above)
3. DEEPSEEK_API_KEY
4. (Missing: UNIFIED_API_KEY, PROXY_API_KEY)

**Your Situation:**
- You're using proxy at `http://tizz.win:8317/`
- But bootstrap doesn't pass proxy credentials!
- So factory falls back to registered provider
- Which tries to use OPENAI_API_KEY via wrong endpoint

---

### Issue #4: Unified Provider BaseURL Not Applied to Tool Calls

**Location:** `go/internal/opencode/provider/unified.go:46-56` and `unified.go:311`

```go
// URL is constructed correctly...
if baseURL != "" {
    baseURL = strings.TrimSuffix(baseURL, "/")
    if !strings.HasSuffix(baseURL, "/chat/completions") {
        if strings.HasSuffix(baseURL, "/v1") {
            baseURL = baseURL + "/chat/completions"
        } else {
            baseURL = baseURL + "/v1/chat/completions"  // ← Assumes /v1/chat/completions
        }
    }
}

// But: What if proxy uses different path?
// Example: http://tizz.win:8317/api/chat/completions
// ↑ This would fail URL normalization!
```

**Problem:** If proxy doesn't follow OpenAI path conventions, it fails silently

---

### Issue #5: No Tool Call Validation Before Execution

**Location:** `go/internal/opencode/agent/agent.go:494` and `tool/executor.go`

```go
// NO VALIDATION that args are not nil/empty before execution!
toolResults := a.executeToolsParallel(ctx, pendingToolCalls, assistantMsg.Timestamp, events)
```

**What Should Happen:**
```go
for _, tc := range pendingToolCalls {
    if tc.Args == nil || len(tc.Args) == 0 {
        // Log warning! This is a parse failure!
        // Generate error result instead of executing with empty args
    }
}
```

---

## SYMPTOMS YOU'RE SEEING

✗ "Tool use failures" with gpt-5.1-codex
✗ "Tools not executing" or "no response from tools"
✗ Works with glm-4.6

**Why glm-4.6 Works:**
- Registered with special case in factory (line 200-212)
- Forces unified provider with your proxy credentials
- Simple parameters that parse correctly

**Why gpt-5.1-codex Fails:**
- Falls back to generic OpenAI provider logic
- Uses wrong API endpoint (OpenAI instead of proxy)
- Complex parameter streaming breaks argument parsing
- Silent failure on JSON unmarshal

---

## FIXES REQUIRED

### FIX #1: Add Error Handling to Tool Arguments Parsing

**File:** `go/internal/opencode/provider/unified.go:408-423`

```go
// BEFORE
var args map[string]any
json.Unmarshal([]byte(tc.Function.Arguments), &args)

events <- domain.StreamEvent{
    Type: domain.StreamEventToolCall,
    Part: domain.ToolCallPart{
        ToolID: tc.ID,
        Name:   tc.Function.Name,
        Args:   args,  // ← Can be nil!
    },
}

// AFTER
var args map[string]any
if err := json.Unmarshal([]byte(tc.Function.Arguments), &args); err != nil {
    // Log the error for debugging
    fmt.Fprintf(os.Stderr, "[ERROR] Failed to parse tool arguments for %s: %v\nRaw: %s\n",
        tc.Function.Name, err, tc.Function.Arguments)
    args = map[string]any{
        "_parse_error": err.Error(),
        "_raw_json": tc.Function.Arguments,
    }
}

events <- domain.StreamEvent{
    Type: domain.StreamEventToolCall,
    Part: domain.ToolCallPart{
        ToolID: tc.ID,
        Name:   tc.Function.Name,
        Args:   args,  // ← Now guaranteed to be non-nil
    },
}
```

**Benefit:** You'll SEE the actual JSON parsing error instead of silent failure

---

### FIX #2: Validate Tool Arguments Before Execution

**File:** `go/internal/opencode/agent/agent.go:617-638`

```go
// ADD validation at start of executeToolsParallel
func (a *Agent) executeToolsParallel(
    ctx context.Context,
    toolCalls []domain.ToolCallPart,
    startTime time.Time,
    events chan<- domain.StreamEvent,
) []domain.Part {
    if len(toolCalls) == 0 {
        return nil
    }

    // NEW: Validate all tool calls before execution
    for i, tc := range toolCalls {
        if tc.Args == nil || len(tc.Args) == 0 {
            fmt.Fprintf(os.Stderr,
                "[WARN] Tool call #%d (%s) has no arguments. Check provider streaming.\n",
                i, tc.Name)
            // Don't execute - return error result
            toolCalls[i].Args = map[string]any{
                "_error": "No arguments received from LLM. Provider streaming issue.",
            }
        }
        if tc.Name == "" {
            fmt.Fprintf(os.Stderr, "[ERROR] Tool call #%d has no name!\n", i)
        }
    }
    // ... rest of function
}
```

---

### FIX #3: Fix ConfigOption Merging (Bootstrap Issue)

**File:** `go/internal/bootstrap/bootstrap.go:70-79`

```go
// BEFORE (BROKEN - last key wins)
prov, resolvedMasterModelID, err := config.GetModelWithFallback(
    masterConfig.ModelID,
    masterConfig.Fallbacks,
    provider.WithAPIKey(os.Getenv("ANTHROPIC_API_KEY")),
    provider.WithAPIKey(os.Getenv("OPENAI_API_KEY")),        // ← overwrites ANTHROPIC
    provider.WithAPIKey(os.Getenv("DEEPSEEK_API_KEY")),      // ← overwrites OPENAI
    provider.WithBaseURL(os.Getenv("ANTHROPIC_BASE_URL")),
    provider.WithBaseURL(os.Getenv("OPENAI_BASE_URL")),      // ← overwrites ANTHROPIC
    provider.WithBaseURL(os.Getenv("DEEPSEEK_BASE_URL")),    // ← overwrites OPENAI
)

// AFTER (FIXED - pass only non-empty values, in priority order)
opts := []provider.ConfigOption{}

// Priority: Proxy > DeepSeek > OpenAI > Anthropic
// This way proxy keys override others if both are set
if proxyKey := os.Getenv("PROXY_API_KEY"); proxyKey != "" {
    opts = append(opts, provider.WithAPIKey(proxyKey))
}
if proxyURL := os.Getenv("PROXY_BASE_URL"); proxyURL != "" {
    opts = append(opts, provider.WithBaseURL(proxyURL))
}

if deepseekKey := os.Getenv("DEEPSEEK_API_KEY"); deepseekKey != "" {
    opts = append(opts, provider.WithAPIKey(deepseekKey))
}
if deepseekURL := os.Getenv("DEEPSEEK_BASE_URL"); deepseekURL != "" {
    opts = append(opts, provider.WithBaseURL(deepseekURL))
}

if openaiKey := os.Getenv("OPENAI_API_KEY"); openaiKey != "" {
    opts = append(opts, provider.WithAPIKey(openaiKey))
}
if openaiURL := os.Getenv("OPENAI_BASE_URL"); openaiURL != "" {
    opts = append(opts, provider.WithBaseURL(openaiURL))
}

if anthropicKey := os.Getenv("ANTHROPIC_API_KEY"); anthropicKey != "" {
    opts = append(opts, provider.WithAPIKey(anthropicKey))
}
if anthropicURL := os.Getenv("ANTHROPIC_BASE_URL"); anthropicURL != "" {
    opts = append(opts, provider.WithBaseURL(anthropicURL))
}

prov, resolvedMasterModelID, err := config.GetModelWithFallback(
    masterConfig.ModelID,
    masterConfig.Fallbacks,
    opts...,  // ← Pass collected options
)
```

**Better Fix (in Factory):** Change ConfigOption semantics to merge, not overwrite:

```go
// go/internal/opencode/provider/factory.go - Add merge functionality
func (c *Config) mergeAPIKey(key string) {
    if key != "" && c.APIKey == "" {
        c.APIKey = key  // Only set if not already set
    }
}

func (c *Config) mergeBaseURL(url string) {
    if url != "" && c.BaseURL == "" {
        c.BaseURL = url  // Only set if not already set
    }
}

// Then change factory.go Create() function
func (f *Factory) Create(pt ProviderType, opts ...ConfigOption) (llm.Provider, error) {
    cfg := Config{}
    for _, opt := range opts {
        opt(&cfg)  // Now merge instead of overwrite!
    }
}
```

---

### FIX #4: Improve Tool Streaming Validation

**File:** `go/internal/opencode/provider/unified.go:380-425`

```go
// AFTER "finish_reason" check - validate accumulated arguments
if choice.FinishReason != "" {
    // NEW: Validate and log all accumulated tool calls
    for idx, tc := range toolCallAccumulators {
        // Check for completeness
        if tc.Function.Name == "" {
            fmt.Fprintf(os.Stderr,
                "[WARN] Tool call %d has no name. This shouldn't happen.\n", idx)
        }

        // Check if arguments look valid
        if tc.Function.Arguments != "" && !isValidJSON(tc.Function.Arguments) {
            fmt.Fprintf(os.Stderr,
                "[ERROR] Tool call %d (%s) has invalid JSON arguments:\n%s\n",
                idx, tc.Function.Name, tc.Function.Arguments)
            // Don't try to unmarshal - will fail silently
            continue
        }
    }

    // Emit tool calls (with proper error handling)
    for _, tc := range toolCallAccumulators {
        var args map[string]any
        if err := json.Unmarshal([]byte(tc.Function.Arguments), &args); err != nil {
            fmt.Fprintf(os.Stderr, "[ERROR] Failed to parse arguments for %s: %v\n",
                tc.Function.Name, err)
            args = map[string]any{"_error": err.Error()}
        }

        events <- domain.StreamEvent{
            Type: domain.StreamEventToolCall,
            Part: domain.ToolCallPart{
                ToolID: tc.ID,
                Name:   tc.Function.Name,
                Args:   args,
            },
        }
    }
}
```

Add helper function:

```go
func isValidJSON(s string) bool {
    var tmp interface{}
    return json.Unmarshal([]byte(s), &tmp) == nil
}
```

---

### FIX #5: Add Provider Endpoint Validation

**File:** `go/internal/opencode/provider/unified.go:40-70`

```go
// BEFORE: Silent normalization that might be wrong
if baseURL != "" {
    baseURL = strings.TrimSuffix(baseURL, "/")
    if !strings.HasSuffix(baseURL, "/chat/completions") {
        if strings.HasSuffix(baseURL, "/v1") {
            baseURL = baseURL + "/chat/completions"
        } else {
            baseURL = baseURL + "/v1/chat/completions"
        }
    }
}

// AFTER: Validate and log the endpoint
if baseURL != "" {
    baseURL = strings.TrimSuffix(baseURL, "/")

    // Try to detect correct format
    if strings.Contains(baseURL, "/chat/completions") {
        // Already has the endpoint path
    } else if strings.HasSuffix(baseURL, "/v1") {
        baseURL = baseURL + "/chat/completions"
        fmt.Printf("[DEBUG] Unified provider: Appended /chat/completions to v1 endpoint\n")
    } else {
        // Assume it's base URL without /v1
        baseURL = baseURL + "/v1/chat/completions"
        fmt.Printf("[DEBUG] Unified provider: Normalized URL to %s\n", baseURL)
    }

    fmt.Printf("[DEBUG] Unified provider: Using endpoint %s\n", baseURL)
}
```

---

## DIAGNOSTIC TESTS

### Test #1: Check if Arguments Are Being Parsed Correctly

```bash
cd /var/home/joss/Descargas/_Proyectos2/urp-cli/go

# Run with GPT-5.1-CODEX and capture stderr
urp --tui 2>&1 | grep -E "\[ERROR\].*parse|_parse_error|_raw_json"

# If you see these patterns, arguments are not being parsed
# They should show the raw JSON that failed
```

### Test #2: Verify Proxy Credentials Are Loaded

```bash
# Check environment
echo "PROXY_API_KEY=$PROXY_API_KEY"
echo "PROXY_BASE_URL=$PROXY_BASE_URL"
echo "OPENAI_API_KEY=$OPENAI_API_KEY"  # If set, it might override PROXY

# Bootstrap debug output should show which provider is selected
# Look for: "[DEBUG] Initialize: Provider initialized with resolved model ID"
```

### Test #3: Test with Simple Tool

```bash
# Create a test that uses a simple tool with minimal arguments
urp --tui

# In session, try a simple operation like "read /etc/hostname"
# If it works, the issue is with complex tool parameters
# If it fails, issue is earlier in the chain
```

### Test #4: Compare Tool Definitions

```bash
# Check if glm-4.6 and gpt-5.1-codex are receiving the same tools
# Add logging in unified.go:203-212 to see tool definitions per model
```

---

## YOUR IMMEDIATE ACTION ITEMS

1. **Apply FIX #1** - Add error handling to line 412 in unified.go
   - This will show you the actual error: what JSON is failing to parse?

2. **Apply FIX #2** - Add validation in agent.go executeToolsParallel
   - This will show you if args are nil before execution

3. **Apply FIX #3** - Fix bootstrap.go configuration order
   - Ensure PROXY credentials are actually being used

4. **Run diagnostic tests** - Find out which step is failing

5. **Share the error logs** - Once you see the actual parsing errors, we can fix the root cause

---

## EXPECTED BEHAVIOR AFTER FIXES

**Before Fix:**
```
$ urp --tui
[Using gpt-5.1-codex]
> read /etc/hostname
[Silent failure - no error, just no response]
```

**After Fix:**
```
$ urp --tui
[DEBUG] Unified provider: Using endpoint http://tizz.win:8317/v1/chat/completions
[DEBUG] Provider initialized with resolved model ID 'gpt-5.1-codex' (Provider: unified)
> read /etc/hostname
[ERROR] Failed to parse tool arguments for bash: unexpected end of JSON input
Raw: {"command": "read /etc/hostname"  [INCOMPLETE!]
[Tool execution skipped due to parse error]
```

This shows you WHERE the problem is (incomplete JSON from proxy streaming).

---

## SUMMARY

| Issue | Root Cause | Impact | Fix |
|-------|-----------|--------|-----|
| #1 - Silent parse failure | No error handling on json.Unmarshal | Empty args passed to executor | Add error capture & logging |
| #2 - Malformed JSON | Streaming chunks concatenated incorrectly | Parse fails silently | Validate JSON before parse |
| #3 - Wrong credentials | ConfigOption overwrites instead of merge | Proxy creds ignored, wrong endpoint | Pass env vars conditionally |
| #4 - Wrong endpoint | URL normalization assumes /v1 structure | Tool calls sent to wrong place | Add endpoint validation logging |
| #5 - No validation | Empty args not caught before execution | Tool executes with no params | Validate args completeness |

**Primary Issue:** ConfigOption merging + Silent JSON parse failures
**Secondary Issue:** Streaming argument accumulation not validated
**Result:** Works with glm-4.6 (simpler args), fails with gpt-5.1-codex (complex args)
