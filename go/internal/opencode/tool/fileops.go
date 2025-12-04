package tool

import (
	"bufio"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/bmatcuk/doublestar/v4"
)

func readFileWithLines(path string, offset, limit int) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", fmt.Errorf("open file: %w", err)
	}
	defer file.Close()

	var lines []string
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 1024*1024), 1024*1024)

	lineNum := 0
	for scanner.Scan() {
		lineNum++
		if offset > 0 && lineNum < offset {
			continue
		}
		if len(lines) >= limit {
			break
		}

		line := scanner.Text()
		// Truncate long lines
		if len(line) > 2000 {
			line = line[:2000] + "..."
		}
		lines = append(lines, fmt.Sprintf("%6d\t%s", lineNum, line))
	}

	if err := scanner.Err(); err != nil {
		return "", fmt.Errorf("read file: %w", err)
	}

	return strings.Join(lines, "\n"), nil
}

func writeFile(path, content string) error {
	// Create parent directories
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("create directory: %w", err)
	}

	return os.WriteFile(path, []byte(content), 0644)
}

func editFile(path, oldStr, newStr string, replaceAll bool) (int, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return 0, fmt.Errorf("read file: %w", err)
	}

	str := string(content)
	count := strings.Count(str, oldStr)

	if count == 0 {
		return 0, fmt.Errorf("old_string not found in file")
	}

	if count > 1 && !replaceAll {
		return 0, fmt.Errorf("old_string found %d times - use replace_all or provide more context", count)
	}

	var newContent string
	if replaceAll {
		newContent = strings.ReplaceAll(str, oldStr, newStr)
	} else {
		newContent = strings.Replace(str, oldStr, newStr, 1)
		count = 1
	}

	if err := os.WriteFile(path, []byte(newContent), 0644); err != nil {
		return 0, fmt.Errorf("write file: %w", err)
	}

	return count, nil
}

func globFiles(basePath, pattern string) ([]string, error) {
	var matches []string

	fsys := os.DirFS(basePath)
	err := doublestar.GlobWalk(fsys, pattern, func(path string, d fs.DirEntry) error {
		if !d.IsDir() {
			matches = append(matches, filepath.Join(basePath, path))
		}
		return nil
	})

	if err != nil {
		return nil, fmt.Errorf("glob: %w", err)
	}

	return matches, nil
}

func listDir(path string) ([]string, error) {
	entries, err := os.ReadDir(path)
	if err != nil {
		return nil, fmt.Errorf("read directory: %w", err)
	}

	var result []string
	for _, e := range entries {
		info, err := e.Info()
		if err != nil {
			continue
		}

		prefix := "  "
		if e.IsDir() {
			prefix = "📁"
		}

		size := ""
		if !e.IsDir() {
			size = formatSize(info.Size())
		}

		result = append(result, fmt.Sprintf("%s %s %s", prefix, e.Name(), size))
	}

	return result, nil
}

func formatSize(bytes int64) string {
	const unit = 1024
	if bytes < unit {
		return fmt.Sprintf("%d B", bytes)
	}
	div, exp := int64(unit), 0
	for n := bytes / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(bytes)/float64(div), "KMGTPE"[exp])
}
