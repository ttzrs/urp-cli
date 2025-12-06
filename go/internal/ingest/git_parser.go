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
)

// GitParser handles reading and parsing git history.
type GitParser struct {
	repoPath string
}

// NewGitParser creates a new git parser.
func NewGitParser(path string) *GitParser {
	return &GitParser{repoPath: path}
}

// ParseCommits reads the git log and returns a channel of commit batches.
// using a channel allows for streaming processing.
func (p *GitParser) ParseCommits(ctx context.Context, maxCommits int) ([]commitBatch, map[string]bool, error) {
	cmd := exec.CommandContext(ctx, "git", "-C", p.repoPath, "log",
		fmt.Sprintf("--max-count=%d", maxCommits),
		"--format=%H|%an|%ae|%at|%s",
		"--name-only",
	)

	output, err := cmd.Output()
	if err != nil {
		return nil, nil, fmt.Errorf("git log failed: %w", err)
	}

	var batches []commitBatch
	authors := make(map[string]bool)
	scanner := bufio.NewScanner(strings.NewReader(string(output)))

	var currentCommit *domain.Commit
	var files []string

	for scanner.Scan() {
		line := scanner.Text()

		if strings.Contains(line, "|") && strings.Count(line, "|") >= 4 {
			if currentCommit != nil {
				batches = append(batches, commitBatch{commit: currentCommit, files: files})
				authors[currentCommit.Author] = true
			}

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

	if currentCommit != nil {
		batches = append(batches, commitBatch{commit: currentCommit, files: files})
		authors[currentCommit.Author] = true
	}

	return batches, authors, nil
}

func (p *GitParser) ParseBranches(ctx context.Context) ([]string, error) {
	cmd := exec.CommandContext(ctx, "git", "-C", p.repoPath, "branch", "-a", "--format=%(refname:short)")
	output, err := cmd.Output()
	if err != nil {
		return nil, err
	}

	var branches []string
	scanner := bufio.NewScanner(strings.NewReader(string(output)))
	for scanner.Scan() {
		branch := strings.TrimSpace(scanner.Text())
		if branch != "" {
			branches = append(branches, branch)
		}
	}
	return branches, nil
}
