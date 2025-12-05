package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"

	"github.com/joss/urp/internal/alerts"
	"github.com/joss/urp/internal/audit"
	"github.com/joss/urp/internal/memory"
)

// checkDB exits if the database is not connected.
func checkDB() {
	if db == nil {
		fmt.Fprintln(os.Stderr, "Error: Not connected to graph database")
		os.Exit(1)
	}
}

func auditCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "audit",
		Short: "Audit logging and analysis",
		Long: `Query and analyze audit events from operations.

Audit events capture all urp operations with git context,
timing, and status information for debugging and analysis.`,
	}

	// urp audit log [--category CAT] [--status STATUS] [--limit N]
	var category, status string
	var limit int
	logCmd := &cobra.Command{
		Use:   "log",
		Short: "Show audit log",
		Long: `Display recent audit events with filters.

Examples:
  urp audit log                    # Show recent events
  urp audit log --category code    # Show code operations
  urp audit log --status error     # Show errors only
  urp audit log --limit 50         # Show last 50 events`,
		Run: func(cmd *cobra.Command, args []string) {
			checkDB()

			sessCtx := memory.NewContext()
			store := audit.NewStore(db, sessCtx.SessionID)
			filter := audit.QueryFilter{
				Category: audit.Category(category),
				Status:   audit.Status(status),
				Limit:    limit,
			}

			events, err := store.Query(context.Background(), filter)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error: %v\n", err)
				os.Exit(1)
			}

			if len(events) == 0 {
				fmt.Println("No audit events found")
				return
			}

			fmt.Printf("AUDIT LOG (%d events)\n", len(events))
			fmt.Println()

			for _, e := range events {
				statusIcon := "•"
				switch e.Status {
				case audit.StatusSuccess:
					statusIcon = "✓"
				case audit.StatusError:
					statusIcon = "✗"
				case audit.StatusWarning:
					statusIcon = "!"
				case audit.StatusTimeout:
					statusIcon = "⏱"
				}

				// Format: [status] category/operation (duration) @ commit
				line := fmt.Sprintf("%s [%s] %s/%s",
					statusIcon,
					e.StartedAt.Format("15:04:05"),
					e.Category,
					e.Operation,
				)

				if e.DurationMs > 0 {
					line += fmt.Sprintf(" (%dms)", e.DurationMs)
				}

				if e.Git.CommitShort != "" {
					line += fmt.Sprintf(" @ %s", e.Git.CommitShort)
				}

				fmt.Println(line)

				if e.ErrorMessage != "" && e.Status == audit.StatusError {
					fmt.Printf("    └─ %s\n", truncateStr(e.ErrorMessage, 70))
				}
			}
		},
	}
	logCmd.Flags().StringVarP(&category, "category", "c", "", "Filter by category (code, git, events, etc)")
	logCmd.Flags().StringVarP(&status, "status", "s", "", "Filter by status (success, error, warning, timeout)")
	logCmd.Flags().IntVarP(&limit, "limit", "n", 20, "Number of events to show")

	// urp audit errors
	errorsCmd := &cobra.Command{
		Use:   "errors",
		Short: "Show recent errors",
		Run: func(cmd *cobra.Command, args []string) {
			checkDB()

			sessCtx := memory.NewContext()
			store := audit.NewStore(db, sessCtx.SessionID)
			events, err := store.GetErrors(context.Background(), 20)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error: %v\n", err)
				os.Exit(1)
			}

			if len(events) == 0 {
				fmt.Println("No errors found")
				return
			}

			fmt.Printf("RECENT ERRORS (%d)\n", len(events))
			fmt.Println()

			for _, e := range events {
				fmt.Printf("✗ [%s] %s/%s @ %s\n",
					e.StartedAt.Format("2006-01-02 15:04:05"),
					e.Category,
					e.Operation,
					e.Git.CommitShort,
				)
				if e.ErrorMessage != "" {
					fmt.Printf("  Error: %s\n", e.ErrorMessage)
				}
				if e.Command != "" {
					fmt.Printf("  Command: %s\n", truncateStr(e.Command, 60))
				}
				fmt.Println()
			}
		},
	}

	// urp audit stats
	statsCmd := &cobra.Command{
		Use:   "stats",
		Short: "Show audit statistics",
		Run: func(cmd *cobra.Command, args []string) {
			checkDB()

			sessCtx := memory.NewContext()
			store := audit.NewStore(db, sessCtx.SessionID)

			// Get overall stats
			stats, err := store.GetStats(context.Background())
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error: %v\n", err)
				os.Exit(1)
			}

			fmt.Println("AUDIT STATISTICS")
			fmt.Println()
			fmt.Printf("  Total events:   %v\n", stats["total"])
			fmt.Printf("  Success:        %v\n", stats["success"])
			fmt.Printf("  Errors:         %v\n", stats["errors"])
			fmt.Printf("  Warnings:       %v\n", stats["warnings"])
			fmt.Printf("  Timeouts:       %v\n", stats["timeouts"])
			fmt.Println()

			if avg, ok := stats["avg_duration_ms"].(float64); ok && avg > 0 {
				fmt.Printf("  Avg duration:   %.0fms\n", avg)
			}
			if max, ok := stats["max_duration_ms"].(int64); ok && max > 0 {
				fmt.Printf("  Max duration:   %dms\n", max)
			}

			// Get stats by category
			fmt.Println()
			fmt.Println("BY CATEGORY:")

			catStats, err := store.GetStatsByCategory(context.Background())
			if err == nil && len(catStats) > 0 {
				for cat, cs := range catStats {
					total := cs["total"]
					errors := cs["errors"]
					fmt.Printf("  %-12s %v total, %v errors\n", cat+":", total, errors)
				}
			}
		},
	}

	// urp audit commit <hash>
	commitCmd := &cobra.Command{
		Use:   "commit <hash>",
		Short: "Show events for a commit",
		Args:  cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			checkDB()

			sessCtx := memory.NewContext()
			store := audit.NewStore(db, sessCtx.SessionID)
			events, err := store.GetEventsByCommit(context.Background(), args[0])
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error: %v\n", err)
				os.Exit(1)
			}

			if len(events) == 0 {
				fmt.Printf("No events found for commit %s\n", args[0])
				return
			}

			fmt.Printf("EVENTS FOR COMMIT %s (%d)\n", args[0], len(events))
			fmt.Println()

			for _, e := range events {
				statusIcon := "✓"
				if e.Status == audit.StatusError {
					statusIcon = "✗"
				}
				fmt.Printf("%s %s/%s (%dms)\n",
					statusIcon,
					e.Category,
					e.Operation,
					e.DurationMs,
				)
			}
		},
	}

	// urp audit metrics
	metricsCmd := &cobra.Command{
		Use:   "metrics",
		Short: "Show operation metrics",
		Long: `Display metrics statistics for operations.

Metrics include latency, error rates, and output sizes
aggregated across operations.`,
		Run: func(cmd *cobra.Command, args []string) {
			checkDB()

			sessCtx := memory.NewContext()
			metricsStore := audit.NewMetricsStore(db, sessCtx.SessionID)

			// Show latency stats
			fmt.Println("LATENCY METRICS")
			fmt.Println()

			for _, cat := range []audit.Category{audit.CategoryCode, audit.CategoryGit, audit.CategorySystem} {
				stats, err := metricsStore.GetHistoricalStats(context.Background(), audit.MetricLatency, cat, 10)
				if err != nil || len(stats) == 0 {
					continue
				}

				fmt.Printf("  %s:\n", cat)
				for _, s := range stats {
					fmt.Printf("    %-20s mean=%.0fms p95=%.0fms p99=%.0fms (n=%d)\n",
						s.Operation+":",
						s.Mean, s.P95, s.P99, s.Count)
				}
			}

			// Show error rate stats
			fmt.Println()
			fmt.Println("ERROR RATES")
			fmt.Println()

			for _, cat := range []audit.Category{audit.CategoryCode, audit.CategoryGit, audit.CategorySystem} {
				stats, err := metricsStore.GetHistoricalStats(context.Background(), audit.MetricErrorRate, cat, 10)
				if err != nil || len(stats) == 0 {
					continue
				}

				fmt.Printf("  %s:\n", cat)
				for _, s := range stats {
					rate := 0.0
					if s.Count > 0 {
						rate = (s.Sum / float64(s.Count)) * 100
					}
					fmt.Printf("    %-20s %.1f%% (%d/%d)\n",
						s.Operation+":",
						rate, int(s.Sum), s.Count)
				}
			}
		},
	}

	// urp audit anomalies [--level LEVEL]
	var anomalyLevel string
	anomaliesCmd := &cobra.Command{
		Use:   "anomalies",
		Short: "Show detected anomalies",
		Long: `Display anomalies detected in operation metrics.

Anomaly levels:
  low      - Minor deviation (1.5-2 sigma)
  medium   - Moderate deviation (2-3 sigma)
  high     - Significant deviation (3+ sigma)
  critical - Threshold breach or extreme deviation (4+ sigma)`,
		Run: func(cmd *cobra.Command, args []string) {
			checkDB()

			sessCtx := memory.NewContext()
			anomalyStore := audit.NewAnomalyStore(db, sessCtx.SessionID)

			var anomalies []audit.Anomaly
			var err error

			if anomalyLevel != "" {
				anomalies, err = anomalyStore.GetByLevel(context.Background(), audit.AnomalyLevel(anomalyLevel), 50)
			} else {
				anomalies, err = anomalyStore.GetRecent(context.Background(), 50)
			}

			if err != nil {
				fmt.Fprintf(os.Stderr, "Error: %v\n", err)
				os.Exit(1)
			}

			if len(anomalies) == 0 {
				fmt.Println("No anomalies detected")
				return
			}

			fmt.Printf("DETECTED ANOMALIES (%d)\n", len(anomalies))
			fmt.Println()

			for _, a := range anomalies {
				levelIcon := "•"
				switch a.Level {
				case audit.AnomalyLow:
					levelIcon = "○"
				case audit.AnomalyMedium:
					levelIcon = "◐"
				case audit.AnomalyHigh:
					levelIcon = "●"
				case audit.AnomalyCritical:
					levelIcon = "◉"
				}

				fmt.Printf("%s [%s] %s/%s\n",
					levelIcon,
					a.Level,
					a.Category,
					a.Operation,
				)
				fmt.Printf("    %s\n", a.Description)
				if a.ZScore != 0 {
					fmt.Printf("    z-score: %.2f (value=%.2f expected=%.2f)\n",
						a.ZScore, a.Value, a.Expected)
				}
				fmt.Printf("    @ %s\n", a.DetectedAt.Format("2006-01-02 15:04:05"))
				fmt.Println()
			}
		},
	}
	anomaliesCmd.Flags().StringVarP(&anomalyLevel, "level", "l", "", "Filter by level (low, medium, high, critical)")

	// urp audit baseline [--compute]
	var computeBaseline bool
	baselineCmd := &cobra.Command{
		Use:   "baseline",
		Short: "Show or compute baselines",
		Long: `Display operation baselines used for anomaly detection.

Use --compute to calculate new baselines from recent metrics.`,
		Run: func(cmd *cobra.Command, args []string) {
			checkDB()

			sessCtx := memory.NewContext()

			if computeBaseline {
				fmt.Println("Computing baselines from metrics...")

				// Create collector and populate from recent events
				collector := audit.NewMetricsCollector(24*time.Hour, 10000)

				// Get recent events to populate collector
				store := audit.NewStore(db, sessCtx.SessionID)
				events, err := store.Query(context.Background(), audit.QueryFilter{Limit: 1000})
				if err != nil {
					fmt.Fprintf(os.Stderr, "Error: %v\n", err)
					os.Exit(1)
				}

				for i := range events {
					collector.Record(&events[i])
				}

				// Compute and show baselines
				detector := audit.NewAnomalyDetector(collector, audit.DefaultThresholds)
				allStats := collector.GetAllStats()

				fmt.Println()
				fmt.Println("COMPUTED BASELINES")
				fmt.Println()

				for _, stats := range allStats {
					if stats.Count < 10 {
						continue
					}

					baseline := detector.ComputeBaseline(stats.Type, stats.Category, stats.Operation)
					if baseline == nil {
						continue
					}

					fmt.Printf("  %s/%s/%s:\n", stats.Type, stats.Category, stats.Operation)
					fmt.Printf("    mean=%.2f stddev=%.2f min=%.2f max=%.2f (n=%d)\n",
						baseline.Mean, baseline.StdDev, baseline.Min, baseline.Max, baseline.SampleSize)
				}

				return
			}

			// Show existing baselines from graph
			metricsStore := audit.NewMetricsStore(db, sessCtx.SessionID)

			fmt.Println("STORED BASELINES")
			fmt.Println()

			for _, metricType := range []audit.MetricType{audit.MetricLatency, audit.MetricErrorRate, audit.MetricOutputSize} {
				fmt.Printf("%s:\n", metricType)

				for _, cat := range []audit.Category{audit.CategoryCode, audit.CategoryGit, audit.CategorySystem} {
					stats, err := metricsStore.GetHistoricalStats(context.Background(), metricType, cat, 5)
					if err != nil || len(stats) == 0 {
						continue
					}

					for _, s := range stats {
						fmt.Printf("  %s/%-15s mean=%.2f stddev=%.2f (n=%d)\n",
							cat, s.Operation+":", s.Mean, s.StdDev, s.Count)
					}
				}
				fmt.Println()
			}
		},
	}
	baselineCmd.Flags().BoolVar(&computeBaseline, "compute", false, "Compute new baselines from recent metrics")

	// urp audit heal [--dry-run] [--level LEVEL]
	var healDryRun bool
	var healLevel string
	healCmd := &cobra.Command{
		Use:   "heal",
		Short: "Auto-heal detected anomalies",
		Long: `Attempt to remediate detected anomalies automatically.

Remediation actions:
  retry      - Retry the failed operation
  rollback   - Rollback to previous git state
  restart    - Restart affected service
  notify     - Send notification (no auto-fix)
  escalate   - Escalate to critical alert
  clear_cache - Clear relevant caches
  skip       - Skip (no action)

Use --dry-run to see what would be done without executing.`,
		Run: func(cmd *cobra.Command, args []string) {
			checkDB()

			sessCtx := memory.NewContext()
			anomalyStore := audit.NewAnomalyStore(db, sessCtx.SessionID)
			healingStore := audit.NewHealingStore(db, sessCtx.SessionID)
			healer := audit.NewHealer()

			// Get anomalies to heal
			var anomalies []audit.Anomaly
			var err error

			if healLevel != "" {
				anomalies, err = anomalyStore.GetByLevel(context.Background(), audit.AnomalyLevel(healLevel), 20)
			} else {
				anomalies, err = anomalyStore.GetRecent(context.Background(), 20)
			}

			if err != nil {
				fmt.Fprintf(os.Stderr, "Error: %v\n", err)
				os.Exit(1)
			}

			if len(anomalies) == 0 {
				fmt.Println("No anomalies to heal")
				return
			}

			if healDryRun {
				fmt.Printf("DRY RUN - Would heal %d anomalies:\n\n", len(anomalies))

				for _, a := range anomalies {
					rule := healer.FindRule(&a)
					if rule == nil {
						fmt.Printf("  %-10s %s/%s - no matching rule\n",
							"[skip]", a.Category, a.Operation)
						continue
					}

					canHeal, reason := healer.CanHeal(&a, rule)
					status := fmt.Sprintf("[%s]", rule.Action)
					if !canHeal {
						status = "[blocked: " + reason + "]"
					}

					fmt.Printf("  %-20s %s/%s - %s\n",
						status, a.Category, a.Operation, a.Description)
				}
				return
			}

			// Execute healing
			fmt.Printf("HEALING %d ANOMALIES\n\n", len(anomalies))

			results := healer.HealAll(context.Background(), anomalies)

			successCount := 0
			for _, r := range results {
				icon := "✗"
				if r.Success {
					icon = "✓"
					successCount++
				}

				fmt.Printf("%s [%s] %s\n", icon, r.Action, r.Message)

				// Persist result
				if err := healingStore.Save(context.Background(), &r); err != nil {
					fmt.Fprintf(os.Stderr, "  Warning: failed to save result: %v\n", err)
				}
			}

			fmt.Printf("\nHealed %d/%d anomalies\n", successCount, len(results))
		},
	}
	healCmd.Flags().BoolVar(&healDryRun, "dry-run", false, "Show what would be done without executing")
	healCmd.Flags().StringVarP(&healLevel, "level", "l", "", "Only heal anomalies of this level")

	// urp audit history
	historyCmd := &cobra.Command{
		Use:   "history",
		Short: "Show healing history",
		Run: func(cmd *cobra.Command, args []string) {
			checkDB()

			sessCtx := memory.NewContext()
			healingStore := audit.NewHealingStore(db, sessCtx.SessionID)

			results, err := healingStore.GetRecent(context.Background(), 30)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error: %v\n", err)
				os.Exit(1)
			}

			if len(results) == 0 {
				fmt.Println("No healing history")
				return
			}

			fmt.Printf("HEALING HISTORY (%d attempts)\n\n", len(results))

			for _, r := range results {
				icon := "✗"
				if r.Success {
					icon = "✓"
				}

				fmt.Printf("%s [%s] %s @ %s (%dms)\n",
					icon,
					r.Action,
					r.Message,
					r.AttemptedAt.Format("15:04:05"),
					r.DurationMs,
				)

				if r.RollbackRef != "" {
					fmt.Printf("    └─ rollback: %s\n", r.RollbackRef[:7])
				}
			}

			// Show stats
			stats, err := healingStore.GetStats(context.Background())
			if err == nil {
				fmt.Println()
				fmt.Printf("Total: %v  Success: %v  Failed: %v\n",
					stats["total"], stats["success"], stats["failed"])
			}
		},
	}

	// urp audit rules
	rulesCmd := &cobra.Command{
		Use:   "rules",
		Short: "Show remediation rules",
		Run: func(cmd *cobra.Command, args []string) {
			healer := audit.NewHealer()

			fmt.Println("REMEDIATION RULES")
			fmt.Println()

			// Access rules via reflection isn't ideal, but we can describe defaults
			rules := []struct {
				name   string
				action string
				desc   string
			}{
				{"high-latency-retry", "retry", "Retry operations with high latency"},
				{"critical-latency-escalate", "escalate", "Escalate critical latency issues"},
				{"code-error-rollback", "rollback", "Rollback code changes on persistent errors"},
				{"git-error-notify", "notify", "Notify on git operation failures"},
				{"system-error-restart", "restart", "Restart system services on critical failures"},
				{"large-output-clear", "clear_cache", "Clear cache when output sizes spike"},
			}

			for _, r := range rules {
				fmt.Printf("  %-28s [%s]\n", r.name, r.action)
				fmt.Printf("    %s\n\n", r.desc)
			}

			_ = healer // Keep reference
		},
	}

	cmd.AddCommand(logCmd, errorsCmd, statsCmd, commitCmd, metricsCmd, anomaliesCmd, baselineCmd, healCmd, historyCmd, rulesCmd)
	return cmd
}

// alertCmd provides commands for sending and managing system alerts
func alertCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "alert",
		Short: "System alert commands",
		Long:  "Send and manage alerts that Claude receives via hooks",
	}

	// urp alert send <level> <component> <title> <message>
	sendCmd := &cobra.Command{
		Use:   "send <level> <component> <title> <message>",
		Short: "Send a system alert",
		Long: `Send an alert that will be injected into Claude's context.

Levels: info, warning, error, critical

Examples:
  urp alert send error worker "Worker Crashed" "Worker-1 exited with code 137"
  urp alert send critical container "OOM Kill" "Container ran out of memory"`,
		Args: cobra.ExactArgs(4),
		Run: func(cmd *cobra.Command, args []string) {
			level := alerts.Level(args[0])
			component := args[1]
			title := args[2]
			message := args[3]

			ctx := make(map[string]interface{})
			if ctxFlag, _ := cmd.Flags().GetString("context"); ctxFlag != "" {
				json.Unmarshal([]byte(ctxFlag), &ctx)
			}

			var alert *alerts.Alert
			switch level {
			case alerts.LevelInfo:
				alert = alerts.Info(component, title, message)
			case alerts.LevelWarning:
				alert = alerts.Warning(component, title, message)
			case alerts.LevelError:
				alert = alerts.Error(component, title, message, ctx)
			case alerts.LevelCritical:
				alert = alerts.Critical(component, title, message, ctx)
			default:
				fmt.Fprintf(os.Stderr, "Invalid level: %s (use info, warning, error, critical)\n", level)
				os.Exit(1)
			}

			fmt.Printf("Alert sent: %s\n", alert.ID)
			fmt.Printf("  Level: %s\n", alert.Level)
			fmt.Printf("  Component: %s\n", alert.Component)
			fmt.Printf("  Title: %s\n", alert.Title)
		},
	}
	sendCmd.Flags().String("context", "", "JSON context data")

	// urp alert list
	listCmd := &cobra.Command{
		Use:   "list",
		Short: "List active alerts",
		Run: func(cmd *cobra.Command, args []string) {
			active := alerts.Global().GetActive()
			if len(active) == 0 {
				fmt.Println("No active alerts")
				return
			}

			fmt.Printf("%d active alert(s):\n\n", len(active))
			for _, a := range active {
				icon := "i"
				switch a.Level {
				case alerts.LevelWarning:
					icon = "!"
				case alerts.LevelError:
					icon = "X"
				case alerts.LevelCritical:
					icon = "!!"
				}
				fmt.Printf("[%s] %s: %s\n", icon, a.Component, a.Title)
				fmt.Printf("    %s\n", a.Message)
				fmt.Printf("    ID: %s\n\n", a.ID)
			}
		},
	}

	// urp alert resolve <id>
	resolveCmd := &cobra.Command{
		Use:   "resolve <alert-id>",
		Short: "Resolve an alert",
		Args:  cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			alerts.Resolve(args[0])
			fmt.Printf("Resolved: %s\n", args[0])
		},
	}

	// urp alert dir
	dirCmd := &cobra.Command{
		Use:   "dir",
		Short: "Show alert directory path",
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Println(alerts.GetAlertDir())
		},
	}

	cmd.AddCommand(sendCmd, listCmd, resolveCmd, dirCmd)
	return cmd
}
