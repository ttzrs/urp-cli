// Package command implements slash commands for the agent TUI
package command

import (
	"context"
	"fmt"
	"strings"
)

// Command is the interface for slash commands
type Command interface {
	Name() string
	Description() string
	Execute(ctx context.Context, args string, sess *Session) error
}

// Session provides context for command execution
type Session struct {
	ID        string
	Directory string
	Agent     AgentInterface
}

// AgentInterface is what commands need from the agent
type AgentInterface interface {
	Run(ctx context.Context, prompt string) error
	Model() string
	SetModel(modelID string)
}

// Registry holds all available commands
type Registry struct {
	commands map[string]Command
}

// NewRegistry creates a new command registry
func NewRegistry() *Registry {
	return &Registry{
		commands: make(map[string]Command),
	}
}

// Register adds a command to the registry
func (r *Registry) Register(cmd Command) {
	r.commands[cmd.Name()] = cmd
}

// Get retrieves a command by name
func (r *Registry) Get(name string) (Command, bool) {
	cmd, ok := r.commands[name]
	return cmd, ok
}

// List returns all registered commands
func (r *Registry) List() []Command {
	result := make([]Command, 0, len(r.commands))
	for _, cmd := range r.commands {
		result = append(result, cmd)
	}
	return result
}

// Parse parses a slash command input
func Parse(input string) (name string, args string, ok bool) {
	if !strings.HasPrefix(input, "/") {
		return "", "", false
	}

	input = strings.TrimPrefix(input, "/")
	parts := strings.SplitN(input, " ", 2)

	name = strings.ToLower(parts[0])
	if len(parts) > 1 {
		args = parts[1]
	}

	return name, args, true
}

// DefaultRegistry returns a registry with built-in commands
func DefaultRegistry() *Registry {
	r := NewRegistry()
	r.Register(&HelpCommand{registry: r})
	r.Register(&InitCommand{})
	r.Register(&ReviewCommand{})
	r.Register(&CompactCommand{})
	r.Register(&ModelCommand{})
	r.Register(&ClearCommand{})
	return r
}

// HelpCommand shows available commands
type HelpCommand struct {
	registry *Registry
}

func (c *HelpCommand) Name() string        { return "help" }
func (c *HelpCommand) Description() string { return "Show available commands" }

func (c *HelpCommand) Execute(ctx context.Context, args string, sess *Session) error {
	var sb strings.Builder
	sb.WriteString("Available commands:\n\n")

	for _, cmd := range c.registry.List() {
		sb.WriteString(fmt.Sprintf("  /%s - %s\n", cmd.Name(), cmd.Description()))
	}

	fmt.Println(sb.String())
	return nil
}

// InitCommand creates CLAUDE.md for the project
type InitCommand struct{}

func (c *InitCommand) Name() string        { return "init" }
func (c *InitCommand) Description() string { return "Create CLAUDE.md with project context" }

func (c *InitCommand) Execute(ctx context.Context, args string, sess *Session) error {
	if sess.Agent == nil {
		return fmt.Errorf("agent not available")
	}

	prompt := `Analyze this project and create a CLAUDE.md file with:

1. **Build Commands**: How to build, test, lint the project
2. **Code Style**: Language-specific conventions used
3. **Project Structure**: Key directories and their purpose
4. **Key Patterns**: Important design patterns or idioms

Keep it concise (under 100 lines). Focus on what an AI agent needs to know to work effectively in this codebase.

Write the file to CLAUDE.md in the project root.`

	return sess.Agent.Run(ctx, prompt)
}

// ReviewCommand reviews code changes
type ReviewCommand struct{}

func (c *ReviewCommand) Name() string        { return "review" }
func (c *ReviewCommand) Description() string { return "Review uncommitted changes" }

func (c *ReviewCommand) Execute(ctx context.Context, args string, sess *Session) error {
	if sess.Agent == nil {
		return fmt.Errorf("agent not available")
	}

	target := "uncommitted changes"
	if args != "" {
		target = args
	}

	prompt := fmt.Sprintf(`Review the following %s:

1. Run 'git diff' to see the changes
2. Analyze for:
   - Bugs or logic errors
   - Security issues
   - Performance concerns
   - Code style violations
   - Missing error handling
3. Provide specific, actionable feedback

Be concise. Focus on important issues.`, target)

	return sess.Agent.Run(ctx, prompt)
}

// CompactCommand compacts conversation history
type CompactCommand struct{}

func (c *CompactCommand) Name() string        { return "compact" }
func (c *CompactCommand) Description() string { return "Summarize conversation to reduce tokens" }

func (c *CompactCommand) Execute(ctx context.Context, args string, sess *Session) error {
	if sess.Agent == nil {
		return fmt.Errorf("agent not available")
	}

	prompt := `Summarize our conversation so far:

1. What were the main tasks/requests?
2. What was accomplished?
3. What files were modified?
4. What is the current state?

Keep the summary concise but preserve important context for continuing the work.`

	return sess.Agent.Run(ctx, prompt)
}

// ModelCommand shows or changes the model
type ModelCommand struct{}

func (c *ModelCommand) Name() string        { return "model" }
func (c *ModelCommand) Description() string { return "Show or change the model" }

func (c *ModelCommand) Execute(ctx context.Context, args string, sess *Session) error {
	if sess.Agent == nil {
		return fmt.Errorf("agent not available")
	}

	if args == "" {
		fmt.Printf("Current model: %s\n", sess.Agent.Model())
		fmt.Println("\nAvailable models:")
		fmt.Println("  claude-sonnet-4-20250514")
		fmt.Println("  claude-opus-4-20250514")
		fmt.Println("  claude-3-5-haiku-20241022")
		fmt.Println("\nUsage: /model <model-id>")
		return nil
	}

	sess.Agent.SetModel(args)
	fmt.Printf("Model changed to: %s\n", args)
	return nil
}

// ClearCommand clears the screen
type ClearCommand struct{}

func (c *ClearCommand) Name() string        { return "clear" }
func (c *ClearCommand) Description() string { return "Clear the screen" }

func (c *ClearCommand) Execute(ctx context.Context, args string, sess *Session) error {
	fmt.Print("\033[H\033[2J") // ANSI clear screen
	return nil
}
