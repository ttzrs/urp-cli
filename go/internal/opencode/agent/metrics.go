// Package agent provides task efficiency metrics
package agent

import (
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"
)

// TaskMetrics tracks efficiency metrics for a task execution
type TaskMetrics struct {
	mu sync.RWMutex

	// Identity
	TaskID    string    `json:"task_id"`
	Objective string    `json:"objective"`
	StartTime time.Time `json:"start_time"`
	EndTime   time.Time `json:"end_time,omitempty"`

	// Turn tracking
	Turns        []TurnMetric `json:"turns"`
	CurrentTurn  int          `json:"current_turn"`
	MaxTurns     int          `json:"max_turns"` // Threshold before warning

	// Tool efficiency
	ToolCalls      int            `json:"tool_calls"`
	ToolsByType    map[string]int `json:"tools_by_type"`
	ToolErrors     int            `json:"tool_errors"`
	RedundantCalls int            `json:"redundant_calls"` // Same file read twice, etc.

	// Focus tracking
	ScopeCreep     []string `json:"scope_creep"`      // Actions outside objective
	FilesInScope   []string `json:"files_in_scope"`   // Files mentioned in objective
	FilesTouched   []string `json:"files_touched"`    // Files actually modified
	OutOfScope     []string `json:"out_of_scope"`     // Files modified but not in scope

	// Token efficiency
	InputTokens   int `json:"input_tokens"`
	OutputTokens  int `json:"output_tokens"`
	ThinkingTokens int `json:"thinking_tokens"`
	CacheHits     int `json:"cache_hits"`
	CacheMisses   int `json:"cache_misses"`

	// Quality
	Errors       []string `json:"errors"`
	Recoveries   int      `json:"recoveries"`    // Successful error recovery
	Replans      int      `json:"replans"`       // Times approach changed
	FinalSuccess bool     `json:"final_success"`

	// Derived scores (computed at end)
	Scores *EfficiencyScores `json:"scores,omitempty"`
}

// TurnMetric captures data for a single LLM turn
type TurnMetric struct {
	Turn         int           `json:"turn"`
	Duration     time.Duration `json:"duration"`
	InputTokens  int           `json:"input_tokens"`
	OutputTokens int           `json:"output_tokens"`
	ToolCalls    int           `json:"tool_calls"`
	Phase        string        `json:"phase"`
	Action       string        `json:"action"` // What was done this turn
	OnTrack      bool          `json:"on_track"` // Was this turn advancing objective?
}

// EfficiencyScores are computed metrics
type EfficiencyScores struct {
	// 0-100 scores
	FocusScore      float64 `json:"focus_score"`      // How well stayed on task
	ToolEfficiency  float64 `json:"tool_efficiency"`  // Tools used vs needed
	TokenEfficiency float64 `json:"token_efficiency"` // Tokens vs complexity
	RecoveryScore   float64 `json:"recovery_score"`   // Error handling quality
	OverallScore    float64 `json:"overall_score"`    // Weighted average

	// Raw ratios
	TurnsPerTask     float64 `json:"turns_per_task"`
	TokensPerTurn    float64 `json:"tokens_per_turn"`
	ErrorRate        float64 `json:"error_rate"`
	ScopeCreepRate   float64 `json:"scope_creep_rate"`
}

// NewTaskMetrics creates a metrics tracker for a task
func NewTaskMetrics(taskID, objective string) *TaskMetrics {
	return &TaskMetrics{
		TaskID:      taskID,
		Objective:   objective,
		StartTime:   time.Now(),
		MaxTurns:    10,
		ToolsByType: make(map[string]int),
		Turns:       make([]TurnMetric, 0),
	}
}

// RecordTurn logs a turn's metrics
func (m *TaskMetrics) RecordTurn(inputTok, outputTok, toolCalls int, phase, action string, onTrack bool) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.CurrentTurn++
	turn := TurnMetric{
		Turn:         m.CurrentTurn,
		InputTokens:  inputTok,
		OutputTokens: outputTok,
		ToolCalls:    toolCalls,
		Phase:        phase,
		Action:       action,
		OnTrack:      onTrack,
	}

	if len(m.Turns) > 0 {
		// Calculate duration from last turn
		turn.Duration = time.Since(m.StartTime) - m.totalDuration()
	}

	m.Turns = append(m.Turns, turn)
	m.InputTokens += inputTok
	m.OutputTokens += outputTok
	m.ToolCalls += toolCalls
}

func (m *TaskMetrics) totalDuration() time.Duration {
	var d time.Duration
	for _, t := range m.Turns {
		d += t.Duration
	}
	return d
}

// RecordTool logs a tool execution
func (m *TaskMetrics) RecordTool(name string, hadError bool) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.ToolsByType[name]++
	if hadError {
		m.ToolErrors++
	}
}

// RecordScopeCreep marks an action outside the objective
func (m *TaskMetrics) RecordScopeCreep(action string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.ScopeCreep = append(m.ScopeCreep, action)
}

// RecordFileModified tracks a file being changed
func (m *TaskMetrics) RecordFileModified(path string, inScope bool) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if !contains(m.FilesTouched, path) {
		m.FilesTouched = append(m.FilesTouched, path)
	}
	if !inScope && !contains(m.OutOfScope, path) {
		m.OutOfScope = append(m.OutOfScope, path)
	}
}

// RecordError logs an error
func (m *TaskMetrics) RecordError(err string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.Errors = append(m.Errors, err)
}

// RecordRecovery logs successful error recovery
func (m *TaskMetrics) RecordRecovery() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.Recoveries++
}

// RecordReplan logs an approach change
func (m *TaskMetrics) RecordReplan() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.Replans++
}

// Finish computes final scores
func (m *TaskMetrics) Finish(success bool) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.EndTime = time.Now()
	m.FinalSuccess = success
	m.Scores = m.computeScores()
}

func (m *TaskMetrics) computeScores() *EfficiencyScores {
	s := &EfficiencyScores{}

	// Turns per task
	s.TurnsPerTask = float64(m.CurrentTurn)

	// Tokens per turn
	totalTokens := float64(m.InputTokens + m.OutputTokens + m.ThinkingTokens)
	if m.CurrentTurn > 0 {
		s.TokensPerTurn = totalTokens / float64(m.CurrentTurn)
	}

	// Error rate
	if m.ToolCalls > 0 {
		s.ErrorRate = float64(m.ToolErrors) / float64(m.ToolCalls)
	}

	// Scope creep rate
	totalActions := len(m.FilesTouched) + len(m.ScopeCreep)
	if totalActions > 0 {
		s.ScopeCreepRate = float64(len(m.ScopeCreep)+len(m.OutOfScope)) / float64(totalActions)
	}

	// Focus score (100 = perfect focus)
	onTrackTurns := 0
	for _, t := range m.Turns {
		if t.OnTrack {
			onTrackTurns++
		}
	}
	if m.CurrentTurn > 0 {
		s.FocusScore = float64(onTrackTurns) / float64(m.CurrentTurn) * 100
	}

	// Tool efficiency (penalize redundant calls and errors)
	if m.ToolCalls > 0 {
		effective := m.ToolCalls - m.RedundantCalls - m.ToolErrors
		s.ToolEfficiency = float64(effective) / float64(m.ToolCalls) * 100
		if s.ToolEfficiency < 0 {
			s.ToolEfficiency = 0
		}
	} else {
		s.ToolEfficiency = 100
	}

	// Token efficiency (based on expected tokens per turn)
	expectedTokensPerTurn := 2000.0 // Baseline
	if s.TokensPerTurn > 0 {
		ratio := expectedTokensPerTurn / s.TokensPerTurn
		if ratio > 1 {
			ratio = 1
		}
		s.TokenEfficiency = ratio * 100
	}

	// Recovery score
	if len(m.Errors) > 0 {
		s.RecoveryScore = float64(m.Recoveries) / float64(len(m.Errors)) * 100
	} else {
		s.RecoveryScore = 100
	}

	// Overall score (weighted)
	s.OverallScore = (s.FocusScore*0.4 + s.ToolEfficiency*0.25 +
		s.TokenEfficiency*0.2 + s.RecoveryScore*0.15)

	// Bonus for success, penalty for failure
	if m.FinalSuccess {
		s.OverallScore = minFloat(s.OverallScore*1.1, 100)
	} else {
		s.OverallScore *= 0.7
	}

	return s
}

// Report generates a human-readable report
func (m *TaskMetrics) Report() string {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var b strings.Builder

	b.WriteString("═══════════════════════════════════════════\n")
	b.WriteString("           TASK EFFICIENCY REPORT          \n")
	b.WriteString("═══════════════════════════════════════════\n\n")

	// Task info
	b.WriteString(fmt.Sprintf("Task ID:    %s\n", m.TaskID))
	obj := m.Objective
	if len(obj) > 60 {
		obj = obj[:57] + "..."
	}
	b.WriteString(fmt.Sprintf("Objective:  %s\n", obj))
	b.WriteString(fmt.Sprintf("Duration:   %s\n", m.EndTime.Sub(m.StartTime).Round(time.Millisecond)))
	b.WriteString(fmt.Sprintf("Success:    %v\n\n", m.FinalSuccess))

	// Execution stats
	b.WriteString("─── EXECUTION ───\n")
	b.WriteString(fmt.Sprintf("Turns:      %d\n", m.CurrentTurn))
	b.WriteString(fmt.Sprintf("Tool Calls: %d (errors: %d)\n", m.ToolCalls, m.ToolErrors))
	b.WriteString(fmt.Sprintf("Replans:    %d\n", m.Replans))

	// Token usage
	b.WriteString("\n─── TOKENS ───\n")
	b.WriteString(fmt.Sprintf("Input:      %d\n", m.InputTokens))
	b.WriteString(fmt.Sprintf("Output:     %d\n", m.OutputTokens))
	b.WriteString(fmt.Sprintf("Thinking:   %d\n", m.ThinkingTokens))
	b.WriteString(fmt.Sprintf("Total:      %d\n", m.InputTokens+m.OutputTokens+m.ThinkingTokens))

	// Focus
	b.WriteString("\n─── FOCUS ───\n")
	b.WriteString(fmt.Sprintf("Files modified:  %d\n", len(m.FilesTouched)))
	b.WriteString(fmt.Sprintf("Out of scope:    %d\n", len(m.OutOfScope)))
	b.WriteString(fmt.Sprintf("Scope creep:     %d actions\n", len(m.ScopeCreep)))

	// Scores
	if m.Scores != nil {
		b.WriteString("\n─── SCORES ───\n")
		b.WriteString(fmt.Sprintf("Focus:       %.1f/100\n", m.Scores.FocusScore))
		b.WriteString(fmt.Sprintf("Tools:       %.1f/100\n", m.Scores.ToolEfficiency))
		b.WriteString(fmt.Sprintf("Tokens:      %.1f/100\n", m.Scores.TokenEfficiency))
		b.WriteString(fmt.Sprintf("Recovery:    %.1f/100\n", m.Scores.RecoveryScore))
		b.WriteString(fmt.Sprintf("─────────────────\n"))
		b.WriteString(fmt.Sprintf("OVERALL:     %.1f/100 ", m.Scores.OverallScore))
		b.WriteString(m.gradeEmoji(m.Scores.OverallScore))
		b.WriteString("\n")
	}

	b.WriteString("\n═══════════════════════════════════════════\n")
	return b.String()
}

func (m *TaskMetrics) gradeEmoji(score float64) string {
	switch {
	case score >= 90:
		return "⭐ Excellent"
	case score >= 75:
		return "✅ Good"
	case score >= 60:
		return "⚠️ Needs improvement"
	default:
		return "❌ Poor"
	}
}

// JSON returns metrics as JSON
func (m *TaskMetrics) JSON() string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	data, _ := json.MarshalIndent(m, "", "  ")
	return string(data)
}

func minFloat(a, b float64) float64 {
	if a < b {
		return a
	}
	return b
}
