"""
Code ingester - converts source code to graph nodes using PRU primitives.

Primitives implemented:
  D (Domain): File, Function, Class, Struct existence
  ⊆ (Inclusion): CONTAINS relationships (File→Function, Class→Method)
  Φ (Morphism): CALLS, FLOWS_TO relationships
"""
import os
from database import Database
from parser import ParserRegistry, create_default_registry, CodeEntity


class Ingester:
    """Ingests source code into the knowledge graph."""

    def __init__(self, db: Database, registry: ParserRegistry = None):
        self._db = db
        self._registry = registry or create_default_registry()

    def ingest(self, path: str) -> dict:
        """
        Ingest a codebase into the graph.
        Returns stats about ingestion.
        """
        stats = {"files": 0, "entities": 0, "calls": 0, "errors": []}

        for root, dirs, files in os.walk(path):
            # Skip hidden and common non-code directories
            dirs[:] = [d for d in dirs if not d.startswith('.') and d not in
                       ('node_modules', 'vendor', '__pycache__', 'venv', '.git')]

            for file in files:
                file_path = os.path.join(root, file)
                result = self._ingest_file(file_path)
                if result:
                    stats["files"] += 1
                    stats["entities"] += result.get("entities", 0)
                    stats["calls"] += result.get("calls", 0)
                elif result is False:
                    stats["errors"].append(file_path)

        return stats

    def _ingest_file(self, file_path: str) -> dict | None | bool:
        """
        Ingest a single file.
        Returns stats dict, None if skipped, False if error.
        """
        parser_pair = self._registry.get_for_file(file_path)
        if not parser_pair:
            return None  # Not a supported file type

        ts_parser, lang_parser = parser_pair

        try:
            with open(file_path, 'rb') as f:
                tree = ts_parser.parse(f.read())
        except Exception as e:
            print(f"Parse error {file_path}: {e}")
            return False

        # D: Create file node (Domain existence)
        self._db.execute_query(
            "MERGE (:File {path: $path})",
            {"path": file_path}
        )

        entity_count = 0
        call_count = 0

        # Extract and store entities
        for entity in lang_parser.extract_entities(tree, file_path):
            self._ingest_entity(file_path, entity)
            entity_count += 1

        # Φ: Extract call relationships
        for caller, callee in lang_parser.extract_calls(tree, file_path):
            self._ingest_call(caller, callee)
            call_count += 1

        return {"entities": entity_count, "calls": call_count}

    def _ingest_entity(self, file_path: str, entity: CodeEntity):
        """
        Create entity node and relationships.
        D: Entity existence
        ⊆: Containment hierarchy
        """
        # Map entity kind to label
        label_map = {
            'function': 'Function',
            'method': 'Function',  # Methods are functions with parent
            'class': 'Class',
            'struct': 'Struct',
            'interface': 'Interface',
            'type': 'Type',
        }
        label = label_map.get(entity.kind, 'Entity')

        # D: Create entity node
        # Note: start_line from tree-sitter is 0-indexed, we store 1-indexed for human use
        self._db.execute_query(f"""
            MERGE (e:{label} {{signature: $sig}})
            SET e.name = $name,
                e.kind = $kind,
                e.path = $path,
                e.start_line = $start,
                e.end_line = $end
        """, {
            "sig": entity.signature,
            "name": entity.name,
            "kind": entity.kind,
            "path": file_path,
            "start": entity.start_line + 1,  # Convert 0-indexed to 1-indexed
            "end": entity.end_line + 1,
        })

        # ⊆: File contains entity
        self._db.execute_query(f"""
            MATCH (f:File {{path: $path}}), (e:{label} {{signature: $sig}})
            MERGE (f)-[:CONTAINS]->(e)
        """, {
            "path": file_path,
            "sig": entity.signature,
        })

        # ⊆: Parent contains child (for methods)
        if entity.parent:
            self._db.execute_query(f"""
                MATCH (p {{signature: $parent}}), (e:{label} {{signature: $sig}})
                MERGE (p)-[:CONTAINS]->(e)
            """, {
                "parent": entity.parent,
                "sig": entity.signature,
            })

    def _ingest_call(self, caller: str, callee: str):
        """
        Create call relationship.
        Φ: Causal flow from caller to callee
        """
        # Try to find the callee as a known function
        # If not found, create a reference node
        self._db.execute_query("""
            MATCH (caller:Function {signature: $caller})
            MERGE (callee:Reference {name: $callee_name})
            MERGE (caller)-[:CALLS]->(callee)
        """, {
            "caller": caller,
            "callee_name": callee,
        })

    def link_calls_to_definitions(self):
        """
        Post-processing: Link Reference nodes to actual Function definitions.
        Resolves Φ relationships to concrete entities.
        """
        # Link by exact name match
        self._db.execute_query("""
            MATCH (r:Reference), (f:Function)
            WHERE r.name = f.name
            MERGE (r)-[:RESOLVES_TO]->(f)
        """)

        # Link by suffix match (for qualified calls like module.func)
        self._db.execute_query("""
            MATCH (r:Reference), (f:Function)
            WHERE r.name ENDS WITH f.name
            MERGE (r)-[:MAY_RESOLVE_TO]->(f)
        """)


class IncrementalIngester(Ingester):
    """
    Ingester that only processes changed files.
    Uses file modification time or git status.
    """

    def __init__(self, db: Database, registry: ParserRegistry = None):
        super().__init__(db, registry)
        self._processed_files = set()

    def ingest_changed(self, path: str, since_commit: str = None) -> dict:
        """Ingest only files changed since last run or since commit."""
        stats = {"files": 0, "entities": 0, "calls": 0, "skipped": 0}

        if since_commit:
            changed_files = self._get_git_changed_files(path, since_commit)
        else:
            changed_files = self._get_modified_files(path)

        for file_path in changed_files:
            result = self._ingest_file(file_path)
            if result:
                stats["files"] += 1
                stats["entities"] += result.get("entities", 0)
                stats["calls"] += result.get("calls", 0)
            else:
                stats["skipped"] += 1

        return stats

    def _get_git_changed_files(self, path: str, since_commit: str) -> list[str]:
        """Get files changed since a commit."""
        try:
            import git
            repo = git.Repo(path)
            diffs = repo.commit(since_commit).diff('HEAD')
            return [os.path.join(path, d.a_path or d.b_path) for d in diffs]
        except Exception:
            return []

    def _get_modified_files(self, path: str) -> list[str]:
        """Get files modified based on stored timestamps."""
        modified = []
        for root, dirs, files in os.walk(path):
            dirs[:] = [d for d in dirs if not d.startswith('.')]
            for f in files:
                fp = os.path.join(root, f)
                if self._registry.get_for_file(fp):
                    # Check if file was processed before
                    result = self._db.execute_query("""
                        MATCH (f:File {path: $path})
                        RETURN f.last_ingested as ts
                    """, {"path": fp})
                    if not result or os.path.getmtime(fp) > (result[0].get('ts') or 0):
                        modified.append(fp)
        return modified
