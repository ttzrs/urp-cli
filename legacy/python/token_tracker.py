"""
Token Tracker - Monitoreo de uso de tokens con historial temporal.

Rastrea:
- Tokens usados por archivo/función
- Historial de uso por hora
- Presupuesto restante con ajuste dinámico
"""
import os
import json
import time
from datetime import datetime, timedelta
from pathlib import Path
from typing import Optional

# Configuración
TOKENS_FILE = os.getenv('URP_TOKENS_FILE', '/shared/sessions/token_usage.json')
MAX_TOKENS = int(os.getenv('URP_MAX_CONTEXT_TOKENS', '4000'))
HOURLY_BUDGET = int(os.getenv('URP_HOURLY_BUDGET', '50000'))  # Tokens por hora


def estimate_tokens(text: str) -> int:
    """Estimación de tokens: ~4 caracteres por token."""
    if not text:
        return 0
    return len(text) // 4


def estimate_file_tokens(file_path: str) -> int:
    """Estima tokens de un archivo."""
    try:
        with open(file_path, 'r', errors='ignore') as f:
            content = f.read()
        return estimate_tokens(content)
    except Exception:
        return 0


def load_usage() -> dict:
    """Carga el historial de uso de tokens."""
    try:
        if os.path.exists(TOKENS_FILE):
            with open(TOKENS_FILE, 'r') as f:
                return json.load(f)
    except Exception:
        pass

    return {
        "created": datetime.now().isoformat(),
        "hourly_budget": HOURLY_BUDGET,
        "current_hour": datetime.now().strftime("%Y-%m-%d-%H"),
        "hour_used": 0,
        "total_used": 0,
        "history": [],  # Lista de {hour, tokens, files}
        "files": {},    # {path: {tokens, last_read, reads}}
    }


def save_usage(usage: dict):
    """Guarda el historial de uso."""
    try:
        Path(TOKENS_FILE).parent.mkdir(parents=True, exist_ok=True)
        with open(TOKENS_FILE, 'w') as f:
            json.dump(usage, f, indent=2)
    except Exception:
        pass


def _rotate_hour(usage: dict) -> dict:
    """Rota el contador si cambió la hora."""
    current_hour = datetime.now().strftime("%Y-%m-%d-%H")

    if usage.get("current_hour") != current_hour:
        # Guardar hora anterior en historial
        if usage.get("hour_used", 0) > 0:
            usage["history"].append({
                "hour": usage.get("current_hour"),
                "tokens": usage.get("hour_used", 0),
                "timestamp": datetime.now().isoformat()
            })
            # Mantener solo últimas 24 horas
            usage["history"] = usage["history"][-24:]

        # Reset para nueva hora
        usage["current_hour"] = current_hour
        usage["hour_used"] = 0

    return usage


def track_read(file_path: str, tokens: int, context: str = "read"):
    """Registra lectura de tokens."""
    usage = load_usage()
    usage = _rotate_hour(usage)

    # Actualizar contadores
    usage["hour_used"] = usage.get("hour_used", 0) + tokens
    usage["total_used"] = usage.get("total_used", 0) + tokens

    # Actualizar info de archivo
    if "files" not in usage:
        usage["files"] = {}

    file_key = str(file_path)
    if file_key not in usage["files"]:
        usage["files"][file_key] = {
            "tokens": 0,
            "reads": 0,
            "first_read": datetime.now().isoformat()
        }

    usage["files"][file_key]["tokens"] += tokens
    usage["files"][file_key]["reads"] += 1
    usage["files"][file_key]["last_read"] = datetime.now().isoformat()
    usage["files"][file_key]["context"] = context

    save_usage(usage)
    return usage


def get_remaining() -> dict:
    """Obtiene tokens restantes para la hora actual."""
    usage = load_usage()
    usage = _rotate_hour(usage)

    hour_used = usage.get("hour_used", 0)
    hour_budget = usage.get("hourly_budget", HOURLY_BUDGET)
    remaining = max(0, hour_budget - hour_used)

    # Calcular minutos restantes en la hora
    now = datetime.now()
    minutes_left = 60 - now.minute

    # Tasa de uso (tokens por minuto en esta hora)
    minutes_elapsed = max(now.minute, 5)  # Need at least 5 min of data for reliable projection
    rate = hour_used / minutes_elapsed

    # Proyección: si sigue así, ¿alcanza?
    projected = hour_used + (rate * minutes_left)

    # Only warn if:
    # 1. Already used >50% of budget, OR
    # 2. Projected to exceed AND have enough data (>10 min elapsed)
    usage_pct = round(100 * hour_used / hour_budget, 1) if hour_budget else 0
    will_exceed = (
        (usage_pct > 50) or
        (projected > hour_budget and now.minute >= 10)
    )

    return {
        "hour": usage.get("current_hour"),
        "used": hour_used,
        "budget": hour_budget,
        "remaining": remaining,
        "usage_pct": usage_pct,
        "minutes_left": minutes_left,
        "rate_per_min": round(rate, 1),
        "projected_total": round(projected),
        "will_exceed": will_exceed,
        "status": "⚠️ RATE HIGH" if will_exceed else "✅ OK"
    }


def get_stats() -> dict:
    """Obtiene estadísticas completas de uso."""
    usage = load_usage()
    usage = _rotate_hour(usage)

    remaining = get_remaining()

    # Top archivos por tokens
    files = usage.get("files", {})
    top_files = sorted(
        [{"path": k, **v} for k, v in files.items()],
        key=lambda x: x.get("tokens", 0),
        reverse=True
    )[:10]

    # Historial por hora
    history = usage.get("history", [])[-12:]  # Últimas 12 horas

    return {
        "current": remaining,
        "total_all_time": usage.get("total_used", 0),
        "files_tracked": len(files),
        "top_files": top_files,
        "hourly_history": history,
        "session_start": usage.get("created")
    }


def format_status(compact: bool = False) -> str:
    """Formatea el estado actual para mostrar."""
    r = get_remaining()

    if compact:
        bar_len = 20
        filled = int(bar_len * r["usage_pct"] / 100)
        bar = "█" * filled + "░" * (bar_len - filled)
        return f"[{bar}] {r['used']}/{r['budget']} ({r['usage_pct']}%) {r['status']}"

    stats = get_stats()

    lines = [
        "╔════════════════════════════════════════════════════════════╗",
        "║                    TOKEN USAGE                             ║",
        "╠════════════════════════════════════════════════════════════╣",
        f"║ Hour: {r['hour']}                                    ║",
        f"║ Used: {r['used']:,} / {r['budget']:,} ({r['usage_pct']}%)              ║",
        f"║ Remaining: {r['remaining']:,} tokens                           ║",
        f"║ Rate: {r['rate_per_min']:.1f} tokens/min                              ║",
        f"║ Projected: {r['projected_total']:,} tokens by hour end             ║",
        f"║ Status: {r['status']}                                       ║",
        "╠════════════════════════════════════════════════════════════╣",
        f"║ Total (all time): {stats['total_all_time']:,} tokens                  ║",
        f"║ Files tracked: {stats['files_tracked']}                               ║",
        "╚════════════════════════════════════════════════════════════╝",
    ]

    if stats['top_files']:
        lines.append("\nTop files by tokens:")
        for f in stats['top_files'][:5]:
            name = Path(f['path']).name
            lines.append(f"  {name}: {f['tokens']:,} tokens ({f['reads']} reads)")

    return "\n".join(lines)


def adjust_budget(new_budget: int) -> dict:
    """Ajusta el presupuesto por hora."""
    usage = load_usage()
    old_budget = usage.get("hourly_budget", HOURLY_BUDGET)
    usage["hourly_budget"] = new_budget
    save_usage(usage)

    return {
        "old_budget": old_budget,
        "new_budget": new_budget,
        "current_used": usage.get("hour_used", 0),
        "remaining": max(0, new_budget - usage.get("hour_used", 0))
    }


def reset_hour():
    """Resetea el contador de la hora actual."""
    usage = load_usage()
    usage["hour_used"] = 0
    save_usage(usage)
    return {"status": "reset", "hour": usage.get("current_hour")}


# CLI
if __name__ == "__main__":
    import sys

    if len(sys.argv) < 2:
        print(format_status())
        sys.exit(0)

    cmd = sys.argv[1]

    if cmd == "status":
        print(format_status())

    elif cmd == "compact":
        print(format_status(compact=True))

    elif cmd == "remaining":
        r = get_remaining()
        print(json.dumps(r, indent=2))

    elif cmd == "stats":
        s = get_stats()
        print(json.dumps(s, indent=2, default=str))

    elif cmd == "budget":
        if len(sys.argv) > 2:
            new_budget = int(sys.argv[2])
            result = adjust_budget(new_budget)
            print(f"Budget adjusted: {result['old_budget']} → {result['new_budget']}")
            print(f"Remaining this hour: {result['remaining']}")
        else:
            r = get_remaining()
            print(f"Current budget: {r['budget']} tokens/hour")

    elif cmd == "reset":
        result = reset_hour()
        print(f"Hour counter reset for {result['hour']}")

    elif cmd == "track":
        if len(sys.argv) > 2:
            file_path = sys.argv[2]
            tokens = estimate_file_tokens(file_path)
            track_read(file_path, tokens)
            print(f"Tracked: {file_path} ({tokens} tokens)")
        else:
            print("Usage: token_tracker.py track <file_path>")

    else:
        print("Commands: status, compact, remaining, stats, budget [N], reset, track <file>")
