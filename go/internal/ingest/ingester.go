// Package ingest provides code ingestion into the graph.
package ingest

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/joss/urp/internal/domain"
	"github.com/joss/urp/internal/graph"
)

// Ingester builds the knowledge graph from code.
type Ingester struct {
	db       graph.Driver
	registry *Registry
}

// NewIngester creates a new code ingester.
func NewIngester(db graph.Driver) *Ingester {
	return &Ingester{
		db:       db,
		registry: NewRegistry(),
	}
}

// Stats tracks ingestion statistics.
type Stats struct {
	Files         int `json:"files"`
	Functions     int `json:"functions"`
	Structs       int `json:"structs"`
	Interfaces    int `json:"interfaces"`
	Relationships int `json:"relationships"`
	Errors        int `json:"errors"`
}

// Ingest processes a directory and builds the graph.
func (i *Ingester) Ingest(ctx context.Context, rootPath string) (*Stats, error) {
	stats := &Stats{}

	// Walk directory
	err := filepath.Walk(rootPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// Skip hidden directories and common non-code directories
		if info.IsDir() {
			name := info.Name()
			if strings.HasPrefix(name, ".") || name == "node_modules" || name == "vendor" || name == "__pycache__" {
				return filepath.SkipDir
			}
			return nil
		}

		// Skip non-parseable files
		if !i.registry.CanParse(path) {
			return nil
		}

		// Parse file
		entities, relationships, err := i.registry.ParseFile(path)
		if err != nil {
			stats.Errors++
			return nil // Continue on parse errors
		}

		// Store entities
		for _, e := range entities {
			if err := i.storeEntity(ctx, e); err != nil {
				stats.Errors++
				continue
			}

			switch e.Type {
			case domain.EntityFile:
				stats.Files++
			case domain.EntityFunction, domain.EntityMethod:
				stats.Functions++
			case domain.EntityStruct:
				stats.Structs++
			case domain.EntityInterface:
				stats.Interfaces++
			}
		}

		// Store relationships
		for _, r := range relationships {
			if err := i.storeRelationship(ctx, r); err != nil {
				stats.Errors++
				continue
			}
			stats.Relationships++
		}

		return nil
	})

	return stats, err
}

func (i *Ingester) storeEntity(ctx context.Context, e domain.Entity) error {
	var label string
	switch e.Type {
	case domain.EntityFile:
		label = "File"
	case domain.EntityFunction:
		label = "Function"
	case domain.EntityMethod:
		label = "Method"
	case domain.EntityStruct:
		label = "Struct"
	case domain.EntityInterface:
		label = "Interface"
	case domain.EntityClass:
		label = "Class"
	default:
		label = "Entity"
	}

	query := fmt.Sprintf(`
		MERGE (e:%s {id: $id})
		SET e.name = $name,
		    e.path = $path,
		    e.signature = $signature,
		    e.start_line = $start_line,
		    e.end_line = $end_line
	`, label)

	return i.db.ExecuteWrite(ctx, query, map[string]any{
		"id":         e.ID,
		"name":       e.Name,
		"path":       e.Path,
		"signature":  e.Signature,
		"start_line": e.StartLine,
		"end_line":   e.EndLine,
	})
}

func (i *Ingester) storeRelationship(ctx context.Context, r domain.Relationship) error {
	query := fmt.Sprintf(`
		MATCH (from {id: $from})
		MATCH (to {id: $to})
		MERGE (from)-[r:%s]->(to)
	`, r.Type)

	return i.db.ExecuteWrite(ctx, query, map[string]any{
		"from": r.From,
		"to":   r.To,
	})
}

// LinkCalls resolves call references to actual definitions.
func (i *Ingester) LinkCalls(ctx context.Context) error {
	// Find CALLS relationships pointing to unresolved names
	// and link them to actual Function/Method nodes
	query := `
		MATCH (caller)-[c:CALLS]->(name)
		WHERE name:Reference
		MATCH (target:Function)
		WHERE target.name = name.name OR target.name ENDS WITH '.' + name.name
		MERGE (caller)-[:CALLS]->(target)
		DELETE c
	`
	return i.db.ExecuteWrite(ctx, query, nil)
}
