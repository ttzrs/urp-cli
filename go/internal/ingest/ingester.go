// Package ingest provides code ingestion into the graph.
package ingest

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/joss/urp/internal/domain"
	"github.com/joss/urp/internal/graph"
	"github.com/joss/urp/internal/vector"
)

// Ingester builds the knowledge graph from code.
type Ingester struct {
	db       graph.Driver
	registry *Registry
	vectors  vector.Store
	embedder vector.Embedder
}

// NewIngester creates a new code ingester.
func NewIngester(db graph.Driver) *Ingester {
	return &Ingester{
		db:       db,
		registry: NewRegistry(),
		vectors:  vector.Default(),
		embedder: vector.GetDefaultEmbedder(),
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

// storeEntity saves an entity to the graph and vector store.
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

	err := i.db.ExecuteWrite(ctx, query, map[string]any{
		"id":         e.ID,
		"name":       e.Name,
		"path":       e.Path,
		"signature":  e.Signature,
		"start_line": e.StartLine,
		"end_line":   e.EndLine,
	})
	if err != nil {
		return err
	}

	// Index code entities (functions/methods) in vector store
	// We only index if there's code content (requires reading file which we don't have here)
	// For now, we'll just index the signature/name as a placeholder or read the file snippet if possible
	// A better approach would be to pass content down from ParseFile
	
	// Only index functions/methods for now
	if e.Type == domain.EntityFunction || e.Type == domain.EntityMethod {
		if i.vectors != nil && i.embedder != nil {
			// Construct a meaningful representation
			text := fmt.Sprintf("%s %s", e.Type, e.Signature)
			
			// Asynchronously embed to not block ingestion heavily
			// In a real production system, this would be a background job
			go func() {
				ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
				defer cancel()
				
				vec, err := i.embedder.Embed(ctx, text)
				if err == nil {
					i.vectors.Add(ctx, vector.VectorEntry{
						ID:     e.ID, // Use same ID as graph node
						Text:   text,
						Vector: vec,
						Kind:   "code",
						Metadata: map[string]string{
							"path":      e.Path,
							"name":      e.Name,
							"signature": e.Signature,
						},
					})
				}
			}()
		}
	}

	return nil
}

func (i *Ingester) storeRelationship(ctx context.Context, r domain.Relationship) error {
	// For CALLS relationships, create Reference node if target doesn't exist
	// This allows LinkCalls() to resolve them later
	if r.Type == "CALLS" {
		query := `
			MATCH (from {id: $from})
			MERGE (ref:Reference {name: $to})
			MERGE (from)-[:CALLS]->(ref)
		`
		return i.db.ExecuteWrite(ctx, query, map[string]any{
			"from": r.From,
			"to":   r.To,
		})
	}

	// Standard relationship - both nodes must exist
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
	// First, link to Function nodes (exact name match or after dot)
	// e.g., "NewQuerier" matches "NewQuerier", "query.NewQuerier" matches "NewQuerier"
	queryFuncs := `
		MATCH (caller)-[c:CALLS]->(ref:Reference)
		WITH caller, c, ref,
		     CASE WHEN ref.name CONTAINS '.'
		          THEN split(ref.name, '.')[-1]
		          ELSE ref.name END AS funcName
		MATCH (target:Function)
		WHERE target.name = funcName
		MERGE (caller)-[:CALLS]->(target)
	`
	if err := i.db.ExecuteWrite(ctx, queryFuncs, nil); err != nil {
		return fmt.Errorf("linking functions: %w", err)
	}

	// Also link to Method nodes
	queryMethods := `
		MATCH (caller)-[c:CALLS]->(ref:Reference)
		WITH caller, c, ref,
		     CASE WHEN ref.name CONTAINS '.'
		          THEN split(ref.name, '.')[-1]
		          ELSE ref.name END AS methodName
		MATCH (target:Method)
		WHERE target.name = methodName
		MERGE (caller)-[:CALLS]->(target)
	`
	if err := i.db.ExecuteWrite(ctx, queryMethods, nil); err != nil {
		return fmt.Errorf("linking methods: %w", err)
	}

	// Clean up resolved references (optional - keep for debugging)
	// DELETE is commented out to allow inspection
	return nil
}
