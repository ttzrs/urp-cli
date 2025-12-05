// Package audit remediation action implementations.
package audit

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
)

// executeRetry signals that a retry is requested.
// Actual retry is handled by the caller.
func (h *Healer) executeRetry(ctx context.Context, anomaly *Anomaly) error {
	return nil
}

// executeRollback performs a git soft reset to previous commit.
func (h *Healer) executeRollback(ctx context.Context, anomaly *Anomaly) (string, error) {
	// Find last known good commit
	cmd := exec.CommandContext(ctx, "git", "rev-parse", "HEAD~1")
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("cannot find rollback target: %w", err)
	}

	target := strings.TrimSpace(string(out))

	// Soft reset to preserve working directory
	cmd = exec.CommandContext(ctx, "git", "reset", "--soft", target)
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("rollback failed: %w", err)
	}

	return target, nil
}

// executeRestart signals that a service restart is needed.
// Actual restart is operation-specific.
func (h *Healer) executeRestart(ctx context.Context, anomaly *Anomaly) error {
	return nil
}

// executeNotify logs a notification about the anomaly.
func (h *Healer) executeNotify(ctx context.Context, anomaly *Anomaly) error {
	fmt.Printf("[NOTIFY] Anomaly detected: %s - %s\n", anomaly.ID, anomaly.Description)
	return nil
}

// executeClearCache signals cache clearing is needed.
// Actual clearing depends on the specific cache implementation.
func (h *Healer) executeClearCache(ctx context.Context, anomaly *Anomaly) error {
	return nil
}

// executeEscalate triggers critical escalation alerts.
func (h *Healer) executeEscalate(ctx context.Context, anomaly *Anomaly) error {
	fmt.Printf("[ESCALATE] Critical anomaly: %s - %s (z-score: %.2f)\n",
		anomaly.ID, anomaly.Description, anomaly.ZScore)
	return nil
}

// executeKill terminates runaway processes.
// Requires explicit approval for safety.
func (h *Healer) executeKill(ctx context.Context, anomaly *Anomaly) error {
	return fmt.Errorf("kill action requires explicit approval")
}
