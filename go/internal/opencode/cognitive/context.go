// Package cognitive implements dynamic context optimization
// The context sent to LLM adapts based on operation type
package cognitive

import (
	"fmt"
	"path/filepath"
	"strings"
)

// ContextMode determines how much context to include
type ContextMode int

const (
	// ModeFull - Send everything (initial exploration, complex debugging)
	// Use: First read of codebase, major refactoring, unknown territory
	// Budget: 80-100% of available context
	ModeFull ContextMode = iota

	// ModeFocused - Current file + immediate dependencies
	// Use: Editing a specific function, fixing a known bug
	// Budget: 30-50% of available context
	ModeFocused

	// ModeMinimal - Only the function being edited + types
	// Use: Simple changes, known patterns, repetitive edits
	// Budget: 10-20% of available context
	ModeMinimal

	// ModeDelta - Only changed lines + surrounding context
	// Use: Iterating on a fix, refining code
	// Budget: 5-15% of available context
	ModeDelta

	// ModeMemory - State + minimal code (rely on memory)
	// Use: Continuing a task, agent "remembers" the code
	// Budget: 5-10% of available context
	ModeMemory
)

// ContextPriority determines which elements get included first
type ContextPriority int

const (
	PriorityEssential ContextPriority = iota // Must include (current function, error)
	PriorityHigh                             // Important (direct imports, types)
	PriorityMedium                           // Useful (related files, tests)
	PriorityLow                              // Nice to have (docs, examples)
	PriorityNoise                            // Skip (unrelated, verbose output)
)

// ContextItem represents a piece of context to potentially include
type ContextItem struct {
	Type     string          // "code", "file", "function", "error", "memory", "state"
	Path     string          // File path if applicable
	Content  string          // The actual content
	Priority ContextPriority // How important is this
	Tokens   int             // Estimated token count
	Stale    bool            // True if content may be outdated
	LastEdit int             // Turn number when last edited (0 = never)
}

// ContextOptimizer manages dynamic context allocation
type ContextOptimizer struct {
	mode          ContextMode
	budget        int           // Total token budget
	used          int           // Tokens used so far
	items         []ContextItem // Potential items to include
	currentFile   string        // File being edited
	currentFunc   string        // Function being edited
	editHistory   []string      // Recently edited files
	turnNumber    int           // Current conversation turn
	memoryState   string        // Compressed state from previous turns
	errorContext  string        // Current error being fixed (if any)
}

// NewContextOptimizer creates a new optimizer
func NewContextOptimizer(budget int) *ContextOptimizer {
	return &ContextOptimizer{
		mode:        ModeFocused, // Default to focused mode
		budget:      budget,
		items:       make([]ContextItem, 0),
		editHistory: make([]string, 0, 10),
	}
}

// SetMode changes the context mode
func (c *ContextOptimizer) SetMode(mode ContextMode) {
	c.mode = mode
}

// SetCurrentEdit sets what's currently being edited
func (c *ContextOptimizer) SetCurrentEdit(file, function string) {
	c.currentFile = file
	c.currentFunc = function

	// Track edit history (max 10)
	if file != "" {
		c.editHistory = append(c.editHistory, file)
		if len(c.editHistory) > 10 {
			c.editHistory = c.editHistory[1:]
		}
	}
}

// SetError sets error context (triggers priority boost)
func (c *ContextOptimizer) SetError(err string) {
	c.errorContext = err
}

// ClearError clears error context
func (c *ContextOptimizer) ClearError() {
	c.errorContext = ""
}

// SetMemoryState sets compressed state from summarization
func (c *ContextOptimizer) SetMemoryState(state string) {
	c.memoryState = state
}

// NextTurn increments the turn counter
func (c *ContextOptimizer) NextTurn() {
	c.turnNumber++
}

// AddItem adds a potential context item
func (c *ContextOptimizer) AddItem(item ContextItem) {
	// Estimate tokens if not set
	if item.Tokens == 0 {
		item.Tokens = len(item.Content) / 4
	}

	// Auto-determine priority based on current state
	if item.Priority == 0 {
		item.Priority = c.inferPriority(item)
	}

	c.items = append(c.items, item)
}

// inferPriority determines priority based on context
func (c *ContextOptimizer) inferPriority(item ContextItem) ContextPriority {
	// Current file is essential
	if item.Path == c.currentFile {
		return PriorityEssential
	}

	// Recently edited files are high priority
	for _, edited := range c.editHistory {
		if item.Path == edited {
			return PriorityHigh
		}
	}

	// Same directory = medium
	if item.Path != "" && c.currentFile != "" {
		if filepath.Dir(item.Path) == filepath.Dir(c.currentFile) {
			return PriorityMedium
		}
	}

	// Tests for current file = medium
	if item.Type == "file" && strings.Contains(item.Path, "_test") {
		base := strings.TrimSuffix(c.currentFile, ".go")
		if strings.Contains(item.Path, base) {
			return PriorityMedium
		}
	}

	// Error output = essential if we have an error
	if item.Type == "error" && c.errorContext != "" {
		return PriorityEssential
	}

	return PriorityLow
}

// Build constructs the optimized context string
func (c *ContextOptimizer) Build() string {
	c.used = 0

	// Determine budget allocation based on mode
	budgetPct := c.getBudgetPercentage()
	availableBudget := int(float64(c.budget) * budgetPct)

	// Sort items by priority
	sorted := c.sortByPriority()

	var result strings.Builder

	// 1. Always include memory state if in ModeMemory
	if c.mode == ModeMemory && c.memoryState != "" {
		result.WriteString("<memory-state>\n")
		result.WriteString(c.memoryState)
		result.WriteString("\n</memory-state>\n\n")
		c.used += len(c.memoryState) / 4
	}

	// 2. Always include error context if present
	if c.errorContext != "" {
		result.WriteString("<current-error>\n")
		result.WriteString(c.errorContext)
		result.WriteString("\n</current-error>\n\n")
		c.used += len(c.errorContext) / 4
	}

	// 3. Add items by priority until budget exhausted
	for _, item := range sorted {
		if c.used+item.Tokens > availableBudget {
			// Check if we can skip to fit essential items
			if item.Priority > PriorityHigh {
				continue // Skip non-essential
			}
			// Essential/High: try to fit partial
			remaining := availableBudget - c.used
			if remaining > 100 { // At least 100 tokens worth
				partial := c.truncateItem(item, remaining)
				result.WriteString(c.formatItem(partial))
				c.used += partial.Tokens
			}
			break
		}

		result.WriteString(c.formatItem(item))
		c.used += item.Tokens
	}

	// 4. Add mode indicator for LLM awareness
	result.WriteString(c.getModeHint())

	return result.String()
}

// getBudgetPercentage returns the budget allocation for current mode
func (c *ContextOptimizer) getBudgetPercentage() float64 {
	switch c.mode {
	case ModeFull:
		return 0.90
	case ModeFocused:
		return 0.50
	case ModeMinimal:
		return 0.20
	case ModeDelta:
		return 0.15
	case ModeMemory:
		return 0.10
	default:
		return 0.50
	}
}

// sortByPriority sorts items by priority (essential first)
func (c *ContextOptimizer) sortByPriority() []ContextItem {
	// Simple priority sort
	sorted := make([]ContextItem, len(c.items))
	copy(sorted, c.items)

	// Bubble sort (items list is small)
	for i := 0; i < len(sorted); i++ {
		for j := i + 1; j < len(sorted); j++ {
			if sorted[j].Priority < sorted[i].Priority {
				sorted[i], sorted[j] = sorted[j], sorted[i]
			}
		}
	}

	return sorted
}

// truncateItem reduces item content to fit budget
func (c *ContextOptimizer) truncateItem(item ContextItem, maxTokens int) ContextItem {
	maxChars := maxTokens * 4

	if len(item.Content) <= maxChars {
		return item
	}

	// For code: keep first and last portions
	if item.Type == "code" || item.Type == "file" {
		half := maxChars / 2
		truncated := item.Content[:half] + "\n...[truncated]...\n" + item.Content[len(item.Content)-half:]
		item.Content = truncated
		item.Tokens = maxTokens
	} else {
		// For other types: just truncate
		item.Content = item.Content[:maxChars-20] + "\n...[truncated]"
		item.Tokens = maxTokens
	}

	return item
}

// formatItem formats an item for inclusion
func (c *ContextOptimizer) formatItem(item ContextItem) string {
	var b strings.Builder

	switch item.Type {
	case "file", "code":
		b.WriteString(fmt.Sprintf("<file path=\"%s\">\n", item.Path))
		b.WriteString(item.Content)
		b.WriteString("\n</file>\n\n")

	case "function":
		b.WriteString(fmt.Sprintf("<function path=\"%s\" name=\"%s\">\n", item.Path, c.currentFunc))
		b.WriteString(item.Content)
		b.WriteString("\n</function>\n\n")

	case "error":
		b.WriteString("<error>\n")
		b.WriteString(item.Content)
		b.WriteString("\n</error>\n\n")

	case "memory":
		b.WriteString("<memory>\n")
		b.WriteString(item.Content)
		b.WriteString("\n</memory>\n\n")

	default:
		b.WriteString(item.Content)
		b.WriteString("\n\n")
	}

	return b.String()
}

// getModeHint returns a hint for the LLM about current mode
func (c *ContextOptimizer) getModeHint() string {
	var hint string

	switch c.mode {
	case ModeFull:
		hint = "[CONTEXT: Full codebase available. Explore freely.]"
	case ModeFocused:
		hint = fmt.Sprintf("[CONTEXT: Focused on %s. Request other files if needed.]", c.currentFile)
	case ModeMinimal:
		hint = "[CONTEXT: Minimal. Only current function shown. Request more if stuck.]"
	case ModeDelta:
		hint = "[CONTEXT: Delta only. Showing recent changes. You remember the rest.]"
	case ModeMemory:
		hint = "[CONTEXT: Memory mode. Rely on state summary. Request files only if essential.]"
	}

	return "\n" + hint + "\n"
}

// Stats returns current context statistics
func (c *ContextOptimizer) Stats() map[string]int {
	return map[string]int{
		"budget":     c.budget,
		"used":       c.used,
		"items":      len(c.items),
		"turn":       c.turnNumber,
		"mode":       int(c.mode),
		"percentage": int(float64(c.used) / float64(c.budget) * 100),
	}
}

// Clear resets items for next request
func (c *ContextOptimizer) Clear() {
	c.items = c.items[:0]
	c.used = 0
}

// AutoSelectMode automatically selects mode based on situation
func (c *ContextOptimizer) AutoSelectMode(hasError bool, isNewFile bool, editCount int) ContextMode {
	// Error → Full context to understand the problem
	if hasError {
		return ModeFull
	}

	// New file → Full to understand structure
	if isNewFile {
		return ModeFull
	}

	// Many edits to same file → Minimal (we know it well)
	if editCount > 5 {
		return ModeMinimal
	}

	// Few edits → Still focused
	if editCount > 2 {
		return ModeFocused
	}

	// Default: Focused
	return ModeFocused
}

// GetRecommendedMode returns recommended mode with explanation
func (c *ContextOptimizer) GetRecommendedMode(hasError bool, isNewFile bool, editCount int) (ContextMode, string) {
	mode := c.AutoSelectMode(hasError, isNewFile, editCount)

	var reason string
	switch mode {
	case ModeFull:
		if hasError {
			reason = "Error detected - loading full context for debugging"
		} else {
			reason = "New file - loading full context for exploration"
		}
	case ModeFocused:
		reason = fmt.Sprintf("Focused edit (%d changes) - loading relevant context", editCount)
	case ModeMinimal:
		reason = fmt.Sprintf("Familiar territory (%d edits) - minimal context needed", editCount)
	case ModeDelta:
		reason = "Iterating on changes - showing deltas only"
	case ModeMemory:
		reason = "Memory mode - relying on compressed state"
	}

	return mode, reason
}
