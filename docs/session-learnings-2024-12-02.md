# Session Learnings - 2024-12-02

## URP Dev Container Fixes & Improvements

### 1. Surgical Read - Missing Line Numbers

**Problem:** `surgical read` failed with "Target found but missing line numbers"

**Root Cause:** `ingester.py` created Function/Class nodes but didn't save `path` property. Also, tree-sitter returns 0-indexed lines but we need 1-indexed.

**Fix:** `ingester.py:103-117`
```python
SET e.name = $name,
    e.kind = $kind,
    e.path = $path,  # NEW: save file path on entity
    e.start_line = $start,
    e.end_line = $end
# ...
"start": entity.start_line + 1,  # Convert 0-indexed to 1-indexed
"end": entity.end_line + 1,
```

**Impact:** ~93% token savings (23 lines avg function vs 322 lines avg file)

---

### 2. Aliases in Non-Interactive Shell

**Problem:** Aliases and shell functions don't work in `docker exec bash -c '...'`

**Root Cause:** `bash -c` is non-interactive, doesn't load `.bashrc`

**Fix:** `Dockerfile:152`
```dockerfile
echo '[ -f /app/shell_hooks.sh ] && source /app/shell_hooks.sh' >> /etc/bash.bashrc
```

This loads hooks globally for ALL bash sessions.

---

### 3. Token Tracker False Alarms

**Problem:** Token tracker showed "RATE HIGH" warning with only 3.9% usage

**Root Cause:** Linear projection from first minute gives absurd results (1950 tokens × 60 = 117,000)

**Fix:** `token_tracker.py:141-148`
```python
minutes_elapsed = max(now.minute, 5)  # Need 5+ min for reliable projection
will_exceed = (
    (usage_pct > 50) or  # Already high usage
    (projected > budget and now.minute >= 10)  # Projected AND enough data
)
```

---

### 4. Claude Code Write Tool Fails with Proxies

**Problem:** Some OpenAI-compatible proxies don't support the Write/Edit tools properly

**Solution:** Created `claude-write` wrapper that instructs Claude to use Bash+heredoc

**File:** `devcontainer/claude-write`
```bash
INSTRUCTION="REGLA OBLIGATORIA: Para crear o modificar archivos, SIEMPRE usa bash con cat y heredoc.
NUNCA uses la herramienta Write o Edit directamente. Solo usa Bash con cat/heredoc."
exec claude -p "$INSTRUCTION" --max-turns "$MAX_TURNS"
```

---

### 5. Claude Code Permission Errors for Dev User

**Problem:** Claude Code as `dev` user fails with `EACCES: permission denied`

**Root Cause:** `/home/dev/.claude` created by root, missing subdirectories

**Fix:** `Dockerfile:165-168`
```dockerfile
RUN mkdir -p /home/dev/.claude/{projects,debug,statsig,todos,plans,session-env,shell-snapshots} && \
    echo '{"permissions":{"allow":["Bash(*)","Read(*)","Write(*)","Edit(*)","Glob(*)","Grep(*)"]}}' > /home/dev/.claude/settings.json && \
    chown -R dev:dev /home/dev && \
    chmod -R 755 /home/dev/.claude
```

---

### 6. Cascading Container Control

**Problem:** Need to verify Claude CLI can spawn Docker containers

**Solution:** Works! Use `docker run -d --name <name>` (not `--rm`) for persistent containers

**Verification:**
```bash
docker ps
# Shows: devcontainer-urp-1 (Claude CLI) + scraper-cascade (spawned by inner Claude)
```

**Key flags:**
- `-d`: Run in background (daemon)
- `--name`: Give container a name
- NO `--rm`: Keep container after execution

---

## Quick Reference

| Issue | File | Key Change |
|-------|------|------------|
| surgical read | ingester.py | Add `e.path`, convert lines +1 |
| non-interactive aliases | Dockerfile | Source hooks in /etc/bash.bashrc |
| token tracker false alarm | token_tracker.py | Require 10+ min for projection warning |
| Write tool fails | claude-write | Use Bash+heredoc instruction |
| dev user permissions | Dockerfile | Create all .claude subdirs with chown |
| cascading containers | docker run | Use -d --name, no --rm |

## Token Savings Analysis

```
Files: 15
Functions: 164
Avg file: 322 lines (~1,288 tokens)
Avg function: 23 lines (~92 tokens)
Savings: 92.8% per surgical read
```

For 10 function reads per session:
- Whole files: ~12,880 tokens
- Surgical: ~920 tokens
- Saved: ~11,960 tokens (93%)
