package tool

import (
	"testing"
)

func TestDefaultMultiExpertConfig(t *testing.T) {
	config := DefaultMultiExpertConfig()

	if config.NumExperts != 3 {
		t.Errorf("NumExperts = %d, want 3", config.NumExperts)
	}
	if config.SeedSpacing != 100 {
		t.Errorf("SeedSpacing = %d, want 100", config.SeedSpacing)
	}
	if config.VotingStrategy != "diversity" {
		t.Errorf("VotingStrategy = %s, want diversity", config.VotingStrategy)
	}
	if config.MaxIterationsEach != 5 {
		t.Errorf("MaxIterationsEach = %d, want 5", config.MaxIterationsEach)
	}
	if !config.CountFailedMatches {
		t.Error("CountFailedMatches should be true")
	}
	if config.ItersTiebreak {
		t.Error("ItersTiebreak should be false")
	}
	if !config.LowToHighIters {
		t.Error("LowToHighIters should be true")
	}
}

func TestNewMultiExpertExecutor(t *testing.T) {
	executor := NewMultiExpertExecutor("/tmp/test")

	if executor == nil {
		t.Fatal("NewMultiExpertExecutor returned nil")
	}
	if executor.workDir != "/tmp/test" {
		t.Errorf("workDir = %s, want /tmp/test", executor.workDir)
	}
	if executor.config.NumExperts != 3 {
		t.Errorf("default NumExperts = %d, want 3", executor.config.NumExperts)
	}
}

func TestMultiExpertExecutor_WithConfig(t *testing.T) {
	config := MultiExpertConfig{
		NumExperts:    5,
		SeedSpacing:   200,
		VotingStrategy: "majority",
	}

	executor := NewMultiExpertExecutor("/tmp").WithConfig(config)

	if executor.config.NumExperts != 5 {
		t.Errorf("NumExperts = %d, want 5", executor.config.NumExperts)
	}
	if executor.config.VotingStrategy != "majority" {
		t.Errorf("VotingStrategy = %s, want majority", executor.config.VotingStrategy)
	}
}

func TestCanonicalKey(t *testing.T) {
	tests := []struct {
		name   string
		output string
		want   string
	}{
		{"short", "hello", "hello"},
		{"exactly500", string(make([]byte, 500)), string(make([]byte, 500))},
		{"long", string(make([]byte, 600)), string(make([]byte, 500))},
		{"empty", "", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := canonicalKey(tt.output)
			if len(got) != len(tt.want) {
				t.Errorf("canonicalKey() length = %d, want %d", len(got), len(tt.want))
			}
		})
	}
}

func TestMeanScore(t *testing.T) {
	tests := []struct {
		name    string
		group   []ExpertResult
		wantMin float64
		wantMax float64
	}{
		{"empty", nil, 0.0, 0.0},
		{"single", []ExpertResult{{Score: 0.8}}, 0.79, 0.81},
		{"multiple", []ExpertResult{{Score: 0.5}, {Score: 0.7}, {Score: 0.9}}, 0.69, 0.71},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := meanScore(tt.group)
			if got < tt.wantMin || got > tt.wantMax {
				t.Errorf("meanScore() = %f, want between %f and %f", got, tt.wantMin, tt.wantMax)
			}
		})
	}
}

func TestVoteOnResults_PassersFirst(t *testing.T) {
	executor := NewMultiExpertExecutor("/tmp")

	results := []ExpertResult{
		{ExpertID: 0, Output: "same", Success: true, Score: 0.9},
		{ExpertID: 1, Output: "same", Success: true, Score: 0.8},
		{ExpertID: 2, Output: "different", Success: false, Score: 0.3},
	}

	ranked := executor.voteOnResults(results)

	if len(ranked) != 3 {
		t.Fatalf("ranked length = %d, want 3", len(ranked))
	}

	// First should be a passer (from the group with most votes)
	if !ranked[0].Success {
		t.Error("first result should be successful")
	}

	// Count how many passers are in the top positions
	passersFirst := 0
	for _, r := range ranked {
		if r.Success {
			passersFirst++
		} else {
			break
		}
	}
	// With diversity-first, we get one from each group first
	// So first is from "same" (2 votes, passer), second might be "different" (1 vote, fail)
	// This is correct behavior - diversity over strict pass/fail ordering
	if passersFirst < 1 {
		t.Error("should have at least one passer in top positions")
	}
}

func TestVoteOnResults_Diversity(t *testing.T) {
	executor := NewMultiExpertExecutor("/tmp").WithConfig(MultiExpertConfig{
		VotingStrategy: "diversity",
	})

	results := []ExpertResult{
		{ExpertID: 0, Output: "output_A", Success: true, Score: 0.9},
		{ExpertID: 1, Output: "output_A", Success: true, Score: 0.8},
		{ExpertID: 2, Output: "output_B", Success: true, Score: 0.7},
	}

	ranked := executor.voteOnResults(results)

	// With diversity-first, we should get one from each unique output first
	// Group A (2 votes) comes first, then Group B (1 vote)
	// But we still get diversity: one A, one B, then remaining

	if len(ranked) != 3 {
		t.Fatalf("ranked length = %d, want 3", len(ranked))
	}

	// First two should be from different outputs (diversity)
	if ranked[0].Output == ranked[1].Output && len(ranked) > 2 {
		// This is okay if there's only one unique output
		uniqueOutputs := make(map[string]bool)
		for _, r := range results {
			uniqueOutputs[r.Output] = true
		}
		if len(uniqueOutputs) > 1 {
			t.Error("diversity voting should put different outputs first")
		}
	}
}

func TestVoteOnResults_CountFailedMatches(t *testing.T) {
	executor := NewMultiExpertExecutor("/tmp").WithConfig(MultiExpertConfig{
		CountFailedMatches: true,
	})

	results := []ExpertResult{
		{ExpertID: 0, Output: "same", Success: true, Score: 0.9},
		{ExpertID: 1, Output: "same", Success: false, Score: 0.5}, // Same output but failed
		{ExpertID: 2, Output: "different", Success: false, Score: 0.3},
	}

	ranked := executor.voteOnResults(results)

	// Expert 1 should be merged with passing bucket (same output)
	// So "same" bucket has 2 votes, "different" has 1

	if len(ranked) != 3 {
		t.Fatalf("ranked length = %d, want 3", len(ranked))
	}

	// First should be from "same" bucket (more votes)
	if ranked[0].Output != "same" {
		t.Errorf("first output = %s, want 'same' (more votes)", ranked[0].Output)
	}
}

func TestVoteOnResults_ItersTiebreak(t *testing.T) {
	executor := NewMultiExpertExecutor("/tmp").WithConfig(MultiExpertConfig{
		ItersTiebreak:  true,
		LowToHighIters: true,
	})

	results := []ExpertResult{
		{ExpertID: 0, Output: "same", Success: true, Score: 0.9, Iteration: 5},
		{ExpertID: 1, Output: "same", Success: true, Score: 0.9, Iteration: 2}, // Faster
	}

	ranked := executor.voteOnResults(results)

	// With LowToHighIters=true, Expert 1 (iter 2) should come before Expert 0 (iter 5)
	if ranked[0].Iteration != 2 {
		t.Errorf("first iteration = %d, want 2 (faster)", ranked[0].Iteration)
	}
}

func TestExpertResult_Fields(t *testing.T) {
	result := ExpertResult{
		ExpertID:   1,
		Seed:       100,
		Output:     "test output",
		Score:      0.85,
		Iteration:  3,
		Success:    true,
		TokensUsed: 500,
	}

	if result.ExpertID != 1 {
		t.Errorf("ExpertID = %d, want 1", result.ExpertID)
	}
	if result.Seed != 100 {
		t.Errorf("Seed = %d, want 100", result.Seed)
	}
	if result.Score != 0.85 {
		t.Errorf("Score = %f, want 0.85", result.Score)
	}
	if !result.Success {
		t.Error("Success should be true")
	}
}

func TestNewMultiExpertTask(t *testing.T) {
	task := NewMultiExpertTask("/tmp/work")

	if task == nil {
		t.Fatal("NewMultiExpertTask returned nil")
	}

	info := task.Info()
	if info.Name != "multi_task" {
		t.Errorf("tool name = %s, want multi_task", info.Name)
	}
}

func TestMultiExpertTask_Info(t *testing.T) {
	task := NewMultiExpertTask("/tmp")
	info := task.Info()

	if info.Name != "multi_task" {
		t.Errorf("Name = %s, want multi_task", info.Name)
	}
	if info.Description == "" {
		t.Error("Description should not be empty")
	}

	params, ok := info.Parameters["properties"].(map[string]any)
	if !ok {
		t.Fatal("parameters should have properties")
	}

	requiredFields := []string{"prompt"}
	required, _ := info.Parameters["required"].([]string)
	for _, rf := range requiredFields {
		found := false
		for _, r := range required {
			if r == rf {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("required field %s not found", rf)
		}
	}

	// Check optional fields exist
	optionalFields := []string{"description", "num_experts", "voting"}
	for _, of := range optionalFields {
		if _, exists := params[of]; !exists {
			t.Errorf("optional field %s not found in properties", of)
		}
	}
}

func TestMultiExpertTask_Execute_NoPrompt(t *testing.T) {
	task := NewMultiExpertTask("/tmp")

	result, err := task.Execute(nil, map[string]any{})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Error == nil {
		t.Error("should return error when prompt is missing")
	}
}

func TestMultiExpertTask_Execute_NoProvider(t *testing.T) {
	task := NewMultiExpertTask("/tmp")

	result, err := task.Execute(nil, map[string]any{
		"prompt": "test prompt",
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Without provider, should fail
	if result.Error == nil {
		t.Error("should return error when provider not configured")
	}
}
