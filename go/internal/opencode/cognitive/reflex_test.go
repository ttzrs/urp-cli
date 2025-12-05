package cognitive

import (
	"context"
	"testing"
)

func TestExtractFilePaths(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected []string
	}{
		{
			name:     "Go error with line numbers",
			input:    "internal/auth/jwt.go:45:12: undefined: TokenClaims",
			expected: []string{"internal/auth/jwt.go"},
		},
		{
			name:     "Python traceback",
			input:    `File "/app/main.py", line 23, in main`,
			expected: []string{"/app/main.py"},
		},
		{
			name:     "Node.js stack trace",
			input:    "at processTicksAndRejections (/app/src/index.js:45:12)",
			expected: []string{"/app/src/index.js"},
		},
		{
			name: "Multiple files",
			input: `
internal/server/handler.go:12: error
internal/db/connection.go:45: timeout
`,
			expected: []string{"internal/server/handler.go", "internal/db/connection.go"},
		},
		{
			name:     "No file paths",
			input:    "Error: something went wrong",
			expected: []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := extractFilePaths(tt.input)

			if len(result) != len(tt.expected) {
				t.Errorf("got %d paths, want %d", len(result), len(tt.expected))
				return
			}

			// Check all expected paths are present
			resultMap := make(map[string]bool)
			for _, p := range result {
				resultMap[p] = true
			}

			for _, exp := range tt.expected {
				if !resultMap[exp] {
					t.Errorf("missing expected path: %s", exp)
				}
			}
		})
	}
}

func TestTruncateError(t *testing.T) {
	short := "short error"
	if truncateError(short, 100) != short {
		t.Error("should not truncate short string")
	}

	long := "this is a very long error message that should be truncated to fit within the limit"
	truncated := truncateError(long, 30)
	if len(truncated) > 30 {
		t.Errorf("truncated string too long: %d", len(truncated))
	}
	if !endsWith(truncated, "...") {
		t.Error("truncated string should end with ...")
	}
}

func TestHandleTrauma(t *testing.T) {
	reflex := NewReflex(nil, "/test")

	errorOutput := `
panic: runtime error: index out of range
	/app/internal/parser/lexer.go:123
	/app/cmd/main.go:45
`

	ctx := context.Background()
	tc, err := reflex.HandleTrauma(ctx, errorOutput, "/app/internal/parser/lexer.go")
	if err != nil {
		t.Fatalf("HandleTrauma failed: %v", err)
	}

	if tc.Error == "" {
		t.Error("TraumaContext should have error")
	}

	if len(tc.RelatedFiles) == 0 {
		t.Error("TraumaContext should have related files")
	}

	// Verify file paths were extracted
	hasLexer := false
	for _, f := range tc.RelatedFiles {
		if contains(f, "lexer.go") {
			hasLexer = true
		}
	}
	if !hasLexer {
		t.Error("should have extracted lexer.go from error")
	}
}

func TestFormatEmergencyContext(t *testing.T) {
	reflex := NewReflex(nil, "/test")

	tc := &TraumaContext{
		Error:        "undefined: FooBar",
		RelatedFiles: []string{"internal/foo.go", "internal/bar.go"},
		SimilarErrors: []SimilarError{
			{Error: "undefined: X", Solution: "import the package", Score: 0.9},
		},
	}

	formatted := reflex.FormatEmergencyContext(tc)

	if !contains(formatted, "ERROR CONTEXT") {
		t.Error("should have error context header")
	}
	if !contains(formatted, "undefined: FooBar") {
		t.Error("should contain error")
	}
	if !contains(formatted, "internal/foo.go") {
		t.Error("should contain related files")
	}
	if !contains(formatted, "import the package") {
		t.Error("should contain past solutions")
	}
}

func endsWith(s, suffix string) bool {
	return len(s) >= len(suffix) && s[len(s)-len(suffix):] == suffix
}

func contains(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
