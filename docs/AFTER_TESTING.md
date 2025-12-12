# After Running the Tests - Next Steps

Once you've run `./RUN_PROXY_TEST.sh`, use this guide to interpret results.

---

## ✅ If Tools Work (Best Case)

**What you'll see:**
```
[DEBUG] Bootstrap: Using PROXY_API_KEY for authentication
[DEBUG] unified provider: endpoint=http://tizz.win:8317/v1/chat/completions
[DEBUG] Initialize: Provider initialized with resolved model ID 'gpt-5.1-codex'

> read /etc/hostname
hostname
```

**What to do:**
1. ✅ **Celebrate!** gpt-5.1-codex is now working
2. Push the fixes to your repository
3. Update your team's documentation
4. Test with other models to ensure no regressions

---

## ❌ If You See [ERROR] Messages (Debugging Needed)

**Example error:**
```
[ERROR] unified: Failed to parse tool arguments for 'bash': unexpected end of JSON input
Raw JSON: {"command": "read /etc/hostname"
```

**What this means:**
- The proxy is returning incomplete/malformed JSON
- The streaming chunks are being concatenated incorrectly

**What to do:**
1. **Note the exact error message** (copy it)
2. **Check the raw JSON** - is it truncated?
3. **Review TOOL_FAILURE_DIAGNOSIS.md** → Section "Issue #2: Streaming Arguments Accumulation"
4. **Investigate proxy logs:**
   ```bash
   # If your proxy runs in Docker
   docker logs <proxy-container> | tail -50

   # Look for:
   # - Stream errors
   # - Incomplete responses
   # - Timeout issues
   ```

5. **Possible fixes:**
   - Adjust `PROXY_BASE_URL` if proxy uses different endpoint path
   - Increase proxy timeout if streaming is slow
   - Check if proxy supports streaming responses
   - Contact proxy maintainer with error message

---

## ⚠️ If You See [WARN] Messages (Streaming Issue)

**Example warning:**
```
[WARN] Tool call #0 (bash) has no arguments. Check provider streaming.
```

**What this means:**
- Streaming completed but arguments never arrived
- Proxy either:
  - Didn't send arguments field
  - Timed out before streaming completed
  - Uses different JSON schema than expected

**What to do:**
1. **Check if proxy is actually calling tools:**
   ```bash
   # Monitor proxy logs in real-time
   docker logs -f <proxy-container> 2>&1 | grep -i stream
   ```

2. **Verify proxy response format:**
   - Is proxy returning `"arguments"` field?
   - Is it a string or object?
   - Compare with OpenAI API documentation

3. **Check network/timing:**
   ```bash
   # See if requests are timing out
   timeout 30 ./go/urp --tui 2>&1 | tee test.log
   ```

4. **Review TOOL_FAILURE_DIAGNOSIS.md** → Section "Issue #5: Tool Call ID Uniqueness"

---

## 🔧 If You See No Debug Messages

**What to check:**
```bash
# 1. Is Memgraph running?
docker ps | grep memgraph

# 2. Is binary working?
./go/urp version

# 3. Are env vars set?
echo "PROXY_API_KEY=$PROXY_API_KEY"
echo "PROXY_BASE_URL=$PROXY_BASE_URL"

# 4. Try running with explicit variables
PROXY_API_KEY="your-key" \
PROXY_BASE_URL="http://tizz.win:8317" \
URP_MASTER_MODEL_ID="gpt-5.1-codex" \
./go/urp --tui 2>&1 | head -30
```

---

## 📊 Analyzing the Log File

After testing, the script saved output to a file. Analyze it:

```bash
# Find the log file (printed at end of script)
# Or use the most recent one:
LATEST_LOG=$(ls -t /tmp/urp_interactive_test_*.log | head -1)
echo $LATEST_LOG

# Useful queries:

# 1. Check if proxy was used
grep -i "PROXY\|unified" $LATEST_LOG | head -10

# 2. Count errors
grep -c "\[ERROR\]" $LATEST_LOG || echo "0 errors"

# 3. Count warnings
grep -c "\[WARN\]" $LATEST_LOG || echo "0 warnings"

# 4. Extract endpoint
grep "unified provider: endpoint" $LATEST_LOG | tail -1

# 5. Show first error
grep "\[ERROR\]" $LATEST_LOG | head -1

# 6. Show all debug output
grep "\[DEBUG\]" $LATEST_LOG
```

---

## 📋 Information to Share if You Need Help

When reporting issues, include:

```bash
# 1. Test output summary
./RUN_PROXY_TEST.sh 2>&1 > test_summary.txt

# 2. Error messages
grep -E "\[ERROR\]|\[WARN\]" $LATEST_LOG > errors.txt

# 3. Configuration (SAFE - no credentials)
echo "PROXY_BASE_URL=$PROXY_BASE_URL" >> diagnostic.txt
grep "unified provider: endpoint" $LATEST_LOG >> diagnostic.txt

# 4. Memgraph status
./go/urp doctor >> diagnostic.txt

# Share:
# - diagnostic.txt
# - errors.txt
# - error messages (with RAW JSON included)
```

---

## 🚀 Next Steps Based on Outcome

### If Working ✅
```bash
# 1. Commit your successful test
git add AFTER_TESTING.md
git commit -m "docs: document successful proxy testing with gpt-5.1-codex"

# 2. Document the working configuration
cat > PROXY_CONFIG.md << 'EOF'
# Proxy Configuration for gpt-5.1-codex

Working setup:
- PROXY_API_KEY: [configured]
- PROXY_BASE_URL: http://tizz.win:8317
- Model: gpt-5.1-codex

Status: ✅ WORKING
Date: $(date)
EOF

# 3. Test with other models
URP_MASTER_MODEL_ID="other-model" ./go/urp --tui
```

### If Partially Working (Some errors visible) ⚠️
```bash
# You now have VISIBILITY - that's progress!

# 1. Investigate the specific error
cat errors.txt

# 2. Review relevant diagnostic section:
grep -A 20 "Issue #" TOOL_FAILURE_DIAGNOSIS.md | grep -A 20 "your-issue"

# 3. Implement fix based on error type

# 4. Re-test
./RUN_PROXY_TEST.sh
```

### If Not Working Yet ❌
```bash
# You have visibility now - that's good!

# 1. Check that error messages match one of the documented issues
grep -E "\[ERROR\]|\[WARN\]" test_summary.txt

# 2. If error matches Issue #1-5 in TOOL_FAILURE_DIAGNOSIS.md:
#    → Follow the fix instructions
#    → Re-test

# 3. If error is different:
#    → Document it (copy exact message)
#    → Create a new issue
#    → Share: error message + config + proxy info
```

---

## 💡 Quick Reference

| Symptom | Cause | Fix |
|---------|-------|-----|
| No output after command | Proxy not responding | Check proxy, increase timeout |
| `[ERROR] Failed to parse` | JSON incomplete/malformed | Check TOOL_FAILURE_DIAGNOSIS.md Issue #2 |
| `[WARN] has no arguments` | Streaming incomplete | Check proxy logs, increase timeout |
| `[DEBUG] endpoint=http://wrong` | URL normalization wrong | Adjust PROXY_BASE_URL |
| No `[DEBUG]` messages | Bootstrap not running with proxy vars | Set PROXY_API_KEY/PROXY_BASE_URL before launch |

---

## 📞 When to Report an Issue

Provide:
1. Exact `[ERROR]` or `[WARN]` message
2. Raw JSON if shown
3. Your `PROXY_BASE_URL` (safe to share)
4. Output of `./go/urp doctor`
5. What you've already tried

Then we can:
- ✅ Confirm if issue is in URP or proxy
- ✅ Provide targeted fix
- ✅ Add defensive code if needed
- ✅ Update documentation

---

**Good luck! The fact that you now see errors (instead of silence) is huge progress.** 🎉
