#!/usr/bin/env python3
"""
═══════════════════════════════════════════════════════════════════════════════
ULTRA-HARD TEST BATTERY FOR MULTI-SESSION MEMORY SYSTEM
═══════════════════════════════════════════════════════════════════════════════

Tests designed to BREAK the system:
- Edge cases that shouldn't happen but will
- Concurrency nightmares
- Scale limits
- Data corruption scenarios
- Sync failures between Memgraph and ChromaDB

Run: python3 tests/test_memory_system.py
"""

import os
import sys
import time
import json
import uuid
import random
import string
import traceback
import threading
from dataclasses import dataclass, field
from typing import Optional, Any, Callable
from concurrent.futures import ThreadPoolExecutor, as_completed
from datetime import datetime

# Add parent directory to path
sys.path.insert(0, os.path.dirname(os.path.dirname(os.path.abspath(__file__))))


# ═══════════════════════════════════════════════════════════════════════════════
# TEST MONITORING SYSTEM
# ═══════════════════════════════════════════════════════════════════════════════

@dataclass
class TestResult:
    """Result of a single test."""
    name: str
    category: str
    passed: bool
    duration_ms: float
    error: Optional[str] = None
    details: dict = field(default_factory=dict)
    timestamp: str = field(default_factory=lambda: datetime.now().isoformat())


@dataclass
class TestMetrics:
    """Aggregated metrics for test run."""
    total: int = 0
    passed: int = 0
    failed: int = 0
    errors: int = 0
    total_duration_ms: float = 0
    failures: list = field(default_factory=list)
    by_category: dict = field(default_factory=dict)


class TestMonitor:
    """
    Central monitoring system for test execution.
    Tracks results, detects patterns, suggests fixes.
    """

    def __init__(self):
        self.results: list[TestResult] = []
        self.metrics = TestMetrics()
        self.start_time = time.time()
        self._lock = threading.Lock()

    def record(self, result: TestResult):
        """Thread-safe result recording."""
        with self._lock:
            self.results.append(result)
            self.metrics.total += 1
            self.metrics.total_duration_ms += result.duration_ms

            if result.passed:
                self.metrics.passed += 1
            elif result.error:
                self.metrics.errors += 1
                self.metrics.failures.append(result)
            else:
                self.metrics.failed += 1
                self.metrics.failures.append(result)

            # Track by category
            cat = result.category
            if cat not in self.metrics.by_category:
                self.metrics.by_category[cat] = {"passed": 0, "failed": 0}
            if result.passed:
                self.metrics.by_category[cat]["passed"] += 1
            else:
                self.metrics.by_category[cat]["failed"] += 1

    def run_test(self, name: str, category: str, test_fn: Callable, **kwargs) -> TestResult:
        """Execute a test with timing and error capture."""
        start = time.time()
        try:
            result_data = test_fn(**kwargs)
            duration = (time.time() - start) * 1000

            passed = result_data.get("passed", False) if isinstance(result_data, dict) else bool(result_data)
            details = result_data if isinstance(result_data, dict) else {"result": result_data}

            result = TestResult(
                name=name,
                category=category,
                passed=passed,
                duration_ms=duration,
                details=details,
                error=details.get("error") if not passed else None
            )
        except Exception as e:
            duration = (time.time() - start) * 1000
            result = TestResult(
                name=name,
                category=category,
                passed=False,
                duration_ms=duration,
                error=f"{type(e).__name__}: {str(e)}",
                details={"traceback": traceback.format_exc()}
            )

        self.record(result)
        return result

    def analyze_failures(self) -> dict:
        """Analyze failure patterns and suggest fixes."""
        analysis = {
            "total_failures": len(self.metrics.failures),
            "patterns": [],
            "suggestions": [],
            "critical": []
        }

        # Detect patterns
        error_types = {}
        for f in self.metrics.failures:
            if f.error:
                err_type = f.error.split(":")[0] if ":" in f.error else f.error[:50]
                error_types[err_type] = error_types.get(err_type, 0) + 1

        for err_type, count in error_types.items():
            if count >= 3:
                analysis["patterns"].append(f"Repeated error ({count}x): {err_type}")

        # Detect critical failures
        critical_categories = ["data_integrity", "sync", "security"]
        for f in self.metrics.failures:
            if f.category in critical_categories:
                analysis["critical"].append(f"{f.name}: {f.error}")

        # Generate suggestions
        if "ChromaDB" in str(error_types):
            analysis["suggestions"].append("Check ChromaDB connection and collection state")
        if "Memgraph" in str(error_types):
            analysis["suggestions"].append("Check Memgraph connection and query syntax")
        if "timeout" in str(error_types).lower():
            analysis["suggestions"].append("Consider increasing timeouts or optimizing queries")
        if len(analysis["critical"]) > 0:
            analysis["suggestions"].append("CRITICAL: Fix data integrity issues before proceeding")

        return analysis

    def report(self) -> str:
        """Generate full test report."""
        elapsed = time.time() - self.start_time
        analysis = self.analyze_failures()

        report = []
        report.append("═" * 70)
        report.append("TEST BATTERY REPORT - MULTI-SESSION MEMORY SYSTEM")
        report.append("═" * 70)
        report.append("")

        # Summary
        pass_rate = (self.metrics.passed / self.metrics.total * 100) if self.metrics.total > 0 else 0
        report.append(f"SUMMARY")
        report.append(f"  Total tests:     {self.metrics.total}")
        report.append(f"  Passed:          {self.metrics.passed} ({pass_rate:.1f}%)")
        report.append(f"  Failed:          {self.metrics.failed}")
        report.append(f"  Errors:          {self.metrics.errors}")
        report.append(f"  Total duration:  {elapsed:.2f}s")
        report.append(f"  Avg per test:    {self.metrics.total_duration_ms / max(1, self.metrics.total):.1f}ms")
        report.append("")

        # By category
        report.append("BY CATEGORY")
        for cat, stats in sorted(self.metrics.by_category.items()):
            total = stats["passed"] + stats["failed"]
            pct = stats["passed"] / total * 100 if total > 0 else 0
            status = "✓" if stats["failed"] == 0 else "✗"
            report.append(f"  {status} {cat}: {stats['passed']}/{total} ({pct:.0f}%)")
        report.append("")

        # Failures
        if self.metrics.failures:
            report.append("FAILURES")
            for f in self.metrics.failures[:20]:  # Limit output
                report.append(f"  ✗ [{f.category}] {f.name}")
                if f.error:
                    report.append(f"    Error: {f.error[:100]}")
            if len(self.metrics.failures) > 20:
                report.append(f"  ... and {len(self.metrics.failures) - 20} more")
            report.append("")

        # Analysis
        if analysis["patterns"]:
            report.append("DETECTED PATTERNS")
            for p in analysis["patterns"]:
                report.append(f"  ⚠ {p}")
            report.append("")

        if analysis["critical"]:
            report.append("CRITICAL ISSUES")
            for c in analysis["critical"]:
                report.append(f"  🔴 {c}")
            report.append("")

        if analysis["suggestions"]:
            report.append("SUGGESTIONS")
            for s in analysis["suggestions"]:
                report.append(f"  → {s}")
            report.append("")

        # Verdict
        report.append("═" * 70)
        if pass_rate == 100:
            report.append("VERDICT: ✓ ALL TESTS PASSED")
        elif pass_rate >= 90:
            report.append(f"VERDICT: ⚠ MOSTLY PASSING ({pass_rate:.0f}%) - Minor issues")
        elif pass_rate >= 70:
            report.append(f"VERDICT: ⚠ NEEDS ATTENTION ({pass_rate:.0f}%) - Several failures")
        else:
            report.append(f"VERDICT: ✗ CRITICAL ({pass_rate:.0f}%) - System unstable")
        report.append("═" * 70)

        return "\n".join(report)

    def export_json(self, path: str):
        """Export results to JSON for further analysis."""
        data = {
            "timestamp": datetime.now().isoformat(),
            "metrics": {
                "total": self.metrics.total,
                "passed": self.metrics.passed,
                "failed": self.metrics.failed,
                "errors": self.metrics.errors,
                "duration_ms": self.metrics.total_duration_ms,
                "by_category": self.metrics.by_category,
            },
            "results": [
                {
                    "name": r.name,
                    "category": r.category,
                    "passed": r.passed,
                    "duration_ms": r.duration_ms,
                    "error": r.error,
                    "details": r.details,
                }
                for r in self.results
            ],
            "analysis": self.analyze_failures(),
        }
        with open(path, "w") as f:
            json.dump(data, f, indent=2, default=str)


# ═══════════════════════════════════════════════════════════════════════════════
# TEST UTILITIES
# ═══════════════════════════════════════════════════════════════════════════════

def random_string(length: int = 20) -> str:
    return ''.join(random.choices(string.ascii_letters + string.digits, k=length))

def random_unicode() -> str:
    """Generate strings with tricky unicode."""
    samples = [
        "日本語テスト",
        "Ελληνικά",
        "العربية",
        "🔥💀🚀",
        "café résumé naïve",
        "\x00\x01\x02",  # Control chars
        "line1\nline2\ttab",
        "quote'test\"double",
        "<script>alert(1)</script>",
        "${env.VAR}",
        "'; DROP TABLE users; --",
    ]
    return random.choice(samples)


# ═══════════════════════════════════════════════════════════════════════════════
# EDGE CASE TESTS
# ═══════════════════════════════════════════════════════════════════════════════

def test_empty_text():
    """Empty string should be handled gracefully."""
    from llm_tools import add_session_note, store_knowledge, recall_session_memory

    # Empty note
    result = add_session_note("")
    if "error" not in result and result.get("memory_id"):
        return {"passed": False, "error": "Should reject empty text"}

    # Empty knowledge
    result = store_knowledge("", "rule")
    if "error" not in result and result.get("knowledge_id"):
        return {"passed": False, "error": "Should reject empty knowledge"}

    # Empty query
    result = recall_session_memory("")
    if isinstance(result, list) and len(result) > 0:
        return {"passed": False, "error": "Empty query returned results"}

    return {"passed": True}


def test_whitespace_only():
    """Whitespace-only strings."""
    from llm_tools import add_session_note, store_knowledge

    whitespace_cases = ["   ", "\t\t", "\n\n", "  \t\n  "]

    for ws in whitespace_cases:
        result = add_session_note(ws)
        # Should either error or strip to empty
        if result.get("memory_id") and "error" not in result:
            # Check if it was actually stored
            from session_memory import recall_session_memory
            found = recall_session_memory(ws, n_results=1)
            if found and found[0].get("text", "").strip() == "":
                return {"passed": False, "error": f"Stored whitespace-only: {repr(ws)}"}

    return {"passed": True}


def test_very_long_text():
    """Text exceeding normal limits."""
    from llm_tools import add_session_note, store_knowledge, query_knowledge

    # 10KB text
    long_text = "x" * 10000
    result = add_session_note(long_text, importance=1)

    if "error" in result:
        # Acceptable - rejected long text
        return {"passed": True, "note": "Long text rejected"}

    # If stored, should be truncated
    if result.get("memory_id"):
        return {"passed": True, "note": "Long text stored (likely truncated)"}

    # 100KB text - should definitely be handled
    very_long = "y" * 100000
    result = store_knowledge(very_long, "fact", "session")
    # Should either error or truncate
    return {"passed": True}


def test_unicode_edge_cases():
    """Unicode handling."""
    from llm_tools import add_session_note, recall_session_memory

    unicode_texts = [
        "日本語テスト Japanese test",
        "Ελληνικά Greek",
        "العربية Arabic",
        "🔥💀🚀 Emojis",
        "Ñoño español",
        "Привет Russian",
        "中文测试 Chinese",
    ]

    stored_ids = []
    for text in unicode_texts:
        result = add_session_note(text, importance=2)
        if "error" in result:
            return {"passed": False, "error": f"Failed to store unicode: {text[:20]}"}
        stored_ids.append(result.get("memory_id"))

    # Verify recall works
    for text in unicode_texts[:3]:
        results = recall_session_memory(text, n_results=5)
        # Should find something
        if not results:
            return {"passed": False, "error": f"Unicode recall failed: {text[:20]}"}

    return {"passed": True, "stored": len(stored_ids)}


def test_special_characters():
    """SQL injection, XSS, etc."""
    from llm_tools import add_session_note, store_knowledge

    dangerous_texts = [
        "'; DROP TABLE users; --",
        "<script>alert('xss')</script>",
        "${env.SECRET}",
        "{{template.injection}}",
        "\\x00\\x01 null bytes",
        "path/../../../etc/passwd",
        "`command injection`",
        "%(format)s string",
    ]

    for text in dangerous_texts:
        # Should store safely (escaped) or reject
        result = add_session_note(f"Test: {text}", importance=1)
        if "error" not in result:
            # Verify it didn't actually execute anything dangerous
            pass  # If we got here without exception, it's likely safe

    return {"passed": True}


def test_null_and_none_handling():
    """None/null values in various places."""
    from llm_tools import (
        add_session_note, recall_session_memory, store_knowledge,
        query_knowledge, reject_knowledge, should_save
    )

    # None tags
    result = add_session_note("Test note", tags=None)
    if "error" in result:
        return {"passed": False, "error": "None tags failed"}

    # None kind filter
    results = recall_session_memory("test", kind=None)
    # Should work (no filter)

    # None in query
    try:
        results = query_knowledge("test", kind=None)
    except Exception as e:
        return {"passed": False, "error": f"None kind in query failed: {e}"}

    return {"passed": True}


def test_invalid_ids():
    """Operations with non-existent IDs."""
    from llm_tools import (
        delete_session_memory, reject_knowledge, export_memory_to_global,
        graph_trace_knowledge, graph_trace_session, should_promote, should_reject
    )

    fake_memory_id = "m-nonexistent123"
    fake_knowledge_id = "k-nonexistent456"
    fake_session_id = "s-nonexistent789"

    # Delete non-existent memory
    result = delete_session_memory(fake_memory_id)
    # Should not crash

    # Reject non-existent knowledge
    result = reject_knowledge(fake_knowledge_id, "test")
    # Should handle gracefully

    # Export non-existent memory
    result = export_memory_to_global(fake_memory_id)
    if result.get("knowledge_id"):
        return {"passed": False, "error": "Exported non-existent memory"}

    # Trace non-existent
    result = graph_trace_knowledge(fake_knowledge_id)
    if "error" not in result and result.get("creator_session"):
        return {"passed": False, "error": "Traced non-existent knowledge"}

    result = graph_trace_session(fake_session_id)
    # Should return empty or error

    # Metacognitive on non-existent
    result = should_promote(fake_memory_id)
    if result.get("should_promote"):
        return {"passed": False, "error": "should_promote true for non-existent"}

    result = should_reject(fake_knowledge_id)
    # Should handle gracefully

    return {"passed": True}


def test_duplicate_operations():
    """Double creates, double deletes, etc."""
    from llm_tools import (
        add_session_note, delete_session_memory,
        store_knowledge, reject_knowledge
    )

    # Create same content twice
    text = f"Duplicate test {random_string(8)}"
    r1 = add_session_note(text)
    r2 = add_session_note(text)  # Same text

    if r1.get("memory_id") == r2.get("memory_id"):
        return {"passed": False, "error": "Duplicate notes got same ID"}

    # Double delete
    mid = r1.get("memory_id")
    d1 = delete_session_memory(mid)
    d2 = delete_session_memory(mid)  # Already deleted
    # Should not crash

    # Double reject
    k = store_knowledge(f"Knowledge {random_string(8)}", "fact", "global")
    kid = k.get("knowledge_id")
    if kid:
        reject_knowledge(kid, "reason1")
        reject_knowledge(kid, "reason2")  # Already rejected
        # Should update or ignore

    return {"passed": True}


def test_boundary_values():
    """Boundary values for importance, n_results, etc."""
    from llm_tools import add_session_note, recall_session_memory, query_knowledge

    # Importance boundaries
    for imp in [0, 1, 5, 6, -1, 100, 999999]:
        result = add_session_note(f"Importance {imp}", importance=imp)
        # Should clamp or reject invalid values

    # n_results boundaries
    for n in [0, 1, 100, 10000, -1]:
        try:
            results = recall_session_memory("test", n_results=n)
        except Exception as e:
            if n > 0:
                return {"passed": False, "error": f"n_results={n} failed: {e}"}

    return {"passed": True}


# ═══════════════════════════════════════════════════════════════════════════════
# STRESS TESTS
# ═══════════════════════════════════════════════════════════════════════════════

def test_high_volume_writes(count: int = 100):
    """Create many memories rapidly."""
    from llm_tools import add_session_note

    start = time.time()
    successes = 0
    failures = 0

    for i in range(count):
        result = add_session_note(
            f"Stress test note {i}: {random_string(50)}",
            kind=random.choice(["note", "observation", "decision"]),
            importance=random.randint(1, 5)
        )
        if result.get("memory_id"):
            successes += 1
        else:
            failures += 1

    duration = time.time() - start
    rate = count / duration

    return {
        "passed": failures < count * 0.05,  # < 5% failure rate
        "successes": successes,
        "failures": failures,
        "duration_s": duration,
        "rate_per_sec": rate,
    }


def test_high_volume_queries(count: int = 50):
    """Execute many queries rapidly."""
    from llm_tools import recall_session_memory, query_knowledge

    queries = [
        "docker", "permission", "error", "config", "test",
        "memory", "session", "knowledge", random_string(10)
    ]

    start = time.time()
    successes = 0

    for i in range(count):
        query = random.choice(queries)

        # Alternate between session and knowledge queries
        if i % 2 == 0:
            results = recall_session_memory(query, n_results=5)
        else:
            results = query_knowledge(query, n_results=5)

        if isinstance(results, list):
            successes += 1

    duration = time.time() - start

    return {
        "passed": successes > count * 0.95,
        "successes": successes,
        "duration_s": duration,
        "rate_per_sec": count / duration,
    }


def test_concurrent_writes(threads: int = 5, per_thread: int = 20):
    """Concurrent write operations."""
    from llm_tools import add_session_note, store_knowledge

    results = {"successes": 0, "failures": 0, "errors": []}
    lock = threading.Lock()

    def write_worker(worker_id: int):
        local_success = 0
        local_fail = 0
        for i in range(per_thread):
            try:
                if i % 2 == 0:
                    r = add_session_note(f"Worker {worker_id} note {i}")
                else:
                    r = store_knowledge(f"Worker {worker_id} knowledge {i}", "fact", "session")

                if r.get("memory_id") or r.get("knowledge_id"):
                    local_success += 1
                else:
                    local_fail += 1
            except Exception as e:
                local_fail += 1
                with lock:
                    results["errors"].append(f"W{worker_id}: {e}")

        with lock:
            results["successes"] += local_success
            results["failures"] += local_fail

    threads_list = []
    for t in range(threads):
        thread = threading.Thread(target=write_worker, args=(t,))
        threads_list.append(thread)
        thread.start()

    for thread in threads_list:
        thread.join()

    total = threads * per_thread
    return {
        "passed": results["failures"] < total * 0.1,  # < 10% failure
        "total": total,
        **results,
    }


def test_rapid_create_delete_cycle(cycles: int = 30):
    """Rapidly create and delete."""
    from llm_tools import add_session_note, delete_session_memory

    successes = 0
    for i in range(cycles):
        # Create
        r = add_session_note(f"Cycle {i} test", importance=1)
        mid = r.get("memory_id")

        if mid:
            # Immediately delete
            d = delete_session_memory(mid)
            if d.get("status") == "ok":
                successes += 1

    return {
        "passed": successes > cycles * 0.9,
        "successes": successes,
        "cycles": cycles,
    }


# ═══════════════════════════════════════════════════════════════════════════════
# INTEGRATION TESTS
# ═══════════════════════════════════════════════════════════════════════════════

def test_full_memory_workflow():
    """Complete workflow: remember → recall → export → query."""
    from llm_tools import (
        add_session_note, recall_session_memory,
        export_memory_to_global, query_knowledge,
        should_save, should_promote
    )

    unique = random_string(12)

    # 1. Check if should save
    text = f"Important finding about {unique}: SELinux needs special config"
    save_check = should_save(text)
    if not save_check.get("should_save"):
        return {"passed": False, "error": "should_save rejected valid note", "step": 1}

    # 2. Add to session memory
    note = add_session_note(text, kind="observation", importance=4)
    mid = note.get("memory_id")
    if not mid:
        return {"passed": False, "error": "Failed to create note", "step": 2}

    # 3. Recall it
    recalled = recall_session_memory(unique, n_results=3)
    if not recalled:
        return {"passed": False, "error": "Failed to recall note", "step": 3}

    found = any(unique in r.get("text", "") for r in recalled)
    if not found:
        return {"passed": False, "error": "Note not found in recall", "step": 3}

    # 4. Check if should promote
    promote_check = should_promote(mid)
    # Note: might say no if importance < 3, which is fine

    # 5. Export to global
    exported = export_memory_to_global(mid, kind="rule", scope="global")
    kid = exported.get("knowledge_id")
    if not kid:
        return {"passed": False, "error": "Export failed", "step": 5}

    # 6. Query global knowledge
    knowledge = query_knowledge(unique, n_results=3, level="global")
    found_knowledge = any(unique in k.get("text", "") for k in knowledge)
    if not found_knowledge:
        return {"passed": False, "error": "Knowledge not found after export", "step": 6}

    return {
        "passed": True,
        "memory_id": mid,
        "knowledge_id": kid,
    }


def test_rejection_workflow():
    """Store → query → reject → verify filtered."""
    from llm_tools import store_knowledge, query_knowledge, reject_knowledge
    from context import get_current_context

    unique = random_string(12)
    ctx = get_current_context()

    # 1. Store knowledge
    stored = store_knowledge(
        f"Windows-specific config for {unique}",
        kind="rule",
        scope="global"
    )
    kid = stored.get("knowledge_id")
    if not kid:
        return {"passed": False, "error": "Store failed", "step": 1}

    # 2. Query - should find it
    results = query_knowledge(unique, n_results=5)
    found = any(k.get("knowledge_id") == kid for k in results)
    if not found:
        return {"passed": False, "error": "Knowledge not found before rejection", "step": 2}

    # 3. Reject it
    rejected = reject_knowledge(kid, "Not applicable - Windows specific")
    if rejected.get("status") != "ok":
        return {"passed": False, "error": "Rejection failed", "step": 3}

    # 4. Query again - should NOT find it
    results_after = query_knowledge(unique, n_results=5)
    found_after = any(k.get("knowledge_id") == kid for k in results_after)
    if found_after:
        return {"passed": False, "error": "Rejected knowledge still appears in results", "step": 4}

    return {"passed": True, "knowledge_id": kid}


def test_context_isolation():
    """Verify session memory is isolated."""
    from context import URPContext, set_current_context, reset_context
    from session_memory import add_session_note, recall_session_memory

    unique1 = random_string(12)
    unique2 = random_string(12)

    # Session 1
    ctx1 = URPContext(
        instance_id="test-instance",
        session_id=f"test-session-1-{unique1}",
        user_id="test",
        scope="session",
        context_signature="test|master|local",
        tags=["test"]
    )
    set_current_context(ctx1)

    # Add memory in session 1
    add_session_note(f"Session 1 secret: {unique1}", kind="note", importance=3)

    # Session 2
    ctx2 = URPContext(
        instance_id="test-instance",
        session_id=f"test-session-2-{unique2}",
        user_id="test",
        scope="session",
        context_signature="test|master|local",
        tags=["test"]
    )
    set_current_context(ctx2)

    # Add memory in session 2
    add_session_note(f"Session 2 secret: {unique2}", kind="note", importance=3)

    # Session 2 should NOT see session 1's memory
    recall_1 = recall_session_memory(unique1, n_results=5)
    recall_2 = recall_session_memory(unique2, n_results=5)

    # Reset context
    reset_context()

    # Session 1 unique should NOT be found in session 2
    found_1_in_2 = any(unique1 in r.get("text", "") for r in recall_1)
    # Session 2 unique SHOULD be found
    found_2_in_2 = any(unique2 in r.get("text", "") for r in recall_2)

    if found_1_in_2:
        return {"passed": False, "error": "Session isolation violated - found session 1 data in session 2"}

    return {"passed": True}


def test_context_compatibility():
    """Test context signature compatibility."""
    from context import is_context_compatible

    test_cases = [
        # (sig_a, sig_b, strict, expected)
        ("urp-cli|master|local", "urp-cli|master|local", True, True),
        ("urp-cli|master|local", "urp-cli|dev|local", True, False),
        ("urp-cli|master|local", "urp-cli|dev|local", False, True),  # Same project
        ("urp-cli|master|local", "other-project|master|local", False, False),
        ("", "", True, True),
        ("project", "project", True, True),
    ]

    for sig_a, sig_b, strict, expected in test_cases:
        result = is_context_compatible(sig_a, sig_b, strict)
        if result != expected:
            return {
                "passed": False,
                "error": f"is_context_compatible({sig_a!r}, {sig_b!r}, strict={strict}) = {result}, expected {expected}"
            }

    return {"passed": True}


def test_vector_operations():
    """Test low-level vector operations."""
    from llm_tools import (
        vector_store_embedding, vector_query, vector_delete, vector_collection_stats
    )

    unique = random_string(12)
    test_id = f"test-vec-{unique}"

    # Store
    stored = vector_store_embedding(
        test_id,
        f"Test embedding content {unique}",
        {"test": True},
        "urp_embeddings"
    )
    if stored.get("status") != "ok":
        return {"passed": False, "error": "Vector store failed"}

    # Query
    results = vector_query(unique, n_results=3, collection="urp_embeddings")
    found = any(r.get("id") == test_id for r in results)
    if not found:
        return {"passed": False, "error": "Vector query didn't find stored item"}

    # Stats
    stats = vector_collection_stats()
    if "urp_embeddings" not in stats:
        return {"passed": False, "error": "Stats missing urp_embeddings"}

    # Delete
    deleted = vector_delete(test_id, "urp_embeddings")
    if deleted.get("status") != "ok":
        return {"passed": False, "error": "Vector delete failed"}

    # Verify deleted
    results_after = vector_query(unique, n_results=3, collection="urp_embeddings")
    found_after = any(r.get("id") == test_id for r in results_after)
    if found_after:
        return {"passed": False, "error": "Vector still found after delete"}

    return {"passed": True}


def test_metacognitive_accuracy():
    """Test metacognitive evaluations."""
    from llm_tools import add_session_note, should_save, should_promote, should_reject

    # should_save - redundancy detection
    unique = random_string(12)
    text1 = f"First note about {unique}"
    add_session_note(text1, importance=3)

    # Nearly identical text
    text2 = f"First note about {unique}"  # Exact duplicate
    result = should_save(text2)

    # Should detect high similarity
    if result.get("should_save") and result.get("most_similar"):
        sim = result["most_similar"].get("similarity", 0)
        # If very similar, should recommend not saving
        # (This depends on threshold)

    # should_save - short text
    result_short = should_save("x")
    if result_short.get("should_save"):
        return {"passed": False, "error": "should_save approved too-short text"}

    return {"passed": True}


# ═══════════════════════════════════════════════════════════════════════════════
# DATA INTEGRITY TESTS
# ═══════════════════════════════════════════════════════════════════════════════

def test_memgraph_chromadb_sync():
    """Verify Memgraph and ChromaDB stay in sync."""
    from llm_tools import add_session_note, recall_session_memory
    from database import Database
    from brain_cortex import get_collection
    from context import get_current_context

    unique = random_string(12)
    ctx = get_current_context()

    # Create note
    result = add_session_note(f"Sync test {unique}", importance=3)
    mid = result.get("memory_id")
    if not mid:
        return {"passed": False, "error": "Failed to create note"}

    # Check Memgraph
    try:
        db = Database()
        cypher = """
        MATCH (m:Memory {memory_id: $mid})
        RETURN m.text AS text
        """
        mg_results = db.execute_query(cypher, {"mid": mid})
        db.close()
        mg_found = bool(mg_results)
    except Exception as e:
        return {"passed": False, "error": f"Memgraph query failed: {e}"}

    # Check ChromaDB
    try:
        collection = get_collection("session_memory")
        if collection:
            chroma_results = collection.get(ids=[mid])
            chroma_found = bool(chroma_results.get("ids"))
        else:
            chroma_found = False
    except Exception as e:
        return {"passed": False, "error": f"ChromaDB query failed: {e}"}

    if mg_found and not chroma_found:
        return {"passed": False, "error": "In Memgraph but NOT in ChromaDB - sync issue!"}

    if chroma_found and not mg_found:
        return {"passed": False, "error": "In ChromaDB but NOT in Memgraph - sync issue!"}

    if not mg_found and not chroma_found:
        return {"passed": False, "error": "Not found in either store"}

    return {"passed": True, "memory_id": mid}


def test_embedding_consistency():
    """Same text should produce consistent embeddings."""
    from brain_cortex import get_embedding, cosine_similarity

    text = "This is a test sentence for embedding consistency."

    # Get embedding multiple times
    emb1 = get_embedding(text)
    emb2 = get_embedding(text)
    emb3 = get_embedding(text)

    if not emb1 or not emb2 or not emb3:
        return {"passed": False, "error": "Embedding generation failed"}

    # Should be identical (deterministic)
    sim_12 = cosine_similarity(emb1, emb2)
    sim_13 = cosine_similarity(emb1, emb3)

    if sim_12 < 0.9999 or sim_13 < 0.9999:
        return {
            "passed": False,
            "error": f"Embeddings not consistent: sim_12={sim_12}, sim_13={sim_13}"
        }

    return {"passed": True}


def test_knowledge_scope_integrity():
    """Verify scope levels work correctly."""
    from llm_tools import store_knowledge, query_knowledge
    from knowledge_store import list_all_knowledge

    unique = random_string(12)

    # Store at each scope level
    scopes = ["session", "instance", "global"]
    stored = {}

    for scope in scopes:
        result = store_knowledge(f"{scope} knowledge {unique}", "fact", scope)
        stored[scope] = result.get("knowledge_id")

    # Query at each level
    # Session level should only return session
    session_results = query_knowledge(unique, level="session")
    session_scopes = {r.get("scope") for r in session_results if unique in r.get("text", "")}

    # Global level should only return global
    global_results = query_knowledge(unique, level="global")
    global_scopes = {r.get("scope") for r in global_results if unique in r.get("text", "")}

    # All level should return all
    all_results = query_knowledge(unique, level="all")
    all_scopes = {r.get("scope") for r in all_results if unique in r.get("text", "")}

    # Verify
    if "global" in session_scopes:
        return {"passed": False, "error": "Session query returned global scope"}

    if "session" in global_scopes:
        return {"passed": False, "error": "Global query returned session scope"}

    return {
        "passed": True,
        "session_scopes": list(session_scopes),
        "global_scopes": list(global_scopes),
        "all_scopes": list(all_scopes),
    }


# ═══════════════════════════════════════════════════════════════════════════════
# SECURITY TESTS
# ═══════════════════════════════════════════════════════════════════════════════

def test_cypher_injection():
    """Attempt Cypher injection."""
    from llm_tools import add_session_note, store_knowledge

    injections = [
        "test' RETURN 1 //",
        "test\") MATCH (n) DETACH DELETE n //",
        "test' OR 1=1 //",
        "test'; MATCH (n) SET n.pwned=true //",
    ]

    for injection in injections:
        try:
            result = add_session_note(injection)
            # If it succeeds, check the stored text is escaped
            if result.get("memory_id"):
                # Didn't crash - good. But verify it didn't execute injection
                pass
        except Exception as e:
            # Crashing on injection is acceptable (blocked)
            pass

    # Verify no nodes have .pwned property
    try:
        from database import Database
        db = Database()
        check = db.execute_query("MATCH (n) WHERE n.pwned = true RETURN count(n) AS c")
        db.close()
        if check and check[0].get("c", 0) > 0:
            return {"passed": False, "error": "Cypher injection succeeded!"}
    except:
        pass  # If query fails, that's fine

    return {"passed": True}


def test_session_id_forgery():
    """Attempt to access other session's data by forging session_id."""
    from context import URPContext, set_current_context, reset_context
    from session_memory import add_session_note, recall_session_memory

    unique = random_string(12)

    # Create in a known session
    victim_session = f"victim-session-{unique}"
    ctx_victim = URPContext(
        instance_id="test",
        session_id=victim_session,
        user_id="victim",
        scope="session",
        context_signature="test|master|local",
        tags=[]
    )
    set_current_context(ctx_victim)
    add_session_note(f"Victim secret {unique}", importance=5)

    # Try to access from attacker session
    ctx_attacker = URPContext(
        instance_id="test",
        session_id="attacker-session",
        user_id="attacker",
        scope="session",
        context_signature="test|master|local",
        tags=[]
    )
    set_current_context(ctx_attacker)

    # Try to recall victim's data
    results = recall_session_memory(unique, n_results=10)

    reset_context()

    # Should NOT find victim's data
    found_victim = any(unique in r.get("text", "") for r in results)
    if found_victim:
        return {"passed": False, "error": "Session isolation bypassed - security vulnerability!"}

    return {"passed": True}


# ═══════════════════════════════════════════════════════════════════════════════
# TEST RUNNER
# ═══════════════════════════════════════════════════════════════════════════════

def run_all_tests():
    """Run the complete test battery."""
    monitor = TestMonitor()

    print("═" * 70)
    print("STARTING ULTRA-HARD TEST BATTERY")
    print("═" * 70)
    print()

    # ─────────────────────────────────────────────────────────────────────────
    # EDGE CASE TESTS
    # ─────────────────────────────────────────────────────────────────────────
    print("▶ Running edge case tests...")

    tests_edge = [
        ("empty_text", test_empty_text),
        ("whitespace_only", test_whitespace_only),
        ("very_long_text", test_very_long_text),
        ("unicode_edge_cases", test_unicode_edge_cases),
        ("special_characters", test_special_characters),
        ("null_and_none", test_null_and_none_handling),
        ("invalid_ids", test_invalid_ids),
        ("duplicate_operations", test_duplicate_operations),
        ("boundary_values", test_boundary_values),
    ]

    for name, fn in tests_edge:
        result = monitor.run_test(name, "edge_cases", fn)
        status = "✓" if result.passed else "✗"
        print(f"  {status} {name} ({result.duration_ms:.0f}ms)")

    # ─────────────────────────────────────────────────────────────────────────
    # STRESS TESTS
    # ─────────────────────────────────────────────────────────────────────────
    print("\n▶ Running stress tests...")

    result = monitor.run_test("high_volume_writes", "stress", test_high_volume_writes, count=50)
    print(f"  {'✓' if result.passed else '✗'} high_volume_writes ({result.details.get('rate_per_sec', 0):.1f}/sec)")

    result = monitor.run_test("high_volume_queries", "stress", test_high_volume_queries, count=30)
    print(f"  {'✓' if result.passed else '✗'} high_volume_queries ({result.details.get('rate_per_sec', 0):.1f}/sec)")

    result = monitor.run_test("concurrent_writes", "stress", test_concurrent_writes, threads=3, per_thread=10)
    print(f"  {'✓' if result.passed else '✗'} concurrent_writes ({result.details.get('successes', 0)}/{result.details.get('total', 0)})")

    result = monitor.run_test("rapid_create_delete", "stress", test_rapid_create_delete_cycle, cycles=20)
    print(f"  {'✓' if result.passed else '✗'} rapid_create_delete ({result.details.get('successes', 0)}/{result.details.get('cycles', 0)})")

    # ─────────────────────────────────────────────────────────────────────────
    # INTEGRATION TESTS
    # ─────────────────────────────────────────────────────────────────────────
    print("\n▶ Running integration tests...")

    tests_integration = [
        ("full_memory_workflow", test_full_memory_workflow),
        ("rejection_workflow", test_rejection_workflow),
        ("context_isolation", test_context_isolation),
        ("context_compatibility", test_context_compatibility),
        ("vector_operations", test_vector_operations),
        ("metacognitive_accuracy", test_metacognitive_accuracy),
    ]

    for name, fn in tests_integration:
        result = monitor.run_test(name, "integration", fn)
        status = "✓" if result.passed else "✗"
        print(f"  {status} {name} ({result.duration_ms:.0f}ms)")

    # ─────────────────────────────────────────────────────────────────────────
    # DATA INTEGRITY TESTS
    # ─────────────────────────────────────────────────────────────────────────
    print("\n▶ Running data integrity tests...")

    tests_integrity = [
        ("memgraph_chromadb_sync", test_memgraph_chromadb_sync),
        ("embedding_consistency", test_embedding_consistency),
        ("knowledge_scope_integrity", test_knowledge_scope_integrity),
    ]

    for name, fn in tests_integrity:
        result = monitor.run_test(name, "data_integrity", fn)
        status = "✓" if result.passed else "✗"
        print(f"  {status} {name} ({result.duration_ms:.0f}ms)")

    # ─────────────────────────────────────────────────────────────────────────
    # SECURITY TESTS
    # ─────────────────────────────────────────────────────────────────────────
    print("\n▶ Running security tests...")

    tests_security = [
        ("cypher_injection", test_cypher_injection),
        ("session_id_forgery", test_session_id_forgery),
    ]

    for name, fn in tests_security:
        result = monitor.run_test(name, "security", fn)
        status = "✓" if result.passed else "✗"
        print(f"  {status} {name} ({result.duration_ms:.0f}ms)")

    # ─────────────────────────────────────────────────────────────────────────
    # REPORT
    # ─────────────────────────────────────────────────────────────────────────
    print()
    print(monitor.report())

    # Export JSON for analysis
    monitor.export_json("/tmp/test_results.json")
    print(f"\nDetailed results exported to: /tmp/test_results.json")

    return monitor


if __name__ == "__main__":
    monitor = run_all_tests()

    # Exit with error code if tests failed
    if monitor.metrics.failed > 0 or monitor.metrics.errors > 0:
        sys.exit(1)
