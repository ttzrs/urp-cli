# ═══════════════════════════════════════════════════════════════════════════════
# Dreamer: Background Maintenance Process ("REM Sleep")
# ═══════════════════════════════════════════════════════════════════════════════
#
# Runs when system is idle. Performs:
# - Re-ingestion of modified files
# - Orphan node cleanup
# - Batch embedding generation
# - Old event pruning
#
# "The unexamined graph is not worth querying." - Socrates, probably

import os
import sys
import time
import logging
from datetime import datetime, timedelta
from pathlib import Path

# Configure logging
logging.basicConfig(
    level=logging.INFO,
    format='%(asctime)s [DREAMER] %(message)s',
    datefmt='%H:%M:%S'
)
log = logging.getLogger(__name__)


# ═══════════════════════════════════════════════════════════════════════════════
# Configuration
# ═══════════════════════════════════════════════════════════════════════════════

IDLE_THRESHOLD_CPU = float(os.getenv('DREAMER_CPU_THRESHOLD', '15'))  # Max CPU% to activate
CHECK_INTERVAL_SEC = int(os.getenv('DREAMER_INTERVAL', '60'))  # Seconds between checks
PRUNE_EVENTS_DAYS = int(os.getenv('DREAMER_PRUNE_DAYS', '7'))  # Delete events older than N days
EMBEDDING_BATCH_SIZE = int(os.getenv('DREAMER_BATCH_SIZE', '10'))  # Embeddings per cycle
CODEBASE_PATH = os.getenv('CODEBASE_PATH', '/codebase')


# ═══════════════════════════════════════════════════════════════════════════════
# System Monitoring
# ═══════════════════════════════════════════════════════════════════════════════

def get_cpu_usage() -> float:
    """Get current CPU usage percentage."""
    try:
        import psutil
        return psutil.cpu_percent(interval=1)
    except ImportError:
        # Fallback: read from /proc/stat
        try:
            with open('/proc/stat', 'r') as f:
                line = f.readline()
                parts = line.split()
                idle = int(parts[4])
                total = sum(int(p) for p in parts[1:])
                return 100 * (1 - idle / total) if total > 0 else 50
        except Exception:
            return 50  # Assume moderate load if we can't measure


def is_system_idle() -> bool:
    """Check if system is idle enough for maintenance."""
    cpu = get_cpu_usage()
    idle = cpu < IDLE_THRESHOLD_CPU
    if idle:
        log.debug(f"System idle (CPU: {cpu:.1f}%)")
    return idle


# ═══════════════════════════════════════════════════════════════════════════════
# Database Connection
# ═══════════════════════════════════════════════════════════════════════════════

_db = None


def get_db():
    """Lazy database connection with retry."""
    global _db
    if _db is None:
        try:
            from database import Database
            _db = Database()
            log.info("Connected to graph database")
        except Exception as e:
            log.warning(f"Database not available: {e}")
            return None
    return _db


# ═══════════════════════════════════════════════════════════════════════════════
# Dream Tasks
# ═══════════════════════════════════════════════════════════════════════════════

def dream_reingest_modified():
    """
    Re-ingest files modified since last scan.
    Keeps the graph fresh without manual intervention.
    """
    db = get_db()
    if not db:
        return 0

    if not os.path.exists(CODEBASE_PATH):
        return 0

    try:
        # Get last ingest timestamp
        result = db.execute_query("""
            MATCH (f:File)
            RETURN max(f.ingested_at) as last_ingest
        """)
        last_ingest = 0
        for r in result:
            if r.get('last_ingest'):
                last_ingest = r['last_ingest']

        # Find modified files
        modified = []
        for root, dirs, files in os.walk(CODEBASE_PATH):
            # Skip common ignore patterns
            dirs[:] = [d for d in dirs if d not in {'.git', 'node_modules', '__pycache__', 'venv', '.venv'}]

            for f in files:
                if f.endswith(('.py', '.go', '.js', '.ts')):
                    path = os.path.join(root, f)
                    mtime = os.path.getmtime(path)
                    if mtime > last_ingest:
                        modified.append(path)

        if not modified:
            return 0

        # Re-ingest modified files
        log.info(f"Re-ingesting {len(modified)} modified files")
        from ingester import Ingester
        ingester = Ingester(db)

        for path in modified[:10]:  # Limit per cycle
            try:
                ingester._ingest_file(path)
            except Exception as e:
                log.warning(f"Failed to ingest {path}: {e}")

        return len(modified)

    except Exception as e:
        log.error(f"Re-ingest failed: {e}")
        return 0


def dream_cleanup_orphans():
    """
    Remove orphan nodes (no relationships).
    Keeps the graph clean and queries fast.
    """
    db = get_db()
    if not db:
        return 0

    try:
        # Find and delete orphan Reference nodes (unresolved calls)
        result = db.execute_query("""
            MATCH (r:Reference)
            WHERE NOT (r)-[]-()
            WITH r LIMIT 100
            DELETE r
            RETURN count(*) as deleted
        """)

        deleted = 0
        for r in result:
            deleted = r.get('deleted', 0)

        if deleted > 0:
            log.info(f"Cleaned up {deleted} orphan nodes")

        return deleted

    except Exception as e:
        log.error(f"Orphan cleanup failed: {e}")
        return 0


def dream_generate_embeddings():
    """
    Generate embeddings for entities that don't have them.
    Enables semantic search in wisdom/novelty.
    """
    db = get_db()
    if not db:
        return 0

    try:
        from brain_cortex import get_embedding
        import json

        # Find functions without embeddings
        result = db.execute_query("""
            MATCH (f:Function)
            WHERE f.embedding IS NULL AND f.signature IS NOT NULL
            RETURN id(f) as node_id, f.signature as sig, f.name as name
            LIMIT $limit
        """, {"limit": EMBEDDING_BATCH_SIZE})

        count = 0
        for r in result:
            sig = r.get('sig') or r.get('name')
            if not sig:
                continue

            vec = get_embedding(sig)
            if vec:
                db.execute_query("""
                    MATCH (f:Function) WHERE id(f) = $nid
                    SET f.embedding = $vec
                """, {"nid": r['node_id'], "vec": vec})
                count += 1

        if count > 0:
            log.info(f"Generated {count} embeddings")

        return count

    except ImportError:
        log.debug("Embedding model not available")
        return 0
    except Exception as e:
        log.error(f"Embedding generation failed: {e}")
        return 0


def dream_prune_old_events():
    """
    Delete old terminal events.
    Prevents unbounded graph growth.
    """
    db = get_db()
    if not db:
        return 0

    try:
        cutoff = int(time.time()) - (PRUNE_EVENTS_DAYS * 24 * 60 * 60)

        # Keep Solutions and their related events
        result = db.execute_query("""
            MATCH (e:TerminalEvent)
            WHERE e.timestamp < $cutoff
              AND NOT (e)-[:CONTRIBUTED_TO]->(:Solution)
            WITH e LIMIT 100
            DELETE e
            RETURN count(*) as deleted
        """, {"cutoff": cutoff})

        deleted = 0
        for r in result:
            deleted = r.get('deleted', 0)

        if deleted > 0:
            log.info(f"Pruned {deleted} old events (older than {PRUNE_EVENTS_DAYS} days)")

        return deleted

    except Exception as e:
        log.error(f"Event pruning failed: {e}")
        return 0


def dream_consolidate_patterns():
    """
    Find frequently successful command patterns and suggest consolidation.
    """
    db = get_db()
    if not db:
        return 0

    try:
        # Find command sequences that appear often before success
        result = db.execute_query("""
            MATCH (e1:TerminalEvent)-[:CONTRIBUTED_TO]->(s:Solution)
            WITH e1.cmd_base as cmd, count(*) as freq
            WHERE freq > 3
            RETURN cmd, freq
            ORDER BY freq DESC
            LIMIT 5
        """)

        patterns = list(result)
        if patterns:
            log.info(f"Top success patterns: {[(p['cmd'], p['freq']) for p in patterns]}")

        return len(patterns)

    except Exception as e:
        log.error(f"Pattern analysis failed: {e}")
        return 0


# ═══════════════════════════════════════════════════════════════════════════════
# Main Loop
# ═══════════════════════════════════════════════════════════════════════════════

def dream_cycle():
    """Run one dream cycle (all maintenance tasks)."""
    tasks = [
        ("reingest", dream_reingest_modified),
        ("orphans", dream_cleanup_orphans),
        ("embeddings", dream_generate_embeddings),
        ("prune", dream_prune_old_events),
        ("patterns", dream_consolidate_patterns),
    ]

    total_work = 0
    for name, task in tasks:
        try:
            work = task()
            total_work += work
        except Exception as e:
            log.error(f"Task {name} crashed: {e}")

    return total_work


def run_dreamer():
    """Main dreamer loop."""
    log.info("Dreamer awakening...")
    log.info(f"Config: CPU threshold={IDLE_THRESHOLD_CPU}%, interval={CHECK_INTERVAL_SEC}s")

    consecutive_idle = 0
    IDLE_REQUIRED = 3  # Require N consecutive idle checks before dreaming

    while True:
        try:
            if is_system_idle():
                consecutive_idle += 1

                if consecutive_idle >= IDLE_REQUIRED:
                    log.info("Entering REM sleep...")
                    work = dream_cycle()

                    if work > 0:
                        log.info(f"Dream cycle completed: {work} items processed")
                    else:
                        log.debug("Dream cycle: nothing to do")

                    consecutive_idle = 0  # Reset after work
            else:
                consecutive_idle = 0
                log.debug("System busy, staying awake")

            time.sleep(CHECK_INTERVAL_SEC)

        except KeyboardInterrupt:
            log.info("Dreamer shutting down...")
            break
        except Exception as e:
            log.error(f"Dreamer error: {e}")
            time.sleep(CHECK_INTERVAL_SEC)


# ═══════════════════════════════════════════════════════════════════════════════
# CLI
# ═══════════════════════════════════════════════════════════════════════════════

if __name__ == "__main__":
    import argparse

    parser = argparse.ArgumentParser(description="Background maintenance daemon")
    parser.add_argument("--once", action="store_true", help="Run one cycle and exit")
    parser.add_argument("--task", choices=["reingest", "orphans", "embeddings", "prune", "patterns"],
                        help="Run specific task")
    args = parser.parse_args()

    if args.task:
        tasks = {
            "reingest": dream_reingest_modified,
            "orphans": dream_cleanup_orphans,
            "embeddings": dream_generate_embeddings,
            "prune": dream_prune_old_events,
            "patterns": dream_consolidate_patterns,
        }
        result = tasks[args.task]()
        print(f"Task {args.task}: {result} items")
    elif args.once:
        log.info("Running single dream cycle...")
        work = dream_cycle()
        print(f"Dream cycle completed: {work} items processed")
    else:
        run_dreamer()
