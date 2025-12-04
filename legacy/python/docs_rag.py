# ═══════════════════════════════════════════════════════════════════════════════
# External Documentation RAG
# ═══════════════════════════════════════════════════════════════════════════════
#
# Ingest external library documentation into the graph.
# When agent uses an unknown library, it can query the docs
# instead of hallucinating the API.
#
# Supports:
# - Markdown files
# - HTML documentation
# - Python docstrings (via pydoc)
# - Web URLs

import os
import re
import hashlib
import time
from typing import Iterator
from pathlib import Path


# ═══════════════════════════════════════════════════════════════════════════════
# Database Connection
# ═══════════════════════════════════════════════════════════════════════════════

_db = None


def get_db():
    """Lazy database connection."""
    global _db
    if _db is None:
        try:
            from database import Database
            _db = Database()
        except Exception:
            return None
    return _db


# ═══════════════════════════════════════════════════════════════════════════════
# Document Chunking
# ═══════════════════════════════════════════════════════════════════════════════

def chunk_markdown(text: str, max_chunk: int = 500) -> Iterator[dict]:
    """
    Split markdown into semantic chunks (by headers).

    Preserves structure better than arbitrary character splits.
    """
    # Split by headers
    sections = re.split(r'\n(#{1,3}\s+[^\n]+)\n', text)

    current_header = "Overview"
    current_content = []

    for i, section in enumerate(sections):
        if re.match(r'^#{1,3}\s+', section):
            # This is a header
            if current_content:
                content = '\n'.join(current_content).strip()
                if content:
                    yield {
                        'header': current_header,
                        'content': content[:max_chunk],
                        'type': 'markdown'
                    }
            current_header = section.strip('#').strip()
            current_content = []
        else:
            current_content.append(section)

    # Yield final section
    if current_content:
        content = '\n'.join(current_content).strip()
        if content:
            yield {
                'header': current_header,
                'content': content[:max_chunk],
                'type': 'markdown'
            }


def chunk_html(html: str, max_chunk: int = 500) -> Iterator[dict]:
    """
    Extract text from HTML and chunk by paragraphs/sections.
    """
    try:
        from html.parser import HTMLParser

        class TextExtractor(HTMLParser):
            def __init__(self):
                super().__init__()
                self.text = []
                self.current_tag = None

            def handle_starttag(self, tag, attrs):
                self.current_tag = tag

            def handle_data(self, data):
                text = data.strip()
                if text and self.current_tag not in ('script', 'style'):
                    self.text.append(text)

        parser = TextExtractor()
        parser.feed(html)
        full_text = '\n'.join(parser.text)

        # Chunk by paragraphs
        paragraphs = full_text.split('\n\n')
        current_chunk = []
        current_size = 0

        for para in paragraphs:
            if current_size + len(para) > max_chunk and current_chunk:
                yield {
                    'header': 'HTML Section',
                    'content': '\n'.join(current_chunk),
                    'type': 'html'
                }
                current_chunk = []
                current_size = 0

            current_chunk.append(para)
            current_size += len(para)

        if current_chunk:
            yield {
                'header': 'HTML Section',
                'content': '\n'.join(current_chunk),
                'type': 'html'
            }

    except Exception:
        # Fallback: strip tags with regex
        text = re.sub(r'<[^>]+>', ' ', html)
        text = re.sub(r'\s+', ' ', text).strip()
        yield {
            'header': 'Documentation',
            'content': text[:max_chunk],
            'type': 'html'
        }


def get_pydoc(module_name: str) -> Iterator[dict]:
    """
    Extract documentation from a Python module using pydoc.
    """
    try:
        import pydoc
        import importlib

        module = importlib.import_module(module_name)
        doc = pydoc.render_doc(module, title='%s')

        # Split by class/function definitions
        sections = re.split(r'\n(\s*(?:class|def)\s+\w+)', doc)

        for i in range(0, len(sections), 2):
            header = sections[i + 1].strip() if i + 1 < len(sections) else "Module"
            content = sections[i] if i < len(sections) else ""

            if content.strip():
                yield {
                    'header': header,
                    'content': content[:500],
                    'type': 'pydoc',
                    'module': module_name
                }

    except ImportError:
        yield {
            'header': module_name,
            'content': f"Module '{module_name}' not installed.",
            'type': 'error'
        }
    except Exception as e:
        yield {
            'header': module_name,
            'content': f"Error extracting docs: {e}",
            'type': 'error'
        }


# ═══════════════════════════════════════════════════════════════════════════════
# Graph Ingestion
# ═══════════════════════════════════════════════════════════════════════════════

def ingest_docs(library_name: str, chunks: list[dict]) -> int:
    """
    Ingest documentation chunks into the graph.

    Creates:
    - :Library node for the library
    - :DocChunk nodes for each chunk
    - :HAS_DOC edges connecting them
    """
    db = get_db()
    if not db:
        print("Database not available")
        return 0

    try:
        from brain_cortex import get_embedding
        import json

        # Create Library node
        db.execute_query("""
            MERGE (lib:Library {name: $name})
            SET lib.ingested_at = $ts
        """, {"name": library_name, "ts": int(time.time())})

        count = 0
        for chunk in chunks:
            # Generate embedding for semantic search
            vec = get_embedding(chunk['content'])
            embedding_str = json.dumps(vec) if vec else "null"

            # Create chunk ID from content hash
            chunk_id = hashlib.md5(chunk['content'].encode()).hexdigest()[:12]

            db.execute_query(f"""
                MATCH (lib:Library {{name: $lib_name}})
                MERGE (c:DocChunk {{id: $chunk_id}})
                SET c.header = $header,
                    c.content = $content,
                    c.type = $type,
                    c.embedding = {embedding_str}
                MERGE (lib)-[:HAS_DOC]->(c)
            """, {
                "lib_name": library_name,
                "chunk_id": chunk_id,
                "header": chunk['header'],
                "content": chunk['content'],
                "type": chunk['type']
            })
            count += 1

        return count

    except Exception as e:
        print(f"Ingestion failed: {e}")
        return 0


def query_docs(library_name: str, query: str, limit: int = 3) -> list[dict]:
    """
    Query documentation using semantic search.

    Returns most relevant chunks for the query.
    """
    db = get_db()
    if not db:
        return []

    try:
        from brain_cortex import get_embedding, cosine_similarity

        query_vec = get_embedding(query)
        if not query_vec:
            return []

        # Get all chunks for this library
        results = db.execute_query("""
            MATCH (lib:Library {name: $name})-[:HAS_DOC]->(c:DocChunk)
            WHERE c.embedding IS NOT NULL
            RETURN c.header as header, c.content as content, c.embedding as embedding
        """, {"name": library_name})

        # Calculate similarities
        matches = []
        for r in results:
            if not r.get('embedding'):
                continue
            sim = cosine_similarity(query_vec, r['embedding'])
            matches.append({
                'header': r['header'],
                'content': r['content'],
                'similarity': sim
            })

        # Sort by similarity and return top N
        matches.sort(key=lambda x: x['similarity'], reverse=True)
        return matches[:limit]

    except Exception as e:
        print(f"Query failed: {e}")
        return []


# ═══════════════════════════════════════════════════════════════════════════════
# CLI
# ═══════════════════════════════════════════════════════════════════════════════

def main():
    import argparse

    parser = argparse.ArgumentParser(description="External documentation RAG")
    subparsers = parser.add_subparsers(dest="action", required=True)

    # ingest - add documentation
    p = subparsers.add_parser("ingest", help="Ingest documentation")
    p.add_argument("library", help="Library name")
    p.add_argument("source", help="Path to markdown/html file, URL, or Python module")
    p.add_argument("--type", choices=["md", "html", "pydoc", "auto"], default="auto")

    # query - search documentation
    p = subparsers.add_parser("query", help="Query documentation")
    p.add_argument("library", help="Library name")
    p.add_argument("query", help="Search query")
    p.add_argument("--limit", type=int, default=3)

    # list - show ingested libraries
    p = subparsers.add_parser("list", help="List ingested libraries")

    args = parser.parse_args()

    if args.action == "ingest":
        source = args.source
        lib = args.library
        doc_type = args.type

        # Auto-detect type
        if doc_type == "auto":
            if source.endswith('.md'):
                doc_type = "md"
            elif source.endswith('.html') or source.endswith('.htm'):
                doc_type = "html"
            elif source.startswith('http'):
                doc_type = "html"
            else:
                doc_type = "pydoc"

        # Get chunks
        chunks = []
        if doc_type == "pydoc":
            chunks = list(get_pydoc(source))
        elif doc_type == "md":
            if os.path.exists(source):
                with open(source, 'r') as f:
                    chunks = list(chunk_markdown(f.read()))
            else:
                print(f"File not found: {source}")
                return
        elif doc_type == "html":
            if source.startswith('http'):
                try:
                    import urllib.request
                    with urllib.request.urlopen(source) as response:
                        html = response.read().decode('utf-8')
                        chunks = list(chunk_html(html))
                except Exception as e:
                    print(f"Failed to fetch URL: {e}")
                    return
            elif os.path.exists(source):
                with open(source, 'r') as f:
                    chunks = list(chunk_html(f.read()))
            else:
                print(f"File not found: {source}")
                return

        if chunks:
            count = ingest_docs(lib, chunks)
            print(f"Ingested {count} chunks for '{lib}'")
        else:
            print("No chunks extracted")

    elif args.action == "query":
        results = query_docs(args.library, args.query, args.limit)
        if results:
            print(f"Documentation for '{args.library}' matching '{args.query}':\n")
            for r in results:
                print(f"## {r['header']} (similarity: {r['similarity']:.2f})")
                print(r['content'][:300])
                print()
        else:
            print(f"No documentation found for '{args.library}'")

    elif args.action == "list":
        db = get_db()
        if db:
            results = db.execute_query("""
                MATCH (lib:Library)-[:HAS_DOC]->(c:DocChunk)
                RETURN lib.name as name, count(c) as chunks
            """)
            print("Ingested libraries:")
            for r in results:
                print(f"  - {r['name']}: {r['chunks']} chunks")


if __name__ == "__main__":
    main()
