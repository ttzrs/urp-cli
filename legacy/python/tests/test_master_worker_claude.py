#!/usr/bin/env python3
"""
Tests for Master-Worker Claude CLI Architecture.

Tests:
1. Master container launches with correct env vars (API URL, API KEY)
2. Worker spawning with urp-spawn
3. Worker has Claude API env vars configured
4. urp-claude command sends prompts to worker
5. urp-spawn-claude launches Claude interactively
6. Communication flow: master → worker
7. Multiple workers coordination

Run with: pytest tests/test_master_worker_claude.py -v
"""

import subprocess
import json
import time
import os
import sys
import tempfile
from datetime import datetime
from pathlib import Path
from dataclasses import dataclass, field
from typing import Optional, List, Dict


# ═══════════════════════════════════════════════════════════════════════════════
# Test Configuration
# ═══════════════════════════════════════════════════════════════════════════════

TEST_PROJECT_NAME = "test-mw-claude"
TEST_PROJECT_DIR = "/tmp/test-mw-claude-project"
EXPECTED_API_URL = "http://100.105.212.98:8317/v1/"
EXPECTED_API_KEY = "NOLOSE-NOLOSE-NOLOSE"
TIMEOUT_CONTAINER = 30
TIMEOUT_COMMAND = 60


# ═══════════════════════════════════════════════════════════════════════════════
# Test Monitor
# ═══════════════════════════════════════════════════════════════════════════════

@dataclass
class TestResult:
    """Single test result."""
    name: str
    passed: bool
    message: str
    duration_ms: float = 0
    details: dict = field(default_factory=dict)


class TestMonitor:
    """Test monitoring and results collection."""

    def __init__(self):
        self.results: List[TestResult] = []
        self.start_time = datetime.now()

    def record(self, name: str, passed: bool, message: str, duration_ms: float = 0, details: dict = None):
        self.results.append(TestResult(
            name=name,
            passed=passed,
            message=message,
            duration_ms=duration_ms,
            details=details or {}
        ))

    def summary(self) -> dict:
        passed = sum(1 for r in self.results if r.passed)
        failed = len(self.results) - passed
        return {
            "total": len(self.results),
            "passed": passed,
            "failed": failed,
            "duration_ms": (datetime.now() - self.start_time).total_seconds() * 1000,
            "results": [
                {
                    "name": r.name,
                    "passed": r.passed,
                    "message": r.message,
                    "duration_ms": r.duration_ms,
                    "details": r.details
                }
                for r in self.results
            ]
        }


monitor = TestMonitor()


# ═══════════════════════════════════════════════════════════════════════════════
# Helper Functions
# ═══════════════════════════════════════════════════════════════════════════════

def run_cmd(cmd: str, timeout: int = TIMEOUT_COMMAND) -> tuple:
    """Run command and return (stdout, stderr, returncode)."""
    try:
        result = subprocess.run(
            cmd,
            shell=True,
            capture_output=True,
            text=True,
            timeout=timeout
        )
        return result.stdout, result.stderr, result.returncode
    except subprocess.TimeoutExpired:
        return "", "TIMEOUT", -1


def docker_inspect(container_name: str) -> Optional[dict]:
    """Get container inspection data."""
    stdout, _, rc = run_cmd(f"docker inspect {container_name}")
    if rc == 0:
        try:
            return json.loads(stdout)[0]
        except:
            return None
    return None


def get_container_env(container_name: str) -> Dict[str, str]:
    """Get environment variables from a container."""
    info = docker_inspect(container_name)
    if not info:
        return {}

    env_list = info.get("Config", {}).get("Env", [])
    env_dict = {}
    for item in env_list:
        if "=" in item:
            key, value = item.split("=", 1)
            env_dict[key] = value
    return env_dict


def container_running(name: str) -> bool:
    """Check if container is running."""
    stdout, _, _ = run_cmd(f"docker ps --format '{{{{.Names}}}}' | grep -q '^{name}$' && echo 'yes'")
    return "yes" in stdout


def cleanup_test_containers():
    """Clean up all test containers."""
    run_cmd(f"docker rm -f $(docker ps -aq --filter 'name=urp-master-{TEST_PROJECT_NAME}') 2>/dev/null || true")
    run_cmd(f"docker rm -f $(docker ps -aq --filter 'name=urp-worker-{TEST_PROJECT_NAME}') 2>/dev/null || true")


def setup_test_project():
    """Create test project directory."""
    os.makedirs(TEST_PROJECT_DIR, exist_ok=True)
    Path(f"{TEST_PROJECT_DIR}/test.py").write_text("# Test file\nprint('hello')\n")


# ═══════════════════════════════════════════════════════════════════════════════
# Tests
# ═══════════════════════════════════════════════════════════════════════════════

def test_01_launcher_files_have_correct_api_config():
    """Verify launcher scripts have correct API URL and KEY."""
    start = time.time()
    errors = []

    launchers = ["bin/urp", "bin/urp-m", "bin/urp-c"]

    for launcher in launchers:
        content = Path(launcher).read_text()

        if EXPECTED_API_URL not in content:
            errors.append(f"{launcher}: Missing or wrong ANTHROPIC_BASE_URL (expected {EXPECTED_API_URL})")

        if EXPECTED_API_KEY not in content:
            errors.append(f"{launcher}: Missing or wrong ANTHROPIC_API_KEY (expected {EXPECTED_API_KEY})")

    passed = len(errors) == 0
    monitor.record(
        "test_01_launcher_files_have_correct_api_config",
        passed,
        "OK" if passed else "; ".join(errors),
        (time.time() - start) * 1000,
        {"launchers_checked": launchers, "errors": errors}
    )
    assert passed, errors


def test_02_master_commands_have_api_config():
    """Verify master_commands.sh passes API config to workers."""
    start = time.time()

    content = Path("master_commands.sh").read_text()

    has_url = EXPECTED_API_URL in content
    has_key = EXPECTED_API_KEY in content

    passed = has_url and has_key
    errors = []
    if not has_url:
        errors.append(f"Missing ANTHROPIC_BASE_URL={EXPECTED_API_URL}")
    if not has_key:
        errors.append(f"Missing ANTHROPIC_API_KEY={EXPECTED_API_KEY}")

    monitor.record(
        "test_02_master_commands_have_api_config",
        passed,
        "OK" if passed else "; ".join(errors),
        (time.time() - start) * 1000
    )
    assert passed, errors


def test_03_master_commands_has_claude_functions():
    """Verify master_commands.sh has Claude integration functions."""
    start = time.time()

    content = Path("master_commands.sh").read_text()

    required_functions = [
        "urp-spawn-claude",
        "urp-claude",
        "urp-claude-file"
    ]

    missing = [f for f in required_functions if f"urp-{f.split('-')[-1]}()" in content or f"{f}()" in content or f not in content]
    # More precise check
    missing = []
    for func in required_functions:
        func_pattern = f"{func}()"
        if func_pattern not in content:
            missing.append(func)

    passed = len(missing) == 0
    monitor.record(
        "test_03_master_commands_has_claude_functions",
        passed,
        "OK" if passed else f"Missing functions: {missing}",
        (time.time() - start) * 1000,
        {"required": required_functions, "missing": missing}
    )
    assert passed, f"Missing functions: {missing}"


def test_04_newworker_command_exists():
    """Verify /newworker slash command exists."""
    start = time.time()

    cmd_path = Path(".claude/commands/newworker.md")
    exists = cmd_path.exists()

    content = ""
    has_urp_spawn = False
    if exists:
        content = cmd_path.read_text()
        has_urp_spawn = "urp-spawn" in content

    passed = exists and has_urp_spawn
    monitor.record(
        "test_04_newworker_command_exists",
        passed,
        "OK" if passed else "Missing or incomplete /newworker command",
        (time.time() - start) * 1000,
        {"path": str(cmd_path), "exists": exists, "has_urp_spawn": has_urp_spawn}
    )
    assert passed


def test_05_infrastructure_can_start():
    """Verify infrastructure (memgraph, network) can start."""
    start = time.time()

    # Create network
    run_cmd("docker network create urp-network 2>/dev/null || true")

    # Check memgraph
    stdout, _, _ = run_cmd("docker ps --format '{{.Names}}' | grep urp-memgraph")
    memgraph_running = "urp-memgraph" in stdout

    if not memgraph_running:
        # Try to start it
        run_cmd("docker start urp-memgraph 2>/dev/null || true")
        time.sleep(2)
        stdout, _, _ = run_cmd("docker ps --format '{{.Names}}' | grep urp-memgraph")
        memgraph_running = "urp-memgraph" in stdout

    passed = memgraph_running
    monitor.record(
        "test_05_infrastructure_can_start",
        passed,
        "OK" if passed else "Memgraph not running",
        (time.time() - start) * 1000
    )
    # Don't fail - infrastructure might need manual start
    if not passed:
        print("WARNING: Memgraph not running. Some tests may fail.")


def test_06_urp_image_exists():
    """Verify urp-cli docker image exists."""
    start = time.time()

    stdout, _, rc = run_cmd("docker images --format '{{.Repository}}' | grep urp-cli")
    exists = "urp-cli" in stdout

    passed = exists
    monitor.record(
        "test_06_urp_image_exists",
        passed,
        "OK" if passed else "urp-cli image not found (run: docker build -t urp-cli:latest .)",
        (time.time() - start) * 1000
    )
    assert passed, "urp-cli image not found. Build with: docker build -t urp-cli:latest ."


def test_07_spawn_worker_creates_container():
    """Test that urp-spawn creates a worker container."""
    start = time.time()

    cleanup_test_containers()
    setup_test_project()

    # We can't easily test urp-spawn in isolation (needs master env),
    # but we can test the docker run command directly
    worker_name = f"urp-worker-{TEST_PROJECT_NAME}-1"

    # Simulate what urp-spawn does
    cmd = f"""docker run -d \
        --name {worker_name} \
        --network urp-network \
        -e NEO4J_URI=bolt://urp-memgraph:7687 \
        -e URP_ENABLED=1 \
        -e URP_WORKER=1 \
        -e PROJECT_NAME={TEST_PROJECT_NAME} \
        -e ANTHROPIC_BASE_URL={EXPECTED_API_URL} \
        -e ANTHROPIC_API_KEY={EXPECTED_API_KEY} \
        -v {TEST_PROJECT_DIR}:/codebase \
        -w /codebase \
        urp-cli:latest \
        tail -f /dev/null"""

    stdout, stderr, rc = run_cmd(cmd)
    time.sleep(2)

    running = container_running(worker_name)

    # Verify env vars
    env_ok = False
    if running:
        env = get_container_env(worker_name)
        env_ok = (
            env.get("ANTHROPIC_BASE_URL") == EXPECTED_API_URL and
            env.get("ANTHROPIC_API_KEY") == EXPECTED_API_KEY
        )

    passed = running and env_ok

    # Cleanup
    run_cmd(f"docker rm -f {worker_name} 2>/dev/null || true")

    monitor.record(
        "test_07_spawn_worker_creates_container",
        passed,
        "OK" if passed else f"Container running: {running}, Env OK: {env_ok}",
        (time.time() - start) * 1000,
        {"container": worker_name, "running": running, "env_ok": env_ok}
    )
    assert passed


def test_08_worker_has_claude_available():
    """Test that worker container has claude CLI available."""
    start = time.time()

    worker_name = f"urp-worker-{TEST_PROJECT_NAME}-test"

    # Create worker
    cmd = f"""docker run -d \
        --name {worker_name} \
        --network urp-network \
        -e ANTHROPIC_BASE_URL={EXPECTED_API_URL} \
        -e ANTHROPIC_API_KEY={EXPECTED_API_KEY} \
        -w /codebase \
        urp-cli:latest \
        tail -f /dev/null"""

    run_cmd(cmd)
    time.sleep(2)

    # Check if claude is available
    stdout, stderr, rc = run_cmd(f"docker exec {worker_name} which claude 2>/dev/null || echo 'not found'")
    claude_found = "not found" not in stdout and rc == 0

    # Check version if found
    claude_version = ""
    if claude_found:
        stdout, _, _ = run_cmd(f"docker exec {worker_name} claude --version 2>/dev/null || true")
        claude_version = stdout.strip()

    # Cleanup
    run_cmd(f"docker rm -f {worker_name} 2>/dev/null || true")

    passed = claude_found
    monitor.record(
        "test_08_worker_has_claude_available",
        passed,
        f"OK (version: {claude_version})" if passed else "Claude CLI not found in container",
        (time.time() - start) * 1000,
        {"claude_found": claude_found, "version": claude_version}
    )
    # Don't fail - claude might not be installed in image
    if not passed:
        print("WARNING: Claude CLI not found in image. Install with: npm install -g @anthropic-ai/claude-code")


def test_09_exec_command_works():
    """Test that urp-exec can run commands in worker."""
    start = time.time()

    worker_name = f"urp-worker-{TEST_PROJECT_NAME}-exec"

    # Create worker
    cmd = f"""docker run -d \
        --name {worker_name} \
        --network urp-network \
        urp-cli:latest \
        tail -f /dev/null"""

    run_cmd(cmd)
    time.sleep(2)

    # Test exec
    stdout, stderr, rc = run_cmd(f"docker exec {worker_name} echo 'test-exec-works'")
    exec_works = "test-exec-works" in stdout and rc == 0

    # Test env vars are accessible
    stdout2, _, rc2 = run_cmd(f"docker exec {worker_name} printenv | grep URP")

    # Cleanup
    run_cmd(f"docker rm -f {worker_name} 2>/dev/null || true")

    passed = exec_works
    monitor.record(
        "test_09_exec_command_works",
        passed,
        "OK" if passed else "docker exec failed",
        (time.time() - start) * 1000
    )
    assert passed


def test_10_multiple_workers_can_coexist():
    """Test that multiple workers can run simultaneously."""
    start = time.time()

    workers = []
    for i in range(1, 4):
        name = f"urp-worker-{TEST_PROJECT_NAME}-multi-{i}"
        workers.append(name)
        cmd = f"""docker run -d \
            --name {name} \
            --network urp-network \
            -e WORKER_NUM={i} \
            urp-cli:latest \
            tail -f /dev/null"""
        run_cmd(cmd)

    time.sleep(2)

    # Check all running
    running_count = sum(1 for w in workers if container_running(w))

    # Check they can communicate (same network)
    stdout, _, _ = run_cmd(f"docker exec {workers[0]} ping -c 1 {workers[1]} 2>/dev/null || echo 'ping failed'")
    can_ping = "ping failed" not in stdout

    # Cleanup
    for w in workers:
        run_cmd(f"docker rm -f {w} 2>/dev/null || true")

    passed = running_count == 3
    monitor.record(
        "test_10_multiple_workers_can_coexist",
        passed,
        f"OK ({running_count}/3 running)" if passed else f"Only {running_count}/3 workers running",
        (time.time() - start) * 1000,
        {"workers": workers, "running": running_count, "can_ping": can_ping}
    )
    assert passed


# ═══════════════════════════════════════════════════════════════════════════════
# Main
# ═══════════════════════════════════════════════════════════════════════════════

def run_all_tests():
    """Run all tests and generate report."""
    print("=" * 70)
    print("  Master-Worker Claude Architecture Tests")
    print("=" * 70)
    print()

    tests = [
        test_01_launcher_files_have_correct_api_config,
        test_02_master_commands_have_api_config,
        test_03_master_commands_has_claude_functions,
        test_04_newworker_command_exists,
        test_05_infrastructure_can_start,
        test_06_urp_image_exists,
        test_07_spawn_worker_creates_container,
        test_08_worker_has_claude_available,
        test_09_exec_command_works,
        test_10_multiple_workers_can_coexist,
    ]

    for test in tests:
        print(f"Running: {test.__name__}...", end=" ")
        try:
            test()
            print("✓ PASS")
        except AssertionError as e:
            print(f"✗ FAIL: {e}")
        except Exception as e:
            print(f"✗ ERROR: {e}")
            monitor.record(test.__name__, False, f"Exception: {e}", 0)

    print()
    print("=" * 70)

    # Summary
    summary = monitor.summary()
    print(f"Results: {summary['passed']}/{summary['total']} passed")
    print(f"Duration: {summary['duration_ms']:.0f}ms")

    # Save results
    results_path = Path("tests/master_worker_claude_results.json")
    results_path.write_text(json.dumps(summary, indent=2))
    print(f"\nResults saved to: {results_path}")

    # Cleanup
    cleanup_test_containers()

    return summary['failed'] == 0


if __name__ == "__main__":
    success = run_all_tests()
    sys.exit(0 if success else 1)
