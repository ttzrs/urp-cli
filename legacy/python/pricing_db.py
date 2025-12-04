#!/usr/bin/env python3
"""
Pricing Database - SQLite storage for model pricing and token usage.

Provides:
- Model pricing table (preloaded with Claude API prices)
- Token usage tracking with model and cost calculation
- Shared across all containers via volume mount
"""
import os
import sqlite3
from datetime import datetime
from pathlib import Path
from typing import Optional, Dict, List, Tuple
from contextlib import contextmanager

# Shared database path (mounted volume)
DB_PATH = Path(os.getenv('URP_DB_PATH', '/shared/sessions/urp.db'))
DB_PATH.parent.mkdir(parents=True, exist_ok=True)

# Identity
WORKER_ID = os.getenv('WORKER_NUM', os.getenv('HOSTNAME', 'unknown'))
PROJECT_NAME = os.getenv('PROJECT_NAME', 'unknown')
IS_MASTER = os.getenv('URP_MASTER', '0') == '1'

# Claude API pricing (per 1M tokens) - December 2024
MODEL_PRICING = {
    # Latest models
    'claude-opus-4-5-20251101': {'input': 5.00, 'output': 25.00, 'alias': 'opus-4.5'},
    'claude-sonnet-4-5-20250929': {'input': 3.00, 'output': 15.00, 'alias': 'sonnet-4.5'},
    'claude-haiku-4-5-20250929': {'input': 1.00, 'output': 5.00, 'alias': 'haiku-4.5'},

    # Extended thinking (Sonnet 4.5 with >200K prompt)
    'claude-sonnet-4-5-extended': {'input': 6.00, 'output': 22.50, 'alias': 'sonnet-4.5-ext'},

    # Previous generation
    'claude-opus-4-1-20250219': {'input': 15.00, 'output': 75.00, 'alias': 'opus-4.1'},
    'claude-sonnet-4-20250514': {'input': 3.00, 'output': 15.00, 'alias': 'sonnet-4'},

    # Haiku variants
    'claude-3-5-haiku-20241022': {'input': 0.80, 'output': 4.00, 'alias': 'haiku-3.5'},
    'claude-3-haiku-20240307': {'input': 0.25, 'output': 1.25, 'alias': 'haiku-3'},

    # Shortcuts (common aliases)
    'opus': {'input': 5.00, 'output': 25.00, 'alias': 'opus-4.5'},
    'sonnet': {'input': 3.00, 'output': 15.00, 'alias': 'sonnet-4.5'},
    'haiku': {'input': 1.00, 'output': 5.00, 'alias': 'haiku-4.5'},

    # Default fallback
    'unknown': {'input': 3.00, 'output': 15.00, 'alias': 'default-sonnet'},
}


@contextmanager
def get_db():
    """Get database connection with auto-commit."""
    conn = sqlite3.connect(str(DB_PATH), timeout=10.0)
    conn.row_factory = sqlite3.Row
    try:
        yield conn
        conn.commit()
    finally:
        conn.close()


def init_db():
    """Initialize database schema."""
    with get_db() as conn:
        cursor = conn.cursor()

        # Model pricing table
        cursor.execute('''
            CREATE TABLE IF NOT EXISTS model_pricing (
                model_id TEXT PRIMARY KEY,
                alias TEXT,
                input_price REAL NOT NULL,
                output_price REAL NOT NULL,
                context_size INTEGER DEFAULT 200000,
                updated_at TEXT DEFAULT CURRENT_TIMESTAMP
            )
        ''')

        # Token usage table
        cursor.execute('''
            CREATE TABLE IF NOT EXISTS token_usage (
                id INTEGER PRIMARY KEY AUTOINCREMENT,
                timestamp TEXT NOT NULL,
                project TEXT NOT NULL,
                worker_id TEXT NOT NULL,
                model TEXT NOT NULL,
                input_tokens INTEGER NOT NULL,
                output_tokens INTEGER NOT NULL,
                total_tokens INTEGER NOT NULL,
                input_cost REAL NOT NULL,
                output_cost REAL NOT NULL,
                total_cost REAL NOT NULL,
                context TEXT
            )
        ''')

        # Session aggregates table
        cursor.execute('''
            CREATE TABLE IF NOT EXISTS session_stats (
                id INTEGER PRIMARY KEY AUTOINCREMENT,
                project TEXT NOT NULL,
                worker_id TEXT NOT NULL,
                session_start TEXT NOT NULL,
                session_end TEXT,
                total_input INTEGER DEFAULT 0,
                total_output INTEGER DEFAULT 0,
                total_cost REAL DEFAULT 0,
                request_count INTEGER DEFAULT 0,
                models_used TEXT
            )
        ''')

        # Hourly aggregates for quick queries
        cursor.execute('''
            CREATE TABLE IF NOT EXISTS hourly_stats (
                hour TEXT NOT NULL,
                project TEXT NOT NULL,
                worker_id TEXT NOT NULL,
                model TEXT NOT NULL,
                input_tokens INTEGER DEFAULT 0,
                output_tokens INTEGER DEFAULT 0,
                total_cost REAL DEFAULT 0,
                request_count INTEGER DEFAULT 0,
                PRIMARY KEY (hour, project, worker_id, model)
            )
        ''')

        # Indexes for performance
        cursor.execute('CREATE INDEX IF NOT EXISTS idx_usage_project ON token_usage(project)')
        cursor.execute('CREATE INDEX IF NOT EXISTS idx_usage_worker ON token_usage(worker_id)')
        cursor.execute('CREATE INDEX IF NOT EXISTS idx_usage_timestamp ON token_usage(timestamp)')
        cursor.execute('CREATE INDEX IF NOT EXISTS idx_hourly_hour ON hourly_stats(hour)')

        # Populate model pricing
        for model_id, prices in MODEL_PRICING.items():
            cursor.execute('''
                INSERT OR REPLACE INTO model_pricing (model_id, alias, input_price, output_price)
                VALUES (?, ?, ?, ?)
            ''', (model_id, prices.get('alias', model_id), prices['input'], prices['output']))


def get_model_pricing(model: str) -> Dict[str, float]:
    """Get pricing for a model (with fallback)."""
    # Try exact match first
    if model in MODEL_PRICING:
        return MODEL_PRICING[model]

    # Try from database
    with get_db() as conn:
        cursor = conn.cursor()
        cursor.execute('SELECT input_price, output_price FROM model_pricing WHERE model_id = ?', (model,))
        row = cursor.fetchone()
        if row:
            return {'input': row['input_price'], 'output': row['output_price']}

    # Try partial match (e.g., "opus" in "claude-opus-4-5...")
    model_lower = model.lower()
    for key, prices in MODEL_PRICING.items():
        if key in model_lower or model_lower in key:
            return prices

    # Default to Sonnet pricing
    return MODEL_PRICING['unknown']


def calculate_cost(input_tokens: int, output_tokens: int, model: str) -> Tuple[float, float, float]:
    """Calculate cost for tokens. Returns (input_cost, output_cost, total_cost)."""
    pricing = get_model_pricing(model)
    input_cost = (input_tokens / 1_000_000) * pricing['input']
    output_cost = (output_tokens / 1_000_000) * pricing['output']
    return input_cost, output_cost, input_cost + output_cost


def track_usage(input_tokens: int, output_tokens: int, model: str = 'sonnet', context: str = ''):
    """Track token usage with cost calculation."""
    init_db()  # Ensure tables exist

    timestamp = datetime.now().isoformat()
    hour = datetime.now().strftime('%Y-%m-%d-%H')
    total_tokens = input_tokens + output_tokens
    input_cost, output_cost, total_cost = calculate_cost(input_tokens, output_tokens, model)

    with get_db() as conn:
        cursor = conn.cursor()

        # Insert usage record
        cursor.execute('''
            INSERT INTO token_usage
            (timestamp, project, worker_id, model, input_tokens, output_tokens,
             total_tokens, input_cost, output_cost, total_cost, context)
            VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
        ''', (timestamp, PROJECT_NAME, WORKER_ID, model, input_tokens, output_tokens,
              total_tokens, input_cost, output_cost, total_cost, context[:200] if context else ''))

        # Update hourly aggregates
        cursor.execute('''
            INSERT INTO hourly_stats (hour, project, worker_id, model, input_tokens, output_tokens, total_cost, request_count)
            VALUES (?, ?, ?, ?, ?, ?, ?, 1)
            ON CONFLICT(hour, project, worker_id, model) DO UPDATE SET
                input_tokens = input_tokens + excluded.input_tokens,
                output_tokens = output_tokens + excluded.output_tokens,
                total_cost = total_cost + excluded.total_cost,
                request_count = request_count + 1
        ''', (hour, PROJECT_NAME, WORKER_ID, model, input_tokens, output_tokens, total_cost))

    return {
        'input_tokens': input_tokens,
        'output_tokens': output_tokens,
        'total_tokens': total_tokens,
        'input_cost': input_cost,
        'output_cost': output_cost,
        'total_cost': total_cost,
        'model': model,
    }


def get_worker_stats(worker_id: str = None, project: str = None) -> Dict:
    """Get stats for a specific worker."""
    init_db()
    wid = worker_id or WORKER_ID
    proj = project or PROJECT_NAME
    hour = datetime.now().strftime('%Y-%m-%d-%H')

    with get_db() as conn:
        cursor = conn.cursor()

        # Current hour stats
        cursor.execute('''
            SELECT
                SUM(input_tokens) as input,
                SUM(output_tokens) as output,
                SUM(total_cost) as cost,
                SUM(request_count) as reqs
            FROM hourly_stats
            WHERE hour = ? AND worker_id = ? AND project = ?
        ''', (hour, wid, proj))
        hour_row = cursor.fetchone()

        # Session (all time) stats
        cursor.execute('''
            SELECT
                SUM(input_tokens) as input,
                SUM(output_tokens) as output,
                SUM(total_cost) as cost,
                COUNT(*) as reqs
            FROM token_usage
            WHERE worker_id = ? AND project = ?
        ''', (wid, proj))
        session_row = cursor.fetchone()

        # Model breakdown
        cursor.execute('''
            SELECT
                model,
                SUM(input_tokens) as input,
                SUM(output_tokens) as output,
                SUM(total_cost) as cost,
                COUNT(*) as reqs
            FROM token_usage
            WHERE worker_id = ? AND project = ?
            GROUP BY model
        ''', (wid, proj))
        models = {row['model']: {
            'input': row['input'] or 0,
            'output': row['output'] or 0,
            'cost': row['cost'] or 0,
            'requests': row['reqs'] or 0,
        } for row in cursor.fetchall()}

    return {
        'worker_id': wid,
        'project': proj,
        'current_hour': {
            'hour': hour,
            'input': hour_row['input'] or 0 if hour_row else 0,
            'output': hour_row['output'] or 0 if hour_row else 0,
            'cost': hour_row['cost'] or 0 if hour_row else 0,
            'requests': hour_row['reqs'] or 0 if hour_row else 0,
        },
        'session': {
            'input': session_row['input'] or 0 if session_row else 0,
            'output': session_row['output'] or 0 if session_row else 0,
            'cost': session_row['cost'] or 0 if session_row else 0,
            'requests': session_row['reqs'] or 0 if session_row else 0,
        },
        'by_model': models,
    }


def get_all_workers_stats(project: str = None) -> Dict:
    """Get stats for all workers in a project."""
    init_db()
    proj = project or PROJECT_NAME
    hour = datetime.now().strftime('%Y-%m-%d-%H')

    with get_db() as conn:
        cursor = conn.cursor()

        # Get all workers
        cursor.execute('''
            SELECT DISTINCT worker_id FROM token_usage WHERE project = ?
        ''', (proj,))
        workers = [row['worker_id'] for row in cursor.fetchall()]

    result = {'workers': {}, 'totals': {'input': 0, 'output': 0, 'cost': 0, 'requests': 0}}

    for wid in workers:
        stats = get_worker_stats(wid, proj)
        result['workers'][wid] = stats
        result['totals']['input'] += stats['session']['input']
        result['totals']['output'] += stats['session']['output']
        result['totals']['cost'] += stats['session']['cost']
        result['totals']['requests'] += stats['session']['requests']

    return result


def get_model_stats(project: str = None) -> Dict:
    """Get stats grouped by model."""
    init_db()
    proj = project or PROJECT_NAME

    with get_db() as conn:
        cursor = conn.cursor()
        cursor.execute('''
            SELECT
                model,
                SUM(input_tokens) as input,
                SUM(output_tokens) as output,
                SUM(total_cost) as cost,
                COUNT(*) as reqs
            FROM token_usage
            WHERE project = ?
            GROUP BY model
            ORDER BY cost DESC
        ''', (proj,))

        models = {}
        for row in cursor.fetchall():
            pricing = get_model_pricing(row['model'])
            models[row['model']] = {
                'input_tokens': row['input'] or 0,
                'output_tokens': row['output'] or 0,
                'total_cost': row['cost'] or 0,
                'requests': row['reqs'] or 0,
                'pricing': pricing,
            }

    return models


def format_worker_status(compact: bool = False) -> str:
    """Format current worker's status."""
    stats = get_worker_stats()

    if compact:
        s = stats['session']
        return f"Worker {WORKER_ID}: {s['input']+s['output']:,} tok | ${s['cost']:.4f} ({s['requests']} reqs)"

    s = stats['session']
    h = stats['current_hour']
    models = stats['by_model']

    lines = [
        f"╔═══════════════════════════════════════════════════════════════════╗",
        f"║  WORKER TOKEN USAGE: {stats['worker_id']:<43} ║",
        f"╠═══════════════════════════════════════════════════════════════════╣",
        f"║  Current Hour ({h['hour']}):                                   ║",
        f"║    Input:    {h['input']:>12,} tokens                            ║",
        f"║    Output:   {h['output']:>12,} tokens                            ║",
        f"║    Cost:     ${h['cost']:>11.4f} ({h['requests']} requests)              ║",
        f"╠═══════════════════════════════════════════════════════════════════╣",
        f"║  Session Total:                                                   ║",
        f"║    Input:    {s['input']:>12,} tokens                            ║",
        f"║    Output:   {s['output']:>12,} tokens                            ║",
        f"║    Cost:     ${s['cost']:>11.4f} ({s['requests']} requests)              ║",
    ]

    if models:
        lines.append(f"╠═══════════════════════════════════════════════════════════════════╣")
        lines.append(f"║  By Model:                                                        ║")
        for model, m in models.items():
            alias = MODEL_PRICING.get(model, {}).get('alias', model[:20])
            lines.append(f"║    {alias:<16} {m['input']+m['output']:>10,} tok  ${m['cost']:>8.4f}  ({m['requests']:>3} reqs) ║")

    lines.append(f"╚═══════════════════════════════════════════════════════════════════╝")
    return "\n".join(lines)


def format_master_view() -> str:
    """Format aggregated view for master."""
    all_stats = get_all_workers_stats()
    model_stats = get_model_stats()
    hour = datetime.now().strftime('%Y-%m-%d-%H')

    workers = all_stats['workers']
    totals = all_stats['totals']

    lines = [
        f"╔═══════════════════════════════════════════════════════════════════════════════╗",
        f"║                      MASTER TOKEN MONITOR - {PROJECT_NAME:<28} ║",
        f"╠═══════════════════════════════════════════════════════════════════════════════╣",
        f"║  Hour: {hour}                      Workers: {len(workers):<27} ║",
        f"╠═══════════════════════════════════════════════════════════════════════════════╣",
    ]

    # Per-worker breakdown
    if workers:
        lines.append(f"║  {'WORKER':<15} {'INPUT':>12} {'OUTPUT':>12} {'COST':>12} {'REQS':>6} ║")
        lines.append(f"║  {'-'*15} {'-'*12} {'-'*12} {'-'*12} {'-'*6} ║")

        for wid, stats in workers.items():
            s = stats['session']
            lines.append(f"║  {wid:<15} {s['input']:>12,} {s['output']:>12,} ${s['cost']:>10.4f} {s['requests']:>6} ║")
    else:
        lines.append(f"║  (no workers reporting)                                                       ║")

    # Model breakdown
    if model_stats:
        lines.append(f"╠═══════════════════════════════════════════════════════════════════════════════╣")
        lines.append(f"║  BY MODEL:                                                                    ║")
        lines.append(f"║  {'MODEL':<20} {'INPUT':>10} {'OUTPUT':>10} {'COST':>10} {'$/M IN':>8} {'$/M OUT':>8} ║")

        for model, m in model_stats.items():
            alias = MODEL_PRICING.get(model, {}).get('alias', model[:20])
            p = m['pricing']
            lines.append(f"║  {alias:<20} {m['input_tokens']:>10,} {m['output_tokens']:>10,} ${m['total_cost']:>8.4f} ${p['input']:>6.2f} ${p['output']:>6.2f} ║")

    # Totals
    lines.extend([
        f"╠═══════════════════════════════════════════════════════════════════════════════╣",
        f"║  TOTALS                                                                       ║",
        f"║    Input:    {totals['input']:>12,} tokens                                    ║",
        f"║    Output:   {totals['output']:>12,} tokens                                    ║",
        f"║    Cost:     ${totals['cost']:>11.4f}                                         ║",
        f"║    Requests: {totals['requests']:>12,}                                         ║",
        f"╚═══════════════════════════════════════════════════════════════════════════════╝",
    ])

    return "\n".join(lines)


def list_models() -> str:
    """List all available models with pricing."""
    lines = [
        f"╔═══════════════════════════════════════════════════════════════════╗",
        f"║                    CLAUDE API PRICING (per 1M tokens)             ║",
        f"╠═══════════════════════════════════════════════════════════════════╣",
        f"║  {'MODEL':<30} {'INPUT':>10} {'OUTPUT':>10} {'ALIAS':<12} ║",
        f"║  {'-'*30} {'-'*10} {'-'*10} {'-'*12} ║",
    ]

    # Group by category
    latest = ['claude-opus-4-5-20251101', 'claude-sonnet-4-5-20250929', 'claude-haiku-4-5-20250929']
    previous = ['claude-opus-4-1-20250219', 'claude-sonnet-4-20250514']
    haiku = ['claude-3-5-haiku-20241022', 'claude-3-haiku-20240307']

    for model in latest + previous + haiku:
        if model in MODEL_PRICING:
            p = MODEL_PRICING[model]
            lines.append(f"║  {model:<30} ${p['input']:>8.2f} ${p['output']:>8.2f} {p['alias']:<12} ║")

    lines.append(f"╚═══════════════════════════════════════════════════════════════════╝")
    return "\n".join(lines)


# CLI
if __name__ == "__main__":
    import sys
    import json as json_module

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

    elif cmd == "models" or cmd == "pricing":
        print(list_models())

    elif cmd == "json":
        if IS_MASTER:
            print(json_module.dumps(get_all_workers_stats(), indent=2))
        else:
            print(json_module.dumps(get_worker_stats(), indent=2))

    elif cmd == "track":
        # track <input> <output> [model] [context]
        if len(sys.argv) >= 4:
            inp = int(sys.argv[2])
            out = int(sys.argv[3])
            model = sys.argv[4] if len(sys.argv) > 4 else 'sonnet'
            ctx = sys.argv[5] if len(sys.argv) > 5 else ''
            result = track_usage(inp, out, model, ctx)
            print(f"Tracked: +{inp} in, +{out} out = {inp+out} tokens | ${result['total_cost']:.6f} ({model})")
        else:
            print("Usage: pricing_db.py track <input> <output> [model] [context]")

    elif cmd == "cost":
        # Calculate cost for given tokens
        if len(sys.argv) >= 4:
            inp = int(sys.argv[2])
            out = int(sys.argv[3])
            model = sys.argv[4] if len(sys.argv) > 4 else 'sonnet'
            ic, oc, tc = calculate_cost(inp, out, model)
            print(f"Model: {model}")
            print(f"Input:  {inp:>12,} × ${get_model_pricing(model)['input']:.2f}/1M = ${ic:.6f}")
            print(f"Output: {out:>12,} × ${get_model_pricing(model)['output']:.2f}/1M = ${oc:.6f}")
            print(f"Total:  ${tc:.6f}")
        else:
            print("Usage: pricing_db.py cost <input_tokens> <output_tokens> [model]")

    elif cmd == "init":
        init_db()
        print(f"Database initialized at {DB_PATH}")

    else:
        print("Commands:")
        print("  status      - Show current worker status")
        print("  compact     - One-line worker status")
        print("  master/all  - Show all workers (master view)")
        print("  models      - List models with pricing")
        print("  json        - JSON output")
        print("  track <in> <out> [model] [ctx] - Track usage")
        print("  cost <in> <out> [model] - Calculate cost")
        print("  init        - Initialize database")
