package agent

import (
	"testing"
)

func TestNewTaskClassifier(t *testing.T) {
	tc := NewTaskClassifier()
	if tc == nil {
		t.Fatal("NewTaskClassifier returned nil")
	}
	if len(tc.keywords) == 0 {
		t.Error("keywords should be populated")
	}
}

func TestTaskClassifier_Classify_TaskTypes(t *testing.T) {
	tc := NewTaskClassifier()

	tests := []struct {
		prompt   string
		wantType TaskType
	}{
		{"find all files that import the database package", TaskTypeExplore},
		{"search for usages of the User struct", TaskTypeExplore},
		{"where is the authentication middleware", TaskTypeExplore},
		{"list all go files", TaskTypeExplore},

		{"explain how the router works", TaskTypeExplain},
		{"what does this function do", TaskTypeExplain},
		{"summarize the codebase structure", TaskTypeExplain},

		{"fix the bug in the login handler", TaskTypeBugfix},
		{"debug the failing test", TaskTypeBugfix},
		{"resolve the nil pointer error", TaskTypeBugfix},
		{"the server crashes on startup", TaskTypeBugfix},

		{"refactor the database layer", TaskTypeRefactor},
		{"clean up the code in utils.go", TaskTypeRefactor},
		{"simplify the authentication flow", TaskTypeRefactor},

		{"implement a new caching layer", TaskTypeFeature},
		{"add pagination to the API", TaskTypeFeature},
		{"create a new endpoint for users", TaskTypeFeature},

		{"write tests for the handler", TaskTypeTest},
		{"add unit tests for the service", TaskTypeTest},
	}

	for _, tt := range tests {
		result := tc.Classify(tt.prompt, nil)
		if result.TaskType != tt.wantType {
			t.Errorf("Classify(%q) = %s, want %s", tt.prompt, result.TaskType, tt.wantType)
		}
	}
}

func TestTaskClassifier_Classify_Environment(t *testing.T) {
	tc := NewTaskClassifier()

	tests := []struct {
		prompt  string
		wantEnv string
	}{
		{"fix the bug in main.go", "go"},
		{"run go test ./...", "go"},
		{"implement the handler in golang", "go"},

		{"fix the bug in app.py", "python"},
		{"run pytest", "python"},
		{"add a new python import", "python"},

		{"fix the component in App.tsx", "typescript"},
		{"run npm test", "typescript"}, // npm matches typescript pattern first

		{"fix main.rs", "rust"},
		{"run cargo build", "rust"},
	}

	for _, tt := range tests {
		result := tc.Classify(tt.prompt, nil)
		if result.Environment != tt.wantEnv {
			t.Errorf("Classify(%q).Environment = %s, want %s", tt.prompt, result.Environment, tt.wantEnv)
		}
	}
}

func TestTaskClassifier_Classify_Complexity(t *testing.T) {
	tc := NewTaskClassifier()

	// Simple task
	simple := tc.Classify("find the main.go file", nil)
	if simple.Complexity > 0.4 {
		t.Errorf("simple task complexity = %f, expected <= 0.4", simple.Complexity)
	}

	// Complex task
	complex := tc.Classify("refactor the entire authentication system across multiple files to use a new architecture", nil)
	if complex.Complexity < 0.6 {
		t.Errorf("complex task complexity = %f, expected >= 0.6", complex.Complexity)
	}
}

func TestTaskClassifier_Classify_RequiredCaps(t *testing.T) {
	tc := NewTaskClassifier()

	// Basic code task
	basic := tc.Classify("find the function", nil)
	if !containsString(basic.RequiredCaps, "code") {
		t.Error("basic task should require 'code' capability")
	}

	// Vision task
	vision := tc.Classify("analyze this screenshot and fix the UI issue", nil)
	if !containsString(vision.RequiredCaps, "vision") {
		t.Error("vision task should require 'vision' capability")
	}

	// Long context
	longCtx := tc.Classify("analyze the entire project structure and all files", nil)
	if !containsString(longCtx.RequiredCaps, "long_context") {
		t.Error("long context task should require 'long_context' capability")
	}
}

func TestTaskClassifier_Classify_HasImages(t *testing.T) {
	tc := NewTaskClassifier()

	noImages := tc.Classify("fix the bug in main.go", nil)
	if noImages.HasImages {
		t.Error("expected HasImages = false")
	}

	withImages := tc.Classify("look at this screenshot and fix the issue", nil)
	if !withImages.HasImages {
		t.Error("expected HasImages = true")
	}
}

func TestTaskClassifier_Classify_EstTokens(t *testing.T) {
	tc := NewTaskClassifier()

	// Short prompt
	short := tc.Classify("fix bug", nil)
	if short.EstTokens < 100 {
		t.Errorf("EstTokens = %d, expected >= 100", short.EstTokens)
	}

	// Long prompt
	long := tc.Classify(longPrompt(), nil)
	if long.EstTokens < 100 {
		t.Errorf("long prompt EstTokens = %d, expected >= 100", long.EstTokens)
	}
}

func TestTaskClassifier_Classify_WithContext(t *testing.T) {
	tc := NewTaskClassifier()

	// Without context
	noCtx := tc.Classify("fix the handler", nil)

	// With context showing errors
	ctx := &TaskContext{
		TurnCount: 10,
		Errors:    []string{"previous error"},
		FilesRead: []string{"handler.go", "service.go"},
	}
	withCtx := tc.Classify("fix the handler", ctx)

	// Context should increase complexity
	if withCtx.Complexity <= noCtx.Complexity {
		t.Errorf("context should increase complexity: %f <= %f", withCtx.Complexity, noCtx.Complexity)
	}

	// Context with .go files should detect go environment
	if withCtx.Environment != "go" {
		t.Errorf("Environment = %s, want go (from context files)", withCtx.Environment)
	}
}

func TestTaskClassifier_detectTaskType_Unknown(t *testing.T) {
	tc := NewTaskClassifier()

	// Gibberish prompt
	result := tc.Classify("asdfghjkl qwerty", nil)
	if result.TaskType != TaskTypeUnknown {
		t.Errorf("expected TaskTypeUnknown for gibberish, got %s", result.TaskType)
	}
}

func TestTaskClassifier_detectTaskType_Confidence(t *testing.T) {
	tc := NewTaskClassifier()

	// Clear task type should have high confidence
	clear := tc.Classify("fix the bug fix the error debug the crash", nil)
	if clear.Confidence < 0.7 {
		t.Errorf("clear bugfix task confidence = %f, expected >= 0.7", clear.Confidence)
	}

	// Ambiguous task
	ambiguous := tc.Classify("help", nil)
	if ambiguous.Confidence > 0.5 {
		t.Errorf("ambiguous task confidence = %f, expected <= 0.5", ambiguous.Confidence)
	}
}

func TestDefaultTaskClassifier(t *testing.T) {
	if DefaultTaskClassifier == nil {
		t.Fatal("DefaultTaskClassifier is nil")
	}

	// Should work
	result := DefaultTaskClassifier.Classify("find main.go", nil)
	if result.TaskType != TaskTypeExplore {
		t.Errorf("TaskType = %s, want explore", result.TaskType)
	}
}

func TestTaskClassification_Fields(t *testing.T) {
	tc := &TaskClassification{
		TaskType:     TaskTypeBugfix,
		Complexity:   0.6,
		Environment:  "go",
		RequiredCaps: []string{"code", "tool_use"},
		EstTokens:    1000,
		HasImages:    false,
		Confidence:   0.8,
	}

	if tc.TaskType != TaskTypeBugfix {
		t.Error("TaskType mismatch")
	}
	if tc.Complexity != 0.6 {
		t.Error("Complexity mismatch")
	}
	if tc.Environment != "go" {
		t.Error("Environment mismatch")
	}
}

func TestTaskType_Constants(t *testing.T) {
	types := []TaskType{
		TaskTypeExplore,
		TaskTypeExplain,
		TaskTypeBugfix,
		TaskTypeRefactor,
		TaskTypeFeature,
		TaskTypeTest,
		TaskTypeUnknown,
	}

	seen := make(map[TaskType]bool)
	for _, tt := range types {
		if seen[tt] {
			t.Errorf("duplicate task type: %s", tt)
		}
		seen[tt] = true
	}
}

func TestGetFileExtension(t *testing.T) {
	tests := []struct {
		path string
		want string
	}{
		{"main.go", ".go"},
		{"app.py", ".py"},
		{"component.tsx", ".tsx"},
		{"no-extension", ""},
		{"/path/to/file.rs", ".rs"},
	}

	for _, tt := range tests {
		got := getFileExtension(tt.path)
		if got != tt.want {
			t.Errorf("getFileExtension(%q) = %q, want %q", tt.path, got, tt.want)
		}
	}
}

// Helper functions

func containsString(slice []string, s string) bool {
	for _, item := range slice {
		if item == s {
			return true
		}
	}
	return false
}

func longPrompt() string {
	return `Please help me refactor the entire authentication system.
	This involves updating the user model, changing the password hashing algorithm,
	implementing JWT tokens, adding refresh token support, creating new middleware
	for route protection, updating all the API endpoints that use authentication,
	adding rate limiting, implementing account lockout after failed attempts,
	and writing comprehensive tests for all the new functionality.`
}
