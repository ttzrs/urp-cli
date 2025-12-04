"""
Multi-language parser architecture using tree-sitter.
Extensible registry pattern - add new languages by implementing LanguageParser.
"""
from abc import ABC, abstractmethod
from dataclasses import dataclass
from typing import Iterator, Optional
from tree_sitter import Language, Parser, Node

# Language modules - lazy imports
_LANGUAGE_MODULES = {
    'python': 'tree_sitter_python',
    'go': 'tree_sitter_go',
}


@dataclass
class CodeEntity:
    """Unified representation of code entities across languages."""
    kind: str  # 'function', 'class', 'method', 'struct', 'interface'
    name: str
    signature: str
    start_line: int
    end_line: int
    parent: Optional[str] = None  # For methods inside classes/structs


class LanguageParser(ABC):
    """Abstract base for language-specific parsers."""

    @property
    @abstractmethod
    def extensions(self) -> tuple[str, ...]:
        """File extensions this parser handles."""
        pass

    @abstractmethod
    def extract_entities(self, tree, file_path: str) -> Iterator[CodeEntity]:
        """Extract code entities from AST."""
        pass

    @abstractmethod
    def extract_calls(self, tree, file_path: str) -> Iterator[tuple[str, str]]:
        """Extract function calls as (caller, callee) pairs."""
        pass


class PythonParser(LanguageParser):
    """Python-specific AST extraction."""

    @property
    def extensions(self) -> tuple[str, ...]:
        return ('.py',)

    def extract_entities(self, tree, file_path: str) -> Iterator[CodeEntity]:
        for node in tree.root_node.children:
            if node.type == 'function_definition':
                name = node.child_by_field_name('name').text.decode('utf8')
                yield CodeEntity(
                    kind='function',
                    name=name,
                    signature=f"{file_path}::{name}",
                    start_line=node.start_point[0],
                    end_line=node.end_point[0],
                )
            elif node.type == 'class_definition':
                class_name = node.child_by_field_name('name').text.decode('utf8')
                yield CodeEntity(
                    kind='class',
                    name=class_name,
                    signature=f"{file_path}::{class_name}",
                    start_line=node.start_point[0],
                    end_line=node.end_point[0],
                )
                # Extract methods
                for child in self._find_methods(node):
                    method_name = child.child_by_field_name('name').text.decode('utf8')
                    yield CodeEntity(
                        kind='method',
                        name=method_name,
                        signature=f"{file_path}::{class_name}.{method_name}",
                        start_line=child.start_point[0],
                        end_line=child.end_point[0],
                        parent=f"{file_path}::{class_name}",
                    )

    def _find_methods(self, class_node: Node) -> Iterator[Node]:
        body = class_node.child_by_field_name('body')
        if body:
            for child in body.children:
                if child.type == 'function_definition':
                    yield child

    def extract_calls(self, tree, file_path: str) -> Iterator[tuple[str, str]]:
        """Extract call relationships from AST."""
        current_func = None
        for node in self._walk(tree.root_node):
            if node.type == 'function_definition':
                name_node = node.child_by_field_name('name')
                if name_node:
                    current_func = f"{file_path}::{name_node.text.decode('utf8')}"
            elif node.type == 'call' and current_func:
                func_node = node.child_by_field_name('function')
                if func_node:
                    callee = self._extract_callee_name(func_node)
                    if callee:
                        yield (current_func, callee)

    def _extract_callee_name(self, node: Node) -> Optional[str]:
        if node.type == 'identifier':
            return node.text.decode('utf8')
        elif node.type == 'attribute':
            return node.text.decode('utf8')
        return None

    def _walk(self, node: Node) -> Iterator[Node]:
        yield node
        for child in node.children:
            yield from self._walk(child)


class GoParser(LanguageParser):
    """Go-specific AST extraction."""

    @property
    def extensions(self) -> tuple[str, ...]:
        return ('.go',)

    def extract_entities(self, tree, file_path: str) -> Iterator[CodeEntity]:
        for node in tree.root_node.children:
            if node.type == 'function_declaration':
                name = node.child_by_field_name('name')
                if name:
                    func_name = name.text.decode('utf8')
                    yield CodeEntity(
                        kind='function',
                        name=func_name,
                        signature=f"{file_path}::{func_name}",
                        start_line=node.start_point[0],
                        end_line=node.end_point[0],
                    )
            elif node.type == 'method_declaration':
                name = node.child_by_field_name('name')
                receiver = node.child_by_field_name('receiver')
                if name and receiver:
                    method_name = name.text.decode('utf8')
                    recv_type = self._extract_receiver_type(receiver)
                    yield CodeEntity(
                        kind='method',
                        name=method_name,
                        signature=f"{file_path}::{recv_type}.{method_name}",
                        start_line=node.start_point[0],
                        end_line=node.end_point[0],
                        parent=f"{file_path}::{recv_type}",
                    )
            elif node.type == 'type_declaration':
                for spec in self._find_type_specs(node):
                    yield spec

    def _extract_receiver_type(self, receiver: Node) -> str:
        for child in self._walk(receiver):
            if child.type == 'type_identifier':
                return child.text.decode('utf8')
        return "unknown"

    def _find_type_specs(self, node: Node) -> Iterator[CodeEntity]:
        for child in node.children:
            if child.type == 'type_spec':
                name = child.child_by_field_name('name')
                type_def = child.child_by_field_name('type')
                if name and type_def:
                    type_name = name.text.decode('utf8')
                    kind = 'struct' if type_def.type == 'struct_type' else 'interface' if type_def.type == 'interface_type' else 'type'
                    yield CodeEntity(
                        kind=kind,
                        name=type_name,
                        signature=f"{file_path}::{type_name}",
                        start_line=node.start_point[0],
                        end_line=node.end_point[0],
                    )

    def extract_calls(self, tree, file_path: str) -> Iterator[tuple[str, str]]:
        current_func = None
        for node in self._walk(tree.root_node):
            if node.type in ('function_declaration', 'method_declaration'):
                name_node = node.child_by_field_name('name')
                if name_node:
                    current_func = f"{file_path}::{name_node.text.decode('utf8')}"
            elif node.type == 'call_expression' and current_func:
                func_node = node.child_by_field_name('function')
                if func_node:
                    callee = self._extract_callee_name(func_node)
                    if callee:
                        yield (current_func, callee)

    def _extract_callee_name(self, node: Node) -> Optional[str]:
        if node.type == 'identifier':
            return node.text.decode('utf8')
        elif node.type == 'selector_expression':
            return node.text.decode('utf8')
        return None

    def _walk(self, node: Node) -> Iterator[Node]:
        yield node
        for child in node.children:
            yield from self._walk(child)


class ParserRegistry:
    """Registry of language parsers. Extensible - just register new parsers."""

    def __init__(self):
        self._parsers: dict[str, tuple[Parser, LanguageParser]] = {}
        self._ext_map: dict[str, str] = {}

    def register(self, lang: str, parser_impl: LanguageParser):
        """Register a language parser."""
        try:
            module = __import__(_LANGUAGE_MODULES[lang], fromlist=['language'])
            ts_parser = Parser(Language(module.language()))
            self._parsers[lang] = (ts_parser, parser_impl)
            for ext in parser_impl.extensions:
                self._ext_map[ext] = lang
        except (ImportError, KeyError) as e:
            print(f"Warning: Could not load {lang} parser: {e}")

    def get_for_file(self, file_path: str) -> Optional[tuple[Parser, LanguageParser]]:
        """Get parser for file based on extension."""
        for ext, lang in self._ext_map.items():
            if file_path.endswith(ext):
                return self._parsers.get(lang)
        return None

    def parse(self, file_path: str):
        """Parse a file and return the AST tree."""
        parser_pair = self.get_for_file(file_path)
        if not parser_pair:
            return None
        ts_parser, _ = parser_pair
        with open(file_path, 'rb') as f:
            return ts_parser.parse(f.read())


# Default registry with Python and Go
def create_default_registry() -> ParserRegistry:
    registry = ParserRegistry()
    registry.register('python', PythonParser())
    try:
        registry.register('go', GoParser())
    except Exception:
        pass  # Go parser optional
    return registry


# Backwards compatibility
class CodeParser:
    """Legacy interface - wraps ParserRegistry for Python only."""
    def __init__(self):
        self._registry = create_default_registry()

    def parse(self, file_path: str):
        return self._registry.parse(file_path)
