# Model Selection: Usage Examples

**Quick Reference** for using the new unified model selection system.

---

## Basic Usage

### Simple Model Selection

```go
package main

import (
    "context"
    "github.com/joss/urp/internal/opencode/agent"
)

func main() {
    ctx := context.Background()
    service := agent.DefaultModelService  // Initialized in bootstrap

    // Define task
    input := &agent.DecisionInput{
        SessionID:   "sess-12345",
        TaskType:    agent.TaskTypeFeature,
        Complexity:  0.6,
        EstTokens:   40000,
        BudgetLimit: 0.50,
        Strategy:    "balanced",  // Default
    }

    // Get model + decision log
    provider, selection, decision, err := service.SelectAndProvision(ctx, input)
    if err != nil {
        panic(err)
    }

    fmt.Printf("Selected: %s (confidence: %.2f)\n", selection.ModelID, selection.Confidence)
    fmt.Printf("Why: %s\n", decision.Reason)
    fmt.Printf("Reasoning chain: %v\n", decision.Reasoning)
}
```

### Decision-Only (No Provider)

Use when you want to decide without provisioning:

```go
// Plan a model selection without creating provider yet
selection, decision, err := service.Select(ctx, input)

if selection.Confidence > 0.8 {
    // High confidence - provision now
    provider, err := service.Provision(ctx, selection.ModelID)
} else {
    // Low confidence - maybe ask user?
    fmt.Printf("Low confidence: %s\n", decision.Reason)
}
```

---

## Strategy Examples

### Cost-Optimized (Minimize Spending)

```go
input := &agent.DecisionInput{
    SessionID:    "sess-explore",
    TaskType:     agent.TaskTypeExplore,
    Complexity:   0.2,      // Simple task
    EstTokens:    5000,
    BudgetLimit:  0.01,     // Only 1 cent!
    Strategy:     "cost",   // ← COST STRATEGY
    RequiredCaps: []string{"code"},
}

provider, selection, _, _ := service.SelectAndProvision(ctx, input)
// Likely: deepseek-chat or gemini-1.5-flash
```

### Quality-Focused (Best Possible)

```go
input := &agent.DecisionInput{
    SessionID:    "sess-code-review",
    TaskType:     agent.TaskTypeReview,  // Code review
    Complexity:   0.9,                   // Complex
    EstTokens:    80000,
    HasImages:    true,                  // Includes diagrams
    BudgetLimit:  2.0,                   // Budget is available
    Strategy:     "quality",             // ← QUALITY STRATEGY
    RequiredCaps: []string{"code", "vision", "reasoning"},
}

provider, selection, _, _ := service.SelectAndProvision(ctx, input)
// Likely: claude-opus-4-5-20251101
```

### Speed-Optimized (Fast Response)

```go
input := &agent.DecisionInput{
    SessionID:    "sess-quick-exploration",
    TaskType:     agent.TaskTypeExplore,
    Complexity:   0.3,
    EstTokens:    2000,
    BudgetLimit:  0.05,
    Strategy:     "speed",       // ← SPEED STRATEGY
}

provider, selection, _, _ := service.SelectAndProvision(ctx, input)
// Likely: claude-haiku-4-5 or gemini-1.5-flash
```

### Balanced (Smart Default)

```go
// Most common - let the engine decide based on task complexity
input := &agent.DecisionInput{
    SessionID:    "sess-typical",
    TaskType:     agent.TaskTypeBugfix,
    Complexity:   0.55,          // Medium complexity
    EstTokens:    30000,
    BudgetLimit:  0.30,
    Strategy:     "balanced",    // ← BALANCED (default)
}

provider, selection, decision, _ := service.SelectAndProvision(ctx, input)

// For this complexity (0.55), balanced strategy picks:
// - Complexity > 0.75 → opus (not matched)
// - Complexity > 0.5 → sonnet (✓ MATCHED)
// Selected: claude-sonnet-4-5-20250929
```

---

## Testing

All tests passing:

```bash
$ go test ./internal/opencode/agent/ -v
✅ TestDecisionEngineBalancedStrategy
✅ TestDecisionEngineAuditTrail
✅ TestDecisionEngineStrategies
✅ TestModelServiceIntegration
✅ TestModelServiceAudit

PASS: all tests (0.006s)
```

---

**Ready to integrate!** See `MODEL_SELECTION_MIGRATION.md` for implementation details.
