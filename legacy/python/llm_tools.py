# ═══════════════════════════════════════════════════════════════════════════════
# LLM Tools: Complete API for Memory-Augmented Agents
# ═══════════════════════════════════════════════════════════════════════════════
#
# This module provides the COMPLETE toolset for an LLM to manage:
# - Session memory (private cognitive space)
# - Shared knowledge (instance/global)
# - Context identity
# - Traceability
# - Vector operations
# - Metacognitive helpers
#
# Designed for small LLMs (Qwen3-3B, etc.) to function as memory-augmented agents.

from typing import Optional, Any
import json

# ═══════════════════════════════════════════════════════════════════════════════
# A. SESSION MEMORY (Private per-session)
# ═══════════════════════════════════════════════════════════════════════════════

def add_session_note(
    text: str,
    kind: str = "note",
    importance: int = 2,
    tags: Optional[list[str]] = None
) -> dict:
    """
    Store a note in the current session's private memory.

    Use this to remember:
    - Observations and findings
    - Intermediate conclusions
    - Decisions made
    - Important context

    Args:
        text: What to remember (must be non-empty)
        kind: Type - "note", "summary", "decision", "result", "observation"
        importance: 1-5 (1=trivial, 5=critical)
        tags: Optional categorization tags

    Returns:
        {"memory_id": str, "status": "ok"} or {"error": str}

    Example:
        add_session_note("User prefers verbose output", kind="observation", importance=3)
    """
    # Validate text before calling
    if not text or not text.strip():
        return {"error": "Text cannot be empty"}

    from session_memory import add_session_note as _add
    try:
        mid = _add(text, kind=kind, importance=importance, tags=tags)
        if not mid:
            return {"error": "Failed to store note (validation failed)"}
        return {"memory_id": mid, "status": "ok"}
    except Exception as e:
        return {"error": str(e)}


def recall_session_memory(
    query: str,
    n_results: int = 5,
    kind: Optional[str] = None,
    min_importance: int = 1
) -> list[dict]:
    """
    Search your private session memory.

    SEARCH HERE FIRST before querying global knowledge.
    This is YOUR memory for THIS session - fast and isolated.

    Args:
        query: What to search for (semantic search)
        n_results: Max results to return
        kind: Filter by type ("note", "summary", etc.)
        min_importance: Minimum importance level (1-5)

    Returns:
        List of {memory_id, text, kind, importance, similarity}

    Example:
        recall_session_memory("docker configuration")
    """
    from session_memory import recall_session_memory as _recall
    try:
        return _recall(query, n_results=n_results, kind=kind, min_importance=min_importance)
    except Exception as e:
        return [{"error": str(e)}]


def list_session_memory() -> list[dict]:
    """
    Get ALL memories from the current session.

    Use for:
    - Auditing what you've learned
    - Generating session summaries
    - Exporting before session ends

    Returns:
        List of all memory entries for this session
    """
    from session_memory import dump_session_memory
    try:
        return dump_session_memory()
    except Exception as e:
        return [{"error": str(e)}]


def delete_session_memory(memory_id: str) -> dict:
    """
    Delete a specific memory from session.

    Use when you realize a conclusion was wrong.

    Args:
        memory_id: The ID of the memory to delete

    Returns:
        {"status": "ok"} or {"error": str}
    """
    from session_memory import delete_session_memory as _delete
    try:
        success = _delete(memory_id)
        return {"status": "ok" if success else "failed"}
    except Exception as e:
        return {"error": str(e)}


def summarize_session_history() -> dict:
    """
    Generate a compressed summary of the entire session.

    Use for:
    - Checkpoint before complex operation
    - Context reconstruction
    - End-of-session wrap-up

    Returns:
        {
            "session_id": str,
            "total_memories": int,
            "by_kind": {kind: count},
            "memories": [list of all memories]
        }
    """
    from session_memory import get_session_memory_stats, dump_session_memory
    from context import get_current_context
    try:
        ctx = get_current_context()
        stats = get_session_memory_stats()
        memories = dump_session_memory()
        return {
            "session_id": ctx.session_id,
            "instance_id": ctx.instance_id,
            "context_signature": ctx.context_signature,
            "total_memories": stats.get("total_memories", 0),
            "by_kind": stats.get("by_kind", {}),
            "memories": memories,
        }
    except Exception as e:
        return {"error": str(e)}


# ═══════════════════════════════════════════════════════════════════════════════
# B. SHARED KNOWLEDGE (Instance/Global)
# ═══════════════════════════════════════════════════════════════════════════════

def store_knowledge(
    text: str,
    kind: str,
    scope: str = "session"
) -> dict:
    """
    Store knowledge that can be shared across sessions.

    Args:
        text: The knowledge content (must be non-empty)
        kind: Type - "error", "fix", "rule", "pattern", "plan", "insight", "fact"
        scope: Visibility level:
            - "session": Only this session
            - "instance": Same container/deployment
            - "global": Available everywhere

    Returns:
        {"knowledge_id": str, "status": "ok"} or {"error": str}

    Example:
        store_knowledge(
            "SELinux requires label:disable for docker.sock",
            kind="rule",
            scope="global"
        )
    """
    # Validate text before calling
    if not text or not text.strip():
        return {"error": "Text cannot be empty"}

    from knowledge_store import store_knowledge as _store
    try:
        kid = _store(text, kind=kind, scope=scope)
        if not kid:
            return {"error": "Failed to store knowledge (validation failed)"}
        return {"knowledge_id": kid, "status": "ok"}
    except Exception as e:
        return {"error": str(e)}


def query_knowledge(
    query: str,
    n_results: int = 5,
    level: str = "all",
    kind: Optional[str] = None
) -> list[dict]:
    """
    Search shared knowledge with multi-level strategy.

    Search order (when level="all"):
    1. Session knowledge (your session's shared items)
    2. Instance knowledge (same container)
    3. Global knowledge (all sources)

    Automatically filters out rejected knowledge.

    Args:
        query: What to search for
        n_results: Max results
        level: "session", "instance", "global", or "all" (cascading)
        kind: Filter by type

    Returns:
        List of {knowledge_id, text, kind, scope, similarity, source_level}

    Example:
        query_knowledge("docker permissions fedora", level="all")
    """
    from knowledge_store import query_knowledge as _query
    try:
        return _query(query, n_results=n_results, level=level, kind=kind)
    except Exception as e:
        return [{"error": str(e)}]


def reject_knowledge(knowledge_id: str, reason: str) -> dict:
    """
    Mark knowledge as NOT APPLICABLE for this session.

    Use when knowledge from another session doesn't apply to your context.
    This prevents it from appearing in future searches FOR THIS SESSION.

    Args:
        knowledge_id: ID of knowledge to reject
        reason: Why it doesn't apply (e.g., "different dataset", "wrong OS")

    Returns:
        {"status": "ok"} or {"error": str}

    Example:
        reject_knowledge("k-abc123", "This is for Windows, I'm on Linux")
    """
    from knowledge_store import reject_knowledge as _reject
    try:
        success = _reject(knowledge_id, reason)
        return {"status": "ok" if success else "failed"}
    except Exception as e:
        return {"error": str(e)}


def export_memory_to_global(
    memory_id: str,
    kind: str = "rule",
    scope: str = "global"
) -> dict:
    """
    Promote a session memory to shared knowledge.

    Use when you discover something that would help OTHER sessions.

    Args:
        memory_id: ID of the session memory to promote
        kind: Type for the new knowledge entry
        scope: "instance" or "global"

    Returns:
        {"knowledge_id": str, "status": "ok"} or {"error": str}

    Example:
        # Found a useful fix in session, promote it globally
        export_memory_to_global("m-xyz789", kind="fix", scope="global")
    """
    from knowledge_store import export_memory_to_knowledge
    try:
        kid = export_memory_to_knowledge(memory_id, kind=kind, scope=scope)
        if kid:
            return {"knowledge_id": kid, "status": "ok"}
        return {"error": "Export failed"}
    except Exception as e:
        return {"error": str(e)}


def list_global_knowledge(kind: Optional[str] = None, limit: int = 50) -> list[dict]:
    """
    List all global knowledge entries.

    Use to see what rules/patterns/fixes are available globally.

    Args:
        kind: Filter by type (None = all types)
        limit: Max entries to return

    Returns:
        List of knowledge entries
    """
    from knowledge_store import list_all_knowledge
    try:
        return list_all_knowledge(kind=kind, limit=limit)
    except Exception as e:
        return [{"error": str(e)}]


# ═══════════════════════════════════════════════════════════════════════════════
# C. CONTEXT & IDENTITY
# ═══════════════════════════════════════════════════════════════════════════════

def get_current_context() -> dict:
    """
    Get the current session's identity context.

    Returns:
        {
            "instance_id": container/deployment ID,
            "session_id": unique session ID,
            "user_id": user identifier,
            "scope": current scope,
            "context_signature": environment fingerprint,
            "tags": list of context tags
        }

    The context_signature is used to determine if knowledge from
    other sessions is compatible with your current environment.
    """
    from context import get_current_context as _get
    try:
        ctx = _get()
        return {
            "instance_id": ctx.instance_id,
            "session_id": ctx.session_id,
            "user_id": ctx.user_id,
            "scope": ctx.scope,
            "context_signature": ctx.context_signature,
            "tags": ctx.tags,
            "started_at": ctx.started_at,
        }
    except Exception as e:
        return {"error": str(e)}


def set_context_tags(tags: list[str]) -> dict:
    """
    Update the current context's semantic tags.

    Use to mark:
    - Current dataset being processed
    - Phase of task (exploration, implementation, testing)
    - Environment characteristics

    Args:
        tags: New list of tags

    Returns:
        {"status": "ok", "tags": list} or {"error": str}

    Example:
        set_context_tags(["UNSW-NB15", "training-phase", "gpu-enabled"])
    """
    from context import get_current_context
    try:
        ctx = get_current_context()
        ctx.tags = tags
        return {"status": "ok", "tags": ctx.tags}
    except Exception as e:
        return {"error": str(e)}


# ═══════════════════════════════════════════════════════════════════════════════
# D. TRACEABILITY / AUDIT
# ═══════════════════════════════════════════════════════════════════════════════

def graph_trace_knowledge(knowledge_id: str) -> dict:
    """
    Get full provenance trace for a piece of knowledge.

    Returns:
        {
            "knowledge_id": str,
            "creator_session": who created it,
            "used_by": sessions that used it,
            "rejected_by": sessions that rejected it,
            "promoted_from": original memory_id if promoted,
            "context_signature": original context
        }
    """
    from database import Database
    try:
        db = Database()
        cypher = """
        MATCH (k:Knowledge {knowledge_id: $kid})
        OPTIONAL MATCH (creator:Session)-[:CREATED]->(k)
        OPTIONAL MATCH (user:Session)-[:USED]->(k)
        OPTIONAL MATCH (rejector:Session)-[:REJECTED]->(k)
        OPTIONAL MATCH (k)-[:PROMOTED_FROM]->(m:Memory)
        RETURN k.knowledge_id AS id,
               k.kind AS kind,
               k.scope AS scope,
               k.context_signature AS ctx_sig,
               creator.session_id AS creator,
               collect(DISTINCT user.session_id) AS used_by,
               collect(DISTINCT rejector.session_id) AS rejected_by,
               m.memory_id AS promoted_from
        """
        results = db.execute_query(cypher, {"kid": knowledge_id})
        db.close()
        if results:
            r = results[0]
            return {
                "knowledge_id": r["id"],
                "kind": r["kind"],
                "scope": r["scope"],
                "context_signature": r["ctx_sig"],
                "creator_session": r["creator"],
                "used_by": [s for s in r["used_by"] if s],
                "rejected_by": [s for s in r["rejected_by"] if s],
                "promoted_from": r["promoted_from"],
            }
        return {"error": "Knowledge not found"}
    except Exception as e:
        return {"error": str(e)}


def graph_trace_session(session_id: Optional[str] = None) -> dict:
    """
    Get activity trace for a session.

    Args:
        session_id: Session to trace (None = current session)

    Returns:
        {
            "session_id": str,
            "memories_created": count,
            "knowledge_created": count,
            "knowledge_used": list of IDs,
            "knowledge_rejected": list of IDs,
            "knowledge_exported": list of IDs
        }
    """
    from database import Database
    from context import get_current_context
    try:
        if session_id is None:
            session_id = get_current_context().session_id

        db = Database()
        cypher = """
        MATCH (s:Session {session_id: $sid})
        OPTIONAL MATCH (s)-[:HAS_MEMORY]->(m:Memory)
        OPTIONAL MATCH (s)-[:CREATED]->(kc:Knowledge)
        OPTIONAL MATCH (s)-[:USED]->(ku:Knowledge)
        OPTIONAL MATCH (s)-[:REJECTED]->(kr:Knowledge)
        OPTIONAL MATCH (s)-[:EXPORTED]->(ke:Knowledge)
        RETURN s.session_id AS session_id,
               count(DISTINCT m) AS memories_created,
               count(DISTINCT kc) AS knowledge_created,
               collect(DISTINCT ku.knowledge_id) AS knowledge_used,
               collect(DISTINCT kr.knowledge_id) AS knowledge_rejected,
               collect(DISTINCT ke.knowledge_id) AS knowledge_exported
        """
        results = db.execute_query(cypher, {"sid": session_id})
        db.close()
        if results:
            r = results[0]
            return {
                "session_id": r["session_id"],
                "memories_created": r["memories_created"],
                "knowledge_created": r["knowledge_created"],
                "knowledge_used": [k for k in r["knowledge_used"] if k],
                "knowledge_rejected": [k for k in r["knowledge_rejected"] if k],
                "knowledge_exported": [k for k in r["knowledge_exported"] if k],
            }
        return {"error": "Session not found"}
    except Exception as e:
        return {"error": str(e)}


# ═══════════════════════════════════════════════════════════════════════════════
# E. VECTOR OPERATIONS (Low-level ChromaDB access)
# ═══════════════════════════════════════════════════════════════════════════════

def vector_store_embedding(
    doc_id: str,
    text: str,
    metadata: Optional[dict] = None,
    collection: str = "urp_embeddings"
) -> dict:
    """
    Store text with its embedding in ChromaDB.

    Low-level function. Prefer add_session_note or store_knowledge.

    Args:
        doc_id: Unique ID
        text: Text to embed
        metadata: Optional metadata dict
        collection: ChromaDB collection name

    Returns:
        {"status": "ok"} or {"error": str}
    """
    from brain_cortex import store_embedding
    try:
        success = store_embedding(doc_id, text, metadata, collection)
        return {"status": "ok" if success else "failed"}
    except Exception as e:
        return {"error": str(e)}


def vector_query(
    text: str,
    n_results: int = 5,
    collection: str = "urp_embeddings",
    where: Optional[dict] = None
) -> list[dict]:
    """
    Direct semantic search in ChromaDB.

    Low-level function. Prefer recall_session_memory or query_knowledge.

    Args:
        text: Query text
        n_results: Max results
        collection: ChromaDB collection name
        where: Optional metadata filter

    Returns:
        List of {id, document, metadata, distance, similarity}
    """
    from brain_cortex import query_similar
    try:
        return query_similar(text, n_results, collection, where)
    except Exception as e:
        return [{"error": str(e)}]


def vector_delete(doc_id: str, collection: str = "urp_embeddings") -> dict:
    """
    Delete a document from ChromaDB.

    Args:
        doc_id: Document ID to delete
        collection: ChromaDB collection name

    Returns:
        {"status": "ok"} or {"error": str}
    """
    from brain_cortex import get_collection
    try:
        coll = get_collection(collection)
        if coll:
            coll.delete(ids=[doc_id])
            return {"status": "ok"}
        return {"error": "Collection not found"}
    except Exception as e:
        return {"error": str(e)}


def vector_collection_stats() -> dict:
    """
    Get stats for all vector collections.

    Returns:
        {
            "urp_embeddings": {"count": N},
            "session_memory": {"count": N},
            "urp_knowledge": {"count": N}
        }
    """
    from brain_cortex import get_all_collections_stats
    try:
        return get_all_collections_stats()
    except Exception as e:
        return {"error": str(e)}


# ═══════════════════════════════════════════════════════════════════════════════
# F. METACOGNITIVE HELPERS
# ═══════════════════════════════════════════════════════════════════════════════

def should_save(note: str, threshold: float = 0.8) -> dict:
    """
    Evaluate if a note is worth saving to session memory.

    Checks:
    - Is it too similar to existing memories? (redundancy)
    - Is it substantial enough? (length/content)
    - Does it add new information?

    Args:
        note: The text to evaluate
        threshold: Similarity threshold (0.8 = reject if >80% similar)

    Returns:
        {
            "should_save": bool,
            "reason": str,
            "most_similar": {id, similarity} or None
        }

    Example:
        result = should_save("Docker needs root permissions")
        if result["should_save"]:
            add_session_note("Docker needs root permissions")
    """
    from metacognitive import evaluate_save
    try:
        return evaluate_save(note, threshold)
    except Exception as e:
        return {"should_save": True, "reason": f"Error evaluating: {e}", "most_similar": None}


def should_promote(memory_id: str) -> dict:
    """
    Evaluate if a session memory should be promoted to global knowledge.

    Checks:
    - Is it general enough? (not too session-specific)
    - Has similar knowledge already been promoted?
    - Is it stable/confirmed information?

    Args:
        memory_id: ID of the session memory to evaluate

    Returns:
        {
            "should_promote": bool,
            "reason": str,
            "suggested_scope": "instance" or "global",
            "suggested_kind": str
        }

    Example:
        result = should_promote("m-abc123")
        if result["should_promote"]:
            export_memory_to_global("m-abc123", scope=result["suggested_scope"])
    """
    from metacognitive import evaluate_promote
    try:
        return evaluate_promote(memory_id)
    except Exception as e:
        return {"should_promote": False, "reason": f"Error evaluating: {e}"}


def should_reject(knowledge_id: str) -> dict:
    """
    Evaluate if knowledge from another session should be rejected.

    Checks:
    - Is the context compatible?
    - Does it conflict with current session observations?
    - Is it relevant to current task?

    Args:
        knowledge_id: ID of the knowledge to evaluate

    Returns:
        {
            "should_reject": bool,
            "reason": str,
            "context_compatible": bool
        }

    Example:
        result = should_reject("k-xyz789")
        if result["should_reject"]:
            reject_knowledge("k-xyz789", result["reason"])
    """
    from metacognitive import evaluate_reject
    try:
        return evaluate_reject(knowledge_id)
    except Exception as e:
        return {"should_reject": False, "reason": f"Error evaluating: {e}"}


# ═══════════════════════════════════════════════════════════════════════════════
# TOOL REGISTRY (for LLM function calling)
# ═══════════════════════════════════════════════════════════════════════════════

TOOLS = {
    # A. Session Memory
    "add_session_note": add_session_note,
    "recall_session_memory": recall_session_memory,
    "list_session_memory": list_session_memory,
    "delete_session_memory": delete_session_memory,
    "summarize_session_history": summarize_session_history,

    # B. Shared Knowledge
    "store_knowledge": store_knowledge,
    "query_knowledge": query_knowledge,
    "reject_knowledge": reject_knowledge,
    "export_memory_to_global": export_memory_to_global,
    "list_global_knowledge": list_global_knowledge,

    # C. Context
    "get_current_context": get_current_context,
    "set_context_tags": set_context_tags,

    # D. Traceability
    "graph_trace_knowledge": graph_trace_knowledge,
    "graph_trace_session": graph_trace_session,

    # E. Vectors
    "vector_store_embedding": vector_store_embedding,
    "vector_query": vector_query,
    "vector_delete": vector_delete,
    "vector_collection_stats": vector_collection_stats,

    # F. Metacognitive
    "should_save": should_save,
    "should_promote": should_promote,
    "should_reject": should_reject,
}


def call_tool(name: str, **kwargs) -> Any:
    """
    Call a tool by name with arguments.

    For LLM function calling integration.

    Example:
        result = call_tool("add_session_note", text="Hello", importance=3)
    """
    if name not in TOOLS:
        return {"error": f"Unknown tool: {name}"}
    return TOOLS[name](**kwargs)


def list_tools() -> list[str]:
    """Return list of available tool names."""
    return list(TOOLS.keys())


def get_tool_help(name: str) -> str:
    """Get docstring for a tool."""
    if name not in TOOLS:
        return f"Unknown tool: {name}"
    return TOOLS[name].__doc__ or "No documentation"


# ═══════════════════════════════════════════════════════════════════════════════
# CLI Interface
# ═══════════════════════════════════════════════════════════════════════════════

if __name__ == "__main__":
    import sys

    if len(sys.argv) < 2:
        print("LLM Memory Tools")
        print("================")
        print("\nAvailable tools:")
        for name in list_tools():
            print(f"  {name}")
        print("\nUsage: python llm_tools.py <tool_name> [json_args]")
        print("Example: python llm_tools.py add_session_note '{\"text\": \"Hello\", \"importance\": 3}'")
        sys.exit(0)

    tool_name = sys.argv[1]

    if tool_name == "--help" or tool_name == "-h":
        if len(sys.argv) > 2:
            print(get_tool_help(sys.argv[2]))
        else:
            for name in list_tools():
                print(f"\n=== {name} ===")
                print(get_tool_help(name))
        sys.exit(0)

    kwargs = {}
    if len(sys.argv) > 2:
        try:
            kwargs = json.loads(sys.argv[2])
        except json.JSONDecodeError:
            # Try as simple key=value
            for arg in sys.argv[2:]:
                if "=" in arg:
                    k, v = arg.split("=", 1)
                    kwargs[k] = v

    result = call_tool(tool_name, **kwargs)
    print(json.dumps(result, indent=2, default=str))
