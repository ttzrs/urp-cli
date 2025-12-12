package harness

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// ProgressLog manages append-only session progress tracking.
// Enables cross-session resume by reading recent entries.
// Format: [ISO8601-timestamp] Session <sessionID>: <entry>
// Example: [2025-12-12T14:30:45Z] Session abc123: Tried approach X, failed with error Y
type ProgressLog struct {
	workDir string
	logPath string
}

// NewProgressLog creates a new ProgressLog manager for a workspace.
func NewProgressLog(workDir string) *ProgressLog {
	return &ProgressLog{
		workDir: workDir,
		logPath: filepath.Join(workDir, ".urp", "progress.txt"),
	}
}

// Append adds a new entry to the progress log.
// Thread-safe via OS-level file locking (append mode).
func (p *ProgressLog) Append(sessionID, entry string) error {
	if p.logPath == "" {
		return fmt.Errorf("progress log path not initialized")
	}

	timestamp := time.Now().Format(time.RFC3339)
	line := fmt.Sprintf("[%s] Session %s: %s\n", timestamp, sessionID, entry)

	// Open in append mode (OS handles exclusivity)
	f, err := os.OpenFile(p.logPath, os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		return fmt.Errorf("failed to open progress log: %w", err)
	}
	defer f.Close()

	if _, err := f.WriteString(line); err != nil {
		return fmt.Errorf("failed to write progress log: %w", err)
	}

	return nil
}

// ReadRecent reads the last N entries from the progress log.
// Returns entries in chronological order (oldest first).
// Useful for resume context: agent reads last 5-10 entries to understand prior progress.
func (p *ProgressLog) ReadRecent(limit int) ([]string, error) {
	if limit <= 0 {
		return nil, fmt.Errorf("limit must be > 0")
	}

	f, err := os.Open(p.logPath)
	if err != nil {
		if os.IsNotExist(err) {
			return []string{}, nil // File doesn't exist yet (first run)
		}
		return nil, fmt.Errorf("failed to open progress log: %w", err)
	}
	defer f.Close()

	var lines []string
	scanner := bufio.NewScanner(f)

	// Skip header comments (lines starting with #)
	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(strings.TrimSpace(line), "#") && strings.TrimSpace(line) != "" {
			lines = append(lines, line)
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("failed to read progress log: %w", err)
	}

	// Return last N entries
	if len(lines) > limit {
		return lines[len(lines)-limit:], nil
	}

	return lines, nil
}

// ReadAll reads all non-comment entries from the progress log.
func (p *ProgressLog) ReadAll() ([]string, error) {
	f, err := os.Open(p.logPath)
	if err != nil {
		if os.IsNotExist(err) {
			return []string{}, nil
		}
		return nil, fmt.Errorf("failed to open progress log: %w", err)
	}
	defer f.Close()

	var lines []string
	scanner := bufio.NewScanner(f)

	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(strings.TrimSpace(line), "#") && strings.TrimSpace(line) != "" {
			lines = append(lines, line)
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("failed to read progress log: %w", err)
	}

	return lines, nil
}

// ExtractFailures extracts entries containing failure indicators.
// Used to identify approaches that didn't work, preventing repeated attempts.
// Patterns: "failed", "error", "panic", "crashed", "timeout"
func (p *ProgressLog) ExtractFailures() ([]string, error) {
	entries, err := p.ReadAll()
	if err != nil {
		return nil, err
	}

	failurePatterns := []string{"failed", "error", "panic", "crashed", "timeout"}
	failureRegex := regexp.MustCompile(
		fmt.Sprintf("(?i)(%s)", strings.Join(failurePatterns, "|")),
	)

	var failures []string
	for _, entry := range entries {
		if failureRegex.MatchString(entry) {
			failures = append(failures, entry)
		}
	}

	return failures, nil
}

// ExtractSuccesses extracts entries indicating successful completion.
// Patterns: "succeeded", "completed", "done", "resolved", "fixed"
func (p *ProgressLog) ExtractSuccesses() ([]string, error) {
	entries, err := p.ReadAll()
	if err != nil {
		return nil, err
	}

	successPatterns := []string{"succeeded", "completed", "done", "resolved", "fixed", "implemented"}
	successRegex := regexp.MustCompile(
		fmt.Sprintf("(?i)(%s)", strings.Join(successPatterns, "|")),
	)

	var successes []string
	for _, entry := range entries {
		if successRegex.MatchString(entry) {
			successes = append(successes, entry)
		}
	}

	return successes, nil
}

// GetSessionEntries returns all entries for a specific session.
func (p *ProgressLog) GetSessionEntries(sessionID string) ([]string, error) {
	entries, err := p.ReadAll()
	if err != nil {
		return nil, err
	}

	sessionPattern := regexp.MustCompile(fmt.Sprintf(`Session %s:`, regexp.QuoteMeta(sessionID)))

	var sessionEntries []string
	for _, entry := range entries {
		if sessionPattern.MatchString(entry) {
			sessionEntries = append(sessionEntries, entry)
		}
	}

	return sessionEntries, nil
}

// Clear truncates the progress log (destructive operation, use carefully).
// Typically used when archiving a project or starting fresh.
func (p *ProgressLog) Clear() error {
	header := "# URP Progress Log - DO NOT EDIT MANUALLY\n"
	header += fmt.Sprintf("# Cleared: %s\n", time.Now().Format(time.RFC3339))
	header += "# Format: [timestamp] Session <sessionID>: <entry>\n\n"

	return os.WriteFile(p.logPath, []byte(header), 0644)
}
