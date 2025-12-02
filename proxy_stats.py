#!/usr/bin/env python3
"""
proxy_stats.py - CLIProxyAPI Statistics Client

Fetches usage statistics from router-for.me proxy.
Works for all tools going through the proxy (Claude Code, OpenCode, etc.)
"""

import os
import sys
import json
import urllib.request
import urllib.error
from datetime import datetime

# Configuration
PROXY_URL = os.getenv("PROXY_URL", "http://100.105.212.98:8317")
MANAGEMENT_KEY = os.getenv("CLIPROXYAPI_KEY", "")

# Colors
RED = '\033[0;31m'
GREEN = '\033[0;32m'
YELLOW = '\033[1;33m'
CYAN = '\033[0;36m'
MAGENTA = '\033[0;35m'
NC = '\033[0m'


def fetch_usage() -> dict:
    """Fetch usage statistics from proxy."""
    url = f"{PROXY_URL}/v0/management/usage"

    req = urllib.request.Request(url)
    req.add_header("X-Management-Key", MANAGEMENT_KEY)
    req.add_header("Accept", "application/json")

    try:
        with urllib.request.urlopen(req, timeout=10) as response:
            return json.loads(response.read().decode())
    except urllib.error.HTTPError as e:
        if e.code == 401:
            print(f"{RED}Error: Invalid management key{NC}")
        elif e.code == 404:
            print(f"{RED}Error: Usage endpoint not found{NC}")
        else:
            print(f"{RED}Error: HTTP {e.code}{NC}")
        return {}
    except urllib.error.URLError as e:
        print(f"{RED}Error: Cannot connect to proxy at {PROXY_URL}{NC}")
        print(f"  {e.reason}")
        return {}
    except Exception as e:
        print(f"{RED}Error: {e}{NC}")
        return {}


def check_enabled() -> bool:
    """Check if usage statistics collection is enabled."""
    url = f"{PROXY_URL}/v0/management/usage-statistics-enabled"

    req = urllib.request.Request(url)
    req.add_header("X-Management-Key", MANAGEMENT_KEY)

    try:
        with urllib.request.urlopen(req, timeout=5) as response:
            data = json.loads(response.read().decode())
            return data.get("usage-statistics-enabled", False)
    except:
        return False


def format_tokens(n: int) -> str:
    """Format token count with thousands separator."""
    if n >= 1_000_000:
        return f"{n/1_000_000:.2f}M"
    elif n >= 1_000:
        return f"{n/1_000:.1f}K"
    return str(n)


def print_status(data: dict):
    """Print usage status."""
    if not data:
        return

    print(f"{MAGENTA}═══════════════════════════════════════════════════════════{NC}")
    print(f"{MAGENTA}         PROXY USAGE (CLIProxyAPI){NC}")
    print(f"{MAGENTA}═══════════════════════════════════════════════════════════{NC}")
    print()

    # Overall stats
    total_req = data.get("total_requests", 0)
    success = data.get("success_count", 0)
    failures = data.get("failure_count", 0)
    total_tokens = data.get("total_tokens", 0)

    print(f"  {GREEN}Overall:{NC}")
    print(f"    Requests:  {total_req:,} ({GREEN}{success} ok{NC}, {RED}{failures} fail{NC})")
    print(f"    Tokens:    {format_tokens(total_tokens)}")
    print()

    # Per-endpoint breakdown
    endpoints = data.get("endpoints", {})
    if endpoints:
        print(f"  {GREEN}By Endpoint:{NC}")
        for endpoint, stats in endpoints.items():
            reqs = stats.get("requests", 0)
            inp = stats.get("input_tokens", 0)
            out = stats.get("output_tokens", 0)
            models = stats.get("models", {})

            # Simplify endpoint name
            name = endpoint.replace("/api/provider/", "").replace("/v1/", " ")
            print(f"    {CYAN}{name}{NC}")
            print(f"      Requests: {reqs:,}  In: {format_tokens(inp)}  Out: {format_tokens(out)}")

            if models:
                for model, mstats in models.items():
                    m_inp = mstats.get("input_tokens", 0)
                    m_out = mstats.get("output_tokens", 0)
                    print(f"        {YELLOW}{model}{NC}: {format_tokens(m_inp)} in / {format_tokens(m_out)} out")
        print()

    # Daily breakdown
    daily = data.get("by_day", {})
    if daily:
        print(f"  {GREEN}By Day:{NC}")
        for day, stats in sorted(daily.items(), reverse=True)[:7]:
            reqs = stats.get("requests", 0)
            tokens = stats.get("tokens", 0)
            print(f"    {day}: {reqs:,} requests, {format_tokens(tokens)} tokens")
        print()


def print_compact(data: dict):
    """Print one-line summary."""
    if not data:
        print("No data")
        return

    total_req = data.get("total_requests", 0)
    total_tokens = data.get("total_tokens", 0)
    success = data.get("success_count", 0)

    print(f"Proxy: {total_req} req | {format_tokens(total_tokens)} tokens | {success} ok")


def print_json(data: dict):
    """Print raw JSON."""
    print(json.dumps(data, indent=2))


def print_models(data: dict):
    """Print per-model breakdown."""
    if not data:
        return

    print(f"{MAGENTA}═══════════════════════════════════════════════════════════{NC}")
    print(f"{MAGENTA}              USAGE BY MODEL{NC}")
    print(f"{MAGENTA}═══════════════════════════════════════════════════════════{NC}")
    print()

    # Aggregate by model across all endpoints
    models = {}
    for endpoint, stats in data.get("endpoints", {}).items():
        for model, mstats in stats.get("models", {}).items():
            if model not in models:
                models[model] = {"requests": 0, "input": 0, "output": 0, "cached": 0}
            models[model]["requests"] += mstats.get("requests", 0)
            models[model]["input"] += mstats.get("input_tokens", 0)
            models[model]["output"] += mstats.get("output_tokens", 0)
            models[model]["cached"] += mstats.get("cached_tokens", 0)

    if not models:
        print("  No model data available")
        return

    # Sort by total tokens
    sorted_models = sorted(models.items(), key=lambda x: x[1]["input"] + x[1]["output"], reverse=True)

    for model, stats in sorted_models:
        total = stats["input"] + stats["output"]
        print(f"  {CYAN}{model}{NC}")
        print(f"    Requests: {stats['requests']:,}")
        print(f"    Input:    {format_tokens(stats['input'])}")
        print(f"    Output:   {format_tokens(stats['output'])}")
        if stats["cached"]:
            print(f"    Cached:   {format_tokens(stats['cached'])}")
        print(f"    Total:    {format_tokens(total)}")
        print()


def main():
    if not MANAGEMENT_KEY:
        # Try to load from secrets
        secrets_file = "/app/secrets.env"
        if os.path.exists(secrets_file):
            with open(secrets_file) as f:
                for line in f:
                    if line.startswith("CLIPROXYAPI_KEY="):
                        global MANAGEMENT_KEY
                        MANAGEMENT_KEY = line.strip().split("=", 1)[1].strip('"\'')
                        break

    if not MANAGEMENT_KEY:
        print(f"{RED}Error: CLIPROXYAPI_KEY not set{NC}")
        print("Set via environment variable or add to ~/.urp/secrets.env")
        sys.exit(1)

    cmd = sys.argv[1] if len(sys.argv) > 1 else "status"

    if cmd == "status":
        data = fetch_usage()
        print_status(data)

    elif cmd == "compact":
        data = fetch_usage()
        print_compact(data)

    elif cmd == "json":
        data = fetch_usage()
        print_json(data)

    elif cmd == "models":
        data = fetch_usage()
        print_models(data)

    elif cmd == "enabled":
        enabled = check_enabled()
        print(f"Usage statistics collection: {GREEN if enabled else RED}{'enabled' if enabled else 'disabled'}{NC}")

    elif cmd == "help":
        print("Usage: proxy_stats.py [command]")
        print()
        print("Commands:")
        print("  status   - Show usage statistics (default)")
        print("  compact  - One-line summary")
        print("  models   - Per-model breakdown")
        print("  json     - Raw JSON output")
        print("  enabled  - Check if collection is enabled")
        print()
        print("Environment:")
        print("  PROXY_URL        - Proxy base URL (default: http://100.105.212.98:8317)")
        print("  CLIPROXYAPI_KEY  - Management API key")

    else:
        print(f"Unknown command: {cmd}")
        print("Use 'proxy_stats.py help' for usage")
        sys.exit(1)


if __name__ == "__main__":
    main()
