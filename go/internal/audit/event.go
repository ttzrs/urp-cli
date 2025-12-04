// Package audit provides structured logging and auditing for URP operations.
package audit

import (
	"os/exec"
	"strings"
	"time"
)

// Category represents the type of operation being audited.
type Category string

const (
	CategoryCode        Category = "code"
	CategoryOrchestrate Category = "orchestrate"
	CategoryEvents      Category = "events"
	CategoryMemory      Category = "memory"
	CategoryKnowledge   Category = "knowledge"
	CategorySystem      Category = "system"
	CategoryGit         Category = "git"
	CategoryInfra       Category = "infra"
	CategoryCognitive   Category = "cognitive"
	CategorySkills      Category = "skills"
)

// Status represents the outcome of an operation.
type Status string

const (
	StatusSuccess Status = "success"
	StatusError   Status = "error"
	StatusWarning Status = "warning"
	StatusTimeout Status = "timeout"
)

// AuditEvent represents a single auditable operation.
type AuditEvent struct {
	EventID string `json:"event_id"`

	// Operation details
	Category  Category `json:"category"`
	Operation string   `json:"operation"`
	Command   string   `json:"command,omitempty"`

	// Result
	Status       Status `json:"status"`
	ExitCode     int    `json:"exit_code,omitempty"`
	ErrorMessage string `json:"error_message,omitempty"`
	OutputSize   int    `json:"output_size,omitempty"`

	// Timing
	StartedAt   time.Time     `json:"started_at"`
	CompletedAt time.Time     `json:"completed_at,omitempty"`
	DurationMs  int64         `json:"duration_ms,omitempty"`
	Duration    time.Duration `json:"-"`

	// Git context
	Git GitContext `json:"git"`

	// Session context
	SessionID string `json:"session_id,omitempty"`
	WorkerID  string `json:"worker_id,omitempty"`
	Project   string `json:"project,omitempty"`
}

// GitContext holds git-related context for an audit event.
type GitContext struct {
	CommitHash  string `json:"commit_hash,omitempty"`
	CommitShort string `json:"commit_short,omitempty"`
	Branch      string `json:"branch,omitempty"`
	IsDirty     bool   `json:"is_dirty"`
	Author      string `json:"author,omitempty"`
	RepoRoot    string `json:"repo_root,omitempty"`
}

// GetGitContext captures the current git state.
func GetGitContext() GitContext {
	ctx := GitContext{}

	// Get commit hash
	if out, err := exec.Command("git", "rev-parse", "HEAD").Output(); err == nil {
		ctx.CommitHash = strings.TrimSpace(string(out))
		if len(ctx.CommitHash) >= 7 {
			ctx.CommitShort = ctx.CommitHash[:7]
		}
	}

	// Get branch name
	if out, err := exec.Command("git", "rev-parse", "--abbrev-ref", "HEAD").Output(); err == nil {
		ctx.Branch = strings.TrimSpace(string(out))
	}

	// Check if working tree is dirty
	if out, err := exec.Command("git", "status", "--porcelain").Output(); err == nil {
		ctx.IsDirty = len(strings.TrimSpace(string(out))) > 0
	}

	// Get author of HEAD commit
	if out, err := exec.Command("git", "log", "-1", "--format=%an").Output(); err == nil {
		ctx.Author = strings.TrimSpace(string(out))
	}

	// Get repo root
	if out, err := exec.Command("git", "rev-parse", "--show-toplevel").Output(); err == nil {
		ctx.RepoRoot = strings.TrimSpace(string(out))
	}

	return ctx
}

// GetGitContextAt captures git state at a specific path.
func GetGitContextAt(path string) GitContext {
	ctx := GitContext{}

	// Get commit hash
	if out, err := exec.Command("git", "-C", path, "rev-parse", "HEAD").Output(); err == nil {
		ctx.CommitHash = strings.TrimSpace(string(out))
		if len(ctx.CommitHash) >= 7 {
			ctx.CommitShort = ctx.CommitHash[:7]
		}
	}

	// Get branch name
	if out, err := exec.Command("git", "-C", path, "rev-parse", "--abbrev-ref", "HEAD").Output(); err == nil {
		ctx.Branch = strings.TrimSpace(string(out))
	}

	// Check if working tree is dirty
	if out, err := exec.Command("git", "-C", path, "status", "--porcelain").Output(); err == nil {
		ctx.IsDirty = len(strings.TrimSpace(string(out))) > 0
	}

	// Get author of HEAD commit
	if out, err := exec.Command("git", "-C", path, "log", "-1", "--format=%an").Output(); err == nil {
		ctx.Author = strings.TrimSpace(string(out))
	}

	// Get repo root
	if out, err := exec.Command("git", "-C", path, "rev-parse", "--show-toplevel").Output(); err == nil {
		ctx.RepoRoot = strings.TrimSpace(string(out))
	}

	return ctx
}

// Complete finalizes the event with timing and status.
func (e *AuditEvent) Complete(status Status, err error) {
	e.CompletedAt = time.Now()
	e.Duration = e.CompletedAt.Sub(e.StartedAt)
	e.DurationMs = e.Duration.Milliseconds()
	e.Status = status

	if err != nil {
		e.ErrorMessage = err.Error()
		if status == "" {
			e.Status = StatusError
		}
	}
}

// SetError sets error details on the event.
func (e *AuditEvent) SetError(err error, exitCode int) {
	if err != nil {
		e.ErrorMessage = err.Error()
		e.ExitCode = exitCode
		if e.Status == "" {
			e.Status = StatusError
		}
	}
}
