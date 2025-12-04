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

	// Execute based on source type
	var output string
	var execErr error

	switch skill.SourceType {
	case "file":
		output, execErr = e.executeFile(ctx, skill, input)
	case "builtin":
		output, execErr = e.executeBuiltin(ctx, skill, input)
	case "mcp":
		output, execErr = e.executeMCP(ctx, skill, input)
	default:
		output = fmt.Sprintf("Skill loaded: %s\nCategory: %s\nAgent: %s\nContext: %v",
			skill.Name, skill.Category, skill.Agent, skill.ContextFiles)
	}

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

func (e *Executor) executeFile(ctx context.Context, skill *Skill, input string) (string, error) {
	// Read file content
	content, err := os.ReadFile(skill.Source)
	if err != nil {
		return "", fmt.Errorf("reading skill file: %w", err)
	}

	// Load context files
	var contextContent strings.Builder
	for _, cf := range skill.ContextFiles {
		expanded := expandPath(cf)
		data, err := os.ReadFile(expanded)
		if err == nil {
			contextContent.WriteString(fmt.Sprintf("\n--- %s ---\n%s\n", cf, string(data)))
		}
	}

	return fmt.Sprintf("=== SKILL: %s ===\n\n%s\n\n=== CONTEXT ===\n%s\n\n=== INPUT ===\n%s",
		skill.Name, string(content), contextContent.String(), input), nil
}

func (e *Executor) executeBuiltin(ctx context.Context, skill *Skill, input string) (string, error) {
	switch skill.Name {
	case "researcher":
		return fmt.Sprintf("RESEARCH MODE ACTIVATED\nQuery: %s\nAgent: researcher\nUse web search and document analysis.", input), nil
	case "pentester":
		return fmt.Sprintf("SECURITY TESTING MODE\nTarget: %s\nAgent: pentester\nAuthorized testing only.", input), nil
	case "designer":
		return fmt.Sprintf("VISUAL DESIGN MODE\nTask: %s\nAgent: designer\nBrowser automation available.", input), nil
	case "upgrade":
		return fmt.Sprintf("UPGRADE PLANNING MODE\nScope: %s\nIterating versions to achieve objective.", input), nil
	default:
		return fmt.Sprintf("Builtin skill: %s\nInput: %s", skill.Name, input), nil
	}
}

func (e *Executor) executeMCP(ctx context.Context, skill *Skill, input string) (string, error) {
	// MCP skills call external MCP servers
	// For now, just document what would happen
	return fmt.Sprintf("MCP SKILL: %s\nSource: %s\nInput: %s\n\nThis would invoke the MCP protocol.",
		skill.Name, skill.Source, input), nil
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
