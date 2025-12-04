#!/usr/bin/env python3
"""
Token Monitor - Master-Worker Token Consumption Tracking

Provides:
- Per-worker token tracking (input/output tokens from Claude API)
- Model-aware pricing and cost calculation
- Master aggregation view
- SQLite backend via pricing_db.py

This module delegates to pricing_db.py for storage and formatting.
Maintains backward compatibility with existing CLI commands.
"""
import os
import sys
from datetime import datetime

# Identity
WORKER_ID = os.getenv('WORKER_NUM', os.getenv('HOSTNAME', 'unknown'))
PROJECT_NAME = os.getenv('PROJECT_NAME', 'unknown')
IS_MASTER = os.getenv('URP_MASTER', '0') == '1'

# Import the SQLite-based pricing database
try:
    from pricing_db import (
        track_usage,
        get_worker_stats,
        get_all_workers_stats,
        get_model_stats,
        format_worker_status as _format_worker_status,
        format_master_view as _format_master_view,
        list_models,
        calculate_cost,
        init_db,
    )
    USE_SQLITE = True
except ImportError:
    USE_SQLITE = False


def track_api_call(input_tokens: int, output_tokens: int, model: str = "sonnet", context: str = ""):
    """
    Track a Claude API call's token consumption.
    Called after each API response.
    """
    if USE_SQLITE:
        return track_usage(input_tokens, output_tokens, model, context)
    else:
        # Minimal fallback - just print warning
        print(f"Warning: pricing_db not available, tokens not tracked")
        return {'input_tokens': input_tokens, 'output_tokens': output_tokens}


def get_worker_status(worker_id: str = None):
    """Get status for a specific worker."""
    if USE_SQLITE:
        return get_worker_stats(worker_id)
    return {'worker_id': worker_id or WORKER_ID, 'session': {}, 'current_hour': {}}


def get_aggregate_status():
    """Get aggregated status across all workers (master view)."""
    if USE_SQLITE:
        return get_all_workers_stats()
    return {'workers': {}, 'totals': {'input': 0, 'output': 0, 'cost': 0, 'requests': 0}}


def format_worker_status(compact: bool = False) -> str:
    """Format current worker's status."""
    if USE_SQLITE:
        return _format_worker_status(compact)

    # Minimal fallback
    return f"Worker {WORKER_ID}: (pricing_db not available)"


def format_master_view() -> str:
    """Format aggregated view for master (all workers)."""
    if USE_SQLITE:
        return _format_master_view()

    return "(pricing_db not available)"


def format_compact_all() -> str:
    """One-liner for all workers."""
    if USE_SQLITE:
        stats = get_all_workers_stats()
        t = stats['totals']
        w = len(stats['workers'])
        return f"[{w} workers] Total: {t['input']+t['output']:,} tok | ${t['cost']:.4f} ({t['requests']} reqs)"

    return "(pricing_db not available)"


# CLI
if __name__ == "__main__":
    if USE_SQLITE:
        init_db()

    if len(sys.argv) < 2:
        if IS_MASTER:
            print(format_master_view())
        else:
            print(format_worker_status())
        sys.exit(0)

    cmd = sys.argv[1]

    if cmd == "status":
        print(format_worker_status())

    elif cmd == "compact":
        print(format_worker_status(compact=True))

    elif cmd == "master" or cmd == "all":
        print(format_master_view())

    elif cmd == "compact-all":
        print(format_compact_all())

    elif cmd == "models" or cmd == "pricing":
        if USE_SQLITE:
            print(list_models())
        else:
            print("pricing_db not available")

    elif cmd == "json":
        import json
        if IS_MASTER:
            print(json.dumps(get_aggregate_status(), indent=2))
        else:
            print(json.dumps(get_worker_status(), indent=2))

    elif cmd == "track":
        # track <input> <output> [model] [context]
        if len(sys.argv) >= 4:
            inp = int(sys.argv[2])
            out = int(sys.argv[3])
            model = sys.argv[4] if len(sys.argv) > 4 else 'sonnet'
            ctx = sys.argv[5] if len(sys.argv) > 5 else ''
            result = track_api_call(inp, out, model, ctx)
            cost = result.get('total_cost', 0) if isinstance(result, dict) else 0
            print(f"Tracked: +{inp} in, +{out} out = {inp+out} tokens | ${cost:.6f} ({model})")
        else:
            print("Usage: token_monitor.py track <input> <output> [model] [context]")

    elif cmd == "cost":
        if USE_SQLITE and len(sys.argv) >= 4:
            inp = int(sys.argv[2])
            out = int(sys.argv[3])
            model = sys.argv[4] if len(sys.argv) > 4 else 'sonnet'
            ic, oc, tc = calculate_cost(inp, out, model)
            print(f"Cost for {inp:,} in + {out:,} out ({model}): ${tc:.6f}")
        else:
            print("Usage: token_monitor.py cost <input> <output> [model]")

    else:
        print("Commands:")
        print("  status      - Show current worker status")
        print("  compact     - One-line worker status")
        print("  master/all  - Show all workers (master view)")
        print("  compact-all - One-line all workers")
        print("  models      - List models with pricing")
        print("  json        - JSON output")
        print("  track <in> <out> [model] [ctx] - Track usage")
        print("  cost <in> <out> [model] - Calculate cost")
