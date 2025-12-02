// Package ingest provides code parsing and graph building.
package ingest

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"

	"github.com/joss/urp/internal/domain"
)

// Parser extracts entities from source code.
type Parser interface {
	// Extensions returns file extensions this parser handles.
	Extensions() []string

	// Parse extracts entities from a file.
	Parse(path string, content []byte) ([]domain.Entity, []domain.Relationship, error)
}

// GoParser parses Go source files using the native AST.
type GoParser struct{}

// NewGoParser creates a new Go parser.
func NewGoParser() *GoParser {
	return &GoParser{}
}

// Extensions returns Go file extensions.
func (p *GoParser) Extensions() []string {
	return []string{".go"}
}

// Parse extracts entities and relationships from Go source.
func (p *GoParser) Parse(path string, content []byte) ([]domain.Entity, []domain.Relationship, error) {
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, path, content, parser.ParseComments)
	if err != nil {
		return nil, nil, err
	}

	var entities []domain.Entity
	var relationships []domain.Relationship

	// File entity
	fileEntity := domain.Entity{
		ID:   path,
		Type: domain.EntityFile,
		Name: filepath.Base(path),
		Path: path,
	}
	entities = append(entities, fileEntity)

	// Walk AST
	ast.Inspect(f, func(n ast.Node) bool {
		switch decl := n.(type) {
		case *ast.FuncDecl:
			// Function or method
			name := decl.Name.Name
			signature := buildFuncSignature(decl)
			entityType := domain.EntityFunction

			// Check if it's a method
			if decl.Recv != nil && len(decl.Recv.List) > 0 {
				entityType = domain.EntityMethod
				// Get receiver type
				if t := getReceiverType(decl.Recv.List[0].Type); t != "" {
					name = t + "." + name
				}
			}

			startLine := fset.Position(decl.Pos()).Line
			endLine := fset.Position(decl.End()).Line

			funcEntity := domain.Entity{
				ID:        path + "::" + name,
				Type:      entityType,
				Name:      name,
				Path:      path,
				Signature: signature,
				StartLine: startLine,
				EndLine:   endLine,
			}
			entities = append(entities, funcEntity)

			// File CONTAINS Function
			relationships = append(relationships, domain.Relationship{
				From: path,
				To:   funcEntity.ID,
				Type: "CONTAINS",
			})

			// Extract calls
			calls := extractCalls(decl.Body)
			for _, call := range calls {
				relationships = append(relationships, domain.Relationship{
					From: funcEntity.ID,
					To:   call,
					Type: "CALLS",
				})
			}

		case *ast.TypeSpec:
			// Struct or interface
			name := decl.Name.Name
			var entityType domain.EntityType

			switch decl.Type.(type) {
			case *ast.StructType:
				entityType = domain.EntityStruct
			case *ast.InterfaceType:
				entityType = domain.EntityInterface
			default:
				return true
			}

			startLine := fset.Position(decl.Pos()).Line
			endLine := fset.Position(decl.Type.End()).Line

			typeEntity := domain.Entity{
				ID:        path + "::" + name,
				Type:      entityType,
				Name:      name,
				Path:      path,
				StartLine: startLine,
				EndLine:   endLine,
			}
			entities = append(entities, typeEntity)

			// File CONTAINS Type
			relationships = append(relationships, domain.Relationship{
				From: path,
				To:   typeEntity.ID,
				Type: "CONTAINS",
			})
		}
		return true
	})

	return entities, relationships, nil
}

func buildFuncSignature(decl *ast.FuncDecl) string {
	var params []string
	if decl.Type.Params != nil {
		for _, p := range decl.Type.Params.List {
			typeStr := exprToString(p.Type)
			for range p.Names {
				params = append(params, typeStr)
			}
			if len(p.Names) == 0 {
				params = append(params, typeStr)
			}
		}
	}

	var results []string
	if decl.Type.Results != nil {
		for _, r := range decl.Type.Results.List {
			typeStr := exprToString(r.Type)
			for range r.Names {
				results = append(results, typeStr)
			}
			if len(r.Names) == 0 {
				results = append(results, typeStr)
			}
		}
	}

	sig := decl.Name.Name + "(" + strings.Join(params, ", ") + ")"
	if len(results) > 0 {
		if len(results) == 1 {
			sig += " " + results[0]
		} else {
			sig += " (" + strings.Join(results, ", ") + ")"
		}
	}
	return sig
}

func getReceiverType(expr ast.Expr) string {
	switch t := expr.(type) {
	case *ast.Ident:
		return t.Name
	case *ast.StarExpr:
		if id, ok := t.X.(*ast.Ident); ok {
			return id.Name
		}
	}
	return ""
}

func exprToString(expr ast.Expr) string {
	switch t := expr.(type) {
	case *ast.Ident:
		return t.Name
	case *ast.StarExpr:
		return "*" + exprToString(t.X)
	case *ast.ArrayType:
		return "[]" + exprToString(t.Elt)
	case *ast.MapType:
		return "map[" + exprToString(t.Key) + "]" + exprToString(t.Value)
	case *ast.SelectorExpr:
		return exprToString(t.X) + "." + t.Sel.Name
	case *ast.InterfaceType:
		return "interface{}"
	case *ast.FuncType:
		return "func"
	case *ast.ChanType:
		return "chan " + exprToString(t.Value)
	case *ast.Ellipsis:
		return "..." + exprToString(t.Elt)
	}
	return "any"
}

func extractCalls(body *ast.BlockStmt) []string {
	if body == nil {
		return nil
	}

	var calls []string
	seen := make(map[string]bool)

	ast.Inspect(body, func(n ast.Node) bool {
		if call, ok := n.(*ast.CallExpr); ok {
			name := getCallName(call.Fun)
			if name != "" && !seen[name] {
				calls = append(calls, name)
				seen[name] = true
			}
		}
		return true
	})

	return calls
}

func getCallName(expr ast.Expr) string {
	switch e := expr.(type) {
	case *ast.Ident:
		return e.Name
	case *ast.SelectorExpr:
		// pkg.Func or obj.Method
		if x, ok := e.X.(*ast.Ident); ok {
			return x.Name + "." + e.Sel.Name
		}
		return e.Sel.Name
	}
	return ""
}

// Registry manages multiple parsers.
type Registry struct {
	parsers map[string]Parser
}

// NewRegistry creates a parser registry.
func NewRegistry() *Registry {
	r := &Registry{
		parsers: make(map[string]Parser),
	}
	// Register default parsers
	r.Register(NewGoParser())
	return r
}

// Register adds a parser to the registry.
func (r *Registry) Register(p Parser) {
	for _, ext := range p.Extensions() {
		r.parsers[ext] = p
	}
}

// GetParser returns a parser for the given file extension.
func (r *Registry) GetParser(ext string) Parser {
	return r.parsers[ext]
}

// CanParse returns true if the registry can parse the file.
func (r *Registry) CanParse(path string) bool {
	ext := filepath.Ext(path)
	_, ok := r.parsers[ext]
	return ok
}

// ParseFile parses a file using the appropriate parser.
func (r *Registry) ParseFile(path string) ([]domain.Entity, []domain.Relationship, error) {
	ext := filepath.Ext(path)
	p := r.parsers[ext]
	if p == nil {
		return nil, nil, nil // Skip unsupported files
	}

	content, err := os.ReadFile(path)
	if err != nil {
		return nil, nil, err
	}

	return p.Parse(path, content)
}
