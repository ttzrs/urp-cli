package compiler

import (
	"cere/internal/graph"
	"cere/internal/llm"
	"context"
	"fmt"
	"strings"
)

// ContextCompiler is responsible for transforming the Graph State into a linear Prompt.
type ContextCompiler struct {
	graphClient *graph.Client
	gate        llm.GateClient
}

func NewContextCompiler(g *graph.Client, gate llm.GateClient) *ContextCompiler {
	return &ContextCompiler{
		graphClient: g,
		gate:        gate,
	}
}

// Compile builds the "Compiled View" for the current execution step.
// It implements the "Context as Computed View" pattern.
func (c *ContextCompiler) Compile(ctx context.Context, sessionID string, goal string, rawLogs string) (string, error) {
	// 1. Stable Prefix (System Prompt + Immutable Context)
	prefix := c.getStablePrefix(goal)

	// 2. Computed State (Query Memgraph for CURRENT_STATE)
	stateStr, err := c.getComputedState(ctx, sessionID)
	if err != nil {
		return "", fmt.Errorf("failed to compute state: %w", err)
	}

	// 3. Gated Logs (The "Noise Filter" Pass)
	// We pass the raw logs through the Gate. If they are noise, they disappear.
	filteredLogs, err := c.gate.FilterNoise(ctx, goal, rawLogs)
	if err != nil {
		// On error, maybe fallback or log warning. For now return error.
		return "", fmt.Errorf("gate failure: %w", err)
	}
	
	logSection := ""
	if filteredLogs != "" {
		logSection = fmt.Sprintf("\n<RECENT_LOGS>\n%s\n</RECENT_LOGS>", filteredLogs)
	}

	// 4. Relevant Knowledge (Retrieval)
	rules, err := c.graphClient.GetRelevantRules(ctx, goal)
	if err != nil {
		return "", fmt.Errorf("retrieval failure: %w", err)
	}
	
	knowledgeSection := ""
	if len(rules) > 0 {
		knowledgeSection = fmt.Sprintf("\n<RELEVANT_KNOWLEDGE>\n%s\n</RELEVANT_KNOWLEDGE>", strings.Join(rules, "\n"))
	}

	// 5. Assemble
	return fmt.Sprintf("%s\n%s%s%s", prefix, stateStr, knowledgeSection, logSection), nil
}

func (c *ContextCompiler) getStablePrefix(goal string) string {
	return fmt.Sprintf(`<SYSTEM>
You are URP (Universal Robotic Programmer).
Paradigm: State-Based Engineering.
</SYSTEM>
<GOAL>
%s
</GOAL>`, goal)
}

func (c *ContextCompiler) getComputedState(ctx context.Context, sessionID string) (string, error) {
	state, err := c.graphClient.GetComputedState(ctx, sessionID)
	if err != nil {
		return "", err
	}

	var errorsBuilder strings.Builder
	if len(state.ActiveErrors) > 0 {
		for _, e := range state.ActiveErrors {
			errorsBuilder.WriteString(fmt.Sprintf("- %s\n", e))
		}
	} else {
		errorsBuilder.WriteString("(None)\n")
	}

	var filesBuilder strings.Builder
	if len(state.ModifiedFiles) > 0 {
		for _, f := range state.ModifiedFiles {
			filesBuilder.WriteString(fmt.Sprintf("- %s\n", f))
		}
	} else {
		filesBuilder.WriteString("(None)\n")
	}

	return fmt.Sprintf(`<CURRENT_STATE>
[ACTIVE_ERRORS]
%s
[MODIFIED_FILES]
%s
</CURRENT_STATE>`, errorsBuilder.String(), filesBuilder.String()), nil
}
