#!/usr/bin/env python3
"""
local_stats.py - Local Token Usage Statistics

Reads from SQLite database populated by urp-proxy.
Shows usage for THIS container only.
"""

import os
import sys
import sqlite3
from datetime import datetime, timedelta

# Configuration
DB_PATH = os.getenv("STATS_DB", "/app/sessions/proxy_stats.db")

# Colors
RED = '\033[0;31m'
GREEN = '\033[0;32m'
YELLOW = '\033[1;33m'
CYAN = '\033[0;36m'
MAGENTA = '\033[0;35m'
NC = '\033[0m'


def format_tokens(n: int) -> str:
    """Format token count."""
    if n >= 1_000_000:
        return f"{n/1_000_000:.2f}M"
    elif n >= 1_000:
        return f"{n/1_000:.1f}K"
    return str(n)


def get_db():
    """Get database connection."""
    if not os.path.exists(DB_PATH):
        print(f"{RED}No stats yet (proxy not used){NC}")
        return None
    return sqlite3.connect(DB_PATH)


def print_status():
    """Print usage statistics."""
    conn = get_db()
    if not conn:
        return

    cursor = conn.cursor()

    print(f"{MAGENTA}═══════════════════════════════════════════════════════════{NC}")
    print(f"{MAGENTA}         LOCAL TOKEN USAGE (this container){NC}")
    print(f"{MAGENTA}═══════════════════════════════════════════════════════════{NC}")
    print()

    # Total stats
    cursor.execute("""
        SELECT COUNT(*),
               COALESCE(SUM(input_tokens), 0),
               COALESCE(SUM(output_tokens), 0),
               COALESCE(SUM(cached_tokens), 0)
        FROM requests
    """)
    total_req, total_in, total_out, total_cached = cursor.fetchone()

    print(f"  {GREEN}Total:{NC}")
    print(f"    Requests: {total_req:,}")
    print(f"    Input:    {format_tokens(total_in)}")
    print(f"    Output:   {format_tokens(total_out)}")
    if total_cached > 0:
        print(f"    Cached:   {format_tokens(total_cached)}")
    print(f"    {YELLOW}Combined: {format_tokens(total_in + total_out)}{NC}")
    print()

    # By model
    cursor.execute("""
        SELECT model,
               COUNT(*) as reqs,
               SUM(input_tokens) as inp,
               SUM(output_tokens) as out,
               SUM(cached_tokens) as cached
        FROM requests
        GROUP BY model
        ORDER BY (inp + out) DESC
    """)
    rows = cursor.fetchall()

    if rows:
        print(f"  {GREEN}By Model:{NC}")
        for model, reqs, inp, out, cached in rows:
            print(f"    {CYAN}{model}{NC}")
            print(f"      {reqs} reqs | in: {format_tokens(inp)} | out: {format_tokens(out)}", end="")
            if cached:
                print(f" | cached: {format_tokens(cached)}")
            else:
                print()
        print()

    # Last hour
    hour_ago = (datetime.utcnow() - timedelta(hours=1)).strftime('%Y-%m-%dT%H:%M:%S')
    cursor.execute("""
        SELECT COUNT(*),
               COALESCE(SUM(input_tokens + output_tokens), 0)
        FROM requests
        WHERE timestamp > ?
    """, (hour_ago,))
    hour_req, hour_tokens = cursor.fetchone()

    print(f"  {GREEN}Last Hour:{NC}")
    print(f"    Requests: {hour_req:,}")
    print(f"    Tokens:   {format_tokens(hour_tokens)}")
    print()

    # Avg response time
    cursor.execute("SELECT AVG(duration_ms) FROM requests WHERE status = 200")
    avg_duration = cursor.fetchone()[0]
    if avg_duration:
        print(f"  {GREEN}Performance:{NC}")
        print(f"    Avg response: {avg_duration:.0f}ms")
        print()

    conn.close()


def print_compact():
    """One-line summary."""
    conn = get_db()
    if not conn:
        return

    cursor = conn.cursor()
    cursor.execute("""
        SELECT COUNT(*),
               COALESCE(SUM(input_tokens + output_tokens), 0)
        FROM requests
    """)
    total_req, total_tokens = cursor.fetchone()
    conn.close()

    print(f"Local: {total_req} req | {format_tokens(total_tokens)} tokens")


def print_recent(n: int = 10):
    """Print recent requests."""
    conn = get_db()
    if not conn:
        return

    cursor = conn.cursor()
    cursor.execute("""
        SELECT timestamp, model, input_tokens, output_tokens, duration_ms, status
        FROM requests
        ORDER BY timestamp DESC
        LIMIT ?
    """, (n,))
    rows = cursor.fetchall()

    print(f"{MAGENTA}═══════════════════════════════════════════════════════════{NC}")
    print(f"{MAGENTA}                RECENT REQUESTS{NC}")
    print(f"{MAGENTA}═══════════════════════════════════════════════════════════{NC}")
    print()

    for ts, model, inp, out, dur, status in rows:
        ts_short = ts[11:19] if len(ts) > 19 else ts
        status_color = GREEN if status == 200 else RED
        print(f"  {ts_short}  {CYAN}{model[:30]:<30}{NC}  in:{format_tokens(inp):>6} out:{format_tokens(out):>6}  {dur:>5}ms  {status_color}{status}{NC}")

    conn.close()


def print_json():
    """Print raw JSON."""
    import json
    conn = get_db()
    if not conn:
        print("{}")
        return

    cursor = conn.cursor()
    cursor.execute("""
        SELECT timestamp, model, endpoint, input_tokens, output_tokens,
               cached_tokens, duration_ms, status, container
        FROM requests
        ORDER BY timestamp DESC
        LIMIT 100
    """)
    rows = cursor.fetchall()
    conn.close()

    data = [{
        "timestamp": r[0],
        "model": r[1],
        "endpoint": r[2],
        "input_tokens": r[3],
        "output_tokens": r[4],
        "cached_tokens": r[5],
        "duration_ms": r[6],
        "status": r[7],
        "container": r[8]
    } for r in rows]

    print(json.dumps(data, indent=2))


def main():
    cmd = sys.argv[1] if len(sys.argv) > 1 else "status"

    if cmd == "status":
        print_status()
    elif cmd == "compact":
        print_compact()
    elif cmd == "recent":
        n = int(sys.argv[2]) if len(sys.argv) > 2 else 10
        print_recent(n)
    elif cmd == "json":
        print_json()
    elif cmd == "help":
        print("Usage: local_stats.py [command]")
        print()
        print("Commands:")
        print("  status  - Show usage statistics (default)")
        print("  compact - One-line summary")
        print("  recent  - Recent requests")
        print("  json    - Raw JSON output")
        print()
        print(f"Database: {DB_PATH}")
    else:
        print(f"Unknown command: {cmd}")
        sys.exit(1)


if __name__ == "__main__":
    main()
