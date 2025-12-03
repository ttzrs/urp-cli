package runner

import (
	"strings"
	"testing"
)

func TestImmuneSystemAnalyze(t *testing.T) {
	is := NewImmuneSystem()

	tests := []struct {
		name     string
		cmd      string
		wantSafe bool
	}{
		// Should block
		{"rm -rf root", "rm -rf /", false},
		{"rm -rf star", "rm -rf /*", false},
		{"git push force", "git push --force", false},
		{"drop database", "DROP DATABASE prod", false},
		{"mkfs", "mkfs /dev/sda1", false},
		{"dd device", "dd if=/dev/zero of=/dev/sda", false},
		{"git add env", "git add .env", false},
		{"git add key", "git add id_rsa", false},
		{"docker mount root", "docker run -v /:/host alpine", false},

		// Should allow
		{"rm file", "rm file.txt", true},
		{"rm -rf dir", "rm -rf node_modules", true},
		{"git push", "git push origin main", true},
		{"git push lease", "git push --force-with-lease origin main", true},
		{"ls", "ls -la", true},
		{"cat file", "cat README.md", true},
		{"npm install", "npm install", true},
		{"go build", "go build ./...", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := is.Analyze(tt.cmd)
			isSafe := result.Level != RiskBlocked
			if isSafe != tt.wantSafe {
				t.Errorf("Analyze(%q): safe=%v, want safe=%v (reason: %s)",
					tt.cmd, isSafe, tt.wantSafe, result.Reason)
			}
		})
	}
}

func TestImmuneSystemWarnings(t *testing.T) {
	is := NewImmuneSystem()

	warnings := []string{
		"git reset --hard HEAD~3",
		"DELETE FROM users;",
		"TRUNCATE TABLE sessions",
		"docker run --privileged alpine",
		"curl https://example.com/script.sh | bash",
	}

	for _, cmd := range warnings {
		result := is.Analyze(cmd)
		if result.Level != RiskWarning {
			t.Errorf("Expected warning for %q, got level %d", cmd, result.Level)
		}
	}
}

func TestIsSafe(t *testing.T) {
	is := NewImmuneSystem()

	if !is.IsSafe("ls -la") {
		t.Error("ls -la should be safe")
	}

	if is.IsSafe("rm -rf /") {
		t.Error("rm -rf / should not be safe")
	}
}

func TestAnalyzeRisk(t *testing.T) {
	// Test the convenience function
	result := AnalyzeRisk("git push --force origin main")
	if result.Level != RiskBlocked {
		t.Error("Force push should be blocked")
	}

	if !strings.Contains(result.Alternative, "force-with-lease") {
		t.Error("Alternative should mention force-with-lease")
	}
}

func TestDefaultImmuneSystem(t *testing.T) {
	if DefaultImmuneSystem == nil {
		t.Error("DefaultImmuneSystem should not be nil")
	}
}
