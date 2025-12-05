package skills

import (
	"context"
	"fmt"
	"os"
	"strings"
)

// SkillRunner executes skills of a specific type.
type SkillRunner interface {
	Run(ctx context.Context, skill *Skill, input string) (string, error)
}

// skillRunners maps source types to their runners.
var skillRunners = map[string]SkillRunner{
	"file":    &FileRunner{},
	"builtin": &BuiltinRunner{},
	"mcp":     &MCPRunner{},
}

// RegisterRunner adds a new skill runner type.
func RegisterRunner(sourceType string, runner SkillRunner) {
	skillRunners[sourceType] = runner
}

// GetRunner returns the runner for a source type.
func GetRunner(sourceType string) (SkillRunner, bool) {
	r, ok := skillRunners[sourceType]
	return r, ok
}

// FileRunner executes file-based skills.
type FileRunner struct{}

func (r *FileRunner) Run(ctx context.Context, skill *Skill, input string) (string, error) {
	content, err := os.ReadFile(skill.Source)
	if err != nil {
		return "", fmt.Errorf("reading skill file: %w", err)
	}

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

// BuiltinRunner executes builtin skills.
type BuiltinRunner struct{}

// builtinHandlers maps builtin skill names to their output generators.
var builtinHandlers = map[string]func(string) string{
	"researcher": func(input string) string {
		return fmt.Sprintf("RESEARCH MODE ACTIVATED\nQuery: %s\nAgent: researcher\nUse web search and document analysis.", input)
	},
	"pentester": func(input string) string {
		return fmt.Sprintf("SECURITY TESTING MODE\nTarget: %s\nAgent: pentester\nAuthorized testing only.", input)
	},
	"designer": func(input string) string {
		return fmt.Sprintf("VISUAL DESIGN MODE\nTask: %s\nAgent: designer\nBrowser automation available.", input)
	},
	"upgrade": func(input string) string {
		return fmt.Sprintf("UPGRADE PLANNING MODE\nScope: %s\nIterating versions to achieve objective.", input)
	},
}

// RegisterBuiltin adds a new builtin skill handler.
func RegisterBuiltin(name string, handler func(string) string) {
	builtinHandlers[name] = handler
}

func (r *BuiltinRunner) Run(ctx context.Context, skill *Skill, input string) (string, error) {
	if handler, ok := builtinHandlers[skill.Name]; ok {
		return handler(input), nil
	}
	return fmt.Sprintf("Builtin skill: %s\nInput: %s", skill.Name, input), nil
}

// MCPRunner executes MCP protocol skills.
type MCPRunner struct{}

func (r *MCPRunner) Run(ctx context.Context, skill *Skill, input string) (string, error) {
	return fmt.Sprintf("MCP SKILL: %s\nSource: %s\nInput: %s\n\nThis would invoke the MCP protocol.",
		skill.Name, skill.Source, input), nil
}

// DefaultRunner provides fallback output for unknown types.
type DefaultRunner struct{}

func (r *DefaultRunner) Run(ctx context.Context, skill *Skill, input string) (string, error) {
	return fmt.Sprintf("Skill loaded: %s\nCategory: %s\nAgent: %s\nContext: %v",
		skill.Name, skill.Category, skill.Agent, skill.ContextFiles), nil
}
