// Package ingest provides git history loading.
package ingest

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/joss/urp/internal/domain"
	"github.com/joss/urp/internal/graph"
	urpstrings "github.com/joss/urp/internal/strings"
)

// GitLoader loads git history into the graph.
type GitLoader struct {
	db   graph.Driver
	path string
}

// NewGitLoader creates a new git loader.
func NewGitLoader(db graph.Driver, path string) *GitLoader {
	return &GitLoader{
		db:   db,
		path: path,
	}
}

// GitStats tracks git loading statistics.
type GitStats struct {
	Commits  int `json:"commits"`
	Authors  int `json:"authors"`
	Branches int `json:"branches"`
}

// commitBatch holds commits for batch insertion.
type commitBatch struct {
	commit *domain.Commit
	files  []string
}

// LoadHistory ingests git commit history.
// Optimized: batches commits and uses UNWIND for bulk insertion.
func (g *GitLoader) LoadHistory(ctx context.Context, maxCommits int) (*GitStats, error) {
	stats := &GitStats{}

	// 1. Parsing Phase (Delegated to GitParser - SRP)
	parser := NewGitParser(g.path)
	batches, authors, err := parser.ParseCommits(ctx, maxCommits)
	if err != nil {
		return nil, fmt.Errorf("parsing commits: %w", err)
	}

	// 2. Storage Phase
	if err := g.storeBatch(ctx, batches); err != nil {
		return nil, err
	}

	stats.Commits = len(batches)
	stats.Authors = len(authors)

	// 3. Branches Phase
	branches, err := parser.ParseBranches(ctx)
	if err != nil {
		return nil, fmt.Errorf("parsing branches: %w", err)
	}

	if err := g.storeBranches(ctx, branches); err != nil {
		return nil, err
	}
	stats.Branches = len(branches)

	return stats, nil
}

const batchChunkSize = 500 // Process in chunks to avoid memory issues

// storeBatch inserts all commits in a single transaction using UNWIND.
// For large repos, processes in chunks of 500 commits.
func (g *GitLoader) storeBatch(ctx context.Context, batches []commitBatch) error {
	if len(batches) == 0 {
		return nil
	}

	// Process in chunks for large repos
	for i := 0; i < len(batches); i += batchChunkSize {
		end := i + batchChunkSize
		if end > len(batches) {
			end = len(batches)
		}
		if err := g.storeChunk(ctx, batches[i:end]); err != nil {
			return err
		}
	}
	return nil
}

// storeChunk inserts a chunk of commits using UNWIND.
func (g *GitLoader) storeChunk(ctx context.Context, batches []commitBatch) error {
	// Build commits list for UNWIND
	commits := make([]map[string]any, 0, len(batches))
	for _, b := range batches {
		commits = append(commits, map[string]any{
			"hash":      b.commit.Hash,
			"message":   urpstrings.TruncateNoEllipsis(b.commit.Message, 200),
			"timestamp": b.commit.Timestamp.Unix(),
			"datetime":  b.commit.Timestamp.Format(time.RFC3339),
			"author":    b.commit.Author,
			"email":     b.commit.Email,
		})
	}

	// Single query for all commits + authors
	query := `
		UNWIND $commits AS c
		MERGE (commit:Commit {hash: c.hash})
		SET commit.message = c.message,
		    commit.timestamp = c.timestamp,
		    commit.datetime = c.datetime
		MERGE (author:Author {email: c.email})
		SET author.name = c.author
		MERGE (author)-[:AUTHORED]->(commit)
	`

	if err := g.db.ExecuteWrite(ctx, query, map[string]any{"commits": commits}); err != nil {
		return fmt.Errorf("batch commit insert: %w", err)
	}

	// Build file touches list
	var touches []map[string]any
	for _, b := range batches {
		for _, file := range b.files {
			touches = append(touches, map[string]any{
				"hash": b.commit.Hash,
				"path": file,
				"name": lastSegment(file),
			})
		}
	}

	if len(touches) == 0 {
		return nil
	}

	// Single query for all file touches
	fileQuery := `
		UNWIND $touches AS t
		MATCH (c:Commit {hash: t.hash})
		MERGE (f:File {path: t.path})
		SET f.name = t.name
		MERGE (c)-[:TOUCHED]->(f)
	`

	if err := g.db.ExecuteWrite(ctx, fileQuery, map[string]any{"touches": touches}); err != nil {
		return fmt.Errorf("batch file touches: %w", err)
	}

	return nil
}

func (g *GitLoader) storeBranches(ctx context.Context, branches []string) error {
	if len(branches) == 0 {
		return nil
	}

	// Prepare data for UNWIND
	var branchMaps []map[string]any
	for _, b := range branches {
		branchMaps = append(branchMaps, map[string]any{"name": b})
	}

	// Single UNWIND query for all branches
	query := `
		UNWIND $branches AS b
		MERGE (:Branch {name: b.name})
	`
	if err := g.db.ExecuteWrite(ctx, query, map[string]any{"branches": branchMaps}); err != nil {
		return fmt.Errorf("store branches: %w", err)
	}

	return nil
}

// GenerateCoEvolutionWeights links files that are modified in the same commit.
// It creates CO_CHANGED_WITH relationships and updates their weights.
func (g *GitLoader) GenerateCoEvolutionWeights(ctx context.Context) error {
	// This query finds pairs of files modified in the same commit
	// and creates/updates a weighted relationship between them.
	// We use id(f1) < id(f2) to avoid double counting and self-loops.
	query := `
		MATCH (c:Commit)-[:TOUCHED]->(f1:File)
		MATCH (c)-[:TOUCHED]->(f2:File)
		WHERE id(f1) < id(f2)
		MERGE (f1)-[r:CO_CHANGED_WITH]-(f2)
		ON CREATE SET r.weight = 1
		ON MATCH SET r.weight = r.weight + 1
	`
	if err := g.db.ExecuteWrite(ctx, query, nil); err != nil {
		return fmt.Errorf("generating co-evolution weights: %w", err)
	}
	return nil
}

func lastSegment(path string) string {
	parts := strings.Split(path, "/")
	return parts[len(parts)-1]
}
