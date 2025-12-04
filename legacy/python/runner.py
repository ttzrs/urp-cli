"""
Transparent command runner + Cognitive skills

τ (Vector): Command execution timeline
Φ (Morphism): Exit codes, resource usage as energy flow
⊥ (Orthogonal): Failures and conflicts

Cognitive extensions:
- wisdom: Query similar past errors and their solutions
- novelty: Check if proposed code breaks existing patterns
- focus: Selective context loading for attention management
- tokens: Track token usage with hourly budget
"""
import os
import sys
import time
import subprocess
import shlex
import json
from datetime import datetime
from typing import Optional

# Token tracking (lazy import)
def _track_tokens(file_path: str, tokens: int, context: str = "read"):
    """Track token usage if tracker available."""
    try:
        from token_tracker import track_read
        track_read(file_path, tokens, context)
    except ImportError:
        pass

# Lazy import database to avoid startup overhead for simple commands
_db = None


def get_db():
    """Lazy database connection."""
    global _db
    if _db is None:
        try:
            from database import Database
            _db = Database()
        except Exception:
            _db = False  # Mark as failed, don't retry
    return _db if _db else None


def log_to_graph(
    command: str,
    exit_code: int,
    duration: float,
    cwd: str,
    stdout_preview: str = "",
    stderr_preview: str = "",
):
    """
    Log command execution to the graph (τ + Φ + ⊥).
    Fails silently - never block the real command.
    """
    db = get_db()
    if not db:
        return

    try:
        # Escape single quotes for Cypher
        cmd_safe = command.replace("'", "\\'")[:500]
        stdout_safe = stdout_preview.replace("'", "\\'")[:200]
        stderr_safe = stderr_preview.replace("'", "\\'")[:200]
        cwd_safe = cwd.replace("'", "\\'")

        # Determine event type based on command
        cmd_base = command.split()[0] if command else "unknown"
        event_type = "TerminalCommand"

        # Classify by primitive
        labels = ["Event", "TerminalEvent"]
        if exit_code != 0:
            labels.append("Conflict")  # ⊥

        if cmd_base in ('git', 'svn', 'hg'):
            labels.append("VCSEvent")
        elif cmd_base in ('docker', 'podman', 'kubectl', 'k3s'):
            labels.append("ContainerEvent")
        elif cmd_base in ('npm', 'pip', 'cargo', 'go', 'make', 'mvn', 'gradle'):
            labels.append("BuildEvent")
        elif cmd_base in ('pytest', 'jest', 'mocha', 'go test'):
            labels.append("TestEvent")

        label_str = ":".join(labels)

        # Generate embedding for errors (for wisdom lookups)
        embedding_str = "null"
        if exit_code != 0 and stderr_preview:
            try:
                from brain_cortex import get_embedding
                vec = get_embedding(stderr_preview)
                if vec:
                    embedding_str = json.dumps(vec)
            except Exception:
                pass

        # Get project name from environment
        project_name = os.environ.get("PROJECT_NAME", "unknown")

        db.execute_query(f"""
            CREATE (e:{label_str} {{
                command: $cmd,
                cmd_base: $base,
                exit_code: $exit,
                duration_sec: $dur,
                cwd: $cwd,
                stdout_preview: $stdout,
                stderr_preview: $stderr,
                timestamp: $ts,
                datetime: $dt,
                project: $project,
                embedding: {embedding_str}
            }})
        """, {
            "cmd": cmd_safe,
            "base": cmd_base,
            "exit": exit_code,
            "dur": round(duration, 3),
            "cwd": cwd_safe,
            "stdout": stdout_safe,
            "stderr": stderr_safe,
            "ts": int(time.time()),
            "dt": datetime.now().isoformat(),
            "project": project_name,
        })

        # Link to session context if exists
        db.execute_query("""
            MATCH (s:Session {active: true})
            MATCH (e:TerminalEvent)
            WHERE e.timestamp = $ts
            MERGE (s)-[:EXECUTED]->(e)
        """, {"ts": int(time.time())})

    except Exception:
        pass  # Silent fail - never block the command


def run_transparent(args: list[str], capture: bool = False) -> int:
    """
    Execute command transparently, log to graph, return exit code.

    Args:
        args: Command and arguments as list ['git', 'status']
        capture: If True, capture output for logging (slower, no colors)

    Returns:
        Exit code from the command
    """
    if not args:
        return 0

    command_str = shlex.join(args)

    # ─────────────────────────────────────────────────────────────────────────
    # PHASE 0: IMMUNE SYSTEM CHECK (⊥ Orthogonal - Pre-execution filter)
    # ─────────────────────────────────────────────────────────────────────────
    try:
        from immune_system import analyze_risk, get_safe_alternative

        is_safe, reason = analyze_risk(command_str)

        if not is_safe:
            print(f"\n{reason}", file=sys.stderr)
            print(f"SUGGESTION: {get_safe_alternative(reason)}", file=sys.stderr)

            # Log the blocked attempt to graph (agent "feels pain" without damage)
            log_to_graph(
                command=command_str,
                exit_code=126,  # 126 = command cannot execute
                duration=0.0,
                cwd=os.getcwd(),
                stderr_preview=reason,
            )
            return 126
    except ImportError:
        pass  # Immune system not available, proceed without filter

    cwd = os.getcwd()
    start_time = time.time()

    stdout_preview = ""
    stderr_preview = ""

    try:
        if capture:
            # Capture mode: get output for logging but lose interactivity
            result = subprocess.run(
                args,
                capture_output=True,
                text=True,
            )
            exit_code = result.returncode
            stdout_preview = result.stdout[:500] if result.stdout else ""
            stderr_preview = result.stderr[:500] if result.stderr else ""

            # Print output to terminal
            if result.stdout:
                sys.stdout.write(result.stdout)
            if result.stderr:
                sys.stderr.write(result.stderr)
        else:
            # Transparent mode: preserve colors, interactivity, everything
            # Output streams directly to terminal
            result = subprocess.run(args)
            exit_code = result.returncode

    except FileNotFoundError:
        print(f"Command not found: {args[0]}", file=sys.stderr)
        exit_code = 127
        stderr_preview = f"Command not found: {args[0]}"
    except KeyboardInterrupt:
        exit_code = 130
        stderr_preview = "Interrupted"
    except Exception as e:
        print(f"Error executing command: {e}", file=sys.stderr)
        exit_code = 1
        stderr_preview = str(e)

    duration = time.time() - start_time

    # Log to graph (async-ish, but synchronous for simplicity)
    log_to_graph(
        command=command_str,
        exit_code=exit_code,
        duration=duration,
        cwd=cwd,
        stdout_preview=stdout_preview,
        stderr_preview=stderr_preview,
    )

    return exit_code


def run_with_resources(args: list[str]) -> int:
    """
    Execute command and measure resource usage (Φ energy).
    Uses /usr/bin/time if available.
    """
    if not args:
        return 0

    # Try to use GNU time for resource measurement
    time_cmd = "/usr/bin/time"
    if os.path.exists(time_cmd):
        # -v for verbose output, stderr for timing info
        full_args = [time_cmd, "-v"] + args
        return run_transparent(full_args, capture=False)
    else:
        return run_transparent(args, capture=False)


def query_recent_commands(limit: int = 20, errors_only: bool = False, project: str = None) -> list[dict]:
    """Query recent terminal events from the graph.

    Args:
        limit: Max number of results
        errors_only: Only show failed commands
        project: Filter by project name (None = all projects)
    """
    db = get_db()
    if not db:
        return []

    try:
        where_clauses = []
        if errors_only:
            where_clauses.append("e.exit_code <> 0")
        if project:
            where_clauses.append("e.project = $project")

        where_clause = "WHERE " + " AND ".join(where_clauses) if where_clauses else ""

        results = db.execute_query(f"""
            MATCH (e:TerminalEvent)
            {where_clause}
            RETURN e.command as cmd,
                   e.exit_code as exit,
                   e.duration_sec as duration,
                   e.datetime as time,
                   e.cwd as cwd,
                   e.project as project
            ORDER BY e.timestamp DESC
            LIMIT $limit
        """, {"limit": limit, "project": project})
        return [dict(r) for r in results]
    except Exception:
        return []


def query_pain(minutes: int = 5, project: str = None) -> list[dict]:
    """
    Find recent errors/conflicts (⊥).
    The "pain" the system experienced.

    Args:
        minutes: Look back N minutes
        project: Filter by project name (None = all projects)
    """
    db = get_db()
    if not db:
        return []

    try:
        cutoff = int(time.time()) - (minutes * 60)

        project_clause = "AND e.project = $project" if project else ""

        results = db.execute_query(f"""
            MATCH (e:Conflict)
            WHERE e.timestamp > $cutoff {project_clause}
            RETURN e.command as cmd,
                   e.exit_code as exit,
                   e.stderr_preview as error,
                   e.datetime as time,
                   e.cwd as cwd,
                   e.project as project
            ORDER BY e.timestamp DESC
        """, {"cutoff": cutoff, "project": project})
        return [dict(r) for r in results]
    except Exception:
        return []


def start_session(name: str = None) -> bool:
    """Start a new session context (T tensor)."""
    db = get_db()
    if not db:
        return False

    try:
        # Deactivate any existing sessions
        db.execute_query("MATCH (s:Session {active: true}) SET s.active = false")

        # Create new session
        session_name = name or f"session_{int(time.time())}"
        db.execute_query("""
            CREATE (s:Session {
                name: $name,
                active: true,
                started: $ts,
                cwd: $cwd
            })
        """, {
            "name": session_name,
            "ts": int(time.time()),
            "cwd": os.getcwd(),
        })
        return True
    except Exception:
        return False


# ═══════════════════════════════════════════════════════════════════════════════
# Working Memory Manager (Token Economy)
# ═══════════════════════════════════════════════════════════════════════════════

SESSION_FILE = os.path.join(os.path.dirname(__file__), ".session_context.json")
MAX_CONTEXT_TOKENS = int(os.getenv('URP_MAX_CONTEXT_TOKENS', '4000'))


def load_working_memory() -> dict:
    """Load working memory from session file."""
    if not os.path.exists(SESSION_FILE):
        return {"focused_nodes": [], "context_cache": {}}
    try:
        with open(SESSION_FILE, 'r') as f:
            return json.load(f)
    except Exception:
        return {"focused_nodes": [], "context_cache": {}}


def save_working_memory(data: dict):
    """Persist working memory."""
    try:
        with open(SESSION_FILE, 'w') as f:
            json.dump(data, f, indent=2)
    except Exception:
        pass


def estimate_tokens(text: str) -> int:
    """Rough token estimation: ~4 chars per token."""
    return len(text) // 4


def _calculate_eviction_score(item: dict) -> float:
    """
    Calculate eviction priority score.
    Lower score = evict first.

    Replaces simple FIFO with LRU + importance scoring.
    """
    age_sec = time.time() - item.get('added_at', 0)
    importance = item.get('importance', 1)
    access_count = item.get('access_count', 1)

    # Normalize age (0-1, newer = higher score = keep longer)
    # 1 hour decay
    age_score = max(0, 1 - (age_sec / 3600))

    # Normalize importance (assume 1-5 scale)
    importance_score = min(importance, 5) / 5.0

    # Normalize access count (cap at 10)
    access_score = min(access_count, 10) / 10.0

    # Weighted score: importance most important, then recency, then usage
    return (importance_score * 0.4) + (age_score * 0.3) + (access_score * 0.3)


def add_to_focus(target: str, importance: int = 2) -> dict:
    """Add target to working memory with LRU+importance eviction."""
    state = load_working_memory()

    # If already in focus, update access count (LRU touch)
    if target in state["focused_nodes"]:
        if target in state.get("context_cache", {}):
            state["context_cache"][target]["access_count"] = \
                state["context_cache"][target].get("access_count", 1) + 1
            state["context_cache"][target]["last_access"] = int(time.time())
            save_working_memory(state)
        return {"action": "touched", "target": target, "reason": "Updated access count"}

    # Get context for the target
    context = focus_context(target, depth=2)

    # Estimate tokens
    context_str = json.dumps(context)
    tokens = estimate_tokens(context_str)

    # Check budget
    total_tokens = sum(c.get('tokens', 0) for c in state.get("context_cache", {}).values())
    evicted = []

    if total_tokens + tokens > MAX_CONTEXT_TOKENS:
        # Build scored list of current items
        scored_items = []
        for node in state["focused_nodes"]:
            cache = state.get("context_cache", {}).get(node, {})
            item = {
                "name": node,
                "tokens": cache.get("tokens", 0),
                "added_at": cache.get("added_at", 0),
                "importance": cache.get("importance", 1),
                "access_count": cache.get("access_count", 1)
            }
            item["score"] = _calculate_eviction_score(item)
            scored_items.append(item)

        # Sort by score ascending (lowest score = evict first)
        scored_items.sort(key=lambda x: x["score"])

        # Evict lowest-scored items until we have room
        for item in scored_items:
            if (total_tokens + tokens) <= MAX_CONTEXT_TOKENS:
                break
            old = item["name"]
            if old in state["focused_nodes"]:
                state["focused_nodes"].remove(old)
                old_tokens = state["context_cache"].get(old, {}).get('tokens', 0)
                total_tokens -= old_tokens
                if old in state["context_cache"]:
                    del state["context_cache"][old]
                evicted.append({"name": old, "score": round(item["score"], 3)})

    # Add new target
    state["focused_nodes"].append(target)
    state["context_cache"][target] = {
        "context": context,
        "tokens": tokens,
        "added_at": int(time.time()),
        "last_access": int(time.time()),
        "importance": importance,
        "access_count": 1
    }

    save_working_memory(state)

    # Track token usage
    _track_tokens(target, tokens, "focus")

    result = {
        "action": "added",
        "target": target,
        "tokens": tokens,
        "importance": importance,
        "total_focused": len(state["focused_nodes"]),
        "context": context
    }
    if evicted:
        result["evicted"] = [e["name"] for e in evicted]
        result["evicted_details"] = evicted
    return result


def remove_from_focus(target: str) -> dict:
    """Remove specific target from working memory."""
    state = load_working_memory()

    if target not in state["focused_nodes"]:
        return {"action": "skip", "target": target, "reason": "Not in focus"}

    state["focused_nodes"].remove(target)
    tokens_freed = 0
    if target in state["context_cache"]:
        tokens_freed = state["context_cache"][target].get('tokens', 0)
        del state["context_cache"][target]

    save_working_memory(state)

    return {
        "action": "removed",
        "target": target,
        "tokens_freed": tokens_freed,
        "remaining": len(state["focused_nodes"])
    }


def clear_working_memory() -> dict:
    """Clear all focused context (tabula rasa)."""
    state = load_working_memory()

    old_count = len(state["focused_nodes"])
    old_tokens = sum(c.get('tokens', 0) for c in state.get("context_cache", {}).values())

    state["focused_nodes"] = []
    state["context_cache"] = {}
    save_working_memory(state)

    return {
        "action": "cleared",
        "nodes_removed": old_count,
        "tokens_freed": old_tokens
    }


def get_working_memory_status() -> dict:
    """Show current working memory state and token usage."""
    state = load_working_memory()

    items = []
    total_tokens = 0
    for node in state["focused_nodes"]:
        cache = state.get("context_cache", {}).get(node, {})
        tokens = cache.get('tokens', 0)
        total_tokens += tokens
        items.append({
            "name": node,
            "tokens": tokens,
            "age_sec": int(time.time()) - cache.get('added_at', 0)
        })

    return {
        "focused_count": len(state["focused_nodes"]),
        "total_tokens": total_tokens,
        "budget": MAX_CONTEXT_TOKENS,
        "usage_pct": round(100 * total_tokens / MAX_CONTEXT_TOKENS, 1) if MAX_CONTEXT_TOKENS else 0,
        "items": items
    }


def get_accumulated_context() -> str:
    """
    Return all focused context as a single string.
    This is what gets injected into agent prompts.
    """
    state = load_working_memory()

    if not state["focused_nodes"]:
        return ""

    output = "\n--- 🧠 WORKING MEMORY (Auto-Injected) ---\n"

    for node in state["focused_nodes"]:
        cache = state.get("context_cache", {}).get(node, {})
        ctx = cache.get('context', {})

        output += f"\n## FOCUS: {node}\n"
        if ctx.get('nodes'):
            output += f"Nodes: {', '.join(ctx['nodes'][:10])}\n"
        if ctx.get('edges'):
            for edge in ctx['edges'][:5]:
                output += f"  {edge['from']} --{edge['type']}--> {edge['to']}\n"

    output += f"\n// {len(state['focused_nodes'])} items, ~{sum(c.get('tokens', 0) for c in state.get('context_cache', {}).values())} tokens\n"
    return output


# ═══════════════════════════════════════════════════════════════════════════════
# Cognitive Skills
# ═══════════════════════════════════════════════════════════════════════════════


def consult_wisdom(error_msg: str, threshold: float = 0.7, project: str = None) -> list[dict]:
    """
    Find similar past errors and their resolutions.

    Uses vector similarity to find semantically related errors,
    not just string matching.

    Args:
        error_msg: The error to search for
        threshold: Similarity threshold (0.0-1.0)
        project: Filter by project name (None = all projects for cross-learning)
    """
    db = get_db()
    if not db:
        return []

    try:
        from brain_cortex import get_embedding, cosine_similarity

        # Embed the current error
        query_vec = get_embedding(error_msg)
        if not query_vec:
            return []

        # Get all past conflicts with embeddings
        project_clause = "AND e.project = $project" if project else ""
        results = db.execute_query(f"""
            MATCH (e:Conflict)
            WHERE e.embedding IS NOT NULL {project_clause}
            RETURN e.command as cmd,
                   e.stderr_preview as error,
                   e.datetime as time,
                   e.embedding as embedding,
                   e.project as project
            ORDER BY e.timestamp DESC
            LIMIT 100
        """, {"project": project})

        # Calculate similarities locally
        # (Memgraph has distance functions but may not have cosine built-in)
        matches = []
        for r in results:
            if not r.get('embedding'):
                continue
            sim = cosine_similarity(query_vec, r['embedding'])
            if sim >= threshold:
                matches.append({
                    'cmd': r['cmd'],
                    'error': r['error'],
                    'time': r['time'],
                    'project': r.get('project', 'unknown'),
                    'similarity': round(sim, 3)
                })

        # Sort by similarity
        matches.sort(key=lambda x: x['similarity'], reverse=True)
        return matches[:5]

    except Exception as e:
        print(f"Wisdom query failed: {e}", file=sys.stderr)
        return []


def check_novelty(code_or_text: str) -> dict:
    """
    Check how novel/unusual a piece of code is compared to the codebase.

    Returns novelty score 0.0 (common pattern) to 1.0 (completely new).
    """
    db = get_db()
    result = {
        'novelty': 0.5,
        'level': 'unknown',
        'message': 'Could not calculate novelty'
    }

    try:
        from brain_cortex import get_embedding, calculate_novelty

        # Embed the new code
        new_vec = get_embedding(code_or_text)
        if not new_vec:
            return result

        # Get existing function/code embeddings
        query_result = db.execute_query("""
            MATCH (f:Function)
            WHERE f.embedding IS NOT NULL
            RETURN f.embedding as embedding
            LIMIT 100
        """) if db else []

        history = [r['embedding'] for r in query_result if r.get('embedding')]

        if not history:
            # No history = everything is new
            result['novelty'] = 1.0
            result['level'] = 'pioneer'
            result['message'] = 'No existing patterns. You are the pioneer.'
            return result

        # Calculate novelty
        novelty = calculate_novelty(new_vec, history)
        result['novelty'] = round(novelty, 3)

        if novelty < 0.3:
            result['level'] = 'safe'
            result['message'] = 'Standard pattern. Safe to proceed.'
        elif novelty < 0.7:
            result['level'] = 'moderate'
            result['message'] = 'Some innovation. Review recommended.'
        else:
            result['level'] = 'high'
            result['message'] = 'Novel pattern! Verify this is intentional.'

        return result

    except Exception as e:
        result['message'] = f"Novelty check failed: {e}"
        return result


def consolidate_learning(description: str = "Solution validated", minutes: int = 10) -> dict:
    """
    Reinforce successful command sequences in the graph.

    When a task succeeds, this creates a :Solution node and links
    the successful command chain to it. Future wisdom queries will
    find this validated solution.

    This is "Long-Term Potentiation" - converting short-term experience
    into permanent knowledge.
    """
    db = get_db()
    if not db:
        return {'success': False, 'error': 'No database connection'}

    try:
        cutoff = int(time.time()) - (minutes * 60)

        # 1. Find recent successful commands (not conflicts)
        results = db.execute_query("""
            MATCH (e:TerminalEvent)
            WHERE e.timestamp > $cutoff
              AND NOT e:Conflict
            RETURN e.command as cmd,
                   e.timestamp as ts,
                   e.exit_code as exit_code,
                   id(e) as node_id
            ORDER BY e.timestamp ASC
        """, {"cutoff": cutoff})

        events = list(results)

        if not events:
            return {
                'success': False,
                'error': 'No successful commands in the last {} minutes'.format(minutes)
            }

        # 2. Create a Solution node (crystallized knowledge)
        solution_id = f"sol_{int(time.time())}"

        db.execute_query("""
            CREATE (s:Solution {
                id: $sol_id,
                description: $desc,
                created_at: $ts,
                command_count: $count
            })
        """, {
            "sol_id": solution_id,
            "desc": description,
            "ts": int(time.time()),
            "count": len(events)
        })

        # 3. Link events to solution (CONTRIBUTED_TO relationship)
        # This creates the "golden path" that wisdom can find later
        for event in events:
            db.execute_query("""
                MATCH (e:TerminalEvent) WHERE id(e) = $eid
                MATCH (s:Solution {id: $sol_id})
                MERGE (e)-[:CONTRIBUTED_TO]->(s)
            """, {"eid": event['node_id'], "sol_id": solution_id})

        # 4. If there were conflicts before success, link them too
        # (so wisdom knows what error this solution fixes)
        conflicts = db.execute_query("""
            MATCH (c:Conflict)
            WHERE c.timestamp > $cutoff AND c.timestamp < $first_success
            RETURN id(c) as node_id, c.stderr_preview as error
        """, {"cutoff": cutoff - 300, "first_success": events[0]['ts']})

        for conflict in conflicts:
            db.execute_query("""
                MATCH (c:Conflict) WHERE id(c) = $cid
                MATCH (s:Solution {id: $sol_id})
                MERGE (s)-[:RESOLVES]->(c)
            """, {"cid": conflict['node_id'], "sol_id": solution_id})

        return {
            'success': True,
            'solution_id': solution_id,
            'commands_linked': len(events),
            'conflicts_resolved': len(list(conflicts)) if conflicts else 0,
            'description': description
        }

    except Exception as e:
        return {'success': False, 'error': str(e)}


def focus_context(target: str, depth: int = 2) -> dict:
    """
    Get focused context around a target (file, function, class).

    Returns a subgraph of only relevant nodes to reduce LLM hallucination.
    """
    db = get_db()
    if not db:
        return {'nodes': [], 'edges': [], 'error': 'No database connection'}

    # Sanitize depth (Memgraph doesn't support parameterized variable-length paths)
    depth = max(1, min(int(depth), 5))

    try:
        # Find the target node and its neighbors up to depth
        # Note: depth is interpolated directly because Memgraph doesn't support $depth in paths
        query = f"""
            MATCH (target)
            WHERE target.path CONTAINS $target
               OR target.name CONTAINS $target
               OR target.signature CONTAINS $target
            MATCH path = (target)-[*1..{depth}]-(related)
            RETURN DISTINCT
                labels(target)[0] as target_type,
                target.name as target_name,
                target.path as target_path,
                labels(related)[0] as related_type,
                related.name as related_name,
                related.path as related_path,
                type(relationships(path)[0]) as edge_type
            LIMIT 50
        """
        results = db.execute_query(query, {"target": target})

        nodes = set()
        edges = []

        for r in results:
            nodes.add(f"{r['target_type']}:{r.get('target_name') or r.get('target_path')}")
            nodes.add(f"{r['related_type']}:{r.get('related_name') or r.get('related_path')}")
            edges.append({
                'from': r.get('target_name') or r.get('target_path'),
                'to': r.get('related_name') or r.get('related_path'),
                'type': r['edge_type']
            })

        return {
            'target': target,
            'depth': depth,
            'nodes': list(nodes),
            'edges': edges
        }

    except Exception as e:
        return {'nodes': [], 'edges': [], 'error': str(e)}


def surgical_read(target: str) -> dict:
    """
    Read only the specific lines of a function/class from the file.

    Uses graph metadata (start_line, end_line) to extract surgical context.
    Saves ~90% tokens vs reading whole file.
    """
    db = get_db()
    if not db:
        return {'error': 'No database connection'}

    try:
        # Find target in graph with line numbers
        results = db.execute_query("""
            MATCH (e)
            WHERE (e:Function OR e:Class)
              AND (e.name = $target OR e.signature CONTAINS $target)
            RETURN e.name as name,
                   e.path as path,
                   e.file_path as file_path,
                   e.start_line as start_line,
                   e.end_line as end_line,
                   e.signature as signature,
                   labels(e)[0] as type
            LIMIT 1
        """, {"target": target})

        result = None
        for r in results:
            result = dict(r)
            break

        if not result:
            return {'error': f"Target '{target}' not found in graph. Run 'urp ingest' first."}

        path = result.get('path') or result.get('file_path')
        start = result.get('start_line')
        end = result.get('end_line')

        if not path or not start or not end:
            return {'error': 'Target found but missing line numbers. Re-ingest with AST parser.'}

        # Read only the surgical slice
        if not os.path.exists(path):
            return {'error': f"File not found: {path}"}

        with open(path, 'r', encoding='utf-8', errors='replace') as f:
            lines = f.readlines()

        # Adjust for 0-based index, add 1 line context before
        start_idx = max(0, start - 2)
        end_idx = min(len(lines), end)

        code = ''.join(lines[start_idx:end_idx])

        # Get dependencies (what this calls)
        deps_result = db.execute_query("""
            MATCH (e)-[:CALLS]->(dep)
            WHERE e.name = $target
            RETURN DISTINCT dep.name as dep
            LIMIT 10
        """, {"target": target})
        deps = [r['dep'] for r in deps_result if r.get('dep')]

        # Get callers (what calls this)
        callers_result = db.execute_query("""
            MATCH (caller)-[:CALLS]->(e)
            WHERE e.name = $target
            RETURN DISTINCT caller.name as caller
            LIMIT 5
        """, {"target": target})
        callers = [r['caller'] for r in callers_result if r.get('caller')]

        # Track token usage for surgical read
        code_tokens = estimate_tokens(code)
        _track_tokens(path, code_tokens, "surgical")

        return {
            'name': result['name'],
            'type': result['type'],
            'path': path,
            'start_line': start,
            'end_line': end,
            'code': code,
            'dependencies': deps,
            'callers': callers,
            'tokens_saved': f"~{len(lines) - (end - start)} lines not loaded",
            'tokens_used': code_tokens
        }

    except Exception as e:
        return {'error': str(e)}


def main():
    """CLI entry point for runner module."""
    import argparse

    parser = argparse.ArgumentParser(description="Transparent command runner + cognitive skills")
    subparsers = parser.add_subparsers(dest="action", required=True)

    # run - execute a command
    p = subparsers.add_parser("run", help="Run a command transparently")
    p.add_argument("cmd", nargs=argparse.REMAINDER, help="Command to run")
    p.add_argument("--capture", action="store_true", help="Capture output for logging")

    # recent - show recent commands
    p = subparsers.add_parser("recent", help="Show recent commands")
    p.add_argument("--limit", type=int, default=20, help="Number of commands")
    p.add_argument("--errors", action="store_true", help="Only show errors")
    p.add_argument("--project", help="Filter by project name")
    p.add_argument("--all", action="store_true", help="Show all projects (default)")

    # pain - show recent errors
    p = subparsers.add_parser("pain", help="Show recent errors")
    p.add_argument("--minutes", type=int, default=5, help="Look back N minutes")
    p.add_argument("--project", help="Filter by project name")
    p.add_argument("--all", action="store_true", help="Show all projects (default)")

    # session - start a session
    p = subparsers.add_parser("session", help="Start a new session")
    p.add_argument("--name", help="Session name")

    # wisdom - consult past errors
    p = subparsers.add_parser("wisdom", help="Find similar past errors")
    p.add_argument("error", help="Error message to search for")
    p.add_argument("--threshold", type=float, default=0.7, help="Similarity threshold")
    p.add_argument("--project", help="Filter by project name")
    p.add_argument("--all", action="store_true", help="Search all projects (default)")

    # novelty - check code novelty
    p = subparsers.add_parser("novelty", help="Check if code is novel/unusual")
    p.add_argument("code", help="Code or text to check")

    # focus - get focused context
    p = subparsers.add_parser("focus", help="Get focused context for a target")
    p.add_argument("target", help="Target file/function/class name")
    p.add_argument("--depth", type=int, default=2, help="Relationship depth")

    # learn - consolidate successful commands into solution
    p = subparsers.add_parser("learn", help="Consolidate recent success into knowledge")
    p.add_argument("description", nargs="?", default="Solution validated",
                   help="Description of what was solved")
    p.add_argument("--minutes", type=int, default=10, help="Look back N minutes")

    # surgical - read only specific lines of a function/class
    p = subparsers.add_parser("surgical", help="Read only specific function/class (not whole file)")
    p.add_argument("target", help="Function or class name to extract")

    # unfocus - remove from working memory
    p = subparsers.add_parser("unfocus", help="Remove target from working memory")
    p.add_argument("target", help="Target to remove from focus")

    # clear - clear all working memory
    subparsers.add_parser("clear", help="Clear all working memory (tabula rasa)")

    # status - show working memory status
    subparsers.add_parser("status", help="Show working memory status and token usage")

    # context - show accumulated context (for injection)
    subparsers.add_parser("context", help="Show accumulated context (what gets injected)")

    # tokens - token tracking commands
    p = subparsers.add_parser("tokens", help="Token usage tracking")
    p.add_argument("--budget", type=int, help="Set new hourly budget")
    p.add_argument("--reset", action="store_true", help="Reset hour counter")
    p.add_argument("--compact", action="store_true", help="Compact output")

    # ─────────────────────────────────────────────────────────────────────────
    # MEMORY MANAGEMENT COMMANDS
    # ─────────────────────────────────────────────────────────────────────────

    # remember - add to session memory
    p = subparsers.add_parser("remember", help="Add note to session memory")
    p.add_argument("text", help="Text to remember")
    p.add_argument("--kind", default="note",
                   choices=["note", "summary", "decision", "result", "observation"],
                   help="Type of memory")
    p.add_argument("--importance", type=int, default=2, help="Importance (1-5)")
    p.add_argument("--tags", help="Comma-separated tags")

    # recall - search session memory
    p = subparsers.add_parser("recall", help="Search session memory")
    p.add_argument("query", help="What to search for")
    p.add_argument("--n", type=int, default=5, help="Max results")
    p.add_argument("--kind", help="Filter by type")

    # memories - list all session memories
    subparsers.add_parser("memories", help="List all session memories")

    # knowledge - store/query shared knowledge
    p = subparsers.add_parser("knowledge", help="Store or query knowledge")
    p.add_argument("action", choices=["store", "query", "list", "reject", "export"],
                   help="Knowledge action")
    p.add_argument("text", nargs="?", help="Text to store or query")
    p.add_argument("--kind", default="rule", help="Knowledge type")
    p.add_argument("--scope", default="global", choices=["session", "instance", "global"],
                   help="Visibility scope")
    p.add_argument("--id", help="Knowledge ID (for reject/export)")
    p.add_argument("--reason", help="Rejection reason")
    p.add_argument("--n", type=int, default=5, help="Max results for query")

    # memstats - memory statistics
    subparsers.add_parser("memstats", help="Show memory and knowledge statistics")

    # identity - show current context/identity
    subparsers.add_parser("identity", help="Show current session identity/context")

    # should - metacognitive evaluation
    p = subparsers.add_parser("should", help="Metacognitive evaluation")
    p.add_argument("action", choices=["save", "promote", "reject"],
                   help="What to evaluate")
    p.add_argument("target", help="Text (for save) or ID (for promote/reject)")

    args = parser.parse_args()

    if args.action == "run":
        if not args.cmd:
            print("No command specified", file=sys.stderr)
            sys.exit(1)
        exit_code = run_transparent(args.cmd, capture=args.capture)
        sys.exit(exit_code)

    elif args.action == "recent":
        # --project overrides --all; if neither, show all
        project_filter = args.project if hasattr(args, 'project') and args.project else None
        results = query_recent_commands(args.limit, args.errors, project=project_filter)
        # Add header showing scope
        scope = f"project:{project_filter}" if project_filter else "all projects"
        print(f"# Recent commands ({scope})")
        print(json.dumps(results, indent=2))

    elif args.action == "pain":
        project_filter = args.project if hasattr(args, 'project') and args.project else None
        results = query_pain(args.minutes, project=project_filter)
        # Use trace renderer for causal narrative
        from brain_render import render_as_trace
        scope = f"project:{project_filter}" if project_filter else "all projects"
        print(render_as_trace(results, f"Errors in last {args.minutes} minutes ({scope})"))

    elif args.action == "session":
        if start_session(args.name):
            print(f"Session started: {args.name or 'auto'}")
        else:
            print("Failed to start session", file=sys.stderr)
            sys.exit(1)

    elif args.action == "wisdom":
        project_filter = args.project if hasattr(args, 'project') and args.project else None
        results = consult_wisdom(args.error, args.threshold, project=project_filter)
        # Use renderer for LLM-friendly output
        from brain_render import render_wisdom_result
        scope = f"(project: {project_filter})" if project_filter else "(all projects)"
        print(f"# Wisdom search {scope}")
        print(render_wisdom_result(results))

    elif args.action == "novelty":
        result = check_novelty(args.code)
        # Use renderer for LLM-friendly output
        from brain_render import render_novelty_result
        print(render_novelty_result(result['novelty'], result['level'], result['message']))

    elif args.action == "focus":
        # Add to working memory (persistent)
        result = add_to_focus(args.target)
        if result.get('action') == 'skip':
            print(f"Already focused: {args.target}")
        else:
            print(f"FOCUSED: {args.target} (+{result.get('tokens', 0)} tokens)")
            if result.get('evicted'):
                print(f"EVICTED (budget): {', '.join(result['evicted'])}")

        # Show the context
        ctx = result.get('context') or focus_context(args.target, args.depth)
        if ctx.get('error'):
            print(f"Focus failed: {ctx['error']}", file=sys.stderr)
            sys.exit(1)
        from brain_render import render_as_code
        print(render_as_code(
            [{'name': n, 'type': n.split(':')[0]} for n in ctx.get('nodes', [])],
            ctx.get('edges', [])
        ))

    elif args.action == "learn":
        result = consolidate_learning(args.description, args.minutes)
        if result.get('success'):
            print(f"LEARNED: {result['description']}")
            print(f"  Solution ID: {result['solution_id']}")
            print(f"  Commands linked: {result['commands_linked']}")
            if result.get('conflicts_resolved'):
                print(f"  Conflicts resolved: {result['conflicts_resolved']}")
            print("Knowledge crystallized. Future wisdom queries will find this solution.")
        else:
            print(f"Learning failed: {result.get('error')}", file=sys.stderr)
            sys.exit(1)

    elif args.action == "surgical":
        result = surgical_read(args.target)
        if result.get('error'):
            print(f"Surgical read failed: {result['error']}", file=sys.stderr)
            sys.exit(1)
        # Use renderer for LLM-friendly output
        from brain_render import render_code_fragment
        print(render_code_fragment(
            name=result['name'],
            path=result['path'],
            code=result['code'],
            deps=result.get('dependencies'),
            callers=result.get('callers'),
            line_start=result.get('start_line')
        ))
        print(f"\n// {result['tokens_saved']}")

    elif args.action == "unfocus":
        result = remove_from_focus(args.target)
        if result.get('action') == 'skip':
            print(f"Not in focus: {args.target}")
        else:
            print(f"UNFOCUSED: {args.target} (-{result.get('tokens_freed', 0)} tokens)")
            print(f"Remaining in focus: {result.get('remaining', 0)}")

    elif args.action == "clear":
        result = clear_working_memory()
        print(f"TABULA RASA: Cleared {result['nodes_removed']} nodes")
        print(f"Tokens freed: {result['tokens_freed']}")

    elif args.action == "status":
        status = get_working_memory_status()
        print(f"WORKING MEMORY STATUS")
        print(f"  Budget: {status['total_tokens']}/{status['budget']} tokens ({status['usage_pct']}%)")
        print(f"  Items: {status['focused_count']}")
        if status['items']:
            print("\n  Items in focus:")
            for item in status['items']:
                age = item['age_sec']
                age_str = f"{age//60}m" if age >= 60 else f"{age}s"
                print(f"    - {item['name']}: {item['tokens']} tokens (age: {age_str})")

    elif args.action == "context":
        ctx = get_accumulated_context()
        if ctx:
            print(ctx)
        else:
            print("(Working memory empty)")

    elif args.action == "tokens":
        try:
            from token_tracker import format_status, adjust_budget, reset_hour, get_remaining
            if args.budget:
                result = adjust_budget(args.budget)
                print(f"Budget adjusted: {result['old_budget']} → {result['new_budget']}")
                print(f"Remaining this hour: {result['remaining']}")
            elif args.reset:
                result = reset_hour()
                print(f"Hour counter reset for {result['hour']}")
            else:
                print(format_status(compact=args.compact))
        except ImportError:
            print("Token tracker not available", file=sys.stderr)
            sys.exit(1)

    # ─────────────────────────────────────────────────────────────────────────
    # MEMORY MANAGEMENT HANDLERS
    # ─────────────────────────────────────────────────────────────────────────

    elif args.action == "remember":
        from llm_tools import add_session_note
        tags = args.tags.split(",") if args.tags else None
        result = add_session_note(args.text, kind=args.kind, importance=args.importance, tags=tags)
        if result.get("error"):
            print(f"Error: {result['error']}", file=sys.stderr)
            sys.exit(1)
        print(f"REMEMBERED [{args.kind}]: {args.text[:50]}...")
        print(f"  ID: {result['memory_id']}")
        print(f"  Importance: {args.importance}/5")

    elif args.action == "recall":
        from llm_tools import recall_session_memory
        results = recall_session_memory(args.query, n_results=args.n, kind=args.kind)
        if not results:
            print("No memories found")
        else:
            print(f"RECALL: '{args.query}' ({len(results)} results)\n")
            for r in results:
                if r.get("error"):
                    print(f"  Error: {r['error']}")
                    continue
                sim = r.get('similarity', 0)
                print(f"  [{sim:.0%}] ({r.get('kind', '?')}) {r.get('text', '')[:60]}...")
                print(f"       ID: {r.get('memory_id', '?')}")

    elif args.action == "memories":
        from llm_tools import list_session_memory
        memories = list_session_memory()
        if not memories:
            print("No memories in current session")
        else:
            print(f"SESSION MEMORIES ({len(memories)} total)\n")
            for m in memories:
                if m.get("error"):
                    print(f"  Error: {m['error']}")
                    continue
                print(f"  [{m.get('kind', '?')}] {m.get('text', '')[:50]}...")
                print(f"       ID: {m.get('memory_id', '?')} | Importance: {m.get('importance', '?')}/5")

    elif args.action == "knowledge":
        from llm_tools import store_knowledge, query_knowledge, reject_knowledge, export_memory_to_global, list_global_knowledge

        if args.action == "knowledge":
            if args.action == "knowledge" and hasattr(args, 'action'):
                kaction = getattr(args, 'action', None)
                # Re-read from parsed args
                pass

        # Determine sub-action from positional 'action' arg
        kaction = args.__dict__.get('action', 'query')
        # Actually args.action is "knowledge", we need args' second positional
        # Let me fix this - the action is stored in a different way
        import sys as _sys
        # Parse the actual knowledge action from argv
        if len(_sys.argv) > 2:
            kaction = _sys.argv[2]
        else:
            kaction = "list"

        if kaction == "store":
            if not args.text:
                print("Error: text required for store", file=sys.stderr)
                sys.exit(1)
            result = store_knowledge(args.text, kind=args.kind, scope=args.scope)
            if result.get("error"):
                print(f"Error: {result['error']}", file=sys.stderr)
                sys.exit(1)
            print(f"STORED [{args.kind}] scope={args.scope}")
            print(f"  ID: {result['knowledge_id']}")

        elif kaction == "query":
            if not args.text:
                print("Error: query text required", file=sys.stderr)
                sys.exit(1)
            results = query_knowledge(args.text, n_results=args.n)
            if not results:
                print("No knowledge found")
            else:
                print(f"KNOWLEDGE: '{args.text}' ({len(results)} results)\n")
                for r in results:
                    if r.get("error"):
                        print(f"  Error: {r['error']}")
                        continue
                    sim = r.get('similarity', 0)
                    print(f"  [{sim:.0%}] {r.get('scope', '?')}/{r.get('kind', '?')}: {r.get('text', '')[:50]}...")
                    print(f"       ID: {r.get('knowledge_id', '?')}")

        elif kaction == "list":
            results = list_global_knowledge(kind=args.kind if args.kind != "rule" else None)
            if not results:
                print("No knowledge stored")
            else:
                print(f"ALL KNOWLEDGE ({len(results)} entries)\n")
                for r in results:
                    if r.get("error"):
                        print(f"  Error: {r['error']}")
                        continue
                    print(f"  [{r.get('scope', '?')}/{r.get('kind', '?')}] {r.get('text', '')[:50]}...")
                    print(f"       ID: {r.get('knowledge_id', '?')}")

        elif kaction == "reject":
            if not args.id:
                print("Error: --id required for reject", file=sys.stderr)
                sys.exit(1)
            reason = args.reason or "Not applicable"
            result = reject_knowledge(args.id, reason)
            if result.get("error"):
                print(f"Error: {result['error']}", file=sys.stderr)
                sys.exit(1)
            print(f"REJECTED: {args.id}")
            print(f"  Reason: {reason}")

        elif kaction == "export":
            if not args.id:
                print("Error: --id (memory_id) required for export", file=sys.stderr)
                sys.exit(1)
            result = export_memory_to_global(args.id, kind=args.kind, scope=args.scope)
            if result.get("error"):
                print(f"Error: {result['error']}", file=sys.stderr)
                sys.exit(1)
            print(f"EXPORTED memory {args.id} → knowledge {result['knowledge_id']}")
            print(f"  Scope: {args.scope} | Kind: {args.kind}")

    elif args.action == "memstats":
        from llm_tools import summarize_session_history, vector_collection_stats
        from knowledge_store import get_knowledge_stats

        print("MEMORY STATISTICS\n")

        # Session memory
        session = summarize_session_history()
        if session.get("error"):
            print(f"  Session: Error - {session['error']}")
        else:
            print(f"  Session: {session.get('session_id', '?')}")
            print(f"    Memories: {session.get('total_memories', 0)}")
            by_kind = session.get('by_kind', {})
            if by_kind:
                print(f"    By kind: {by_kind}")

        # Knowledge stats
        print()
        kstats = get_knowledge_stats()
        if kstats.get("error"):
            print(f"  Knowledge: Error - {kstats['error']}")
        else:
            print(f"  Knowledge: {kstats.get('total_knowledge', 0)} total")
            print(f"    By scope: {kstats.get('by_scope', {})}")

        # Vector stats
        print()
        vstats = vector_collection_stats()
        if vstats.get("error"):
            print(f"  Vectors: Error - {vstats['error']}")
        else:
            print("  Vector collections:")
            for name, stats in vstats.items():
                if isinstance(stats, dict):
                    print(f"    {name}: {stats.get('count', '?')} embeddings")

    elif args.action == "identity":
        from llm_tools import get_current_context
        ctx = get_current_context()
        if ctx.get("error"):
            print(f"Error: {ctx['error']}", file=sys.stderr)
            sys.exit(1)
        print("IDENTITY / CONTEXT\n")
        print(f"  Instance ID:  {ctx.get('instance_id', '?')}")
        print(f"  Session ID:   {ctx.get('session_id', '?')}")
        print(f"  User ID:      {ctx.get('user_id', '?')}")
        print(f"  Scope:        {ctx.get('scope', '?')}")
        print(f"  Signature:    {ctx.get('context_signature', '?')}")
        print(f"  Tags:         {ctx.get('tags', [])}")
        print(f"  Started:      {ctx.get('started_at', '?')}")

    elif args.action == "should":
        from llm_tools import should_save, should_promote, should_reject

        if args.__dict__.get('action') == "should":
            # Get the sub-action
            import sys as _sys
            if len(_sys.argv) > 2:
                saction = _sys.argv[2]
            else:
                print("Error: specify save, promote, or reject", file=sys.stderr)
                sys.exit(1)

            target = _sys.argv[3] if len(_sys.argv) > 3 else args.target

            if saction == "save":
                result = should_save(target)
                verdict = "✓ SAVE" if result.get("should_save") else "✗ SKIP"
                print(f"{verdict}: {result.get('reason', '?')}")
                if result.get("most_similar"):
                    sim = result["most_similar"]
                    print(f"  Most similar: {sim.get('memory_id', '?')} ({sim.get('similarity', 0):.0%})")

            elif saction == "promote":
                result = should_promote(target)
                verdict = "✓ PROMOTE" if result.get("should_promote") else "✗ KEEP"
                print(f"{verdict}: {result.get('reason', '?')}")
                if result.get("should_promote"):
                    print(f"  Suggested scope: {result.get('suggested_scope', '?')}")
                    print(f"  Suggested kind: {result.get('suggested_kind', '?')}")

            elif saction == "reject":
                result = should_reject(target)
                verdict = "✓ REJECT" if result.get("should_reject") else "✗ KEEP"
                print(f"{verdict}: {result.get('reason', '?')}")
                print(f"  Context compatible: {result.get('context_compatible', '?')}")


if __name__ == "__main__":
    main()
