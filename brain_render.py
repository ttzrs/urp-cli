# ═══════════════════════════════════════════════════════════════════════════════
# Brain Render: Graph → LLM-Digestible Formats
# ═══════════════════════════════════════════════════════════════════════════════
#
# LLMs don't understand raw graph JSON. They understand:
# - Code (signatures, imports, decorators)
# - Markdown (headers, lists, emphasis)
# - Narrative (cause → effect chains)
#
# This module converts graph query results into formats that maximize
# LLM comprehension while minimizing token count.

from typing import Any
from dataclasses import dataclass


@dataclass
class RenderConfig:
    """Controls verbosity and format."""
    mode: str = "code"        # code | markdown | trace | minimal
    max_lines: int = 50       # Truncate long outputs
    show_line_numbers: bool = False
    include_decorators: bool = True


# ═══════════════════════════════════════════════════════════════════════════════
# Mode: CODE (Graph-as-Pseudocode)
# ═══════════════════════════════════════════════════════════════════════════════
# Best for: Structure, dependencies, call graphs
# LLM reads it like type definitions


def render_as_code(nodes: list[dict], edges: list[dict], config: RenderConfig = None) -> str:
    """
    Convert graph topology to pseudo-code.

    Output looks like TypeScript .d.ts or Python stubs.
    LLMs are trained on millions of these → high comprehension.
    """
    config = config or RenderConfig()
    lines = []
    lines.append("// TOPOLOGY MAP (not real code)")

    # Group by file
    by_file: dict[str, list] = {}
    for node in nodes:
        path = node.get('path') or node.get('file_path') or 'unknown'
        if path not in by_file:
            by_file[path] = []
        by_file[path].append(node)

    # Build edge lookup: source_id → [(rel_type, target_name)]
    edge_map: dict[str, list] = {}
    for edge in edges:
        src = edge.get('from') or edge.get('source')
        if src not in edge_map:
            edge_map[src] = []
        edge_map[src].append((
            edge.get('type', 'RELATES'),
            edge.get('to') or edge.get('target')
        ))

    for path, entities in by_file.items():
        lines.append(f"\nmodule '{_short_path(path)}' {{")

        for entity in entities:
            name = entity.get('name') or entity.get('signature', '?')
            etype = entity.get('type') or _infer_type(entity)

            # Add decorators for dependencies (if enabled)
            if config.include_decorators:
                deps = edge_map.get(name, [])
                for rel_type, target in deps[:5]:  # Limit decorators
                    lines.append(f"  @{rel_type}({target})")

            # Render signature
            sig = entity.get('signature', f"{etype.lower()} {name}")
            lines.append(f"  {sig} {{ ... }}")

        lines.append("}")

    return "\n".join(lines[:config.max_lines])


# ═══════════════════════════════════════════════════════════════════════════════
# Mode: TRACE (Causal Narrative)
# ═══════════════════════════════════════════════════════════════════════════════
# Best for: Errors, timelines, debugging
# LLM reads it like a story with cause → effect


def render_as_trace(events: list[dict], conclusion: str = None) -> str:
    """
    Convert temporal events to inverse-chronological narrative.

    Most recent (the pain) at top. Root cause at bottom.
    """
    if not events:
        return "No events to trace."

    lines = []
    lines.append("### CAUSAL TRACE (newest → oldest)")
    lines.append("")

    # Sort by timestamp descending
    sorted_events = sorted(events, key=lambda e: e.get('timestamp', 0), reverse=True)

    for i, event in enumerate(sorted_events[:10]):  # Limit to 10
        # Determine icon based on exit code or type
        exit_code = event.get('exit_code', 0)
        if exit_code != 0:
            icon = "X"  # Error
        elif 'Commit' in str(event.get('type', '')):
            icon = ">"  # Git
        else:
            icon = "o"  # Normal

        # Format time delta
        time_str = event.get('datetime', event.get('time', '?'))
        if isinstance(time_str, str) and len(time_str) > 16:
            time_str = time_str[11:16]  # Just HH:MM

        # The event description
        cmd = event.get('cmd') or event.get('command') or event.get('message', '?')
        error = event.get('error') or event.get('stderr_preview', '')

        if i == 0:
            lines.append(f"[{icon}] LATEST: {cmd}")
            if error:
                lines.append(f"    Error: {error[:100]}")
        else:
            lines.append(f"[{icon}] {time_str}: {cmd[:60]}")

    if conclusion:
        lines.append("")
        lines.append(f"CONCLUSION: {conclusion}")

    return "\n".join(lines)


# ═══════════════════════════════════════════════════════════════════════════════
# Mode: MINIMAL (Extreme Token Economy)
# ═══════════════════════════════════════════════════════════════════════════════
# Best for: Quick lookups, yes/no decisions
# Just the facts, no decoration


def render_minimal(data: Any, label: str = "DATA") -> str:
    """
    Absolute minimum tokens. For when every token counts.
    """
    if isinstance(data, list):
        if not data:
            return f"{label}: (empty)"
        # Just names/keys
        items = [str(d.get('name') or d.get('cmd') or d)[:30] for d in data[:10]]
        return f"{label}: {', '.join(items)}"

    if isinstance(data, dict):
        # Key=value pairs, truncated
        pairs = [f"{k}={str(v)[:20]}" for k, v in list(data.items())[:5]]
        return f"{label}: {' | '.join(pairs)}"

    return f"{label}: {str(data)[:100]}"


# ═══════════════════════════════════════════════════════════════════════════════
# Mode: ULTRA-MINIMAL (Extreme Token Economy)
# ═══════════════════════════════════════════════════════════════════════════════
# Best for: High-frequency operations, status checks
# Symbols instead of words. ~90% token reduction.

# Symbol mappings for ultra-minimal mode
_SYMBOLS = {
    # Status
    'success': '✓',
    'error': '✗',
    'warning': '⚠',
    'info': 'ℹ',
    'pending': '◌',
    'running': '◐',
    'complete': '●',

    # Types
    'file': '📄',
    'function': 'ƒ',
    'class': '◆',
    'module': '▣',
    'container': '⬡',

    # Actions
    'add': '+',
    'remove': '-',
    'modify': '~',
    'move': '→',
    'call': '→',

    # Severity
    'high': '!!!',
    'medium': '!!',
    'low': '!',

    # Time
    'recent': '◉',
    'old': '○',
}


def render_ultra_minimal(data: Any, label: str = None) -> str:
    """
    Extreme compression: symbols only, minimal prose.
    Use for: status bars, quick checks, high-frequency queries.

    Token savings: ~90% vs full text.

    Examples:
    - Events: "✓git ✓npm ✗pytest" instead of "git succeeded, npm succeeded, pytest failed"
    - Files: "📄main.py ƒfoo ƒbar" instead of "File main.py contains functions foo and bar"
    - Errors: "✗[!!!] ModuleNotFound" instead of "Error (high severity): ModuleNotFoundError"
    """
    if data is None:
        return "∅"

    if isinstance(data, bool):
        return "✓" if data else "✗"

    if isinstance(data, (int, float)):
        # Compact number formatting
        if isinstance(data, float):
            return f"{data:.1f}"
        if data >= 1000000:
            return f"{data/1000000:.1f}M"
        if data >= 1000:
            return f"{data/1000:.1f}K"
        return str(data)

    if isinstance(data, str):
        # Check for known status words
        lower = data.lower()
        if lower in _SYMBOLS:
            return _SYMBOLS[lower]
        # Truncate long strings
        return data[:20] + "…" if len(data) > 20 else data

    if isinstance(data, list):
        if not data:
            return "[]"

        # Detect if it's events (has exit_code), nodes (has type), or generic
        sample = data[0] if data else {}

        if isinstance(sample, dict):
            if 'exit_code' in sample:
                # Events: show as status chain
                parts = []
                for item in data[:10]:
                    status = "✓" if item.get('exit_code', 0) == 0 else "✗"
                    cmd = item.get('cmd', item.get('command', '?'))
                    # Extract base command
                    base = cmd.split()[0] if isinstance(cmd, str) else '?'
                    parts.append(f"{status}{base}")
                return " ".join(parts)

            elif 'type' in sample or 'labels' in sample:
                # Nodes: show type + name
                parts = []
                for item in data[:10]:
                    etype = item.get('type', 'entity')
                    name = item.get('name', '?')
                    symbol = _SYMBOLS.get(etype.lower(), '•')
                    parts.append(f"{symbol}{name[:15]}")
                return " ".join(parts)

            elif 'severity' in sample or 'type' in sample:
                # Noise patterns or issues
                parts = []
                for item in data[:5]:
                    sev = _SYMBOLS.get(item.get('severity', 'low'), '!')
                    ptype = item.get('type', '?')[:10]
                    count = item.get('count', '')
                    parts.append(f"{sev}{ptype}:{count}")
                return " ".join(parts)

            else:
                # Generic dict list: key=val format
                parts = []
                for item in data[:5]:
                    first_key = next(iter(item.keys()), None)
                    if first_key:
                        val = str(item[first_key])[:10]
                        parts.append(val)
                return ",".join(parts)

        else:
            # List of primitives
            return ",".join(str(x)[:10] for x in data[:10])

    if isinstance(data, dict):
        # Detect specific dict types
        if 'exit_code' in data:
            # Single event
            status = "✓" if data.get('exit_code', 0) == 0 else "✗"
            cmd = data.get('cmd', data.get('command', '?'))
            return f"{status}{cmd[:20]}"

        if 'tokens' in data and 'items' in data:
            # Working memory status
            return f"◐{data.get('count', '?')}:{data.get('tokens', 0)}tok"

        if 'used' in data and 'budget' in data:
            # Token budget
            pct = data.get('usage_pct', 0)
            icon = "✓" if pct < 50 else ("⚠" if pct < 80 else "✗")
            return f"{icon}{pct}%"

        if 'mode' in data:
            # Mode status
            mode = data.get('mode', '?')[:4]
            return f"▣{mode}"

        # Generic dict: compact key=val
        parts = []
        for k, v in list(data.items())[:4]:
            key = k[:6]
            val = render_ultra_minimal(v) if isinstance(v, (dict, list)) else str(v)[:8]
            parts.append(f"{key}={val}")
        return " ".join(parts)

    return str(data)[:30]


def render_status_bar(context_status: dict, token_status: dict, mode: str = None) -> str:
    """
    Render a single-line status bar for context optimization.

    Output: "[▣hyb] ◐5:1.2K ✓23% ⚠old:3"

    Components:
    - Mode indicator
    - Working memory (count:tokens)
    - Token budget %
    - Top warning
    """
    parts = []

    # Mode
    if mode:
        mode_short = mode[:3]
        parts.append(f"[▣{mode_short}]")

    # Working memory
    wm = context_status.get('working_memory', {})
    count = wm.get('count', 0)
    tokens = wm.get('total_tokens', 0)
    if tokens >= 1000:
        tokens_str = f"{tokens/1000:.1f}K"
    else:
        tokens_str = str(tokens)
    parts.append(f"◐{count}:{tokens_str}")

    # Token budget
    ts = token_status or {}
    pct = ts.get('usage_pct', 0)
    icon = "✓" if pct < 50 else ("⚠" if pct < 80 else "✗")
    parts.append(f"{icon}{pct}%")

    # Top warning from noise patterns
    noise = context_status.get('noise_patterns', [])
    if noise:
        top = noise[0]
        sev = _SYMBOLS.get(top.get('severity', 'low'), '!')
        ptype = top.get('type', '?')[:6]
        count = top.get('count', 0)
        parts.append(f"{sev}{ptype}:{count}")

    return " ".join(parts)


# ═══════════════════════════════════════════════════════════════════════════════
# Mode: MARKDOWN (Structured Narrative)
# ═══════════════════════════════════════════════════════════════════════════════
# Best for: Explanations, summaries, reports
# Good when user will also read the output


def render_as_markdown(data: dict, title: str = "Context") -> str:
    """
    Structured markdown for mixed audiences (LLM + human).
    """
    lines = []
    lines.append(f"## {title}")
    lines.append("")

    for section, content in data.items():
        lines.append(f"### {section}")

        if isinstance(content, list):
            for item in content[:10]:
                if isinstance(item, dict):
                    name = item.get('name') or item.get('cmd') or '?'
                    detail = item.get('path') or item.get('time') or ''
                    lines.append(f"- **{name}** {detail}")
                else:
                    lines.append(f"- {item}")
        elif isinstance(content, dict):
            for k, v in content.items():
                lines.append(f"- {k}: {v}")
        else:
            lines.append(str(content))

        lines.append("")

    return "\n".join(lines)


# ═══════════════════════════════════════════════════════════════════════════════
# Smart Renderer (Auto-selects based on data shape)
# ═══════════════════════════════════════════════════════════════════════════════


def render_smart(data: Any, hint: str = None) -> str:
    """
    Automatically choose best rendering based on data type and hint.

    Hints: 'error', 'deps', 'history', 'structure', 'quick'
    """
    # Error/pain data → trace format
    if hint in ('error', 'pain', 'trace'):
        if isinstance(data, list):
            return render_as_trace(data)

    # Dependency/structure data → code format
    if hint in ('deps', 'structure', 'focus', 'topology'):
        nodes = data.get('nodes', []) if isinstance(data, dict) else data
        edges = data.get('edges', []) if isinstance(data, dict) else []
        return render_as_code(nodes, edges)

    # Quick lookup → minimal
    if hint == 'quick':
        return render_minimal(data)

    # Default: markdown for mixed data
    if isinstance(data, dict):
        return render_as_markdown(data)

    if isinstance(data, list):
        return render_as_trace(data) if _looks_like_events(data) else render_minimal(data, "Items")

    return str(data)


# ═══════════════════════════════════════════════════════════════════════════════
# Code Fragment Renderer (For surgical focus)
# ═══════════════════════════════════════════════════════════════════════════════


def render_code_fragment(
    name: str,
    path: str,
    code: str,
    deps: list[str] = None,
    callers: list[str] = None,
    line_start: int = None
) -> str:
    """
    Render a code fragment with minimal but useful context.

    This is what focus() outputs - surgical precision.
    """
    lines = []
    lines.append(f"// FOCUS: {name}")
    lines.append(f"// File: {_short_path(path)}" + (f":{line_start}" if line_start else ""))

    # Dependencies as compact list (not decorators - save tokens)
    if deps:
        lines.append(f"// Uses: {', '.join(deps[:5])}")

    if callers:
        lines.append(f"// Called by: {', '.join(callers[:3])}")

    lines.append("")

    # The actual code (already trimmed by caller)
    lines.append(code.strip())

    return "\n".join(lines)


def render_wisdom_result(matches: list[dict]) -> str:
    """
    Render wisdom (similar past errors) in a decision-friendly format.
    """
    if not matches:
        return "WISDOM: No similar errors found. You're the pioneer."

    lines = []
    lines.append("WISDOM: Similar past errors found:")
    lines.append("")

    for i, m in enumerate(matches[:3], 1):
        sim = int(m.get('similarity', 0) * 100)
        cmd = m.get('cmd', '?')[:50]
        error = m.get('error', '')[:80]

        lines.append(f"{i}. [{sim}% match] {cmd}")
        if error:
            lines.append(f"   Error: {error}")
        lines.append("")

    # Decision hint
    top_sim = int(matches[0].get('similarity', 0) * 100)
    if top_sim >= 80:
        lines.append("RECOMMENDATION: Apply the historical solution.")
    elif top_sim >= 50:
        lines.append("RECOMMENDATION: Review the past solution, may need adaptation.")
    else:
        lines.append("RECOMMENDATION: Low confidence match. Investigate fresh.")

    return "\n".join(lines)


def render_novelty_result(score: float, level: str, message: str) -> str:
    """
    Render novelty check in action-oriented format.
    """
    icon = {'safe': 'OK', 'moderate': '??', 'high': '!!', 'pioneer': '**', 'unknown': '?'}
    return f"NOVELTY [{icon.get(level, '?')}]: {score:.0%} - {message}"


# ═══════════════════════════════════════════════════════════════════════════════
# Helpers
# ═══════════════════════════════════════════════════════════════════════════════


def _short_path(path: str) -> str:
    """Truncate path to last 2 components."""
    if not path:
        return "?"
    parts = path.replace("\\", "/").split("/")
    return "/".join(parts[-2:]) if len(parts) > 2 else path


def _infer_type(node: dict) -> str:
    """Guess entity type from node properties."""
    labels = node.get('labels', [])
    if 'Function' in labels:
        return 'func'
    if 'Class' in labels:
        return 'class'
    if 'File' in labels:
        return 'file'
    return 'entity'


def _looks_like_events(data: list) -> bool:
    """Check if list looks like temporal events."""
    if not data:
        return False
    sample = data[0]
    return isinstance(sample, dict) and any(
        k in sample for k in ('timestamp', 'datetime', 'exit_code', 'cmd')
    )
