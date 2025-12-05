// Package tool provides the "Poor Man's LSP" - CLI-based diagnostics.
// Uses external tools (go vet, tsc, eslint, etc.) to check code quality
// without requiring a full LSP implementation.
package tool

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/joss/urp/internal/opencode/domain"
)

// Diagnostics provides code quality checks using CLI tools
type Diagnostics struct {
	workDir string
}

// NewDiagnostics creates a new diagnostics tool
func NewDiagnostics(workDir string) *Diagnostics {
	return &Diagnostics{workDir: workDir}
}

func (d *Diagnostics) Info() domain.Tool {
	return domain.Tool{
		ID:   "diagnostics",
		Name: "diagnostics",
		Description: `Run code diagnostics using CLI tools (Poor Man's LSP).
Detects language from files and runs appropriate checker:
- Go: go vet, go build -n
- TypeScript/JavaScript: tsc --noEmit, eslint
- Python: ruff check, mypy
- Rust: cargo check`,
		Parameters: domain.JSONSchema{
			"type": "object",
			"properties": map[string]any{
				"path": map[string]any{
					"type":        "string",
					"description": "File or directory to check (defaults to workdir)",
				},
				"language": map[string]any{
					"type":        "string",
					"enum":        []string{"go", "typescript", "javascript", "python", "rust", "auto"},
					"description": "Language to check (auto-detects if not specified)",
				},
				"fix": map[string]any{
					"type":        "boolean",
					"description": "Attempt to auto-fix issues (where supported)",
				},
			},
		},
	}
}

func (d *Diagnostics) Execute(ctx context.Context, args map[string]any) (*Result, error) {
	path := d.workDir
	if p, ok := args["path"].(string); ok && p != "" {
		path = p
		if !filepath.IsAbs(path) {
			path = filepath.Join(d.workDir, path)
		}
	}

	lang := "auto"
	if l, ok := args["language"].(string); ok && l != "" {
		lang = l
	}

	fix := false
	if f, ok := args["fix"].(bool); ok {
		fix = f
	}

	// Auto-detect language
	if lang == "auto" {
		lang = detectLanguage(path)
	}

	var output strings.Builder
	var errors []string

	switch lang {
	case "go":
		errors = d.checkGo(ctx, path, fix, &output)
	case "typescript", "javascript":
		errors = d.checkTypeScript(ctx, path, fix, &output)
	case "python":
		errors = d.checkPython(ctx, path, fix, &output)
	case "rust":
		errors = d.checkRust(ctx, path, fix, &output)
	default:
		return &Result{
			Title:  "Diagnostics",
			Output: fmt.Sprintf("Unknown or unsupported language: %s", lang),
		}, nil
	}

	// Build summary
	result := output.String()
	if len(errors) > 0 {
		result += fmt.Sprintf("\n\n⚠ Found %d issue(s)\n", len(errors))
	} else {
		result += "\n\n✓ No issues found\n"
	}

	return &Result{
		Title:  fmt.Sprintf("Diagnostics (%s)", lang),
		Output: result,
		Metadata: map[string]any{
			"language":    lang,
			"path":        path,
			"error_count": len(errors),
		},
	}, nil
}

// checkGo runs Go diagnostics
func (d *Diagnostics) checkGo(ctx context.Context, path string, fix bool, output *strings.Builder) []string {
	var errors []string

	// Determine if path is file or directory
	dir := path
	if info, err := os.Stat(path); err == nil && !info.IsDir() {
		dir = filepath.Dir(path)
	}

	output.WriteString("=== Go Diagnostics ===\n\n")

	// 1. go vet
	output.WriteString("$ go vet ./...\n")
	cmd := exec.CommandContext(ctx, "go", "vet", "./...")
	cmd.Dir = dir
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	cmd.Run()

	vetOutput := stderr.String() + stdout.String()
	if vetOutput != "" {
		output.WriteString(vetOutput)
		errors = append(errors, parseGoErrors(vetOutput)...)
	} else {
		output.WriteString("✓ No issues\n")
	}
	output.WriteString("\n")

	// 2. go build (check compilation)
	output.WriteString("$ go build ./...\n")
	cmd = exec.CommandContext(ctx, "go", "build", "./...")
	cmd.Dir = dir
	stdout.Reset()
	stderr.Reset()
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	cmd.Run()

	buildOutput := stderr.String() + stdout.String()
	if buildOutput != "" {
		output.WriteString(buildOutput)
		errors = append(errors, parseGoErrors(buildOutput)...)
	} else {
		output.WriteString("✓ Compiles successfully\n")
	}
	output.WriteString("\n")

	// 3. staticcheck (if available)
	if _, err := exec.LookPath("staticcheck"); err == nil {
		output.WriteString("$ staticcheck ./...\n")
		cmd = exec.CommandContext(ctx, "staticcheck", "./...")
		cmd.Dir = dir
		stdout.Reset()
		stderr.Reset()
		cmd.Stdout = &stdout
		cmd.Stderr = &stderr
		cmd.Run()

		scOutput := stderr.String() + stdout.String()
		if scOutput != "" {
			output.WriteString(scOutput)
			errors = append(errors, parseGoErrors(scOutput)...)
		} else {
			output.WriteString("✓ No issues\n")
		}
	}

	return errors
}

// checkTypeScript runs TypeScript/JavaScript diagnostics
func (d *Diagnostics) checkTypeScript(ctx context.Context, path string, fix bool, output *strings.Builder) []string {
	var errors []string

	dir := path
	if info, err := os.Stat(path); err == nil && !info.IsDir() {
		dir = filepath.Dir(path)
	}

	output.WriteString("=== TypeScript/JavaScript Diagnostics ===\n\n")

	// 1. tsc --noEmit (if tsconfig.json exists)
	tsconfigPath := filepath.Join(dir, "tsconfig.json")
	if _, err := os.Stat(tsconfigPath); err == nil {
		output.WriteString("$ tsc --noEmit\n")
		cmd := exec.CommandContext(ctx, "npx", "tsc", "--noEmit")
		cmd.Dir = dir
		var stdout, stderr bytes.Buffer
		cmd.Stdout = &stdout
		cmd.Stderr = &stderr
		cmd.Run()

		tscOutput := stdout.String() + stderr.String()
		if tscOutput != "" {
			output.WriteString(tscOutput)
			errors = append(errors, parseTsErrors(tscOutput)...)
		} else {
			output.WriteString("✓ No type errors\n")
		}
		output.WriteString("\n")
	}

	// 2. eslint (if available)
	if _, err := exec.LookPath("eslint"); err == nil {
		args := []string{"."}
		if fix {
			args = append([]string{"--fix"}, args...)
		}

		output.WriteString(fmt.Sprintf("$ eslint %s\n", strings.Join(args, " ")))
		cmd := exec.CommandContext(ctx, "npx", append([]string{"eslint"}, args...)...)
		cmd.Dir = dir
		var stdout, stderr bytes.Buffer
		cmd.Stdout = &stdout
		cmd.Stderr = &stderr
		cmd.Run()

		eslintOutput := stdout.String() + stderr.String()
		if strings.Contains(eslintOutput, "error") || strings.Contains(eslintOutput, "warning") {
			output.WriteString(eslintOutput)
			// Count errors
			errorCount := strings.Count(eslintOutput, "error")
			for i := 0; i < errorCount; i++ {
				errors = append(errors, "eslint error")
			}
		} else {
			output.WriteString("✓ No linting issues\n")
		}
	}

	return errors
}

// checkPython runs Python diagnostics
func (d *Diagnostics) checkPython(ctx context.Context, path string, fix bool, output *strings.Builder) []string {
	var errors []string

	dir := path
	if info, err := os.Stat(path); err == nil && !info.IsDir() {
		dir = filepath.Dir(path)
	}

	output.WriteString("=== Python Diagnostics ===\n\n")

	// 1. ruff (fast Python linter)
	if _, err := exec.LookPath("ruff"); err == nil {
		args := []string{"check", "."}
		if fix {
			args = []string{"check", "--fix", "."}
		}

		output.WriteString(fmt.Sprintf("$ ruff %s\n", strings.Join(args, " ")))
		cmd := exec.CommandContext(ctx, "ruff", args...)
		cmd.Dir = dir
		var stdout, stderr bytes.Buffer
		cmd.Stdout = &stdout
		cmd.Stderr = &stderr
		cmd.Run()

		ruffOutput := stdout.String() + stderr.String()
		if ruffOutput != "" {
			output.WriteString(ruffOutput)
			errors = append(errors, parsePythonErrors(ruffOutput)...)
		} else {
			output.WriteString("✓ No issues\n")
		}
		output.WriteString("\n")
	}

	// 2. mypy (type checking)
	if _, err := exec.LookPath("mypy"); err == nil {
		output.WriteString("$ mypy .\n")
		cmd := exec.CommandContext(ctx, "mypy", ".")
		cmd.Dir = dir
		var stdout, stderr bytes.Buffer
		cmd.Stdout = &stdout
		cmd.Stderr = &stderr
		cmd.Run()

		mypyOutput := stdout.String() + stderr.String()
		if strings.Contains(mypyOutput, "error:") {
			output.WriteString(mypyOutput)
			errors = append(errors, parsePythonErrors(mypyOutput)...)
		} else {
			output.WriteString("✓ No type errors\n")
		}
	}

	return errors
}

// checkRust runs Rust diagnostics
func (d *Diagnostics) checkRust(ctx context.Context, path string, fix bool, output *strings.Builder) []string {
	var errors []string

	dir := path
	if info, err := os.Stat(path); err == nil && !info.IsDir() {
		dir = filepath.Dir(path)
	}

	// Find Cargo.toml
	cargoPath := filepath.Join(dir, "Cargo.toml")
	if _, err := os.Stat(cargoPath); err != nil {
		// Try parent directories
		for d := dir; d != "/" && d != "."; d = filepath.Dir(d) {
			cargoPath = filepath.Join(d, "Cargo.toml")
			if _, err := os.Stat(cargoPath); err == nil {
				dir = d
				break
			}
		}
	}

	output.WriteString("=== Rust Diagnostics ===\n\n")

	// cargo check
	args := []string{"check", "--message-format=short"}
	output.WriteString(fmt.Sprintf("$ cargo %s\n", strings.Join(args, " ")))

	cmd := exec.CommandContext(ctx, "cargo", args...)
	cmd.Dir = dir
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	cmd.Run()

	cargoOutput := stderr.String() + stdout.String()
	if strings.Contains(cargoOutput, "error[") || strings.Contains(cargoOutput, "warning:") {
		output.WriteString(cargoOutput)
		// Count errors
		errorCount := strings.Count(cargoOutput, "error[")
		for i := 0; i < errorCount; i++ {
			errors = append(errors, "cargo error")
		}
	} else {
		output.WriteString("✓ Compiles successfully\n")
	}

	// clippy (if available)
	if _, err := exec.LookPath("cargo-clippy"); err == nil {
		output.WriteString("\n$ cargo clippy\n")
		cmd = exec.CommandContext(ctx, "cargo", "clippy", "--", "-W", "clippy::all")
		cmd.Dir = dir
		stdout.Reset()
		stderr.Reset()
		cmd.Stdout = &stdout
		cmd.Stderr = &stderr
		cmd.Run()

		clippyOutput := stderr.String() + stdout.String()
		if strings.Contains(clippyOutput, "warning:") {
			output.WriteString(clippyOutput)
		} else {
			output.WriteString("✓ No clippy warnings\n")
		}
	}

	return errors
}

// detectLanguage auto-detects the project language
func detectLanguage(path string) string {
	dir := path
	if info, err := os.Stat(path); err == nil && !info.IsDir() {
		// Check file extension
		ext := strings.ToLower(filepath.Ext(path))
		switch ext {
		case ".go":
			return "go"
		case ".ts", ".tsx":
			return "typescript"
		case ".js", ".jsx", ".mjs":
			return "javascript"
		case ".py":
			return "python"
		case ".rs":
			return "rust"
		}
		dir = filepath.Dir(path)
	}

	// Check for project markers
	if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
		return "go"
	}
	if _, err := os.Stat(filepath.Join(dir, "tsconfig.json")); err == nil {
		return "typescript"
	}
	if _, err := os.Stat(filepath.Join(dir, "package.json")); err == nil {
		return "javascript"
	}
	if _, err := os.Stat(filepath.Join(dir, "pyproject.toml")); err == nil {
		return "python"
	}
	if _, err := os.Stat(filepath.Join(dir, "requirements.txt")); err == nil {
		return "python"
	}
	if _, err := os.Stat(filepath.Join(dir, "Cargo.toml")); err == nil {
		return "rust"
	}

	return "unknown"
}

// Error parsers

func parseGoErrors(output string) []string {
	var errors []string
	lines := strings.Split(output, "\n")
	for _, line := range lines {
		if strings.Contains(line, ".go:") && (strings.Contains(line, "error") || strings.Contains(line, ":")) {
			errors = append(errors, strings.TrimSpace(line))
		}
	}
	return errors
}

func parseTsErrors(output string) []string {
	var errors []string
	lines := strings.Split(output, "\n")
	for _, line := range lines {
		if strings.Contains(line, "error TS") {
			errors = append(errors, strings.TrimSpace(line))
		}
	}
	return errors
}

func parsePythonErrors(output string) []string {
	var errors []string
	lines := strings.Split(output, "\n")
	for _, line := range lines {
		if strings.Contains(line, "error:") || strings.Contains(line, " E") {
			errors = append(errors, strings.TrimSpace(line))
		}
	}
	return errors
}
