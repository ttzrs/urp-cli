# ═══════════════════════════════════════════════════════════════════════════════
# Session Memory: Private cognitive space for each URP session
# ═══════════════════════════════════════════════════════════════════════════════
#
# Each session has its own "mind" that accumulates:
# - Notes and observations
# - Summaries and conclusions
# - Intermediate results
# - Decisions made
#
# This memory is:
# - Isolated: doesn't contaminate global knowledge
# - Fast: first place to search
# - Traceable: auditable via Memgraph
# - Exportable: can promote to global knowledge

import os
import time
import uuid
from typing import Optional, Any
from dataclasses import dataclass

from context import URPContext, get_current_context
from database import Database

# ChromaDB imports (lazy)
_chroma_client = None
_CHROMA_PATH = os.getenv('URP_CHROMA_PATH', '/app/chroma')


def _get_chroma():
    """Lazy load ChromaDB client."""
    global _chroma_client
    if _chroma_client is None:
        try:
            import chromadb
            from chromadb.config import Settings
            os.makedirs(_CHROMA_PATH, exist_ok=True)
            _chroma_client = chromadb.PersistentClient(
                path=_CHROMA_PATH,
                settings=Settings(anonymized_telemetry=False)
            )
        except Exception as e:
            print(f"⚠️  ChromaDB not available: {e}")
            return None
    return _chroma_client


def _get_session_collection():
    """Get or create the session_memory collection."""
    client = _get_chroma()
    if client is None:
        return None
    try:
        return client.get_or_create_collection(
            name="session_memory",
            metadata={"hnsw:space": "cosine"}
        )
    except Exception as e:
        print(f"⚠️  Failed to get session_memory collection: {e}")
        return None


def _get_embedding(text: str) -> list[float]:
    """Get embedding for text (uses brain_cortex)."""
    try:
        from brain_cortex import get_embedding
        return get_embedding(text)
    except Exception:
        return []


@dataclass
class MemoryEntry:
    """A single memory entry."""
    memory_id: str
    kind: str  # "note", "summary", "decision", "result", "observation"
    text: str
    importance: int  # 1-5
    session_id: str
    instance_id: str
    created_at: str
    tags: list[str]


# ═══════════════════════════════════════════════════════════════════════════════
# Core Session Memory API
# ═══════════════════════════════════════════════════════════════════════════════

def add_session_note(
    text: str,
    kind: str = "note",
    importance: int = 2,
    tags: Optional[list[str]] = None,
    ctx: Optional[URPContext] = None,
) -> str:
    """
    Add a note/memory to the current session.

    Args:
        text: The content to remember
        kind: Type of memory ("note", "summary", "decision", "result", "observation")
        importance: 1-5 (1=low, 5=critical)
        tags: Optional tags for filtering
        ctx: Optional context override

    Returns:
        memory_id of the created memory, or empty string if validation fails
    """
    # Validate text
    if not text or not text.strip():
        return ""  # Reject empty/whitespace-only text

    # Clamp importance to valid range
    importance = max(1, min(5, importance))

    if ctx is None:
        ctx = get_current_context()

    memory_id = f"m-{uuid.uuid4().hex[:12]}"
    now = time.strftime("%Y-%m-%dT%H:%M:%SZ", time.gmtime())
    all_tags = list(ctx.tags) + (tags or [])

    # 1) Store in Memgraph
    try:
        db = Database()
        cypher = """
        MERGE (i:Instance {instance_id: $instance_id})
          ON CREATE SET i.created_at = $now
        MERGE (s:Session {session_id: $session_id})
          ON CREATE SET s.instance_id = $instance_id,
                        s.user_id = $user_id,
                        s.started_at = $now,
                        s.context_signature = $ctx_sig
        MERGE (i)-[:HAS_SESSION]->(s)
        CREATE (m:Memory {
            memory_id: $memory_id,
            kind: $kind,
            text: $text,
            importance: $importance,
            created_at: $now,
            tags: $tags
        })
        CREATE (s)-[:HAS_MEMORY {at: $now}]->(m)
        RETURN m.memory_id AS id
        """
        db.execute_query(cypher, {
            "instance_id": ctx.instance_id,
            "session_id": ctx.session_id,
            "user_id": ctx.user_id,
            "ctx_sig": ctx.context_signature,
            "memory_id": memory_id,
            "kind": kind,
            "text": text[:500],  # Truncate for graph storage
            "importance": importance,
            "tags": all_tags,
            "now": now,
        })
        db.close()
    except Exception as e:
        print(f"⚠️  Memgraph write failed: {e}")

    # 2) Store embedding in ChromaDB
    collection = _get_session_collection()
    if collection is not None:
        embedding = _get_embedding(text)
        if embedding:
            try:
                collection.upsert(
                    ids=[memory_id],
                    embeddings=[embedding],
                    documents=[text],
                    metadatas=[{
                        "memory_id": memory_id,
                        "kind": kind,
                        "importance": importance,
                        "session_id": ctx.session_id,
                        "instance_id": ctx.instance_id,
                        "context_signature": ctx.context_signature,
                        "created_at": now,
                        "tags": ",".join(all_tags),
                    }]
                )
            except Exception as e:
                print(f"⚠️  ChromaDB write failed: {e}")

    return memory_id


def recall_session_memory(
    query: str,
    n_results: int = 5,
    kind: Optional[str] = None,
    min_importance: int = 1,
    ctx: Optional[URPContext] = None,
) -> list[dict]:
    """
    Recall memories from the current session.

    This is the FIRST place to search - your session's private memory.

    Args:
        query: What to search for
        n_results: Max results
        kind: Filter by memory type
        min_importance: Minimum importance level
        ctx: Optional context override

    Returns:
        List of {memory_id, text, kind, importance, similarity} dicts
    """
    if ctx is None:
        ctx = get_current_context()

    collection = _get_session_collection()
    if collection is None:
        return []

    embedding = _get_embedding(query)
    if not embedding:
        return []

    # Build filter
    where = {"session_id": ctx.session_id}
    if kind:
        where["kind"] = kind

    try:
        results = collection.query(
            query_embeddings=[embedding],
            n_results=n_results,
            where=where,
            include=["documents", "metadatas", "distances"]
        )

        output = []
        for i in range(len(results["ids"][0])):
            meta = results["metadatas"][0][i] if results["metadatas"] else {}
            importance = meta.get("importance", 1)

            # Filter by importance
            if importance < min_importance:
                continue

            distance = results["distances"][0][i] if results["distances"] else 1.0
            output.append({
                "memory_id": results["ids"][0][i],
                "text": results["documents"][0][i] if results["documents"] else "",
                "kind": meta.get("kind", "note"),
                "importance": importance,
                "similarity": 1 - distance,
                "created_at": meta.get("created_at", ""),
            })

        return output

    except Exception as e:
        print(f"⚠️  Session memory query failed: {e}")
        return []


def dump_session_memory(ctx: Optional[URPContext] = None) -> list[dict]:
    """
    Get ALL memories for the current session.

    Useful for auditing: "What has this session learned?"
    """
    if ctx is None:
        ctx = get_current_context()

    try:
        db = Database()
        cypher = """
        MATCH (s:Session {session_id: $session_id})-[:HAS_MEMORY]->(m:Memory)
        RETURN m.memory_id AS memory_id,
               m.kind AS kind,
               m.text AS text,
               m.importance AS importance,
               m.created_at AS created_at,
               m.tags AS tags
        ORDER BY m.created_at DESC
        """
        results = db.execute_query(cypher, {"session_id": ctx.session_id})
        db.close()
        return results or []
    except Exception as e:
        print(f"⚠️  Failed to dump session memory: {e}")
        return []


def get_session_memory_stats(ctx: Optional[URPContext] = None) -> dict:
    """Get stats about the current session's memory."""
    if ctx is None:
        ctx = get_current_context()

    collection = _get_session_collection()
    if collection is None:
        return {"error": "ChromaDB not available"}

    try:
        # Count memories for this session
        # ChromaDB doesn't have direct count with filter, so we query with high limit
        results = collection.get(
            where={"session_id": ctx.session_id},
            limit=10000,
            include=["metadatas"]
        )

        total = len(results["ids"]) if results["ids"] else 0

        # Count by kind
        by_kind: dict[str, int] = {}
        if results["metadatas"]:
            for meta in results["metadatas"]:
                kind = meta.get("kind", "unknown")
                by_kind[kind] = by_kind.get(kind, 0) + 1

        return {
            "session_id": ctx.session_id,
            "total_memories": total,
            "by_kind": by_kind,
        }

    except Exception as e:
        return {"error": str(e)}


# ═══════════════════════════════════════════════════════════════════════════════
# Memory Management
# ═══════════════════════════════════════════════════════════════════════════════

def delete_session_memory(memory_id: str, ctx: Optional[URPContext] = None) -> bool:
    """Delete a specific memory from the session."""
    if ctx is None:
        ctx = get_current_context()

    success = True

    # Delete from ChromaDB
    collection = _get_session_collection()
    if collection:
        try:
            collection.delete(ids=[memory_id])
        except Exception as e:
            print(f"⚠️  ChromaDB delete failed: {e}")
            success = False

    # Delete from Memgraph
    try:
        db = Database()
        cypher = """
        MATCH (s:Session {session_id: $session_id})-[r:HAS_MEMORY]->(m:Memory {memory_id: $memory_id})
        DELETE r, m
        """
        db.execute_query(cypher, {
            "session_id": ctx.session_id,
            "memory_id": memory_id,
        })
        db.close()
    except Exception as e:
        print(f"⚠️  Memgraph delete failed: {e}")
        success = False

    return success


def clear_session_memory(ctx: Optional[URPContext] = None) -> int:
    """
    Clear ALL memories for the current session.

    Returns number of memories deleted.
    """
    if ctx is None:
        ctx = get_current_context()

    count = 0

    # Get all memory IDs for this session from ChromaDB
    collection = _get_session_collection()
    if collection:
        try:
            results = collection.get(
                where={"session_id": ctx.session_id},
                limit=10000,
            )
            if results["ids"]:
                count = len(results["ids"])
                collection.delete(ids=results["ids"])
        except Exception as e:
            print(f"⚠️  ChromaDB clear failed: {e}")

    # Clear from Memgraph
    try:
        db = Database()
        cypher = """
        MATCH (s:Session {session_id: $session_id})-[r:HAS_MEMORY]->(m:Memory)
        DELETE r, m
        """
        db.execute_query(cypher, {"session_id": ctx.session_id})
        db.close()
    except Exception as e:
        print(f"⚠️  Memgraph clear failed: {e}")

    return count


# ═══════════════════════════════════════════════════════════════════════════════
# CLI / Debug
# ═══════════════════════════════════════════════════════════════════════════════

if __name__ == "__main__":
    import json

    ctx = get_current_context()
    print(f"Session: {ctx.session_id}")
    print(f"Instance: {ctx.instance_id}")
    print()

    # Add a test memory
    mid = add_session_note(
        "Testing session memory with a sample note about docker permissions",
        kind="note",
        importance=3,
        tags=["test", "docker"]
    )
    print(f"Added memory: {mid}")

    # Recall
    results = recall_session_memory("docker permission")
    print(f"\nRecall 'docker permission':")
    for r in results:
        print(f"  [{r['similarity']:.2f}] {r['text'][:50]}...")

    # Stats
    stats = get_session_memory_stats()
    print(f"\nSession stats: {json.dumps(stats, indent=2)}")
