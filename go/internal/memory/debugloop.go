// Package memory provides debug loop (Propose-Test-Analyze-Refine) functionality.
package memory

import (
	"context"
	"sync"
	"time"

	"github.com/joss/urp/internal/graph"
)

// DebugState represents the current state of a debug session.
type DebugState string

const (
	StatePropose DebugState = "propose" // Generating hypotheses
	StateTest    DebugState = "test"    // Testing hypotheses
	StateAnalyze DebugState = "analyze" // Analyzing results
	StateRefine  DebugState = "refine"  // Refining based on analysis
	StateResolved DebugState = "resolved" // Bug fixed
	StateFailed  DebugState = "failed"  // Unable to resolve
)

// Hypothesis represents a proposed cause/fix.
type Hypothesis struct {
	ID          string            `json:"id"`
	Description string            `json:"description"`
	Confidence  float64           `json:"confidence"`
	TestCmd     string            `json:"test_cmd"`      // Command to test this hypothesis
	TestResult  *TestResult       `json:"test_result"`   // Result after testing
	Metadata    map[string]string `json:"metadata"`
	CreatedAt   time.Time         `json:"created_at"`
	TestedAt    *time.Time        `json:"tested_at"`
	Priority    int               `json:"priority"` // Higher = test first
}

// TestResult captures the outcome of testing a hypothesis.
type TestResult struct {
	Passed       bool     `json:"passed"`
	Output       string   `json:"output"`
	ErrorPattern string   `json:"error_pattern"` // Extracted error pattern
	Duration     float64  `json:"duration_ms"`
	Insights     []string `json:"insights"` // What we learned
}

// DebugSession tracks a single debug loop session.
type DebugSession struct {
	ID            string       `json:"id"`
	Problem       string       `json:"problem"`        // Original problem description
	ErrorSignature string      `json:"error_signature"` // Unique error identifier
	State         DebugState   `json:"state"`
	Hypotheses    []Hypothesis `json:"hypotheses"`
	Iterations    int          `json:"iterations"`
	MaxIterations int          `json:"max_iterations"`
	StartedAt     time.Time    `json:"started_at"`
	ResolvedAt    *time.Time   `json:"resolved_at"`
	Resolution    string       `json:"resolution"` // How it was fixed
	SessionID     string       `json:"session_id"` // Parent session
}

// DebugLoop orchestrates the Propose-Test-Analyze-Refine cycle.
type DebugLoop struct {
	mu       sync.RWMutex
	sessions map[string]*DebugSession
	db       graph.Driver
	learner  *AutoLearner
	config   DebugLoopConfig
}

// DebugLoopConfig configures the debug loop behavior.
type DebugLoopConfig struct {
	MaxIterations      int     // Max PTAR cycles (default: 10)
	MinConfidence      float64 // Min confidence to test (default: 0.3)
	AutoPromoteSuccess bool    // Auto-promote successful fixes (default: true)
}

// DefaultDebugLoopConfig returns sensible defaults.
func DefaultDebugLoopConfig() DebugLoopConfig {
	return DebugLoopConfig{
		MaxIterations:      10,
		MinConfidence:      0.3,
		AutoPromoteSuccess: true,
	}
}

// NewDebugLoop creates a new debug loop.
func NewDebugLoop(db graph.Driver, learner *AutoLearner, config DebugLoopConfig) *DebugLoop {
	if config.MaxIterations <= 0 {
		config.MaxIterations = 10
	}
	if config.MinConfidence <= 0 {
		config.MinConfidence = 0.3
	}

	return &DebugLoop{
		sessions: make(map[string]*DebugSession),
		db:       db,
		learner:  learner,
		config:   config,
	}
}

// StartSession begins a new debug session.
func (d *DebugLoop) StartSession(problem, errorSignature, parentSessionID string) *DebugSession {
	d.mu.Lock()
	defer d.mu.Unlock()

	session := &DebugSession{
		ID:             generateDebugID(),
		Problem:        problem,
		ErrorSignature: errorSignature,
		State:          StatePropose,
		Hypotheses:     make([]Hypothesis, 0),
		MaxIterations:  d.config.MaxIterations,
		StartedAt:      time.Now(),
		SessionID:      parentSessionID,
	}

	d.sessions[session.ID] = session
	return session
}

// GetSession retrieves a debug session.
func (d *DebugLoop) GetSession(sessionID string) *DebugSession {
	d.mu.RLock()
	defer d.mu.RUnlock()

	return d.sessions[sessionID]
}

// Propose adds hypotheses to a debug session.
func (d *DebugLoop) Propose(sessionID string, hypotheses []Hypothesis) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	session, exists := d.sessions[sessionID]
	if !exists {
		return nil
	}

	for i := range hypotheses {
		hypotheses[i].ID = generateHypothesisID()
		hypotheses[i].CreatedAt = time.Now()
		if hypotheses[i].Metadata == nil {
			hypotheses[i].Metadata = make(map[string]string)
		}
	}

	session.Hypotheses = append(session.Hypotheses, hypotheses...)

	// Sort by priority descending, then confidence descending
	d.sortHypotheses(session)

	session.State = StateTest
	return nil
}

// sortHypotheses sorts by priority then confidence.
func (d *DebugLoop) sortHypotheses(session *DebugSession) {
	h := session.Hypotheses
	for i := 0; i < len(h); i++ {
		for j := i + 1; j < len(h); j++ {
			swap := false
			if h[j].Priority > h[i].Priority {
				swap = true
			} else if h[j].Priority == h[i].Priority && h[j].Confidence > h[i].Confidence {
				swap = true
			}
			if swap {
				h[i], h[j] = h[j], h[i]
			}
		}
	}
}

// GetNextHypothesis returns the next untested hypothesis.
func (d *DebugLoop) GetNextHypothesis(sessionID string) *Hypothesis {
	d.mu.RLock()
	defer d.mu.RUnlock()

	session, exists := d.sessions[sessionID]
	if !exists {
		return nil
	}

	for i := range session.Hypotheses {
		if session.Hypotheses[i].TestResult == nil &&
			session.Hypotheses[i].Confidence >= d.config.MinConfidence {
			return &session.Hypotheses[i]
		}
	}
	return nil
}

// RecordTestResult records the result of testing a hypothesis.
func (d *DebugLoop) RecordTestResult(sessionID, hypothesisID string, result TestResult) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	session, exists := d.sessions[sessionID]
	if !exists {
		return nil
	}

	now := time.Now()
	for i := range session.Hypotheses {
		if session.Hypotheses[i].ID == hypothesisID {
			session.Hypotheses[i].TestResult = &result
			session.Hypotheses[i].TestedAt = &now

			if result.Passed {
				// Success! Move to analyze
				session.State = StateAnalyze
			}
			return nil
		}
	}

	return nil
}

// Analyze transitions to analyze state and extracts insights.
func (d *DebugLoop) Analyze(sessionID string) []string {
	d.mu.Lock()
	defer d.mu.Unlock()

	session, exists := d.sessions[sessionID]
	if !exists {
		return nil
	}

	var insights []string

	// Analyze all test results
	passedCount := 0
	failedCount := 0
	var errorPatterns []string

	for _, h := range session.Hypotheses {
		if h.TestResult != nil {
			if h.TestResult.Passed {
				passedCount++
				insights = append(insights, "Hypothesis '"+h.Description+"' succeeded")
			} else {
				failedCount++
				if h.TestResult.ErrorPattern != "" {
					errorPatterns = append(errorPatterns, h.TestResult.ErrorPattern)
				}
			}
			insights = append(insights, h.TestResult.Insights...)
		}
	}

	// Generate summary insights
	if passedCount > 0 {
		insights = append(insights, "Found working solution after "+string(rune(session.Iterations+48))+" iterations")
	}

	if len(errorPatterns) > 0 {
		insights = append(insights, "Common error patterns: "+errorPatterns[0])
	}

	session.State = StateRefine
	return insights
}

// Refine adjusts hypotheses based on analysis and continues or concludes.
func (d *DebugLoop) Refine(sessionID string, newHypotheses []Hypothesis) (bool, error) {
	d.mu.Lock()
	defer d.mu.Unlock()

	session, exists := d.sessions[sessionID]
	if !exists {
		return false, nil
	}

	session.Iterations++

	// Check for success
	for _, h := range session.Hypotheses {
		if h.TestResult != nil && h.TestResult.Passed {
			now := time.Now()
			session.State = StateResolved
			session.ResolvedAt = &now
			session.Resolution = h.Description

			// Learn from success
			if d.learner != nil && d.config.AutoPromoteSuccess {
				d.learner.RecordFix(session.Problem, h.Description, session.SessionID, map[string]string{
					"error_signature": session.ErrorSignature,
					"iterations":      string(rune(session.Iterations + 48)),
				})
			}

			return true, nil // Resolved
		}
	}

	// Check iteration limit
	if session.Iterations >= session.MaxIterations {
		session.State = StateFailed
		return false, nil // Failed
	}

	// Add new refined hypotheses
	if len(newHypotheses) > 0 {
		for i := range newHypotheses {
			newHypotheses[i].ID = generateHypothesisID()
			newHypotheses[i].CreatedAt = time.Now()
			// Boost priority for refined hypotheses
			newHypotheses[i].Priority += 10
		}
		session.Hypotheses = append(session.Hypotheses, newHypotheses...)
		d.sortHypotheses(session)
	}

	session.State = StateTest
	return false, nil // Continue
}

// MarkResolved manually marks a session as resolved.
func (d *DebugLoop) MarkResolved(sessionID, resolution string) {
	d.mu.Lock()
	defer d.mu.Unlock()

	session, exists := d.sessions[sessionID]
	if !exists {
		return
	}

	now := time.Now()
	session.State = StateResolved
	session.ResolvedAt = &now
	session.Resolution = resolution

	if d.learner != nil && d.config.AutoPromoteSuccess {
		d.learner.RecordFix(session.Problem, resolution, session.SessionID, map[string]string{
			"error_signature": session.ErrorSignature,
		})
	}
}

// MarkFailed manually marks a session as failed.
func (d *DebugLoop) MarkFailed(sessionID string) {
	d.mu.Lock()
	defer d.mu.Unlock()

	session, exists := d.sessions[sessionID]
	if !exists {
		return
	}

	session.State = StateFailed
}

// GetActiveSessions returns all non-resolved sessions.
func (d *DebugLoop) GetActiveSessions() []*DebugSession {
	d.mu.RLock()
	defer d.mu.RUnlock()

	var active []*DebugSession
	for _, s := range d.sessions {
		if s.State != StateResolved && s.State != StateFailed {
			active = append(active, s)
		}
	}
	return active
}

// SuggestHypotheses generates initial hypotheses based on error signature.
func (d *DebugLoop) SuggestHypotheses(ctx context.Context, errorSignature, problem string) []Hypothesis {
	var suggestions []Hypothesis

	// Query for similar past solutions
	if d.db != nil {
		query := `
			MATCH (k:Knowledge)
			WHERE k.kind = 'solution' AND k.text CONTAINS $error
			RETURN k.text as text, k.confidence as confidence
			LIMIT 5
		`

		records, err := d.db.Execute(ctx, query, map[string]any{"error": errorSignature})
		if err == nil {
			for _, r := range records {
				text := graph.GetString(r, "text")
				conf := 0.5
				if c, ok := r["confidence"].(float64); ok {
					conf = c
				}

				suggestions = append(suggestions, Hypothesis{
					Description: text,
					Confidence:  conf,
					Priority:    5, // Learned solutions get priority
				})
			}
		}
	}

	// Query learner for recent similar fixes
	if d.learner != nil {
		related := d.learner.SuggestRelated("", errorSignature)
		for _, e := range related {
			if e.Solution != "" {
				suggestions = append(suggestions, Hypothesis{
					Description: e.Solution,
					Confidence:  e.Confidence,
					Priority:    3,
				})
			}
		}
	}

	return suggestions
}

// Stats returns debug loop statistics.
func (d *DebugLoop) Stats() map[string]any {
	d.mu.RLock()
	defer d.mu.RUnlock()

	resolved := 0
	failed := 0
	active := 0
	totalIterations := 0

	for _, s := range d.sessions {
		switch s.State {
		case StateResolved:
			resolved++
		case StateFailed:
			failed++
		default:
			active++
		}
		totalIterations += s.Iterations
	}

	avgIterations := 0.0
	if resolved > 0 {
		avgIterations = float64(totalIterations) / float64(resolved)
	}

	return map[string]any{
		"total_sessions":   len(d.sessions),
		"resolved":         resolved,
		"failed":           failed,
		"active":           active,
		"avg_iterations":   avgIterations,
		"total_iterations": totalIterations,
	}
}

// PersistToGraph saves resolved debug sessions to the graph.
func (d *DebugLoop) PersistToGraph(ctx context.Context) error {
	if d.db == nil {
		return nil
	}

	d.mu.RLock()
	defer d.mu.RUnlock()

	for _, s := range d.sessions {
		if s.State != StateResolved || s.Resolution == "" {
			continue
		}

		query := `
			MERGE (d:DebugSession {id: $id})
			SET d.problem = $problem,
			    d.error_signature = $error_sig,
			    d.resolution = $resolution,
			    d.iterations = $iterations,
			    d.resolved_at = $resolved_at
		`

		var resolvedAt int64
		if s.ResolvedAt != nil {
			resolvedAt = s.ResolvedAt.Unix()
		}

		err := d.db.ExecuteWrite(ctx, query, map[string]any{
			"id":          s.ID,
			"problem":     s.Problem,
			"error_sig":   s.ErrorSignature,
			"resolution":  s.Resolution,
			"iterations":  s.Iterations,
			"resolved_at": resolvedAt,
		})
		if err != nil {
			return err
		}
	}

	return nil
}

var debugIDCounter int64

func generateDebugID() string {
	debugIDCounter++
	return "dbg-" + time.Now().Format("20060102150405") + "-" + string(rune('a'+debugIDCounter%26))
}

func generateHypothesisID() string {
	return "hyp-" + time.Now().Format("150405.000000")
}
