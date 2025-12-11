// Package agent provides task classification for intelligent model routing
package agent

import (
	"regexp"
	"strings"
)

// TaskType represents the type of task being performed
type TaskType string

const (
	TaskTypeExplore  TaskType = "explore"  // Search, find, list, understand
	TaskTypeExplain  TaskType = "explain"  // Explain, describe, summarize
	TaskTypeBugfix   TaskType = "bugfix"   // Fix, debug, repair, resolve
	TaskTypeRefactor TaskType = "refactor" // Refactor, restructure, clean up
	TaskTypeFeature  TaskType = "feature"  // Implement, add, create, build
	TaskTypeTest     TaskType = "test"     // Test, verify, validate
	TaskTypeReview   TaskType = "review"   // Review, validate code (TEACHER)
	TaskTypeOptimize TaskType = "optimize" // Optimize, improve perf (TEACHER)
	TaskTypeAudit    TaskType = "audit"    // Security audit (TEACHER)
	TaskTypeUnknown  TaskType = "unknown"  // Default fallback
)

// TaskClassification holds the analysis of a task
type TaskClassification struct {
	TaskType     TaskType // Primary task type
	Complexity   float64  // 0-1 (simple→complex)
	Environment  string   // go, python, typescript, rust, etc.
	RequiredCaps []string // Required capabilities: tool_use, code, reasoning, vision, long_context
	EstTokens    int      // Estimated input tokens
	HasImages    bool     // Task includes images
	Confidence   float64  // Confidence in classification
}

// TaskClassifier analyzes prompts to determine task characteristics
type TaskClassifier struct {
	keywords map[TaskType][]string
}

// NewTaskClassifier creates a new classifier with default patterns
func NewTaskClassifier() *TaskClassifier {
	tc := &TaskClassifier{
		keywords: map[TaskType][]string{
			TaskTypeExplore: {
				"find", "search", "locate", "where", "list", "show",
				"what files", "what functions", "how does", "understand",
				"explore", "look for", "check", "see", "scan",
			},
			TaskTypeExplain: {
				"explain", "describe", "summarize", "what is", "what does",
				"how does", "why", "tell me about", "documentation",
			},
			TaskTypeBugfix: {
				"fix", "bug", "error", "issue", "broken", "doesn't work",
				"fails", "crash", "debug", "repair", "resolve", "problem",
				"not working", "wrong", "incorrect",
			},
			TaskTypeRefactor: {
				"refactor", "clean up", "restructure", "reorganize",
				"improve", "optimize", "simplify", "rewrite", "modernize",
			},
			TaskTypeFeature: {
				"implement", "add", "create", "build", "new feature",
				"develop", "make", "write", "design", "integrate",
			},
			TaskTypeTest: {
				"test", "tests", "testing", "unit test", "coverage", "spec", "assertion",
			},
			// TEACHER tasks (trigger expensive model for validation)
			TaskTypeReview: {
				"review", "validate", "check my code", "is this correct",
				"look over", "code review", "pr review", "verify my",
			},
			TaskTypeOptimize: {
				"optimize", "performance", "make faster", "improve speed",
				"reduce memory", "efficiency", "bottleneck",
			},
			TaskTypeAudit: {
				"security audit", "audit", "vulnerabilities", "security review",
				"check for security", "secure", "owasp", "injection",
			},
		},
		// Note: Removed envPatterns map - patterns stored separately below
	}
	return tc
}

// Classify analyzes a prompt and returns task classification
func (tc *TaskClassifier) Classify(prompt string, context *TaskContext) *TaskClassification {
	prompt = strings.ToLower(prompt)

	result := &TaskClassification{
		TaskType:    TaskTypeUnknown,
		Complexity:  0.5,
		Confidence:  0.5,
		RequiredCaps: []string{"code"},
	}

	// Detect task type
	result.TaskType, result.Confidence = tc.detectTaskType(prompt)

	// Detect environment
	result.Environment = tc.detectEnvironment(prompt, context)

	// Calculate complexity
	result.Complexity = tc.calculateComplexity(prompt, result.TaskType, context)

	// Determine required capabilities
	result.RequiredCaps = tc.detectCapabilities(prompt, result.TaskType)

	// Estimate tokens (rough approximation)
	result.EstTokens = tc.estimateTokens(prompt, context)

	// Check for images
	result.HasImages = tc.hasImages(prompt, context)

	return result
}

// detectTaskType finds the most likely task type from keywords
func (tc *TaskClassifier) detectTaskType(prompt string) (TaskType, float64) {
	scores := make(map[TaskType]int)

	for taskType, keywords := range tc.keywords {
		for _, kw := range keywords {
			if strings.Contains(prompt, kw) {
				scores[taskType]++
			}
		}
	}

	if len(scores) == 0 {
		return TaskTypeUnknown, 0.3
	}

	// Find highest scoring type
	var bestType TaskType
	var bestScore int
	var totalScore int

	for t, s := range scores {
		totalScore += s
		if s > bestScore {
			bestScore = s
			bestType = t
		}
	}

	// Confidence based on how dominant the best type is
	confidence := float64(bestScore) / float64(totalScore)
	if confidence > 0.8 {
		confidence = 0.9
	}

	return bestType, confidence
}

// detectEnvironment determines the programming environment
func (tc *TaskClassifier) detectEnvironment(prompt string, context *TaskContext) string {
	// Define patterns in deterministic order (more specific first)
	// TypeScript checked before JavaScript since both match "npm"
	envChecks := []struct {
		name    string
		pattern *regexp.Regexp
	}{
		{"go", regexp.MustCompile(`(?i)(\.go\b|golang|go\s+mod|go\s+test|package\s+\w+)`)},
		{"python", regexp.MustCompile(`(?i)(\.py\b|python|pip\s+|pytest|import\s+\w+)`)},
		{"typescript", regexp.MustCompile(`(?i)(\.tsx?\b|typescript|npm\s+|yarn\s+|node)`)},
		{"javascript", regexp.MustCompile(`(?i)(\.jsx?\b|javascript)`)},
		{"rust", regexp.MustCompile(`(?i)(\.rs\b|rust|cargo|rustc)`)},
		{"java", regexp.MustCompile(`(?i)(\.java\b|java|maven|gradle)`)},
		{"c", regexp.MustCompile(`(?i)(\.c\b|\.h\b|gcc|clang|makefile)`)},
		{"cpp", regexp.MustCompile(`(?i)(\.cpp\b|\.hpp\b|c\+\+|cmake)`)},
		{"shell", regexp.MustCompile(`(?i)(\.sh\b|bash|shell|script)`)},
	}

	// Check prompt for language hints in order
	for _, check := range envChecks {
		if check.pattern.MatchString(prompt) {
			return check.name
		}
	}

	// Check context for file extensions
	if context != nil {
		for _, f := range context.FilesRead {
			ext := strings.ToLower(getFileExtension(f))
			switch ext {
			case ".go":
				return "go"
			case ".py":
				return "python"
			case ".ts", ".tsx":
				return "typescript"
			case ".js", ".jsx":
				return "javascript"
			case ".rs":
				return "rust"
			case ".java":
				return "java"
			case ".c", ".h":
				return "c"
			case ".cpp", ".hpp", ".cc":
				return "cpp"
			}
		}
	}

	return "unknown"
}

// calculateComplexity estimates task complexity (0-1)
func (tc *TaskClassifier) calculateComplexity(prompt string, taskType TaskType, context *TaskContext) float64 {
	complexity := 0.5 // Base

	// Task type baseline
	switch taskType {
	case TaskTypeExplore:
		complexity = 0.2
	case TaskTypeExplain:
		complexity = 0.3
	case TaskTypeBugfix:
		complexity = 0.5
	case TaskTypeRefactor:
		complexity = 0.7
	case TaskTypeFeature:
		complexity = 0.7
	case TaskTypeTest:
		complexity = 0.4
	}

	// Complexity modifiers

	// Multiple files mentioned increases complexity
	fileCount := strings.Count(prompt, ".go") + strings.Count(prompt, ".py") +
		strings.Count(prompt, ".ts") + strings.Count(prompt, ".js")
	if fileCount > 3 {
		complexity += 0.15
	}

	// Long prompts suggest more complex tasks
	wordCount := len(strings.Fields(prompt))
	if wordCount > 100 {
		complexity += 0.1
	}
	if wordCount > 200 {
		complexity += 0.1
	}

	// Complexity keywords
	complexKeywords := []string{
		"multiple", "all", "entire", "whole", "comprehensive",
		"architecture", "system", "redesign", "migrate",
	}
	for _, kw := range complexKeywords {
		if strings.Contains(prompt, kw) {
			complexity += 0.05
		}
	}

	// Context-based adjustments
	if context != nil {
		// More turns = more complex task
		if context.TurnCount > 5 {
			complexity += 0.1
		}
		// Errors encountered = harder task
		if len(context.Errors) > 0 {
			complexity += 0.1
		}
	}

	// Clamp to [0, 1]
	if complexity > 1.0 {
		complexity = 1.0
	}
	if complexity < 0.0 {
		complexity = 0.0
	}

	return complexity
}

// detectCapabilities determines required model capabilities
func (tc *TaskClassifier) detectCapabilities(prompt string, taskType TaskType) []string {
	caps := []string{"code"} // Always need code capability

	// Tool use for most interactive tasks
	if taskType != TaskTypeExplain {
		caps = append(caps, "tool_use")
	}

	// Reasoning for complex tasks
	switch taskType {
	case TaskTypeBugfix, TaskTypeRefactor, TaskTypeFeature:
		caps = append(caps, "reasoning")
	}

	// Check for specific requirements
	if strings.Contains(prompt, "image") || strings.Contains(prompt, "screenshot") ||
		strings.Contains(prompt, "diagram") || strings.Contains(prompt, "visual") {
		caps = append(caps, "vision")
	}

	// Long context for large codebases
	if strings.Contains(prompt, "entire") || strings.Contains(prompt, "all files") ||
		strings.Contains(prompt, "whole project") {
		caps = append(caps, "long_context")
	}

	return caps
}

// estimateTokens approximates input token count
func (tc *TaskClassifier) estimateTokens(prompt string, context *TaskContext) int {
	// Rough estimate: ~4 chars per token
	promptTokens := len(prompt) / 4

	// Add context tokens
	if context != nil {
		// Previous turns
		promptTokens += context.TurnCount * 500

		// Files read
		promptTokens += len(context.FilesRead) * 200
	}

	// Minimum
	if promptTokens < 100 {
		promptTokens = 100
	}

	return promptTokens
}

// hasImages checks if task involves images
func (tc *TaskClassifier) hasImages(prompt string, context *TaskContext) bool {
	imageKeywords := []string{"image", "screenshot", "picture", "diagram", "visual", "photo"}
	for _, kw := range imageKeywords {
		if strings.Contains(prompt, kw) {
			return true
		}
	}
	return false
}

// getFileExtension extracts file extension
func getFileExtension(path string) string {
	lastDot := strings.LastIndex(path, ".")
	if lastDot == -1 {
		return ""
	}
	return path[lastDot:]
}

// DefaultTaskClassifier is the global classifier instance
var DefaultTaskClassifier = NewTaskClassifier()
