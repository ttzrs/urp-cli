// Package agent provides budget tracking for cost management
package agent

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// CostRecord represents a single cost entry
type CostRecord struct {
	ModelID   string    `json:"model_id"`
	Cost      float64   `json:"cost"`
	Tokens    int       `json:"tokens"`
	TaskType  string    `json:"task_type"`
	Timestamp time.Time `json:"timestamp"`
}

// ModelCosts tracks costs per model
type ModelCosts struct {
	ModelID    string  `json:"model_id"`
	TotalCost  float64 `json:"total_cost"`
	CallCount  int     `json:"call_count"`
	AvgCost    float64 `json:"avg_cost"`
	TotalTokens int    `json:"total_tokens"`
}

// BudgetTracker manages cost limits and tracks spending
type BudgetTracker struct {
	mu             sync.RWMutex
	DailyLimit     float64
	SessionLimit   float64
	MaxPerTask     float64 // Max cost per task
	CurrentDaily   float64
	CurrentSession float64
	LastReset      time.Time

	// Detailed tracking
	history    []CostRecord
	modelCosts map[string]*ModelCosts
	alertFunc  func(msg string) // Optional alert callback

	// Persistence
	persistPath string
	maxHistory  int
}

// NewBudgetTracker creates a budget tracker with default limits
func NewBudgetTracker() *BudgetTracker {
	return &BudgetTracker{
		DailyLimit:   5.0,  // $5/day
		SessionLimit: 1.0,  // $1/session
		MaxPerTask:   0.20, // $0.20/task
		LastReset:    time.Now(),
		history:      make([]CostRecord, 0),
		modelCosts:   make(map[string]*ModelCosts),
		maxHistory:   500,
	}
}

// NewBudgetTrackerWithLimits creates a budget tracker with custom limits
func NewBudgetTrackerWithLimits(daily, session, perTask float64) *BudgetTracker {
	return &BudgetTracker{
		DailyLimit:   daily,
		SessionLimit: session,
		MaxPerTask:   perTask,
		LastReset:    time.Now(),
		history:      make([]CostRecord, 0),
		modelCosts:   make(map[string]*ModelCosts),
		maxHistory:   500,
	}
}

// NewBudgetTrackerWithPath creates a budget tracker with persistence
func NewBudgetTrackerWithPath(path string) *BudgetTracker {
	b := NewBudgetTracker()
	b.persistPath = path
	b.Load() // Load existing data
	return b
}

// CanAffordTask checks if we can afford the estimated cost
func (b *BudgetTracker) CanAffordTask(estCost float64) bool {
	b.mu.RLock()
	defer b.mu.RUnlock()

	b.maybeResetDailyUnlocked()

	// Check all limits
	if b.CurrentDaily+estCost > b.DailyLimit {
		return false
	}
	if b.CurrentSession+estCost > b.SessionLimit {
		return false
	}
	if b.MaxPerTask > 0 && estCost > b.MaxPerTask {
		return false
	}
	return true
}

// Record adds a cost to the tracker (simple version)
func (b *BudgetTracker) Record(cost float64) {
	b.RecordDetailed("", cost, 0, "")
}

// RecordDetailed adds a cost with full details
func (b *BudgetTracker) RecordDetailed(modelID string, cost float64, tokens int, taskType string) {
	b.mu.Lock()
	defer b.mu.Unlock()

	b.maybeResetDailyUnlocked()
	b.CurrentDaily += cost
	b.CurrentSession += cost

	// Add to history
	record := CostRecord{
		ModelID:   modelID,
		Cost:      cost,
		Tokens:    tokens,
		TaskType:  taskType,
		Timestamp: time.Now(),
	}
	b.history = append(b.history, record)

	// Trim history if too long
	if len(b.history) > b.maxHistory {
		b.history = b.history[len(b.history)-b.maxHistory:]
	}

	// Update model costs
	if modelID != "" {
		mc, exists := b.modelCosts[modelID]
		if !exists {
			mc = &ModelCosts{ModelID: modelID}
			b.modelCosts[modelID] = mc
		}
		mc.TotalCost += cost
		mc.CallCount++
		mc.AvgCost = mc.TotalCost / float64(mc.CallCount)
		mc.TotalTokens += tokens
	}

	// Check alerts
	b.checkAlertsUnlocked()

	// Auto-save
	if b.persistPath != "" {
		b.saveUnlocked()
	}
}

// maybeResetDailyUnlocked resets daily counter if day changed (must hold lock)
func (b *BudgetTracker) maybeResetDailyUnlocked() {
	now := time.Now()
	if now.YearDay() != b.LastReset.YearDay() || now.Year() != b.LastReset.Year() {
		b.CurrentDaily = 0
		b.LastReset = now
	}
}

// checkAlertsUnlocked sends alerts if thresholds exceeded (must hold lock)
func (b *BudgetTracker) checkAlertsUnlocked() {
	if b.alertFunc == nil {
		return
	}

	// Alert at 80% daily
	if b.CurrentDaily > b.DailyLimit*0.8 && b.CurrentDaily-0.01 <= b.DailyLimit*0.8 {
		b.alertFunc("Budget alert: Daily spending at 80%")
	}
	// Alert at 80% session
	if b.CurrentSession > b.SessionLimit*0.8 && b.CurrentSession-0.01 <= b.SessionLimit*0.8 {
		b.alertFunc("Budget alert: Session spending at 80%")
	}
	// Alert if over budget
	if b.CurrentDaily > b.DailyLimit {
		b.alertFunc("Budget alert: Daily limit exceeded!")
	}
	if b.CurrentSession > b.SessionLimit {
		b.alertFunc("Budget alert: Session limit exceeded!")
	}
}

// ResetSession resets the session counter
func (b *BudgetTracker) ResetSession() {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.CurrentSession = 0
}

// GetRemaining returns remaining budget for session and daily
func (b *BudgetTracker) GetRemaining() (session, daily float64) {
	b.mu.RLock()
	defer b.mu.RUnlock()

	b.maybeResetDailyUnlocked()
	return b.SessionLimit - b.CurrentSession, b.DailyLimit - b.CurrentDaily
}

// GetUsage returns current usage
func (b *BudgetTracker) GetUsage() (session, daily float64) {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.CurrentSession, b.CurrentDaily
}

// GetUsagePercent returns usage as percentages
func (b *BudgetTracker) GetUsagePercent() (session, daily float64) {
	b.mu.RLock()
	defer b.mu.RUnlock()

	if b.SessionLimit > 0 {
		session = b.CurrentSession / b.SessionLimit * 100
	}
	if b.DailyLimit > 0 {
		daily = b.CurrentDaily / b.DailyLimit * 100
	}
	return
}

// SetLimits updates the budget limits
func (b *BudgetTracker) SetLimits(daily, session float64) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.DailyLimit = daily
	b.SessionLimit = session
}

// SetMaxPerTask sets the maximum cost per task
func (b *BudgetTracker) SetMaxPerTask(max float64) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.MaxPerTask = max
}

// SetAlertFunc sets the alert callback
func (b *BudgetTracker) SetAlertFunc(f func(string)) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.alertFunc = f
}

// IsOverBudget returns true if any limit is exceeded
func (b *BudgetTracker) IsOverBudget() bool {
	b.mu.RLock()
	defer b.mu.RUnlock()

	b.maybeResetDailyUnlocked()
	return b.CurrentDaily > b.DailyLimit || b.CurrentSession > b.SessionLimit
}

// GetModelCosts returns cost breakdown by model
func (b *BudgetTracker) GetModelCosts() map[string]*ModelCosts {
	b.mu.RLock()
	defer b.mu.RUnlock()

	result := make(map[string]*ModelCosts)
	for k, v := range b.modelCosts {
		copy := *v
		result[k] = &copy
	}
	return result
}

// GetHistory returns recent cost records
func (b *BudgetTracker) GetHistory(limit int) []CostRecord {
	b.mu.RLock()
	defer b.mu.RUnlock()

	if limit <= 0 || limit > len(b.history) {
		limit = len(b.history)
	}

	// Return most recent
	start := len(b.history) - limit
	if start < 0 {
		start = 0
	}

	result := make([]CostRecord, limit)
	copy(result, b.history[start:])
	return result
}

// Summary returns a budget summary
type BudgetSummary struct {
	DailyLimit     float64            `json:"daily_limit"`
	SessionLimit   float64            `json:"session_limit"`
	MaxPerTask     float64            `json:"max_per_task"`
	CurrentDaily   float64            `json:"current_daily"`
	CurrentSession float64            `json:"current_session"`
	DailyPercent   float64            `json:"daily_percent"`
	SessionPercent float64            `json:"session_percent"`
	ModelCosts     map[string]*ModelCosts `json:"model_costs"`
	TotalCalls     int                `json:"total_calls"`
}

// GetSummary returns a complete budget summary
func (b *BudgetTracker) GetSummary() *BudgetSummary {
	b.mu.RLock()
	defer b.mu.RUnlock()

	b.maybeResetDailyUnlocked()

	totalCalls := 0
	for _, mc := range b.modelCosts {
		totalCalls += mc.CallCount
	}

	var dailyPct, sessionPct float64
	if b.DailyLimit > 0 {
		dailyPct = b.CurrentDaily / b.DailyLimit * 100
	}
	if b.SessionLimit > 0 {
		sessionPct = b.CurrentSession / b.SessionLimit * 100
	}

	modelCostsCopy := make(map[string]*ModelCosts)
	for k, v := range b.modelCosts {
		copy := *v
		modelCostsCopy[k] = &copy
	}

	return &BudgetSummary{
		DailyLimit:     b.DailyLimit,
		SessionLimit:   b.SessionLimit,
		MaxPerTask:     b.MaxPerTask,
		CurrentDaily:   b.CurrentDaily,
		CurrentSession: b.CurrentSession,
		DailyPercent:   dailyPct,
		SessionPercent: sessionPct,
		ModelCosts:     modelCostsCopy,
		TotalCalls:     totalCalls,
	}
}

// budgetData is the serialized format for persistence
type budgetData struct {
	DailyLimit     float64               `json:"daily_limit"`
	SessionLimit   float64               `json:"session_limit"`
	MaxPerTask     float64               `json:"max_per_task"`
	CurrentDaily   float64               `json:"current_daily"`
	LastReset      time.Time             `json:"last_reset"`
	History        []CostRecord          `json:"history"`
	ModelCosts     map[string]*ModelCosts `json:"model_costs"`
}

// Save persists budget data to disk
func (b *BudgetTracker) Save() error {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.saveUnlocked()
}

func (b *BudgetTracker) saveUnlocked() error {
	if b.persistPath == "" {
		return nil
	}

	data := budgetData{
		DailyLimit:   b.DailyLimit,
		SessionLimit: b.SessionLimit,
		MaxPerTask:   b.MaxPerTask,
		CurrentDaily: b.CurrentDaily,
		LastReset:    b.LastReset,
		History:      b.history,
		ModelCosts:   b.modelCosts,
	}

	dir := filepath.Dir(b.persistPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	jsonData, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(b.persistPath, jsonData, 0644)
}

// Load reads budget data from disk
func (b *BudgetTracker) Load() error {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.persistPath == "" {
		return nil
	}

	jsonData, err := os.ReadFile(b.persistPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}

	var data budgetData
	if err := json.Unmarshal(jsonData, &data); err != nil {
		return err
	}

	// Restore limits
	if data.DailyLimit > 0 {
		b.DailyLimit = data.DailyLimit
	}
	if data.SessionLimit > 0 {
		b.SessionLimit = data.SessionLimit
	}
	if data.MaxPerTask > 0 {
		b.MaxPerTask = data.MaxPerTask
	}

	// Restore daily (if same day)
	now := time.Now()
	if data.LastReset.YearDay() == now.YearDay() && data.LastReset.Year() == now.Year() {
		b.CurrentDaily = data.CurrentDaily
	}
	b.LastReset = data.LastReset

	// Restore history and model costs
	b.history = data.History
	if b.history == nil {
		b.history = make([]CostRecord, 0)
	}
	b.modelCosts = data.ModelCosts
	if b.modelCosts == nil {
		b.modelCosts = make(map[string]*ModelCosts)
	}

	return nil
}
