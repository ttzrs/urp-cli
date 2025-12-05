package skills

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/oklog/ulid/v2"
)

// NOTE: expandPath is used by runner.go, so strings import stays

// Executor runs skills.
type Executor struct {
	store     *Store
	sessionID string
}

// NewExecutor creates a skill executor.
func NewExecutor(store *Store, sessionID string) *Executor {
	return &Executor{
		store:     store,
		sessionID: sessionID,
	}
}

// ExecuteResult contains skill execution output.
type ExecuteResult struct {
	SkillID      string        `json:"skill_id"`
	SkillName    string        `json:"skill_name"`
	Success      bool          `json:"success"`
	Output       string        `json:"output"`
	Error        string        `json:"error,omitempty"`
	Duration     time.Duration `json:"duration"`
	ContextFiles []string      `json:"context_files,omitempty"`
	Agent        string        `json:"agent,omitempty"`
}

// Execute runs a skill by name.
func (e *Executor) Execute(ctx context.Context, name string, input string) (*ExecuteResult, error) {
	start := time.Now()

	// Find skill
	skill, err := e.store.GetByName(ctx, name)
	if err != nil {
		return nil, fmt.Errorf("skill not found: %s", name)
	}

	result := &ExecuteResult{
		SkillID:      skill.ID,
		SkillName:    skill.Name,
		ContextFiles: skill.ContextFiles,
		Agent:        skill.Agent,
	}

	// Execute using registry (OCP - extensible without modification)
	var output string
	var execErr error

	runner, ok := GetRunner(skill.SourceType)
	if !ok {
		runner = &DefaultRunner{}
	}
	output, execErr = runner.Run(ctx, skill, input)

	result.Duration = time.Since(start)
	result.Output = output

	if execErr != nil {
		result.Success = false
		result.Error = execErr.Error()
	} else {
		result.Success = true
	}

	// Log execution
	e.store.IncrementUsage(ctx, skill.ID)
	e.store.LogExecution(ctx, &SkillExecution{
		ID:        ulid.Make().String(),
		SkillID:   skill.ID,
		SessionID: e.sessionID,
		Input:     input,
		Output:    output,
		Duration:  result.Duration.Milliseconds(),
		Success:   result.Success,
		Error:     result.Error,
		Timestamp: time.Now(),
	})

	return result, nil
}

// ExecuteScript runs a skill as a shell script.
func (e *Executor) ExecuteScript(ctx context.Context, name string, args []string) (string, error) {
	skill, err := e.store.GetByName(ctx, name)
	if err != nil {
		return "", err
	}

	if skill.SourceType != "file" {
		return "", fmt.Errorf("skill %s is not a script", name)
	}

	// Check if executable
	ext := filepath.Ext(skill.Source)
	var cmd *exec.Cmd

	switch ext {
	case ".sh":
		cmd = exec.CommandContext(ctx, "bash", append([]string{skill.Source}, args...)...)
	case ".py":
		cmd = exec.CommandContext(ctx, "python3", append([]string{skill.Source}, args...)...)
	case ".ts", ".js":
		cmd = exec.CommandContext(ctx, "bun", append([]string{skill.Source}, args...)...)
	default:
		return "", fmt.Errorf("unsupported script type: %s", ext)
	}

	output, err := cmd.CombinedOutput()
	return string(output), err
}

func expandPath(path string) string {
	if strings.HasPrefix(path, "~/") {
		home, _ := os.UserHomeDir()
		return filepath.Join(home, path[2:])
	}
	return path
}
