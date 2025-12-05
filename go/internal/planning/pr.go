package planning

import (
	"fmt"
	"os/exec"
	"strings"
)

// PRResult represents the result of creating a PR.
type PRResult struct {
	URL        string `json:"url"`
	Number     int    `json:"number"`
	Branch     string `json:"branch"`
	BaseBranch string `json:"base_branch"`
}

// CreatePR creates a pull request for a task branch.
// Uses gh CLI to create PR against base branch.
func CreatePR(repoPath, branch, baseBranch, title, body string) (*PRResult, error) {
	// Push branch to remote
	pushCmd := exec.Command("git", "-C", repoPath, "push", "-u", "origin", branch)
	if out, err := pushCmd.CombinedOutput(); err != nil {
		return nil, fmt.Errorf("failed to push branch: %s: %w", string(out), err)
	}

	// Create PR using gh CLI
	args := []string{
		"pr", "create",
		"--base", baseBranch,
		"--head", branch,
		"--title", title,
		"--body", body,
	}

	cmd := exec.Command("gh", args...)
	cmd.Dir = repoPath
	out, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("failed to create PR: %s: %w", string(out), err)
	}

	// gh pr create returns the PR URL
	url := strings.TrimSpace(string(out))

	return &PRResult{
		URL:        url,
		Branch:     branch,
		BaseBranch: baseBranch,
	}, nil
}

// MergePR merges a PR by number.
func MergePR(repoPath string, prNumber int, squash bool) error {
	args := []string{"pr", "merge", fmt.Sprintf("%d", prNumber), "--delete-branch"}
	if squash {
		args = append(args, "--squash")
	} else {
		args = append(args, "--merge")
	}

	cmd := exec.Command("gh", args...)
	cmd.Dir = repoPath
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to merge PR: %s: %w", string(out), err)
	}
	return nil
}

// GetPRStatus gets the status of a PR.
func GetPRStatus(repoPath string, prNumber int) (string, error) {
	cmd := exec.Command("gh", "pr", "view", fmt.Sprintf("%d", prNumber), "--json", "state", "-q", ".state")
	cmd.Dir = repoPath
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

// getBranchForTask gets the current git branch.
func getBranchForTask(repoPath string) string {
	cmd := exec.Command("git", "-C", repoPath, "rev-parse", "--abbrev-ref", "HEAD")
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// hasCommits checks if branch has commits ahead of base.
func hasCommits(repoPath, baseBranch, branch string) bool {
	// git log base..branch --oneline
	cmd := exec.Command("git", "-C", repoPath, "log", fmt.Sprintf("%s..%s", baseBranch, branch), "--oneline")
	out, err := cmd.Output()
	if err != nil {
		return false
	}
	return len(strings.TrimSpace(string(out))) > 0
}
