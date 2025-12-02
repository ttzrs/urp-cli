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

// LoadHistory ingests git commit history.
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

	scanner := bufio.NewScanner(strings.NewReader(string(output)))
	var currentCommit *domain.Commit
	var files []string

	for scanner.Scan() {
		line := scanner.Text()

		if strings.Contains(line, "|") && strings.Count(line, "|") >= 4 {
			// Save previous commit
			if currentCommit != nil {
				if err := g.storeCommit(ctx, currentCommit, files); err == nil {
					stats.Commits++
					authors[currentCommit.Author] = true
				}
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
			// File changed in this commit
			files = append(files, line)
		}
	}

	// Save last commit
	if currentCommit != nil {
		if err := g.storeCommit(ctx, currentCommit, files); err == nil {
			stats.Commits++
			authors[currentCommit.Author] = true
		}
	}

	stats.Authors = len(authors)

	// Load branches
	branches, err := g.loadBranches(ctx)
	if err == nil {
		stats.Branches = branches
	}

	return stats, nil
}

func (g *GitLoader) storeCommit(ctx context.Context, c *domain.Commit, files []string) error {
	// Create commit node
	query := `
		MERGE (c:Commit {hash: $hash})
		SET c.message = $message,
		    c.timestamp = $timestamp,
		    c.datetime = $datetime

		MERGE (a:Author {email: $email})
		SET a.name = $author

		MERGE (a)-[:AUTHORED]->(c)
	`

	err := g.db.ExecuteWrite(ctx, query, map[string]any{
		"hash":      c.Hash,
		"message":   truncate(c.Message, 200),
		"timestamp": c.Timestamp.Unix(),
		"datetime":  c.Timestamp.Format(time.RFC3339),
		"author":    c.Author,
		"email":     c.Email,
	})
	if err != nil {
		return err
	}

	// Link to files
	for _, file := range files {
		fileQuery := `
			MATCH (c:Commit {hash: $hash})
			MERGE (f:File {path: $path})
			SET f.name = $name
			MERGE (c)-[:TOUCHED]->(f)
		`
		_ = g.db.ExecuteWrite(ctx, fileQuery, map[string]any{
			"hash": c.Hash,
			"path": file,
			"name": lastSegment(file),
		})
	}

	return nil
}

func (g *GitLoader) loadBranches(ctx context.Context) (int, error) {
	cmd := exec.CommandContext(ctx, "git", "-C", g.path, "branch", "-a", "--format=%(refname:short)")
	output, err := cmd.Output()
	if err != nil {
		return 0, err
	}

	count := 0
	scanner := bufio.NewScanner(strings.NewReader(string(output)))
	for scanner.Scan() {
		branch := strings.TrimSpace(scanner.Text())
		if branch == "" {
			continue
		}

		query := `MERGE (b:Branch {name: $name})`
		if err := g.db.ExecuteWrite(ctx, query, map[string]any{"name": branch}); err == nil {
			count++
		}
	}

	return count, nil
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
