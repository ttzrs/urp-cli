// Package cognitive provides the CognitiveEngine that integrates
// all cognitive subsystems for intelligent context management
package cognitive

import (
	"context"
	"sync"

	"github.com/joss/urp/internal/opencode/domain"
)

// Engine integrates all cognitive subsystems
// This is the main entry point for cognitive operations
type Engine struct {
	mu sync.RWMutex

	// Subsystems
	optimizer *ContextOptimizer // Dynamic context allocation
	signals   *SignalInjector   // System signal injection
	hygiene   *Hygiene          // Memory cleanup
	reflex    *Reflex           // Error handling (optional, needs graph)

	// State
	tokenBudget   int     // Total token budget for context
	currentFile   string  // File currently being edited
	currentFunc   string  // Function currently being edited
	editCount     int     // Number of edits in current task
	hasError      bool    // Whether we're handling an error
	isNewFile     bool    // Whether we're working on a new file
	profile       AgentProfile
}

// EngineConfig configures the cognitive engine
type EngineConfig struct {
	TokenBudget int          // Total context token budget
	Profile     AgentProfile // Agent profile for formatting
	Hygiene     HygieneConfig
}

// DefaultEngineConfig returns sensible defaults
func DefaultEngineConfig() EngineConfig {
	return EngineConfig{
		TokenBudget: 200000,
		Profile:     ProfileBuild,
		Hygiene:     DefaultHygieneConfig(),
	}
}

// NewEngine creates a new cognitive engine
func NewEngine(cfg EngineConfig) *Engine {
	return &Engine{
		optimizer:   NewContextOptimizer(cfg.TokenBudget),
		signals:     NewSignalInjector(cfg.Profile),
		hygiene:     NewHygiene(cfg.Hygiene),
		tokenBudget: cfg.TokenBudget,
		profile:     cfg.Profile,
	}
}

// WithReflex adds error context enrichment (requires graph connection)
func (e *Engine) WithReflex(r *Reflex) *Engine {
	e.reflex = r
	return e
}

// --- State Updates ---

// SetCurrentEdit updates the current editing context
func (e *Engine) SetCurrentEdit(file, function string) {
	e.mu.Lock()
	defer e.mu.Unlock()

	if file != e.currentFile {
		// New file - reset counters
		e.isNewFile = true
		e.editCount = 0
	} else {
		// Same file - no longer "new" after first edit
		e.isNewFile = false
		e.editCount++
	}

	e.currentFile = file
	e.currentFunc = function
	e.optimizer.SetCurrentEdit(file, function)
}

// SetTokenUsage updates current context window usage (0.0 - 1.0)
func (e *Engine) SetTokenUsage(usage float64) {
	e.signals.SetTokenUsage(usage)
}

// SetError marks that we're handling an error
func (e *Engine) SetError(errorMsg string) {
	e.mu.Lock()
	e.hasError = true
	e.mu.Unlock()

	e.optimizer.SetError(errorMsg)
	e.signals.AddError("tool", errorMsg)
}

// ClearError clears error state
func (e *Engine) ClearError() {
	e.mu.Lock()
	e.hasError = false
	e.mu.Unlock()

	e.optimizer.ClearError()
	e.signals.ClearType(SignalError)
}

// SetDockerHealth updates Docker status
func (e *Engine) SetDockerHealth(healthy bool, err string) {
	e.signals.SetDockerHealth(healthy, err)
}

// SetGraphHealth updates graph database status
func (e *Engine) SetGraphHealth(healthy bool) {
	e.signals.SetGraphHealth(healthy)
}

// AddRetry records an autocorrection attempt
func (e *Engine) AddRetry(attempt, maxAttempts int) {
	e.signals.AddRetry(attempt, maxAttempts)
}

// NextTurn advances the conversation turn counter
func (e *Engine) NextTurn() {
	e.optimizer.NextTurn()
}

// --- Context Optimization ---

// OptimizeMode returns the recommended context mode and reason
func (e *Engine) OptimizeMode() (ContextMode, string) {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.optimizer.GetRecommendedMode(e.hasError, e.isNewFile, e.editCount)
}

// SetMode overrides the automatic mode selection
func (e *Engine) SetMode(mode ContextMode) {
	e.optimizer.SetMode(mode)
}

// AddContextItem adds a potential context item
func (e *Engine) AddContextItem(item ContextItem) {
	e.optimizer.AddItem(item)
}

// BuildContext constructs the optimized context string
func (e *Engine) BuildContext() string {
	mode, _ := e.OptimizeMode()
	e.optimizer.SetMode(mode)
	return e.optimizer.Build()
}

// ClearContext clears pending context items
func (e *Engine) ClearContext() {
	e.optimizer.Clear()
}

// --- Signal Injection ---

// ShouldInjectSignals determines if signals should be injected
func (e *Engine) ShouldInjectSignals(normalMsgTokens int) InjectionDecision {
	return e.signals.ShouldInject(normalMsgTokens, e.tokenBudget)
}

// InjectSignals injects system signals into a prompt if appropriate
func (e *Engine) InjectSignals(prompt string) (string, bool) {
	return e.signals.InjectIntoPrompt(prompt, e.tokenBudget)
}

// HasUrgentSignals returns true if there are urgent signals
func (e *Engine) HasUrgentSignals() bool {
	return e.signals.HasUrgent()
}

// --- Memory Hygiene ---

// CleanMessages applies memory hygiene to message history
func (e *Engine) CleanMessages(messages []domain.Message, taskSolved bool) []domain.Message {
	return e.hygiene.CleanMessages(messages, taskSolved)
}

// EstimateTokenSavings estimates tokens saved by cleaning
func (e *Engine) EstimateTokenSavings(original, cleaned []domain.Message) int {
	return e.hygiene.EstimateTokenSavings(original, cleaned)
}

// --- Error Handling ---

// HandleError enriches error context using graph (if available)
func (e *Engine) HandleError(ctx context.Context, errorOutput, currentFile string) (*TraumaContext, error) {
	e.SetError(errorOutput)

	if e.reflex != nil {
		return e.reflex.HandleTrauma(ctx, errorOutput, currentFile)
	}

	// Basic trauma context without graph
	paths := extractFilePaths(errorOutput)
	return &TraumaContext{
		Error:        truncateError(errorOutput, 500),
		RelatedFiles: paths,
	}, nil
}

// FormatErrorContext formats error context for LLM
func (e *Engine) FormatErrorContext(tc *TraumaContext) string {
	if e.reflex != nil {
		return e.reflex.FormatEmergencyContext(tc)
	}

	// Basic format without graph enrichment
	return "<emergency-context>\n<error>\n" + tc.Error + "\n</error>\n</emergency-context>"
}

// --- Statistics ---

// Stats returns cognitive engine statistics
func (e *Engine) Stats() map[string]interface{} {
	e.mu.RLock()
	defer e.mu.RUnlock()

	contextStats := e.optimizer.Stats()
	signalStats := e.signals.SignalStats()

	return map[string]interface{}{
		"context": map[string]int{
			"budget":     contextStats["budget"],
			"used":       contextStats["used"],
			"items":      contextStats["items"],
			"turn":       contextStats["turn"],
			"mode":       contextStats["mode"],
			"percentage": contextStats["percentage"],
		},
		"signals": map[string]int{
			"total":  signalStats["total"],
			"urgent": signalStats["urgent"],
		},
		"state": map[string]interface{}{
			"currentFile": e.currentFile,
			"editCount":   e.editCount,
			"hasError":    e.hasError,
			"isNewFile":   e.isNewFile,
			"profile":     int(e.profile),
		},
	}
}
