# ═══════════════════════════════════════════════════════════════════════════════
# Brain Cortex: Embedding-based Semantic Memory
# ═══════════════════════════════════════════════════════════════════════════════
#
# Provides vector embeddings for semantic similarity search.
# Used by wisdom (find similar errors) and novelty (detect pattern breaks).
# Persistence via ChromaDB at /app/chroma

import os
import numpy as np
from typing import Optional

# Lazy load model to avoid startup cost when not needed
_model = None
_chroma_client = None
_MODEL_NAME = os.getenv('URP_EMBEDDING_MODEL', 'all-MiniLM-L6-v2')
_CHROMA_PATH = os.getenv('URP_CHROMA_PATH', '/app/chroma')


def _get_model():
    """Lazy load the sentence transformer model."""
    global _model
    if _model is None:
        try:
            from sentence_transformers import SentenceTransformer
            _model = SentenceTransformer(_MODEL_NAME)
        except ImportError:
            return None
        except Exception as e:
            print(f"⚠️  Failed to load embedding model: {e}")
            return None
    return _model


def _get_chroma():
    """Lazy load ChromaDB persistent client."""
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
        except ImportError:
            print("⚠️  chromadb not installed, vector persistence disabled")
            return None
        except Exception as e:
            print(f"⚠️  Failed to initialize ChromaDB: {e}")
            return None
    return _chroma_client


def get_collection(name: str = "urp_embeddings"):
    """Get or create a ChromaDB collection."""
    client = _get_chroma()
    if client is None:
        return None
    try:
        return client.get_or_create_collection(
            name=name,
            metadata={"hnsw:space": "cosine"}
        )
    except Exception as e:
        print(f"⚠️  Failed to get collection {name}: {e}")
        return None


def get_embedding(text: str) -> list[float]:
    """
    Convert text to a vector embedding.

    Returns empty list if model unavailable (graceful degradation).
    """
    if not text or not text.strip():
        return []

    model = _get_model()
    if model is None:
        return []

    try:
        # Truncate very long text to avoid memory issues
        text = text[:2000]
        return model.encode(text).tolist()
    except Exception:
        return []


def store_embedding(doc_id: str, text: str, metadata: dict = None,
                    collection_name: str = "urp_embeddings") -> bool:
    """
    Store text with its embedding in ChromaDB.

    Args:
        doc_id: Unique identifier for the document
        text: Text to embed and store
        metadata: Optional metadata dict
        collection_name: ChromaDB collection name

    Returns:
        True if stored successfully, False otherwise
    """
    collection = get_collection(collection_name)
    if collection is None:
        return False

    embedding = get_embedding(text)
    if not embedding:
        return False

    try:
        collection.upsert(
            ids=[doc_id],
            embeddings=[embedding],
            documents=[text],
            metadatas=[metadata or {}]
        )
        return True
    except Exception as e:
        print(f"⚠️  Failed to store embedding: {e}")
        return False


def query_similar(text: str, n_results: int = 5,
                  collection_name: str = "urp_embeddings",
                  where: dict = None) -> list[dict]:
    """
    Find similar documents in ChromaDB.

    Args:
        text: Query text
        n_results: Max number of results
        collection_name: ChromaDB collection name
        where: Optional metadata filter

    Returns:
        List of {id, document, metadata, distance} dicts
    """
    collection = get_collection(collection_name)
    if collection is None:
        return []

    embedding = get_embedding(text)
    if not embedding:
        return []

    try:
        results = collection.query(
            query_embeddings=[embedding],
            n_results=n_results,
            where=where,
            include=["documents", "metadatas", "distances"]
        )

        output = []
        for i in range(len(results["ids"][0])):
            output.append({
                "id": results["ids"][0][i],
                "document": results["documents"][0][i] if results["documents"] else None,
                "metadata": results["metadatas"][0][i] if results["metadatas"] else {},
                "distance": results["distances"][0][i] if results["distances"] else None,
                "similarity": 1 - results["distances"][0][i] if results["distances"] else None
            })
        return output
    except Exception as e:
        print(f"⚠️  Query failed: {e}")
        return []


def cosine_similarity(vec_a: list[float], vec_b: list[float]) -> float:
    """Calculate cosine similarity between two vectors."""
    if not vec_a or not vec_b:
        return 0.0

    a = np.array(vec_a)
    b = np.array(vec_b)

    dot = np.dot(a, b)
    norm_a = np.linalg.norm(a)
    norm_b = np.linalg.norm(b)

    if norm_a == 0 or norm_b == 0:
        return 0.0

    return float(dot / (norm_a * norm_b))


def calculate_novelty(new_vec: list[float], history_vecs: list[list[float]]) -> float:
    """
    Calculate how novel a vector is compared to historical vectors.

    Returns:
        0.0 = Identical to existing patterns (boring/safe)
        1.0 = Completely new pattern (risky/innovative)
    """
    if not new_vec:
        return 0.5  # Unknown

    if not history_vecs:
        return 1.0  # No history = everything is new

    # Calculate similarity to centroid of existing patterns
    centroid = np.mean(history_vecs, axis=0)
    similarity = cosine_similarity(new_vec, centroid.tolist())

    # Convert similarity [-1, 1] to novelty [0, 1]
    # High similarity = low novelty
    return max(0.0, 1.0 - similarity)


def find_most_similar(query_vec: list[float],
                      candidates: list[tuple[str, list[float]]],
                      threshold: float = 0.7) -> list[tuple[str, float]]:
    """
    Find candidates most similar to query vector (in-memory version).

    Args:
        query_vec: The query embedding
        candidates: List of (id, embedding) tuples
        threshold: Minimum similarity to include

    Returns:
        List of (id, similarity) tuples, sorted by similarity descending
    """
    if not query_vec:
        return []

    results = []
    for cand_id, cand_vec in candidates:
        if not cand_vec:
            continue
        sim = cosine_similarity(query_vec, cand_vec)
        if sim >= threshold:
            results.append((cand_id, sim))

    return sorted(results, key=lambda x: x[1], reverse=True)


def get_collection_stats(collection_name: str = "urp_embeddings") -> dict:
    """Get stats about a collection."""
    collection = get_collection(collection_name)
    if collection is None:
        return {"error": "Collection not available"}

    try:
        return {
            "name": collection_name,
            "count": collection.count()
        }
    except Exception as e:
        return {"error": str(e)}


# ─────────────────────────────────────────────────────────────────────────────
# Preload script (run during Docker build)
# ─────────────────────────────────────────────────────────────────────────────

if __name__ == "__main__":
    print(f"Preloading embedding model: {_MODEL_NAME}")
    model = _get_model()
    if model:
        # Warm up with a test encode
        _ = model.encode("test")
        print("✓ Model loaded and cached")
    else:
        print("✗ Failed to load model")

    print(f"\nInitializing ChromaDB at: {_CHROMA_PATH}")
    client = _get_chroma()
    if client:
        stats = get_collection_stats()
        print(f"✓ ChromaDB ready: {stats}")
    else:
        print("✗ ChromaDB not available")
