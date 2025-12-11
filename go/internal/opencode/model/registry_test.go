package model

import (
	"os"
	"path/filepath"
	"testing"
)

func TestNewModelRegistry(t *testing.T) {
	registry := NewModelRegistry()
	if registry == nil {
		t.Fatal("NewModelRegistry returned nil")
	}
}

func TestModelRegistry_LoadBuiltinDefaults(t *testing.T) {
	registry := NewModelRegistry()
	registry.loadBuiltinDefaults()

	// Should have multiple models
	if registry.Count() < 10 {
		t.Errorf("expected at least 10 builtin models, got %d", registry.Count())
	}

	// Check specific models exist
	models := []string{"claude-sonnet-4-20250514", "deepseek-chat", "gpt-4o", "gemini-1.5-flash"}
	for _, id := range models {
		if model := registry.Get(id); model == nil {
			t.Errorf("expected model %s to exist", id)
		}
	}
}

func TestModelRegistry_Get_ByID(t *testing.T) {
	registry := NewModelRegistry()
	registry.loadBuiltinDefaults()

	model := registry.Get("claude-sonnet-4-20250514")
	if model == nil {
		t.Fatal("expected to find claude-sonnet-4-20250514")
	}
	if model.QualityTier != 2 {
		t.Errorf("QualityTier = %d, want 2", model.QualityTier)
	}
}

func TestModelRegistry_Get_ByAlias(t *testing.T) {
	registry := NewModelRegistry()
	registry.loadBuiltinDefaults()

	tests := []struct {
		alias    string
		wantID   string
	}{
		{"sonnet", "claude-sonnet-4-20250514"},
		{"deepseek", "deepseek-chat"},
		{"haiku", "claude-3-5-haiku-20241022"},
		{"flash", "gemini-1.5-flash"},
		{"opus", "claude-opus-4-20250514"},
	}

	for _, tt := range tests {
		model := registry.Get(tt.alias)
		if model == nil {
			t.Errorf("alias %s: expected to find model", tt.alias)
			continue
		}
		if model.ID != tt.wantID {
			t.Errorf("alias %s: got ID %s, want %s", tt.alias, model.ID, tt.wantID)
		}
	}
}

func TestModelRegistry_Get_NotFound(t *testing.T) {
	registry := NewModelRegistry()
	registry.loadBuiltinDefaults()

	model := registry.Get("nonexistent-model")
	if model != nil {
		t.Error("expected nil for nonexistent model")
	}
}

func TestModelRegistry_ListByTier(t *testing.T) {
	registry := NewModelRegistry()
	registry.loadBuiltinDefaults()

	// Tier 3 (premium)
	tier3 := registry.ListByTier(3)
	if len(tier3) < 1 {
		t.Error("expected at least 1 tier 3 model")
	}
	for _, m := range tier3 {
		if m.QualityTier != 3 {
			t.Errorf("tier 3 list contains model with tier %d", m.QualityTier)
		}
	}

	// Tier 1 (basic/fast)
	tier1 := registry.ListByTier(1)
	if len(tier1) < 3 {
		t.Errorf("expected at least 3 tier 1 models, got %d", len(tier1))
	}
}

func TestModelRegistry_ListEnabled(t *testing.T) {
	registry := NewModelRegistry()
	registry.loadBuiltinDefaults()

	enabled := registry.ListEnabled()
	if len(enabled) == 0 {
		t.Error("expected enabled models")
	}

	for _, m := range enabled {
		if !m.Enabled {
			t.Errorf("ListEnabled returned disabled model: %s", m.ID)
		}
	}
}

func TestModelRegistry_CheapestWithCap(t *testing.T) {
	registry := NewModelRegistry()
	registry.loadBuiltinDefaults()

	// Cheapest with tool_use capability
	cheapest := registry.CheapestWithCap("tool_use", 1)
	if cheapest == nil {
		t.Fatal("expected to find model with tool_use")
	}
	if !cheapest.HasCapability("tool_use") {
		t.Error("returned model doesn't have tool_use capability")
	}

	// Cheapest with vision
	vision := registry.CheapestWithCap("vision", 1)
	if vision == nil {
		t.Fatal("expected to find model with vision")
	}
	if !vision.HasCapability("vision") {
		t.Error("returned model doesn't have vision capability")
	}

	// With minimum tier 2
	tier2Vision := registry.CheapestWithCap("vision", 2)
	if tier2Vision == nil {
		t.Fatal("expected to find tier 2+ model with vision")
	}
	if tier2Vision.QualityTier < 2 {
		t.Errorf("expected tier >= 2, got %d", tier2Vision.QualityTier)
	}
}

func TestModelRegistry_CheapestForContext(t *testing.T) {
	registry := NewModelRegistry()
	registry.loadBuiltinDefaults()

	// Small context - should get cheap model
	small := registry.CheapestForContext(10000, 1)
	if small == nil {
		t.Fatal("expected to find model for 10k context")
	}

	// Large context - needs gemini
	large := registry.CheapestForContext(500000, 1)
	if large == nil {
		t.Fatal("expected to find model for 500k context")
	}
	if large.ContextSize < 500000 {
		t.Errorf("model context %d < required 500000", large.ContextSize)
	}
}

func TestModelInfo_HasCapability(t *testing.T) {
	model := &ModelInfo{
		ID:           "test",
		Capabilities: []string{"tool_use", "code", "reasoning"},
	}

	if !model.HasCapability("code") {
		t.Error("expected to have 'code' capability")
	}
	if model.HasCapability("vision") {
		t.Error("should not have 'vision' capability")
	}
}

func TestModelInfo_EstimateCost(t *testing.T) {
	model := &ModelInfo{
		ID:         "test",
		InputCost:  3.0,  // $3/1M
		OutputCost: 15.0, // $15/1M
	}

	// 1000 input + 500 output tokens
	cost := model.EstimateCost(1000, 500)
	expectedCost := 1000*3.0/1_000_000 + 500*15.0/1_000_000

	// Use tolerance for floating point comparison
	diff := cost - expectedCost
	if diff < 0 {
		diff = -diff
	}
	if diff > 0.000001 {
		t.Errorf("cost = %f, want %f", cost, expectedCost)
	}
}

func TestModelRegistry_LoadFromFile(t *testing.T) {
	// Create temp config file
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "models.yaml")

	config := `models:
  - id: test-model-1
    alias: [test1, tm1]
    quality_tier: 2
    speed_tier: 2
    input_cost: 1.0
    output_cost: 2.0
    context_size: 50000
    capabilities: [code, reasoning]
    enabled: true
  - id: test-model-2
    alias: [test2]
    quality_tier: 1
    speed_tier: 3
    input_cost: 0.5
    output_cost: 1.0
    context_size: 30000
    capabilities: [code]
    enabled: true
`
	if err := os.WriteFile(configPath, []byte(config), 0644); err != nil {
		t.Fatalf("failed to write config: %v", err)
	}

	registry := NewModelRegistry()
	if err := registry.LoadFromFile(configPath); err != nil {
		t.Fatalf("LoadFromFile failed: %v", err)
	}

	// Check models loaded
	if registry.Count() != 2 {
		t.Errorf("expected 2 models, got %d", registry.Count())
	}

	// Check by ID
	m1 := registry.Get("test-model-1")
	if m1 == nil {
		t.Fatal("expected test-model-1")
	}
	if m1.InputCost != 1.0 {
		t.Errorf("InputCost = %f, want 1.0", m1.InputCost)
	}

	// Check by alias
	m1Alias := registry.Get("test1")
	if m1Alias == nil || m1Alias.ID != "test-model-1" {
		t.Error("alias lookup failed")
	}
}

func TestModelRegistry_LoadFromFile_Error(t *testing.T) {
	registry := NewModelRegistry()
	err := registry.LoadFromFile("/nonexistent/path/models.yaml")
	if err == nil {
		t.Error("expected error for nonexistent file")
	}
}

func TestModelRegistry_Register(t *testing.T) {
	registry := NewModelRegistry()

	model := &ModelInfo{
		ID:           "custom-model",
		Alias:        []string{"custom"},
		QualityTier:  2,
		InputCost:    1.0,
		OutputCost:   2.0,
		Capabilities: []string{"code"},
		Enabled:      true,
	}

	registry.Register(model)

	// Should be retrievable
	retrieved := registry.Get("custom-model")
	if retrieved == nil {
		t.Fatal("expected to find registered model")
	}

	// Should be retrievable by alias
	byAlias := registry.Get("custom")
	if byAlias == nil {
		t.Fatal("expected to find by alias")
	}
}

func TestDefaultModelRegistry(t *testing.T) {
	// DefaultModelRegistry should be initialized
	if DefaultModelRegistry == nil {
		t.Fatal("DefaultModelRegistry is nil")
	}

	// Should have models
	if DefaultModelRegistry.Count() == 0 {
		t.Error("DefaultModelRegistry has no models")
	}

	// Should be able to get common models
	if DefaultModelRegistry.Get("sonnet") == nil {
		t.Error("DefaultModelRegistry missing sonnet alias")
	}
}

func TestModelRegistry_Tiers(t *testing.T) {
	registry := NewModelRegistry()
	registry.loadBuiltinDefaults()

	// Verify tier distribution
	tier1 := registry.ListByTier(1)
	tier2 := registry.ListByTier(2)
	tier3 := registry.ListByTier(3)

	if len(tier1) == 0 {
		t.Error("no tier 1 models")
	}
	if len(tier2) == 0 {
		t.Error("no tier 2 models")
	}
	if len(tier3) == 0 {
		t.Error("no tier 3 models")
	}

	// Tier 1 should be cheaper than tier 3
	cheapTier1 := tier1[0]
	expensiveTier3 := tier3[0]

	if cheapTier1.InputCost >= expensiveTier3.InputCost {
		// Find actual cheapest in tier 1
		for _, m := range tier1 {
			if m.InputCost < cheapTier1.InputCost {
				cheapTier1 = m
			}
		}
	}

	t.Logf("Tier 1 cheapest: %s ($%.2f), Tier 3: %s ($%.2f)",
		cheapTier1.ID, cheapTier1.InputCost,
		expensiveTier3.ID, expensiveTier3.InputCost)
}
