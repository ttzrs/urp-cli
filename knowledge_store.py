# ═══════════════════════════════════════════════════════════════════════════════
# Knowledge Store: Shared KB with multi-level search
# ═══════════════════════════════════════════════════════════════════════════════
#
# Manages shared knowledge across sessions/instances with:
# - 3-level search: session → instance → global
# - Rejection tracking (filter noise from other sessions)
# - Knowledge promotion (session → global)
# - Full traceability in Memgraph

import os
import time
import uuid
from typing import Optional, Any
from dataclasses import dataclass

from context import URPContext, get_current_context, is_context_compatible
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


def _get_knowledge_collection():
    """Get or create the shared knowledge collection."""
    client = _get_chroma()
    if client is None:
        return None
    try:
        return client.get_or_create_collection(
            name="urp_knowledge",
            metadata={"hnsw:space": "cosine"}
        )
    except Exception as e:
        print(f"⚠️  Failed to get urp_knowledge collection: {e}")
        return None


def _get_embedding(text: str) -> list[float]:
    """Get embedding for text."""
    try:
        from brain_cortex import get_embedding
        return get_embedding(text)
    except Exception:
        return []


# ═══════════════════════════════════════════════════════════════════════════════
# Knowledge CRUD
# ═══════════════════════════════════════════════════════════════════════════════

def store_knowledge(
    text: str,
    kind: str,
    scope: str = "session",
    ctx: Optional[URPContext] = None,
    knowledge_id: Optional[str] = None,
    extra_meta: Optional[dict] = None,
) -> str:
    """
    Store a piece of knowledge in Memgraph + ChromaDB.

    Args:
        text: The knowledge content
        kind: Type ("error", "fix", "rule", "pattern", "plan", "insight")
        scope: Visibility ("session", "instance", "global")
        ctx: Optional context override
        knowledge_id: Optional ID (auto-generated if not provided)
        extra_meta: Additional metadata

    Returns:
        knowledge_id
    """
    if ctx is None:
        ctx = get_current_context()

    if knowledge_id is None:
        knowledge_id = f"k-{uuid.uuid4().hex[:12]}"

    now = time.strftime("%Y-%m-%dT%H:%M:%SZ", time.gmtime())

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
        MERGE (k:Knowledge {knowledge_id: $knowledge_id})
          ON CREATE SET k.kind = $kind,
                        k.text = $text,
                        k.scope = $scope,
                        k.created_at = $now,
                        k.context_signature = $ctx_sig
          ON MATCH SET k.text = $text,
                       k.scope = $scope
        MERGE (s)-[:CREATED {at: $now}]->(k)
        RETURN k.knowledge_id AS id
        """
        db.execute_query(cypher, {
            "instance_id": ctx.instance_id,
            "session_id": ctx.session_id,
            "user_id": ctx.user_id,
            "ctx_sig": ctx.context_signature,
            "knowledge_id": knowledge_id,
            "kind": kind,
            "text": text[:1000],  # Truncate for graph
            "scope": scope,
            "now": now,
        })
        db.close()
    except Exception as e:
        print(f"⚠️  Memgraph write failed: {e}")

    # 2) Store embedding in ChromaDB
    collection = _get_knowledge_collection()
    if collection is not None:
        embedding = _get_embedding(text)
        if embedding:
            meta = {
                "knowledge_id": knowledge_id,
                "kind": kind,
                "scope": scope,
                "session_id": ctx.session_id,
                "instance_id": ctx.instance_id,
                "user_id": ctx.user_id,
                "context_signature": ctx.context_signature,
                "created_at": now,
            }
            if extra_meta:
                meta.update(extra_meta)

            try:
                collection.upsert(
                    ids=[knowledge_id],
                    embeddings=[embedding],
                    documents=[text],
                    metadatas=[meta]
                )
            except Exception as e:
                print(f"⚠️  ChromaDB write failed: {e}")

    return knowledge_id


# ═══════════════════════════════════════════════════════════════════════════════
# Rejection System
# ═══════════════════════════════════════════════════════════════════════════════

def reject_knowledge(
    knowledge_id: str,
    reason: str,
    ctx: Optional[URPContext] = None,
) -> bool:
    """
    Mark a piece of knowledge as "not applicable" for this session.

    The knowledge won't appear in future searches for this session.

    Args:
        knowledge_id: ID of the knowledge to reject
        reason: Why it doesn't apply (e.g., "different dataset", "wrong stack")
        ctx: Optional context override

    Returns:
        True if successful
    """
    if ctx is None:
        ctx = get_current_context()

    try:
        db = Database()
        cypher = """
        MATCH (s:Session {session_id: $session_id})
        MATCH (k:Knowledge {knowledge_id: $knowledge_id})
        MERGE (s)-[r:REJECTED]->(k)
          ON CREATE SET r.at = timestamp(), r.reason = $reason
          ON MATCH SET r.reason = $reason
        """
        db.execute_query(cypher, {
            "session_id": ctx.session_id,
            "knowledge_id": knowledge_id,
            "reason": reason,
        })
        db.close()
        return True
    except Exception as e:
        print(f"⚠️  Failed to reject knowledge: {e}")
        return False


def get_rejected_ids(ctx: Optional[URPContext] = None) -> set[str]:
    """Get all knowledge IDs rejected by the current session."""
    if ctx is None:
        ctx = get_current_context()

    try:
        db = Database()
        cypher = """
        MATCH (s:Session {session_id: $session_id})-[:REJECTED]->(k:Knowledge)
        RETURN k.knowledge_id AS id
        """
        results = db.execute_query(cypher, {"session_id": ctx.session_id})
        db.close()
        return {r["id"] for r in results} if results else set()
    except Exception:
        return set()


def mark_knowledge_used(
    knowledge_id: str,
    similarity: float = 0.0,
    ctx: Optional[URPContext] = None,
) -> bool:
    """
    Mark that this session USED a piece of knowledge.

    This helps track which knowledge is actually useful.
    """
    if ctx is None:
        ctx = get_current_context()

    try:
        db = Database()
        cypher = """
        MATCH (s:Session {session_id: $session_id})
        MATCH (k:Knowledge {knowledge_id: $knowledge_id})
        MERGE (s)-[r:USED]->(k)
          ON CREATE SET r.at = timestamp(), r.similarity = $similarity
        """
        db.execute_query(cypher, {
            "session_id": ctx.session_id,
            "knowledge_id": knowledge_id,
            "similarity": similarity,
        })
        db.close()
        return True
    except Exception as e:
        print(f"⚠️  Failed to mark knowledge used: {e}")
        return False


# ═══════════════════════════════════════════════════════════════════════════════
# Multi-level Query
# ═══════════════════════════════════════════════════════════════════════════════

def query_knowledge(
    text: str,
    n_results: int = 10,
    level: str = "all",
    kind: Optional[str] = None,
    ctx: Optional[URPContext] = None,
) -> list[dict]:
    """
    Search knowledge with multi-level strategy.

    Search order:
    1. Current session's knowledge
    2. Same instance + compatible context
    3. Global knowledge

    Args:
        text: Query text
        n_results: Max results per level
        level: "session", "instance", "global", or "all" (cascading)
        kind: Filter by knowledge type
        ctx: Optional context override

    Returns:
        List of {knowledge_id, text, kind, scope, similarity, source_level} dicts
    """
    if ctx is None:
        ctx = get_current_context()

    collection = _get_knowledge_collection()
    if collection is None:
        return []

    embedding = _get_embedding(text)
    if not embedding:
        return []

    rejected = get_rejected_ids(ctx)
    collected: list[dict] = []
    seen_ids: set[str] = set()

    def _query_level(where: dict, source: str) -> list[dict]:
        """Query a single level and filter results."""
        try:
            results = collection.query(
                query_embeddings=[embedding],
                n_results=n_results,
                where=where,
                include=["documents", "metadatas", "distances"]
            )

            out = []
            for i in range(len(results["ids"][0])):
                kid = results["ids"][0][i]

                # Skip rejected
                if kid in rejected:
                    continue

                # Skip already seen
                if kid in seen_ids:
                    continue

                seen_ids.add(kid)

                meta = results["metadatas"][0][i] if results["metadatas"] else {}
                distance = results["distances"][0][i] if results["distances"] else 1.0

                # Filter by kind if specified
                if kind and meta.get("kind") != kind:
                    continue

                out.append({
                    "knowledge_id": kid,
                    "text": results["documents"][0][i] if results["documents"] else "",
                    "kind": meta.get("kind", "unknown"),
                    "scope": meta.get("scope", "unknown"),
                    "similarity": 1 - distance,
                    "source_level": source,
                    "session_id": meta.get("session_id", ""),
                    "instance_id": meta.get("instance_id", ""),
                    "context_signature": meta.get("context_signature", ""),
                })

            return out
        except Exception as e:
            print(f"⚠️  Query level {source} failed: {e}")
            return []

    # Level 1: Current session
    if level in ("session", "all"):
        results = _query_level(
            {"session_id": ctx.session_id},
            "session"
        )
        collected.extend(results)

        if level == "session":
            return collected[:n_results]

    # Level 2: Same instance + compatible context
    if level in ("instance", "all"):
        # ChromaDB requires $and for multiple conditions
        results = _query_level(
            {"$and": [
                {"instance_id": {"$eq": ctx.instance_id}},
                {"scope": {"$eq": "instance"}}
            ]},
            "instance"
        )
        # Filter by context compatibility
        for r in results:
            if is_context_compatible(r["context_signature"], ctx.context_signature):
                collected.append(r)

        if level == "instance":
            return collected[:n_results]

    # Level 3: Global
    if level in ("global", "all"):
        results = _query_level(
            {"scope": "global"},
            "global"
        )
        collected.extend(results)

    # Sort by similarity and return
    collected.sort(key=lambda x: x["similarity"], reverse=True)
    return collected[:n_results]


# ═══════════════════════════════════════════════════════════════════════════════
# Knowledge Promotion
# ═══════════════════════════════════════════════════════════════════════════════

def export_memory_to_knowledge(
    memory_id: str,
    kind: str = "rule",
    scope: str = "global",
    ctx: Optional[URPContext] = None,
) -> Optional[str]:
    """
    Promote a session memory to shared knowledge.

    Args:
        memory_id: The session memory to export
        kind: Knowledge type for the exported item
        scope: Target scope ("instance" or "global")
        ctx: Optional context override

    Returns:
        knowledge_id if successful, None otherwise
    """
    if ctx is None:
        ctx = get_current_context()

    try:
        db = Database()

        # Get the memory
        cypher = """
        MATCH (s:Session {session_id: $session_id})-[:HAS_MEMORY]->(m:Memory {memory_id: $memory_id})
        RETURN m.text AS text, m.tags AS tags
        """
        results = db.execute_query(cypher, {
            "session_id": ctx.session_id,
            "memory_id": memory_id,
        })

        if not results:
            print(f"⚠️  Memory {memory_id} not found in session")
            db.close()
            return None

        text = results[0]["text"]
        tags = results[0].get("tags", [])

        # Create knowledge
        knowledge_id = store_knowledge(
            text=text,
            kind=kind,
            scope=scope,
            ctx=ctx,
            extra_meta={"source_memory": memory_id, "tags": ",".join(tags) if tags else ""},
        )

        # Record the export relationship
        cypher = """
        MATCH (s:Session {session_id: $session_id})
        MATCH (k:Knowledge {knowledge_id: $knowledge_id})
        MERGE (s)-[:EXPORTED {at: timestamp()}]->(k)
        """
        db.execute_query(cypher, {
            "session_id": ctx.session_id,
            "knowledge_id": knowledge_id,
        })

        db.close()
        return knowledge_id

    except Exception as e:
        print(f"⚠️  Failed to export memory: {e}")
        return None


def promote_knowledge_to_global(
    knowledge_id: str,
    ctx: Optional[URPContext] = None,
) -> bool:
    """
    Promote existing knowledge from session/instance scope to global.

    Useful when knowledge has been validated across multiple sessions.
    """
    if ctx is None:
        ctx = get_current_context()

    # Update in Memgraph
    try:
        db = Database()
        cypher = """
        MATCH (k:Knowledge {knowledge_id: $knowledge_id})
        SET k.scope = 'global', k.promoted_at = timestamp()
        """
        db.execute_query(cypher, {"knowledge_id": knowledge_id})
        db.close()
    except Exception as e:
        print(f"⚠️  Memgraph update failed: {e}")
        return False

    # Update in ChromaDB metadata
    collection = _get_knowledge_collection()
    if collection:
        try:
            # Get current entry
            result = collection.get(ids=[knowledge_id], include=["metadatas", "documents", "embeddings"])
            if result["ids"]:
                meta = result["metadatas"][0]
                meta["scope"] = "global"
                collection.update(
                    ids=[knowledge_id],
                    metadatas=[meta]
                )
        except Exception as e:
            print(f"⚠️  ChromaDB update failed: {e}")
            return False

    return True


# ═══════════════════════════════════════════════════════════════════════════════
# Stats & Audit
# ═══════════════════════════════════════════════════════════════════════════════

def get_knowledge_stats(ctx: Optional[URPContext] = None) -> dict:
    """Get stats about the knowledge store."""
    collection = _get_knowledge_collection()
    if collection is None:
        return {"error": "ChromaDB not available"}

    try:
        total = collection.count()

        # Get counts by scope (approximation via sampling)
        by_scope = {"session": 0, "instance": 0, "global": 0}

        for scope in by_scope.keys():
            results = collection.get(
                where={"scope": scope},
                limit=10000,
            )
            by_scope[scope] = len(results["ids"]) if results["ids"] else 0

        return {
            "total_knowledge": total,
            "by_scope": by_scope,
        }

    except Exception as e:
        return {"error": str(e)}


def get_knowledge_provenance(knowledge_id: str) -> dict:
    """
    Get full provenance info for a piece of knowledge.

    Shows: who created it, who used it, who rejected it.
    """
    try:
        db = Database()
        cypher = """
        MATCH (k:Knowledge {knowledge_id: $knowledge_id})
        OPTIONAL MATCH (creator:Session)-[c:CREATED]->(k)
        OPTIONAL MATCH (user:Session)-[u:USED]->(k)
        OPTIONAL MATCH (rejector:Session)-[r:REJECTED]->(k)
        RETURN k.kind AS kind,
               k.scope AS scope,
               k.created_at AS created_at,
               k.context_signature AS context,
               collect(DISTINCT {session: creator.session_id, at: c.at}) AS created_by,
               collect(DISTINCT {session: user.session_id, at: u.at}) AS used_by,
               collect(DISTINCT {session: rejector.session_id, reason: r.reason}) AS rejected_by
        """
        results = db.execute_query(cypher, {"knowledge_id": knowledge_id})
        db.close()

        if results:
            return results[0]
        return {"error": "Knowledge not found"}

    except Exception as e:
        return {"error": str(e)}


# ═══════════════════════════════════════════════════════════════════════════════
# CLI / Debug
# ═══════════════════════════════════════════════════════════════════════════════

if __name__ == "__main__":
    import json

    ctx = get_current_context()
    print(f"Session: {ctx.session_id}")
    print(f"Instance: {ctx.instance_id}")
    print()

    # Store test knowledge
    kid = store_knowledge(
        "SELinux blocks docker socket access in containers. Fix: security_opt label:disable",
        kind="fix",
        scope="instance",
    )
    print(f"Stored knowledge: {kid}")

    # Query
    results = query_knowledge("docker socket permission denied")
    print(f"\nQuery 'docker socket permission denied':")
    for r in results:
        print(f"  [{r['similarity']:.2f}] [{r['source_level']}] {r['text'][:50]}...")

    # Stats
    stats = get_knowledge_stats()
    print(f"\nKnowledge stats: {json.dumps(stats, indent=2)}")
