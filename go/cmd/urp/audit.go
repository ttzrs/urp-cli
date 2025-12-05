package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"

	"github.com/joss/urp/internal/audit"
	"github.com/joss/urp/internal/memory"
	"github.com/joss/urp/internal/render"
)

func auditCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "audit",
		Short: "Audit logging and analysis",
		Long: `Query and analyze audit events from operations.

Audit events capture all urp operations with git context,
timing, and status information for debugging and analysis.`,
	}

	cmd.AddCommand(
		auditLogCmd(),
		auditErrorsCmd(),
		auditStatsCmd(),
		auditCommitCmd(),
		auditMetricsCmd(),
		auditAnomaliesCmd(),
		auditBaselineCmd(),
		auditHealCmd(),
		auditHistoryCmd(),
		auditRulesCmd(),
	)
	return cmd
}

// newAuditService creates an audit service with current session context.
func newAuditService() *audit.Service {
	sessCtx := memory.NewContext()
	store := audit.NewStore(db, sessCtx.SessionID)
	return audit.NewService(store, nil)
}

func auditLogCmd() *cobra.Command {
	var category, status string
	var limit int

	cmd := &cobra.Command{
		Use:   "log",
		Short: "Show audit log",
		Long: `Display recent audit events with filters.

Examples:
  urp audit log                    # Show recent events
  urp audit log --category code    # Show code operations
  urp audit log --status error     # Show errors only
  urp audit log --limit 50         # Show last 50 events`,
		Run: func(cmd *cobra.Command, args []string) {
			requireDBSimple()

			sessCtx := memory.NewContext()
			store := audit.NewStore(db, sessCtx.SessionID)
			filter := audit.QueryFilter{
				Category: audit.Category(category),
				Status:   audit.Status(status),
				Limit:    limit,
			}

			events, err := store.Query(context.Background(), filter)
			if err != nil {
				fatalError(err)
			}

			r := render.NewAudit()
			r.Events(events)
		},
	}
	cmd.Flags().StringVarP(&category, "category", "c", "", "Filter by category (code, git, events, etc)")
	cmd.Flags().StringVarP(&status, "status", "s", "", "Filter by status (success, error, warning, timeout)")
	cmd.Flags().IntVarP(&limit, "limit", "n", 20, "Number of events to show")

	return cmd
}

func auditErrorsCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "errors",
		Short: "Show recent errors",
		Run: func(cmd *cobra.Command, args []string) {
			requireDBSimple()

			svc := newAuditService()
			events, err := svc.GetErrors(context.Background(), time.Time{}, "", 20)
			if err != nil {
				fatalError(err)
			}

			r := render.NewAudit()
			r.Errors(events)
		},
	}
}

func auditStatsCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "stats",
		Short: "Show audit statistics",
		Run: func(cmd *cobra.Command, args []string) {
			requireDBSimple()

			svc := newAuditService()
			stats, err := svc.GetStats(context.Background())
			if err != nil {
				fatalError(err)
			}

			r := render.NewAudit()
			r.Stats(stats)
		},
	}
}

func auditCommitCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "commit <hash>",
		Short: "Show events for a commit",
		Args:  cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			requireDBSimple()

			svc := newAuditService()
			events, err := svc.GetEventsByCommit(context.Background(), args[0])
			if err != nil {
				fatalError(err)
			}

			r := render.NewAudit()
			r.CommitEvents(args[0], events)
		},
	}
}

func auditMetricsCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "metrics",
		Short: "Show operation metrics",
		Long: `Display metrics statistics for operations.

Metrics include latency, error rates, and output sizes
aggregated across operations.`,
		Run: func(cmd *cobra.Command, args []string) {
			requireDBSimple()

			sessCtx := memory.NewContext()
			metricsStore := audit.NewMetricsStore(db, sessCtx.SessionID)
			ctx := context.Background()

			// Collect latency stats by category
			latency := make(map[audit.Category][]audit.MetricStats)
			errorRates := make(map[audit.Category][]audit.MetricStats)

			for _, cat := range []audit.Category{audit.CategoryCode, audit.CategoryGit, audit.CategorySystem} {
				if stats, err := metricsStore.GetHistoricalStats(ctx, audit.MetricLatency, cat, 10); err == nil && len(stats) > 0 {
					latency[cat] = stats
				}
				if stats, err := metricsStore.GetHistoricalStats(ctx, audit.MetricErrorRate, cat, 10); err == nil && len(stats) > 0 {
					errorRates[cat] = stats
				}
			}

			r := render.NewAudit()
			r.Metrics(latency, errorRates)
		},
	}
}

func auditAnomaliesCmd() *cobra.Command {
	var anomalyLevel string

	cmd := &cobra.Command{
		Use:   "anomalies",
		Short: "Show detected anomalies",
		Long: `Display anomalies detected in operation metrics.

Anomaly levels:
  low      - Minor deviation (1.5-2 sigma)
  medium   - Moderate deviation (2-3 sigma)
  high     - Significant deviation (3+ sigma)
  critical - Threshold breach or extreme deviation (4+ sigma)`,
		Run: func(cmd *cobra.Command, args []string) {
			requireDBSimple()

			sessCtx := memory.NewContext()
			anomalyStore := audit.NewAnomalyStore(db, sessCtx.SessionID)
			ctx := context.Background()

			var anomalies []audit.Anomaly
			var err error

			if anomalyLevel != "" {
				anomalies, err = anomalyStore.GetByLevel(ctx, audit.AnomalyLevel(anomalyLevel), 50)
			} else {
				anomalies, err = anomalyStore.GetRecent(ctx, 50)
			}

			if err != nil {
				fatalError(err)
			}

			r := render.NewAudit()
			r.Anomalies(anomalies)
		},
	}
	cmd.Flags().StringVarP(&anomalyLevel, "level", "l", "", "Filter by level (low, medium, high, critical)")

	return cmd
}

func auditBaselineCmd() *cobra.Command {
	var computeBaseline bool

	cmd := &cobra.Command{
		Use:   "baseline",
		Short: "Show or compute baselines",
		Long: `Display operation baselines used for anomaly detection.

Use --compute to calculate new baselines from recent metrics.`,
		Run: func(cmd *cobra.Command, args []string) {
			requireDBSimple()

			sessCtx := memory.NewContext()
			ctx := context.Background()

			if computeBaseline {
				fmt.Println("Computing baselines from metrics...")

				collector := audit.NewMetricsCollector(24*time.Hour, 10000)
				store := audit.NewStore(db, sessCtx.SessionID)
				events, err := store.Query(ctx, audit.QueryFilter{Limit: 1000})
				if err != nil {
					fmt.Fprintf(os.Stderr, "Error: %v\n", err)
					os.Exit(1)
				}

				for i := range events {
					collector.Record(&events[i])
				}

				detector := audit.NewAnomalyDetector(collector, audit.DefaultThresholds)
				allStats := collector.GetAllStats()

				var baselines []audit.Baseline
				for _, stats := range allStats {
					if stats.Count < 10 {
						continue
					}
					baseline := detector.ComputeBaseline(stats.Type, stats.Category, stats.Operation)
					if baseline != nil {
						baselines = append(baselines, *baseline)
					}
				}

				r := render.NewAudit()
				r.Baselines(baselines)
				return
			}

			// Show existing baselines
			metricsStore := audit.NewMetricsStore(db, sessCtx.SessionID)
			w := render.Stdout()
			w.Header("STORED BASELINES")

			for _, metricType := range []audit.MetricType{audit.MetricLatency, audit.MetricErrorRate, audit.MetricOutputSize} {
				w.Println("%s:", metricType)
				for _, cat := range []audit.Category{audit.CategoryCode, audit.CategoryGit, audit.CategorySystem} {
					stats, err := metricsStore.GetHistoricalStats(ctx, metricType, cat, 5)
					if err != nil || len(stats) == 0 {
						continue
					}
					for _, s := range stats {
						w.Item("%s/%-15s mean=%.2f stddev=%.2f (n=%d)", cat, s.Operation+":", s.Mean, s.StdDev, s.Count)
					}
				}
				w.Line()
			}
		},
	}
	cmd.Flags().BoolVar(&computeBaseline, "compute", false, "Compute new baselines from recent metrics")

	return cmd
}

func auditHealCmd() *cobra.Command {
	var healDryRun bool
	var healLevel string

	cmd := &cobra.Command{
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
			requireDBSimple()

			sessCtx := memory.NewContext()
			ctx := context.Background()
			anomalyStore := audit.NewAnomalyStore(db, sessCtx.SessionID)
			healingStore := audit.NewHealingStore(db, sessCtx.SessionID)
			healer := audit.NewHealer()

			var anomalies []audit.Anomaly
			var err error

			if healLevel != "" {
				anomalies, err = anomalyStore.GetByLevel(ctx, audit.AnomalyLevel(healLevel), 20)
			} else {
				anomalies, err = anomalyStore.GetRecent(ctx, 20)
			}

			if err != nil {
				fatalError(err)
			}

			if len(anomalies) == 0 {
				fmt.Println("No anomalies to heal")
				return
			}

			r := render.NewAudit()

			if healDryRun {
				var previews []render.HealPreview
				for _, a := range anomalies {
					rule := healer.FindRule(&a)
					p := render.HealPreview{
						Category:    string(a.Category),
						Operation:   a.Operation,
						Description: a.Description,
					}
					if rule == nil {
						p.Action = "skip"
						p.CanHeal = false
						p.Reason = "no matching rule"
					} else {
						p.Action = string(rule.Action)
						p.CanHeal, p.Reason = healer.CanHeal(&a, rule)
					}
					previews = append(previews, p)
				}
				r.HealDryRun(previews)
				return
			}

			results := healer.HealAll(ctx, anomalies)
			for _, res := range results {
				if err := healingStore.Save(ctx, &res); err != nil {
					fmt.Fprintf(os.Stderr, "  Warning: failed to save result: %v\n", err)
				}
			}
			r.HealResults(results, len(anomalies))
		},
	}
	cmd.Flags().BoolVar(&healDryRun, "dry-run", false, "Show what would be done without executing")
	cmd.Flags().StringVarP(&healLevel, "level", "l", "", "Only heal anomalies of this level")

	return cmd
}

func auditHistoryCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "history",
		Short: "Show healing history",
		Run: func(cmd *cobra.Command, args []string) {
			requireDBSimple()

			sessCtx := memory.NewContext()
			ctx := context.Background()
			healingStore := audit.NewHealingStore(db, sessCtx.SessionID)

			results, err := healingStore.GetRecent(ctx, 30)
			if err != nil {
				fatalError(err)
			}

			stats, _ := healingStore.GetStats(ctx)
			r := render.NewAudit()
			r.HealHistory(results, stats)
		},
	}
}

func auditRulesCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "rules",
		Short: "Show remediation rules",
		Run: func(cmd *cobra.Command, args []string) {
			r := render.NewAudit()
			r.Rules(render.DefaultRules())
		},
	}
}
