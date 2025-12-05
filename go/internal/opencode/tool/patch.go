package tool

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/joss/urp/internal/opencode/domain"
)

// Patch applies unified diff patches to files
type Patch struct {
	workDir string
}

func NewPatch(workDir string) *Patch {
	return &Patch{workDir: workDir}
}

func (p *Patch) Info() domain.Tool {
	return domain.Tool{
		Name:        "patch",
		Description: "Apply a unified diff patch to a file",
		Parameters: domain.JSONSchema{
			"type": "object",
			"properties": map[string]any{
				"file_path": map[string]any{
					"type":        "string",
					"description": "Path to the file to patch",
				},
				"patch": map[string]any{
					"type":        "string",
					"description": "Unified diff patch content",
				},
			},
			"required": []string{"file_path", "patch"},
		},
	}
}

func (p *Patch) Execute(ctx context.Context, args map[string]any) (*Result, error) {
	filePath, _ := args["file_path"].(string)
	patchContent, _ := args["patch"].(string)

	if filePath == "" {
		return &Result{Error: fmt.Errorf("file_path is required")}, nil
	}
	if patchContent == "" {
		return &Result{Error: fmt.Errorf("patch is required")}, nil
	}

	// Resolve path
	if !filepath.IsAbs(filePath) {
		filePath = filepath.Join(p.workDir, filePath)
	}

	// Read original file
	content, err := os.ReadFile(filePath)
	if err != nil && !os.IsNotExist(err) {
		return &Result{Error: fmt.Errorf("read file: %w", err)}, nil
	}

	original := string(content)
	lines := strings.Split(original, "\n")

	// Parse and apply hunks
	hunks, err := parseHunks(patchContent)
	if err != nil {
		return &Result{Error: fmt.Errorf("parse patch: %w", err)}, nil
	}

	// Apply hunks in reverse order to preserve line numbers
	for i := len(hunks) - 1; i >= 0; i-- {
		lines, err = applyHunk(lines, hunks[i])
		if err != nil {
			return &Result{Error: fmt.Errorf("apply hunk %d: %w", i+1, err)}, nil
		}
	}

	// Write result
	result := strings.Join(lines, "\n")

	// Atomic write
	dir := filepath.Dir(filePath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return &Result{Error: fmt.Errorf("mkdir: %w", err)}, nil
	}

	tmpFile, err := os.CreateTemp(dir, ".patch-*")
	if err != nil {
		return &Result{Error: fmt.Errorf("create temp: %w", err)}, nil
	}
	tmpPath := tmpFile.Name()

	if _, err := tmpFile.WriteString(result); err != nil {
		tmpFile.Close()
		os.Remove(tmpPath)
		return &Result{Error: fmt.Errorf("write: %w", err)}, nil
	}
	tmpFile.Close()

	// Preserve permissions
	if info, err := os.Stat(filePath); err == nil {
		os.Chmod(tmpPath, info.Mode())
	}

	if err := os.Rename(tmpPath, filePath); err != nil {
		os.Remove(tmpPath)
		return &Result{Error: fmt.Errorf("rename: %w", err)}, nil
	}

	// Calculate stats
	oldLines := len(strings.Split(original, "\n"))
	newLines := len(lines)
	added := 0
	removed := 0
	for _, h := range hunks {
		for _, l := range h.Lines {
			if strings.HasPrefix(l, "+") && !strings.HasPrefix(l, "+++") {
				added++
			} else if strings.HasPrefix(l, "-") && !strings.HasPrefix(l, "---") {
				removed++
			}
		}
	}

	return &Result{
		Title:  fmt.Sprintf("Patched %s", filepath.Base(filePath)),
		Output: fmt.Sprintf("Applied %d hunks: +%d -%d lines (%d → %d total)", len(hunks), added, removed, oldLines, newLines),
		Metadata: map[string]any{
			"file_path": filePath,
			"hunks":     len(hunks),
			"added":     added,
			"removed":   removed,
		},
	}, nil
}

// Hunk represents a single diff hunk
type Hunk struct {
	OldStart int
	OldCount int
	NewStart int
	NewCount int
	Lines    []string
}

// hunkHeader matches @@ -old_start,old_count +new_start,new_count @@
var hunkHeader = regexp.MustCompile(`^@@ -(\d+)(?:,(\d+))? \+(\d+)(?:,(\d+))? @@`)

func parseHunks(patch string) ([]Hunk, error) {
	var hunks []Hunk
	scanner := bufio.NewScanner(strings.NewReader(patch))

	var current *Hunk
	for scanner.Scan() {
		line := scanner.Text()

		// Skip diff headers
		if strings.HasPrefix(line, "---") || strings.HasPrefix(line, "+++") ||
			strings.HasPrefix(line, "diff ") || strings.HasPrefix(line, "index ") {
			continue
		}

		// New hunk?
		if matches := hunkHeader.FindStringSubmatch(line); matches != nil {
			if current != nil {
				hunks = append(hunks, *current)
			}

			oldStart, _ := strconv.Atoi(matches[1])
			oldCount := 1
			if matches[2] != "" {
				oldCount, _ = strconv.Atoi(matches[2])
			}
			newStart, _ := strconv.Atoi(matches[3])
			newCount := 1
			if matches[4] != "" {
				newCount, _ = strconv.Atoi(matches[4])
			}

			current = &Hunk{
				OldStart: oldStart,
				OldCount: oldCount,
				NewStart: newStart,
				NewCount: newCount,
			}
			continue
		}

		// Collect hunk lines
		if current != nil && (strings.HasPrefix(line, "+") || strings.HasPrefix(line, "-") || strings.HasPrefix(line, " ")) {
			current.Lines = append(current.Lines, line)
		}
	}

	if current != nil {
		hunks = append(hunks, *current)
	}

	if len(hunks) == 0 {
		return nil, fmt.Errorf("no hunks found in patch")
	}

	return hunks, nil
}

func applyHunk(lines []string, h Hunk) ([]string, error) {
	// Convert to 0-indexed
	start := h.OldStart - 1
	if start < 0 {
		start = 0
	}

	// Build new lines for this section
	var newSection []string
	oldIdx := start

	for _, line := range h.Lines {
		if len(line) == 0 {
			continue
		}

		switch line[0] {
		case ' ':
			// Context - verify match
			content := line[1:]
			if oldIdx >= len(lines) {
				return nil, fmt.Errorf("context line beyond file end")
			}
			if lines[oldIdx] != content {
				return nil, fmt.Errorf("context mismatch at line %d: expected %q, got %q", oldIdx+1, content, lines[oldIdx])
			}
			newSection = append(newSection, content)
			oldIdx++

		case '-':
			// Remove - verify match
			content := line[1:]
			if oldIdx >= len(lines) {
				return nil, fmt.Errorf("remove line beyond file end")
			}
			if lines[oldIdx] != content {
				return nil, fmt.Errorf("remove mismatch at line %d: expected %q, got %q", oldIdx+1, content, lines[oldIdx])
			}
			oldIdx++

		case '+':
			// Add
			newSection = append(newSection, line[1:])
		}
	}

	// Replace the section in the original file
	end := oldIdx
	result := make([]string, 0, len(lines)-h.OldCount+len(newSection))
	result = append(result, lines[:start]...)
	result = append(result, newSection...)
	result = append(result, lines[end:]...)

	return result, nil
}
