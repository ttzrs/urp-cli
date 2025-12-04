"""
Git history loader - maps Git DAG to graph primitives.
τ (Vector Ordinal): Commit sequence
T (Tensor): Branch/context state
Φ (Morphism): File changes as causal flow
"""
import os
from dataclasses import dataclass
from typing import Iterator, Optional
from datetime import datetime

try:
    import git
    HAS_GIT = True
except ImportError:
    HAS_GIT = False


@dataclass
class CommitNode:
    """Represents a commit in the temporal graph."""
    hash: str
    short_hash: str
    message: str
    timestamp: int
    author_name: str
    author_email: str
    parent_hashes: list[str]


@dataclass
class FileChange:
    """Represents a file modification in a commit."""
    commit_hash: str
    file_path: str
    insertions: int
    deletions: int
    change_type: str  # 'A' add, 'M' modify, 'D' delete, 'R' rename


class GitLoader:
    """Loads Git history into the graph database."""

    def __init__(self, db):
        if not HAS_GIT:
            raise ImportError("GitPython not installed. Run: pip install gitpython")
        self._db = db
        self._repo = None

    def load_repository(self, repo_path: str):
        """Initialize repository connection."""
        self._repo = git.Repo(repo_path)
        return self

    def ingest_history(self, branch: str = 'HEAD', max_commits: Optional[int] = None) -> int:
        """
        Ingest commit history into the graph.
        Returns number of commits processed.
        """
        if not self._repo:
            raise ValueError("Repository not loaded. Call load_repository first.")

        count = 0
        commits = self._repo.iter_commits(branch)

        for commit in commits:
            if max_commits and count >= max_commits:
                break

            self._ingest_commit(commit)
            count += 1

        return count

    def _ingest_commit(self, commit):
        """Create commit node and relationships."""
        # τ: Temporal node
        self._db.execute_query("""
            MERGE (c:Commit {hash: $hash})
            SET c.short_hash = $short_hash,
                c.message = $message,
                c.timestamp = $timestamp,
                c.datetime = $datetime
        """, {
            "hash": commit.hexsha,
            "short_hash": commit.hexsha[:8],
            "message": commit.message.strip()[:500],  # Truncate long messages
            "timestamp": commit.committed_date,
            "datetime": datetime.fromtimestamp(commit.committed_date).isoformat(),
        })

        # D: Author as domain entity
        self._db.execute_query("""
            MERGE (a:Author {email: $email})
            SET a.name = $name
        """, {
            "email": commit.author.email,
            "name": commit.author.name,
        })

        # Φ: Authorship relationship
        self._db.execute_query("""
            MATCH (a:Author {email: $email}), (c:Commit {hash: $hash})
            MERGE (a)-[:AUTHORED]->(c)
        """, {
            "email": commit.author.email,
            "hash": commit.hexsha,
        })

        # τ: Parent relationships (temporal sequence)
        for parent in commit.parents:
            self._db.execute_query("""
                MERGE (p:Commit {hash: $parent_hash})
                WITH p
                MATCH (c:Commit {hash: $hash})
                MERGE (p)-[:PARENT_OF]->(c)
            """, {
                "parent_hash": parent.hexsha,
                "hash": commit.hexsha,
            })

        # Φ: File changes (causal morphism)
        try:
            for file_path, stats in commit.stats.files.items():
                self._db.execute_query("""
                    MATCH (c:Commit {hash: $hash})
                    MERGE (f:File {path: $path})
                    MERGE (c)-[:TOUCHED {
                        insertions: $ins,
                        deletions: $del
                    }]->(f)
                """, {
                    "hash": commit.hexsha,
                    "path": file_path,
                    "ins": stats.get('insertions', 0),
                    "del": stats.get('deletions', 0),
                })
        except Exception:
            pass  # Some commits may not have stats

    def ingest_branches(self) -> int:
        """
        Ingest branch information as context (Tensor T).
        Returns number of branches processed.
        """
        if not self._repo:
            raise ValueError("Repository not loaded")

        count = 0
        for branch in self._repo.branches:
            self._db.execute_query("""
                MERGE (b:Branch {name: $name})
                SET b.is_remote = false
                WITH b
                MATCH (c:Commit {hash: $hash})
                MERGE (b)-[:POINTS_TO]->(c)
            """, {
                "name": branch.name,
                "hash": branch.commit.hexsha,
            })
            count += 1

        # Remote branches
        for ref in self._repo.remotes.origin.refs if self._repo.remotes else []:
            try:
                self._db.execute_query("""
                    MERGE (b:Branch {name: $name})
                    SET b.is_remote = true
                    WITH b
                    MATCH (c:Commit {hash: $hash})
                    MERGE (b)-[:POINTS_TO]->(c)
                """, {
                    "name": ref.name,
                    "hash": ref.commit.hexsha,
                })
                count += 1
            except Exception:
                pass

        return count

    def get_file_history(self, file_path: str) -> list[dict]:
        """Query commits that touched a specific file."""
        results = self._db.execute_query("""
            MATCH (c:Commit)-[t:TOUCHED]->(f:File {path: $path})
            OPTIONAL MATCH (a:Author)-[:AUTHORED]->(c)
            RETURN c.hash as hash, c.message as message,
                   c.datetime as date, a.name as author,
                   t.insertions as ins, t.deletions as del
            ORDER BY c.timestamp DESC
        """, {"path": file_path})
        return [dict(r) for r in results]

    def get_author_expertise(self, path_pattern: str = "%") -> list[dict]:
        """Find who knows most about a path pattern."""
        results = self._db.execute_query("""
            MATCH (a:Author)-[:AUTHORED]->(c:Commit)-[:TOUCHED]->(f:File)
            WHERE f.path CONTAINS $pattern
            RETURN a.name as author, a.email as email,
                   count(c) as commits,
                   sum(c.timestamp) as activity
            ORDER BY commits DESC
        """, {"pattern": path_pattern})
        return [dict(r) for r in results]

    def get_hotspots(self, since_days: int = 30) -> list[dict]:
        """Find files with most changes (instability indicators)."""
        import time
        cutoff = int(time.time()) - (since_days * 86400)

        results = self._db.execute_query("""
            MATCH (c:Commit)-[:TOUCHED]->(f:File)
            WHERE c.timestamp > $cutoff
            RETURN f.path as file, count(c) as changes
            ORDER BY changes DESC
            LIMIT 20
        """, {"cutoff": cutoff})
        return [dict(r) for r in results]
