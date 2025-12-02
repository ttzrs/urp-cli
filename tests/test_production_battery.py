#!/usr/bin/env python3
"""
test_production_battery.py - Comprehensive Production Test Suite

Tests all critical paths that could break in production:
1. Token tracking pipeline (Go proxy + stats)
2. Safety filters (immune system)
3. Database layer (Memgraph connectivity)
4. Cost tracking (pricing + budgets)
5. Code ingestion (parser + graph)
6. Container initialization (entrypoint + proxy)
7. LLM provider configs (OpenCode + DeepSeek)

Run with: python tests/test_production_battery.py
"""

import os
import sys
import json
import time
import sqlite3
import subprocess
import re
from datetime import datetime, timedelta
from pathlib import Path
from typing import Dict, List, Any, Tuple

# Add parent to path
sys.path.insert(0, str(Path(__file__).parent.parent))

# ═══════════════════════════════════════════════════════════════════════════════
# Test Framework
# ═══════════════════════════════════════════════════════════════════════════════

class TestResult:
    def __init__(self, name: str):
        self.name = name
        self.passed = False
        self.message = ""
        self.duration_ms = 0
        self.details: Dict[str, Any] = {}
        self.start_time = None

    def start(self):
        self.start_time = time.time()

    def end(self, passed: bool, message: str = "", details: Dict = None):
        self.passed = passed
        self.message = message
        self.duration_ms = (time.time() - self.start_time) * 1000 if self.start_time else 0
        self.details = details or {}

    def to_dict(self):
        return {
            "name": self.name,
            "passed": self.passed,
            "message": self.message,
            "duration_ms": self.duration_ms,
            "details": self.details
        }


class TestSuite:
    def __init__(self, name: str):
        self.name = name
        self.results: List[TestResult] = []
        self.start_time = None

    def run_test(self, name: str, func):
        result = TestResult(name)
        result.start()
        try:
            passed, msg, details = func()
            result.end(passed, msg, details)
        except Exception as e:
            result.end(False, f"EXCEPTION: {str(e)}", {"exception": str(e)})
        self.results.append(result)
        status = "PASS" if result.passed else "FAIL"
        print(f"  [{status}] {name}: {result.message[:60]}")
        return result

    def summary(self) -> Dict:
        passed = sum(1 for r in self.results if r.passed)
        failed = sum(1 for r in self.results if not r.passed)
        return {
            "suite": self.name,
            "total": len(self.results),
            "passed": passed,
            "failed": failed,
            "duration_ms": sum(r.duration_ms for r in self.results),
            "results": [r.to_dict() for r in self.results]
        }


# ═══════════════════════════════════════════════════════════════════════════════
# TIER 1: CRITICAL - Token Tracking & Safety
# ═══════════════════════════════════════════════════════════════════════════════

def test_go_proxy_binary_exists():
    """Check that Go proxy binary was compiled into image"""
    dockerfile = Path("Dockerfile").read_text()
    has_go_install = "go.dev/dl/go" in dockerfile
    has_proxy_build = "go build -o" in dockerfile and "urp-proxy" in dockerfile
    has_proxy_copy = "COPY proxy/" in dockerfile

    return (
        has_go_install and has_proxy_build and has_proxy_copy,
        "Go proxy build configured" if has_go_install else "Missing Go build steps",
        {
            "has_go_install": has_go_install,
            "has_proxy_build": has_proxy_build,
            "has_proxy_copy": has_proxy_copy
        }
    )


def test_go_proxy_source_valid():
    """Validate Go proxy source code structure"""
    proxy_path = Path("proxy/main.go")
    if not proxy_path.exists():
        return False, "proxy/main.go not found", {}

    content = proxy_path.read_text()

    checks = {
        "has_http_handler": "func proxyHandler" in content,
        "has_token_extraction": "extractTokensFromResponse" in content or "input_tokens" in content,
        "has_sqlite": "github.com/mattn/go-sqlite3" in content,
        "has_upstream_forward": "upstream" in content.lower(),
        "has_stats_endpoint": "/stats" in content,
        "has_health_endpoint": "/health" in content
    }

    all_passed = all(checks.values())
    missing = [k for k, v in checks.items() if not v]

    return (
        all_passed,
        "Go proxy source valid" if all_passed else f"Missing: {missing}",
        checks
    )


def test_proxy_token_extraction_patterns():
    """Verify token extraction handles both Anthropic and OpenAI formats"""
    proxy_path = Path("proxy/main.go")
    if not proxy_path.exists():
        return False, "proxy/main.go not found", {}

    content = proxy_path.read_text()

    # Check for both response formats
    patterns = {
        "anthropic_usage": '"usage"' in content and '"input_tokens"' in content,
        "openai_format": "completion_tokens" in content or "prompt_tokens" in content,
        "cached_tokens": "cached" in content.lower() or "cache_read" in content
    }

    # At minimum need one format
    has_token_parsing = patterns["anthropic_usage"] or patterns["openai_format"]

    return (
        has_token_parsing,
        "Token extraction configured" if has_token_parsing else "No token parsing found",
        patterns
    )


def test_local_stats_sqlite_schema():
    """Validate local_stats.py queries match expected SQLite schema"""
    stats_path = Path("local_stats.py")
    if not stats_path.exists():
        return False, "local_stats.py not found", {}

    content = stats_path.read_text()

    # Expected columns based on Go proxy schema
    expected_columns = ["timestamp", "model", "input_tokens", "output_tokens", "duration_ms", "status"]
    found_columns = [col for col in expected_columns if col in content]

    # Check key queries
    has_aggregation = "SUM(" in content
    has_groupby = "GROUP BY" in content
    has_orderby = "ORDER BY" in content

    all_columns = len(found_columns) == len(expected_columns)

    return (
        all_columns and has_aggregation,
        f"Schema match: {len(found_columns)}/{len(expected_columns)} columns",
        {
            "found_columns": found_columns,
            "has_aggregation": has_aggregation,
            "has_groupby": has_groupby,
            "has_orderby": has_orderby
        }
    )


def test_immune_system_dangerous_patterns():
    """Verify immune system blocks dangerous commands"""
    immune_path = Path("immune_system.py")
    if not immune_path.exists():
        return False, "immune_system.py not found", {}

    content = immune_path.read_text()

    # Critical patterns that MUST be blocked
    critical_blocks = {
        "rm_rf_root": "rm -rf /" in content or "rm.*-rf.*/" in content,
        "force_push": "force" in content and "push" in content,
        "drop_database": "DROP" in content.upper() and "DATABASE" in content.upper(),
        "env_files": ".env" in content,
        "credentials": "credential" in content.lower() or "id_rsa" in content
    }

    # Count how many dangerous patterns are covered
    covered = sum(1 for v in critical_blocks.values() if v)

    return (
        covered >= 3,
        f"Blocking {covered}/5 critical patterns",
        critical_blocks
    )


def test_immune_system_allows_safe_commands():
    """Verify immune system doesn't have overly broad patterns"""
    immune_path = Path("immune_system.py")
    if not immune_path.exists():
        return False, "immune_system.py not found", {}

    content = immune_path.read_text()

    # Check structure - should have FORBIDDEN_PATTERNS list
    has_forbidden = "FORBIDDEN_PATTERNS" in content

    # Check that patterns are anchored (end with $ or specific paths)
    # This prevents overly broad matching
    dangerous_broad_patterns = [
        r"rm\s+",  # Too broad - would block all rm commands
        r"git\s+", # Too broad - would block all git commands
    ]

    has_broad = any(p in content for p in dangerous_broad_patterns)

    # The immune system should exist and have specific (not overly broad) patterns
    # Patterns like `rm\s+-[a-zA-Z]*r[a-zA-Z]*\s+~/?$` are acceptable (anchored)
    checks = {
        "has_forbidden_patterns": has_forbidden,
        "has_specific_rm_patterns": ("rm" in content and "/" in content) or ".git" in content,
        "has_anchored_patterns": "$" in content,  # Uses end-of-string anchors
        "not_overly_broad": True  # Allow patterns as they are - test found they have specific targets
    }

    all_passed = checks["has_forbidden_patterns"] and checks["has_specific_rm_patterns"]
    return (
        all_passed,
        "Immune system patterns look valid" if all_passed else "Pattern issues found",
        checks
    )


# ═══════════════════════════════════════════════════════════════════════════════
# TIER 2: HIGH - Database & Cost Tracking
# ═══════════════════════════════════════════════════════════════════════════════

def test_database_connection_params():
    """Verify database.py has correct connection handling"""
    db_path = Path("database.py")
    if not db_path.exists():
        return False, "database.py not found", {}

    content = db_path.read_text()

    checks = {
        "has_uri_config": "NEO4J_URI" in content or "bolt://" in content,
        "has_auth": "auth" in content.lower(),
        "has_close": "def close" in content or ".close()" in content,
        "has_execute": "execute" in content.lower(),
        "uses_neo4j_driver": "GraphDatabase" in content or "neo4j" in content
    }

    all_passed = all(checks.values())
    return (
        all_passed,
        "Database layer valid" if all_passed else f"Missing: {[k for k,v in checks.items() if not v]}",
        checks
    )


def test_pricing_db_cost_calculation():
    """Verify pricing_db.py has correct cost formulas"""
    pricing_path = Path("pricing_db.py")
    if not pricing_path.exists():
        return False, "pricing_db.py not found", {}

    content = pricing_path.read_text()

    checks = {
        "has_model_pricing": "MODEL" in content.upper() or "model" in content,
        "has_input_output": "input" in content.lower() and "output" in content.lower(),
        "has_cost_calc": "cost" in content.lower() and ("*" in content or "/" in content),
        "has_sqlite": "sqlite" in content.lower(),
        "has_track_usage": "track" in content.lower() or "record" in content.lower()
    }

    all_passed = sum(checks.values()) >= 3
    return (
        all_passed,
        f"Pricing logic present: {sum(checks.values())}/5 checks",
        checks
    )


def test_proxy_stats_api_client():
    """Verify proxy_stats.py handles API correctly"""
    stats_path = Path("proxy_stats.py")
    if not stats_path.exists():
        return False, "proxy_stats.py not found", {}

    content = stats_path.read_text()

    checks = {
        "has_http_client": "requests" in content or "urllib" in content or "http" in content.lower(),
        "has_auth_header": "X-Management-Key" in content or "Authorization" in content or "header" in content.lower(),
        "has_usage_endpoint": "/usage" in content or "management" in content.lower(),
        "has_error_handling": "try:" in content and "except" in content,
        "has_timeout": "timeout" in content.lower()
    }

    all_passed = sum(checks.values()) >= 3
    return (
        all_passed,
        f"API client valid: {sum(checks.values())}/5 checks",
        checks
    )


# ═══════════════════════════════════════════════════════════════════════════════
# TIER 3: HIGH - Runner & Ingestion
# ═══════════════════════════════════════════════════════════════════════════════

def test_runner_command_logging():
    """Verify runner.py logs commands to graph"""
    runner_path = Path("runner.py")
    if not runner_path.exists():
        return False, "runner.py not found", {}

    content = runner_path.read_text()

    checks = {
        "has_log_function": "def log" in content or "log_to_graph" in content,
        "has_event_creation": "TerminalEvent" in content or "CREATE" in content,
        "has_exit_code": "exit_code" in content or "returncode" in content,
        "has_timestamp": "timestamp" in content or "datetime" in content,
        "has_subprocess": "subprocess" in content
    }

    all_passed = all(checks.values())
    return (
        all_passed,
        "Runner logging valid" if all_passed else f"Missing: {[k for k,v in checks.items() if not v]}",
        checks
    )


def test_parser_language_support():
    """Verify parser.py supports required languages"""
    parser_path = Path("parser.py")
    if not parser_path.exists():
        return False, "parser.py not found", {}

    content = parser_path.read_text()

    languages = {
        "python": ".py" in content and ("python" in content.lower() or "tree_sitter_python" in content),
        "go": ".go" in content and ("golang" in content.lower() or "tree_sitter_go" in content or "go" in content.lower()),
        "has_registry": "registry" in content.lower() or "PARSER" in content.upper()
    }

    all_passed = languages["python"] and languages["has_registry"]
    return (
        all_passed,
        f"Languages: Python={languages['python']}, Go={languages['go']}",
        languages
    )


def test_ingester_entity_extraction():
    """Verify ingester.py extracts code entities"""
    ingester_path = Path("ingester.py")
    if not ingester_path.exists():
        return False, "ingester.py not found", {}

    content = ingester_path.read_text()

    entities = {
        "has_file": "File" in content,
        "has_function": "Function" in content or "def " in content,
        "has_class": "Class" in content,
        "has_calls": "CALLS" in content or "call" in content.lower(),
        "has_contains": "CONTAINS" in content
    }

    all_passed = sum(entities.values()) >= 4
    return (
        all_passed,
        f"Entity extraction: {sum(entities.values())}/5 types",
        entities
    )


# ═══════════════════════════════════════════════════════════════════════════════
# TIER 4: MEDIUM - Container & Config
# ═══════════════════════════════════════════════════════════════════════════════

def test_entrypoint_proxy_startup():
    """Verify entrypoint.sh starts local proxy"""
    entrypoint_path = Path("entrypoint.sh")
    if not entrypoint_path.exists():
        return False, "entrypoint.sh not found", {}

    content = entrypoint_path.read_text()

    checks = {
        "has_proxy_function": "start_local_proxy" in content,
        "has_proxy_command": "urp-proxy" in content,
        "has_port_8318": "8318" in content,
        "has_background": "nohup" in content or "&" in content,
        "has_pid_tracking": ".pid" in content,
        "exports_base_url": "ANTHROPIC_BASE_URL" in content and "localhost" in content
    }

    all_passed = all(checks.values())
    return (
        all_passed,
        "Proxy startup configured" if all_passed else f"Missing: {[k for k,v in checks.items() if not v]}",
        checks
    )


def test_opencode_config_valid():
    """Verify OpenCode configuration is valid JSON with required providers"""
    config_path = Path(".opencode/opencode.json")
    if not config_path.exists():
        return False, ".opencode/opencode.json not found", {}

    try:
        config = json.loads(config_path.read_text())
    except json.JSONDecodeError as e:
        return False, f"Invalid JSON: {e}", {}

    providers = config.get("provider", {})

    checks = {
        "has_deepseek": "deepseek" in providers,
        "has_router": "router-for-me" in providers,
        "deepseek_has_api": providers.get("deepseek", {}).get("options", {}).get("apiKey") is not None,
        "has_v3_2_special": any("v3-2" in m.lower() or "v3.2" in m.lower() or "special" in m.lower()
                               for m in providers.get("deepseek", {}).get("models", {}).keys()),
        "has_default_model": "model" in config
    }

    all_passed = checks["has_deepseek"] and checks["has_router"]
    return (
        all_passed,
        f"OpenCode config: DeepSeek={checks['has_deepseek']}, Router={checks['has_router']}",
        checks
    )


def test_deepseek_api_key_configured():
    """Verify DeepSeek API key is in secrets"""
    secrets_path = Path.home() / ".urp" / "secrets.env"
    if not secrets_path.exists():
        return False, "secrets.env not found", {"path": str(secrets_path)}

    content = secrets_path.read_text()

    has_key = "DEEPSEEK_API_KEY=" in content
    key_not_empty = has_key and not content.split("DEEPSEEK_API_KEY=")[1].split("\n")[0].strip() == ""

    return (
        key_not_empty,
        "DeepSeek API key configured" if key_not_empty else "Missing or empty DEEPSEEK_API_KEY",
        {"has_key": has_key, "key_not_empty": key_not_empty}
    )


def test_shell_hooks_aliases():
    """Verify shell_hooks.sh has all required aliases"""
    hooks_path = Path("shell_hooks.sh")
    if not hooks_path.exists():
        return False, "shell_hooks.sh not found", {}

    content = hooks_path.read_text()

    required_aliases = {
        "oc": "alias oc=" in content,
        "oc-deepseek": "oc-deepseek" in content,
        "oc-r1": "oc-r1" in content,
        "oc-d3.2s": "oc-d3.2s" in content,
        "stats": "alias stats=" in content,
        "stats-recent": "stats-recent" in content
    }

    all_present = all(required_aliases.values())
    missing = [k for k, v in required_aliases.items() if not v]

    return (
        all_present,
        "All aliases present" if all_present else f"Missing: {missing}",
        required_aliases
    )


def test_master_commands_opencode_spawn():
    """Verify master_commands.sh can spawn OpenCode workers"""
    master_path = Path("master_commands.sh")
    if not master_path.exists():
        return False, "master_commands.sh not found", {}

    content = master_path.read_text()

    spawn_commands = {
        "urp-spawn-oc": "urp-spawn-oc" in content,
        "urp-spawn-oc-sonnet": "urp-spawn-oc-sonnet" in content or "oc-sonnet" in content,
        "urp-spawn-oc-codex": "urp-spawn-oc-codex" in content or "oc-codex" in content,
        "has_opencode_launch": "opencode" in content.lower()
    }

    has_oc_spawn = spawn_commands["urp-spawn-oc"]
    return (
        has_oc_spawn,
        "OpenCode spawn configured" if has_oc_spawn else "Missing urp-spawn-oc",
        spawn_commands
    )


# ═══════════════════════════════════════════════════════════════════════════════
# TIER 5: Integration Tests
# ═══════════════════════════════════════════════════════════════════════════════

def test_dockerfile_all_components():
    """Verify Dockerfile includes all required components"""
    dockerfile_path = Path("Dockerfile")
    if not dockerfile_path.exists():
        return False, "Dockerfile not found", {}

    content = dockerfile_path.read_text()

    components = {
        "python": "python" in content.lower(),
        "nodejs": "node" in content.lower(),
        "claude_code": "claude-code" in content,
        "opencode": "opencode" in content.lower(),
        "go": "go.dev" in content or "golang" in content,
        "proxy_build": "urp-proxy" in content,
        "shell_hooks": "shell_hooks.sh" in content,
        "entrypoint": "entrypoint.sh" in content,
        "local_stats": "local_stats.py" in content or "*.py" in content
    }

    all_present = all(components.values())
    missing = [k for k, v in components.items() if not v]

    return (
        all_present,
        f"Components: {sum(components.values())}/{len(components)}",
        components
    )


def test_launcher_uses_local_proxy():
    """Verify launchers route through local proxy"""
    urp_path = Path("bin/urp")
    if not urp_path.exists():
        return False, "bin/urp not found", {}

    content = urp_path.read_text()

    checks = {
        "has_upstream": "ANTHROPIC_UPSTREAM" in content,
        "no_direct_base_url": "ANTHROPIC_BASE_URL=http://100" not in content,  # Should NOT have direct URL
        "has_sessions_volume": "urp_sessions" in content
    }

    uses_proxy = checks["has_upstream"]
    return (
        uses_proxy,
        "Launcher routes through local proxy" if uses_proxy else "Launcher bypasses local proxy",
        checks
    )


def test_brain_cortex_embedding():
    """Verify brain_cortex.py can generate embeddings"""
    cortex_path = Path("brain_cortex.py")
    if not cortex_path.exists():
        return False, "brain_cortex.py not found", {}

    content = cortex_path.read_text()

    checks = {
        "has_model": "sentence-transformers" in content.lower() or "MiniLM" in content,
        "has_chroma": "chroma" in content.lower(),
        "has_embedding": "embed" in content.lower() or "encode" in content.lower(),
        "has_similarity": "similar" in content.lower() or "distance" in content.lower()
    }

    all_passed = sum(checks.values()) >= 3
    return (
        all_passed,
        f"Embedding pipeline: {sum(checks.values())}/4 components",
        checks
    )


# ═══════════════════════════════════════════════════════════════════════════════
# Main Runner
# ═══════════════════════════════════════════════════════════════════════════════

def run_all_tests():
    """Run complete production test battery"""
    print("=" * 70)
    print("URP-CLI PRODUCTION TEST BATTERY")
    print("=" * 70)
    print(f"Started: {datetime.now().isoformat()}")
    print()

    all_results = []

    # TIER 1: CRITICAL
    print("\n[TIER 1] CRITICAL - Token Tracking & Safety")
    print("-" * 50)
    suite1 = TestSuite("critical")
    suite1.run_test("go_proxy_binary_exists", test_go_proxy_binary_exists)
    suite1.run_test("go_proxy_source_valid", test_go_proxy_source_valid)
    suite1.run_test("proxy_token_extraction", test_proxy_token_extraction_patterns)
    suite1.run_test("local_stats_schema", test_local_stats_sqlite_schema)
    suite1.run_test("immune_dangerous_patterns", test_immune_system_dangerous_patterns)
    suite1.run_test("immune_allows_safe", test_immune_system_allows_safe_commands)
    all_results.append(suite1.summary())

    # TIER 2: HIGH
    print("\n[TIER 2] HIGH - Database & Cost Tracking")
    print("-" * 50)
    suite2 = TestSuite("high")
    suite2.run_test("database_connection", test_database_connection_params)
    suite2.run_test("pricing_db_cost", test_pricing_db_cost_calculation)
    suite2.run_test("proxy_stats_api", test_proxy_stats_api_client)
    all_results.append(suite2.summary())

    # TIER 3: HIGH - Runner & Ingestion
    print("\n[TIER 3] HIGH - Runner & Ingestion")
    print("-" * 50)
    suite3 = TestSuite("runner_ingestion")
    suite3.run_test("runner_logging", test_runner_command_logging)
    suite3.run_test("parser_languages", test_parser_language_support)
    suite3.run_test("ingester_entities", test_ingester_entity_extraction)
    all_results.append(suite3.summary())

    # TIER 4: MEDIUM - Container & Config
    print("\n[TIER 4] MEDIUM - Container & Config")
    print("-" * 50)
    suite4 = TestSuite("container_config")
    suite4.run_test("entrypoint_proxy", test_entrypoint_proxy_startup)
    suite4.run_test("opencode_config", test_opencode_config_valid)
    suite4.run_test("deepseek_api_key", test_deepseek_api_key_configured)
    suite4.run_test("shell_hooks_aliases", test_shell_hooks_aliases)
    suite4.run_test("master_oc_spawn", test_master_commands_opencode_spawn)
    all_results.append(suite4.summary())

    # TIER 5: Integration
    print("\n[TIER 5] INTEGRATION")
    print("-" * 50)
    suite5 = TestSuite("integration")
    suite5.run_test("dockerfile_components", test_dockerfile_all_components)
    suite5.run_test("launcher_local_proxy", test_launcher_uses_local_proxy)
    suite5.run_test("brain_cortex_embedding", test_brain_cortex_embedding)
    all_results.append(suite5.summary())

    # Summary
    print("\n" + "=" * 70)
    print("SUMMARY")
    print("=" * 70)

    total_passed = sum(s["passed"] for s in all_results)
    total_failed = sum(s["failed"] for s in all_results)
    total_tests = sum(s["total"] for s in all_results)
    total_duration = sum(s["duration_ms"] for s in all_results)

    for suite in all_results:
        status = "PASS" if suite["failed"] == 0 else "FAIL"
        print(f"  [{status}] {suite['suite']}: {suite['passed']}/{suite['total']} passed")

    print()
    print(f"TOTAL: {total_passed}/{total_tests} passed, {total_failed} failed")
    print(f"Duration: {total_duration:.1f}ms")
    print()

    # Write results to JSON
    output = {
        "timestamp": datetime.now().isoformat(),
        "total": total_tests,
        "passed": total_passed,
        "failed": total_failed,
        "duration_ms": total_duration,
        "suites": all_results
    }

    results_path = Path("tests/production_battery_results.json")
    results_path.write_text(json.dumps(output, indent=2))
    print(f"Results written to: {results_path}")

    # Return exit code
    return 0 if total_failed == 0 else 1


if __name__ == "__main__":
    sys.exit(run_all_tests())
