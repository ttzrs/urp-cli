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
	"github.com/joss/urp/internal/memory"
	"github.com/joss/urp/internal/query"
	"github.com/joss/urp/internal/render"
	"github.com/joss/urp/internal/runner"
	"github.com/joss/urp/internal/runtime"
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
	rootCmd.AddCommand(memCmd())
	rootCmd.AddCommand(kbCmd())
	rootCmd.AddCommand(focusCmd())
	rootCmd.AddCommand(sysCmd())

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

func memCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "mem",
		Short: "Session memory commands",
		Long:  "Private cognitive space for the current session",
	}

	// Get the context
	getCtx := func() *memory.Context {
		return memory.NewContext()
	}

	// urp mem add <text>
	var kind string
	var importance int
	addCmd := &cobra.Command{
		Use:   "add <text>",
		Short: "Add a note to session memory",
		Long:  "Remember something for this session (note, decision, observation)",
		Args:  cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			if db == nil {
				fmt.Fprintln(os.Stderr, "Error: Not connected to graph")
				os.Exit(1)
			}

			ctx := getCtx()
			mem := memory.NewSessionMemory(db, ctx)
			id, err := mem.Add(context.Background(), args[0], kind, importance, nil)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error: %v\n", err)
				os.Exit(1)
			}

			fmt.Printf("Remembered: %s\n", id)
			fmt.Printf("  Kind: %s, Importance: %d\n", kind, importance)
		},
	}
	addCmd.Flags().StringVarP(&kind, "kind", "k", "note", "Memory type (note|decision|summary|observation)")
	addCmd.Flags().IntVarP(&importance, "importance", "i", 2, "Importance 1-5")

	// urp mem recall <query>
	var limit int
	recallCmd := &cobra.Command{
		Use:   "recall <query>",
		Short: "Search session memories",
		Args:  cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			if db == nil {
				fmt.Fprintln(os.Stderr, "Error: Not connected to graph")
				os.Exit(1)
			}

			ctx := getCtx()
			mem := memory.NewSessionMemory(db, ctx)
			results, err := mem.Recall(context.Background(), args[0], limit, "", 1)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error: %v\n", err)
				os.Exit(1)
			}

			if len(results) == 0 {
				fmt.Println("No matching memories found")
				return
			}

			fmt.Println("RECALL: Matching memories")
			for _, m := range results {
				fmt.Printf("  [%.0f%%] [%s] %s\n", m.Similarity*100, m.Kind, truncateStr(m.Text, 60))
			}
		},
	}
	recallCmd.Flags().IntVarP(&limit, "limit", "n", 10, "Max results")

	// urp mem list
	listCmd := &cobra.Command{
		Use:   "list",
		Short: "List all session memories",
		Run: func(cmd *cobra.Command, args []string) {
			if db == nil {
				fmt.Fprintln(os.Stderr, "Error: Not connected to graph")
				os.Exit(1)
			}

			ctx := getCtx()
			mem := memory.NewSessionMemory(db, ctx)
			results, err := mem.List(context.Background())
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error: %v\n", err)
				os.Exit(1)
			}

			if len(results) == 0 {
				fmt.Println("No memories in this session")
				return
			}

			fmt.Printf("MEMORIES: %d items\n", len(results))
			for _, m := range results {
				fmt.Printf("  %s [%s] %s\n", m.MemoryID, m.Kind, truncateStr(m.Text, 50))
			}
		},
	}

	// urp mem stats
	statsCmd := &cobra.Command{
		Use:   "stats",
		Short: "Show session memory statistics",
		Run: func(cmd *cobra.Command, args []string) {
			if db == nil {
				fmt.Fprintln(os.Stderr, "Error: Not connected to graph")
				os.Exit(1)
			}

			ctx := getCtx()
			mem := memory.NewSessionMemory(db, ctx)
			stats, err := mem.Stats(context.Background())
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error: %v\n", err)
				os.Exit(1)
			}

			out, _ := json.MarshalIndent(stats, "", "  ")
			fmt.Println(string(out))
		},
	}

	// urp mem clear
	clearCmd := &cobra.Command{
		Use:   "clear",
		Short: "Clear all session memories",
		Run: func(cmd *cobra.Command, args []string) {
			if db == nil {
				fmt.Fprintln(os.Stderr, "Error: Not connected to graph")
				os.Exit(1)
			}

			ctx := getCtx()
			mem := memory.NewSessionMemory(db, ctx)
			count, err := mem.Clear(context.Background())
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error: %v\n", err)
				os.Exit(1)
			}

			fmt.Printf("Cleared %d memories\n", count)
		},
	}

	cmd.AddCommand(addCmd, recallCmd, listCmd, statsCmd, clearCmd)
	return cmd
}

func kbCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "kb",
		Short: "Knowledge base commands",
		Long:  "Shared knowledge across sessions",
	}

	getCtx := func() *memory.Context {
		return memory.NewContext()
	}

	// urp kb store <text>
	var kind, scope string
	storeCmd := &cobra.Command{
		Use:   "store <text>",
		Short: "Store knowledge",
		Args:  cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			if db == nil {
				fmt.Fprintln(os.Stderr, "Error: Not connected to graph")
				os.Exit(1)
			}

			ctx := getCtx()
			kb := memory.NewKnowledgeStore(db, ctx)
			id, err := kb.Store(context.Background(), args[0], kind, scope)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error: %v\n", err)
				os.Exit(1)
			}

			fmt.Printf("Stored: %s\n", id)
			fmt.Printf("  Kind: %s, Scope: %s\n", kind, scope)
		},
	}
	storeCmd.Flags().StringVarP(&kind, "kind", "k", "rule", "Knowledge type (error|fix|rule|pattern)")
	storeCmd.Flags().StringVarP(&scope, "scope", "s", "session", "Visibility (session|instance|global)")

	// urp kb query <text>
	var limit int
	var level string
	queryCmd := &cobra.Command{
		Use:   "query <text>",
		Short: "Search knowledge",
		Args:  cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			if db == nil {
				fmt.Fprintln(os.Stderr, "Error: Not connected to graph")
				os.Exit(1)
			}

			ctx := getCtx()
			kb := memory.NewKnowledgeStore(db, ctx)
			results, err := kb.Query(context.Background(), args[0], limit, level, "")
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error: %v\n", err)
				os.Exit(1)
			}

			if len(results) == 0 {
				fmt.Println("No matching knowledge found")
				return
			}

			fmt.Println("KNOWLEDGE: Matching entries")
			for _, k := range results {
				fmt.Printf("  [%.0f%%] [%s/%s] %s\n",
					k.Similarity*100, k.Scope, k.Kind, truncateStr(k.Text, 50))
			}
		},
	}
	queryCmd.Flags().IntVarP(&limit, "limit", "n", 10, "Max results")
	queryCmd.Flags().StringVarP(&level, "level", "l", "all", "Search level (session|instance|global|all)")

	// urp kb list
	listCmd := &cobra.Command{
		Use:   "list",
		Short: "List all knowledge",
		Run: func(cmd *cobra.Command, args []string) {
			if db == nil {
				fmt.Fprintln(os.Stderr, "Error: Not connected to graph")
				os.Exit(1)
			}

			ctx := getCtx()
			kb := memory.NewKnowledgeStore(db, ctx)
			results, err := kb.List(context.Background(), "", "", 50)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error: %v\n", err)
				os.Exit(1)
			}

			if len(results) == 0 {
				fmt.Println("No knowledge stored")
				return
			}

			fmt.Printf("KNOWLEDGE: %d entries\n", len(results))
			for _, k := range results {
				fmt.Printf("  %s [%s/%s] %s\n", k.KnowledgeID, k.Scope, k.Kind, truncateStr(k.Text, 40))
			}
		},
	}

	// urp kb reject <id> <reason>
	rejectCmd := &cobra.Command{
		Use:   "reject <knowledge-id> <reason>",
		Short: "Mark knowledge as not applicable",
		Args:  cobra.ExactArgs(2),
		Run: func(cmd *cobra.Command, args []string) {
			if db == nil {
				fmt.Fprintln(os.Stderr, "Error: Not connected to graph")
				os.Exit(1)
			}

			ctx := getCtx()
			kb := memory.NewKnowledgeStore(db, ctx)
			err := kb.Reject(context.Background(), args[0], args[1])
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error: %v\n", err)
				os.Exit(1)
			}

			fmt.Printf("Rejected: %s\n", args[0])
			fmt.Printf("  Reason: %s\n", args[1])
		},
	}

	// urp kb promote <id>
	promoteCmd := &cobra.Command{
		Use:   "promote <knowledge-id>",
		Short: "Promote knowledge to global scope",
		Args:  cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			if db == nil {
				fmt.Fprintln(os.Stderr, "Error: Not connected to graph")
				os.Exit(1)
			}

			ctx := getCtx()
			kb := memory.NewKnowledgeStore(db, ctx)
			err := kb.Promote(context.Background(), args[0])
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error: %v\n", err)
				os.Exit(1)
			}

			fmt.Printf("Promoted to global: %s\n", args[0])
		},
	}

	// urp kb stats
	statsCmd := &cobra.Command{
		Use:   "stats",
		Short: "Show knowledge statistics",
		Run: func(cmd *cobra.Command, args []string) {
			if db == nil {
				fmt.Fprintln(os.Stderr, "Error: Not connected to graph")
				os.Exit(1)
			}

			ctx := getCtx()
			kb := memory.NewKnowledgeStore(db, ctx)
			stats, err := kb.Stats(context.Background())
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error: %v\n", err)
				os.Exit(1)
			}

			out, _ := json.MarshalIndent(stats, "", "  ")
			fmt.Println(string(out))
		},
	}

	cmd.AddCommand(storeCmd, queryCmd, listCmd, rejectCmd, promoteCmd, statsCmd)
	return cmd
}

func focusCmd() *cobra.Command {
	var depth int

	cmd := &cobra.Command{
		Use:   "focus <target>",
		Short: "Load focused context around a target",
		Long:  "Load minimal context for surgical precision (reduces hallucination)",
		Args:  cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			if db == nil {
				fmt.Fprintln(os.Stderr, "Error: Not connected to graph")
				os.Exit(1)
			}

			svc := memory.NewFocusService(db)
			result, err := svc.Focus(context.Background(), args[0], depth)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Warning: %v\n", err)
			}

			if result != nil && result.Rendered != "" {
				fmt.Println(result.Rendered)
			} else {
				fmt.Println("// No entities found")
			}
		},
	}

	cmd.Flags().IntVarP(&depth, "depth", "d", 1, "Expansion depth (1-3)")

	return cmd
}

func sysCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "sys",
		Short: "System/runtime commands",
		Long:  "Container observation: vitals, topology, health (Φ energy primitives)",
	}

	// urp sys vitals
	vitalsCmd := &cobra.Command{
		Use:   "vitals",
		Short: "Show container CPU/RAM metrics",
		Long:  "Display energy metrics for running containers (Φ primitive)",
		Run: func(cmd *cobra.Command, args []string) {
			obs := runtime.NewObserver(db)

			if obs.Runtime() == "" {
				fmt.Println("No container runtime detected (docker/podman)")
				return
			}

			states, err := obs.Vitals(context.Background())
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error: %v\n", err)
				os.Exit(1)
			}

			if len(states) == 0 {
				fmt.Println("No running containers")
				return
			}

			fmt.Println("VITALS (Φ energy)")
			fmt.Println()
			for _, s := range states {
				memPct := fmt.Sprintf("%.1f%%", s.MemoryPct)
				cpuPct := fmt.Sprintf("%.1f%%", s.CPUPercent)
				fmt.Printf("  %-20s  CPU: %6s  MEM: %6s (%s / %s)\n",
					truncateStr(s.Name, 20),
					cpuPct,
					memPct,
					formatBytes(s.MemoryBytes),
					formatBytes(s.MemoryLimit))
			}
		},
	}

	// urp sys topology
	topologyCmd := &cobra.Command{
		Use:   "topology",
		Short: "Show container network map",
		Long:  "Display container network topology (⊆ inclusion)",
		Run: func(cmd *cobra.Command, args []string) {
			obs := runtime.NewObserver(db)

			topo, err := obs.Topology(context.Background())
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error: %v\n", err)
				os.Exit(1)
			}

			if topo.Error != "" {
				fmt.Printf("Warning: %s\n\n", topo.Error)
			}

			if len(topo.Containers) == 0 {
				fmt.Println("No containers found")
				return
			}

			fmt.Println("TOPOLOGY (⊆ network map)")
			fmt.Println()

			// Group by network
			byNetwork := make(map[string][]string)
			for _, c := range topo.Containers {
				for _, net := range c.Networks {
					byNetwork[net] = append(byNetwork[net], c.Name)
				}
			}

			for net, containers := range byNetwork {
				fmt.Printf("  [%s]\n", net)
				for _, name := range containers {
					fmt.Printf("    └── %s\n", name)
				}
			}
		},
	}

	// urp sys health
	healthCmd := &cobra.Command{
		Use:   "health",
		Short: "Check container health issues",
		Long:  "Detect container problems (⊥ orthogonal conflicts)",
		Run: func(cmd *cobra.Command, args []string) {
			obs := runtime.NewObserver(db)

			issues, err := obs.Health(context.Background())
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error: %v\n", err)
				os.Exit(1)
			}

			if len(issues) == 0 {
				fmt.Println("HEALTH: All containers healthy")
				return
			}

			fmt.Println("HEALTH (⊥ issues detected)")
			fmt.Println()
			for _, issue := range issues {
				icon := "⚠"
				if issue.Severity == "ERROR" || issue.Severity == "FATAL" {
					icon = "✗"
				}
				fmt.Printf("  %s [%s] %s: %s\n",
					icon, issue.Type, issue.Container, issue.Detail)
			}
		},
	}

	// urp sys runtime
	runtimeCmd := &cobra.Command{
		Use:   "runtime",
		Short: "Show detected container runtime",
		Run: func(cmd *cobra.Command, args []string) {
			obs := runtime.NewObserver(db)
			rt := obs.Runtime()
			if rt == "" {
				fmt.Println("No container runtime detected")
				return
			}
			fmt.Printf("Runtime: %s\n", rt)
		},
	}

	cmd.AddCommand(vitalsCmd, topologyCmd, healthCmd, runtimeCmd)
	return cmd
}

func formatBytes(b int64) string {
	const unit = 1024
	if b < unit {
		return fmt.Sprintf("%dB", b)
	}
	div, exp := int64(unit), 0
	for n := b / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f%cB", float64(b)/float64(div), "KMGTPE"[exp])
}
