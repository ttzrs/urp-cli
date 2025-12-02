// Package main provides the URP CLI entrypoint.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/joss/urp/internal/cognitive"
	"github.com/joss/urp/internal/graph"
	"github.com/joss/urp/internal/ingest"
	"github.com/joss/urp/internal/query"
	"github.com/joss/urp/internal/render"
	"github.com/joss/urp/internal/runner"
)

var (
	version = "0.1.0"
	db      graph.Driver
	pretty  = true
)

func main() {
	// Wire up environment lookup
	graph.SetEnvLookup(os.LookupEnv)

	rootCmd := &cobra.Command{
		Use:   "urp",
		Short: "Universal Repository Perception - AI agent senses for code",
		Long: `URP gives AI agents structured perception of code, git history,
and runtime state through PRU primitives (D, τ, Φ, ⊆, ⊥, P, T).

Use 'urp <noun> <verb>' pattern for all commands.`,
		PersistentPreRun: func(cmd *cobra.Command, args []string) {
			// Connect to graph (lazy, may fail)
			var err error
			db, err = graph.Connect()
			if err != nil {
				// Silent fail for status command
				db = nil
			}
		},
		PersistentPostRun: func(cmd *cobra.Command, args []string) {
			if db != nil {
				db.Close()
			}
		},
		Run: func(cmd *cobra.Command, args []string) {
			// Default: show status
			showStatus()
		},
	}

	rootCmd.PersistentFlags().BoolVar(&pretty, "pretty", true, "Pretty print output")
	rootCmd.PersistentFlags().Bool("json", false, "Output as JSON")

	// Add command groups
	rootCmd.AddCommand(versionCmd())
	rootCmd.AddCommand(eventsCmd())
	rootCmd.AddCommand(sessionCmd())
	rootCmd.AddCommand(codeCmd())
	rootCmd.AddCommand(gitCmd())
	rootCmd.AddCommand(thinkCmd())

	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

func versionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Show URP version",
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Printf("urp version %s\n", version)
		},
	}
}

func eventsCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "events",
		Short: "Terminal event commands",
		Long:  "Query and manage terminal events (τ + Φ primitives)",
	}

	// urp events run <cmd>
	runCmd := &cobra.Command{
		Use:   "run [command...]",
		Short: "Execute command and log to graph",
		Long:  "Run a command transparently, logging execution to the knowledge graph",
		Args:  cobra.MinimumNArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			executor := runner.NewExecutor(db)
			result := executor.Run(context.Background(), args)
			os.Exit(result.ExitCode)
		},
	}

	// urp events list
	var limit int
	var project string
	listCmd := &cobra.Command{
		Use:   "list",
		Short: "Show recent commands",
		Run: func(cmd *cobra.Command, args []string) {
			if db == nil {
				fmt.Fprintln(os.Stderr, "Error: Not connected to graph")
				os.Exit(1)
			}

			store := runner.NewEventStore(db)
			events, err := store.ListRecent(context.Background(), limit, project)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error: %v\n", err)
				os.Exit(1)
			}

			r := render.New(pretty)
			fmt.Print(r.Events(events))
		},
	}
	listCmd.Flags().IntVarP(&limit, "limit", "n", 20, "Number of events to show")
	listCmd.Flags().StringVarP(&project, "project", "p", "", "Filter by project")

	// urp events errors
	var minutes int
	errorsCmd := &cobra.Command{
		Use:   "errors",
		Short: "Show recent errors (pain)",
		Long:  "Show recent command failures (⊥ conflicts)",
		Run: func(cmd *cobra.Command, args []string) {
			if db == nil {
				fmt.Fprintln(os.Stderr, "Error: Not connected to graph")
				os.Exit(1)
			}

			store := runner.NewEventStore(db)
			conflicts, err := store.ListErrors(context.Background(), minutes, project)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error: %v\n", err)
				os.Exit(1)
			}

			r := render.New(pretty)
			title := fmt.Sprintf("Errors in last %d minutes", minutes)
			fmt.Print(r.Errors(conflicts, title))
		},
	}
	errorsCmd.Flags().IntVarP(&minutes, "minutes", "m", 5, "Look back N minutes")
	errorsCmd.Flags().StringVarP(&project, "project", "p", "", "Filter by project")

	cmd.AddCommand(runCmd, listCmd, errorsCmd)
	return cmd
}

func sessionCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "session",
		Short: "Session management",
	}

	// urp session id
	idCmd := &cobra.Command{
		Use:   "id",
		Short: "Show current session identity",
		Run: func(cmd *cobra.Command, args []string) {
			project := os.Getenv("PROJECT_NAME")
			if project == "" {
				project = "unknown"
			}
			hostname, _ := os.Hostname()

			fmt.Println("IDENTITY / CONTEXT")
			fmt.Println()
			fmt.Printf("  Project:  %s\n", project)
			fmt.Printf("  Host:     %s\n", hostname)
			fmt.Printf("  CWD:      %s\n", getCwd())
		},
	}

	cmd.AddCommand(idCmd)
	return cmd
}

func showStatus() {
	project := os.Getenv("PROJECT_NAME")
	if project == "" {
		project = "unknown"
	}

	connected := false
	eventCount := 0

	if db != nil {
		if err := db.Ping(context.Background()); err == nil {
			connected = true

			// Count events
			records, err := db.Execute(context.Background(),
				"MATCH (e:TerminalEvent) RETURN count(e) as count", nil)
			if err == nil && len(records) > 0 {
				if c, ok := records[0]["count"].(int64); ok {
					eventCount = int(c)
				}
			}
		}
	}

	r := render.New(pretty)
	fmt.Print(r.Status(connected, project, eventCount))
}

func getCwd() string {
	cwd, err := os.Getwd()
	if err != nil {
		return "unknown"
	}
	return cwd
}

func codeCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "code",
		Short: "Code analysis commands",
		Long:  "Parse and analyze code (D, Φ, ⊆ primitives)",
	}

	// urp code ingest <path>
	ingestCmd := &cobra.Command{
		Use:   "ingest <path>",
		Short: "Parse code into graph",
		Args:  cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			if db == nil {
				fmt.Fprintln(os.Stderr, "Error: Not connected to graph")
				os.Exit(1)
			}

			ingester := ingest.NewIngester(db)
			stats, err := ingester.Ingest(context.Background(), args[0])
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error: %v\n", err)
				os.Exit(1)
			}

			out, _ := json.MarshalIndent(stats, "", "  ")
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
			if db == nil {
				fmt.Fprintln(os.Stderr, "Error: Not connected to graph")
				os.Exit(1)
			}

			q := query.NewQuerier(db)
			deps, err := q.FindDeps(context.Background(), args[0], depth)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error: %v\n", err)
				os.Exit(1)
			}

			out, _ := json.MarshalIndent(deps, "", "  ")
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
			if db == nil {
				fmt.Fprintln(os.Stderr, "Error: Not connected to graph")
				os.Exit(1)
			}

			q := query.NewQuerier(db)
			impacts, err := q.FindImpact(context.Background(), args[0], depth)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error: %v\n", err)
				os.Exit(1)
			}

			out, _ := json.MarshalIndent(impacts, "", "  ")
			fmt.Println(string(out))
		},
	}
	impactCmd.Flags().IntVarP(&depth, "depth", "d", 3, "Max depth")

	// urp code dead
	deadCmd := &cobra.Command{
		Use:   "dead",
		Short: "Find unused code (⊥ unused)",
		Run: func(cmd *cobra.Command, args []string) {
			if db == nil {
				fmt.Fprintln(os.Stderr, "Error: Not connected to graph")
				os.Exit(1)
			}

			q := query.NewQuerier(db)
			dead, err := q.FindDeadCode(context.Background())
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error: %v\n", err)
				os.Exit(1)
			}

			out, _ := json.MarshalIndent(dead, "", "  ")
			fmt.Println(string(out))
		},
	}

	// urp code cycles
	cyclesCmd := &cobra.Command{
		Use:   "cycles",
		Short: "Find circular dependencies (⊥ conflict)",
		Run: func(cmd *cobra.Command, args []string) {
			if db == nil {
				fmt.Fprintln(os.Stderr, "Error: Not connected to graph")
				os.Exit(1)
			}

			q := query.NewQuerier(db)
			cycles, err := q.FindCycles(context.Background())
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error: %v\n", err)
				os.Exit(1)
			}

			out, _ := json.MarshalIndent(cycles, "", "  ")
			fmt.Println(string(out))
		},
	}

	// urp code hotspots
	var days int
	hotspotsCmd := &cobra.Command{
		Use:   "hotspots",
		Short: "Find high-churn areas (τ + Φ)",
		Run: func(cmd *cobra.Command, args []string) {
			if db == nil {
				fmt.Fprintln(os.Stderr, "Error: Not connected to graph")
				os.Exit(1)
			}

			q := query.NewQuerier(db)
			hotspots, err := q.FindHotspots(context.Background(), days)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error: %v\n", err)
				os.Exit(1)
			}

			out, _ := json.MarshalIndent(hotspots, "", "  ")
			fmt.Println(string(out))
		},
	}
	hotspotsCmd.Flags().IntVarP(&days, "days", "d", 30, "Look back N days")

	// urp code stats
	statsCmd := &cobra.Command{
		Use:   "stats",
		Short: "Show graph statistics",
		Run: func(cmd *cobra.Command, args []string) {
			if db == nil {
				fmt.Fprintln(os.Stderr, "Error: Not connected to graph")
				os.Exit(1)
			}

			q := query.NewQuerier(db)
			stats, err := q.GetStats(context.Background())
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error: %v\n", err)
				os.Exit(1)
			}

			out, _ := json.MarshalIndent(stats, "", "  ")
			fmt.Println(string(out))
		},
	}

	cmd.AddCommand(ingestCmd, depsCmd, impactCmd, deadCmd, cyclesCmd, hotspotsCmd, statsCmd)
	return cmd
}

func gitCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "git",
		Short: "Git history commands",
		Long:  "Load and query git history (τ primitive)",
	}

	// urp git ingest <path>
	var maxCommits int
	ingestCmd := &cobra.Command{
		Use:   "ingest <path>",
		Short: "Load git history into graph",
		Args:  cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			if db == nil {
				fmt.Fprintln(os.Stderr, "Error: Not connected to graph")
				os.Exit(1)
			}

			loader := ingest.NewGitLoader(db, args[0])
			stats, err := loader.LoadHistory(context.Background(), maxCommits)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error: %v\n", err)
				os.Exit(1)
			}

			out, _ := json.MarshalIndent(stats, "", "  ")
			fmt.Println(string(out))
		},
	}
	ingestCmd.Flags().IntVarP(&maxCommits, "max", "m", 500, "Max commits to load")

	// urp git history <file>
	var limit int
	historyCmd := &cobra.Command{
		Use:   "history <file>",
		Short: "Show file change history",
		Args:  cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			if db == nil {
				fmt.Fprintln(os.Stderr, "Error: Not connected to graph")
				os.Exit(1)
			}

			q := query.NewQuerier(db)
			history, err := q.GetHistory(context.Background(), args[0], limit)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error: %v\n", err)
				os.Exit(1)
			}

			out, _ := json.MarshalIndent(history, "", "  ")
			fmt.Println(string(out))
		},
	}
	historyCmd.Flags().IntVarP(&limit, "limit", "n", 20, "Max commits")

	cmd.AddCommand(ingestCmd, historyCmd)
	return cmd
}

func thinkCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "think",
		Short: "Cognitive skills",
		Long:  "AI-like reasoning: wisdom, novelty, learning",
	}

	// urp think wisdom <error>
	var threshold float64
	var project string
	wisdomCmd := &cobra.Command{
		Use:   "wisdom <error-message>",
		Short: "Find similar past errors",
		Long:  "Search for similar errors and their solutions",
		Args:  cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			if db == nil {
				fmt.Fprintln(os.Stderr, "Error: Not connected to graph")
				os.Exit(1)
			}

			svc := cognitive.NewWisdomService(db)
			matches, err := svc.ConsultWisdom(context.Background(), args[0], threshold, project)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error: %v\n", err)
				os.Exit(1)
			}

			if len(matches) == 0 {
				fmt.Println("WISDOM: No similar past errors found")
				fmt.Println("RECOMMENDATION: This may be a new type of error. Proceed with investigation.")
				return
			}

			fmt.Println("WISDOM: Similar past errors found")
			fmt.Println()
			for i, m := range matches {
				fmt.Printf("%d. [%.0f%%] %s\n", i+1, m.Similarity*100, m.Command)
				fmt.Printf("   Error: %s\n", truncateStr(m.Error, 80))
				if m.Solution != "" {
					fmt.Printf("   Solution: %s\n", m.Solution)
				}
				fmt.Println()
			}
		},
	}
	wisdomCmd.Flags().Float64VarP(&threshold, "threshold", "t", 0.3, "Similarity threshold (0-1)")
	wisdomCmd.Flags().StringVarP(&project, "project", "p", "", "Filter by project")

	// urp think novelty <code>
	noveltyCmd := &cobra.Command{
		Use:   "novelty <code-or-pattern>",
		Short: "Check if code is novel/unusual",
		Long:  "Analyze how novel a piece of code is compared to the codebase",
		Args:  cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			if db == nil {
				fmt.Fprintln(os.Stderr, "Error: Not connected to graph")
				os.Exit(1)
			}

			svc := cognitive.NewNoveltyService(db)
			result, err := svc.CheckNovelty(context.Background(), args[0])
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error: %v\n", err)
				os.Exit(1)
			}

			// Visual indicator
			indicator := "🟢"
			if result.Level == "moderate" {
				indicator = "🟡"
			} else if result.Level == "high" || result.Level == "pioneer" {
				indicator = "🔴"
			}

			fmt.Printf("NOVELTY: %.0f%% %s\n", result.Novelty*100, indicator)
			fmt.Printf("Level: %s\n", result.Level)
			fmt.Printf("Message: %s\n", result.Message)
			if result.Matches > 0 {
				fmt.Printf("Similar patterns found: %d\n", result.Matches)
			}
		},
	}

	// urp think learn <description>
	var minutes int
	learnCmd := &cobra.Command{
		Use:   "learn [description]",
		Short: "Consolidate recent success into knowledge",
		Long:  "Create a Solution node linking recent successful commands",
		Args:  cobra.MaximumNArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			if db == nil {
				fmt.Fprintln(os.Stderr, "Error: Not connected to graph")
				os.Exit(1)
			}

			description := "Solution validated"
			if len(args) > 0 {
				description = args[0]
			}

			svc := cognitive.NewLearningService(db)
			result, err := svc.ConsolidateLearning(context.Background(), description, minutes)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error: %v\n", err)
				os.Exit(1)
			}

			if !result.Success {
				fmt.Printf("Learning failed: %s\n", result.Error)
				os.Exit(1)
			}

			fmt.Printf("LEARNED: %s\n", result.Description)
			fmt.Printf("  Solution ID: %s\n", result.SolutionID)
			fmt.Printf("  Commands linked: %d\n", result.CommandsLinked)
			if result.ConflictsResolved > 0 {
				fmt.Printf("  Conflicts resolved: %d\n", result.ConflictsResolved)
			}
			fmt.Println("Knowledge crystallized. Future wisdom queries will find this solution.")
		},
	}
	learnCmd.Flags().IntVarP(&minutes, "minutes", "m", 10, "Look back N minutes")

	cmd.AddCommand(wisdomCmd, noveltyCmd, learnCmd)
	return cmd
}

func truncateStr(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
