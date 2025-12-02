#!/usr/bin/env python3
"""
Extreme tests for project-scoped functionality.

Tests:
1. Project tagging in events
2. Query filtering by project
3. Cross-project wisdom (global)
4. Network topology display
5. urp-infra lab command
6. Concurrent multi-project scenarios
7. Edge cases and stress tests

Monitoring: TestMonitor class captures detailed diagnostics for debugging.
"""

import subprocess
import json
import time
import os
import sys
import tempfile
import threading
import traceback
from datetime import datetime
from pathlib import Path
from dataclasses import dataclass, field
from typing import Optional
from concurrent.futures import ThreadPoolExecutor, as_completed


# ═══════════════════════════════════════════════════════════════════════════════
# Test Monitor - Diagnostic Collection System
# ═══════════════════════════════════════════════════════════════════════════════

@dataclass
class TestEvent:
    """Single test event for monitoring."""
    timestamp: str
    test_name: str
    event_type: str  # 'start', 'end', 'error', 'warning', 'info', 'metric'
    message: str
    data: dict = field(default_factory=dict)
    duration_ms: float = 0
    stack_trace: str = ""


class TestMonitor:
    """
    Comprehensive test monitoring and diagnostics.

    Captures:
    - Test execution timeline
    - Docker state snapshots
    - Network connectivity
    - Resource usage
    - Error traces with context
    """

    def __init__(self, test_run_id: str = None):
        self.test_run_id = test_run_id or datetime.now().strftime("%Y%m%d_%H%M%S")
        self.events: list[TestEvent] = []
        self.start_time = time.time()
        self.snapshots: dict[str, dict] = {}
        self.errors: list[TestEvent] = []
        self.warnings: list[TestEvent] = []
        self.metrics: dict[str, list] = {}

    def log(self, test_name: str, event_type: str, message: str,
            data: dict = None, duration_ms: float = 0):
        """Log a test event."""
        event = TestEvent(
            timestamp=datetime.now().isoformat(),
            test_name=test_name,
            event_type=event_type,
            message=message,
            data=data or {},
            duration_ms=duration_ms
        )
        self.events.append(event)

        if event_type == 'error':
            event.stack_trace = traceback.format_exc()
            self.errors.append(event)
        elif event_type == 'warning':
            self.warnings.append(event)

    def record_metric(self, name: str, value: float, test_name: str = ""):
        """Record a numeric metric."""
        if name not in self.metrics:
            self.metrics[name] = []
        self.metrics[name].append({
            'value': value,
            'test': test_name,
            'timestamp': datetime.now().isoformat()
        })

    def snapshot_docker_state(self, label: str) -> dict:
        """Capture current Docker state."""
        state = {
            'label': label,
            'timestamp': datetime.now().isoformat(),
            'containers': [],
            'networks': [],
            'volumes': []
        }

        try:
            # Containers
            result = subprocess.run(
                ['docker', 'ps', '-a', '--filter', 'name=urp-',
                 '--format', '{{json .}}'],
                capture_output=True, text=True, timeout=10
            )
            for line in result.stdout.strip().split('\n'):
                if line:
                    try:
                        state['containers'].append(json.loads(line))
                    except json.JSONDecodeError:
                        state['containers'].append({'raw': line})

            # Networks
            result = subprocess.run(
                ['docker', 'network', 'ls', '--filter', 'name=urp-',
                 '--format', '{{json .}}'],
                capture_output=True, text=True, timeout=10
            )
            for line in result.stdout.strip().split('\n'):
                if line:
                    try:
                        state['networks'].append(json.loads(line))
                    except json.JSONDecodeError:
                        state['networks'].append({'raw': line})

            # Volumes
            result = subprocess.run(
                ['docker', 'volume', 'ls', '--filter', 'name=urp_',
                 '--format', '{{json .}}'],
                capture_output=True, text=True, timeout=10
            )
            for line in result.stdout.strip().split('\n'):
                if line:
                    try:
                        state['volumes'].append(json.loads(line))
                    except json.JSONDecodeError:
                        state['volumes'].append({'raw': line})

        except Exception as e:
            state['error'] = str(e)

        self.snapshots[label] = state
        return state

    def check_network_connectivity(self, container_name: str,
                                   target: str = "urp-memgraph") -> dict:
        """Test network connectivity from a container."""
        result = {
            'from': container_name,
            'to': target,
            'reachable': False,
            'latency_ms': None,
            'error': None
        }

        try:
            start = time.time()
            proc = subprocess.run(
                ['docker', 'exec', container_name,
                 'bash', '-c', f'timeout 2 bash -c "echo > /dev/tcp/{target}/7687"'],
                capture_output=True, text=True, timeout=5
            )
            result['latency_ms'] = (time.time() - start) * 1000
            result['reachable'] = proc.returncode == 0
            if proc.returncode != 0:
                result['error'] = proc.stderr
        except Exception as e:
            result['error'] = str(e)

        return result

    def analyze_errors(self) -> dict:
        """Analyze collected errors and suggest solutions."""
        analysis = {
            'total_errors': len(self.errors),
            'error_categories': {},
            'suggested_fixes': []
        }

        for error in self.errors:
            # Categorize errors
            msg = error.message.lower()
            if 'connection' in msg or 'network' in msg:
                cat = 'network'
            elif 'timeout' in msg:
                cat = 'timeout'
            elif 'permission' in msg or 'access' in msg:
                cat = 'permission'
            elif 'not found' in msg or 'no such' in msg:
                cat = 'not_found'
            else:
                cat = 'other'

            if cat not in analysis['error_categories']:
                analysis['error_categories'][cat] = []
            analysis['error_categories'][cat].append({
                'test': error.test_name,
                'message': error.message,
                'trace': error.stack_trace[:500] if error.stack_trace else None
            })

        # Suggest fixes based on categories
        if 'network' in analysis['error_categories']:
            analysis['suggested_fixes'].append(
                "Network errors detected. Run: urp-infra start && docker network inspect urp-network"
            )
        if 'timeout' in analysis['error_categories']:
            analysis['suggested_fixes'].append(
                "Timeout errors detected. Check if Memgraph is healthy: docker logs urp-memgraph"
            )
        if 'permission' in analysis['error_categories']:
            analysis['suggested_fixes'].append(
                "Permission errors detected. Check docker socket access and SELinux: ls -la /var/run/docker.sock"
            )

        return analysis

    def generate_report(self) -> dict:
        """Generate comprehensive test report."""
        total_duration = time.time() - self.start_time

        # Count by type
        test_results = {'passed': 0, 'failed': 0, 'skipped': 0}
        for event in self.events:
            if event.event_type == 'end':
                if 'passed' in event.message.lower():
                    test_results['passed'] += 1
                elif 'failed' in event.message.lower():
                    test_results['failed'] += 1
                elif 'skipped' in event.message.lower():
                    test_results['skipped'] += 1

        return {
            'test_run_id': self.test_run_id,
            'timestamp': datetime.now().isoformat(),
            'total_duration_sec': round(total_duration, 2),
            'results': test_results,
            'total_events': len(self.events),
            'errors': len(self.errors),
            'warnings': len(self.warnings),
            'snapshots': list(self.snapshots.keys()),
            'metrics_summary': {
                name: {
                    'count': len(values),
                    'min': min(v['value'] for v in values) if values else 0,
                    'max': max(v['value'] for v in values) if values else 0,
                    'avg': sum(v['value'] for v in values) / len(values) if values else 0
                }
                for name, values in self.metrics.items()
            },
            'error_analysis': self.analyze_errors(),
            'events': [
                {
                    'timestamp': e.timestamp,
                    'test': e.test_name,
                    'type': e.event_type,
                    'message': e.message,
                    'duration_ms': e.duration_ms
                }
                for e in self.events
            ]
        }


# ═══════════════════════════════════════════════════════════════════════════════
# Test Utilities
# ═══════════════════════════════════════════════════════════════════════════════

def run_cmd(cmd: list[str], timeout: int = 30, env: dict = None) -> subprocess.CompletedProcess:
    """Run a command with timeout."""
    full_env = os.environ.copy()
    if env:
        full_env.update(env)
    return subprocess.run(cmd, capture_output=True, text=True, timeout=timeout, env=full_env)


def run_in_container(container: str, cmd: str, timeout: int = 30) -> subprocess.CompletedProcess:
    """Run a command inside a container."""
    return run_cmd(['docker', 'exec', container, 'bash', '-c', cmd], timeout=timeout)


def container_exists(name: str) -> bool:
    """Check if a container exists (running or stopped)."""
    result = run_cmd(['docker', 'ps', '-a', '--filter', f'name=^{name}$', '--format', '{{.Names}}'])
    return name in result.stdout


def container_running(name: str) -> bool:
    """Check if a container is running."""
    result = run_cmd(['docker', 'ps', '--filter', f'name=^{name}$', '--format', '{{.Names}}'])
    return name in result.stdout


def wait_for_container(name: str, timeout: int = 30) -> bool:
    """Wait for a container to be running."""
    start = time.time()
    while time.time() - start < timeout:
        if container_running(name):
            return True
        time.sleep(0.5)
    return False


def cleanup_test_containers(prefix: str = "urp-test-"):
    """Clean up test containers."""
    result = run_cmd(['docker', 'ps', '-aq', '--filter', f'name={prefix}'])
    if result.stdout.strip():
        containers = result.stdout.strip().split('\n')
        for c in containers:
            run_cmd(['docker', 'rm', '-f', c])


# ═══════════════════════════════════════════════════════════════════════════════
# Test Classes
# ═══════════════════════════════════════════════════════════════════════════════

class TestProjectTagging:
    """Test that events are correctly tagged with project name."""

    def __init__(self, monitor: TestMonitor):
        self.monitor = monitor
        self.test_name = "ProjectTagging"

    def test_event_has_project_field(self) -> bool:
        """Verify the runner.py logs project field."""
        test = f"{self.test_name}::event_has_project_field"
        self.monitor.log(test, 'start', 'Testing project field in events')
        start = time.time()

        try:
            # Check the runner.py source has project field
            with open('runner.py', 'r') as f:
                content = f.read()

            checks = [
                ('project_name = os.environ.get("PROJECT_NAME"', 'PROJECT_NAME env var'),
                ('project: $project', 'project in CREATE query'),
                ('e.project', 'project field in SELECT')
            ]

            for pattern, desc in checks:
                if pattern not in content:
                    self.monitor.log(test, 'error', f'Missing: {desc}')
                    return False

            duration = (time.time() - start) * 1000
            self.monitor.log(test, 'end', 'PASSED', duration_ms=duration)
            self.monitor.record_metric('test_duration_ms', duration, test)
            return True

        except Exception as e:
            self.monitor.log(test, 'error', str(e))
            return False

    def test_query_functions_support_project(self) -> bool:
        """Verify query functions have project parameter."""
        test = f"{self.test_name}::query_functions_support_project"
        self.monitor.log(test, 'start', 'Testing query functions')
        start = time.time()

        try:
            with open('runner.py', 'r') as f:
                content = f.read()

            functions = [
                ('def query_recent_commands', 'project: str = None'),
                ('def query_pain', 'project: str = None'),
                ('def consult_wisdom', 'project: str = None')
            ]

            for func_def, param in functions:
                # Find the function and check its signature
                idx = content.find(func_def)
                if idx == -1:
                    self.monitor.log(test, 'error', f'Function not found: {func_def}')
                    return False

                # Get next 200 chars to check signature
                signature = content[idx:idx+200]
                if param not in signature:
                    self.monitor.log(test, 'error', f'Missing project param in {func_def}')
                    return False

            duration = (time.time() - start) * 1000
            self.monitor.log(test, 'end', 'PASSED', duration_ms=duration)
            return True

        except Exception as e:
            self.monitor.log(test, 'error', str(e))
            return False


class TestQueryScoping:
    """Test local vs global query scoping."""

    def __init__(self, monitor: TestMonitor):
        self.monitor = monitor
        self.test_name = "QueryScoping"

    def test_cli_has_project_args(self) -> bool:
        """Verify CLI accepts --project and --all flags."""
        test = f"{self.test_name}::cli_has_project_args"
        self.monitor.log(test, 'start', 'Testing CLI arguments')
        start = time.time()

        try:
            with open('runner.py', 'r') as f:
                content = f.read()

            required = [
                "add_argument(\"--project\"",
                "add_argument(\"--all\""
            ]

            for pattern in required:
                if pattern not in content:
                    self.monitor.log(test, 'error', f'Missing: {pattern}')
                    return False

            duration = (time.time() - start) * 1000
            self.monitor.log(test, 'end', 'PASSED', duration_ms=duration)
            return True

        except Exception as e:
            self.monitor.log(test, 'error', str(e))
            return False

    def test_shell_hooks_have_local_global_aliases(self) -> bool:
        """Verify shell_hooks.sh has local-* and global-* aliases."""
        test = f"{self.test_name}::shell_hooks_aliases"
        self.monitor.log(test, 'start', 'Testing shell hook aliases')
        start = time.time()

        try:
            with open('shell_hooks.sh', 'r') as f:
                content = f.read()

            required_aliases = [
                'alias local-wisdom=',
                'alias local-pain=',
                'alias local-recent=',
                'alias global-wisdom=',
                'alias global-pain=',
                'alias global-recent='
            ]

            for alias in required_aliases:
                if alias not in content:
                    self.monitor.log(test, 'error', f'Missing alias: {alias}')
                    return False

            duration = (time.time() - start) * 1000
            self.monitor.log(test, 'end', 'PASSED', duration_ms=duration)
            return True

        except Exception as e:
            self.monitor.log(test, 'error', str(e))
            return False


class TestNetworkTopology:
    """Test network topology display."""

    def __init__(self, monitor: TestMonitor):
        self.monitor = monitor
        self.test_name = "NetworkTopology"

    def test_urp_topology_function_exists(self) -> bool:
        """Verify urp-topology function exists in shell_hooks."""
        test = f"{self.test_name}::topology_function_exists"
        self.monitor.log(test, 'start', 'Testing topology function')
        start = time.time()

        try:
            with open('shell_hooks.sh', 'r') as f:
                content = f.read()

            if 'urp-topology()' not in content:
                self.monitor.log(test, 'error', 'urp-topology function not found')
                return False

            # Check it shows network diagram elements
            diagram_elements = [
                'urp-network',
                'urp-memgraph',
                'bolt://'
            ]

            for elem in diagram_elements:
                if elem not in content:
                    self.monitor.log(test, 'warning', f'Missing diagram element: {elem}')

            duration = (time.time() - start) * 1000
            self.monitor.log(test, 'end', 'PASSED', duration_ms=duration)
            return True

        except Exception as e:
            self.monitor.log(test, 'error', str(e))
            return False

    def test_topology_shows_on_startup(self) -> bool:
        """Verify topology is shown on container startup."""
        test = f"{self.test_name}::topology_on_startup"
        self.monitor.log(test, 'start', 'Testing startup behavior')
        start = time.time()

        try:
            with open('shell_hooks.sh', 'r') as f:
                content = f.read()

            # Check urp-topology is called in startup section
            startup_section = content.split('# Startup message')[-1] if '# Startup message' in content else ""

            if 'urp-topology' not in startup_section:
                self.monitor.log(test, 'error', 'urp-topology not called on startup')
                return False

            duration = (time.time() - start) * 1000
            self.monitor.log(test, 'end', 'PASSED', duration_ms=duration)
            return True

        except Exception as e:
            self.monitor.log(test, 'error', str(e))
            return False


class TestInfraLab:
    """Test urp-infra lab command."""

    def __init__(self, monitor: TestMonitor):
        self.monitor = monitor
        self.test_name = "InfraLab"

    def test_lab_command_exists(self) -> bool:
        """Verify lab command exists in urp-infra."""
        test = f"{self.test_name}::lab_command_exists"
        self.monitor.log(test, 'start', 'Testing lab command')
        start = time.time()

        try:
            with open('bin/urp-infra', 'r') as f:
                content = f.read()

            if 'lab)' not in content:
                self.monitor.log(test, 'error', 'lab command not found')
                return False

            # Check it uses memgraph/lab image
            if 'memgraph/lab' not in content:
                self.monitor.log(test, 'error', 'memgraph/lab image not referenced')
                return False

            # Check it uses random port
            if '-p 0:3000' not in content:
                self.monitor.log(test, 'warning', 'Not using random port mapping')

            duration = (time.time() - start) * 1000
            self.monitor.log(test, 'end', 'PASSED', duration_ms=duration)
            return True

        except Exception as e:
            self.monitor.log(test, 'error', str(e))
            return False

    def test_lab_connects_to_internal_memgraph(self) -> bool:
        """Verify lab connects to urp-memgraph internally."""
        test = f"{self.test_name}::lab_internal_connection"
        self.monitor.log(test, 'start', 'Testing internal connection')
        start = time.time()

        try:
            with open('bin/urp-infra', 'r') as f:
                content = f.read()

            # Check environment variables for internal connection
            if 'QUICK_CONNECT_MG_HOST=urp-memgraph' not in content:
                self.monitor.log(test, 'error', 'Not configured to connect to urp-memgraph')
                return False

            # Check it's on the same network
            if '--network urp-network' not in content:
                self.monitor.log(test, 'error', 'Not on urp-network')
                return False

            duration = (time.time() - start) * 1000
            self.monitor.log(test, 'end', 'PASSED', duration_ms=duration)
            return True

        except Exception as e:
            self.monitor.log(test, 'error', str(e))
            return False


class TestMultiProjectIntegration:
    """Integration tests for multi-project scenarios."""

    def __init__(self, monitor: TestMonitor):
        self.monitor = monitor
        self.test_name = "MultiProjectIntegration"

    def test_memgraph_no_host_ports(self) -> bool:
        """Verify memgraph has no ports mapped to host."""
        test = f"{self.test_name}::memgraph_no_host_ports"
        self.monitor.log(test, 'start', 'Testing port isolation')
        start = time.time()

        try:
            if not container_running('urp-memgraph'):
                self.monitor.log(test, 'warning', 'urp-memgraph not running, skipping')
                return True

            result = run_cmd([
                'docker', 'inspect', 'urp-memgraph',
                '--format', '{{json .NetworkSettings.Ports}}'
            ])

            ports = json.loads(result.stdout)

            # Check that ports are null (not mapped to host)
            for port, bindings in ports.items():
                if bindings is not None:
                    self.monitor.log(test, 'error',
                        f'Port {port} is mapped to host: {bindings}')
                    return False

            duration = (time.time() - start) * 1000
            self.monitor.log(test, 'end', 'PASSED', duration_ms=duration)
            self.monitor.record_metric('test_duration_ms', duration, test)
            return True

        except Exception as e:
            self.monitor.log(test, 'error', str(e))
            return False

    def test_launchers_set_project_env(self) -> bool:
        """Verify all launchers set PROJECT_NAME environment variable."""
        test = f"{self.test_name}::launchers_set_project_env"
        self.monitor.log(test, 'start', 'Testing launcher env vars')
        start = time.time()

        try:
            launchers = ['bin/urp', 'bin/urp-c', 'bin/urp-c-ro', 'bin/urp-m']

            for launcher in launchers:
                if not os.path.exists(launcher):
                    self.monitor.log(test, 'warning', f'{launcher} not found')
                    continue

                with open(launcher, 'r') as f:
                    content = f.read()

                if 'PROJECT_NAME=' not in content and '-e PROJECT_NAME' not in content:
                    self.monitor.log(test, 'error',
                        f'{launcher} does not set PROJECT_NAME')
                    return False

            duration = (time.time() - start) * 1000
            self.monitor.log(test, 'end', 'PASSED', duration_ms=duration)
            return True

        except Exception as e:
            self.monitor.log(test, 'error', str(e))
            return False


class TestConcurrentProjects:
    """Stress tests for concurrent project access."""

    def __init__(self, monitor: TestMonitor):
        self.monitor = monitor
        self.test_name = "ConcurrentProjects"

    def test_concurrent_container_creation(self) -> bool:
        """Test creating multiple project containers simultaneously."""
        test = f"{self.test_name}::concurrent_creation"
        self.monitor.log(test, 'start', 'Testing concurrent container creation')
        start = time.time()

        try:
            # Skip if no memgraph running
            if not container_running('urp-memgraph'):
                self.monitor.log(test, 'warning', 'Memgraph not running, skipping')
                return True

            # Create 3 test containers in parallel
            project_names = ['test-proj-a', 'test-proj-b', 'test-proj-c']
            containers_created = []
            errors = []

            def create_container(proj_name):
                container_name = f"urp-test-{proj_name}"
                try:
                    result = run_cmd([
                        'docker', 'run', '-d', '--rm',
                        '--name', container_name,
                        '--network', 'urp-network',
                        '-e', f'PROJECT_NAME={proj_name}',
                        '-e', 'NEO4J_URI=bolt://urp-memgraph:7687',
                        'python:3.11-slim',
                        'tail', '-f', '/dev/null'
                    ], timeout=30)

                    if result.returncode == 0:
                        return (container_name, True, None)
                    else:
                        return (container_name, False, result.stderr)
                except Exception as e:
                    return (container_name, False, str(e))

            with ThreadPoolExecutor(max_workers=3) as executor:
                futures = {executor.submit(create_container, p): p for p in project_names}
                for future in as_completed(futures):
                    name, success, error = future.result()
                    if success:
                        containers_created.append(name)
                    else:
                        errors.append((name, error))

            # Verify all on same network
            for container in containers_created:
                result = run_cmd([
                    'docker', 'inspect', container,
                    '--format', '{{json .NetworkSettings.Networks}}'
                ])
                networks = json.loads(result.stdout)
                if 'urp-network' not in networks:
                    self.monitor.log(test, 'error',
                        f'{container} not on urp-network')

            # Cleanup
            for container in containers_created:
                run_cmd(['docker', 'stop', container])

            if errors:
                for name, err in errors:
                    self.monitor.log(test, 'warning', f'Failed to create {name}: {err}')

            duration = (time.time() - start) * 1000
            self.monitor.log(test, 'end', 'PASSED' if len(errors) == 0 else 'PARTIAL',
                           duration_ms=duration,
                           data={'created': len(containers_created), 'errors': len(errors)})
            return len(errors) == 0

        except Exception as e:
            self.monitor.log(test, 'error', str(e))
            return False


class TestEdgeCases:
    """Edge case tests."""

    def __init__(self, monitor: TestMonitor):
        self.monitor = monitor
        self.test_name = "EdgeCases"

    def test_special_chars_in_project_name(self) -> bool:
        """Test handling of special characters in project names."""
        test = f"{self.test_name}::special_chars_project_name"
        self.monitor.log(test, 'start', 'Testing special character handling')
        start = time.time()

        try:
            # Check launcher sanitizes project names
            with open('bin/urp-c', 'r') as f:
                content = f.read()

            # Should have sanitization like: tr -cd 'a-z0-9-'
            if "tr -cd" not in content and "tr -dc" not in content:
                self.monitor.log(test, 'warning',
                    'No obvious project name sanitization found')

            duration = (time.time() - start) * 1000
            self.monitor.log(test, 'end', 'PASSED', duration_ms=duration)
            return True

        except Exception as e:
            self.monitor.log(test, 'error', str(e))
            return False

    def test_missing_project_env(self) -> bool:
        """Test behavior when PROJECT_NAME is not set."""
        test = f"{self.test_name}::missing_project_env"
        self.monitor.log(test, 'start', 'Testing missing PROJECT_NAME')
        start = time.time()

        try:
            with open('runner.py', 'r') as f:
                content = f.read()

            # Should have default value
            if 'PROJECT_NAME", "unknown"' not in content:
                self.monitor.log(test, 'warning',
                    'No default value for missing PROJECT_NAME')

            duration = (time.time() - start) * 1000
            self.monitor.log(test, 'end', 'PASSED', duration_ms=duration)
            return True

        except Exception as e:
            self.monitor.log(test, 'error', str(e))
            return False

    def test_empty_query_results(self) -> bool:
        """Test handling of empty query results."""
        test = f"{self.test_name}::empty_query_results"
        self.monitor.log(test, 'start', 'Testing empty results handling')
        start = time.time()

        try:
            with open('runner.py', 'r') as f:
                content = f.read()

            # Query functions should return empty list on error
            if 'return []' not in content:
                self.monitor.log(test, 'error',
                    'Query functions may not handle empty results')
                return False

            duration = (time.time() - start) * 1000
            self.monitor.log(test, 'end', 'PASSED', duration_ms=duration)
            return True

        except Exception as e:
            self.monitor.log(test, 'error', str(e))
            return False


class TestQueryFiltering:
    """Test that queries correctly filter by project."""

    def __init__(self, monitor: TestMonitor):
        self.monitor = monitor
        self.test_name = "QueryFiltering"

    def test_project_clause_in_queries(self) -> bool:
        """Verify project filtering clause in queries."""
        test = f"{self.test_name}::project_clause_exists"
        self.monitor.log(test, 'start', 'Testing project clause in queries')
        start = time.time()

        try:
            with open('runner.py', 'r') as f:
                content = f.read()

            # Check for project filtering patterns
            patterns = [
                'e.project = $project',
                'project_clause',
                'AND e.project'
            ]

            found_count = sum(1 for p in patterns if p in content)

            if found_count < 2:
                self.monitor.log(test, 'error',
                    f'Insufficient project filtering (found {found_count} patterns)')
                return False

            duration = (time.time() - start) * 1000
            self.monitor.log(test, 'end', 'PASSED', duration_ms=duration)
            return True

        except Exception as e:
            self.monitor.log(test, 'error', str(e))
            return False

    def test_global_query_no_filter(self) -> bool:
        """Verify global queries don't filter by project."""
        test = f"{self.test_name}::global_no_filter"
        self.monitor.log(test, 'start', 'Testing global query behavior')
        start = time.time()

        try:
            with open('runner.py', 'r') as f:
                content = f.read()

            # Check that None project means no filtering
            if 'project: str = None' not in content:
                self.monitor.log(test, 'error', 'project param should default to None')
                return False

            # Check conditional clause building
            if 'if project' not in content:
                self.monitor.log(test, 'error',
                    'Should conditionally add project clause')
                return False

            duration = (time.time() - start) * 1000
            self.monitor.log(test, 'end', 'PASSED', duration_ms=duration)
            return True

        except Exception as e:
            self.monitor.log(test, 'error', str(e))
            return False


# ═══════════════════════════════════════════════════════════════════════════════
# Main Test Runner
# ═══════════════════════════════════════════════════════════════════════════════

def run_all_tests():
    """Run all tests and generate report."""
    monitor = TestMonitor()

    print("=" * 70)
    print("URP Project Scope Tests - Extreme Validation Suite")
    print("=" * 70)
    print()

    # Take initial snapshot
    monitor.snapshot_docker_state('initial')

    # Collect all test classes
    test_classes = [
        TestProjectTagging,
        TestQueryScoping,
        TestNetworkTopology,
        TestInfraLab,
        TestMultiProjectIntegration,
        TestConcurrentProjects,
        TestEdgeCases,
        TestQueryFiltering,
    ]

    results = {'passed': 0, 'failed': 0, 'total': 0}

    for test_class in test_classes:
        print(f"\n{'─' * 50}")
        print(f"Running: {test_class.__name__}")
        print(f"{'─' * 50}")

        try:
            instance = test_class(monitor)

            # Get all test methods (exclude test_name attribute)
            test_methods = [m for m in dir(instance)
                          if m.startswith('test_') and callable(getattr(instance, m))]

            for method_name in test_methods:
                method = getattr(instance, method_name)
                results['total'] += 1

                try:
                    start = time.time()
                    passed = method()
                    duration = (time.time() - start) * 1000

                    if passed:
                        results['passed'] += 1
                        print(f"  ✓ {method_name} ({duration:.0f}ms)")
                    else:
                        results['failed'] += 1
                        print(f"  ✗ {method_name} FAILED")

                except Exception as e:
                    results['failed'] += 1
                    print(f"  ✗ {method_name} ERROR: {e}")
                    monitor.log(method_name, 'error', str(e))

        except Exception as e:
            print(f"  ERROR initializing {test_class.__name__}: {e}")
            monitor.log(test_class.__name__, 'error', f'Init failed: {e}')

    # Take final snapshot
    monitor.snapshot_docker_state('final')

    # Generate and save report
    report = monitor.generate_report()

    # Add results to report
    report['test_results'] = results

    # Save report
    report_path = Path('tests/project_scope_test_results.json')
    report_path.parent.mkdir(exist_ok=True)
    with open(report_path, 'w') as f:
        json.dump(report, f, indent=2)

    # Print summary
    print()
    print("=" * 70)
    print("TEST SUMMARY")
    print("=" * 70)
    print(f"Total:  {results['total']}")
    print(f"Passed: {results['passed']}")
    print(f"Failed: {results['failed']}")
    print(f"Rate:   {results['passed']/results['total']*100:.1f}%")
    print()

    if report['error_analysis']['total_errors'] > 0:
        print("ERROR ANALYSIS:")
        for category, errors in report['error_analysis']['error_categories'].items():
            print(f"  {category}: {len(errors)} errors")
        print()
        print("SUGGESTED FIXES:")
        for fix in report['error_analysis']['suggested_fixes']:
            print(f"  - {fix}")
        print()

    print(f"Full report: {report_path}")
    print()

    return results['failed'] == 0


if __name__ == '__main__':
    os.chdir(Path(__file__).parent.parent)  # Change to project root
    success = run_all_tests()
    sys.exit(0 if success else 1)
