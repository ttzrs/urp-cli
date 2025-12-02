# ═══════════════════════════════════════════════════════════════════════════════
# Metacognitive Module: Self-Evaluation for Memory Decisions
# ═══════════════════════════════════════════════════════════════════════════════
#
# Provides helper functions that help the LLM decide:
# - Should I save this note? (redundancy check)
# - Should I promote this to global? (generality check)
# - Should I reject this knowledge? (compatibility check)
#
# These are NOT autonomous decisions - they provide RECOMMENDATIONS.
# The LLM makes the final call.

from typing import Optional
from context import get_current_context, is_context_compatible, URPContext


def evaluate_save(
    note: str,
    threshold: float = 0.8,
    ctx: Optional[URPContext] = None,
) -> dict:
    """
    Evaluate whether a note is worth saving to session memory.

    Checks:
    1. Length/substance check
    2. Redundancy check (similarity to existing memories)

    Args:
        note: The text to evaluate
        threshold: Similarity threshold above which we consider redundant
        ctx: Optional context override

    Returns:
        {
            "should_save": bool,
            "reason": str,
            "most_similar": {"memory_id": str, "similarity": float} or None
        }
    """
    if ctx is None:
        ctx = get_current_context()

    # 1. Substance check
    if not note or len(note.strip()) < 10:
        return {
            "should_save": False,
            "reason": "Note too short (< 10 chars)",
            "most_similar": None,
        }

    # Extremely long notes might be better as knowledge
    if len(note) > 2000:
        return {
            "should_save": True,
            "reason": "Consider storing as knowledge instead (very long)",
            "most_similar": None,
        }

    # 2. Redundancy check
    try:
        from session_memory import recall_session_memory
        similar = recall_session_memory(note, n_results=1, ctx=ctx)

        if similar:
            best = similar[0]
            if best.get("similarity", 0) >= threshold:
                return {
                    "should_save": False,
                    "reason": f"Too similar to existing memory ({best['similarity']:.0%})",
                    "most_similar": {
                        "memory_id": best["memory_id"],
                        "similarity": best["similarity"],
                        "text_preview": best.get("text", "")[:100],
                    },
                }

            # Found something but not too similar
            return {
                "should_save": True,
                "reason": "Novel enough to save",
                "most_similar": {
                    "memory_id": best["memory_id"],
                    "similarity": best["similarity"],
                },
            }
    except Exception as e:
        # If we can't check, default to save
        return {
            "should_save": True,
            "reason": f"Could not check redundancy: {e}",
            "most_similar": None,
        }

    # No similar memories found - definitely save
    return {
        "should_save": True,
        "reason": "No similar memories found",
        "most_similar": None,
    }


def evaluate_promote(
    memory_id: str,
    ctx: Optional[URPContext] = None,
) -> dict:
    """
    Evaluate whether a session memory should be promoted to global knowledge.

    Checks:
    1. Does it exist and is it substantial?
    2. Is it general enough (not session-specific)?
    3. Does similar global knowledge already exist?

    Args:
        memory_id: ID of the session memory to evaluate
        ctx: Optional context override

    Returns:
        {
            "should_promote": bool,
            "reason": str,
            "suggested_scope": "instance" or "global",
            "suggested_kind": str
        }
    """
    if ctx is None:
        ctx = get_current_context()

    try:
        from database import Database
        db = Database()

        # Get the memory
        cypher = """
        MATCH (s:Session {session_id: $session_id})-[:HAS_MEMORY]->(m:Memory {memory_id: $memory_id})
        RETURN m.text AS text, m.kind AS kind, m.importance AS importance, m.tags AS tags
        """
        results = db.execute_query(cypher, {
            "session_id": ctx.session_id,
            "memory_id": memory_id,
        })
        db.close()

        if not results:
            return {
                "should_promote": False,
                "reason": "Memory not found in current session",
                "suggested_scope": None,
                "suggested_kind": None,
            }

        mem = results[0]
        text = mem["text"]
        kind = mem.get("kind", "note")
        importance = mem.get("importance", 2)
        tags = mem.get("tags", [])

    except Exception as e:
        return {
            "should_promote": False,
            "reason": f"Error fetching memory: {e}",
            "suggested_scope": None,
            "suggested_kind": None,
        }

    # 1. Importance check
    if importance < 3:
        return {
            "should_promote": False,
            "reason": f"Importance too low ({importance}/5) for promotion",
            "suggested_scope": None,
            "suggested_kind": None,
        }

    # 2. Length check
    if len(text) < 20:
        return {
            "should_promote": False,
            "reason": "Content too short for useful knowledge",
            "suggested_scope": None,
            "suggested_kind": None,
        }

    # 3. Check for session-specific markers
    session_specific_markers = [
        ctx.session_id,
        "this session",
        "right now",
        "just now",
        "current run",
    ]
    for marker in session_specific_markers:
        if marker.lower() in text.lower():
            return {
                "should_promote": False,
                "reason": f"Contains session-specific reference: '{marker}'",
                "suggested_scope": None,
                "suggested_kind": None,
            }

    # 4. Check for existing similar global knowledge
    try:
        from knowledge_store import query_knowledge
        similar = query_knowledge(text, n_results=1, level="global", ctx=ctx)

        if similar:
            best = similar[0]
            if best.get("similarity", 0) >= 0.85:
                return {
                    "should_promote": False,
                    "reason": f"Similar global knowledge exists ({best['similarity']:.0%} match)",
                    "suggested_scope": None,
                    "suggested_kind": None,
                }
    except Exception:
        pass  # Continue if check fails

    # 5. Determine suggested scope and kind
    suggested_scope = "global"
    suggested_kind = _infer_knowledge_kind(text, kind, tags)

    # Instance-specific indicators
    instance_markers = ["this container", "this instance", "local config", "dev environment"]
    for marker in instance_markers:
        if marker.lower() in text.lower():
            suggested_scope = "instance"
            break

    return {
        "should_promote": True,
        "reason": "Memory appears general and useful enough for promotion",
        "suggested_scope": suggested_scope,
        "suggested_kind": suggested_kind,
    }


def _infer_knowledge_kind(text: str, original_kind: str, tags: list) -> str:
    """Infer the best knowledge kind based on content."""
    text_lower = text.lower()

    # Error patterns
    if any(w in text_lower for w in ["error", "exception", "failed", "traceback"]):
        return "error"

    # Fix patterns
    if any(w in text_lower for w in ["fix", "solved", "resolved", "workaround"]):
        return "fix"

    # Rule patterns
    if any(w in text_lower for w in ["always", "never", "must", "should", "requires"]):
        return "rule"

    # Pattern patterns
    if any(w in text_lower for w in ["pattern", "approach", "technique", "method"]):
        return "pattern"

    # Map from memory kinds
    kind_map = {
        "decision": "rule",
        "summary": "insight",
        "observation": "fact",
        "result": "fact",
        "note": "insight",
    }
    return kind_map.get(original_kind, "insight")


def evaluate_reject(
    knowledge_id: str,
    ctx: Optional[URPContext] = None,
) -> dict:
    """
    Evaluate whether knowledge from another session should be rejected.

    Checks:
    1. Context compatibility
    2. Already rejected?
    3. Relevance to current context

    Args:
        knowledge_id: ID of the knowledge to evaluate
        ctx: Optional context override

    Returns:
        {
            "should_reject": bool,
            "reason": str,
            "context_compatible": bool
        }
    """
    if ctx is None:
        ctx = get_current_context()

    try:
        from knowledge_store import get_rejected_ids

        # Already rejected?
        rejected = get_rejected_ids(ctx)
        if knowledge_id in rejected:
            return {
                "should_reject": False,
                "reason": "Already rejected",
                "context_compatible": None,
            }

    except Exception:
        pass

    # Get knowledge details
    try:
        from brain_cortex import get_collection

        collection = get_collection("urp_knowledge")
        if collection is None:
            return {
                "should_reject": False,
                "reason": "Cannot access knowledge store",
                "context_compatible": None,
            }

        results = collection.get(ids=[knowledge_id], include=["metadatas", "documents"])
        if not results["ids"]:
            return {
                "should_reject": False,
                "reason": "Knowledge not found",
                "context_compatible": None,
            }

        meta = results["metadatas"][0]
        text = results["documents"][0]
        knowledge_ctx_sig = meta.get("context_signature", "")

    except Exception as e:
        return {
            "should_reject": False,
            "reason": f"Error fetching knowledge: {e}",
            "context_compatible": None,
        }

    # 1. Context compatibility check
    context_compatible = is_context_compatible(knowledge_ctx_sig, ctx.context_signature)

    if not context_compatible:
        return {
            "should_reject": True,
            "reason": f"Incompatible context: '{knowledge_ctx_sig}' vs '{ctx.context_signature}'",
            "context_compatible": False,
        }

    # 2. Check for obvious irrelevance markers
    irrelevance_markers = _get_irrelevance_markers(ctx)
    for marker, reason in irrelevance_markers:
        if marker.lower() in text.lower():
            return {
                "should_reject": True,
                "reason": reason,
                "context_compatible": True,
            }

    # No reason to reject
    return {
        "should_reject": False,
        "reason": "Knowledge appears compatible and potentially relevant",
        "context_compatible": True,
    }


def _get_irrelevance_markers(ctx: URPContext) -> list[tuple[str, str]]:
    """Get markers that indicate knowledge is irrelevant for current context."""
    markers = []

    # OS-specific
    if "linux" in ctx.context_signature.lower() or "fedora" in ctx.context_signature.lower():
        markers.extend([
            ("windows", "Windows-specific, we're on Linux"),
            ("macos", "macOS-specific, we're on Linux"),
            ("powershell", "PowerShell-specific, we're on Linux"),
        ])

    if "windows" in ctx.context_signature.lower():
        markers.extend([
            ("systemd", "systemd-specific, we're on Windows"),
            ("apt-get", "apt-specific, we're on Windows"),
            ("yum", "yum-specific, we're on Windows"),
        ])

    # Container-specific
    if "docker" in ctx.context_signature.lower():
        markers.append(("podman-specific", "Podman-specific, we're using Docker"))

    if "podman" in ctx.context_signature.lower():
        markers.append(("docker-specific", "Docker-specific, we're using Podman"))

    return markers


# ═══════════════════════════════════════════════════════════════════════════════
# Utility Functions
# ═══════════════════════════════════════════════════════════════════════════════

def calculate_memory_importance(text: str) -> int:
    """
    Suggest an importance level for a piece of text.

    Returns:
        1-5 importance score
    """
    text_lower = text.lower()

    # Critical indicators (5)
    if any(w in text_lower for w in ["critical", "breaking", "security", "data loss", "production"]):
        return 5

    # High importance (4)
    if any(w in text_lower for w in ["important", "error", "failed", "exception", "must"]):
        return 4

    # Medium importance (3)
    if any(w in text_lower for w in ["note", "remember", "config", "setting", "requires"]):
        return 3

    # Lower importance (2)
    if any(w in text_lower for w in ["observation", "noticed", "seems", "might"]):
        return 2

    # Default (2)
    return 2


def suggest_memory_kind(text: str) -> str:
    """
    Suggest the best memory kind for a piece of text.

    Returns:
        "note", "summary", "decision", "result", or "observation"
    """
    text_lower = text.lower()

    if any(w in text_lower for w in ["decided", "chose", "will use", "going with"]):
        return "decision"

    if any(w in text_lower for w in ["in summary", "overall", "conclusion", "found that"]):
        return "summary"

    if any(w in text_lower for w in ["result:", "output:", "returned", "produced"]):
        return "result"

    if any(w in text_lower for w in ["noticed", "observed", "saw that", "appears"]):
        return "observation"

    return "note"


# ═══════════════════════════════════════════════════════════════════════════════
# CLI / Debug
# ═══════════════════════════════════════════════════════════════════════════════

if __name__ == "__main__":
    import json
    import sys

    if len(sys.argv) < 2:
        print("Metacognitive Evaluation Tools")
        print("===============================")
        print("\nUsage:")
        print("  python metacognitive.py should_save <text>")
        print("  python metacognitive.py should_promote <memory_id>")
        print("  python metacognitive.py should_reject <knowledge_id>")
        print("  python metacognitive.py suggest_importance <text>")
        print("  python metacognitive.py suggest_kind <text>")
        sys.exit(0)

    cmd = sys.argv[1]

    if cmd == "should_save" and len(sys.argv) > 2:
        result = evaluate_save(sys.argv[2])
        print(json.dumps(result, indent=2))

    elif cmd == "should_promote" and len(sys.argv) > 2:
        result = evaluate_promote(sys.argv[2])
        print(json.dumps(result, indent=2))

    elif cmd == "should_reject" and len(sys.argv) > 2:
        result = evaluate_reject(sys.argv[2])
        print(json.dumps(result, indent=2))

    elif cmd == "suggest_importance" and len(sys.argv) > 2:
        score = calculate_memory_importance(sys.argv[2])
        print(f"Suggested importance: {score}/5")

    elif cmd == "suggest_kind" and len(sys.argv) > 2:
        kind = suggest_memory_kind(sys.argv[2])
        print(f"Suggested kind: {kind}")

    else:
        print(f"Unknown command: {cmd}")
        sys.exit(1)
