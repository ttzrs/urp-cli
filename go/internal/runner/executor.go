// Package runner provides command execution with logging.
package runner

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/joss/urp/internal/config"
	"github.com/joss/urp/internal/domain"
	"github.com/joss/urp/internal/graph"
	urpstrings "github.com/joss/urp/internal/strings"
)

// Immune interface for command safety checking (DIP).
type Immune interface {
	Analyze(command string) RiskResult
}

// Executor runs commands and logs them to the graph.
type Executor struct {
	db      graph.Driver
	immune  Immune
	project string
}

// NewExecutor creates a new command executor.
// Uses default ImmuneSystem if none provided via WithImmune.
func NewExecutor(db graph.Driver, opts ...ExecutorOption) *Executor {
	project := config.Env().Project
	if project == "" {
		project = "unknown"
	}

	e := &Executor{
		db:      db,
		immune:  DefaultImmuneSystem,
		project: project,
	}
	for _, opt := range opts {
		opt(e)
	}
	return e
}

// ExecutorOption configures an Executor.
type ExecutorOption func(*Executor)

// WithImmune sets a custom Immune implementation (for testing).
func WithImmune(i Immune) ExecutorOption {
	return func(e *Executor) { e.immune = i }
}

// Result contains the output of a command execution.
type Result struct {
	ExitCode    int
	Duration    time.Duration
	Stdout      string
	Stderr      string
	WasBlocked  bool
	BlockReason string
}

// Run executes a command transparently (output goes to terminal).
func (e *Executor) Run(ctx context.Context, args []string) Result {
	if len(args) == 0 {
		return Result{ExitCode: 0}
	}

	command := strings.Join(args, " ")

	// Phase 0: Immune system check
	risk := e.immune.Analyze(command)
	if risk.Level == RiskBlocked {
		fmt.Fprintf(os.Stderr, "\n%s\n", risk.Reason)
		fmt.Fprintf(os.Stderr, "SUGGESTION: %s\n", risk.Alternative)

		// Log blocked attempt
		e.logToGraph(ctx, domain.Event{
			Command:       command,
			CmdBase:       args[0],
			ExitCode:      126,
			DurationSec:   0,
			Cwd:           getCwd(),
			StderrPreview: risk.Reason,
			Timestamp:     time.Now(),
			Project:       e.project,
			Type:          domain.ClassifyCommand(args[0]),
			IsConflict:    true,
		})

		return Result{
			ExitCode:    126,
			WasBlocked:  true,
			BlockReason: risk.Reason,
		}
	}

	if risk.Level == RiskWarning {
		fmt.Fprintf(os.Stderr, "\n%s\n", risk.Reason)
		fmt.Fprintf(os.Stderr, "SUGGESTION: %s\n", risk.Alternative)
	}

	// Execute command
	start := time.Now()
	cmd := exec.CommandContext(ctx, args[0], args[1:]...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	err := cmd.Run()
	duration := time.Since(start)

	exitCode := 0
	stderrPreview := ""
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		} else {
			exitCode = 1
			stderrPreview = err.Error()
		}
	}

	// Log to graph
	e.logToGraph(ctx, domain.Event{
		Command:       command,
		CmdBase:       args[0],
		ExitCode:      exitCode,
		DurationSec:   duration.Seconds(),
		Cwd:           getCwd(),
		StderrPreview: stderrPreview,
		Timestamp:     time.Now(),
		Project:       e.project,
		Type:          domain.ClassifyCommand(args[0]),
		IsConflict:    exitCode != 0,
	})

	return Result{
		ExitCode: exitCode,
		Duration: duration,
	}
}

// logToGraph logs the event to Memgraph.
func (e *Executor) logToGraph(ctx context.Context, event domain.Event) {
	if e.db == nil {
		return
	}

	labels := []string{"Event", "TerminalEvent"}
	if event.IsConflict {
		labels = append(labels, "Conflict")
	}

	switch event.Type {
	case domain.EventVCS:
		labels = append(labels, "VCSEvent")
	case domain.EventContainer:
		labels = append(labels, "ContainerEvent")
	case domain.EventBuild:
		labels = append(labels, "BuildEvent")
	case domain.EventTest:
		labels = append(labels, "TestEvent")
	}

	labelStr := strings.Join(labels, ":")

	query := fmt.Sprintf(`
		CREATE (e:%s {
			command: $command,
			cmd_base: $cmd_base,
			exit_code: $exit_code,
			duration_sec: $duration_sec,
			cwd: $cwd,
			stderr_preview: $stderr_preview,
			timestamp: $timestamp,
			datetime: $datetime,
			project: $project
		})
	`, labelStr)

	params := map[string]any{
		"command":        urpstrings.TruncateNoEllipsis(event.Command, 500),
		"cmd_base":       event.CmdBase,
		"exit_code":      event.ExitCode,
		"duration_sec":   event.DurationSec,
		"cwd":            event.Cwd,
		"stderr_preview": urpstrings.TruncateNoEllipsis(event.StderrPreview, 200),
		"timestamp":      event.Timestamp.Unix(),
		"datetime":       event.Timestamp.Format(time.RFC3339),
		"project":        event.Project,
	}

	_ = e.db.ExecuteWrite(ctx, query, params)
}

// getCwd delegates to urpstrings.GetCwd.
func getCwd() string {
	return urpstrings.GetCwd()
}

