"""
Container/Runtime Observer - Propiocepción del sistema.
Φ (Morphism): CPU/RAM as energy flow
D (Domain): Container existence and boundaries
τ (Vector): Log events as temporal sequence
⊥ (Orthogonal): Health status, conflicts
"""
import os
import re
import time
import json
from dataclasses import dataclass
from typing import Iterator, Optional
from datetime import datetime

try:
    import docker
    HAS_DOCKER = True
except ImportError:
    HAS_DOCKER = False


@dataclass
class ContainerState:
    """Current state of a container (Φ energy levels)."""
    id: str
    name: str
    status: str
    cpu_percent: float
    memory_bytes: int
    memory_limit: int
    network_rx: int
    network_tx: int


@dataclass
class LogEvent:
    """A parsed log event (τ temporal node)."""
    container_id: str
    timestamp: datetime
    level: str  # INFO, WARN, ERROR, FATAL
    message: str
    source: str


class ContainerObserver:
    """Observes Docker containers and feeds state into the graph."""

    # Patterns to detect log severity
    LOG_PATTERNS = [
        (r'\b(FATAL|PANIC|CRITICAL)\b', 'FATAL'),
        (r'\b(ERROR|ERR|EXCEPTION|FAIL)\b', 'ERROR'),
        (r'\b(WARN|WARNING)\b', 'WARN'),
        (r'\b(INFO|DEBUG|TRACE)\b', 'INFO'),
    ]

    def __init__(self, db):
        self._db = db
        self._client = None
        self._docker_error = None

        if not HAS_DOCKER:
            self._docker_error = "Docker SDK not installed. Run: pip install docker"
            return

        try:
            self._client = docker.from_env()
            # Test connection
            self._client.ping()
        except Exception as e:
            self._docker_error = f"Docker not accessible: {e}"
            self._client = None

    def snapshot_all(self) -> list[ContainerState]:
        """
        Take a snapshot of all running containers.
        Returns list of container states.
        """
        if not self._client:
            return []

        states = []
        for container in self._client.containers.list():
            state = self._get_container_state(container)
            if state:
                states.append(state)
                self._store_state(state)
        return states

    def get_error(self) -> Optional[str]:
        """Return Docker connection error if any."""
        return self._docker_error

    def _get_container_state(self, container) -> Optional[ContainerState]:
        """Extract energy metrics from a container."""
        try:
            stats = container.stats(stream=False)
            cpu = self._calculate_cpu_percent(stats)
            mem = stats['memory_stats'].get('usage', 0)
            mem_limit = stats['memory_stats'].get('limit', 1)

            networks = stats.get('networks', {})
            rx = sum(n.get('rx_bytes', 0) for n in networks.values())
            tx = sum(n.get('tx_bytes', 0) for n in networks.values())

            return ContainerState(
                id=container.short_id,
                name=container.name,
                status=container.status,
                cpu_percent=cpu,
                memory_bytes=mem,
                memory_limit=mem_limit,
                network_rx=rx,
                network_tx=tx,
            )
        except Exception as e:
            print(f"Warning: Could not get stats for {container.name}: {e}")
            return None

    def _calculate_cpu_percent(self, stats: dict) -> float:
        """Calculate CPU percentage from Docker stats."""
        try:
            cpu_delta = stats['cpu_stats']['cpu_usage']['total_usage'] - \
                        stats['precpu_stats']['cpu_usage']['total_usage']
            system_delta = stats['cpu_stats']['system_cpu_usage'] - \
                           stats['precpu_stats']['system_cpu_usage']
            num_cpus = len(stats['cpu_stats']['cpu_usage'].get('percpu_usage', [1]))
            if system_delta > 0:
                return (cpu_delta / system_delta) * num_cpus * 100.0
        except (KeyError, ZeroDivisionError):
            pass
        return 0.0

    def _store_state(self, state: ContainerState):
        """Store container state in graph (D + Φ)."""
        self._db.execute_query("""
            MERGE (c:Container {id: $id})
            SET c.name = $name,
                c.status = $status,
                c.cpu_phi = $cpu,
                c.mem_bytes = $mem,
                c.mem_limit = $mem_limit,
                c.mem_percent = $mem_pct,
                c.net_rx = $rx,
                c.net_tx = $tx,
                c.last_seen = $ts
        """, {
            "id": state.id,
            "name": state.name,
            "status": state.status,
            "cpu": round(state.cpu_percent, 2),
            "mem": state.memory_bytes,
            "mem_limit": state.memory_limit,
            "mem_pct": round(state.memory_bytes / max(state.memory_limit, 1) * 100, 2),
            "rx": state.network_rx,
            "tx": state.network_tx,
            "ts": int(time.time()),
        })

    def watch_logs(self, container_name: str, tail: int = 100) -> list[LogEvent]:
        """
        Capture recent logs and extract events (τ temporal).
        """
        if not self._client:
            return []

        try:
            container = self._client.containers.get(container_name)
            logs = container.logs(tail=tail, timestamps=True).decode('utf-8', errors='replace')
            events = list(self._parse_logs(container.short_id, logs))

            # Store error events in graph
            for event in events:
                if event.level in ('ERROR', 'FATAL'):
                    self._store_log_event(event)

            return events
        except docker.errors.NotFound:
            return []

    def _parse_logs(self, container_id: str, logs: str) -> Iterator[LogEvent]:
        """Parse log lines into structured events."""
        for line in logs.strip().split('\n'):
            if not line:
                continue

            # Try to extract timestamp (Docker format: 2024-01-01T00:00:00.000000000Z)
            ts_match = re.match(r'^(\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2})', line)
            if ts_match:
                try:
                    ts = datetime.fromisoformat(ts_match.group(1))
                except ValueError:
                    ts = datetime.now()
                message = line[ts_match.end():].strip()
            else:
                ts = datetime.now()
                message = line

            # Detect severity
            level = 'INFO'
            for pattern, lvl in self.LOG_PATTERNS:
                if re.search(pattern, message, re.IGNORECASE):
                    level = lvl
                    break

            yield LogEvent(
                container_id=container_id,
                timestamp=ts,
                level=level,
                message=message[:500],  # Truncate
                source='docker',
            )

    def _store_log_event(self, event: LogEvent):
        """Store significant log events in graph."""
        self._db.execute_query("""
            MATCH (c:Container {id: $cid})
            CREATE (e:LogEvent {
                level: $level,
                message: $msg,
                timestamp: $ts
            })
            CREATE (c)-[:EMITTED]->(e)
        """, {
            "cid": event.container_id,
            "level": event.level,
            "msg": event.message,
            "ts": event.timestamp.isoformat(),
        })

    def get_topology(self) -> dict:
        """
        Get container network topology (⊆ inclusion).
        Returns dict of containers and their network connections.
        """
        topology = {"containers": [], "networks": [], "error": self._docker_error}

        if not self._client:
            return topology

        for container in self._client.containers.list():
            info = container.attrs
            nets = info.get('NetworkSettings', {}).get('Networks', {})

            c_info = {
                "id": container.short_id,
                "name": container.name,
                "image": info.get('Config', {}).get('Image', ''),
                "ports": list(info.get('NetworkSettings', {}).get('Ports', {}).keys()),
                "networks": list(nets.keys()),
                "mounts": [m['Destination'] for m in info.get('Mounts', [])],
            }
            topology["containers"].append(c_info)

            for net_name, net_info in nets.items():
                if net_name not in topology["networks"]:
                    topology["networks"].append(net_name)

                # Store topology in graph
                self._db.execute_query("""
                    MERGE (c:Container {id: $cid})
                    MERGE (n:Network {name: $net})
                    MERGE (c)-[:CONNECTED_TO {ip: $ip}]->(n)
                """, {
                    "cid": container.short_id,
                    "net": net_name,
                    "ip": net_info.get('IPAddress', ''),
                })

        return topology

    def check_health(self) -> list[dict]:
        """
        Check container health status (⊥ orthogonal conflicts).
        Returns list of health issues.
        """
        issues = []

        if not self._client:
            if self._docker_error:
                issues.append({"container": "docker", "issue": "unavailable", "detail": self._docker_error})
            return issues

        for container in self._client.containers.list(all=True):
            state = container.attrs.get('State', {})
            health = state.get('Health', {})

            # Restart loops
            restart_count = state.get('RestartCount', 0)
            if restart_count > 3:
                issues.append({
                    "container": container.name,
                    "type": "RESTART_LOOP",
                    "severity": "ERROR",
                    "detail": f"Restarted {restart_count} times",
                })

            # Health check failures
            if health.get('Status') == 'unhealthy':
                issues.append({
                    "container": container.name,
                    "type": "UNHEALTHY",
                    "severity": "ERROR",
                    "detail": health.get('Log', [{}])[-1].get('Output', '')[:200],
                })

            # OOMKilled
            if state.get('OOMKilled'):
                issues.append({
                    "container": container.name,
                    "type": "OOM_KILLED",
                    "severity": "FATAL",
                    "detail": "Container killed by OOM",
                })

            # Exited with error
            if state.get('Status') == 'exited' and state.get('ExitCode', 0) != 0:
                issues.append({
                    "container": container.name,
                    "type": "EXIT_ERROR",
                    "severity": "ERROR",
                    "detail": f"Exit code: {state.get('ExitCode')}",
                })

        return issues


class ProcessObserver:
    """Observes host processes when not using Docker."""

    def __init__(self, db):
        self._db = db

    def snapshot_processes(self, filter_cmd: Optional[str] = None) -> list[dict]:
        """Get process info from /proc (Linux) or ps."""
        import subprocess

        try:
            result = subprocess.run(
                ['ps', 'aux', '--no-headers'],
                capture_output=True,
                text=True,
                timeout=5,
            )

            processes = []
            for line in result.stdout.strip().split('\n'):
                parts = line.split(None, 10)
                if len(parts) >= 11:
                    cmd = parts[10]
                    if filter_cmd and filter_cmd not in cmd:
                        continue
                    processes.append({
                        "user": parts[0],
                        "pid": parts[1],
                        "cpu": float(parts[2]),
                        "mem": float(parts[3]),
                        "command": cmd[:200],
                    })

            return processes
        except Exception as e:
            return [{"error": str(e)}]
