#!/usr/bin/env python3
"""
Test suite for Context Optimization System.

Tests:
1. Three optimization modes (semi, auto, hybrid)
2. Eviction scoring (LRU + importance)
3. Noise detection patterns
4. Metrics collection for A/B testing
5. Ultra-minimal rendering
"""
import os
import sys
import json
import time
import tempfile
from datetime import datetime
from pathlib import Path

# Add parent to path for imports
sys.path.insert(0, os.path.dirname(os.path.dirname(os.path.abspath(__file__))))


class TestResult:
    """Track test results."""
    def __init__(self):
        self.passed = 0
        self.failed = 0
        self.total = 0
        self.events = []

    def record(self, test_name: str, passed: bool, message: str = "", duration_ms: float = 0):
        self.total += 1
        if passed:
            self.passed += 1
        else:
            self.failed += 1
        self.events.append({
            "timestamp": datetime.now().isoformat(),
            "test": test_name,
            "type": "end",
            "message": "PASSED" if passed else f"FAILED: {message}",
            "duration_ms": duration_ms
        })


class EvictionScoring:
    """Test LRU + importance eviction scoring."""

    test_name = "EvictionScoring"

    @staticmethod
    def test_score_calculation(result: TestResult):
        """Test that eviction score is calculated correctly."""
        from runner import _calculate_eviction_score

        # New, high importance, frequently accessed = high score (keep)
        high_score_item = {
            'added_at': time.time() - 60,  # 1 min old
            'importance': 5,
            'access_count': 10
        }
        high_score = _calculate_eviction_score(high_score_item)

        # Old, low importance, rarely accessed = low score (evict)
        low_score_item = {
            'added_at': time.time() - 7200,  # 2 hours old
            'importance': 1,
            'access_count': 1
        }
        low_score = _calculate_eviction_score(low_score_item)

        passed = high_score > low_score
        result.record(
            f"{EvictionScoring.test_name}::score_calculation",
            passed,
            f"High score ({high_score:.3f}) should be > low score ({low_score:.3f})"
        )

    @staticmethod
    def test_age_decay(result: TestResult):
        """Test that older items get lower scores."""
        from runner import _calculate_eviction_score

        new_item = {'added_at': time.time() - 60, 'importance': 2, 'access_count': 1}
        old_item = {'added_at': time.time() - 3600, 'importance': 2, 'access_count': 1}

        new_score = _calculate_eviction_score(new_item)
        old_score = _calculate_eviction_score(old_item)

        passed = new_score > old_score
        result.record(
            f"{EvictionScoring.test_name}::age_decay",
            passed,
            f"New ({new_score:.3f}) should be > old ({old_score:.3f})"
        )

    @staticmethod
    def test_importance_weight(result: TestResult):
        """Test that importance affects score more than age."""
        from runner import _calculate_eviction_score

        # High importance but old
        high_imp = {'added_at': time.time() - 1800, 'importance': 5, 'access_count': 1}
        # Low importance but new
        low_imp = {'added_at': time.time() - 60, 'importance': 1, 'access_count': 1}

        high_score = _calculate_eviction_score(high_imp)
        low_score = _calculate_eviction_score(low_imp)

        # Importance should outweigh recency (40% vs 30%)
        passed = high_score > low_score
        result.record(
            f"{EvictionScoring.test_name}::importance_weight",
            passed,
            f"High importance ({high_score:.3f}) should be > low importance ({low_score:.3f})"
        )


class NoiseDetection:
    """Test noise pattern detection."""

    test_name = "NoiseDetection"

    @staticmethod
    def test_old_items_detected(result: TestResult):
        """Test that old items are detected as noise."""
        from context_manager import detect_noise_patterns

        now = int(time.time())
        items = [
            {"name": "old1", "tokens": 100, "age_sec": 2000, "old": True, "importance": 1, "access_count": 1},
            {"name": "old2", "tokens": 100, "age_sec": 2500, "old": True, "importance": 1, "access_count": 1},
            {"name": "new1", "tokens": 100, "age_sec": 60, "old": False, "importance": 1, "access_count": 1},
        ]

        patterns = detect_noise_patterns(items)
        old_pattern = next((p for p in patterns if p["type"] == "old_items"), None)

        passed = old_pattern is not None and old_pattern["count"] == 2
        result.record(
            f"{NoiseDetection.test_name}::old_items_detected",
            passed,
            f"Should detect 2 old items, got {old_pattern}"
        )

    @staticmethod
    def test_unused_items_detected(result: TestResult):
        """Test that unused items are detected."""
        from context_manager import detect_noise_patterns

        items = [
            {"name": "unused1", "tokens": 100, "age_sec": 1000, "old": False, "importance": 1, "access_count": 1},
            {"name": "used1", "tokens": 100, "age_sec": 1000, "old": False, "importance": 1, "access_count": 5},
        ]

        patterns = detect_noise_patterns(items)
        unused_pattern = next((p for p in patterns if p["type"] == "unused"), None)

        passed = unused_pattern is not None and unused_pattern["count"] == 1
        result.record(
            f"{NoiseDetection.test_name}::unused_items_detected",
            passed,
            f"Should detect 1 unused item, got {unused_pattern}"
        )

    @staticmethod
    def test_duplicate_basenames(result: TestResult):
        """Test that duplicate basenames are detected."""
        from context_manager import detect_noise_patterns

        items = [
            {"name": "/path/a/runner.py", "tokens": 100, "age_sec": 100, "old": False, "importance": 1, "access_count": 1},
            {"name": "/path/b/runner.py", "tokens": 100, "age_sec": 200, "old": False, "importance": 1, "access_count": 1},
            {"name": "/path/c/other.py", "tokens": 100, "age_sec": 100, "old": False, "importance": 1, "access_count": 1},
        ]

        patterns = detect_noise_patterns(items)
        dup_pattern = next((p for p in patterns if p["type"] == "duplicate_basenames"), None)

        passed = dup_pattern is not None
        result.record(
            f"{NoiseDetection.test_name}::duplicate_basenames",
            passed,
            f"Should detect duplicate basenames, got {dup_pattern}"
        )


class OptimizationModes:
    """Test three optimization modes."""

    test_name = "OptimizationModes"

    @staticmethod
    def test_mode_semi_auto(result: TestResult):
        """Test semi-auto mode generates user-copy instruction."""
        from context_manager import mode_semi_auto

        output = mode_semi_auto()

        passed = (
            output.get("mode") == "semi" and
            "instruction" in output and
            "copy_to_compact" in output and
            output.get("user_action_required") == True
        )
        result.record(
            f"{OptimizationModes.test_name}::mode_semi_auto",
            passed,
            f"Semi-auto should generate instruction, got {list(output.keys())}"
        )

    @staticmethod
    def test_mode_aggressive(result: TestResult):
        """Test aggressive mode cleans automatically."""
        from context_manager import mode_aggressive

        output = mode_aggressive()

        passed = (
            output.get("mode") == "auto" and
            "tokens_freed" in output and
            "items_removed" in output and
            output.get("user_action_required") == False
        )
        result.record(
            f"{OptimizationModes.test_name}::mode_aggressive",
            passed,
            f"Aggressive should auto-clean, got {list(output.keys())}"
        )

    @staticmethod
    def test_mode_hybrid(result: TestResult):
        """Test hybrid mode: auto-clean safe, ask for uncertain."""
        from context_manager import mode_hybrid

        output = mode_hybrid()

        passed = (
            output.get("mode") == "hybrid" and
            "auto_cleaned" in output and
            "pending_decisions" in output
        )
        result.record(
            f"{OptimizationModes.test_name}::mode_hybrid",
            passed,
            f"Hybrid should have auto_cleaned and pending, got {list(output.keys())}"
        )

    @staticmethod
    def test_mode_switching(result: TestResult):
        """Test mode can be switched."""
        from context_manager import set_mode, get_current_mode

        # Set to auto
        set_mode("auto")
        mode1 = get_current_mode()

        # Set to semi
        set_mode("semi")
        mode2 = get_current_mode()

        # Reset to hybrid
        set_mode("hybrid")
        mode3 = get_current_mode()

        passed = mode1 == "auto" and mode2 == "semi" and mode3 == "hybrid"
        result.record(
            f"{OptimizationModes.test_name}::mode_switching",
            passed,
            f"Modes: auto={mode1}, semi={mode2}, hybrid={mode3}"
        )


class MetricsCollection:
    """Test A/B testing metrics collection."""

    test_name = "MetricsCollection"

    @staticmethod
    def test_record_tokens_saved(result: TestResult):
        """Test recording tokens saved metric."""
        from context_manager import record_metric, load_metrics

        # Record some metrics
        record_metric("auto", "tokens_saved", 1000)
        record_metric("auto", "tokens_saved", 500)

        metrics = load_metrics()
        auto_data = metrics["by_mode"].get("auto", {})

        passed = (
            auto_data.get("tokens_saved", 0) >= 1500 and
            auto_data.get("actions", 0) >= 2
        )
        result.record(
            f"{MetricsCollection.test_name}::record_tokens_saved",
            passed,
            f"Should have 1500+ tokens, got {auto_data.get('tokens_saved', 0)}"
        )

    @staticmethod
    def test_record_quality(result: TestResult):
        """Test recording quality feedback."""
        from context_manager import record_quality, load_metrics, set_mode

        set_mode("hybrid")
        record_quality(4)
        record_quality(5)

        metrics = load_metrics()
        hybrid_data = metrics["by_mode"].get("hybrid", {})

        passed = hybrid_data.get("quality_count", 0) >= 2
        result.record(
            f"{MetricsCollection.test_name}::record_quality",
            passed,
            f"Should have 2+ quality samples, got {hybrid_data.get('quality_count', 0)}"
        )

    @staticmethod
    def test_mode_stats(result: TestResult):
        """Test getting stats per mode."""
        from context_manager import get_mode_stats

        stats = get_mode_stats()

        passed = (
            "semi" in stats and
            "auto" in stats and
            "hybrid" in stats and
            all("avg_tokens_saved" in s for s in stats.values())
        )
        result.record(
            f"{MetricsCollection.test_name}::mode_stats",
            passed,
            f"Should have stats for all modes, got {list(stats.keys())}"
        )

    @staticmethod
    def test_recommend_best_mode(result: TestResult):
        """Test mode recommendation."""
        from context_manager import recommend_best_mode

        rec = recommend_best_mode()

        passed = (
            "recommended" in rec and
            rec["recommended"] in ("semi", "auto", "hybrid") and
            "reason" in rec
        )
        result.record(
            f"{MetricsCollection.test_name}::recommend_best_mode",
            passed,
            f"Should recommend a valid mode, got {rec.get('recommended')}"
        )


class UltraMinimalRender:
    """Test ultra-minimal rendering."""

    test_name = "UltraMinimalRender"

    @staticmethod
    def test_render_events(result: TestResult):
        """Test rendering events as symbol chain."""
        from brain_render import render_ultra_minimal

        events = [
            {"exit_code": 0, "cmd": "git status"},
            {"exit_code": 0, "cmd": "npm install"},
            {"exit_code": 1, "cmd": "pytest tests"},
        ]

        output = render_ultra_minimal(events)

        # Should be like "✓git ✓npm ✗pytest"
        passed = "✓" in output and "✗" in output and len(output) < 50
        result.record(
            f"{UltraMinimalRender.test_name}::render_events",
            passed,
            f"Should be compact symbols, got: {output}"
        )

    @staticmethod
    def test_render_token_status(result: TestResult):
        """Test rendering token budget status."""
        from brain_render import render_ultra_minimal

        status = {"used": 25000, "budget": 50000, "usage_pct": 50}
        output = render_ultra_minimal(status)

        # Should be compact like "✓50%" or "⚠50%"
        passed = "50" in output and len(output) < 20
        result.record(
            f"{UltraMinimalRender.test_name}::render_token_status",
            passed,
            f"Should be compact percentage, got: {output}"
        )

    @staticmethod
    def test_render_status_bar(result: TestResult):
        """Test full status bar rendering."""
        from brain_render import render_status_bar

        context_status = {
            "working_memory": {"count": 5, "total_tokens": 1200},
            "noise_patterns": [{"type": "old_items", "count": 3, "severity": "medium"}]
        }
        token_status = {"usage_pct": 45}

        output = render_status_bar(context_status, token_status, mode="hybrid")

        # Should be single-line compact status
        passed = "hyb" in output and "1.2K" in output and len(output) < 80
        result.record(
            f"{UltraMinimalRender.test_name}::render_status_bar",
            passed,
            f"Should be compact status bar, got: {output}"
        )

    @staticmethod
    def test_token_savings(result: TestResult):
        """Test that ultra-minimal uses fewer tokens than regular."""
        from brain_render import render_ultra_minimal, render_as_trace

        events = [
            {"exit_code": 0, "cmd": "git status", "datetime": "2025-01-01T12:00:00"},
            {"exit_code": 1, "cmd": "pytest tests/unit", "error": "AssertionError: test failed"},
            {"exit_code": 0, "cmd": "npm run build"},
        ]

        ultra = render_ultra_minimal(events)
        full = render_as_trace(events)

        # Ultra should be much shorter
        savings = 1 - (len(ultra) / len(full))
        passed = savings > 0.5  # At least 50% savings

        result.record(
            f"{UltraMinimalRender.test_name}::token_savings",
            passed,
            f"Should save 50%+ tokens, saved {savings:.0%} (ultra={len(ultra)}, full={len(full)})"
        )


def run_all_tests():
    """Run all test classes."""
    result = TestResult()

    test_classes = [
        EvictionScoring,
        NoiseDetection,
        OptimizationModes,
        MetricsCollection,
        UltraMinimalRender,
    ]

    print("=" * 60)
    print("Context Optimization Test Suite")
    print("=" * 60)
    print()

    for test_class in test_classes:
        print(f"Running {test_class.test_name}...")
        for method_name in dir(test_class):
            if method_name.startswith('test_'):
                method = getattr(test_class, method_name)
                if callable(method):
                    try:
                        start = time.time()
                        method(result)
                        duration = (time.time() - start) * 1000
                    except Exception as e:
                        result.record(
                            f"{test_class.test_name}::{method_name}",
                            False,
                            str(e)
                        )

    print()
    print("=" * 60)
    print(f"Results: {result.passed}/{result.total} passed, {result.failed} failed")
    print("=" * 60)

    # Print failures
    failures = [e for e in result.events if "FAILED" in e["message"]]
    if failures:
        print("\nFailures:")
        for f in failures:
            print(f"  - {f['test']}: {f['message']}")

    # Save results
    output_file = os.path.join(os.path.dirname(__file__), "context_optimization_results.json")
    with open(output_file, 'w') as f:
        json.dump({
            "test_run_id": datetime.now().strftime("%Y%m%d_%H%M%S"),
            "timestamp": datetime.now().isoformat(),
            "results": {
                "passed": result.passed,
                "failed": result.failed,
                "total": result.total
            },
            "events": result.events
        }, f, indent=2)

    print(f"\nResults saved to: {output_file}")

    return result.failed == 0


if __name__ == "__main__":
    success = run_all_tests()
    sys.exit(0 if success else 1)
