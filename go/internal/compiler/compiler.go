package compiler

import (
	"context"
	"fmt"
	"strings"

	"github.com/joss/urp/internal/gate"
)

// ContextCompiler is responsible for transforming the Graph State into a linear Prompt.
type ContextCompiler struct {
	store             *Store
	gate              gate.GateClient
	strategyRetriever StrategyRetriever
}

// StrategyRetriever defines the contract for fetching learned strategies.
type StrategyRetriever interface {
	GetStrategyHint(ctx context.Context, goal string) (string, error)
}

func NewContextCompiler(store *Store, gate gate.GateClient) *ContextCompiler {
	return &ContextCompiler{
		store: store,
		gate:  gate,
	}
}

// SetStrategyRetriever injects the strategy retrieval mechanism.
func (c *ContextCompiler) SetStrategyRetriever(sr StrategyRetriever) {
	c.strategyRetriever = sr
}

// Compile builds the "Compiled View" for the current execution step.
// It implements the "Context as Computed View" pattern.
func (c *ContextCompiler) Compile(ctx context.Context, sessionID string, goal string, rawLogs string, workDir string) (string, error) {
	// 1. Stable Prefix (System Prompt + Immutable Context)
	prefix := c.getStablePrefix(goal, workDir)

	// 2. Computed State (Query Memgraph for CURRENT_STATE)
	stateStr, err := c.getComputedState(ctx, sessionID)
	if err != nil {
		return "", fmt.Errorf("failed to compute state: %w", err)
	}

	// 3. Gated Logs (The "Noise Filter" Pass)
	// We pass the raw logs through the Gate. If they are noise, they disappear.
	// If rawLogs is empty, skip calling the gate
	logSection := ""
	if rawLogs != "" {
		filteredLogs, err := c.gate.FilterNoise(ctx, goal, rawLogs)
		if err != nil {
			// On error, maybe fallback or log warning. For now return error.
			return "", fmt.Errorf("gate failure: %w", err)
		}
		
		if filteredLogs != "" {
			logSection = fmt.Sprintf("\n<RECENT_LOGS>\n%s\n</RECENT_LOGS>", filteredLogs)
		}
	}

	// 4. Relevant Knowledge (Retrieval)
	rules, err := c.store.GetRelevantRules(ctx, goal)
	if err != nil {
		return "", fmt.Errorf("retrieval failure: %w", err)
	}
	
	knowledgeSection := ""
	if len(rules) > 0 {
		knowledgeSection = fmt.Sprintf("\n<RELEVANT_KNOWLEDGE>\n%s\n</RELEVANT_KNOWLEDGE>", strings.Join(rules, "\n"))
	}

	// 5. Learned Strategy (End of Cycle Retrieval)
	strategySection := ""
	if c.strategyRetriever != nil {
		hint, err := c.strategyRetriever.GetStrategyHint(ctx, goal)
		if err == nil && hint != "" {
			strategySection = fmt.Sprintf("\n<LEARNED_STRATEGY>\n%s\n</LEARNED_STRATEGY>", hint)
		} else if err != nil {
			// Log but don't fail?
			// fmt.Printf("Strategy retrieval failed: %v\n", err)
		}
	}

	// 6. Assemble
	return fmt.Sprintf("%s\n%s%s%s%s", prefix, stateStr, knowledgeSection, logSection, strategySection), nil
}

func (c *ContextCompiler) getStablePrefix(goal, workDir string) string {
	wdInfo := ""
	if workDir != "" {
		wdInfo = fmt.Sprintf("\nWorking Directory: %s\nIMPORTANT: When using graph tools (graph_search, code_deps, graph_stats), ALWAYS use the 'root_path' parameter set to '%s' to scope your search.", workDir, workDir)
	}

	return fmt.Sprintf(`<SYSTEM>
You are URP (Universal Robotic Programmer).
Paradigm: State-Based Engineering.%s
</SYSTEM>
<GOAL>
%s
</GOAL>`, wdInfo, goal)
}

func (c *ContextCompiler) getComputedState(ctx context.Context, sessionID string) (string, error) {
	state, err := c.store.GetComputedState(ctx, sessionID)
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
