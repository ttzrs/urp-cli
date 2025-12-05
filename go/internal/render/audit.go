package render

import (
	"time"

	"github.com/joss/urp/internal/audit"
)

// Audit renders audit-specific output.
type Audit struct {
	*Writer
}

// NewAudit creates an Audit renderer writing to stdout.
func NewAudit() *Audit {
	return &Audit{Writer: Stdout()}
}

// Events renders a list of audit events.
func (a *Audit) Events(events []audit.AuditEvent) {
	if len(events) == 0 {
		a.Empty("No audit events found")
		return
	}

	a.Header("AUDIT LOG (%d events)", len(events))

	for _, e := range events {
		icon := StatusIcon(string(e.Status))
		line := a.formatEventLine(icon, &e)
		a.Println(line)

		if e.ErrorMessage != "" && e.Status == audit.StatusError {
			a.Nested("%s", Truncate(e.ErrorMessage, 70))
		}
	}
}

// Errors renders error events.
func (a *Audit) Errors(events []audit.AuditEvent) {
	if len(events) == 0 {
		a.Empty("No errors found")
		return
	}

	a.Header("RECENT ERRORS (%d)", len(events))

	for _, e := range events {
		a.Println("✗ [%s] %s/%s @ %s",
			e.StartedAt.Format("2006-01-02 15:04:05"),
			e.Category,
			e.Operation,
			e.Git.CommitShort,
		)
		if e.ErrorMessage != "" {
			a.Item("Error: %s", e.ErrorMessage)
		}
		if e.Command != "" {
			a.Item("Command: %s", Truncate(e.Command, 60))
		}
		a.Line()
	}
}

// Stats renders audit statistics.
func (a *Audit) Stats(stats *audit.Stats) {
	a.Header("AUDIT STATISTICS")

	a.Item("Total events:   %d", stats.Total)
	a.Item("Success:        %d", stats.Success)
	a.Item("Errors:         %d", stats.Errors)
	a.Item("Warnings:       %d", stats.Warnings)
	a.Item("Timeouts:       %d", stats.Timeouts)
	a.Line()

	if stats.AvgDurationMs > 0 {
		a.Item("Avg duration:   %.0fms", stats.AvgDurationMs)
	}
	if stats.MaxDurationMs > 0 {
		a.Item("Max duration:   %dms", stats.MaxDurationMs)
	}

	if len(stats.ByCategory) > 0 {
		a.Section("BY CATEGORY")
		for cat, cs := range stats.ByCategory {
			a.Item("%-12s %d total, %d errors", cat+":", cs.Total, cs.Errors)
		}
	}
}

// CommitEvents renders events for a specific commit.
func (a *Audit) CommitEvents(hash string, events []audit.AuditEvent) {
	if len(events) == 0 {
		a.Println("No events found for commit %s", hash)
		return
	}

	a.Header("EVENTS FOR COMMIT %s (%d)", hash, len(events))

	for _, e := range events {
		icon := "✓"
		if e.Status == audit.StatusError {
			icon = "✗"
		}
		a.Println("%s %s/%s (%dms)", icon, e.Category, e.Operation, e.DurationMs)
	}
}

// Metrics renders operation metrics.
func (a *Audit) Metrics(latency, errorRates map[audit.Category][]audit.MetricStats) {
	a.Header("LATENCY METRICS")

	for _, cat := range []audit.Category{audit.CategoryCode, audit.CategoryGit, audit.CategorySystem} {
		if stats, ok := latency[cat]; ok && len(stats) > 0 {
			a.Println("  %s:", cat)
			for _, s := range stats {
				a.SubItem("%-20s mean=%.0fms p95=%.0fms p99=%.0fms (n=%d)",
					s.Operation+":", s.Mean, s.P95, s.P99, s.Count)
			}
		}
	}

	a.Section("ERROR RATES")

	for _, cat := range []audit.Category{audit.CategoryCode, audit.CategoryGit, audit.CategorySystem} {
		if stats, ok := errorRates[cat]; ok && len(stats) > 0 {
			a.Println("  %s:", cat)
			for _, s := range stats {
				rate := 0.0
				if s.Count > 0 {
					rate = (s.Sum / float64(s.Count)) * 100
				}
				a.SubItem("%-20s %.1f%% (%d/%d)",
					s.Operation+":", rate, int(s.Sum), s.Count)
			}
		}
	}
}

// Anomalies renders detected anomalies.
func (a *Audit) Anomalies(anomalies []audit.Anomaly) {
	if len(anomalies) == 0 {
		a.Empty("No anomalies detected")
		return
	}

	a.Header("DETECTED ANOMALIES (%d)", len(anomalies))

	for _, an := range anomalies {
		icon := LevelIcon(string(an.Level))
		a.Println("%s [%s] %s/%s", icon, an.Level, an.Category, an.Operation)
		a.SubItem("%s", an.Description)
		if an.ZScore != 0 {
			a.SubItem("z-score: %.2f (value=%.2f expected=%.2f)",
				an.ZScore, an.Value, an.Expected)
		}
		a.SubItem("@ %s", an.DetectedAt.Format("2006-01-02 15:04:05"))
		a.Line()
	}
}

// Baselines renders computed baselines.
func (a *Audit) Baselines(baselines []audit.Baseline) {
	a.Header("COMPUTED BASELINES")

	for _, b := range baselines {
		a.Item("%s/%s/%s:", b.Type, b.Category, b.Operation)
		a.SubItem("mean=%.2f stddev=%.2f min=%.2f max=%.2f (n=%d)",
			b.Mean, b.StdDev, b.Min, b.Max, b.SampleSize)
	}
}

// HealDryRun renders dry-run heal preview.
func (a *Audit) HealDryRun(previews []HealPreview) {
	a.Println("DRY RUN - Would heal %d anomalies:", len(previews))
	a.Line()

	for _, p := range previews {
		status := "[" + p.Action + "]"
		if !p.CanHeal {
			status = "[blocked: " + p.Reason + "]"
		}
		a.Item("%-20s %s/%s - %s", status, p.Category, p.Operation, p.Description)
	}
}

// HealPreview represents a preview of a heal action.
type HealPreview struct {
	Category    string
	Operation   string
	Description string
	Action      string
	CanHeal     bool
	Reason      string
}

// HealResults renders healing results.
func (a *Audit) HealResults(results []audit.RemediationResult, total int) {
	a.Header("HEALING %d ANOMALIES", total)

	successCount := 0
	for _, r := range results {
		icon := BoolIcon(r.Success)
		if r.Success {
			successCount++
		}
		a.Println("%s [%s] %s", icon, r.Action, r.Message)
	}

	a.Line()
	a.Println("Healed %d/%d anomalies", successCount, len(results))
}

// HealHistory renders healing history.
func (a *Audit) HealHistory(results []audit.RemediationResult, stats map[string]any) {
	if len(results) == 0 {
		a.Empty("No healing history")
		return
	}

	a.Header("HEALING HISTORY (%d attempts)", len(results))

	for _, r := range results {
		icon := BoolIcon(r.Success)
		a.Println("%s [%s] %s @ %s (%dms)",
			icon, r.Action, r.Message,
			r.AttemptedAt.Format("15:04:05"),
			r.DurationMs,
		)
		if r.RollbackRef != "" && len(r.RollbackRef) >= 7 {
			a.Nested("rollback: %s", r.RollbackRef[:7])
		}
	}

	if stats != nil {
		a.Line()
		a.Println("Total: %v  Success: %v  Failed: %v",
			stats["total"], stats["success"], stats["failed"])
	}
}

// Rules renders remediation rules.
func (a *Audit) Rules(rules []RuleInfo) {
	a.Header("REMEDIATION RULES")

	for _, r := range rules {
		a.Item("%-28s [%s]", r.Name, r.Action)
		a.SubItem("%s", r.Description)
		a.Line()
	}
}

// RuleInfo describes a remediation rule.
type RuleInfo struct {
	Name        string
	Action      string
	Description string
}

// DefaultRules returns the default rule descriptions.
func DefaultRules() []RuleInfo {
	return []RuleInfo{
		{"high-latency-retry", "retry", "Retry operations with high latency"},
		{"critical-latency-escalate", "escalate", "Escalate critical latency issues"},
		{"code-error-rollback", "rollback", "Rollback code changes on persistent errors"},
		{"git-error-notify", "notify", "Notify on git operation failures"},
		{"system-error-restart", "restart", "Restart system services on critical failures"},
		{"large-output-clear", "clear_cache", "Clear cache when output sizes spike"},
	}
}

func (a *Audit) formatEventLine(icon string, e *audit.AuditEvent) string {
	line := icon + " [" + e.StartedAt.Format("15:04:05") + "] " +
		string(e.Category) + "/" + e.Operation

	if e.DurationMs > 0 {
		line += " (" + formatDuration(time.Duration(e.DurationMs)*time.Millisecond) + ")"
	}

	if e.Git.CommitShort != "" {
		line += " @ " + e.Git.CommitShort
	}

	return line
}

func formatDuration(d time.Duration) string {
	if d < time.Second {
		return d.Round(time.Millisecond).String()
	}
	return d.Round(time.Second).String()
}
