#!/usr/bin/env python3
"""
URP-CLI: Universal Repository Perception for AI Agents.

Skills interface for Claude Code - gives the agent structured perception
of code, git history, and runtime state through PRU primitives.

Usage:
    python cli.py ingest <path>              # Build knowledge graph from code
    python cli.py git <path>                 # Load git history
    python cli.py impact <signature>         # Find what depends on a function
    python cli.py deps <signature>           # Find what a function depends on
    python cli.py history <file>             # Get file change history
    python cli.py hotspots [--days N]        # Find risky/unstable files
    python cli.py dead                       # Find unused code
    python cli.py vitals                     # Check container health
    python cli.py logs <container> [--tail]  # Watch container logs
    python cli.py topology                   # Show container network topology
    python cli.py stats                      # Graph statistics
    python cli.py expert <path_pattern>      # Find who knows this code
    python cli.py reviewers <file>           # Suggest code reviewers
"""
import argparse
import json
import sys
from database import Database


def cmd_ingest(args, db):
    """Ingest codebase into graph (D, ⊆, Φ primitives)."""
    from parser import create_default_registry
    from ingester import Ingester

    registry = create_default_registry()
    ingester = Ingester(db, registry)

    print(f"Ingesting {args.path}...", file=sys.stderr)
    stats = ingester.ingest(args.path)

    # Post-process to link calls
    ingester.link_calls_to_definitions()

    print(json.dumps(stats, indent=2))


def cmd_git(args, db):
    """Load git history (τ primitive)."""
    from git_loader import GitLoader

    loader = GitLoader(db)
    loader.load_repository(args.path)

    print(f"Loading git history from {args.path}...", file=sys.stderr)
    commit_count = loader.ingest_history(max_commits=args.max)
    branch_count = loader.ingest_branches()

    print(json.dumps({
        "commits": commit_count,
        "branches": branch_count,
    }, indent=2))


def cmd_impact(args, db):
    """Find impact of changing a function (Φ inverse)."""
    from querier import Querier
    q = Querier(db)
    results = q.find_impact(args.signature, args.depth)
    print(json.dumps(results, indent=2))


def cmd_deps(args, db):
    """Find dependencies of a function (Φ forward)."""
    from querier import Querier
    q = Querier(db)
    results = q.find_dependencies(args.signature, args.depth)
    print(json.dumps(results, indent=2))


def cmd_history(args, db):
    """Get file history (τ sequence)."""
    from querier import Querier
    q = Querier(db)
    results = q.get_file_history(args.file, args.limit)
    print(json.dumps(results, indent=2))


def cmd_hotspots(args, db):
    """Find risky/unstable areas (τ + Φ combined)."""
    from querier import Querier
    q = Querier(db)
    results = q.analyze_hotspots(args.days)
    print(json.dumps(results, indent=2))


def cmd_dead(args, db):
    """Find dead code (⊥ unused)."""
    from querier import Querier
    q = Querier(db)
    results = q.find_dead_code()
    print(json.dumps(results, indent=2))


def cmd_circular(args, db):
    """Find circular dependencies (⊥ conflict)."""
    from querier import Querier
    q = Querier(db)
    results = q.find_circular_deps()
    print(json.dumps(results, indent=2))


def cmd_vitals(args, db):
    """Check container vitals (Φ energy)."""
    from observer import ContainerObserver
    obs = ContainerObserver(db)

    if obs.get_error():
        print(json.dumps({"error": obs.get_error(), "containers": []}))
        return

    states = obs.snapshot_all()

    output = []
    for s in states:
        output.append({
            "name": s.name,
            "status": s.status,
            "cpu": f"{s.cpu_percent:.1f}%",
            "memory": f"{s.memory_bytes / 1024 / 1024:.0f}MB",
            "mem_pct": f"{s.memory_bytes / max(s.memory_limit, 1) * 100:.1f}%",
        })

    print(json.dumps(output, indent=2))


def cmd_logs(args, db):
    """Watch container logs (τ events)."""
    from observer import ContainerObserver
    obs = ContainerObserver(db)
    events = obs.watch_logs(args.container, tail=args.tail)

    output = []
    for e in events:
        if args.errors and e.level not in ('ERROR', 'FATAL'):
            continue
        output.append({
            "level": e.level,
            "time": e.timestamp.isoformat(),
            "msg": e.message,
        })

    print(json.dumps(output, indent=2))


def cmd_topology(args, db):
    """Show container topology (D + ⊆)."""
    from observer import ContainerObserver
    obs = ContainerObserver(db)
    topo = obs.get_topology()
    print(json.dumps(topo, indent=2))


def cmd_health(args, db):
    """Check for container health issues (⊥ conflicts)."""
    from observer import ContainerObserver
    obs = ContainerObserver(db)
    issues = obs.check_health()
    print(json.dumps(issues, indent=2))


def cmd_stats(args, db):
    """Show graph statistics."""
    from querier import Querier
    q = Querier(db)
    stats = q.get_graph_stats()
    print(json.dumps(stats, indent=2))


def cmd_expert(args, db):
    """Find experts for a code area (τ + D)."""
    from querier import Querier
    q = Querier(db)
    results = q.get_author_expertise(args.pattern)
    print(json.dumps(results, indent=2))


def cmd_reviewers(args, db):
    """Suggest reviewers for a file (τ + D)."""
    from querier import Querier
    q = Querier(db)
    results = q.suggest_reviewers(args.file)
    print(json.dumps(results, indent=2))


def cmd_contents(args, db):
    """Show file contents/entities (⊆)."""
    from querier import Querier
    q = Querier(db)
    results = q.get_file_contents(args.file)
    print(json.dumps(results, indent=2))


def cmd_recent(args, db):
    """Show recently changed files (τ)."""
    from querier import Querier
    q = Querier(db)
    results = q.get_recent_changes(args.days)
    print(json.dumps(results, indent=2))


def main():
    parser = argparse.ArgumentParser(
        description="URP-CLI: Universal Repository Perception",
        formatter_class=argparse.RawDescriptionHelpFormatter,
        epilog=__doc__
    )

    subparsers = parser.add_subparsers(dest="command", required=True)

    # ingest
    p = subparsers.add_parser("ingest", help="Ingest codebase into graph")
    p.add_argument("path", help="Path to codebase")

    # git
    p = subparsers.add_parser("git", help="Load git history")
    p.add_argument("path", help="Path to git repository")
    p.add_argument("--max", type=int, default=500, help="Max commits to load")

    # impact
    p = subparsers.add_parser("impact", help="Find impact of a function")
    p.add_argument("signature", help="Function signature (file::name)")
    p.add_argument("--depth", type=int, default=10, help="Max depth")

    # deps
    p = subparsers.add_parser("deps", help="Find dependencies of a function")
    p.add_argument("signature", help="Function signature")
    p.add_argument("--depth", type=int, default=10, help="Max depth")

    # history
    p = subparsers.add_parser("history", help="Get file change history")
    p.add_argument("file", help="File path")
    p.add_argument("--limit", type=int, default=20, help="Max commits")

    # hotspots
    p = subparsers.add_parser("hotspots", help="Find risky/unstable areas")
    p.add_argument("--days", type=int, default=30, help="Look back N days")

    # dead
    p = subparsers.add_parser("dead", help="Find dead code")

    # circular
    p = subparsers.add_parser("circular", help="Find circular dependencies")

    # vitals
    p = subparsers.add_parser("vitals", help="Check container vitals")

    # logs
    p = subparsers.add_parser("logs", help="Watch container logs")
    p.add_argument("container", help="Container name or ID")
    p.add_argument("--tail", type=int, default=100, help="Lines to fetch")
    p.add_argument("--errors", action="store_true", help="Only show errors")

    # topology
    p = subparsers.add_parser("topology", help="Show container topology")

    # health
    p = subparsers.add_parser("health", help="Check container health")

    # stats
    p = subparsers.add_parser("stats", help="Show graph statistics")

    # expert
    p = subparsers.add_parser("expert", help="Find code experts")
    p.add_argument("pattern", help="Path pattern to search")

    # reviewers
    p = subparsers.add_parser("reviewers", help="Suggest code reviewers")
    p.add_argument("file", help="File path")

    # contents
    p = subparsers.add_parser("contents", help="Show file entities")
    p.add_argument("file", help="File path")

    # recent
    p = subparsers.add_parser("recent", help="Show recently changed files")
    p.add_argument("--days", type=int, default=7, help="Look back N days")

    args = parser.parse_args()

    # Connect to graph database
    db = Database()

    try:
        # Dispatch to command handler
        cmd_map = {
            "ingest": cmd_ingest,
            "git": cmd_git,
            "impact": cmd_impact,
            "deps": cmd_deps,
            "history": cmd_history,
            "hotspots": cmd_hotspots,
            "dead": cmd_dead,
            "circular": cmd_circular,
            "vitals": cmd_vitals,
            "logs": cmd_logs,
            "topology": cmd_topology,
            "health": cmd_health,
            "stats": cmd_stats,
            "expert": cmd_expert,
            "reviewers": cmd_reviewers,
            "contents": cmd_contents,
            "recent": cmd_recent,
        }

        handler = cmd_map.get(args.command)
        if handler:
            handler(args, db)
        else:
            parser.print_help()
    finally:
        db.close()


if __name__ == "__main__":
    main()
