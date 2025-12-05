// Package tui provides the Bubble Tea interactive agent interface.
package tui

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/joss/urp/internal/opencode/domain"
	"github.com/joss/urp/internal/opencode/tokens"
)

// SlashCommand represents a slash command handler
type SlashCommand struct {
	Name        string
	Description string
	Handler     func(m *AgentModel, args string) string
}

// builtinCommands returns all available slash commands
func builtinCommands() map[string]SlashCommand {
	return map[string]SlashCommand{
		"help": {
			Name:        "help",
			Description: "Show available commands",
			Handler:     cmdHelp,
		},
		"clear": {
			Name:        "clear",
			Description: "Clear the output",
			Handler:     cmdClear,
		},
		"compact": {
			Name:        "compact",
			Description: "Compact session history (summarize old messages)",
			Handler:     cmdCompact,
		},
		"tokens": {
			Name:        "tokens",
			Description: "Show token count for current session",
			Handler:     cmdTokens,
		},
		"model": {
			Name:        "model",
			Description: "Show or change current model",
			Handler:     cmdModel,
		},
		"init": {
			Name:        "init",
			Description: "Create CLAUDE.md with project context",
			Handler:     cmdInit,
		},
		"review": {
			Name:        "review",
			Description: "Review uncommitted changes (or specify target)",
			Handler:     cmdReview,
		},
	}
}

// isSlashCommand checks if input starts with /
func isSlashCommand(input string) bool {
	return strings.HasPrefix(strings.TrimSpace(input), "/")
}

// executeSlashCommand parses and runs a slash command
func executeSlashCommand(m *AgentModel, input string) string {
	input = strings.TrimSpace(input)
	if !strings.HasPrefix(input, "/") {
		return ""
	}

	parts := strings.SplitN(input[1:], " ", 2)
	cmdName := strings.ToLower(parts[0])
	args := ""
	if len(parts) > 1 {
		args = parts[1]
	}

	// Include custom commands from .urp/commands/
	cmds := allCommands(m.workDir)
	if cmd, ok := cmds[cmdName]; ok {
		return cmd.Handler(m, args)
	}

	return fmt.Sprintf("Unknown command: /%s. Type /help for available commands.", cmdName)
}

// Command handlers

func cmdHelp(m *AgentModel, args string) string {
	var sb strings.Builder
	sb.WriteString("Built-in commands:\n")
	for name, cmd := range builtinCommands() {
		sb.WriteString(fmt.Sprintf("  /%s - %s\n", name, cmd.Description))
	}

	// Show custom commands if any
	custom := loadCustomCommands(m.workDir)
	if len(custom) > 0 {
		sb.WriteString("\nCustom commands (.urp/commands/):\n")
		for name, cmd := range custom {
			sb.WriteString(fmt.Sprintf("  /%s - %s\n", name, cmd.Description))
		}
	}

	sb.WriteString("\nShortcuts:\n")
	sb.WriteString("  @         - Open file picker\n")
	sb.WriteString("  Ctrl+C    - Cancel running operation\n")
	sb.WriteString("  Ctrl+L    - Clear output\n")
	sb.WriteString("  Ctrl+T    - Toggle tool output\n")
	sb.WriteString("  Alt+Enter - Insert newline\n")
	return sb.String()
}

func cmdClear(m *AgentModel, args string) string {
	m.shared.output.Reset()
	*m.shared.toolCalls = []toolCallInfo{}
	m.viewport.SetContent("")
	return ""
}

func cmdCompact(m *AgentModel, args string) string {
	if m.ag == nil {
		return "Error: Agent not initialized"
	}

	// Get current messages from agent
	messages := m.ag.Messages()
	if len(messages) < 4 {
		return "Session too short to compact (need at least 4 messages)"
	}

	currentTokens := tokens.CountMessages(messages)

	// Create summary prompt from older messages
	oldMessages := messages[:len(messages)-2] // Keep last 2 exchanges
	var summaryParts []string
	for _, msg := range oldMessages {
		for _, part := range msg.Parts {
			switch p := part.(type) {
			case domain.TextPart:
				role := "User"
				if msg.Role == domain.RoleAssistant {
					role = "Assistant"
				}
				summaryParts = append(summaryParts, fmt.Sprintf("%s: %s", role, truncateForSummary(p.Text)))
			case domain.ToolCallPart:
				summaryParts = append(summaryParts, fmt.Sprintf("Tool[%s]: %s", p.Name, truncateForSummary(p.Result)))
			}
		}
	}

	if len(summaryParts) == 0 {
		return "No content to compact"
	}

	// Build compacted context
	summary := fmt.Sprintf("[Session Summary - %s]\n%s",
		time.Now().Format("2006-01-02 15:04"),
		strings.Join(summaryParts, "\n"))

	// Replace old messages with summary
	summaryMsg := domain.Message{
		ID:        "compact-summary",
		SessionID: "",
		Role:      domain.RoleSystem,
		Parts:     []domain.Part{domain.TextPart{Text: summary}},
		Timestamp: time.Now(),
	}

	// Keep recent messages
	recentMessages := messages[len(messages)-2:]
	newMessages := append([]domain.Message{summaryMsg}, recentMessages...)

	// Update agent with compacted history
	m.ag.SetMessages(newMessages)

	newTokens := tokens.CountMessages(newMessages)
	reduction := float64(currentTokens-newTokens) / float64(currentTokens) * 100

	return fmt.Sprintf("Compacted: %d -> %d tokens (%.1f%% reduction)\nKept last %d messages",
		currentTokens, newTokens, reduction, len(recentMessages))
}

func cmdTokens(m *AgentModel, args string) string {
	if m.ag == nil {
		return "Error: Agent not initialized"
	}

	messages := m.ag.Messages()
	total := tokens.CountMessages(messages)

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Session tokens: %d\n", total))
	sb.WriteString(fmt.Sprintf("Messages: %d\n", len(messages)))

	// Show breakdown by role
	userTokens, assistantTokens, systemTokens := 0, 0, 0
	for _, msg := range messages {
		msgTokens := tokens.CountMessage(msg)
		switch msg.Role {
		case domain.RoleUser:
			userTokens += msgTokens
		case domain.RoleAssistant:
			assistantTokens += msgTokens
		case domain.RoleSystem:
			systemTokens += msgTokens
		}
	}

	sb.WriteString(fmt.Sprintf("  User: %d\n", userTokens))
	sb.WriteString(fmt.Sprintf("  Assistant: %d\n", assistantTokens))
	if systemTokens > 0 {
		sb.WriteString(fmt.Sprintf("  System: %d\n", systemTokens))
	}

	// Suggest compact if over threshold
	if total > 50000 {
		sb.WriteString("\n⚠ High token usage. Consider /compact")
	}

	return sb.String()
}

func cmdModel(m *AgentModel, args string) string {
	if m.ag == nil {
		return "Error: Agent not initialized"
	}

	args = strings.TrimSpace(args)

	// Show current model if no args
	if args == "" {
		return fmt.Sprintf("Current model: %s\nUsage: /model <model-id>", m.ag.Model())
	}

	// Set new model
	m.ag.SetModel(args)
	return fmt.Sprintf("Model set to: %s", args)
}

// Helper functions

func truncateForSummary(s string) string {
	// Keep first 200 chars for summary
	s = strings.TrimSpace(s)
	if len(s) > 200 {
		return s[:197] + "..."
	}
	return s
}

// CompactWithLLM uses an LLM to generate a better summary (optional enhancement)
func CompactWithLLM(ctx context.Context, m *AgentModel, messages []domain.Message) (string, error) {
	// This would call the LLM with a summarization prompt
	// For now, we use the simple text-based compaction
	// Future: Use a fast/cheap model (Haiku) to summarize
	return "", fmt.Errorf("LLM compaction not implemented yet")
}

func cmdInit(m *AgentModel, args string) string {
	if m.ag == nil {
		return "Error: Agent not initialized"
	}

	prompt := `Analyze this project and create a CLAUDE.md file with:

1. **Build Commands**: How to build, test, lint the project
2. **Code Style**: Language-specific conventions used
3. **Project Structure**: Key directories and their purpose
4. **Key Patterns**: Important design patterns or idioms

Keep it concise (under 100 lines). Focus on what an AI agent needs to know.
Write the file to CLAUDE.md in the project root.`

	// Queue the prompt to be sent
	m.queuePrompt(prompt)
	return "Running /init - analyzing project..."
}

func cmdReview(m *AgentModel, args string) string {
	if m.ag == nil {
		return "Error: Agent not initialized"
	}

	target := "uncommitted changes"
	if args != "" {
		target = args
	}

	prompt := fmt.Sprintf(`Review %s:

1. Run 'git diff' to see the changes
2. Analyze for:
   - Bugs or logic errors
   - Security issues
   - Performance concerns
   - Code style violations
   - Missing error handling
3. Provide specific, actionable feedback

Be concise. Focus on important issues.`, target)

	m.queuePrompt(prompt)
	return fmt.Sprintf("Running /review - analyzing %s...", target)
}

// loadCustomCommands loads markdown files from .urp/commands/ as slash commands
func loadCustomCommands(workDir string) map[string]SlashCommand {
	result := make(map[string]SlashCommand)

	// Look in .urp/commands/ relative to workDir
	cmdDir := filepath.Join(workDir, ".urp", "commands")
	entries, err := os.ReadDir(cmdDir)
	if err != nil {
		return result // No custom commands
	}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		name := entry.Name()
		if !strings.HasSuffix(name, ".md") {
			continue
		}

		// Read file content
		path := filepath.Join(cmdDir, name)
		content, err := os.ReadFile(path)
		if err != nil {
			continue
		}

		// Command name is filename without .md
		cmdName := strings.TrimSuffix(name, ".md")
		cmdName = strings.ToLower(cmdName)

		// Parse description from first line if it starts with #
		prompt := string(content)
		description := "Custom command: " + cmdName
		lines := strings.SplitN(prompt, "\n", 2)
		if len(lines) > 0 && strings.HasPrefix(lines[0], "# ") {
			description = strings.TrimPrefix(lines[0], "# ")
			if len(lines) > 1 {
				prompt = strings.TrimSpace(lines[1])
			}
		}

		// Create custom command handler (closure captures prompt)
		cmdPrompt := prompt
		result[cmdName] = SlashCommand{
			Name:        cmdName,
			Description: description,
			Handler: func(m *AgentModel, args string) string {
				if m.ag == nil {
					return "Error: Agent not initialized"
				}
				// Replace $ARGS in prompt with actual args
				finalPrompt := strings.ReplaceAll(cmdPrompt, "$ARGS", args)
				m.queuePrompt(finalPrompt)
				return fmt.Sprintf("Running /%s...", cmdName)
			},
		}
	}

	return result
}

// allCommands returns builtin + custom commands
func allCommands(workDir string) map[string]SlashCommand {
	cmds := builtinCommands()
	for name, cmd := range loadCustomCommands(workDir) {
		cmds[name] = cmd
	}
	return cmds
}
