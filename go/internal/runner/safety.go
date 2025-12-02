// Package runner provides command execution with safety checks.
package runner

import (
	"regexp"
	"strings"
)

// RiskLevel indicates the danger level of a command.
type RiskLevel int

const (
	RiskSafe RiskLevel = iota
	RiskWarning
	RiskBlocked
)

// RiskResult contains the analysis of a command's risk.
type RiskResult struct {
	Level       RiskLevel
	Reason      string
	Alternative string
}

// Pattern defines a dangerous command pattern.
type Pattern struct {
	Regex       *regexp.Regexp
	Level       RiskLevel
	Reason      string
	Alternative string
}

// ImmuneSystem filters dangerous commands before execution (⊥ primitive).
type ImmuneSystem struct {
	patterns []Pattern
}

// NewImmuneSystem creates a new immune system with default patterns.
func NewImmuneSystem() *ImmuneSystem {
	return &ImmuneSystem{
		patterns: defaultPatterns(),
	}
}

func defaultPatterns() []Pattern {
	return []Pattern{
		// Filesystem destruction
		{
			Regex:       regexp.MustCompile(`rm\s+(-[rf]+\s+)*(/|/\*|\.\.|~)(\s|$)`),
			Level:       RiskBlocked,
			Reason:      "IMMUNE_BLOCK: Destructive filesystem operation on critical path",
			Alternative: "Be specific: rm -rf ./specific-directory",
		},
		{
			Regex:       regexp.MustCompile(`mkfs\s`),
			Level:       RiskBlocked,
			Reason:      "IMMUNE_BLOCK: Filesystem formatting is blocked",
			Alternative: "Format operations require explicit user confirmation outside URP",
		},
		{
			Regex:       regexp.MustCompile(`dd\s+.*of=/dev/`),
			Level:       RiskBlocked,
			Reason:      "IMMUNE_BLOCK: Direct device write is blocked",
			Alternative: "Device operations require explicit user confirmation",
		},

		// Git history violence
		{
			Regex:       regexp.MustCompile(`git\s+push\s+.*--force(\s|$)`),
			Level:       RiskBlocked,
			Reason:      "IMMUNE_BLOCK: Force push destroys remote history",
			Alternative: "Use: git push --force-with-lease",
		},
		{
			Regex:       regexp.MustCompile(`git\s+reset\s+--hard\s+HEAD~`),
			Level:       RiskWarning,
			Reason:      "WARNING: Hard reset discards uncommitted changes",
			Alternative: "Consider: git stash first, or use git reset --soft",
		},
		{
			Regex:       regexp.MustCompile(`rm\s+-rf\s+\.git(\s|$)`),
			Level:       RiskBlocked,
			Reason:      "IMMUNE_BLOCK: Deleting .git destroys repository history",
			Alternative: "If intentional, do this manually outside URP",
		},

		// Database amnesia
		{
			Regex:       regexp.MustCompile(`(?i)DROP\s+DATABASE`),
			Level:       RiskBlocked,
			Reason:      "IMMUNE_BLOCK: DROP DATABASE requires explicit confirmation",
			Alternative: "Use database admin tools with proper backup procedures",
		},
		{
			Regex:       regexp.MustCompile(`(?i)DELETE\s+FROM\s+\w+\s*(;|$)`),
			Level:       RiskWarning,
			Reason:      "WARNING: DELETE without WHERE clause affects all rows",
			Alternative: "Add WHERE clause: DELETE FROM table WHERE condition",
		},
		{
			Regex:       regexp.MustCompile(`(?i)TRUNCATE\s+TABLE`),
			Level:       RiskWarning,
			Reason:      "WARNING: TRUNCATE removes all data from table",
			Alternative: "Ensure you have backups before truncating",
		},

		// Credential leaks
		{
			Regex:       regexp.MustCompile(`git\s+add\s+.*\.env`),
			Level:       RiskBlocked,
			Reason:      "IMMUNE_BLOCK: .env files contain secrets",
			Alternative: "Add .env to .gitignore, use environment variables",
		},
		{
			Regex:       regexp.MustCompile(`git\s+add\s+.*(id_rsa|id_ed25519|\.pem|\.key)`),
			Level:       RiskBlocked,
			Reason:      "IMMUNE_BLOCK: Private keys must never be committed",
			Alternative: "Add to .gitignore, use secrets management",
		},
		{
			Regex:       regexp.MustCompile(`cat\s+.*(id_rsa|id_ed25519|\.pem|credentials)`),
			Level:       RiskWarning,
			Reason:      "WARNING: Displaying sensitive file contents",
			Alternative: "Avoid displaying credentials in terminal history",
		},

		// Container escape attempts
		{
			Regex:       regexp.MustCompile(`docker\s+run\s+.*--privileged`),
			Level:       RiskWarning,
			Reason:      "WARNING: Privileged containers can escape isolation",
			Alternative: "Use specific capabilities instead: --cap-add=SYS_PTRACE",
		},
		{
			Regex:       regexp.MustCompile(`docker\s+run\s+.*-v\s+/:/`),
			Level:       RiskBlocked,
			Reason:      "IMMUNE_BLOCK: Mounting root filesystem is blocked",
			Alternative: "Mount specific directories: -v /home/user/data:/data",
		},

		// Network dangers
		{
			Regex:       regexp.MustCompile(`curl\s+.*\|\s*(bash|sh)`),
			Level:       RiskWarning,
			Reason:      "WARNING: Piping curl to shell is risky",
			Alternative: "Download first, inspect, then execute: curl -o script.sh URL && cat script.sh && bash script.sh",
		},
	}
}

// Analyze checks a command against safety patterns.
func (is *ImmuneSystem) Analyze(command string) RiskResult {
	cmd := strings.TrimSpace(command)

	for _, p := range is.patterns {
		if p.Regex.MatchString(cmd) {
			return RiskResult{
				Level:       p.Level,
				Reason:      p.Reason,
				Alternative: p.Alternative,
			}
		}
	}

	return RiskResult{
		Level:  RiskSafe,
		Reason: "",
	}
}

// IsSafe returns true if the command is safe to execute.
func (is *ImmuneSystem) IsSafe(command string) bool {
	result := is.Analyze(command)
	return result.Level != RiskBlocked
}

// DefaultImmuneSystem is the global instance.
var DefaultImmuneSystem = NewImmuneSystem()

// AnalyzeRisk is a convenience function using the default immune system.
func AnalyzeRisk(command string) RiskResult {
	return DefaultImmuneSystem.Analyze(command)
}
