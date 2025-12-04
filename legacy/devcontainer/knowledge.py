#!/usr/bin/env python3
"""
Shared Knowledge Manager - Upload/Download knowledge to persistent memory.

Knowledge persists across container restarts in the shared_knowledge volume.
Files are automatically ingested into the graph RAG on container start.

Usage:
    knowledge upload file.md              # Upload to shared knowledge
    knowledge upload file.md --tag api    # Upload with tag
    knowledge list                         # List all knowledge
    knowledge get api                      # Get by tag
    knowledge delete api                   # Delete by tag
    knowledge sync                         # Sync to graph RAG
"""

import os
import sys
import json
import shutil
import hashlib
from pathlib import Path
from datetime import datetime

KNOWLEDGE_DIR = Path("/shared/knowledge")
INDEX_FILE = KNOWLEDGE_DIR / ".index.json"


def load_index() -> dict:
    """Load knowledge index."""
    if INDEX_FILE.exists():
        try:
            return json.loads(INDEX_FILE.read_text())
        except Exception:
            pass
    return {"items": {}}


def save_index(index: dict):
    """Save knowledge index."""
    KNOWLEDGE_DIR.mkdir(parents=True, exist_ok=True)
    INDEX_FILE.write_text(json.dumps(index, indent=2))


def upload(file_path: str, tag: str = None):
    """Upload file to shared knowledge."""
    src = Path(file_path)
    if not src.exists():
        print(f"Error: File not found: {file_path}")
        return False

    # Generate ID from content hash
    content = src.read_bytes()
    file_id = hashlib.md5(content).hexdigest()[:12]

    # Use tag or filename as key
    key = tag or src.stem

    # Copy to knowledge dir
    dest = KNOWLEDGE_DIR / f"{key}_{file_id}{src.suffix}"
    KNOWLEDGE_DIR.mkdir(parents=True, exist_ok=True)
    shutil.copy2(src, dest)

    # Update index
    index = load_index()
    index["items"][key] = {
        "id": file_id,
        "file": dest.name,
        "original": src.name,
        "size": len(content),
        "uploaded_at": datetime.now().isoformat(),
        "synced": False
    }
    save_index(index)

    print(f"Uploaded: {key} ({len(content)} bytes)")
    print(f"Location: {dest}")
    return True


def list_knowledge():
    """List all knowledge items."""
    index = load_index()

    if not index["items"]:
        print("No knowledge items. Use 'knowledge upload <file>' to add.")
        return

    print("Shared Knowledge:")
    print("-" * 60)
    for key, item in index["items"].items():
        synced = "✓" if item.get("synced") else "○"
        size_kb = item.get("size", 0) / 1024
        print(f"  [{synced}] {key:20} {size_kb:>6.1f}KB  {item.get('original', '?')}")

    print("-" * 60)
    print(f"Total: {len(index['items'])} items")


def get_knowledge(tag: str) -> str:
    """Get knowledge file path by tag."""
    index = load_index()
    item = index["items"].get(tag)

    if not item:
        print(f"Not found: {tag}")
        return None

    path = KNOWLEDGE_DIR / item["file"]
    if path.exists():
        print(path)
        return str(path)
    else:
        print(f"File missing: {path}")
        return None


def delete_knowledge(tag: str):
    """Delete knowledge by tag."""
    index = load_index()
    item = index["items"].get(tag)

    if not item:
        print(f"Not found: {tag}")
        return False

    # Delete file
    path = KNOWLEDGE_DIR / item["file"]
    if path.exists():
        path.unlink()

    # Update index
    del index["items"][tag]
    save_index(index)

    print(f"Deleted: {tag}")
    return True


def sync_to_rag():
    """Sync all knowledge to graph RAG."""
    index = load_index()

    if not index["items"]:
        print("Nothing to sync.")
        return 0

    try:
        sys.path.insert(0, "/app")
        from docs_rag import ingest_docs, chunk_markdown
    except ImportError:
        print("Error: docs_rag module not available")
        return 0

    synced = 0
    for key, item in index["items"].items():
        if item.get("synced"):
            continue

        path = KNOWLEDGE_DIR / item["file"]
        if not path.exists():
            continue

        try:
            content = path.read_text()

            if path.suffix == ".md":
                chunks = list(chunk_markdown(content))
            else:
                chunks = [{"header": key, "content": content[:500], "type": "text"}]

            count = ingest_docs(f"knowledge:{key}", chunks)
            if count > 0:
                item["synced"] = True
                synced += 1
                print(f"Synced: {key} ({count} chunks)")

        except Exception as e:
            print(f"Failed to sync {key}: {e}")

    save_index(index)
    print(f"\nSynced {synced} items to graph RAG")
    return synced


def main():
    import argparse

    parser = argparse.ArgumentParser(description="Shared Knowledge Manager")
    subparsers = parser.add_subparsers(dest="action", required=True)

    # upload
    p = subparsers.add_parser("upload", help="Upload file to shared knowledge")
    p.add_argument("file", help="File to upload")
    p.add_argument("--tag", help="Tag/key for the knowledge")

    # list
    subparsers.add_parser("list", help="List all knowledge")

    # get
    p = subparsers.add_parser("get", help="Get knowledge file path")
    p.add_argument("tag", help="Knowledge tag")

    # delete
    p = subparsers.add_parser("delete", help="Delete knowledge")
    p.add_argument("tag", help="Knowledge tag")

    # sync
    subparsers.add_parser("sync", help="Sync all knowledge to graph RAG")

    args = parser.parse_args()

    if args.action == "upload":
        upload(args.file, args.tag)
    elif args.action == "list":
        list_knowledge()
    elif args.action == "get":
        get_knowledge(args.tag)
    elif args.action == "delete":
        delete_knowledge(args.tag)
    elif args.action == "sync":
        sync_to_rag()


if __name__ == "__main__":
    main()
