package tui

import (
	"fmt"

	"github.com/joss/urp/internal/opencode/domain"
)

// handleStreamEvent processes streaming events from the agent
func (m *AgentModel) handleStreamEvent(event domain.StreamEvent) {
	switch event.Type {
	case domain.StreamEventThinking:
		m.shared.output.WriteString(thinkingStyle.Render(event.Content))
		if event.Usage != nil {
			m.thinkTokens += event.Usage.OutputTokens
			// Debug: Log thinking tokens
			if m.debug != nil && m.debug.IsEnabled() {
				preview := event.Content
				if len(preview) > 100 {
					preview = preview[:97] + "..."
				}
				m.debug.AddThinking(preview, event.Usage.OutputTokens)
			}
		}
		// Brain: Focus state when thinking
		m.brain, _ = m.brain.Update(BrainFocusMsg{Task: "Thinking..."})

	case domain.StreamEventText:
		m.shared.output.WriteString(textStyle.Render(event.Content))
		if event.Usage != nil {
			m.outputTokens += event.Usage.OutputTokens
		}

	case domain.StreamEventToolCall:
		m.handleToolCall(event)

	case domain.StreamEventToolDone:
		m.handleToolDone(event)

	case domain.StreamEventError:
		m.shared.output.WriteString(agentErrorStyle.Render(fmt.Sprintf("\nError: %v\n", event.Error)))
		// Debug: Log error
		if m.debug != nil && m.debug.IsEnabled() {
			m.debug.AddError("Stream", event.Error.Error())
		}
		// Brain: Trauma on error
		m.brain, _ = m.brain.Update(BrainTraumaMsg{Err: event.Error})

	case domain.StreamEventUsage:
		m.handleUsageEvent(event)

	case domain.StreamEventPermissionAsk:
		// Debug: Log permission request
		if m.debug != nil && m.debug.IsEnabled() && event.PermissionReq != nil {
			m.debug.AddPermission(
				event.PermissionReq.Tool,
				event.PermissionReq.Command,
				event.PermissionReq.Path,
				"asking...",
			)
		}
	}
}

func (m *AgentModel) handleToolCall(event domain.StreamEvent) {
	if tc, ok := event.Part.(domain.ToolCallPart); ok {
		info := toolCallInfo{
			name:      tc.Name,
			args:      truncateArgsMap(tc.Args),
			collapsed: true,
		}
		*m.shared.toolCalls = append(*m.shared.toolCalls, info)
		m.currentTool = &(*m.shared.toolCalls)[len(*m.shared.toolCalls)-1]
		m.shared.output.WriteString("\n" + toolStyle.Render(fmt.Sprintf("▶ %s", tc.Name)) + "\n")

		// Debug: Log tool call start
		if m.debug != nil && m.debug.IsEnabled() {
			m.debug.AddEvent(DebugEvent{
				Type:    DebugEventTool,
				Title:   fmt.Sprintf("Tool Start: %s", tc.Name),
				Content: truncateArgsMap(tc.Args),
			})
		}

		// Brain: Show tool activity
		switch tc.Name {
		case "write", "edit", "multi_edit", "patch":
			path := getToolPath(tc.Args)
			m.brain, _ = m.brain.Update(BrainWriteMsg{Path: path})
		case "grep", "glob", "read":
			m.brain, _ = m.brain.Update(BrainRecallMsg{Context: tc.Name})
		default:
			m.brain, _ = m.brain.Update(BrainFocusMsg{Task: tc.Name})
		}
	}
}

func (m *AgentModel) handleToolDone(event domain.StreamEvent) {
	if tc, ok := event.Part.(domain.ToolCallPart); ok {
		if m.currentTool != nil {
			m.currentTool.output = truncateOutput(tc.Result)
			m.currentTool.err = tc.Error
			m.currentTool.done = true

			// Debug: Log tool completion
			if m.debug != nil && m.debug.IsEnabled() {
				m.debug.AddTool(tc.Name, tc.Args, tc.Result, tc.Error, tc.Duration)
			}

			if tc.Error != "" {
				m.shared.output.WriteString(agentErrorStyle.Render(fmt.Sprintf("  ✗ %s\n", tc.Error)))
				// Brain: Trauma on tool error
				m.brain, _ = m.brain.Update(BrainTraumaMsg{Err: fmt.Errorf("%s", tc.Error)})
			} else {
				m.shared.output.WriteString(successStyle.Render("  ✓\n"))
			}
		}
	}
}

func (m *AgentModel) handleUsageEvent(event domain.StreamEvent) {
	if event.Usage != nil {
		m.inputTokens = event.Usage.InputTokens
		m.outputTokens = event.Usage.OutputTokens

		// Debug: Log LLM usage (this is critical!)
		if m.debug != nil && m.debug.IsEnabled() {
			model := "unknown"
			if m.ag != nil {
				model = m.ag.Model()
			}
			m.debug.AddEvent(DebugEvent{
				Type:  DebugEventAPI,
				Title: fmt.Sprintf("LLM Call: %s", model),
				Content: fmt.Sprintf("Input: %d tokens\nOutput: %d tokens\nCache Read: %d\nCache Write: %d\nCost: $%.4f",
					event.Usage.InputTokens,
					event.Usage.OutputTokens,
					event.Usage.CacheRead,
					event.Usage.CacheWrite,
					event.Usage.TotalCost),
				Metadata: map[string]string{
					"model":         model,
					"input_tokens":  fmt.Sprintf("%d", event.Usage.InputTokens),
					"output_tokens": fmt.Sprintf("%d", event.Usage.OutputTokens),
					"total_cost":    fmt.Sprintf("$%.4f", event.Usage.TotalCost),
				},
			})
		}

		// Brain: Update token usage for progress bar
		totalTokens := m.inputTokens + m.outputTokens + m.thinkTokens
		m.brain, _ = m.brain.Update(TokenUpdateMsg{Current: totalTokens, Max: m.brain.MaxTokens})
	}
}

// getToolPath extracts path from tool args
func getToolPath(args map[string]any) string {
	if p, ok := args["path"].(string); ok {
		return p
	}
	if p, ok := args["file_path"].(string); ok {
		return p
	}
	return ""
}
