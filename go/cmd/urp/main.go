// Package main provides the URP CLI entrypoint.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/joss/urp/internal/audit"
	"github.com/joss/urp/internal/cognitive"
	"github.com/joss/urp/internal/container"
	"github.com/joss/urp/internal/graph"
	"github.com/joss/urp/internal/ingest"
	"github.com/joss/urp/internal/memory"
	"github.com/joss/urp/internal/orchestrator"
	"github.com/joss/urp/internal/planning"
	"github.com/joss/urp/internal/protocol"
	"github.com/joss/urp/internal/query"
	"github.com/joss/urp/internal/render"
	"github.com/joss/urp/internal/runner"
	"github.com/joss/urp/internal/runtime"
	"github.com/joss/urp/internal/vector"
)

var (
	version     = "0.1.0"
	db          graph.Driver
	pretty      = true
	auditLogger *audit.Logger
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

			// Initialize audit logger
			auditLogger = audit.Global()
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

	// Orchestration command (multi-agent task execution)
	orchestrate := orchestrateCmd()
	orchestrate.GroupID = "infra"
	rootCmd.AddCommand(orchestrate)

	// Audit command
	auditCmd := auditCmd()
	auditCmd.GroupID = "runtime"
	rootCmd.AddCommand(auditCmd)

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
			event := auditLogger.StartWithCommand(audit.CategoryEvents, "run", strings.Join(args, " "))

			executor := runner.NewExecutor(db)
			result := executor.Run(context.Background(), args)

			event.ExitCode = result.ExitCode
			event.OutputSize = len(result.Stdout) + len(result.Stderr)
			if result.ExitCode != 0 {
				event.ErrorMessage = result.Stderr
				auditLogger.LogError(event, fmt.Errorf("exit code %d", result.ExitCode))
			} else {
				auditLogger.LogSuccess(event)
			}
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
			event := auditLogger.Start(audit.CategoryEvents, "list")

			if db == nil {
				auditLogger.LogError(event, fmt.Errorf("not connected to graph"))
				fmt.Fprintln(os.Stderr, "Error: Not connected to graph")
				os.Exit(1)
			}

			store := runner.NewEventStore(db)
			events, err := store.ListRecent(context.Background(), limit, project)
			if err != nil {
				auditLogger.LogError(event, err)
				fmt.Fprintf(os.Stderr, "Error: %v\n", err)
				os.Exit(1)
			}

			auditLogger.LogSuccess(event)

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
			event := auditLogger.Start(audit.CategoryEvents, "errors")

			if db == nil {
				auditLogger.LogError(event, fmt.Errorf("not connected to graph"))
				fmt.Fprintln(os.Stderr, "Error: Not connected to graph")
				os.Exit(1)
			}

			store := runner.NewEventStore(db)
			conflicts, err := store.ListErrors(context.Background(), minutes, project)
			if err != nil {
				auditLogger.LogError(event, err)
				fmt.Fprintf(os.Stderr, "Error: %v\n", err)
				os.Exit(1)
			}

			auditLogger.LogSuccess(event)

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
			event := auditLogger.Start(audit.CategoryCode, "ingest")

			if db == nil {
				auditLogger.LogError(event, fmt.Errorf("not connected to graph"))
				fmt.Fprintln(os.Stderr, "Error: Not connected to graph")
				os.Exit(1)
			}

			ingester := ingest.NewIngester(db)
			stats, err := ingester.Ingest(context.Background(), args[0])
			if err != nil {
				auditLogger.LogError(event, err)
				fmt.Fprintf(os.Stderr, "Error: %v\n", err)
				os.Exit(1)
			}

			out, _ := json.MarshalIndent(stats, "", "  ")
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

			if db == nil {
				auditLogger.LogError(event, fmt.Errorf("not connected to graph"))
				fmt.Fprintln(os.Stderr, "Error: Not connected to graph")
				os.Exit(1)
			}

			q := query.NewQuerier(db)
			deps, err := q.FindDeps(context.Background(), args[0], depth)
			if err != nil {
				auditLogger.LogError(event, err)
				fmt.Fprintf(os.Stderr, "Error: %v\n", err)
				os.Exit(1)
			}

			out, _ := json.MarshalIndent(deps, "", "  ")
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

			if db == nil {
				auditLogger.LogError(event, fmt.Errorf("not connected to graph"))
				fmt.Fprintln(os.Stderr, "Error: Not connected to graph")
				os.Exit(1)
			}

			q := query.NewQuerier(db)
			impacts, err := q.FindImpact(context.Background(), args[0], depth)
			if err != nil {
				auditLogger.LogError(event, err)
				fmt.Fprintf(os.Stderr, "Error: %v\n", err)
				os.Exit(1)
			}

			out, _ := json.MarshalIndent(impacts, "", "  ")
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

			if db == nil {
				auditLogger.LogError(event, fmt.Errorf("not connected to graph"))
				fmt.Fprintln(os.Stderr, "Error: Not connected to graph")
				os.Exit(1)
			}

			q := query.NewQuerier(db)
			dead, err := q.FindDeadCode(context.Background())
			if err != nil {
				auditLogger.LogError(event, err)
				fmt.Fprintf(os.Stderr, "Error: %v\n", err)
				os.Exit(1)
			}

			out, _ := json.MarshalIndent(dead, "", "  ")
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

			if db == nil {
				auditLogger.LogError(event, fmt.Errorf("not connected to graph"))
				fmt.Fprintln(os.Stderr, "Error: Not connected to graph")
				os.Exit(1)
			}

			q := query.NewQuerier(db)
			cycles, err := q.FindCycles(context.Background())
			if err != nil {
				auditLogger.LogError(event, err)
				fmt.Fprintf(os.Stderr, "Error: %v\n", err)
				os.Exit(1)
			}

			out, _ := json.MarshalIndent(cycles, "", "  ")
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

			if db == nil {
				auditLogger.LogError(event, fmt.Errorf("not connected to graph"))
				fmt.Fprintln(os.Stderr, "Error: Not connected to graph")
				os.Exit(1)
			}

			q := query.NewQuerier(db)
			hotspots, err := q.FindHotspots(context.Background(), days)
			if err != nil {
				auditLogger.LogError(event, err)
				fmt.Fprintf(os.Stderr, "Error: %v\n", err)
				os.Exit(1)
			}

			out, _ := json.MarshalIndent(hotspots, "", "  ")
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

			if db == nil {
				auditLogger.LogError(event, fmt.Errorf("not connected to graph"))
				fmt.Fprintln(os.Stderr, "Error: Not connected to graph")
				os.Exit(1)
			}

			q := query.NewQuerier(db)
			stats, err := q.GetStats(context.Background())
			if err != nil {
				auditLogger.LogError(event, err)
				fmt.Fprintf(os.Stderr, "Error: %v\n", err)
				os.Exit(1)
			}

			out, _ := json.MarshalIndent(stats, "", "  ")
			event.OutputSize = len(out)
			auditLogger.LogSuccess(event)
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
			event := auditLogger.Start(audit.CategoryGit, "ingest")

			if db == nil {
				auditLogger.LogError(event, fmt.Errorf("not connected to graph"))
				fmt.Fprintln(os.Stderr, "Error: Not connected to graph")
				os.Exit(1)
			}

			loader := ingest.NewGitLoader(db, args[0])
			stats, err := loader.LoadHistory(context.Background(), maxCommits)
			if err != nil {
				auditLogger.LogError(event, err)
				fmt.Fprintf(os.Stderr, "Error: %v\n", err)
				os.Exit(1)
			}

			out, _ := json.MarshalIndent(stats, "", "  ")
			event.OutputSize = len(out)
			auditLogger.LogSuccess(event)
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
			event := auditLogger.Start(audit.CategoryGit, "history")

			if db == nil {
				auditLogger.LogError(event, fmt.Errorf("not connected to graph"))
				fmt.Fprintln(os.Stderr, "Error: Not connected to graph")
				os.Exit(1)
			}

			q := query.NewQuerier(db)
			history, err := q.GetHistory(context.Background(), args[0], limit)
			if err != nil {
				auditLogger.LogError(event, err)
				fmt.Fprintf(os.Stderr, "Error: %v\n", err)
				os.Exit(1)
			}

			out, _ := json.MarshalIndent(history, "", "  ")
			event.OutputSize = len(out)
			auditLogger.LogSuccess(event)
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
			event := auditLogger.Start(audit.CategoryKnowledge, "wisdom")

			if db == nil {
				auditLogger.LogError(event, fmt.Errorf("not connected to graph"))
				fmt.Fprintln(os.Stderr, "Error: Not connected to graph")
				os.Exit(1)
			}

			svc := cognitive.NewWisdomService(db)
			matches, err := svc.ConsultWisdom(context.Background(), args[0], threshold, project)
			if err != nil {
				auditLogger.LogError(event, err)
				fmt.Fprintf(os.Stderr, "Error: %v\n", err)
				os.Exit(1)
			}

			auditLogger.LogSuccess(event)

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
			event := auditLogger.Start(audit.CategoryKnowledge, "novelty")

			if db == nil {
				auditLogger.LogError(event, fmt.Errorf("not connected to graph"))
				fmt.Fprintln(os.Stderr, "Error: Not connected to graph")
				os.Exit(1)
			}

			svc := cognitive.NewNoveltyService(db)
			result, err := svc.CheckNovelty(context.Background(), args[0])
			if err != nil {
				auditLogger.LogError(event, err)
				fmt.Fprintf(os.Stderr, "Error: %v\n", err)
				os.Exit(1)
			}

			auditLogger.LogSuccess(event)

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
			event := auditLogger.Start(audit.CategoryKnowledge, "learn")

			if db == nil {
				auditLogger.LogError(event, fmt.Errorf("not connected to graph"))
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
				auditLogger.LogError(event, err)
				fmt.Fprintf(os.Stderr, "Error: %v\n", err)
				os.Exit(1)
			}

			if !result.Success {
				auditLogger.LogError(event, fmt.Errorf(result.Error))
				fmt.Printf("Learning failed: %s\n", result.Error)
				os.Exit(1)
			}

			auditLogger.LogSuccess(event)

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
			event := auditLogger.Start(audit.CategorySystem, "vitals")

			obs := runtime.NewObserver(db)

			if obs.Runtime() == "" {
				auditLogger.LogWarning(event, "no container runtime detected")
				fmt.Println("No container runtime detected (docker/podman)")
				return
			}

			states, err := obs.Vitals(context.Background())
			if err != nil {
				auditLogger.LogError(event, err)
				fmt.Fprintf(os.Stderr, "Error: %v\n", err)
				os.Exit(1)
			}

			auditLogger.LogSuccess(event)

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
			event := auditLogger.Start(audit.CategorySystem, "topology")

			obs := runtime.NewObserver(db)

			topo, err := obs.Topology(context.Background())
			if err != nil {
				auditLogger.LogError(event, err)
				fmt.Fprintf(os.Stderr, "Error: %v\n", err)
				os.Exit(1)
			}

			auditLogger.LogSuccess(event)

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
			event := auditLogger.Start(audit.CategorySystem, "health")

			obs := runtime.NewObserver(db)

			issues, err := obs.Health(context.Background())
			if err != nil {
				auditLogger.LogError(event, err)
				fmt.Fprintf(os.Stderr, "Error: %v\n", err)
				os.Exit(1)
			}

			auditLogger.LogSuccess(event)

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
			event := auditLogger.Start(audit.CategorySystem, "runtime")

			obs := runtime.NewObserver(db)
			rt := obs.Runtime()

			auditLogger.LogSuccess(event)

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

// ══════════════════════════════════════════════════════════════════════════════
// ORCHESTRATION COMMANDS (Multi-Agent Task Execution)
// ══════════════════════════════════════════════════════════════════════════════

func orchestrateCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "orchestrate",
		Short: "Multi-agent task orchestration",
		Long: `Execute tasks in parallel across multiple workers.

The orchestrator coordinates multiple workers to execute tasks concurrently,
collecting results and reporting progress in real-time.

This is the E2E flow: User → Master → Workers → Results`,
	}

	// urp orchestrate run <task1> <task2> ...
	var timeout int
	runCmd := &cobra.Command{
		Use:   "run [tasks...]",
		Short: "Run tasks in parallel",
		Long: `Execute tasks in parallel across inline workers.

Each task description becomes an independent worker task.
Results are collected and displayed when all complete.

Examples:
  urp orchestrate run "Find dead code" "Check cycles" "Find hotspots"
  urp orchestrate run --timeout 120 "Long task 1" "Long task 2"`,
		Args: cobra.MinimumNArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			ctx, cancel := context.WithTimeout(context.Background(), time.Duration(timeout)*time.Second)
			defer cancel()

			orch := orchestrator.New()
			defer orch.Shutdown()

			// Set up callbacks for progress
			orch.OnTaskStarted = func(workerID, taskID string) {
				fmt.Printf("  ► [%s] Started: %s\n", workerID, taskID)
			}

			orch.OnTaskComplete = func(workerID, taskID string, result *orchestrator.TaskResult) {
				if result.Success {
					fmt.Printf("  ✓ [%s] Completed: %s (%.1fs)\n", workerID, taskID, result.Duration.Seconds())
				} else {
					fmt.Printf("  ✗ [%s] Failed: %s - %s\n", workerID, taskID, result.Error)
				}
			}

			orch.OnTaskFailed = func(workerID, taskID string, err error) {
				fmt.Printf("  ✗ [%s] Error: %s - %v\n", workerID, taskID, err)
			}

			// Build task definitions
			tasks := make([]orchestrator.TaskDefinition, len(args))
			for i, desc := range args {
				tasks[i] = orchestrator.TaskDefinition{
					ID:          fmt.Sprintf("task-%d", i+1),
					Description: desc,
				}
			}

			fmt.Printf("ORCHESTRATE: Running %d tasks in parallel\n", len(tasks))
			fmt.Println()

			// Create handler that simulates work
			handler := func(ctx context.Context, task *protocol.AssignTaskPayload, reporter *protocol.TaskReporter) error {
				reporter.Progress(0.1, "starting")

				// Simulate analysis based on task description
				output := fmt.Sprintf("Analyzed: %s", task.Description)

				reporter.Progress(0.5, "analyzing")
				time.Sleep(50 * time.Millisecond) // Simulate work

				reporter.Progress(1.0, "done")
				reporter.Complete(output, nil, "")
				return nil
			}

			results, err := orch.ExecuteTasksParallel(ctx, tasks, handler)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error: %v\n", err)
				os.Exit(1)
			}

			// Summary
			fmt.Println()
			fmt.Println("RESULTS:")
			successes := 0
			failures := 0
			for _, task := range tasks {
				result := results[task.ID]
				if result == nil {
					fmt.Printf("  ? %s: no result\n", task.ID)
					continue
				}
				if result.Success {
					successes++
					fmt.Printf("  ✓ %s: %s\n", task.ID, truncateStr(result.Output, 60))
				} else {
					failures++
					fmt.Printf("  ✗ %s: %s\n", task.ID, result.Error)
				}
			}
			fmt.Println()
			fmt.Printf("Summary: %d succeeded, %d failed\n", successes, failures)

			if failures > 0 {
				os.Exit(1)
			}
		},
	}
	runCmd.Flags().IntVarP(&timeout, "timeout", "t", 60, "Timeout in seconds")

	// urp orchestrate shell <command> [count]
	var parallel int
	shellCmd := &cobra.Command{
		Use:   "shell <command>",
		Short: "Run shell command in parallel workers",
		Long: `Execute the same shell command across multiple parallel workers.

Useful for parallel build/test tasks.

Examples:
  urp orchestrate shell "go test ./..." -n 4
  urp orchestrate shell "npm test" -n 3`,
		Args: cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			ctx, cancel := context.WithTimeout(context.Background(), time.Duration(timeout)*time.Second)
			defer cancel()

			orch := orchestrator.New()
			defer orch.Shutdown()

			orch.OnTaskComplete = func(workerID, taskID string, result *orchestrator.TaskResult) {
				if result.Success {
					fmt.Printf("  ✓ [%s] Done (%.1fs)\n", workerID, result.Duration.Seconds())
				} else {
					fmt.Printf("  ✗ [%s] Failed: %s\n", workerID, result.Error)
				}
			}

			// Build tasks
			tasks := make([]orchestrator.TaskDefinition, parallel)
			for i := 0; i < parallel; i++ {
				tasks[i] = orchestrator.TaskDefinition{
					ID:          fmt.Sprintf("shell-%d", i+1),
					Description: args[0],
					Command:     args[0],
				}
			}

			fmt.Printf("ORCHESTRATE: Running '%s' across %d workers\n", args[0], parallel)
			fmt.Println()

			handler := orchestrator.ShellCommandHandler(args[0])
			results, err := orch.ExecuteTasksParallel(ctx, tasks, handler)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error: %v\n", err)
				os.Exit(1)
			}

			// Count results
			successes := 0
			for _, r := range results {
				if r.Success {
					successes++
				}
			}

			fmt.Println()
			fmt.Printf("Summary: %d/%d succeeded\n", successes, parallel)
		},
	}
	shellCmd.Flags().IntVarP(&parallel, "workers", "n", 2, "Number of parallel workers")
	shellCmd.Flags().IntVarP(&timeout, "timeout", "t", 300, "Timeout in seconds")

	// urp orchestrate demo - Run a demo analysis
	var demoSimulate, demoPersist bool
	demoCmd := &cobra.Command{
		Use:   "demo [path]",
		Short: "Run demo multi-agent analysis",
		Long: `Demonstrate multi-agent orchestration with code analysis tasks.

Spawns workers to analyze: dead code, cycles, hotspots, and stats.
By default runs REAL urp commands against the graph database.

Examples:
  urp orchestrate demo              # Analyze current directory (real)
  urp orchestrate demo ./project    # Analyze specific path (real)
  urp orchestrate demo --simulate   # Run simulated demo
  urp orchestrate demo --persist    # Save results to Memgraph`,
		Run: func(cmd *cobra.Command, args []string) {
			ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
			defer cancel()

			startTime := time.Now()

			// Determine target path
			targetPath := "."
			if len(args) > 0 {
				targetPath = args[0]
			}

			orch := orchestrator.New()
			defer orch.Shutdown()

			orch.OnWorkerReady = func(workerID string, caps []string) {
				fmt.Printf("  ◉ Worker %s ready\n", workerID)
			}

			orch.OnTaskStarted = func(workerID, taskID string) {
				fmt.Printf("  ► [%s] %s\n", workerID, taskID)
			}

			orch.OnTaskComplete = func(workerID, taskID string, result *orchestrator.TaskResult) {
				status := "✓"
				if !result.Success {
					status = "✗"
				}
				fmt.Printf("  %s [%s] %s (%.2fs)\n", status, workerID, taskID, result.Duration.Seconds())
			}

			fmt.Println("═══════════════════════════════════════════════════════════")
			fmt.Println("  URP MULTI-AGENT ANALYSIS")
			fmt.Println("═══════════════════════════════════════════════════════════")
			fmt.Println()

			var tasks []orchestrator.TaskDefinition
			var handler protocol.TaskHandler

			if demoSimulate {
				// Simulated demo (original behavior)
				fmt.Println("  Mode:   SIMULATED")
				fmt.Printf("  Tasks:  4 parallel analysis tasks\n")
				fmt.Println()
				fmt.Println("  Progress:")

				tasks = []orchestrator.TaskDefinition{
					{ID: "dead-code", Description: "Find unused functions"},
					{ID: "cycles", Description: "Detect circular dependencies"},
					{ID: "hotspots", Description: "Identify high-churn areas"},
					{ID: "stats", Description: "Get graph statistics"},
				}

				handler = func(ctx context.Context, task *protocol.AssignTaskPayload, reporter *protocol.TaskReporter) error {
					reporter.Progress(0.1, "starting "+task.TaskID)

					var output string
					switch task.TaskID {
					case "dead-code":
						time.Sleep(80 * time.Millisecond)
						output = "Found 3 unused functions: oldHandler, deprecatedUtil, unusedHelper"
					case "cycles":
						time.Sleep(60 * time.Millisecond)
						output = "No circular dependencies detected"
					case "hotspots":
						time.Sleep(100 * time.Millisecond)
						output = "Top hotspots: main.go (42 changes), handler.go (28 changes)"
					case "stats":
						time.Sleep(70 * time.Millisecond)
						output = "Graph: 15 files, 47 functions, 128 edges"
					default:
						time.Sleep(50 * time.Millisecond)
						output = "Analysis complete"
					}

					reporter.Progress(1.0, "done")
					reporter.Complete(output, nil, "")
					return nil
				}
			} else {
				// REAL urp commands
				fmt.Println("  Mode:   REAL (using Memgraph)")
				fmt.Printf("  Target: %s\n", targetPath)
				fmt.Printf("  Tasks:  4 parallel urp commands\n")
				fmt.Println()

				// First, ingest the code if not already
				fmt.Println("  Preparing: ingesting code...")
				ingestCmd := exec.CommandContext(ctx, os.Args[0], "code", "ingest", targetPath)
				if out, err := ingestCmd.CombinedOutput(); err != nil {
					fmt.Printf("    ⚠ Ingest warning: %v\n", err)
					fmt.Printf("      %s\n", strings.TrimSpace(string(out)))
				} else {
					fmt.Println("    ✓ Code ingested")
				}
				fmt.Println()
				fmt.Println("  Progress:")

				tasks = []orchestrator.TaskDefinition{
					{ID: "dead-code", Description: "urp code dead", Command: "code dead"},
					{ID: "cycles", Description: "urp code cycles", Command: "code cycles"},
					{ID: "hotspots", Description: "urp code hotspots", Command: "code hotspots"},
					{ID: "stats", Description: "urp code stats", Command: "code stats"},
				}

				// Real handler that executes urp commands
				handler = func(ctx context.Context, task *protocol.AssignTaskPayload, reporter *protocol.TaskReporter) error {
					reporter.Progress(0.1, "executing "+task.TaskID)

					// Find command from task definition
					var cmdArgs []string
					for _, t := range tasks {
						if t.ID == task.TaskID {
							cmdArgs = strings.Fields(t.Command)
							break
						}
					}
					if len(cmdArgs) == 0 {
						reporter.Failed("unknown task: "+task.TaskID, 1)
						return fmt.Errorf("unknown task")
					}

					// Execute real urp command
					urpCmd := exec.CommandContext(ctx, os.Args[0], cmdArgs...)
					output, err := urpCmd.CombinedOutput()

					if err != nil {
						// Still report output even on error
						reporter.Failed(fmt.Sprintf("command failed: %v\n%s", err, string(output)), 1)
						return err
					}

					reporter.Progress(1.0, "done")
					reporter.Complete(strings.TrimSpace(string(output)), nil, "")
					return nil
				}
			}

			results, err := orch.ExecuteTasksParallel(ctx, tasks, handler)
			if err != nil {
				fmt.Fprintf(os.Stderr, "\nError: %v\n", err)
				os.Exit(1)
			}

			fmt.Println()
			fmt.Println("  Results:")
			fmt.Println("  ─────────────────────────────────────────────────────────")
			for _, task := range tasks {
				result := results[task.ID]
				if result == nil {
					fmt.Printf("  [%s] No result\n", task.ID)
					continue
				}
				if result.Success {
					// Format multi-line output with indent
					lines := strings.Split(result.Output, "\n")
					fmt.Printf("  [%s]\n", task.ID)
					for _, line := range lines {
						if line != "" {
							fmt.Printf("    %s\n", line)
						}
					}
				} else {
					fmt.Printf("  [%s] FAILED: %s\n", task.ID, result.Error)
				}
			}
			fmt.Println()

			// Count successes
			successes := 0
			for _, r := range results {
				if r != nil && r.Success {
					successes++
				}
			}
			fmt.Println("═══════════════════════════════════════════════════════════")
			fmt.Printf("  Summary: %d/%d tasks succeeded\n", successes, len(tasks))

			// Persist results if requested
			if demoPersist && !demoSimulate {
				db, err := graph.Connect()
				if err == nil {
					defer db.Close()
					sessCtx := memory.NewContext()
					po := orchestrator.NewPersistentOrchestrator(db, sessCtx.SessionID)

					description := fmt.Sprintf("Multi-agent analysis of %s", targetPath)
					duration := time.Since(startTime)

					run, err := po.RecordResults(ctx, description, results, duration)
					if err != nil {
						fmt.Printf("  Warning: failed to persist: %v\n", err)
					} else {
						fmt.Printf("  Persisted: %s\n", run.RunID)
					}
				}
			}

			fmt.Println("═══════════════════════════════════════════════════════════")
		},
	}
	demoCmd.Flags().BoolVar(&demoSimulate, "simulate", false, "Run simulated demo instead of real commands")
	demoCmd.Flags().BoolVar(&demoPersist, "persist", false, "Save results to Memgraph")

	// urp orchestrate history - Show recent runs
	var historyLimit int
	historyCmd := &cobra.Command{
		Use:   "history",
		Short: "Show recent orchestration runs",
		Long: `Display history of orchestration runs persisted in Memgraph.

Each run shows: status, task count, success/failure, and duration.`,
		Run: func(cmd *cobra.Command, args []string) {
			ctx := context.Background()

			// Connect to graph
			db, err := graph.Connect()
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error: cannot connect to Memgraph: %v\n", err)
				fmt.Println("Run 'urp infra start' to start Memgraph")
				os.Exit(1)
			}
			defer db.Close()

			// Get session ID
			sessCtx := memory.NewContext()

			// Create persistent orchestrator
			po := orchestrator.NewPersistentOrchestrator(db, sessCtx.SessionID)

			runs, err := po.ListRuns(ctx, historyLimit)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error: %v\n", err)
				os.Exit(1)
			}

			if len(runs) == 0 {
				fmt.Println("No orchestration runs found")
				fmt.Println("Run 'urp orchestrate demo --persist' to create one")
				return
			}

			fmt.Printf("ORCHESTRATION HISTORY (last %d runs)\n\n", len(runs))
			for _, run := range runs {
				status := "✓"
				if run.Status == "failed" {
					status = "✗"
				} else if run.Status == "running" {
					status = "◐"
				}

				fmt.Printf("  %s %s\n", status, run.RunID)
				fmt.Printf("    %s\n", run.Description)
				fmt.Printf("    Tasks: %d/%d succeeded | Duration: %dms\n",
					run.Succeeded, run.TaskCount, run.DurationMs)
				fmt.Printf("    Created: %s\n", run.CreatedAt)
				fmt.Println()
			}
		},
	}
	historyCmd.Flags().IntVarP(&historyLimit, "limit", "n", 10, "Number of runs to show")

	// urp orchestrate stats - Show aggregate stats
	statsCmd := &cobra.Command{
		Use:   "stats",
		Short: "Show orchestration statistics",
		Long:  `Display aggregate statistics for all orchestration runs.`,
		Run: func(cmd *cobra.Command, args []string) {
			ctx := context.Background()

			db, err := graph.Connect()
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error: cannot connect to Memgraph: %v\n", err)
				os.Exit(1)
			}
			defer db.Close()

			sessCtx := memory.NewContext()
			po := orchestrator.NewPersistentOrchestrator(db, sessCtx.SessionID)

			stats, err := po.GetRunStats(ctx)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error: %v\n", err)
				os.Exit(1)
			}

			fmt.Println("ORCHESTRATION STATISTICS")
			fmt.Println()
			fmt.Printf("  Total runs:     %v\n", stats["total_runs"])
			fmt.Printf("  Completed:      %v\n", stats["completed_runs"])
			fmt.Printf("  Failed:         %v\n", stats["failed_runs"])
			fmt.Println()
			fmt.Printf("  Total tasks:    %v\n", stats["total_tasks"])
			fmt.Printf("  Succeeded:      %v\n", stats["total_succeeded"])
			fmt.Printf("  Failed:         %v\n", stats["total_failed"])
			fmt.Println()
			if avg, ok := stats["avg_duration_ms"].(float64); ok && avg > 0 {
				fmt.Printf("  Avg duration:   %.0fms\n", avg)
			}
		},
	}

	cmd.AddCommand(runCmd, shellCmd, demoCmd, historyCmd, statsCmd)
	return cmd
}

// ══════════════════════════════════════════════════════════════════════════════
// AUDIT COMMANDS
// ══════════════════════════════════════════════════════════════════════════════

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
			if db == nil {
				fmt.Fprintln(os.Stderr, "Error: Not connected to graph database")
				os.Exit(1)
			}

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
			if db == nil {
				fmt.Fprintln(os.Stderr, "Error: Not connected to graph database")
				os.Exit(1)
			}

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
			if db == nil {
				fmt.Fprintln(os.Stderr, "Error: Not connected to graph database")
				os.Exit(1)
			}

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
			if db == nil {
				fmt.Fprintln(os.Stderr, "Error: Not connected to graph database")
				os.Exit(1)
			}

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
			if db == nil {
				fmt.Fprintln(os.Stderr, "Error: Not connected to graph database")
				os.Exit(1)
			}

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
			if db == nil {
				fmt.Fprintln(os.Stderr, "Error: Not connected to graph database")
				os.Exit(1)
			}

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
			if db == nil {
				fmt.Fprintln(os.Stderr, "Error: Not connected to graph database")
				os.Exit(1)
			}

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
			if db == nil {
				fmt.Fprintln(os.Stderr, "Error: Not connected to graph database")
				os.Exit(1)
			}

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
			if db == nil {
				fmt.Fprintln(os.Stderr, "Error: Not connected to graph database")
				os.Exit(1)
			}

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
