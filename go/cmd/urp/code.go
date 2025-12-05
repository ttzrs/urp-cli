package main

import (
	"context"
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/joss/urp/internal/audit"
	"github.com/joss/urp/internal/ingest"
	"github.com/joss/urp/internal/query"
)

func codeCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "code",
		Short: "Code analysis commands",
		Long:  "Parse and analyze code (D, Φ, ⊆ primitives)",
	}

	// urp code ingest [path]
	ingestCmd := &cobra.Command{
		Use:   "ingest [path]",
		Short: "Parse code into graph",
		Long:  "Parse code into graph. If no path is provided, uses current directory.",
		Args:  cobra.MaximumNArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			event := auditLogger.Start(audit.CategoryCode, "ingest")

			requireDB(event)

			// Default to current directory
			path := "."
			if len(args) > 0 {
				path = args[0]
			}

			ingester := ingest.NewIngester(db)

			// Progress bar
			var lastLen int
			ingester.SetProgress(func(current, total int, file string) {
				// Clear previous line
				fmt.Printf("\r%s\r", strings.Repeat(" ", lastLen))

				// Progress bar
				pct := float64(current) / float64(total) * 100
				barWidth := 30
				filled := int(float64(barWidth) * float64(current) / float64(total))
				bar := strings.Repeat("█", filled) + strings.Repeat("░", barWidth-filled)

				// Truncate file if too long
				maxFile := 40
				if len(file) > maxFile {
					file = "..." + file[len(file)-maxFile+3:]
				}

				line := fmt.Sprintf("[%s] %3.0f%% (%d/%d) %s", bar, pct, current, total, file)
				lastLen = len(line)
				fmt.Print(line)
			})

			stats, err := ingester.Ingest(context.Background(), path)
			fmt.Println() // New line after progress

			if err != nil {
				exitOnError(event, err)
			}

			// Resolve call references to actual functions/methods
			fmt.Print("Linking call references...")
			if err := ingester.LinkCalls(context.Background()); err != nil {
				fmt.Println(" warning:", err)
			} else {
				fmt.Println(" done")
			}

			out, _ := prettyJSON(stats)
			event.OutputSize = len(out)
			auditLogger.LogSuccess(event)
			fmt.Println(string(out))
		},
	}

	// urp code deps <signature>
	var depth int
	depsCmd := &cobra.Command{
		Use:   "deps <signature>",
		Short: "Find dependencies of a function (Φ forward)",
		Args:  cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			event := auditLogger.Start(audit.CategoryCode, "deps")

			requireDB(event)

			q := query.NewQuerier(db)
			deps, err := q.FindDeps(context.Background(), args[0], depth)
			if err != nil {
				exitOnError(event, err)
			}

			out, _ := prettyJSON(deps)
			event.OutputSize = len(out)
			auditLogger.LogSuccess(event)
			fmt.Println(string(out))
		},
	}
	depsCmd.Flags().IntVarP(&depth, "depth", "d", 3, "Max depth")

	// urp code impact <signature>
	impactCmd := &cobra.Command{
		Use:   "impact <signature>",
		Short: "Find impact of changing a function (Φ inverse)",
		Args:  cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			event := auditLogger.Start(audit.CategoryCode, "impact")

			requireDB(event)

			q := query.NewQuerier(db)
			impacts, err := q.FindImpact(context.Background(), args[0], depth)
			if err != nil {
				exitOnError(event, err)
			}

			out, _ := prettyJSON(impacts)
			event.OutputSize = len(out)
			auditLogger.LogSuccess(event)
			fmt.Println(string(out))
		},
	}
	impactCmd.Flags().IntVarP(&depth, "depth", "d", 3, "Max depth")

	// urp code dead
	deadCmd := &cobra.Command{
		Use:   "dead",
		Short: "Find unused code (⊥ unused)",
		Run: func(cmd *cobra.Command, args []string) {
			event := auditLogger.Start(audit.CategoryCode, "dead")

			requireDB(event)

			q := query.NewQuerier(db)
			dead, err := q.FindDeadCode(context.Background())
			if err != nil {
				exitOnError(event, err)
			}

			out, _ := prettyJSON(dead)
			event.OutputSize = len(out)
			auditLogger.LogSuccess(event)
			fmt.Println(string(out))
		},
	}

	// urp code cycles
	cyclesCmd := &cobra.Command{
		Use:   "cycles",
		Short: "Find circular dependencies (⊥ conflict)",
		Run: func(cmd *cobra.Command, args []string) {
			event := auditLogger.Start(audit.CategoryCode, "cycles")

			requireDB(event)

			q := query.NewQuerier(db)
			cycles, err := q.FindCycles(context.Background())
			if err != nil {
				exitOnError(event, err)
			}

			out, _ := prettyJSON(cycles)
			event.OutputSize = len(out)
			auditLogger.LogSuccess(event)
			fmt.Println(string(out))
		},
	}

	// urp code hotspots
	var days int
	hotspotsCmd := &cobra.Command{
		Use:   "hotspots",
		Short: "Find high-churn areas (τ + Φ)",
		Run: func(cmd *cobra.Command, args []string) {
			event := auditLogger.Start(audit.CategoryCode, "hotspots")

			requireDB(event)

			q := query.NewQuerier(db)
			hotspots, err := q.FindHotspots(context.Background(), days)
			if err != nil {
				exitOnError(event, err)
			}

			out, _ := prettyJSON(hotspots)
			event.OutputSize = len(out)
			auditLogger.LogSuccess(event)
			fmt.Println(string(out))
		},
	}
	hotspotsCmd.Flags().IntVarP(&days, "days", "d", 30, "Look back N days")

	// urp code stats
	statsCmd := &cobra.Command{
		Use:   "stats",
		Short: "Show graph statistics",
		Run: func(cmd *cobra.Command, args []string) {
			event := auditLogger.Start(audit.CategoryCode, "stats")

			requireDB(event)

			q := query.NewQuerier(db)
			stats, err := q.GetStats(context.Background())
			if err != nil {
				exitOnError(event, err)
			}

			out, _ := prettyJSON(stats)
			event.OutputSize = len(out)
			auditLogger.LogSuccess(event)
			fmt.Println(string(out))
		},
	}

	cmd.AddCommand(ingestCmd, depsCmd, impactCmd, deadCmd, cyclesCmd, hotspotsCmd, statsCmd)
	return cmd
}
