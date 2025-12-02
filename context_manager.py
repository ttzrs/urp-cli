"""
Context Optimization Manager for Claude Code Integration.

Implements 4 modes with A/B testing and metrics collection:
- None: No optimization (baseline for comparison)
- Semi-auto: Generate text for user to copy to /compact
- Aggressive: Auto-clean without intervention
- Hybrid: Auto-clean obvious noise, ask for important decisions

Metrics tracked per mode:
- tokens_saved: Total tokens freed
- context_loss_errors: Times important context was lost
- session_quality: User feedback score (1-5)
- actions_taken: Count of optimizations performed
"""
import os
import sys
import json
import time
import re
from datetime import datetime, timedelta
from pathlib import Path
from typing import Optional

# ═══════════════════════════════════════════════════════════════════════════════
# Configuration
# ═══════════════════════════════════════════════════════════════════════════════

# Use local paths if /shared doesn't exist (for testing outside container)
_SHARED_DIR = '/shared/sessions' if os.path.exists('/shared') else os.path.join(os.path.dirname(__file__), '.sessions')
METRICS_FILE = os.getenv('URP_CONTEXT_METRICS', os.path.join(_SHARED_DIR, 'context_metrics.json'))
MODE_FILE = os.getenv('URP_CONTEXT_MODE', os.path.join(_SHARED_DIR, 'context_mode.json'))
SESSION_FILE = os.path.join(os.path.dirname(__file__), ".session_context.json")

# Noise detection thresholds
NOISE_AGE_THRESHOLD_SEC = 30 * 60  # 30 minutes
NOISE_DUPLICATE_THRESHOLD = 0.85   # Similarity for dedup
MAX_WORKING_MEMORY_ITEMS = 20      # Before suggesting cleanup

# Mode weights for scoring (for recommend_best_mode)
WEIGHTS = {
    'tokens_saved': 0.3,
    'quality_score': 0.4,
    'error_rate': 0.3
}


# ═══════════════════════════════════════════════════════════════════════════════
# Mode Management
# ═══════════════════════════════════════════════════════════════════════════════

def load_mode_config() -> dict:
    """Load current optimization mode."""
    try:
        if os.path.exists(MODE_FILE):
            with open(MODE_FILE, 'r') as f:
                return json.load(f)
    except Exception:
        pass
    return {
        "mode": "hybrid",  # Default: safest option
        "set_at": datetime.now().isoformat(),
        "auto_switch": True  # Allow auto-switching based on metrics
    }


def save_mode_config(config: dict):
    """Persist mode config."""
    try:
        Path(MODE_FILE).parent.mkdir(parents=True, exist_ok=True)
        with open(MODE_FILE, 'w') as f:
            json.dump(config, f, indent=2)
    except Exception:
        pass


def get_current_mode() -> str:
    """Get the current optimization mode."""
    return load_mode_config().get("mode", "hybrid")


def set_mode(mode: str) -> dict:
    """Set optimization mode: none, semi, auto, or hybrid."""
    if mode not in ("none", "semi", "auto", "hybrid"):
        return {"error": f"Invalid mode: {mode}. Use: none, semi, auto, hybrid"}

    config = load_mode_config()
    old_mode = config.get("mode", "hybrid")
    config["mode"] = mode
    config["set_at"] = datetime.now().isoformat()
    save_mode_config(config)

    return {
        "old_mode": old_mode,
        "new_mode": mode,
        "message": f"Mode changed: {old_mode} -> {mode}"
    }


# ═══════════════════════════════════════════════════════════════════════════════
# Metrics Collection (A/B Testing)
# ═══════════════════════════════════════════════════════════════════════════════

def load_metrics() -> dict:
    """Load metrics history."""
    try:
        if os.path.exists(METRICS_FILE):
            with open(METRICS_FILE, 'r') as f:
                return json.load(f)
    except Exception:
        pass
    return {
        "created": datetime.now().isoformat(),
        "by_mode": {
            "none": {"actions": 0, "tokens_saved": 0, "errors": 0, "quality_sum": 0, "quality_count": 0},
            "semi": {"actions": 0, "tokens_saved": 0, "errors": 0, "quality_sum": 0, "quality_count": 0},
            "auto": {"actions": 0, "tokens_saved": 0, "errors": 0, "quality_sum": 0, "quality_count": 0},
            "hybrid": {"actions": 0, "tokens_saved": 0, "errors": 0, "quality_sum": 0, "quality_count": 0}
        },
        "events": []  # Recent events for debugging
    }


def save_metrics(metrics: dict):
    """Persist metrics."""
    try:
        Path(METRICS_FILE).parent.mkdir(parents=True, exist_ok=True)
        with open(METRICS_FILE, 'w') as f:
            json.dump(metrics, f, indent=2)
    except Exception:
        pass


def record_metric(mode: str, metric: str, value: float):
    """Record a metric for the given mode."""
    metrics = load_metrics()

    if mode not in metrics["by_mode"]:
        metrics["by_mode"][mode] = {"actions": 0, "tokens_saved": 0, "errors": 0, "quality_sum": 0, "quality_count": 0}

    if metric == "tokens_saved":
        metrics["by_mode"][mode]["tokens_saved"] += value
        metrics["by_mode"][mode]["actions"] += 1
    elif metric == "error":
        metrics["by_mode"][mode]["errors"] += 1
    elif metric == "quality":
        metrics["by_mode"][mode]["quality_sum"] += value
        metrics["by_mode"][mode]["quality_count"] += 1

    # Log event
    metrics["events"].append({
        "timestamp": datetime.now().isoformat(),
        "mode": mode,
        "metric": metric,
        "value": value
    })
    # Keep last 100 events
    metrics["events"] = metrics["events"][-100:]

    save_metrics(metrics)


def record_quality(score: int):
    """Record user quality feedback (1-5)."""
    mode = get_current_mode()
    record_metric(mode, "quality", score)
    return {"mode": mode, "score": score, "message": f"Quality feedback recorded: {score}/5"}


def record_context_loss():
    """Record that important context was lost."""
    mode = get_current_mode()
    record_metric(mode, "error", 1)
    return {"mode": mode, "message": "Context loss recorded"}


def get_mode_stats() -> dict:
    """Get performance statistics per mode."""
    metrics = load_metrics()
    stats = {}

    for mode, data in metrics["by_mode"].items():
        actions = data.get("actions", 0) or 1  # Avoid division by zero
        quality_count = data.get("quality_count", 0) or 1

        stats[mode] = {
            "actions": data.get("actions", 0),
            "avg_tokens_saved": round(data.get("tokens_saved", 0) / actions),
            "total_tokens_saved": data.get("tokens_saved", 0),
            "errors": data.get("errors", 0),
            "error_rate": round(data.get("errors", 0) / actions, 3) if actions > 0 else 0,
            "avg_quality": round(data.get("quality_sum", 0) / quality_count, 2) if quality_count > 0 else None,
            "quality_samples": data.get("quality_count", 0)
        }

    return stats


def recommend_best_mode() -> dict:
    """Based on collected metrics, recommend optimal mode."""
    stats = get_mode_stats()

    # Need at least 5 actions per mode to recommend
    min_actions = 5
    candidates = {m: s for m, s in stats.items() if s["actions"] >= min_actions}

    if not candidates:
        return {
            "recommended": "hybrid",
            "reason": "Not enough data yet. Using default (hybrid).",
            "stats": stats
        }

    # Calculate weighted score for each mode
    scores = {}
    for mode, s in candidates.items():
        # Normalize metrics
        max_tokens = max(c["avg_tokens_saved"] for c in candidates.values()) or 1
        tokens_score = s["avg_tokens_saved"] / max_tokens

        # Quality (default 0.5 if no samples)
        quality_score = (s["avg_quality"] or 2.5) / 5.0

        # Error rate (invert: lower is better)
        error_score = 1.0 - min(s["error_rate"], 1.0)

        # Weighted sum
        scores[mode] = (
            tokens_score * WEIGHTS['tokens_saved'] +
            quality_score * WEIGHTS['quality_score'] +
            error_score * WEIGHTS['error_rate']
        )

    best_mode = max(scores.keys(), key=lambda m: scores[m])

    return {
        "recommended": best_mode,
        "scores": {m: round(s, 3) for m, s in scores.items()},
        "reason": f"Best weighted score: {best_mode} ({scores[best_mode]:.3f})",
        "stats": stats
    }


# ═══════════════════════════════════════════════════════════════════════════════
# Core Analysis Functions
# ═══════════════════════════════════════════════════════════════════════════════

def load_working_memory() -> dict:
    """Load working memory from session file."""
    if not os.path.exists(SESSION_FILE):
        return {"focused_nodes": [], "context_cache": {}}
    try:
        with open(SESSION_FILE, 'r') as f:
            return json.load(f)
    except Exception:
        return {"focused_nodes": [], "context_cache": {}}


def save_working_memory(data: dict):
    """Persist working memory."""
    try:
        with open(SESSION_FILE, 'w') as f:
            json.dump(data, f, indent=2)
    except Exception:
        pass


def analyze_context() -> dict:
    """Analyze current context for optimization opportunities."""
    state = load_working_memory()

    now = int(time.time())
    items = []
    total_tokens = 0
    old_items = 0
    duplicate_candidates = []

    for node in state.get("focused_nodes", []):
        cache = state.get("context_cache", {}).get(node, {})
        tokens = cache.get('tokens', 0)
        added_at = cache.get('added_at', 0)
        age_sec = now - added_at

        total_tokens += tokens
        item = {
            "name": node,
            "tokens": tokens,
            "age_sec": age_sec,
            "old": age_sec > NOISE_AGE_THRESHOLD_SEC,
            "importance": cache.get('importance', 1),
            "access_count": cache.get('access_count', 1)
        }
        items.append(item)

        if item["old"]:
            old_items += 1

    # Token budget status
    try:
        from token_tracker import get_remaining
        token_status = get_remaining()
    except ImportError:
        token_status = {"used": 0, "budget": 50000, "usage_pct": 0}

    # Detect noise patterns
    noise = detect_noise_patterns(items)

    return {
        "working_memory": {
            "items": items,
            "total_tokens": total_tokens,
            "count": len(items),
            "old_items": old_items
        },
        "token_status": token_status,
        "noise_patterns": noise,
        "recommendations": _generate_recommendations(items, noise, token_status)
    }


def detect_noise_patterns(items: list) -> list:
    """Identify noisy patterns in working memory."""
    patterns = []
    now = int(time.time())

    # Pattern 1: Old items (> 30 min)
    old_items = [i for i in items if i.get("old")]
    if old_items:
        patterns.append({
            "type": "old_items",
            "count": len(old_items),
            "tokens": sum(i["tokens"] for i in old_items),
            "items": [i["name"] for i in old_items],
            "severity": "medium" if len(old_items) < 5 else "high"
        })

    # Pattern 2: Low importance items (if we have importance scores)
    low_importance = [i for i in items if i.get("importance", 1) <= 1 and i.get("age_sec", 0) > 600]
    if low_importance:
        patterns.append({
            "type": "low_importance",
            "count": len(low_importance),
            "tokens": sum(i["tokens"] for i in low_importance),
            "items": [i["name"] for i in low_importance],
            "severity": "low"
        })

    # Pattern 3: Unused items (low access count, old)
    unused = [i for i in items if i.get("access_count", 1) <= 1 and i.get("age_sec", 0) > 900]
    if unused:
        patterns.append({
            "type": "unused",
            "count": len(unused),
            "tokens": sum(i["tokens"] for i in unused),
            "items": [i["name"] for i in unused],
            "severity": "medium"
        })

    # Pattern 4: Duplicate basenames (e.g., multiple runner.py from different paths)
    basenames = {}
    for i in items:
        basename = os.path.basename(i["name"]) if "/" in i["name"] else i["name"]
        if basename not in basenames:
            basenames[basename] = []
        basenames[basename].append(i)

    duplicates = {k: v for k, v in basenames.items() if len(v) > 1}
    if duplicates:
        patterns.append({
            "type": "duplicate_basenames",
            "count": sum(len(v) - 1 for v in duplicates.values()),
            "tokens": sum(min(v, key=lambda x: x["age_sec"])["tokens"] for v in duplicates.values()),
            "items": list(duplicates.keys()),
            "severity": "low"
        })

    return patterns


def _generate_recommendations(items: list, noise: list, token_status: dict) -> list:
    """Generate actionable recommendations."""
    recs = []

    # High token usage
    if token_status.get("usage_pct", 0) > 70:
        recs.append({
            "action": "compact",
            "priority": "high",
            "reason": f"Token usage at {token_status['usage_pct']}%",
            "suggested_command": "/compact"
        })

    # Many old items
    old_pattern = next((p for p in noise if p["type"] == "old_items" and p["severity"] == "high"), None)
    if old_pattern:
        recs.append({
            "action": "clean_old",
            "priority": "medium",
            "reason": f"{old_pattern['count']} items > 30 min old ({old_pattern['tokens']} tokens)",
            "suggested_command": "cc-clean"
        })

    # Too many items in memory
    if len(items) > MAX_WORKING_MEMORY_ITEMS:
        recs.append({
            "action": "focus_only",
            "priority": "medium",
            "reason": f"{len(items)} items in memory (max recommended: {MAX_WORKING_MEMORY_ITEMS})",
            "suggested_command": "cc-focus-only <target>"
        })

    # Unused items
    unused_pattern = next((p for p in noise if p["type"] == "unused"), None)
    if unused_pattern:
        recs.append({
            "action": "clean_unused",
            "priority": "low",
            "reason": f"{unused_pattern['count']} unused items",
            "suggested_command": "cc-clean --unused"
        })

    return recs


# ═══════════════════════════════════════════════════════════════════════════════
# Four Optimization Modes
# ═══════════════════════════════════════════════════════════════════════════════

def mode_none() -> dict:
    """
    No optimization mode: Baseline for A/B testing.
    Does nothing, just records that no optimization was performed.
    """
    analysis = analyze_context()

    # Just record the action without doing anything
    record_metric("none", "tokens_saved", 0)

    return {
        "mode": "none",
        "tokens_freed": 0,
        "items_removed": [],
        "message": "No optimization (baseline mode)",
        "analysis": analysis,
        "user_action_required": False
    }


def mode_semi_auto() -> dict:
    """
    Semi-automatic mode: Generate text for user to copy to /compact.
    User decides what to do.
    """
    analysis = analyze_context()
    wm = analysis["working_memory"]
    noise = analysis["noise_patterns"]

    # Determine focus (most recently accessed, highest importance)
    items = sorted(wm["items"], key=lambda x: (-x.get("importance", 1), x.get("age_sec", 0)))
    focus_items = items[:3] if items else []

    # Determine what to preserve (recent, high access)
    recent_items = [i for i in items if i.get("age_sec", 0) < 600][:5]

    # Determine what to discard
    discard_items = []
    for pattern in noise:
        if pattern["type"] in ("old_items", "unused", "low_importance"):
            discard_items.extend(pattern["items"])
    discard_items = list(set(discard_items))[:5]

    # Generate compact instruction
    focus_str = ", ".join(i["name"] for i in focus_items) if focus_items else "current task"
    preserve_str = ", ".join(i["name"] for i in recent_items) if recent_items else "active files"
    discard_str = ", ".join(discard_items) if discard_items else "old commands"

    instruction = f"Focus on: {focus_str}. Preserve: {preserve_str}. Discard: {discard_str}."

    return {
        "mode": "semi",
        "instruction": instruction,
        "copy_to_compact": f"/compact {instruction}",
        "analysis": analysis,
        "user_action_required": True
    }


def mode_aggressive() -> dict:
    """
    Aggressive mode: Auto-clean without user intervention.
    Cleans all detected noise patterns.
    """
    mode = "auto"
    analysis = analyze_context()
    noise = analysis["noise_patterns"]

    tokens_freed = 0
    items_removed = []

    state = load_working_memory()

    # Clean all noise patterns
    for pattern in noise:
        for item_name in pattern["items"]:
            if item_name in state["focused_nodes"]:
                state["focused_nodes"].remove(item_name)
                if item_name in state.get("context_cache", {}):
                    tokens_freed += state["context_cache"][item_name].get("tokens", 0)
                    del state["context_cache"][item_name]
                items_removed.append(item_name)

    save_working_memory(state)

    # Record metrics
    if tokens_freed > 0:
        record_metric(mode, "tokens_saved", tokens_freed)

    return {
        "mode": mode,
        "tokens_freed": tokens_freed,
        "items_removed": items_removed,
        "patterns_cleaned": [p["type"] for p in noise],
        "user_action_required": False
    }


def mode_hybrid() -> dict:
    """
    Hybrid mode: Auto-clean obvious noise, prompt for important decisions.
    Best of both worlds.
    """
    mode = "hybrid"
    analysis = analyze_context()
    noise = analysis["noise_patterns"]

    tokens_freed = 0
    items_removed = []
    pending_decisions = []

    state = load_working_memory()

    for pattern in noise:
        # Auto-clean: old items, unused items, duplicates
        if pattern["type"] in ("old_items", "unused", "duplicate_basenames"):
            for item_name in pattern["items"]:
                if item_name in state["focused_nodes"]:
                    state["focused_nodes"].remove(item_name)
                    if item_name in state.get("context_cache", {}):
                        tokens_freed += state["context_cache"][item_name].get("tokens", 0)
                        del state["context_cache"][item_name]
                    items_removed.append(item_name)

        # Ask user: low importance (might be intentional)
        elif pattern["type"] == "low_importance":
            pending_decisions.append({
                "type": pattern["type"],
                "items": pattern["items"],
                "tokens": pattern["tokens"],
                "question": f"Remove {len(pattern['items'])} low-importance items? ({pattern['tokens']} tokens)"
            })

    save_working_memory(state)

    # Record metrics
    if tokens_freed > 0:
        record_metric(mode, "tokens_saved", tokens_freed)

    return {
        "mode": mode,
        "auto_cleaned": {
            "tokens_freed": tokens_freed,
            "items_removed": items_removed
        },
        "pending_decisions": pending_decisions,
        "user_action_required": len(pending_decisions) > 0
    }


# ═══════════════════════════════════════════════════════════════════════════════
# Eviction Scoring (LRU + Importance)
# ═══════════════════════════════════════════════════════════════════════════════

def calculate_eviction_score(item: dict) -> float:
    """
    Calculate eviction priority score.
    Lower score = evict first.
    """
    age_sec = item.get('age_sec', 0)
    importance = item.get('importance', 1)
    access_count = item.get('access_count', 1)

    # Normalize age (0-1, newer = higher score = keep longer)
    # 1 hour decay
    age_score = max(0, 1 - (age_sec / 3600))

    # Normalize importance (assume 1-5 scale)
    importance_score = min(importance, 5) / 5.0

    # Normalize access count (cap at 10)
    access_score = min(access_count, 10) / 10.0

    # Weighted score
    return (importance_score * 0.4) + (age_score * 0.3) + (access_score * 0.3)


def evict_by_score(target_tokens: int) -> dict:
    """Evict items by score until target tokens are freed."""
    state = load_working_memory()
    now = int(time.time())

    # Build scored list
    scored_items = []
    for node in state.get("focused_nodes", []):
        cache = state.get("context_cache", {}).get(node, {})
        item = {
            "name": node,
            "tokens": cache.get("tokens", 0),
            "age_sec": now - cache.get("added_at", 0),
            "importance": cache.get("importance", 1),
            "access_count": cache.get("access_count", 1)
        }
        item["score"] = calculate_eviction_score(item)
        scored_items.append(item)

    # Sort by score ascending (lowest score = evict first)
    scored_items.sort(key=lambda x: x["score"])

    # Evict until target reached
    tokens_freed = 0
    evicted = []

    for item in scored_items:
        if tokens_freed >= target_tokens:
            break

        name = item["name"]
        if name in state["focused_nodes"]:
            state["focused_nodes"].remove(name)
            if name in state.get("context_cache", {}):
                tokens_freed += state["context_cache"][name].get("tokens", 0)
                del state["context_cache"][name]
            evicted.append({"name": name, "score": item["score"], "tokens": item["tokens"]})

    save_working_memory(state)

    return {
        "tokens_freed": tokens_freed,
        "items_evicted": evicted,
        "remaining_count": len(state["focused_nodes"])
    }


# ═══════════════════════════════════════════════════════════════════════════════
# High-Level Commands (called from shell aliases)
# ═══════════════════════════════════════════════════════════════════════════════

def cmd_status() -> str:
    """Show context status."""
    analysis = analyze_context()
    wm = analysis["working_memory"]
    ts = analysis["token_status"]
    recs = analysis["recommendations"]

    lines = []
    lines.append("CC-STATUS: Context Optimization Manager")
    lines.append("=" * 50)
    lines.append("")

    # Mode
    mode = get_current_mode()
    lines.append(f"Mode: {mode.upper()}")
    lines.append("")

    # Working memory
    lines.append("Working Memory:")
    lines.append(f"  Items: {wm['count']} ({wm['old_items']} old)")
    lines.append(f"  Tokens: {wm['total_tokens']}")
    lines.append("")

    # Token budget
    lines.append("Token Budget (this hour):")
    lines.append(f"  Used: {ts.get('used', 0):,} / {ts.get('budget', 0):,} ({ts.get('usage_pct', 0)}%)")
    lines.append(f"  Status: {ts.get('status', '?')}")
    lines.append("")

    # Noise patterns
    noise = analysis["noise_patterns"]
    if noise:
        lines.append("Detected Noise:")
        for p in noise:
            lines.append(f"  [{p['severity']}] {p['type']}: {p['count']} items, {p['tokens']} tokens")
        lines.append("")

    # Recommendations
    if recs:
        lines.append("Recommendations:")
        for r in recs:
            lines.append(f"  [{r['priority']}] {r['reason']}")
            lines.append(f"         Run: {r['suggested_command']}")
        lines.append("")

    return "\n".join(lines)


def cmd_compact() -> str:
    """Run optimization based on current mode."""
    mode = get_current_mode()

    if mode == "none":
        result = mode_none()
        output = [
            "CC-COMPACT (None mode - baseline)",
            "=" * 40,
            "",
            "No optimization performed.",
            "This mode is for A/B testing baseline comparison.",
            ""
        ]
        return "\n".join(output)

    elif mode == "semi":
        result = mode_semi_auto()
        output = [
            "CC-COMPACT (Semi-auto mode)",
            "=" * 40,
            "",
            "Copy this to /compact:",
            "",
            f"  {result['copy_to_compact']}",
            ""
        ]
        return "\n".join(output)

    elif mode == "auto":
        result = mode_aggressive()
        output = [
            "CC-COMPACT (Aggressive mode)",
            "=" * 40,
            "",
            f"Tokens freed: {result['tokens_freed']}",
            f"Items removed: {len(result['items_removed'])}",
            ""
        ]
        if result['items_removed']:
            output.append("Removed:")
            for item in result['items_removed'][:10]:
                output.append(f"  - {item}")
        return "\n".join(output)

    else:  # hybrid
        result = mode_hybrid()
        output = [
            "CC-COMPACT (Hybrid mode)",
            "=" * 40,
            "",
            f"Auto-cleaned: {result['auto_cleaned']['tokens_freed']} tokens",
            f"Items removed: {len(result['auto_cleaned']['items_removed'])}",
            ""
        ]
        if result['pending_decisions']:
            output.append("Pending decisions:")
            for d in result['pending_decisions']:
                output.append(f"  ? {d['question']}")
        return "\n".join(output)


def cmd_clean(options: list = None) -> str:
    """Clean working memory based on options."""
    options = options or []

    state = load_working_memory()
    now = int(time.time())

    tokens_freed = 0
    items_removed = []

    # Determine what to clean
    clean_old = "--old" in options or not options
    clean_unused = "--unused" in options or not options
    clean_all = "--all" in options

    for node in list(state.get("focused_nodes", [])):
        cache = state.get("context_cache", {}).get(node, {})
        age_sec = now - cache.get("added_at", 0)
        access_count = cache.get("access_count", 1)

        should_remove = False

        if clean_all:
            should_remove = True
        elif clean_old and age_sec > NOISE_AGE_THRESHOLD_SEC:
            should_remove = True
        elif clean_unused and access_count <= 1 and age_sec > 900:
            should_remove = True

        if should_remove:
            state["focused_nodes"].remove(node)
            if node in state.get("context_cache", {}):
                tokens_freed += state["context_cache"][node].get("tokens", 0)
                del state["context_cache"][node]
            items_removed.append(node)

    save_working_memory(state)

    # Record metric
    mode = get_current_mode()
    if tokens_freed > 0:
        record_metric(mode, "tokens_saved", tokens_freed)

    output = [
        "CC-CLEAN",
        "=" * 40,
        f"Tokens freed: {tokens_freed}",
        f"Items removed: {len(items_removed)}",
        f"Remaining: {len(state.get('focused_nodes', []))} items",
        ""
    ]
    return "\n".join(output)


def cmd_noise() -> str:
    """Detect and report noise patterns."""
    state = load_working_memory()
    now = int(time.time())

    items = []
    for node in state.get("focused_nodes", []):
        cache = state.get("context_cache", {}).get(node, {})
        items.append({
            "name": node,
            "tokens": cache.get("tokens", 0),
            "age_sec": now - cache.get("added_at", 0),
            "old": (now - cache.get("added_at", 0)) > NOISE_AGE_THRESHOLD_SEC,
            "importance": cache.get("importance", 1),
            "access_count": cache.get("access_count", 1)
        })

    noise = detect_noise_patterns(items)

    if not noise:
        return "CC-NOISE: No noise patterns detected. Context is clean."

    output = [
        "CC-NOISE: Detected Patterns",
        "=" * 40,
        ""
    ]

    for p in noise:
        output.append(f"[{p['severity'].upper()}] {p['type']}")
        output.append(f"  Items: {p['count']}")
        output.append(f"  Tokens: {p['tokens']}")
        output.append(f"  Candidates: {', '.join(p['items'][:5])}")
        output.append("")

    total_noise_tokens = sum(p['tokens'] for p in noise)
    output.append(f"Total noise tokens: {total_noise_tokens}")
    output.append("")
    output.append("Run 'cc-clean' to remove noise automatically.")

    return "\n".join(output)


def cmd_stats() -> str:
    """Show A/B testing statistics."""
    stats = get_mode_stats()
    recommendation = recommend_best_mode()

    output = [
        "CC-STATS: Mode Performance Comparison",
        "=" * 50,
        ""
    ]

    for mode, s in stats.items():
        output.append(f"MODE: {mode.upper()}")
        output.append(f"  Actions: {s['actions']}")
        output.append(f"  Avg tokens saved: {s['avg_tokens_saved']}")
        output.append(f"  Total tokens saved: {s['total_tokens_saved']}")
        output.append(f"  Error rate: {s['error_rate']:.1%}")
        if s['avg_quality']:
            output.append(f"  Avg quality: {s['avg_quality']:.2f}/5 ({s['quality_samples']} samples)")
        else:
            output.append(f"  Avg quality: (no data)")
        output.append("")

    output.append("-" * 50)
    output.append(f"RECOMMENDATION: {recommendation['recommended'].upper()}")
    output.append(f"Reason: {recommendation['reason']}")

    return "\n".join(output)


def cmd_pre_compact() -> str:
    """Hook for PreCompact - called before /compact runs."""
    mode = get_current_mode()

    if mode == "none":
        # None: do nothing (baseline)
        return "PreCompact: No optimization (none mode - baseline)"

    elif mode == "auto":
        # Aggressive: auto-clean everything
        result = mode_aggressive()
        return f"PreCompact: Freed {result['tokens_freed']} tokens (auto mode)"

    elif mode == "hybrid":
        # Hybrid: auto-clean safe patterns
        result = mode_hybrid()
        return f"PreCompact: Freed {result['auto_cleaned']['tokens_freed']} tokens (hybrid mode)"

    else:
        # Semi: just analyze, don't clean
        analysis = analyze_context()
        noise_tokens = sum(p['tokens'] for p in analysis['noise_patterns'])
        return f"PreCompact: {noise_tokens} tokens of noise detected (semi mode - manual action required)"


# ═══════════════════════════════════════════════════════════════════════════════
# CLI Entry Point
# ═══════════════════════════════════════════════════════════════════════════════

def main():
    import argparse

    parser = argparse.ArgumentParser(description="Context Optimization Manager")
    subparsers = parser.add_subparsers(dest="command", required=True)

    # status
    subparsers.add_parser("status", help="Show context status and recommendations")

    # compact
    subparsers.add_parser("compact", help="Run optimization based on current mode")

    # clean
    p = subparsers.add_parser("clean", help="Clean working memory")
    p.add_argument("--old", action="store_true", help="Clean old items (> 30 min)")
    p.add_argument("--unused", action="store_true", help="Clean unused items")
    p.add_argument("--all", action="store_true", help="Clean everything")

    # detect-noise
    subparsers.add_parser("detect-noise", help="Detect noise patterns")

    # mode
    p = subparsers.add_parser("mode", help="Get or set optimization mode")
    p.add_argument("new_mode", nargs="?", choices=["none", "semi", "auto", "hybrid"],
                   help="New mode to set")

    # stats
    subparsers.add_parser("stats", help="Show A/B testing statistics")

    # recommend
    subparsers.add_parser("recommend", help="Recommend best mode based on metrics")

    # pre-compact
    subparsers.add_parser("pre-compact", help="PreCompact hook (called before /compact)")

    # quality
    p = subparsers.add_parser("quality", help="Record quality feedback")
    p.add_argument("score", type=int, choices=[1, 2, 3, 4, 5], help="Quality score (1-5)")

    # error
    subparsers.add_parser("error", help="Record context loss error")

    args = parser.parse_args()

    if args.command == "status":
        print(cmd_status())

    elif args.command == "compact":
        print(cmd_compact())

    elif args.command == "clean":
        options = []
        if args.old:
            options.append("--old")
        if args.unused:
            options.append("--unused")
        if args.all:
            options.append("--all")
        print(cmd_clean(options))

    elif args.command == "detect-noise":
        print(cmd_noise())

    elif args.command == "mode":
        if args.new_mode:
            result = set_mode(args.new_mode)
            print(f"Mode: {result['old_mode']} -> {result['new_mode']}")
        else:
            print(f"Current mode: {get_current_mode()}")

    elif args.command == "stats":
        print(cmd_stats())

    elif args.command == "recommend":
        result = recommend_best_mode()
        print(f"Recommended mode: {result['recommended']}")
        print(f"Reason: {result['reason']}")

    elif args.command == "pre-compact":
        print(cmd_pre_compact())

    elif args.command == "quality":
        result = record_quality(args.score)
        print(f"Quality {args.score}/5 recorded for mode: {result['mode']}")

    elif args.command == "error":
        result = record_context_loss()
        print(f"Context loss recorded for mode: {result['mode']}")


if __name__ == "__main__":
    main()
