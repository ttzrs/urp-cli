#!/usr/bin/env python3
"""
A/B Testing Orchestrator for Context Optimization Modes.

Spawns 4 parallel containers, each with a different optimization mode,
creates separate git branches, collects metrics, and stores in Memgraph.

Architecture:
┌─────────────────────────────────────────────────────────────────────┐
│                     ab_orchestrator.py                               │
│  - Spawns 4 containers: none, semi, auto, hybrid                    │
│  - Creates branches: ab/<session>/<mode>                            │
│  - Collects metrics during execution                                │
│  - Syncs results to Memgraph for analysis                          │
└─────────────────────────────────────────────────────────────────────┘
"""
import os
import sys
import json
import time
import uuid
import subprocess
import threading
from datetime import datetime
from pathlib import Path
from typing import Optional, Dict, List
from dataclasses import dataclass, asdict
from concurrent.futures import ThreadPoolExecutor, as_completed

# ═══════════════════════════════════════════════════════════════════════════════
# Configuration
# ═══════════════════════════════════════════════════════════════════════════════

MODES = ["none", "semi", "auto", "hybrid"]
AB_SESSIONS_DIR = os.getenv("AB_SESSIONS_DIR", "/shared/ab_sessions")
CONTAINER_IMAGE = os.getenv("URP_IMAGE", "urp-cli:latest")
NETWORK_NAME = "urp-network"
MEMGRAPH_URI = os.getenv("NEO4J_URI", "bolt://urp-memgraph:7687")

# Timeouts
CONTAINER_STARTUP_TIMEOUT = 30  # seconds
TASK_TIMEOUT = 600  # 10 minutes max per task

# ═══════════════════════════════════════════════════════════════════════════════
# Data Models
# ═══════════════════════════════════════════════════════════════════════════════

@dataclass
class ABSession:
    """Represents a parallel A/B testing session."""
    session_id: str
    project_path: str
    task_description: str
    start_time: str
    end_time: Optional[str] = None
    base_branch: str = "main"
    status: str = "pending"  # pending, running, completed, failed


@dataclass
class ABContainer:
    """Represents one container in the A/B test."""
    container_id: str
    session_id: str
    mode: str
    branch_name: str
    container_name: str
    status: str = "pending"
    start_time: Optional[str] = None
    end_time: Optional[str] = None
    exit_code: Optional[int] = None


@dataclass
class ABMetric:
    """Metric collected from a container."""
    session_id: str
    mode: str
    metric_type: str
    value: float
    timestamp: str
    context: Optional[dict] = None


@dataclass
class ABResult:
    """Final result from one container."""
    session_id: str
    mode: str
    tokens_used: int
    tokens_saved: int
    execution_time_ms: int
    errors: int
    quality_indicators: Dict[str, float]
    branch_name: str
    commit_sha: Optional[str] = None
    files_changed: int = 0
    lines_added: int = 0
    lines_removed: int = 0


# ═══════════════════════════════════════════════════════════════════════════════
# Memgraph Integration
# ═══════════════════════════════════════════════════════════════════════════════

class MemgraphSync:
    """Sync A/B test data to Memgraph."""

    def __init__(self, uri: str = MEMGRAPH_URI):
        self.uri = uri
        self._driver = None

    def _get_driver(self):
        """Lazy-load neo4j driver."""
        if self._driver is None:
            try:
                from neo4j import GraphDatabase
                self._driver = GraphDatabase.driver(self.uri)
            except Exception as e:
                print(f"Warning: Could not connect to Memgraph: {e}")
                return None
        return self._driver

    def create_session(self, session: ABSession):
        """Create ABSession node in Memgraph."""
        driver = self._get_driver()
        if not driver:
            return False

        query = """
        MERGE (s:ABSession {session_id: $session_id})
        SET s.project_path = $project_path,
            s.task_description = $task_description,
            s.start_time = $start_time,
            s.base_branch = $base_branch,
            s.status = $status
        RETURN s
        """
        try:
            with driver.session() as db_session:
                db_session.run(query, **asdict(session))
            return True
        except Exception as e:
            print(f"Error creating session: {e}")
            return False

    def create_container(self, container: ABContainer):
        """Create ABContainer node linked to session."""
        driver = self._get_driver()
        if not driver:
            return False

        query = """
        MATCH (s:ABSession {session_id: $session_id})
        MERGE (c:ABContainer {container_id: $container_id})
        SET c.mode = $mode,
            c.branch_name = $branch_name,
            c.container_name = $container_name,
            c.status = $status,
            c.start_time = $start_time
        MERGE (s)-[:RUNS]->(c)
        RETURN c
        """
        try:
            with driver.session() as db_session:
                db_session.run(query, **asdict(container))
            return True
        except Exception as e:
            print(f"Error creating container: {e}")
            return False

    def record_metric(self, metric: ABMetric):
        """Record a metric from a container."""
        driver = self._get_driver()
        if not driver:
            return False

        query = """
        MATCH (c:ABContainer {session_id: $session_id, mode: $mode})
        CREATE (m:ABMetric {
            metric_type: $metric_type,
            value: $value,
            timestamp: $timestamp,
            context: $context_json
        })
        CREATE (c)-[:MEASURED]->(m)
        RETURN m
        """
        try:
            with driver.session() as db_session:
                db_session.run(
                    query,
                    session_id=metric.session_id,
                    mode=metric.mode,
                    metric_type=metric.metric_type,
                    value=metric.value,
                    timestamp=metric.timestamp,
                    context_json=json.dumps(metric.context) if metric.context else "{}"
                )
            return True
        except Exception as e:
            print(f"Error recording metric: {e}")
            return False

    def save_result(self, result: ABResult):
        """Save final result from a container."""
        driver = self._get_driver()
        if not driver:
            return False

        query = """
        MATCH (c:ABContainer {session_id: $session_id, mode: $mode})
        SET c.status = 'completed',
            c.end_time = datetime(),
            c.tokens_used = $tokens_used,
            c.tokens_saved = $tokens_saved,
            c.execution_time_ms = $execution_time_ms,
            c.errors = $errors,
            c.commit_sha = $commit_sha,
            c.files_changed = $files_changed,
            c.lines_added = $lines_added,
            c.lines_removed = $lines_removed

        // Create result node with quality indicators
        CREATE (r:ABResult {
            session_id: $session_id,
            mode: $mode,
            quality_indicators: $quality_json,
            branch_name: $branch_name
        })
        CREATE (c)-[:PRODUCED]->(r)
        RETURN r
        """
        try:
            with driver.session() as db_session:
                db_session.run(
                    query,
                    session_id=result.session_id,
                    mode=result.mode,
                    tokens_used=result.tokens_used,
                    tokens_saved=result.tokens_saved,
                    execution_time_ms=result.execution_time_ms,
                    errors=result.errors,
                    commit_sha=result.commit_sha,
                    files_changed=result.files_changed,
                    lines_added=result.lines_added,
                    lines_removed=result.lines_removed,
                    quality_json=json.dumps(result.quality_indicators),
                    branch_name=result.branch_name
                )
            return True
        except Exception as e:
            print(f"Error saving result: {e}")
            return False

    def update_session_status(self, session_id: str, status: str, end_time: str = None):
        """Update session status."""
        driver = self._get_driver()
        if not driver:
            return False

        query = """
        MATCH (s:ABSession {session_id: $session_id})
        SET s.status = $status
        """
        if end_time:
            query += ", s.end_time = $end_time"
        query += " RETURN s"

        try:
            with driver.session() as db_session:
                db_session.run(query, session_id=session_id, status=status, end_time=end_time)
            return True
        except Exception as e:
            print(f"Error updating session: {e}")
            return False

    def get_mode_statistics(self, limit: int = 100) -> Dict:
        """Get aggregated statistics per mode."""
        driver = self._get_driver()
        if not driver:
            return {}

        query = """
        MATCH (c:ABContainer)
        WHERE c.status = 'completed'
        WITH c.mode AS mode,
             count(c) AS runs,
             avg(c.tokens_used) AS avg_tokens_used,
             avg(c.tokens_saved) AS avg_tokens_saved,
             avg(c.execution_time_ms) AS avg_time_ms,
             sum(c.errors) AS total_errors,
             avg(c.files_changed) AS avg_files_changed
        RETURN mode, runs, avg_tokens_used, avg_tokens_saved,
               avg_time_ms, total_errors, avg_files_changed
        ORDER BY runs DESC
        LIMIT $limit
        """
        try:
            with driver.session() as db_session:
                result = db_session.run(query, limit=limit)
                stats = {}
                for record in result:
                    stats[record["mode"]] = {
                        "runs": record["runs"],
                        "avg_tokens_used": record["avg_tokens_used"],
                        "avg_tokens_saved": record["avg_tokens_saved"],
                        "avg_time_ms": record["avg_time_ms"],
                        "total_errors": record["total_errors"],
                        "avg_files_changed": record["avg_files_changed"]
                    }
                return stats
        except Exception as e:
            print(f"Error getting statistics: {e}")
            return {}

    def get_best_mode(self) -> Dict:
        """Recommend best mode based on collected data."""
        driver = self._get_driver()
        if not driver:
            return {"recommended": "hybrid", "reason": "No data available"}

        query = """
        MATCH (c:ABContainer)
        WHERE c.status = 'completed'
        WITH c.mode AS mode,
             count(c) AS runs,
             avg(toFloat(c.tokens_saved)) AS efficiency,
             avg(toFloat(c.errors)) AS error_rate,
             avg(toFloat(c.execution_time_ms)) AS speed
        WHERE runs >= 3  // Minimum samples

        // Calculate composite score
        // Higher efficiency, lower errors, lower time = better
        WITH mode, runs, efficiency, error_rate, speed,
             (coalesce(efficiency, 0) * 0.4) -
             (coalesce(error_rate, 0) * 100 * 0.3) -
             (coalesce(speed, 0) / 10000 * 0.3) AS score

        RETURN mode, runs, efficiency, error_rate, speed, score
        ORDER BY score DESC
        LIMIT 1
        """
        try:
            with driver.session() as db_session:
                result = db_session.run(query)
                record = result.single()
                if record:
                    return {
                        "recommended": record["mode"],
                        "runs": record["runs"],
                        "efficiency": record["efficiency"],
                        "error_rate": record["error_rate"],
                        "speed": record["speed"],
                        "score": record["score"],
                        "reason": f"Best composite score after {record['runs']} runs"
                    }
                return {"recommended": "hybrid", "reason": "Insufficient data (< 3 runs per mode)"}
        except Exception as e:
            print(f"Error getting best mode: {e}")
            return {"recommended": "hybrid", "reason": str(e)}

    def close(self):
        """Close driver connection."""
        if self._driver:
            self._driver.close()


# ═══════════════════════════════════════════════════════════════════════════════
# Container Management
# ═══════════════════════════════════════════════════════════════════════════════

class ContainerManager:
    """Manage Docker containers for A/B testing."""

    def __init__(self, project_path: str, session_id: str):
        self.project_path = os.path.abspath(project_path)
        self.session_id = session_id
        self.containers: Dict[str, ABContainer] = {}
        self.memgraph = MemgraphSync()

    def _run_docker(self, args: List[str], timeout: int = 60) -> subprocess.CompletedProcess:
        """Run docker command."""
        cmd = ["docker"] + args
        return subprocess.run(cmd, capture_output=True, text=True, timeout=timeout)

    def _create_branch(self, mode: str) -> str:
        """Create git branch for this mode."""
        branch_name = f"ab/{self.session_id}/{mode}"

        # Get current branch
        result = subprocess.run(
            ["git", "-C", self.project_path, "rev-parse", "--abbrev-ref", "HEAD"],
            capture_output=True, text=True
        )
        base_branch = result.stdout.strip() if result.returncode == 0 else "main"

        # Create and checkout branch
        subprocess.run(
            ["git", "-C", self.project_path, "checkout", "-b", branch_name],
            capture_output=True, text=True
        )

        # Return to base branch
        subprocess.run(
            ["git", "-C", self.project_path, "checkout", base_branch],
            capture_output=True, text=True
        )

        return branch_name

    def spawn_container(self, mode: str) -> ABContainer:
        """Spawn a container for the given mode."""
        container_name = f"urp-ab-{self.session_id[:8]}-{mode}"
        branch_name = self._create_branch(mode)

        container = ABContainer(
            container_id=str(uuid.uuid4()),
            session_id=self.session_id,
            mode=mode,
            branch_name=branch_name,
            container_name=container_name,
            status="starting",
            start_time=datetime.now().isoformat()
        )

        # Docker run command
        docker_args = [
            "run", "-d",
            "--name", container_name,
            "--network", NETWORK_NAME,
            "-v", f"{self.project_path}:/workspace",
            "-e", f"URP_MODE={mode}",
            "-e", f"AB_SESSION_ID={self.session_id}",
            "-e", f"AB_BRANCH={branch_name}",
            "-e", f"NEO4J_URI={MEMGRAPH_URI}",
            "-e", f"PROJECT_NAME=ab-{mode}",
            "-w", "/workspace",
            CONTAINER_IMAGE,
            "tail", "-f", "/dev/null"  # Keep alive
        ]

        result = self._run_docker(docker_args)

        if result.returncode == 0:
            container.status = "running"
            self.containers[mode] = container

            # Record in Memgraph
            self.memgraph.create_container(container)

            print(f"  ✓ Container {container_name} started (mode: {mode})")
        else:
            container.status = "failed"
            print(f"  ✗ Failed to start {container_name}: {result.stderr}")

        return container

    def execute_task(self, mode: str, task_script: str) -> ABResult:
        """Execute task in container and collect metrics."""
        container = self.containers.get(mode)
        if not container or container.status != "running":
            return None

        start_time = time.time()

        # Checkout the branch inside container
        self._run_docker([
            "exec", container.container_name,
            "git", "checkout", container.branch_name
        ])

        # Set optimization mode
        self._run_docker([
            "exec", container.container_name,
            "python3", "/app/context_manager.py", "mode", mode
        ])

        # Execute the task
        result = self._run_docker(
            ["exec", container.container_name, "bash", "-c", task_script],
            timeout=TASK_TIMEOUT
        )

        execution_time = int((time.time() - start_time) * 1000)

        # Collect metrics from container
        metrics = self._collect_container_metrics(container)

        # Get git diff stats
        diff_stats = self._get_git_stats(container)

        # Commit changes
        commit_sha = self._commit_changes(container, f"AB test: {mode} mode")

        ab_result = ABResult(
            session_id=self.session_id,
            mode=mode,
            tokens_used=metrics.get("tokens_used", 0),
            tokens_saved=metrics.get("tokens_saved", 0),
            execution_time_ms=execution_time,
            errors=1 if result.returncode != 0 else 0,
            quality_indicators=metrics.get("quality", {}),
            branch_name=container.branch_name,
            commit_sha=commit_sha,
            files_changed=diff_stats.get("files", 0),
            lines_added=diff_stats.get("added", 0),
            lines_removed=diff_stats.get("removed", 0)
        )

        # Save to Memgraph
        self.memgraph.save_result(ab_result)

        return ab_result

    def _collect_container_metrics(self, container: ABContainer) -> Dict:
        """Collect metrics from a running container."""
        result = self._run_docker([
            "exec", container.container_name,
            "python3", "/app/context_manager.py", "stats"
        ])

        # Parse output
        metrics = {
            "tokens_used": 0,
            "tokens_saved": 0,
            "quality": {}
        }

        if result.returncode == 0:
            # Try to extract numbers from output
            lines = result.stdout.split("\n")
            for line in lines:
                if "tokens saved" in line.lower():
                    try:
                        metrics["tokens_saved"] = int(''.join(filter(str.isdigit, line.split(":")[-1])))
                    except:
                        pass

        return metrics

    def _get_git_stats(self, container: ABContainer) -> Dict:
        """Get git diff statistics."""
        result = self._run_docker([
            "exec", container.container_name,
            "git", "diff", "--shortstat", "HEAD~1"
        ])

        stats = {"files": 0, "added": 0, "removed": 0}

        if result.returncode == 0 and result.stdout.strip():
            # Parse: "3 files changed, 100 insertions(+), 50 deletions(-)"
            output = result.stdout.strip()
            import re

            files_match = re.search(r'(\d+) file', output)
            added_match = re.search(r'(\d+) insertion', output)
            removed_match = re.search(r'(\d+) deletion', output)

            if files_match:
                stats["files"] = int(files_match.group(1))
            if added_match:
                stats["added"] = int(added_match.group(1))
            if removed_match:
                stats["removed"] = int(removed_match.group(1))

        return stats

    def _commit_changes(self, container: ABContainer, message: str) -> Optional[str]:
        """Commit changes in container and return SHA."""
        # Stage all changes
        self._run_docker([
            "exec", container.container_name,
            "git", "add", "-A"
        ])

        # Commit
        self._run_docker([
            "exec", container.container_name,
            "git", "commit", "-m", message, "--allow-empty"
        ])

        # Get SHA
        result = self._run_docker([
            "exec", container.container_name,
            "git", "rev-parse", "HEAD"
        ])

        return result.stdout.strip() if result.returncode == 0 else None

    def stop_container(self, mode: str):
        """Stop and remove container."""
        container = self.containers.get(mode)
        if container:
            self._run_docker(["stop", container.container_name])
            self._run_docker(["rm", container.container_name])
            container.status = "stopped"
            container.end_time = datetime.now().isoformat()

    def cleanup_all(self):
        """Stop and remove all containers."""
        for mode in list(self.containers.keys()):
            self.stop_container(mode)
        self.memgraph.close()


# ═══════════════════════════════════════════════════════════════════════════════
# Orchestrator
# ═══════════════════════════════════════════════════════════════════════════════

class ABOrchestrator:
    """Main orchestrator for parallel A/B testing."""

    def __init__(self, project_path: str):
        self.project_path = os.path.abspath(project_path)
        self.session_id = str(uuid.uuid4())[:12]
        self.session: Optional[ABSession] = None
        self.container_manager: Optional[ContainerManager] = None
        self.memgraph = MemgraphSync()
        self.results: Dict[str, ABResult] = {}

    def start_session(self, task_description: str) -> ABSession:
        """Initialize a new A/B testing session."""
        print(f"\n{'='*60}")
        print(f"A/B TESTING SESSION: {self.session_id}")
        print(f"{'='*60}")
        print(f"Project: {self.project_path}")
        print(f"Task: {task_description}")
        print(f"Modes: {', '.join(MODES)}")
        print()

        # Get base branch
        result = subprocess.run(
            ["git", "-C", self.project_path, "rev-parse", "--abbrev-ref", "HEAD"],
            capture_output=True, text=True
        )
        base_branch = result.stdout.strip() if result.returncode == 0 else "main"

        self.session = ABSession(
            session_id=self.session_id,
            project_path=self.project_path,
            task_description=task_description,
            start_time=datetime.now().isoformat(),
            base_branch=base_branch,
            status="running"
        )

        # Record in Memgraph
        self.memgraph.create_session(self.session)

        # Initialize container manager
        self.container_manager = ContainerManager(self.project_path, self.session_id)

        return self.session

    def spawn_all_containers(self) -> Dict[str, ABContainer]:
        """Spawn containers for all modes in parallel."""
        print("Spawning containers...")

        containers = {}
        with ThreadPoolExecutor(max_workers=4) as executor:
            futures = {
                executor.submit(self.container_manager.spawn_container, mode): mode
                for mode in MODES
            }

            for future in as_completed(futures):
                mode = futures[future]
                try:
                    container = future.result()
                    containers[mode] = container
                except Exception as e:
                    print(f"  ✗ Error spawning {mode}: {e}")

        print(f"\nSpawned {len(containers)} containers")
        return containers

    def run_task_parallel(self, task_script: str) -> Dict[str, ABResult]:
        """Run task in all containers in parallel."""
        print(f"\nExecuting task in parallel...")

        results = {}
        with ThreadPoolExecutor(max_workers=4) as executor:
            futures = {
                executor.submit(self.container_manager.execute_task, mode, task_script): mode
                for mode in MODES
            }

            for future in as_completed(futures):
                mode = futures[future]
                try:
                    result = future.result()
                    if result:
                        results[mode] = result
                        print(f"  ✓ {mode}: {result.execution_time_ms}ms, "
                              f"{result.tokens_saved} tokens saved, "
                              f"{result.errors} errors")
                except Exception as e:
                    print(f"  ✗ {mode} failed: {e}")

        self.results = results
        return results

    def analyze_results(self) -> Dict:
        """Analyze and compare results across modes."""
        if not self.results:
            return {"error": "No results to analyze"}

        analysis = {
            "session_id": self.session_id,
            "modes_tested": list(self.results.keys()),
            "comparison": {}
        }

        # Compare each metric
        for mode, result in self.results.items():
            analysis["comparison"][mode] = {
                "tokens_saved": result.tokens_saved,
                "execution_time_ms": result.execution_time_ms,
                "errors": result.errors,
                "files_changed": result.files_changed,
                "efficiency_score": self._calculate_efficiency(result)
            }

        # Find best mode
        best_mode = max(
            analysis["comparison"].keys(),
            key=lambda m: analysis["comparison"][m]["efficiency_score"]
        )
        analysis["best_mode"] = best_mode
        analysis["recommendation"] = f"Mode '{best_mode}' performed best in this session"

        return analysis

    def _calculate_efficiency(self, result: ABResult) -> float:
        """Calculate efficiency score for a result."""
        # Weighted score: tokens_saved (40%) - errors (30%) - time (30%)
        tokens_score = min(result.tokens_saved / 1000, 10)  # Cap at 10
        error_penalty = result.errors * 2
        time_penalty = result.execution_time_ms / 60000  # Minutes

        return tokens_score - error_penalty - time_penalty

    def end_session(self) -> Dict:
        """End session and cleanup."""
        print(f"\n{'='*60}")
        print("SESSION COMPLETE")
        print(f"{'='*60}")

        # Analyze results
        analysis = self.analyze_results()

        # Print comparison
        print("\nResults by mode:")
        print("-" * 40)
        for mode, stats in analysis.get("comparison", {}).items():
            print(f"  {mode.upper()}")
            print(f"    Tokens saved: {stats['tokens_saved']}")
            print(f"    Time: {stats['execution_time_ms']}ms")
            print(f"    Errors: {stats['errors']}")
            print(f"    Efficiency: {stats['efficiency_score']:.2f}")
            print()

        print(f"Best mode: {analysis.get('best_mode', 'N/A')}")
        print(f"{analysis.get('recommendation', '')}")

        # Update session status
        self.memgraph.update_session_status(
            self.session_id,
            "completed",
            datetime.now().isoformat()
        )

        # Cleanup containers
        if self.container_manager:
            self.container_manager.cleanup_all()

        self.memgraph.close()

        # Save session results to file
        self._save_session_results(analysis)

        return analysis

    def _save_session_results(self, analysis: Dict):
        """Save session results to JSON file."""
        os.makedirs(AB_SESSIONS_DIR, exist_ok=True)

        results_file = os.path.join(
            AB_SESSIONS_DIR,
            f"ab_session_{self.session_id}.json"
        )

        with open(results_file, 'w') as f:
            json.dump({
                "session": asdict(self.session) if self.session else {},
                "results": {m: asdict(r) for m, r in self.results.items()},
                "analysis": analysis
            }, f, indent=2)

        print(f"\nResults saved to: {results_file}")


# ═══════════════════════════════════════════════════════════════════════════════
# CLI Commands
# ═══════════════════════════════════════════════════════════════════════════════

def cmd_start(project_path: str, task: str, script: str = None):
    """Start a new A/B testing session."""
    orchestrator = ABOrchestrator(project_path)

    # Start session
    orchestrator.start_session(task)

    # Spawn containers
    orchestrator.spawn_all_containers()

    # Run task if script provided
    if script:
        orchestrator.run_task_parallel(script)

    # End session
    return orchestrator.end_session()


def cmd_stats():
    """Show aggregated statistics from Memgraph."""
    memgraph = MemgraphSync()
    stats = memgraph.get_mode_statistics()
    memgraph.close()

    if not stats:
        print("No A/B testing data available yet.")
        return

    print("\n" + "=" * 60)
    print("A/B TESTING STATISTICS (from Memgraph)")
    print("=" * 60)

    for mode, data in stats.items():
        print(f"\nMODE: {mode.upper()}")
        print(f"  Runs: {data['runs']}")
        print(f"  Avg tokens used: {data['avg_tokens_used']:.0f}")
        print(f"  Avg tokens saved: {data['avg_tokens_saved']:.0f}")
        print(f"  Avg time: {data['avg_time_ms']:.0f}ms")
        print(f"  Total errors: {data['total_errors']}")
        print(f"  Avg files changed: {data['avg_files_changed']:.1f}")


def cmd_recommend():
    """Get recommendation for best mode."""
    memgraph = MemgraphSync()
    rec = memgraph.get_best_mode()
    memgraph.close()

    print("\n" + "=" * 60)
    print("MODE RECOMMENDATION")
    print("=" * 60)
    print(f"\nRecommended: {rec['recommended'].upper()}")
    print(f"Reason: {rec['reason']}")

    if 'score' in rec:
        print(f"\nDetails:")
        print(f"  Runs analyzed: {rec.get('runs', 'N/A')}")
        print(f"  Efficiency: {rec.get('efficiency', 'N/A')}")
        print(f"  Error rate: {rec.get('error_rate', 'N/A')}")
        print(f"  Speed: {rec.get('speed', 'N/A')}ms")
        print(f"  Composite score: {rec.get('score', 'N/A')}")


def cmd_list_sessions():
    """List recent A/B testing sessions."""
    if not os.path.exists(AB_SESSIONS_DIR):
        print("No A/B sessions found.")
        return

    sessions = sorted(Path(AB_SESSIONS_DIR).glob("ab_session_*.json"), reverse=True)[:10]

    print("\n" + "=" * 60)
    print("RECENT A/B SESSIONS")
    print("=" * 60)

    for session_file in sessions:
        with open(session_file) as f:
            data = json.load(f)

        session = data.get("session", {})
        analysis = data.get("analysis", {})

        print(f"\n{session.get('session_id', 'unknown')}")
        print(f"  Task: {session.get('task_description', 'N/A')[:50]}...")
        print(f"  Time: {session.get('start_time', 'N/A')}")
        print(f"  Best mode: {analysis.get('best_mode', 'N/A')}")


def main():
    import argparse

    parser = argparse.ArgumentParser(description="A/B Testing Orchestrator")
    subparsers = parser.add_subparsers(dest="command", required=True)

    # start
    p = subparsers.add_parser("start", help="Start A/B test session")
    p.add_argument("project_path", help="Path to project")
    p.add_argument("--task", "-t", required=True, help="Task description")
    p.add_argument("--script", "-s", help="Script to execute in each container")

    # stats
    subparsers.add_parser("stats", help="Show aggregated statistics")

    # recommend
    subparsers.add_parser("recommend", help="Get mode recommendation")

    # list
    subparsers.add_parser("list", help="List recent sessions")

    args = parser.parse_args()

    if args.command == "start":
        cmd_start(args.project_path, args.task, args.script)
    elif args.command == "stats":
        cmd_stats()
    elif args.command == "recommend":
        cmd_recommend()
    elif args.command == "list":
        cmd_list_sessions()


if __name__ == "__main__":
    main()
