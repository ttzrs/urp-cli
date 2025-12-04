// Package ingest provides git history loading.
package ingest

import (
	"bufio"
	"context"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/joss/urp/internal/domain"
	"github.com/joss/urp/internal/graph"
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
	authors := make(map[string]bool)

	// Get commit log
	cmd := exec.CommandContext(ctx, "git", "-C", g.path, "log",
		fmt.Sprintf("--max-count=%d", maxCommits),
		"--format=%H|%an|%ae|%at|%s",
		"--name-only",
	)

	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("git log failed: %w", err)
	}

	// Parse all commits first
	var batches []commitBatch
	scanner := bufio.NewScanner(strings.NewReader(string(output)))
	var currentCommit *domain.Commit
	var files []string

	for scanner.Scan() {
		line := scanner.Text()

		if strings.Contains(line, "|") && strings.Count(line, "|") >= 4 {
			// Save previous commit
			if currentCommit != nil {
				batches = append(batches, commitBatch{commit: currentCommit, files: files})
				authors[currentCommit.Author] = true
			}

			// Parse new commit
			parts := strings.SplitN(line, "|", 5)
			if len(parts) < 5 {
				continue
			}

			timestamp, _ := strconv.ParseInt(parts[3], 10, 64)
			currentCommit = &domain.Commit{
				Hash:      parts[0],
				Author:    parts[1],
				Email:     parts[2],
				Timestamp: time.Unix(timestamp, 0),
				Message:   parts[4],
			}
			files = nil

		} else if line != "" && currentCommit != nil {
			files = append(files, line)
		}
	}

	// Save last commit
	if currentCommit != nil {
		batches = append(batches, commitBatch{commit: currentCommit, files: files})
		authors[currentCommit.Author] = true
	}

	// Batch insert all commits
	if err := g.storeBatch(ctx, batches); err != nil {
		return nil, err
	}

	stats.Commits = len(batches)
	stats.Authors = len(authors)

	// Load branches
	branches, err := g.loadBranches(ctx)
	if err == nil {
		stats.Branches = branches
	}

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
			"message":   truncate(b.commit.Message, 200),
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

func (g *GitLoader) loadBranches(ctx context.Context) (int, error) {
	cmd := exec.CommandContext(ctx, "git", "-C", g.path, "branch", "-a", "--format=%(refname:short)")
	output, err := cmd.Output()
	if err != nil {
		return 0, err
	}

	// Collect all branches
	var branches []map[string]any
	scanner := bufio.NewScanner(strings.NewReader(string(output)))
	for scanner.Scan() {
		branch := strings.TrimSpace(scanner.Text())
		if branch == "" {
			continue
		}
		branches = append(branches, map[string]any{"name": branch})
	}

	if len(branches) == 0 {
		return 0, nil
	}

	// Single UNWIND query for all branches
	query := `
		UNWIND $branches AS b
		MERGE (:Branch {name: b.name})
	`
	if err := g.db.ExecuteWrite(ctx, query, map[string]any{"branches": branches}); err != nil {
		return 0, err
	}

	return len(branches), nil
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}

func lastSegment(path string) string {
	parts := strings.Split(path, "/")
	return parts[len(parts)-1]
}
