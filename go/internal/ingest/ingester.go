// Package ingest provides code ingestion into the graph.
package ingest

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/bmatcuk/doublestar/v4"
	"github.com/joss/urp/internal/domain"
	"github.com/joss/urp/internal/graph"
	"github.com/joss/urp/internal/vector"
)

const batchSize = 100 // Entities per batch write

// ProgressWriter is called with progress updates
type ProgressWriter func(current, total int, file string)

// Ingester builds the knowledge graph from code.
type Ingester struct {
	db       graph.Driver
	registry *Registry
	vectors  vector.Store
	embedder vector.Embedder
	progress ProgressWriter
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

// SetProgress sets the progress callback
func (i *Ingester) SetProgress(p ProgressWriter) {
	i.progress = p
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

	// Load gitignore patterns
	ignorePatterns := loadGitignore(rootPath)

	// Phase 1: Collect all parseable files
	var files []string
	err := filepath.Walk(rootPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		relPath, _ := filepath.Rel(rootPath, path)

		if info.IsDir() {
			name := info.Name()
			if strings.HasPrefix(name, ".") || name == "node_modules" || name == "vendor" || name == "__pycache__" {
				return filepath.SkipDir
			}
			if isIgnored(relPath+"/", ignorePatterns) {
				return filepath.SkipDir
			}
			return nil
		}

		if isIgnored(relPath, ignorePatterns) {
			return nil
		}
		if !i.registry.CanParse(path) {
			return nil
		}
		files = append(files, path)
		return nil
	})
	if err != nil {
		return stats, err
	}

	total := len(files)
	if total == 0 {
		return stats, nil
	}

	// Phase 2: Parse files and collect entities/relationships
	var allEntities []domain.Entity
	var allRelationships []domain.Relationship
	var mu sync.Mutex

	for idx, path := range files {
		// Progress callback
		if i.progress != nil {
			relPath, _ := filepath.Rel(rootPath, path)
			i.progress(idx+1, total, relPath)
		}

		entities, relationships, err := i.registry.ParseFile(path)
		if err != nil {
			stats.Errors++
			continue
		}

		mu.Lock()
		allEntities = append(allEntities, entities...)
		allRelationships = append(allRelationships, relationships...)
		mu.Unlock()
	}

	// Phase 3: Batch write entities
	if i.progress != nil {
		i.progress(total, total, "writing entities...")
	}

	for idx := 0; idx < len(allEntities); idx += batchSize {
		end := idx + batchSize
		if end > len(allEntities) {
			end = len(allEntities)
		}
		batch := allEntities[idx:end]

		if err := i.storeEntitiesBatch(ctx, batch); err != nil {
			stats.Errors += len(batch)
		} else {
			for _, e := range batch {
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
		}
	}

	// Phase 4: Batch write relationships
	if i.progress != nil {
		i.progress(total, total, "writing relationships...")
	}

	for idx := 0; idx < len(allRelationships); idx += batchSize {
		end := idx + batchSize
		if end > len(allRelationships) {
			end = len(allRelationships)
		}
		batch := allRelationships[idx:end]

		if err := i.storeRelationshipsBatch(ctx, batch); err != nil {
			stats.Errors += len(batch)
		} else {
			stats.Relationships += len(batch)
		}
	}

	// Phase 5: Index functions in vector store (async)
	go i.indexEntitiesAsync(allEntities)

	return stats, nil
}

// storeEntitiesBatch writes multiple entities in a single transaction using UNWIND
func (i *Ingester) storeEntitiesBatch(ctx context.Context, entities []domain.Entity) error {
	if len(entities) == 0 {
		return nil
	}

	// Group by type for efficient batch writes
	byType := make(map[string][]map[string]any)
	for _, e := range entities {
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

		byType[label] = append(byType[label], map[string]any{
			"id":         e.ID,
			"name":       e.Name,
			"path":       e.Path,
			"signature":  e.Signature,
			"start_line": e.StartLine,
			"end_line":   e.EndLine,
		})
	}

	// Execute batch for each type
	for label, items := range byType {
		query := fmt.Sprintf(`
			UNWIND $items AS item
			MERGE (e:%s {id: item.id})
			SET e.name = item.name,
			    e.path = item.path,
			    e.signature = item.signature,
			    e.start_line = item.start_line,
			    e.end_line = item.end_line
		`, label)

		if err := i.db.ExecuteWrite(ctx, query, map[string]any{"items": items}); err != nil {
			return err
		}
	}

	return nil
}

// storeRelationshipsBatch writes multiple relationships in a single transaction
func (i *Ingester) storeRelationshipsBatch(ctx context.Context, rels []domain.Relationship) error {
	if len(rels) == 0 {
		return nil
	}

	// Separate CALLS (need Reference nodes) from others
	var callsRels []map[string]any
	var containsRels []map[string]any

	for _, r := range rels {
		item := map[string]any{"from": r.From, "to": r.To}
		if r.Type == "CALLS" {
			callsRels = append(callsRels, item)
		} else if r.Type == "CONTAINS" {
			containsRels = append(containsRels, item)
		}
	}

	// Batch CONTAINS relationships
	if len(containsRels) > 0 {
		query := `
			UNWIND $items AS item
			MATCH (from {id: item.from})
			MATCH (to {id: item.to})
			MERGE (from)-[:CONTAINS]->(to)
		`
		if err := i.db.ExecuteWrite(ctx, query, map[string]any{"items": containsRels}); err != nil {
			return err
		}
	}

	// Batch CALLS relationships (create Reference nodes)
	if len(callsRels) > 0 {
		query := `
			UNWIND $items AS item
			MATCH (from {id: item.from})
			MERGE (ref:Reference {name: item.to})
			MERGE (from)-[:CALLS]->(ref)
		`
		if err := i.db.ExecuteWrite(ctx, query, map[string]any{"items": callsRels}); err != nil {
			return err
		}
	}

	return nil
}

// indexEntitiesAsync indexes functions/methods in vector store
func (i *Ingester) indexEntitiesAsync(entities []domain.Entity) {
	if i.vectors == nil || i.embedder == nil {
		return
	}

	for _, e := range entities {
		if e.Type != domain.EntityFunction && e.Type != domain.EntityMethod {
			continue
		}

		text := fmt.Sprintf("%s %s", e.Type, e.Signature)

		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		vec, err := i.embedder.Embed(ctx, text)
		cancel()

		if err == nil {
			i.vectors.Add(context.Background(), vector.VectorEntry{
				ID:     e.ID,
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
	}
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

// loadGitignore reads .gitignore patterns from the root path.
func loadGitignore(rootPath string) []string {
	var patterns []string

	// Default patterns for common data/binary files
	defaults := []string{
		"*.csv", "*.tsv", "*.parquet", "*.arrow",
		"*.json", "*.jsonl", "*.ndjson",
		"*.pkl", "*.pickle", "*.joblib",
		"*.h5", "*.hdf5", "*.npy", "*.npz",
		"*.db", "*.sqlite", "*.sqlite3",
		"*.tar", "*.tar.gz", "*.tgz", "*.zip", "*.gz", "*.bz2", "*.xz",
		"*.bin", "*.dat", "*.model", "*.ckpt", "*.pt", "*.pth",
		"*.png", "*.jpg", "*.jpeg", "*.gif", "*.ico", "*.svg",
		"*.pdf", "*.doc", "*.docx", "*.xls", "*.xlsx",
		"data/", "datasets/", "models/", "checkpoints/", "logs/",
	}
	patterns = append(patterns, defaults...)

	// Read .gitignore if exists
	gitignorePath := filepath.Join(rootPath, ".gitignore")
	f, err := os.Open(gitignorePath)
	if err != nil {
		return patterns
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		// Skip comments and empty lines
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		patterns = append(patterns, line)
	}

	return patterns
}

// isIgnored checks if a path matches any gitignore pattern.
func isIgnored(path string, patterns []string) bool {
	for _, pattern := range patterns {
		// Handle directory patterns (ending with /)
		if strings.HasSuffix(pattern, "/") {
			dirPattern := strings.TrimSuffix(pattern, "/")
			if strings.HasPrefix(path, dirPattern+"/") || path == dirPattern+"/" {
				return true
			}
			// Also match as glob
			if matched, _ := doublestar.Match("**/"+dirPattern+"/**", path); matched {
				return true
			}
			continue
		}

		// Handle glob patterns
		if strings.Contains(pattern, "*") || strings.Contains(pattern, "?") {
			// Try direct match
			if matched, _ := doublestar.Match(pattern, path); matched {
				return true
			}
			// Try with **/ prefix for patterns without path separator
			if !strings.Contains(pattern, "/") {
				if matched, _ := doublestar.Match("**/"+pattern, path); matched {
					return true
				}
			}
			continue
		}

		// Exact match or prefix match
		if path == pattern || strings.HasPrefix(path, pattern+"/") {
			return true
		}
		// Check if pattern matches anywhere in path
		if strings.Contains(path, "/"+pattern) || strings.HasPrefix(path, pattern) {
			return true
		}
	}
	return false
}
