"""
Graph querier - implements queries using PRU primitives.

Query types mapped to primitives:
  Φ (Morphism): Impact analysis, call chains, data flow
  τ (Vector): Temporal queries, commit history, evolution
  ⊆ (Inclusion): Hierarchy queries, containment
  ⊥ (Orthogonal): Conflict detection, test results
  T (Tensor): Context filtering (branch, env, version)
"""
from database import Database


class Querier:
    """Query interface for the code knowledge graph."""

    def __init__(self, db: Database):
        self._db = db

    # ═══════════════════════════════════════════════════════════════
    # Φ (Morphism) - Causal Flow Queries
    # ═══════════════════════════════════════════════════════════════

    def find_impact(self, target_signature: str, max_depth: int = 10) -> list[dict]:
        """
        Find all nodes that would be affected if target changes.
        Φ inverse: Who depends on this?
        """
        results = self._db.execute_query("""
            MATCH path = (source)-[:CALLS|FLOWS_TO*1..%d]->(target:Function {signature: $sig})
            RETURN DISTINCT source.signature as affected,
                   source.name as name,
                   length(path) as distance
            ORDER BY distance
        """ % max_depth, {"sig": target_signature})
        return [dict(r) for r in results]

    def find_dependencies(self, source_signature: str, max_depth: int = 10) -> list[dict]:
        """
        Find all nodes that source depends on.
        Φ forward: What does this call?
        """
        results = self._db.execute_query("""
            MATCH path = (source:Function {signature: $sig})-[:CALLS*1..%d]->(target)
            RETURN DISTINCT target.name as dependency,
                   labels(target)[0] as type,
                   length(path) as distance
            ORDER BY distance
        """ % max_depth, {"sig": source_signature})
        return [dict(r) for r in results]

    def find_call_chain(self, from_sig: str, to_sig: str) -> list[dict]:
        """
        Find shortest path between two functions.
        Φ path: How does A reach B?
        """
        results = self._db.execute_query("""
            MATCH path = shortestPath(
                (a:Function {signature: $from})-[:CALLS*]->(b:Function {signature: $to})
            )
            UNWIND nodes(path) as node
            RETURN node.signature as step, node.name as name
        """, {"from": from_sig, "to": to_sig})
        return [dict(r) for r in results]

    # ═══════════════════════════════════════════════════════════════
    # τ (Vector) - Temporal Queries
    # ═══════════════════════════════════════════════════════════════

    def get_file_history(self, file_path: str, limit: int = 20) -> list[dict]:
        """
        Get commit history for a file.
        τ sequence: What happened to this file over time?
        """
        results = self._db.execute_query("""
            MATCH (c:Commit)-[t:TOUCHED]->(f:File {path: $path})
            OPTIONAL MATCH (a:Author)-[:AUTHORED]->(c)
            RETURN c.short_hash as commit,
                   c.message as message,
                   c.datetime as date,
                   a.name as author,
                   t.insertions as adds,
                   t.deletions as dels
            ORDER BY c.timestamp DESC
            LIMIT $limit
        """, {"path": file_path, "limit": limit})
        return [dict(r) for r in results]

    def get_recent_changes(self, days: int = 7) -> list[dict]:
        """
        Get recently changed files.
        τ recent: What changed recently?
        """
        import time
        cutoff = int(time.time()) - (days * 86400)

        results = self._db.execute_query("""
            MATCH (c:Commit)-[:TOUCHED]->(f:File)
            WHERE c.timestamp > $cutoff
            RETURN f.path as file,
                   count(c) as changes,
                   max(c.timestamp) as last_change
            ORDER BY changes DESC
        """, {"cutoff": cutoff})
        return [dict(r) for r in results]

    def get_author_expertise(self, path_pattern: str) -> list[dict]:
        """
        Find who knows most about a code area.
        τ + D: Who touched this domain most?
        """
        results = self._db.execute_query("""
            MATCH (a:Author)-[:AUTHORED]->(c:Commit)-[:TOUCHED]->(f:File)
            WHERE f.path CONTAINS $pattern
            RETURN a.name as author,
                   a.email as email,
                   count(DISTINCT c) as commits,
                   count(DISTINCT f) as files_touched
            ORDER BY commits DESC
        """, {"pattern": path_pattern})
        return [dict(r) for r in results]

    # ═══════════════════════════════════════════════════════════════
    # ⊆ (Inclusion) - Hierarchy Queries
    # ═══════════════════════════════════════════════════════════════

    def get_file_contents(self, file_path: str) -> list[dict]:
        """
        Get all entities contained in a file.
        ⊆ contents: What's inside this file?
        """
        results = self._db.execute_query("""
            MATCH (f:File {path: $path})-[:CONTAINS]->(e)
            RETURN e.name as name,
                   e.kind as kind,
                   e.signature as signature,
                   e.start_line as line
            ORDER BY e.start_line
        """, {"path": file_path})
        return [dict(r) for r in results]

    def get_class_members(self, class_sig: str) -> list[dict]:
        """
        Get all members of a class/struct.
        ⊆ members: What's inside this class?
        """
        results = self._db.execute_query("""
            MATCH (c {signature: $sig})-[:CONTAINS]->(m)
            RETURN m.name as name,
                   m.kind as kind,
                   m.signature as signature
        """, {"sig": class_sig})
        return [dict(r) for r in results]

    def get_module_tree(self, root_path: str) -> list[dict]:
        """
        Get hierarchical view of a directory.
        ⊆ tree: What's the structure?
        """
        results = self._db.execute_query("""
            MATCH (f:File)
            WHERE f.path STARTS WITH $root
            OPTIONAL MATCH (f)-[:CONTAINS]->(e)
            RETURN f.path as file,
                   collect(e.name) as entities
            ORDER BY f.path
        """, {"root": root_path})
        return [dict(r) for r in results]

    # ═══════════════════════════════════════════════════════════════
    # ⊥ (Orthogonal) - Conflict Detection
    # ═══════════════════════════════════════════════════════════════

    def find_circular_deps(self) -> list[dict]:
        """
        Find circular dependencies.
        ⊥ conflict: Self-referential loops are bad.
        """
        results = self._db.execute_query("""
            MATCH path = (a:Function)-[:CALLS*2..10]->(a)
            RETURN [n in nodes(path) | n.name] as cycle,
                   length(path) as length
            LIMIT 20
        """)
        return [dict(r) for r in results]

    def find_dead_code(self) -> list[dict]:
        """
        Find functions that are never called.
        ⊥ unused: No incoming Φ edges.
        """
        results = self._db.execute_query("""
            MATCH (f:Function)
            WHERE NOT (f)<-[:CALLS]-()
            AND NOT f.name STARTS WITH '_'
            AND NOT f.name IN ['main', '__init__', 'setUp', 'tearDown']
            RETURN f.signature as function,
                   f.name as name
        """)
        return [dict(r) for r in results]

    def check_container_health(self) -> list[dict]:
        """
        Check for container issues.
        ⊥ health: Is the runtime stable?
        """
        results = self._db.execute_query("""
            MATCH (c:Container)
            WHERE c.cpu_phi > 90 OR c.mem_percent > 90
            RETURN c.name as container,
                   c.cpu_phi as cpu,
                   c.mem_percent as memory,
                   'HIGH_RESOURCE' as issue
            UNION
            MATCH (c:Container)-[:EMITTED]->(e:LogEvent {level: 'ERROR'})
            RETURN c.name as container,
                   0 as cpu,
                   0 as memory,
                   'ERRORS_DETECTED' as issue
        """)
        return [dict(r) for r in results]

    # ═══════════════════════════════════════════════════════════════
    # T (Tensor) - Context Queries
    # ═══════════════════════════════════════════════════════════════

    def get_branch_diff(self, branch1: str, branch2: str) -> list[dict]:
        """
        Compare two branches.
        T context: What's different between contexts?
        """
        results = self._db.execute_query("""
            MATCH (b1:Branch {name: $b1})-[:POINTS_TO]->(c1:Commit)
            MATCH (b2:Branch {name: $b2})-[:POINTS_TO]->(c2:Commit)
            MATCH path = (c1)-[:PARENT_OF*0..50]->(common)<-[:PARENT_OF*0..50]-(c2)
            WITH common, min(length(path)) as dist
            MATCH (c:Commit)-[:TOUCHED]->(f:File)
            WHERE c.timestamp > common.timestamp
            RETURN DISTINCT f.path as changed_file
        """, {"b1": branch1, "b2": branch2})
        return [dict(r) for r in results]

    # ═══════════════════════════════════════════════════════════════
    # Composite Queries
    # ═══════════════════════════════════════════════════════════════

    def analyze_hotspots(self, days: int = 30) -> list[dict]:
        """
        Find risky areas: high change + high coupling.
        τ + Φ combined: Instability indicators.
        """
        import time
        cutoff = int(time.time()) - (days * 86400)

        results = self._db.execute_query("""
            MATCH (c:Commit)-[:TOUCHED]->(f:File)
            WHERE c.timestamp > $cutoff
            WITH f, count(c) as changes
            OPTIONAL MATCH (f)-[:CONTAINS]->(func:Function)-[:CALLS]->(dep)
            WITH f, changes, count(DISTINCT dep) as dependencies
            WHERE changes > 2
            RETURN f.path as file,
                   changes,
                   dependencies,
                   changes * dependencies as risk_score
            ORDER BY risk_score DESC
            LIMIT 20
        """, {"cutoff": cutoff})
        return [dict(r) for r in results]

    def suggest_reviewers(self, file_path: str) -> list[dict]:
        """
        Suggest code reviewers based on file history.
        τ + D: Who should review this?
        """
        results = self._db.execute_query("""
            MATCH (a:Author)-[:AUTHORED]->(c:Commit)-[:TOUCHED]->(f:File {path: $path})
            WITH a, count(c) as commits, max(c.timestamp) as last_touch
            RETURN a.name as reviewer,
                   a.email as email,
                   commits,
                   last_touch
            ORDER BY commits DESC, last_touch DESC
            LIMIT 5
        """, {"path": file_path})
        return [dict(r) for r in results]

    def get_graph_stats(self) -> dict:
        """Get overall graph statistics."""
        stats = {}

        for label in ['File', 'Function', 'Class', 'Commit', 'Author', 'Container']:
            result = self._db.execute_query(f"MATCH (n:{label}) RETURN count(n) as count")
            stats[label.lower() + 's'] = result[0]['count'] if result else 0

        edge_result = self._db.execute_query("""
            MATCH ()-[r]->()
            RETURN count(r) as edges
        """)
        stats['edges'] = edge_result[0]['edges'] if edge_result else 0

        return stats
