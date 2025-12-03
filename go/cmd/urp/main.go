// Package main provides the URP CLI entrypoint.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/spf13/cobra"

	"github.com/joss/urp/internal/cognitive"
	"github.com/joss/urp/internal/container"
	"github.com/joss/urp/internal/graph"
	"github.com/joss/urp/internal/ingest"
	"github.com/joss/urp/internal/memory"
	"github.com/joss/urp/internal/planning"
	"github.com/joss/urp/internal/protocol"
	"github.com/joss/urp/internal/query"
	"github.com/joss/urp/internal/render"
	"github.com/joss/urp/internal/runner"
	"github.com/joss/urp/internal/runtime"
	"github.com/joss/urp/internal/vector"
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

	// Define command groups
	rootCmd.AddGroup(
		&cobra.Group{ID: "infra", Title: "Infrastructure:"},
		&cobra.Group{ID: "analysis", Title: "Analysis:"},
		&cobra.Group{ID: "cognitive", Title: "Cognitive:"},
		&cobra.Group{ID: "runtime", Title: "Runtime:"},
	)

	// Infrastructure commands
	infra := infraCmd()
	infra.GroupID = "infra"
	rootCmd.AddCommand(infra)

	launch := launchCmd()
	launch.GroupID = "infra"
	rootCmd.AddCommand(launch)

	spawn := spawnCmd()
	spawn.GroupID = "infra"
	rootCmd.AddCommand(spawn)

	workers := workersCmd()
	workers.GroupID = "infra"
	rootCmd.AddCommand(workers)

	attach := attachCmd()
	attach.GroupID = "infra"
	rootCmd.AddCommand(attach)

	kill := killCmd()
	kill.GroupID = "infra"
	rootCmd.AddCommand(kill)

	// Analysis commands
	code := codeCmd()
	code.GroupID = "analysis"
	rootCmd.AddCommand(code)

	git := gitCmd()
	git.GroupID = "analysis"
	rootCmd.AddCommand(git)

	focus := focusCmd()
	focus.GroupID = "analysis"
	rootCmd.AddCommand(focus)

	// Cognitive commands
	think := thinkCmd()
	think.GroupID = "cognitive"
	rootCmd.AddCommand(think)

	mem := memCmd()
	mem.GroupID = "cognitive"
	rootCmd.AddCommand(mem)

	kb := kbCmd()
	kb.GroupID = "cognitive"
	rootCmd.AddCommand(kb)

	vec := vecCmd()
	vec.GroupID = "cognitive"
	rootCmd.AddCommand(vec)

	// Runtime commands
	sys := sysCmd()
	sys.GroupID = "runtime"
	rootCmd.AddCommand(sys)

	events := eventsCmd()
	events.GroupID = "runtime"
	rootCmd.AddCommand(events)

	session := sessionCmd()
	session.GroupID = "runtime"
	rootCmd.AddCommand(session)

	// Planning commands (cognitive group)
	plan := planCmd()
	plan.GroupID = "cognitive"
	rootCmd.AddCommand(plan)

	// Worker command (for protocol communication)
	worker := workerCmd()
	worker.GroupID = "infra"
	rootCmd.AddCommand(worker)

	// Ungrouped
	rootCmd.AddCommand(versionCmd())

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

func vecCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "vec",
		Short: "Vector store commands",
		Long:  "Manage vector embeddings for semantic search",
	}

	// urp vec stats
	statsCmd := &cobra.Command{
		Use:   "stats",
		Short: "Show vector store statistics",
		Run: func(cmd *cobra.Command, args []string) {
			store := vector.Default()
			count, err := store.Count(context.Background())
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error: %v\n", err)
				os.Exit(1)
			}

			embedder := vector.GetDefaultEmbedder()

			fmt.Println("VECTOR STORE")
			fmt.Println()
			fmt.Printf("  Entries:    %d\n", count)
			fmt.Printf("  Dimensions: %d\n", embedder.Dimensions())
			fmt.Printf("  Embedder:   local (hash-based)\n")
		},
	}

	// urp vec search <query>
	var limit int
	var kind string
	searchCmd := &cobra.Command{
		Use:   "search <query>",
		Short: "Search vectors by text",
		Args:  cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			store := vector.Default()
			embedder := vector.GetDefaultEmbedder()

			// Generate embedding for query
			queryVec, err := embedder.Embed(context.Background(), args[0])
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error embedding query: %v\n", err)
				os.Exit(1)
			}

			// Search
			results, err := store.Search(context.Background(), queryVec, limit, kind)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error: %v\n", err)
				os.Exit(1)
			}

			if len(results) == 0 {
				fmt.Println("No matching vectors found")
				return
			}

			fmt.Printf("VECTOR SEARCH: %d results\n", len(results))
			fmt.Println()
			for i, r := range results {
				fmt.Printf("%d. [%.0f%%] [%s] %s\n",
					i+1, r.Score*100, r.Entry.Kind, truncateStr(r.Entry.Text, 60))
				if r.Entry.Metadata != nil {
					if cmd := r.Entry.Metadata["command"]; cmd != "" {
						fmt.Printf("   Command: %s\n", truncateStr(cmd, 50))
					}
					if proj := r.Entry.Metadata["project"]; proj != "" {
						fmt.Printf("   Project: %s\n", proj)
					}
				}
			}
		},
	}
	searchCmd.Flags().IntVarP(&limit, "limit", "n", 10, "Max results")
	searchCmd.Flags().StringVarP(&kind, "kind", "k", "", "Filter by kind (error|code|solution)")

	// urp vec add <text>
	var addKind string
	addCmd := &cobra.Command{
		Use:   "add <text>",
		Short: "Add text to vector store",
		Args:  cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			store := vector.Default()
			embedder := vector.GetDefaultEmbedder()

			// Generate embedding
			vec, err := embedder.Embed(context.Background(), args[0])
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error embedding: %v\n", err)
				os.Exit(1)
			}

			entry := vector.VectorEntry{
				Text:   args[0],
				Vector: vec,
				Kind:   addKind,
			}

			if err := store.Add(context.Background(), entry); err != nil {
				fmt.Fprintf(os.Stderr, "Error: %v\n", err)
				os.Exit(1)
			}

			fmt.Printf("Added to vector store [%s]: %s\n", addKind, truncateStr(args[0], 50))
		},
	}
	addCmd.Flags().StringVarP(&addKind, "kind", "k", "knowledge", "Entry kind (error|code|solution|knowledge)")

	cmd.AddCommand(statsCmd, searchCmd, addCmd)
	return cmd
}

// ══════════════════════════════════════════════════════════════════════════════
// CONTAINER ORCHESTRATION COMMANDS
// ══════════════════════════════════════════════════════════════════════════════

func infraCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "infra",
		Short: "Infrastructure management",
		Long:  "Manage URP container infrastructure (network, memgraph, volumes)",
	}

	// urp infra status
	statusCmd := &cobra.Command{
		Use:   "status",
		Short: "Show infrastructure status",
		Run: func(cmd *cobra.Command, args []string) {
			mgr := container.NewManager(context.Background())
			status := mgr.Status()

			fmt.Println("URP INFRASTRUCTURE")
			fmt.Println()

			// Runtime
			if status.Runtime == container.RuntimeNone {
				fmt.Println("  Runtime:  NOT FOUND")
				fmt.Println()
				fmt.Println("  Install docker or podman to use URP containers")
				return
			}
			fmt.Printf("  Runtime:  %s\n", status.Runtime)

			// Network
			if status.Network {
				fmt.Printf("  Network:  %s ✓\n", container.NetworkName)
			} else {
				fmt.Printf("  Network:  %s (not created)\n", container.NetworkName)
			}

			// Memgraph
			if status.Memgraph != nil {
				fmt.Printf("  Memgraph: %s (%s)\n", status.Memgraph.Name, status.Memgraph.Status)
				if status.Memgraph.Ports != "" {
					fmt.Printf("            Ports: %s\n", status.Memgraph.Ports)
				}
			} else {
				fmt.Println("  Memgraph: not running")
			}

			// Volumes
			fmt.Printf("  Volumes:  %d\n", len(status.Volumes))
			for _, v := range status.Volumes {
				fmt.Printf("            - %s\n", v)
			}

			// Workers
			fmt.Printf("  Workers:  %d\n", len(status.Workers))
			for _, w := range status.Workers {
				fmt.Printf("            - %s (%s)\n", w.Name, w.Status)
			}
		},
	}

	// urp infra start
	startCmd := &cobra.Command{
		Use:   "start",
		Short: "Start infrastructure (network, memgraph)",
		Run: func(cmd *cobra.Command, args []string) {
			mgr := container.NewManager(context.Background())

			fmt.Println("Starting URP infrastructure...")

			if err := mgr.StartInfra(); err != nil {
				fmt.Fprintf(os.Stderr, "Error: %v\n", err)
				os.Exit(1)
			}

			fmt.Println("✓ Network created")
			fmt.Println("✓ Volumes created")
			fmt.Println("✓ Memgraph running")
			fmt.Println()
			fmt.Println("Infrastructure ready. Use 'urp launch' to start a worker.")
		},
	}

	// urp infra stop
	stopCmd := &cobra.Command{
		Use:   "stop",
		Short: "Stop all URP containers",
		Run: func(cmd *cobra.Command, args []string) {
			mgr := container.NewManager(context.Background())

			fmt.Println("Stopping URP containers...")

			if err := mgr.StopInfra(); err != nil {
				fmt.Fprintf(os.Stderr, "Error: %v\n", err)
				os.Exit(1)
			}

			fmt.Println("✓ All containers stopped")
		},
	}

	// urp infra clean
	cleanCmd := &cobra.Command{
		Use:   "clean",
		Short: "Remove all URP containers, volumes, and network",
		Run: func(cmd *cobra.Command, args []string) {
			mgr := container.NewManager(context.Background())

			fmt.Println("Cleaning URP infrastructure...")

			if err := mgr.CleanInfra(); err != nil {
				fmt.Fprintf(os.Stderr, "Error: %v\n", err)
				os.Exit(1)
			}

			fmt.Println("✓ Containers removed")
			fmt.Println("✓ Volumes removed")
			fmt.Println("✓ Network removed")
		},
	}

	// urp infra logs
	var tail int
	logsCmd := &cobra.Command{
		Use:   "logs [container]",
		Short: "Show container logs",
		Args:  cobra.MaximumNArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			mgr := container.NewManager(context.Background())

			containerName := container.MemgraphName
			if len(args) > 0 {
				containerName = args[0]
			}

			logs, err := mgr.Logs(containerName, tail)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error: %v\n", err)
				os.Exit(1)
			}

			fmt.Printf("=== %s logs (last %d lines) ===\n", containerName, tail)
			fmt.Println(logs)
		},
	}
	logsCmd.Flags().IntVarP(&tail, "tail", "n", 50, "Number of lines")

	cmd.AddCommand(statusCmd, startCmd, stopCmd, cleanCmd, logsCmd)
	return cmd
}

func launchCmd() *cobra.Command {
	var worker bool
	var readOnly bool

	cmd := &cobra.Command{
		Use:   "launch [path]",
		Short: "Launch a URP container for a project",
		Long: `Launch a worker or master container for the specified project directory.

Master mode (default): Interactive session with auto-ingest and Claude CLI.
Worker mode: Background container for code changes.

Examples:
  urp launch              # Launch master for current directory (interactive)
  urp launch ~/project    # Launch master for specific path
  urp launch --worker     # Launch background worker instead
  urp launch --readonly   # Launch read-only worker`,
		Args: cobra.MaximumNArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			path := "."
			if len(args) > 0 {
				path = args[0]
			}

			mgr := container.NewManager(context.Background())

			var containerName string
			var err error

			if !worker {
				// Default: launch master (interactive)
				// No output - master entrypoint handles everything
				containerName, err = mgr.LaunchMaster(path)
				if err != nil {
					fmt.Fprintf(os.Stderr, "Error: %v\n", err)
					os.Exit(1)
				}
				// Master runs interactively, exits cleanly
				fmt.Println("\n✓ Master session ended")
			} else {
				// Worker mode: background container
				fmt.Printf("Launching worker for %s...\n", path)
				containerName, err = mgr.LaunchWorker(path, readOnly)
				if err != nil {
					fmt.Fprintf(os.Stderr, "Error: %v\n", err)
					os.Exit(1)
				}
				fmt.Printf("✓ Container started: %s\n", containerName)
				fmt.Println()
				fmt.Printf("Attach with: urp attach %s\n", containerName)
			}
		},
	}

	cmd.Flags().BoolVarP(&worker, "worker", "w", false, "Launch as background worker instead of master")
	cmd.Flags().BoolVarP(&readOnly, "readonly", "r", false, "Read-only access (only with --worker)")

	return cmd
}

func spawnCmd() *cobra.Command {
	var background bool
	var taskID string
	var planID string

	cmd := &cobra.Command{
		Use:   "spawn [num]",
		Short: "Spawn a worker container (from master)",
		Long: `Spawn a new worker container with write access.
Use this from inside a master container to create workers.

Default: interactive, ephemeral (--rm). Use -d for background.
With --task, creates a git branch for the task automatically.

Examples:
  urp spawn           # Interactive worker 1 (exits when done)
  urp spawn 2         # Interactive worker 2
  urp spawn -d        # Background worker (stays alive)
  urp spawn --task task-123 --plan plan-456  # Worker with git branch`,
		Args: cobra.MaximumNArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			// Check if running inside master
			if os.Getenv("URP_MASTER") != "1" {
				fmt.Fprintln(os.Stderr, "Error: spawn must be run from inside a master container")
				fmt.Fprintln(os.Stderr, "Use 'urp launch' first")
				os.Exit(1)
			}

			workerNum := 1
			if len(args) > 0 {
				fmt.Sscanf(args[0], "%d", &workerNum)
			}

			path := getCwd()
			mgr := container.NewManager(context.Background())

			var containerName string
			var err error

			if background {
				containerName, err = mgr.SpawnWorkerBackground(path, workerNum)
				if err != nil {
					fmt.Fprintf(os.Stderr, "Error: %v\n", err)
					os.Exit(1)
				}
				fmt.Printf("✓ Worker spawned: %s\n", containerName)
				fmt.Printf("Attach with: urp attach %s\n", containerName)
			} else {
				// Interactive - runs and exits
				containerName, err = mgr.SpawnWorkerForTask(path, workerNum, planID, taskID)
				if err != nil {
					fmt.Fprintf(os.Stderr, "Error: %v\n", err)
					os.Exit(1)
				}
				fmt.Printf("✓ Worker %s finished\n", containerName)
			}
		},
	}

	cmd.Flags().BoolVarP(&background, "detach", "d", false, "Run in background (stays alive)")
	cmd.Flags().StringVarP(&taskID, "task", "t", "", "Task ID (creates git branch)")
	cmd.Flags().StringVarP(&planID, "plan", "p", "", "Plan ID (required with --task)")

	return cmd
}

func workersCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "workers",
		Short: "List worker containers",
		Run: func(cmd *cobra.Command, args []string) {
			mgr := container.NewManager(context.Background())

			project := os.Getenv("URP_PROJECT")
			workers := mgr.ListWorkers(project)

			if len(workers) == 0 {
				fmt.Println("No workers running")
				return
			}

			fmt.Printf("WORKERS: %d\n", len(workers))
			fmt.Println()
			for i, w := range workers {
				fmt.Printf("  %d. %s\n", i+1, w.Name)
				fmt.Printf("     Image: %s\n", w.Image)
				fmt.Printf("     Status: %s\n", w.Status)
			}
		},
	}

	return cmd
}

func attachCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "attach <container>",
		Short: "Attach to a container",
		Long: `Attach to a running URP container with interactive shell.

Examples:
  urp attach urp-myproject
  urp attach urp-myproject-w1`,
		Args: cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			mgr := container.NewManager(context.Background())

			if err := mgr.AttachWorker(args[0]); err != nil {
				fmt.Fprintf(os.Stderr, "Error: %v\n", err)
				os.Exit(1)
			}
		},
	}

	return cmd
}

func killCmd() *cobra.Command {
	var all bool

	cmd := &cobra.Command{
		Use:   "kill <container>",
		Short: "Kill a worker container",
		Long: `Stop and remove a worker container.

Examples:
  urp kill urp-myproject-w1
  urp kill --all              # Kill all workers`,
		Args: cobra.MaximumNArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			mgr := container.NewManager(context.Background())

			if all {
				project := os.Getenv("URP_PROJECT")
				if err := mgr.KillAllWorkers(project); err != nil {
					fmt.Fprintf(os.Stderr, "Error: %v\n", err)
					os.Exit(1)
				}
				fmt.Println("✓ All workers killed")
				return
			}

			if len(args) == 0 {
				fmt.Fprintln(os.Stderr, "Error: container name required (or use --all)")
				os.Exit(1)
			}

			if err := mgr.KillWorker(args[0]); err != nil {
				fmt.Fprintf(os.Stderr, "Error: %v\n", err)
				os.Exit(1)
			}

			fmt.Printf("✓ Killed: %s\n", args[0])
		},
	}

	cmd.Flags().BoolVarP(&all, "all", "a", false, "Kill all workers")

	return cmd
}

// ══════════════════════════════════════════════════════════════════════════════
// PLANNING COMMANDS (Master/Worker orchestration)
// ══════════════════════════════════════════════════════════════════════════════

func planCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "plan",
		Short: "Plan/task orchestration",
		Long:  "Master/worker task orchestration for multi-agent workflows",
	}

	getPlanner := func() *planning.Planner {
		ctx := memory.NewContext()
		return planning.NewPlanner(db, ctx.SessionID)
	}

	// urp plan create <description> [task1] [task2] ...
	createCmd := &cobra.Command{
		Use:   "create <description> [tasks...]",
		Short: "Create a plan with tasks",
		Long: `Create a new plan with optional tasks.

Examples:
  urp plan create "Refactor auth module"
  urp plan create "Add caching" "Add Redis client" "Update handlers" "Write tests"`,
		Args: cobra.MinimumNArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			if db == nil {
				fmt.Fprintln(os.Stderr, "Error: Not connected to graph")
				os.Exit(1)
			}

			description := args[0]
			tasks := args[1:]

			planner := getPlanner()
			plan, err := planner.CreatePlan(context.Background(), description, tasks)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error: %v\n", err)
				os.Exit(1)
			}

			fmt.Printf("PLAN CREATED: %s\n", plan.PlanID)
			fmt.Printf("  Description: %s\n", plan.Description)
			fmt.Printf("  Status: %s\n", plan.Status)
			if len(plan.Tasks) > 0 {
				fmt.Printf("  Tasks: %d\n", len(plan.Tasks))
				for i, t := range plan.Tasks {
					fmt.Printf("    %d. [%s] %s\n", i+1, t.Status, t.Description)
				}
			}
		},
	}

	// urp plan show <plan_id>
	showCmd := &cobra.Command{
		Use:   "show <plan_id>",
		Short: "Show plan details",
		Args:  cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			if db == nil {
				fmt.Fprintln(os.Stderr, "Error: Not connected to graph")
				os.Exit(1)
			}

			planner := getPlanner()
			plan, err := planner.GetPlan(context.Background(), args[0])
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error: %v\n", err)
				os.Exit(1)
			}

			fmt.Printf("PLAN: %s\n", plan.PlanID)
			fmt.Printf("  Description: %s\n", plan.Description)
			fmt.Printf("  Status: %s\n", plan.Status)
			fmt.Printf("  Created: %s\n", plan.CreatedAt)
			fmt.Println()

			if len(plan.Tasks) > 0 {
				fmt.Printf("TASKS: %d\n", len(plan.Tasks))
				for _, t := range plan.Tasks {
					statusIcon := "○"
					switch t.Status {
					case planning.TaskCompleted:
						statusIcon = "✓"
					case planning.TaskInProgress:
						statusIcon = "►"
					case planning.TaskFailed:
						statusIcon = "✗"
					case planning.TaskAssigned:
						statusIcon = "◐"
					}
					workerInfo := ""
					if t.WorkerID != "" {
						workerInfo = fmt.Sprintf(" [%s]", t.WorkerID)
					}
					fmt.Printf("  %s %d. %s%s\n", statusIcon, t.Order, t.Description, workerInfo)
				}
			} else {
				fmt.Println("No tasks")
			}
		},
	}

	// urp plan list
	var limit int
	listCmd := &cobra.Command{
		Use:   "list",
		Short: "List plans for session",
		Run: func(cmd *cobra.Command, args []string) {
			if db == nil {
				fmt.Fprintln(os.Stderr, "Error: Not connected to graph")
				os.Exit(1)
			}

			planner := getPlanner()
			plans, err := planner.ListPlans(context.Background(), limit)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error: %v\n", err)
				os.Exit(1)
			}

			if len(plans) == 0 {
				fmt.Println("No plans found")
				return
			}

			fmt.Printf("PLANS: %d\n", len(plans))
			fmt.Println()
			for _, p := range plans {
				statusIcon := "○"
				switch p.Status {
				case planning.PlanCompleted:
					statusIcon = "✓"
				case planning.PlanInProgress:
					statusIcon = "►"
				case planning.PlanFailed:
					statusIcon = "✗"
				}
				fmt.Printf("  %s %s: %s\n", statusIcon, p.PlanID, truncateStr(p.Description, 50))
			}
		},
	}
	listCmd.Flags().IntVarP(&limit, "limit", "n", 20, "Max plans to show")

	// urp plan next <plan_id>
	nextCmd := &cobra.Command{
		Use:   "next <plan_id>",
		Short: "Get next pending task",
		Args:  cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			if db == nil {
				fmt.Fprintln(os.Stderr, "Error: Not connected to graph")
				os.Exit(1)
			}

			planner := getPlanner()
			task, err := planner.GetNextTask(context.Background(), args[0])
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error: %v\n", err)
				os.Exit(1)
			}

			if task == nil {
				fmt.Println("No pending tasks")
				return
			}

			fmt.Printf("NEXT TASK: %s\n", task.TaskID)
			fmt.Printf("  Description: %s\n", task.Description)
			fmt.Printf("  Order: %d\n", task.Order)
			fmt.Println()
			fmt.Printf("Assign with: urp plan assign %s <worker_id>\n", task.TaskID)
		},
	}

	// urp plan assign <task_id> <worker_id>
	assignCmd := &cobra.Command{
		Use:   "assign <task_id> <worker_id>",
		Short: "Assign task to worker",
		Args:  cobra.ExactArgs(2),
		Run: func(cmd *cobra.Command, args []string) {
			if db == nil {
				fmt.Fprintln(os.Stderr, "Error: Not connected to graph")
				os.Exit(1)
			}

			planner := getPlanner()
			if err := planner.AssignTask(context.Background(), args[0], args[1]); err != nil {
				fmt.Fprintf(os.Stderr, "Error: %v\n", err)
				os.Exit(1)
			}

			fmt.Printf("✓ Task %s assigned to %s\n", args[0], args[1])
		},
	}

	// urp plan start <task_id>
	startCmd := &cobra.Command{
		Use:   "start <task_id>",
		Short: "Mark task as in progress",
		Args:  cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			if db == nil {
				fmt.Fprintln(os.Stderr, "Error: Not connected to graph")
				os.Exit(1)
			}

			planner := getPlanner()
			if err := planner.StartTask(context.Background(), args[0]); err != nil {
				fmt.Fprintf(os.Stderr, "Error: %v\n", err)
				os.Exit(1)
			}

			fmt.Printf("✓ Task %s started\n", args[0])
		},
	}

	// urp plan complete <task_id> [output]
	var filesChanged string
	completeCmd := &cobra.Command{
		Use:   "complete <task_id> [output]",
		Short: "Mark task as completed",
		Args:  cobra.RangeArgs(1, 2),
		Run: func(cmd *cobra.Command, args []string) {
			if db == nil {
				fmt.Fprintln(os.Stderr, "Error: Not connected to graph")
				os.Exit(1)
			}

			output := ""
			if len(args) > 1 {
				output = args[1]
			}

			var files []string
			if filesChanged != "" {
				files = strings.Split(filesChanged, ",")
			}

			workerID := os.Getenv("URP_WORKER_ID")
			if workerID == "" {
				workerID = "manual"
			}

			planner := getPlanner()
			result, err := planner.CompleteTask(context.Background(), args[0], workerID, output, files)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error: %v\n", err)
				os.Exit(1)
			}

			fmt.Printf("✓ Task completed: %s\n", result.TaskID)
			fmt.Printf("  Result ID: %s\n", result.ResultID)
			if len(result.FilesChanged) > 0 {
				fmt.Printf("  Files changed: %d\n", len(result.FilesChanged))
			}
		},
	}
	completeCmd.Flags().StringVarP(&filesChanged, "files", "f", "", "Comma-separated list of changed files")

	// urp plan fail <task_id> <error>
	failCmd := &cobra.Command{
		Use:   "fail <task_id> <error>",
		Short: "Mark task as failed",
		Args:  cobra.ExactArgs(2),
		Run: func(cmd *cobra.Command, args []string) {
			if db == nil {
				fmt.Fprintln(os.Stderr, "Error: Not connected to graph")
				os.Exit(1)
			}

			workerID := os.Getenv("URP_WORKER_ID")
			if workerID == "" {
				workerID = "manual"
			}

			planner := getPlanner()
			result, err := planner.FailTask(context.Background(), args[0], workerID, args[1])
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error: %v\n", err)
				os.Exit(1)
			}

			fmt.Printf("✗ Task failed: %s\n", result.TaskID)
			fmt.Printf("  Error: %s\n", result.Error)
		},
	}

	// urp plan done <task_id> [output] - complete with PR
	var baseBranch string
	doneCmd := &cobra.Command{
		Use:   "done <task_id> [output]",
		Short: "Complete task and create PR if needed",
		Long: `Complete a task and automatically create a PR if there are commits.

This is the preferred way to complete tasks from workers. It:
1. Marks the task as completed in the graph
2. If on a task branch with commits, creates a PR
3. Stores the PR URL in the task result

Examples:
  urp plan done task-123 "Implemented feature"
  urp plan done task-123 --base main`,
		Args: cobra.RangeArgs(1, 2),
		Run: func(cmd *cobra.Command, args []string) {
			if db == nil {
				fmt.Fprintln(os.Stderr, "Error: Not connected to graph")
				os.Exit(1)
			}

			output := ""
			if len(args) > 1 {
				output = args[1]
			}

			var files []string
			if filesChanged != "" {
				files = strings.Split(filesChanged, ",")
			}

			workerID := os.Getenv("URP_WORKER_ID")
			if workerID == "" {
				workerID = "manual"
			}

			repoPath := getCwd()
			if baseBranch == "" {
				baseBranch = "master"
			}

			planner := getPlanner()
			result, pr, err := planner.CompleteTaskWithPR(context.Background(), args[0], workerID, output, files, repoPath, baseBranch)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error: %v\n", err)
				os.Exit(1)
			}

			fmt.Printf("✓ Task completed: %s\n", result.TaskID)
			fmt.Printf("  Result ID: %s\n", result.ResultID)
			if pr != nil {
				fmt.Printf("  PR created: %s\n", pr.URL)
				fmt.Printf("  Branch: %s → %s\n", pr.Branch, pr.BaseBranch)
			}
		},
	}
	doneCmd.Flags().StringVarP(&filesChanged, "files", "f", "", "Comma-separated list of changed files")
	doneCmd.Flags().StringVarP(&baseBranch, "base", "b", "master", "Base branch for PR")

	// urp plan merge <task_id>
	var squash bool
	mergeCmd := &cobra.Command{
		Use:   "merge <task_id>",
		Short: "Merge the PR for a completed task",
		Long: `Merge the pull request associated with a task.

Use from master to merge worker PRs after review.

Examples:
  urp plan merge task-123
  urp plan merge task-123 --squash`,
		Args: cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			if db == nil {
				fmt.Fprintln(os.Stderr, "Error: Not connected to graph")
				os.Exit(1)
			}

			// Get PR URL from task result
			query := `
				MATCH (task:Task {task_id: $task_id})-[:HAS_RESULT]->(r:Result)
				RETURN r.pr_url as pr_url
			`
			records, err := db.Execute(context.Background(), query, map[string]any{
				"task_id": args[0],
			})
			if err != nil || len(records) == 0 {
				fmt.Fprintln(os.Stderr, "Error: Task or PR not found")
				os.Exit(1)
			}

			prURL, ok := records[0]["pr_url"].(string)
			if !ok || prURL == "" {
				fmt.Fprintln(os.Stderr, "Error: No PR associated with this task")
				os.Exit(1)
			}

			// Extract PR number from URL (format: .../pull/123)
			parts := strings.Split(prURL, "/")
			if len(parts) < 2 {
				fmt.Fprintln(os.Stderr, "Error: Invalid PR URL")
				os.Exit(1)
			}
			var prNum int
			fmt.Sscanf(parts[len(parts)-1], "%d", &prNum)

			repoPath := getCwd()
			if err := planning.MergePR(repoPath, prNum, squash); err != nil {
				fmt.Fprintf(os.Stderr, "Error: %v\n", err)
				os.Exit(1)
			}

			fmt.Printf("✓ PR #%d merged\n", prNum)
			fmt.Printf("  Task: %s\n", args[0])
		},
	}
	mergeCmd.Flags().BoolVarP(&squash, "squash", "s", false, "Squash merge")

	cmd.AddCommand(createCmd, showCmd, listCmd, nextCmd, assignCmd, startCmd, completeCmd, failCmd, doneCmd, mergeCmd)
	return cmd
}

// ══════════════════════════════════════════════════════════════════════════════
// WORKER PROTOCOL COMMANDS
// ══════════════════════════════════════════════════════════════════════════════

func workerCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "worker",
		Short: "Worker protocol commands",
		Long:  "Commands for worker containers communicating with master via protocol",
	}

	// urp worker run - Run as protocol worker (reads stdin, writes stdout)
	runCmd := &cobra.Command{
		Use:   "run",
		Short: "Run as protocol worker",
		Long: `Start worker in protocol mode, communicating with master via stdin/stdout.

The worker reads JSON Line messages from stdin and writes responses to stdout.
All logs go to stderr.

This is typically called by the worker-entrypoint.sh in Docker containers.`,
		Run: func(cmd *cobra.Command, args []string) {
			workerID := os.Getenv("URP_WORKER_ID")
			if workerID == "" {
				hostname, _ := os.Hostname()
				workerID = hostname
			}

			capsEnv := os.Getenv("URP_WORKER_CAPS")
			var caps []string
			if capsEnv != "" {
				caps = strings.Split(capsEnv, ",")
			}

			worker := protocol.NewWorker(workerID, caps)
			worker.SetHandler(workerTaskHandler)

			ctx := context.Background()
			if err := worker.Run(ctx); err != nil {
				fmt.Fprintf(os.Stderr, "Worker error: %v\n", err)
				os.Exit(1)
			}
		},
	}

	cmd.AddCommand(runCmd)
	return cmd
}

// workerTaskHandler handles tasks assigned to this worker
func workerTaskHandler(ctx context.Context, task *protocol.AssignTaskPayload, reporter *protocol.TaskReporter) error {
	reporter.Progress(0.1, "starting task")

	// Log to stderr (stdout is for protocol)
	fmt.Fprintf(os.Stderr, "[worker] Task %s: %s\n", task.TaskID, task.Description)

	// Create the git branch if specified
	if task.Branch != "" {
		reporter.Progress(0.2, "creating git branch")
		cmd := exec.CommandContext(ctx, "git", "checkout", "-b", task.Branch)
		cmd.Dir = "/workspace"
		if err := cmd.Run(); err != nil {
			// Branch might already exist, try checkout
			cmd = exec.CommandContext(ctx, "git", "checkout", task.Branch)
			cmd.Dir = "/workspace"
			if err := cmd.Run(); err != nil {
				reporter.Failed(fmt.Sprintf("failed to checkout branch: %v", err), 1)
				return err
			}
		}
		reporter.Stdout(fmt.Sprintf("On branch %s\n", task.Branch))
	}

	reporter.Progress(0.3, "ready for work")

	// For now, workers are interactive - they'll use the shell
	// In the future, this could run automated tasks based on task.Context
	reporter.Stdout("Worker ready. Execute commands manually or use automation.\n")

	// Wait for context cancellation (master cancels when done)
	<-ctx.Done()

	reporter.Progress(1.0, "task completed")
	reporter.Complete("Worker session ended", nil, "")
	return nil
}
