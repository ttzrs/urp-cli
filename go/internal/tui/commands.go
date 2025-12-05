// Package tui provides the Bubble Tea interactive agent interface.
package tui

import (
	"context"
	"fmt"
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

	cmds := builtinCommands()
	if cmd, ok := cmds[cmdName]; ok {
		return cmd.Handler(m, args)
	}

	return fmt.Sprintf("Unknown command: /%s. Type /help for available commands.", cmdName)
}

// Command handlers

func cmdHelp(m *AgentModel, args string) string {
	var sb strings.Builder
	sb.WriteString("Available commands:\n")
	for name, cmd := range builtinCommands() {
		sb.WriteString(fmt.Sprintf("  /%s - %s\n", name, cmd.Description))
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
