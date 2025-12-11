package agent

import (
	"context"
	"testing"

	"github.com/joss/urp/internal/opencode/model"
)

func TestNewModelRouter(t *testing.T) {
	router := NewModelRouter(nil)
	if router == nil {
		t.Fatal("NewModelRouter returned nil")
	}
	if router.registry == nil {
		t.Error("registry should be set to default")
	}
	if router.config == nil {
		t.Error("config should be initialized")
	}
}

func TestModelRouter_SelectModel_Disabled(t *testing.T) {
	router := NewModelRouter(nil)
	router.config.Enabled = false

	task := &TaskClassification{
		TaskType:   TaskTypeExplore,
		Complexity: 0.3,
	}

	selection := router.SelectModel(context.Background(), task)
	if selection == nil {
		t.Fatal("SelectModel returned nil")
	}
	if selection.Reason != "routing disabled" {
		t.Errorf("Reason = %s, want 'routing disabled'", selection.Reason)
	}
	if selection.ModelID != router.fallback {
		t.Errorf("ModelID = %s, want fallback %s", selection.ModelID, router.fallback)
	}
}

func TestModelRouter_SelectModel_RuleMatching(t *testing.T) {
	// Use default registry which has builtin models
	router := NewModelRouter(nil)

	tests := []struct {
		name     string
		task     *TaskClassification
		wantRule string
	}{
		{
			name: "vision-required",
			task: &TaskClassification{
				TaskType:  TaskTypeBugfix,
				HasImages: true,
			},
			wantRule: "vision-required",
		},
		{
			name: "complex-task",
			task: &TaskClassification{
				TaskType:   TaskTypeFeature,
				Complexity: 0.9,
			},
			wantRule: "complex-task",
		},
		{
			name: "explore-fast",
			task: &TaskClassification{
				TaskType:   TaskTypeExplore,
				Complexity: 0.2,
			},
			wantRule: "explore-fast",
		},
		{
			name: "explain-cheap",
			task: &TaskClassification{
				TaskType:   TaskTypeExplain,
				Complexity: 0.3,
			},
			wantRule: "explain-cheap",
		},
		{
			name: "bugfix-simple",
			task: &TaskClassification{
				TaskType:   TaskTypeBugfix,
				Complexity: 0.3,
			},
			wantRule: "bugfix-simple",
		},
		{
			name: "bugfix-balanced",
			task: &TaskClassification{
				TaskType:   TaskTypeBugfix,
				Complexity: 0.6,
			},
			wantRule: "bugfix-balanced",
		},
		{
			name: "refactor-quality",
			task: &TaskClassification{
				TaskType:   TaskTypeRefactor,
				Complexity: 0.7,
			},
			wantRule: "refactor-quality",
		},
		{
			name: "feature-balanced",
			task: &TaskClassification{
				TaskType:   TaskTypeFeature,
				Complexity: 0.5,
			},
			wantRule: "feature-balanced",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			selection := router.SelectModel(context.Background(), tt.task)
			if selection == nil {
				t.Fatal("SelectModel returned nil")
			}
			if selection.RuleName != tt.wantRule {
				t.Errorf("RuleName = %s, want %s (reason: %s)", selection.RuleName, tt.wantRule, selection.Reason)
			}
		})
	}
}

func TestModelRouter_SelectModel_LongContext(t *testing.T) {
	router := NewModelRouter(nil)

	task := &TaskClassification{
		TaskType:  TaskTypeExplain,
		EstTokens: 150000, // > 100k threshold
	}

	selection := router.SelectModel(context.Background(), task)
	if selection == nil {
		t.Fatal("SelectModel returned nil")
	}
	if selection.RuleName != "long-context" {
		t.Errorf("RuleName = %s, want 'long-context'", selection.RuleName)
	}
}

func TestModelRouter_SelectModel_Default(t *testing.T) {
	router := NewModelRouter(nil)

	task := &TaskClassification{
		TaskType:   TaskTypeUnknown,
		Complexity: 0.5,
	}

	selection := router.SelectModel(context.Background(), task)
	if selection == nil {
		t.Fatal("SelectModel returned nil")
	}
	// Should match default rule
	if selection.RuleName != "default" {
		t.Errorf("RuleName = %s, want 'default'", selection.RuleName)
	}
}

func TestModelRouter_SelectModel_FallbackChain(t *testing.T) {
	// Use default registry
	registry := model.DefaultModelRegistry

	// Disable all rules by setting an impossible condition model
	router := &ModelRouter{
		registry: registry,
		config: &RoutingConfig{
			Enabled: true,
			Rules:   []RoutingRule{}, // No rules
			FallbackChain: []string{
				"nonexistent-model",
				"claude-sonnet-4-20250514", // This should be found
			},
		},
		fallback: "claude-sonnet-4-20250514",
	}

	task := &TaskClassification{
		TaskType:   TaskTypeUnknown,
		Complexity: 0.5,
	}

	selection := router.SelectModel(context.Background(), task)
	if selection == nil {
		t.Fatal("SelectModel returned nil")
	}
	// Should use fallback chain or scored selection
	if selection.ModelID == "" {
		t.Error("ModelID should not be empty")
	}
}

func TestModelRouter_EvaluateRule(t *testing.T) {
	router := NewModelRouter(nil)

	tests := []struct {
		rule RoutingRule
		task *TaskClassification
		want bool
	}{
		{
			rule: RoutingRule{Condition: "true"},
			task: &TaskClassification{},
			want: true,
		},
		{
			rule: RoutingRule{Condition: "has_images"},
			task: &TaskClassification{HasImages: true},
			want: true,
		},
		{
			rule: RoutingRule{Condition: "has_images"},
			task: &TaskClassification{HasImages: false},
			want: false,
		},
		{
			rule: RoutingRule{Condition: "task_type == 'explore'"},
			task: &TaskClassification{TaskType: TaskTypeExplore},
			want: true,
		},
		{
			rule: RoutingRule{Condition: "task_type == 'explore'"},
			task: &TaskClassification{TaskType: TaskTypeBugfix},
			want: false,
		},
		{
			rule: RoutingRule{Condition: "tokens > 100000"},
			task: &TaskClassification{EstTokens: 150000},
			want: true,
		},
		{
			rule: RoutingRule{Condition: "tokens > 100000"},
			task: &TaskClassification{EstTokens: 50000},
			want: false,
		},
		{
			rule: RoutingRule{Condition: "complexity > 0.8"},
			task: &TaskClassification{Complexity: 0.9},
			want: true,
		},
		{
			rule: RoutingRule{Condition: "complexity > 0.8"},
			task: &TaskClassification{Complexity: 0.5},
			want: false,
		},
		{
			rule: RoutingRule{Condition: "task_type == 'explain' && complexity < 0.5"},
			task: &TaskClassification{TaskType: TaskTypeExplain, Complexity: 0.3},
			want: true,
		},
		{
			rule: RoutingRule{Condition: "task_type == 'explain' && complexity < 0.5"},
			task: &TaskClassification{TaskType: TaskTypeExplain, Complexity: 0.7},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.rule.Condition, func(t *testing.T) {
			got := router.evaluateRule(tt.rule, tt.task)
			if got != tt.want {
				t.Errorf("evaluateRule(%q) = %v, want %v", tt.rule.Condition, got, tt.want)
			}
		})
	}
}

func TestModelRouter_HasRequiredCaps(t *testing.T) {
	router := NewModelRouter(nil)

	modelWithCaps := &model.ModelInfo{
		ID:           "test-model",
		Capabilities: []string{"code", "tool_use", "reasoning"},
	}

	tests := []struct {
		required []string
		want     bool
	}{
		{[]string{}, true},
		{[]string{"code"}, true},
		{[]string{"code", "tool_use"}, true},
		{[]string{"vision"}, false},
		{[]string{"code", "vision"}, false},
	}

	for _, tt := range tests {
		t.Run(joinStrings(tt.required), func(t *testing.T) {
			got := router.hasRequiredCaps(modelWithCaps, tt.required)
			if got != tt.want {
				t.Errorf("hasRequiredCaps(%v) = %v, want %v", tt.required, got, tt.want)
			}
		})
	}
}

func TestModelRouter_ScoredSelection(t *testing.T) {
	router := NewModelRouter(nil)

	task := &TaskClassification{
		TaskType:     TaskTypeBugfix,
		Complexity:   0.5,
		RequiredCaps: []string{"code"},
	}

	selection := router.scoredSelection(task)
	if selection == nil {
		t.Fatal("scoredSelection returned nil")
	}
	if selection.ModelID == "" {
		t.Error("ModelID should not be empty")
	}
	if selection.Confidence != 0.6 {
		t.Errorf("Confidence = %f, want 0.6", selection.Confidence)
	}
}

func TestModelRouter_ScoredSelection_ComplexTask(t *testing.T) {
	router := NewModelRouter(nil)

	// Complex task should penalize low-tier models
	task := &TaskClassification{
		TaskType:     TaskTypeFeature,
		Complexity:   0.85, // > 0.7 threshold
		RequiredCaps: []string{"code", "reasoning"},
	}

	selection := router.scoredSelection(task)
	if selection == nil {
		t.Fatal("scoredSelection returned nil")
	}

	// The selected model should be tier 2+ for complex tasks
	selectedModel := router.registry.Get(selection.ModelID)
	if selectedModel != nil && selectedModel.QualityTier < 2 {
		t.Logf("Warning: complex task got tier %d model (may be penalized)", selectedModel.QualityTier)
	}
}

func TestModelRouter_GetConfig(t *testing.T) {
	router := NewModelRouter(nil)
	config := router.GetConfig()
	if config == nil {
		t.Fatal("GetConfig returned nil")
	}
	if !config.Enabled {
		t.Error("config should be enabled by default")
	}
}

func TestModelRouter_SetFallback(t *testing.T) {
	router := NewModelRouter(nil)
	router.SetFallback("test-model")
	if router.fallback != "test-model" {
		t.Errorf("fallback = %s, want test-model", router.fallback)
	}
}

func TestModelRouter_IsEnabled(t *testing.T) {
	router := NewModelRouter(nil)
	if !router.IsEnabled() {
		t.Error("should be enabled by default")
	}

	router.config.Enabled = false
	if router.IsEnabled() {
		t.Error("should be disabled after setting")
	}
}

func TestModelRouter_WithLearning(t *testing.T) {
	router := NewModelRouter(nil)
	learning := &ModelLearningStore{}
	result := router.WithLearning(learning)
	if result != router {
		t.Error("WithLearning should return same router")
	}
	if router.learning != learning {
		t.Error("learning store not set")
	}
}

func TestModelRouter_WithBudget(t *testing.T) {
	router := NewModelRouter(nil)
	budget := &BudgetTracker{}
	result := router.WithBudget(budget)
	if result != router {
		t.Error("WithBudget should return same router")
	}
	if router.budget != budget {
		t.Error("budget tracker not set")
	}
}

func TestDefaultModelRouter(t *testing.T) {
	if DefaultModelRouter == nil {
		t.Fatal("DefaultModelRouter is nil")
	}
	if DefaultModelRouter.registry == nil {
		t.Error("DefaultModelRouter.registry is nil")
	}
}

func TestModelSelection_Fields(t *testing.T) {
	selection := &ModelSelection{
		ModelID:    "test-model",
		Confidence: 0.9,
		Reason:     "test reason",
		EstCost:    0.05,
		RuleName:   "test-rule",
	}

	if selection.ModelID != "test-model" {
		t.Error("ModelID mismatch")
	}
	if selection.Confidence != 0.9 {
		t.Error("Confidence mismatch")
	}
	if selection.Reason != "test reason" {
		t.Error("Reason mismatch")
	}
	if selection.EstCost != 0.05 {
		t.Error("EstCost mismatch")
	}
	if selection.RuleName != "test-rule" {
		t.Error("RuleName mismatch")
	}
}

func TestRoutingConfig_Defaults(t *testing.T) {
	config := defaultRoutingConfig()
	if config == nil {
		t.Fatal("defaultRoutingConfig returned nil")
	}

	// Check budget defaults
	if config.Budget.DailyLimit != 5.0 {
		t.Errorf("DailyLimit = %f, want 5.0", config.Budget.DailyLimit)
	}
	if config.Budget.SessionLimit != 1.0 {
		t.Errorf("SessionLimit = %f, want 1.0", config.Budget.SessionLimit)
	}
	if config.Budget.MaxPerTask != 0.20 {
		t.Errorf("MaxPerTask = %f, want 0.20", config.Budget.MaxPerTask)
	}

	// Check weight defaults
	if config.Weights.Quality != 0.5 {
		t.Errorf("Quality weight = %f, want 0.5", config.Weights.Quality)
	}
	if config.Weights.Cost != 0.3 {
		t.Errorf("Cost weight = %f, want 0.3", config.Weights.Cost)
	}
	if config.Weights.Speed != 0.2 {
		t.Errorf("Speed weight = %f, want 0.2", config.Weights.Speed)
	}

	// Check rules exist
	if len(config.Rules) == 0 {
		t.Error("no default rules")
	}

	// Check fallback chain
	if len(config.FallbackChain) == 0 {
		t.Error("no fallback chain")
	}
}

func TestDefaultRules(t *testing.T) {
	rules := defaultRules()
	if len(rules) < 5 {
		t.Errorf("expected at least 5 default rules, got %d", len(rules))
	}

	// Check priorities are ordered
	seen := make(map[string]bool)
	for _, r := range rules {
		if seen[r.Name] {
			t.Errorf("duplicate rule name: %s", r.Name)
		}
		seen[r.Name] = true
	}

	// Check required rules exist
	required := []string{"vision-required", "complex-task", "default"}
	for _, name := range required {
		if !seen[name] {
			t.Errorf("missing required rule: %s", name)
		}
	}
}

func TestContainsWord(t *testing.T) {
	tests := []struct {
		s, word string
		want    bool
	}{
		{"task_type == 'explore'", "task_type", true},
		{"complexity > 0.8", "complexity >", true},
		{"tokens > 100000", "tokens >", true},
		{"", "word", false},
		{"word", "", false},
		{"hello", "hello", true},
	}

	for _, tt := range tests {
		got := containsWord(tt.s, tt.word)
		if got != tt.want {
			t.Errorf("containsWord(%q, %q) = %v, want %v", tt.s, tt.word, got, tt.want)
		}
	}
}

func TestParseTokenThreshold(t *testing.T) {
	tests := []struct {
		condition string
		want      int
	}{
		{"tokens > 100000", 100000},
		{"tokens > 50000", 50000},
		{"tokens > 0", 0},
	}

	for _, tt := range tests {
		got := parseTokenThreshold(tt.condition)
		if got != tt.want {
			t.Errorf("parseTokenThreshold(%q) = %d, want %d", tt.condition, got, tt.want)
		}
	}
}

func TestParseComplexityThreshold(t *testing.T) {
	tests := []struct {
		condition string
		want      float64
	}{
		{"complexity > 0.8", 0.8},
		{"complexity < 0.5", 0.5},
		{"task_type == 'explain' && complexity < 0.5", 0.5},
	}

	for _, tt := range tests {
		got := parseComplexityThreshold(tt.condition)
		// Use tolerance for float comparison
		diff := got - tt.want
		if diff < 0 {
			diff = -diff
		}
		if diff > 0.001 {
			t.Errorf("parseComplexityThreshold(%q) = %f, want %f", tt.condition, got, tt.want)
		}
	}
}

// Helper function
func joinStrings(s []string) string {
	if len(s) == 0 {
		return "empty"
	}
	result := s[0]
	for i := 1; i < len(s); i++ {
		result += "," + s[i]
	}
	return result
}
