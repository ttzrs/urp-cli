// Package main provides the URP CLI entrypoint.
package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"golang.org/x/term"

	"github.com/joss/urp/internal/audit"
	"github.com/joss/urp/internal/config"
	"github.com/joss/urp/internal/container"
	"github.com/joss/urp/internal/graph"
	"github.com/joss/urp/internal/memory"
	"github.com/joss/urp/internal/orchestrator"
	"github.com/joss/urp/internal/planning"
	"github.com/joss/urp/internal/protocol"
	"github.com/joss/urp/internal/render"
	"github.com/joss/urp/internal/runner"
	"github.com/joss/urp/internal/selftest"
	"github.com/joss/urp/internal/tui"
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
		Use:   "urp [path]",
		Short: "Universal Repository Perception - AI-powered code agent",
		Long: `URP: AI-powered code agent with structured perception.

Usage modes:
  urp              Start interactive OpenCode session (current directory)
  urp <path>       Start interactive session in specified directory
  urp <command>    Run specific URP command (see below)

Use 'urp status' to show infrastructure status.
Use 'urp help' for full command list.`,
		Args: cobra.MaximumNArgs(1),
		PersistentPreRun: func(cmd *cobra.Command, args []string) {
			// Connect to graph (lazy, may fail)
			var err error
			db, err = graph.Connect()
			if err != nil {
				// Silent fail for status command
				db = nil
			}

			// Configure logging to persist events to graph
			if db != nil {
				audit.SetGraphDriver(db)
				// Initialize Memgraph vector store
				vector.SetDefaultStore(vector.NewMemgraphStore(db))
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
			// Determine work directory
			workDir, _ := os.Getwd()
			if len(args) > 0 {
				// Path provided - resolve it
				path := args[0]
				if !filepath.IsAbs(path) {
					path = filepath.Join(workDir, path)
				}
				workDir = path
			}

			// Check for TUI mode (experimental)
			useTUI, _ := cmd.Flags().GetBool("tui")
			if useTUI {
				runInteractiveAgent(workDir)
				return
			}

			// Default: use debug mode (stable)
			runInteractiveAgentDebug(workDir)
		},
	}

	rootCmd.PersistentFlags().BoolVar(&pretty, "pretty", true, "Pretty print output")
	rootCmd.PersistentFlags().Bool("json", false, "Output as JSON")
	rootCmd.Flags().Bool("tui", false, "Use experimental TUI mode")

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

	execC := execCmd()
	execC.GroupID = "infra"
	rootCmd.AddCommand(execC)

	kill := killCmd()
	kill.GroupID = "infra"
	rootCmd.AddCommand(kill)

	ask := askCmd()
	ask.GroupID = "infra"
	rootCmd.AddCommand(ask)

	nemo := nemoCmd()
	nemo.GroupID = "infra"
	rootCmd.AddCommand(nemo)

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

	// OpenCode integration
	oc := opencodeCmd()
	oc.GroupID = "cognitive"
	rootCmd.AddCommand(oc)

	// Skills management
	sk := skillsCmd()
	sk.GroupID = "cognitive"
	rootCmd.AddCommand(sk)

	// Backup management
	bak := backupCmd()
	bak.GroupID = "cognitive"
	rootCmd.AddCommand(bak)

	// Spec-Driven Development
	spec := specCmd()
	spec.GroupID = "cognitive"
	rootCmd.AddCommand(spec)

	// Doctor command (environment diagnostics)
	doctor := doctorCmd()
	doctor.GroupID = "infra"
	rootCmd.AddCommand(doctor)

	// Alert command
	alert := alertCmd()
	alert.GroupID = "runtime"
	rootCmd.AddCommand(alert)

	// Ungrouped
	rootCmd.AddCommand(versionCmd())
	rootCmd.AddCommand(statusCmd())
	rootCmd.AddCommand(tuiCmd())

	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

// statusCmd shows infrastructure status (former default behavior)
func statusCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Show URP infrastructure status",
		Run: func(cmd *cobra.Command, args []string) {
			showStatus()
		},
	}
}

// tuiCmd launches the interactive TUI
func tuiCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "tui",
		Short: "Launch interactive terminal UI",
		Long:  "Start the Bubble Tea powered interactive terminal interface",
		Run: func(cmd *cobra.Command, args []string) {
			if err := tui.Run(); err != nil {
				fmt.Fprintf(os.Stderr, "Error: %v\n", err)
				os.Exit(1)
			}
		},
	}
}

// runInteractiveAgent starts an interactive OpenCode session
func runInteractiveAgent(workDir string) {
	// Verify directory exists
	if _, err := os.Stat(workDir); os.IsNotExist(err) {
		fmt.Fprintf(os.Stderr, "Error: directory does not exist: %s\n", workDir)
		os.Exit(1)
	}

	// Container mode - delegate to claude CLI directly
	if config.Env().ContainerMode {
		cmd := exec.Command("claude")
		cmd.Dir = workDir
		cmd.Stdin = os.Stdin
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		if err := cmd.Run(); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
		return
	}

	// Host mode - use Bubble Tea TUI
	if err := tui.RunAgent(workDir); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

// runInteractiveAgentDebug runs agent with static output (debug mode)
func runInteractiveAgentDebug(workDir string) {
	// Verify directory exists
	if _, err := os.Stat(workDir); os.IsNotExist(err) {
		fmt.Fprintf(os.Stderr, "Error: directory does not exist: %s\n", workDir)
		os.Exit(1)
	}

	fmt.Println("🔧 URP Debug Mode")
	fmt.Printf("📁 Working directory: %s\n", workDir)

	// Use static runner instead of TUI
	if err := tui.RunAgentDebug(workDir); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
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

func doctorCmd() *cobra.Command {
	var verbose bool
	var quick bool

	cmd := &cobra.Command{
		Use:   "doctor",
		Short: "Check environment health",
		Long: `Diagnose the URP runtime environment.

Checks:
  - Container runtime (docker/podman)
  - TTY availability
  - Network configuration
  - Required images
  - Memgraph connectivity

Use --quick for Docker HEALTHCHECK (fast, minimal checks inside container).`,
		Run: func(cmd *cobra.Command, args []string) {
			// Quick mode for Docker HEALTHCHECK
			if quick {
				os.Exit(selftest.RunQuickHealthCheck())
			}

			env := selftest.Check()

			if verbose {
				fmt.Print(env.Summary())
			} else {
				fmt.Println(env.QuickCheck())
			}

			if !env.IsHealthy() {
				os.Exit(1)
			}
		},
	}

	cmd.Flags().BoolVarP(&verbose, "verbose", "v", false, "Show detailed diagnostics")
	cmd.Flags().BoolVarP(&quick, "quick", "q", false, "Fast health check for Docker HEALTHCHECK")
	return cmd
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

			requireDB(event)

			store := runner.NewEventStore(db)
			events, err := store.ListRecent(context.Background(), limit, project)
			if err != nil {
				exitOnError(event, err)
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

			requireDB(event)

			store := runner.NewEventStore(db)
			conflicts, err := store.ListErrors(context.Background(), minutes, project)
			if err != nil {
				exitOnError(event, err)
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

			// Network (show project-scoped name)
			project := config.Env().Project
			networkName := container.NetworkName(project)
			if status.Network {
				fmt.Printf("  Network:  %s ✓\n", networkName)
			} else {
				fmt.Printf("  Network:  %s (not created)\n", networkName)
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
			project := config.Env().Project
			mgr := container.NewManagerForProject(context.Background(), project)

			if project != "" {
				fmt.Printf("Starting URP infrastructure for project: %s\n", project)
			} else {
				fmt.Println("Starting URP infrastructure (default)...")
			}

			if err := mgr.StartInfra(); err != nil {
				fmt.Fprintf(os.Stderr, "Error: %v\n", err)
				os.Exit(1)
			}

			networkName := container.NetworkName(project)
			memgraphName := container.MemgraphName(project)
			fmt.Printf("✓ Network: %s\n", networkName)
			fmt.Println("✓ Volumes created")
			fmt.Printf("✓ Memgraph: %s (no host ports)\n", memgraphName)
			fmt.Println()
			fmt.Println("Infrastructure ready. Use 'urp launch' to start a master.")
		},
	}

	// urp infra stop
	stopCmd := &cobra.Command{
		Use:   "stop",
		Short: "Stop all URP containers for current project",
		Run: func(cmd *cobra.Command, args []string) {
			project := config.Env().Project
			mgr := container.NewManagerForProject(context.Background(), project)

			if project != "" {
				fmt.Printf("Stopping URP containers for project: %s\n", project)
			} else {
				fmt.Println("Stopping URP containers (all)...")
			}

			if err := mgr.StopInfra(); err != nil {
				fmt.Fprintf(os.Stderr, "Error: %v\n", err)
				os.Exit(1)
			}

			fmt.Println("✓ Containers stopped")
		},
	}

	// urp infra clean
	cleanCmd := &cobra.Command{
		Use:   "clean",
		Short: "Remove all URP containers, volumes, and network for current project",
		Run: func(cmd *cobra.Command, args []string) {
			project := config.Env().Project
			mgr := container.NewManagerForProject(context.Background(), project)

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

			// Default to project memgraph (use URP_PROJECT env or empty for default)
			project := config.Env().Project
			containerName := container.MemgraphName(project)
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
  urp launch              # Launch master for current directory
  urp launch ~/project    # Launch master for specific path
  urp launch --worker     # Launch background worker instead
  urp launch --readonly   # Launch read-only worker`,
		Args: cobra.MaximumNArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			// Pre-flight environment check
			env := selftest.Check()
			if !env.IsHealthy() {
				fmt.Fprintf(os.Stderr, "Environment check failed:\n")
				for _, e := range env.Errors {
					fmt.Fprintf(os.Stderr, "  ✗ %s\n", e)
				}
				fmt.Fprintf(os.Stderr, "\nRun 'urp doctor -v' for details.\n")
				os.Exit(1)
			}
			// Show warnings but continue
			for _, w := range env.Warnings {
				fmt.Fprintf(os.Stderr, "⚠ %s\n", w)
			}

			path := "."
			if len(args) > 0 {
				path = args[0]
			}

			mgr := container.NewManager(context.Background())

			var containerName string
			var err error

			if !worker {
				// Default: launch master (interactive or detached)
				containerName, err = mgr.LaunchMaster(path)
				if err != nil {
					fmt.Fprintf(os.Stderr, "Error: %v\n", err)
					os.Exit(1)
				}
				// Check if TTY mode (interactive) or detached
				if term.IsTerminal(int(os.Stdin.Fd())) {
					// Interactive mode - session ended
					fmt.Println("\n✓ Master session ended")
				} else {
					// Detached mode - container running in background
					fmt.Printf("✓ Master started: %s\n", containerName)
					fmt.Println()
					fmt.Println("Container is running in detached mode (no TTY).")
					fmt.Printf("  Execute commands:  urp exec %s <command>\n", containerName)
					fmt.Printf("  View logs:         docker logs -f %s\n", containerName)
					fmt.Printf("  Stop:              docker rm -f %s\n", containerName)
				}
			} else {
				// Standalone mode: background container (no master/worker flow)
				fmt.Printf("Launching standalone for %s...\n", path)
				containerName, err = mgr.LaunchStandalone(path, readOnly)
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

	cmd.Flags().BoolVarP(&worker, "worker", "w", false, "Launch as standalone daemon (no master/worker flow)")
	cmd.Flags().BoolVarP(&readOnly, "readonly", "r", false, "Read-only access (only with --worker)")

	return cmd
}

func spawnCmd() *cobra.Command {
	var background bool

	cmd := &cobra.Command{
		Use:   "spawn [num]",
		Short: "Spawn a worker container (from master)",
		Long: `Spawn a worker container with read-write access.
Run from inside master. Send instructions via urp ask.

Examples:
  urp spawn              # Create worker 1
  urp spawn 2            # Create worker 2
  urp ask urp-proj-w1 "run tests"  # Send instruction`,
		Args: cobra.MaximumNArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			// Check if running inside master
			if !config.Env().IsMaster {
				fmt.Fprintln(os.Stderr, "Error: spawn must be run from inside a master container")
				fmt.Fprintln(os.Stderr, "Use 'urp launch' first")
				os.Exit(1)
			}

			workerNum := 1
			if len(args) > 0 {
				fmt.Sscanf(args[0], "%d", &workerNum)
			}

			// Inside master, use HOST path for worker volume mounts
			path := config.Env().HostPath
			if path == "" {
				path = getCwd()
			}
			mgr := container.NewManager(context.Background())

			var containerName string
			var err error

			containerName, err = mgr.SpawnWorker(path, workerNum)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error: %v\n", err)
				os.Exit(1)
			}
			fmt.Printf("✓ Worker spawned: %s\n", containerName)
			fmt.Printf("  Send tasks: urp ask %s \"<instruction>\"\n", containerName)
			fmt.Printf("  Attach:     urp attach %s\n", containerName)
		},
	}

	cmd.Flags().BoolVarP(&background, "detach", "d", false, "Ignored (workers always detached)")

	return cmd
}

func workersCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "workers",
		Short: "List worker containers",
		Run: func(cmd *cobra.Command, args []string) {
			mgr := container.NewManager(context.Background())

			project := config.Env().Project
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

	// Subcommand: workers health
	var restart bool
	healthCmd := &cobra.Command{
		Use:   "health",
		Short: "Check worker health status",
		Long: `Check health status of all workers.

With --restart flag, automatically restarts unhealthy workers.`,
		Run: func(cmd *cobra.Command, args []string) {
			mgr := container.NewManager(context.Background())
			project := config.Env().Project
			workers := mgr.ListWorkers(project)

			if len(workers) == 0 {
				fmt.Println("No workers to check")
				return
			}

			var unhealthyCount int
			for _, w := range workers {
				health := mgr.CheckWorkerHealth(w.Name)
				status := health.Status
				if health.Health != "" {
					status = health.Health
				}

				icon := "✓"
				if health.Status == "unhealthy" || health.Status == "exited" {
					icon = "✗"
					unhealthyCount++
				} else if health.Status == "starting" {
					icon = "…"
				}

				fmt.Printf("%s %s: %s\n", icon, w.Name, status)
			}

			if restart && unhealthyCount > 0 {
				fmt.Println()
				restarted := mgr.MonitorAndRestartUnhealthy(project)
				for _, name := range restarted {
					fmt.Printf("↻ Restarted: %s\n", name)
				}
			}
		},
	}
	healthCmd.Flags().BoolVarP(&restart, "restart", "r", false, "Restart unhealthy workers")
	cmd.AddCommand(healthCmd)

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

func execCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "exec <container> <command>",
		Short: "Execute command in a container",
		Long: `Execute a command inside a running URP container.

Examples:
  urp exec urp-master-myproject "pytest tests/ -v"
  urp exec urp-myproject-w1 "pip install httpx"`,
		Args: cobra.MinimumNArgs(2),
		Run: func(cmd *cobra.Command, args []string) {
			containerName := args[0]
			command := strings.Join(args[1:], " ")

			mgr := container.NewManager(context.Background())

			if err := mgr.Exec(containerName, command); err != nil {
				fmt.Fprintf(os.Stderr, "Error: %v\n", err)
				os.Exit(1)
			}
		},
	}

	return cmd
}

func askCmd() *cobra.Command {
	var useClaude bool

	cmd := &cobra.Command{
		Use:   "ask <container> <prompt>",
		Short: "Send a prompt to a container's OpenCode agent",
		Long: `Send a prompt to the OpenCode agent running in a container.

This is the primary way to interact with containers when running
without a TTY (e.g., from another Claude session).

Uses the URP OpenCode agent by default. Use --claude to use Claude Code CLI instead.

Examples:
  urp ask urp-master-myproject "Run tests and report results"
  urp ask urp-proj-w1 "Create a hello world main.go"
  urp ask --claude urp-proj-w1 "Use Claude CLI instead"`,
		Args: cobra.MinimumNArgs(2),
		Run: func(cmd *cobra.Command, args []string) {
			containerName := args[0]
			prompt := strings.Join(args[1:], " ")

			mgr := container.NewManager(context.Background())

			var command string
			escapedPrompt := strings.ReplaceAll(prompt, `"`, `\"`)
			escapedPrompt = strings.ReplaceAll(escapedPrompt, `$`, `\$`)

			if useClaude {
				// Legacy: use Claude Code CLI
				command = fmt.Sprintf(`set -a && source /etc/urp/.env && set +a && su -c 'claude -p --dangerously-skip-permissions "%s"' urp`, escapedPrompt)
			} else {
				// Default: use URP OpenCode agent
				command = fmt.Sprintf(`set -a && source /etc/urp/.env && set +a && cd /workspace && urp oc agent -p "%s"`, escapedPrompt)
			}

			if err := mgr.Exec(containerName, command); err != nil {
				fmt.Fprintf(os.Stderr, "Error: %v\n", err)
				os.Exit(1)
			}
		},
	}

	cmd.Flags().BoolVar(&useClaude, "claude", false, "Use Claude Code CLI instead of OpenCode agent")

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
				project := config.Env().Project
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

func nemoCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "nemo",
		Short: "NeMo GPU container control",
		Long: `Control NeMo GPU container for ML/training tasks.

Workers can launch NeMo to delegate GPU-intensive operations.
The NeMo container has full NVIDIA GPU access and PyTorch/NeMo stack.

Examples:
  urp nemo start           # Launch NeMo container
  urp nemo exec "pytest"   # Run command in NeMo
  urp nemo stop            # Stop NeMo container`,
	}

	// urp nemo start
	startCmd := &cobra.Command{
		Use:   "start",
		Short: "Start NeMo GPU container",
		Run: func(cmd *cobra.Command, args []string) {
			mgr := container.NewManager(context.Background())

			// Use host path from env or workspace
			projectPath := config.Env().HostPath
			if projectPath == "" {
				projectPath = getCwd()
			}

			name, err := mgr.LaunchNeMo(projectPath, "")
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error: %v\n", err)
				os.Exit(1)
			}

			fmt.Printf("✓ NeMo started: %s\n", name)
			fmt.Printf("  Run commands: urp nemo exec \"python train.py\"\n")
		},
	}

	// urp nemo exec <command>
	execCmd := &cobra.Command{
		Use:   "exec <command>",
		Short: "Execute command in NeMo container",
		Args:  cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			mgr := container.NewManager(context.Background())

			projectName := config.Env().Project
			if projectName == "" {
				projectName = filepath.Base(getCwd())
			}
			containerName := fmt.Sprintf("urp-nemo-%s", projectName)

			output, err := mgr.ExecNeMo(containerName, args[0])
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error: %v\n", err)
				os.Exit(1)
			}

			fmt.Println(output)
		},
	}

	// urp nemo stop
	stopCmd := &cobra.Command{
		Use:   "stop",
		Short: "Stop NeMo container",
		Run: func(cmd *cobra.Command, args []string) {
			mgr := container.NewManager(context.Background())

			projectName := config.Env().Project
			if projectName == "" {
				projectName = filepath.Base(getCwd())
			}
			containerName := fmt.Sprintf("urp-nemo-%s", projectName)

			if err := mgr.KillNeMo(containerName); err != nil {
				fmt.Fprintf(os.Stderr, "Error: %v\n", err)
				os.Exit(1)
			}

			fmt.Printf("✓ NeMo stopped: %s\n", containerName)
		},
	}

	cmd.AddCommand(startCmd, execCmd, stopCmd)
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

	// urp plan show [plan_id]
	showCmd := &cobra.Command{
		Use:   "show [plan_id]",
		Short: "Show plan details (latest if no ID given)",
		Args:  cobra.MaximumNArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			if db == nil {
				fmt.Fprintln(os.Stderr, "Error: Not connected to graph")
				os.Exit(1)
			}

			planner := getPlanner()
			var plan *planning.Plan
			var err error

			if len(args) > 0 {
				plan, err = planner.GetPlan(context.Background(), args[0])
			} else {
				// Get latest plan
				plans, listErr := planner.ListPlans(context.Background(), 1)
				if listErr != nil {
					fmt.Fprintln(os.Stderr, "No plans found")
					return
				}
				if len(plans) == 0 {
					fmt.Fprintln(os.Stderr, "No plans found")
					return
				}
				plan = &plans[0]
			}
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

			workerID := config.Env().WorkerID
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

			workerID := config.Env().WorkerID
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

			workerID := config.Env().WorkerID
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
			workerID := config.Env().WorkerID
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

	reporter.Progress(0.3, "executing command")

	// Execute the task description as a shell command
	// If it starts with "urp ", execute directly, otherwise use shell
	command := task.Description
	var cmd *exec.Cmd

	if strings.HasPrefix(command, "urp ") {
		// Direct urp command
		args := strings.Fields(command)[1:] // Remove "urp" prefix
		cmd = exec.CommandContext(ctx, "urp", args...)
	} else {
		// Shell command
		cmd = exec.CommandContext(ctx, "sh", "-c", command)
	}
	cmd.Dir = "/workspace"

	reporter.Progress(0.5, "running")
	output, err := cmd.CombinedOutput()

	if err != nil {
		reporter.Failed(fmt.Sprintf("command failed: %v\n%s", err, string(output)), 1)
		return err
	}

	reporter.Progress(1.0, "completed")
	reporter.Complete(strings.TrimSpace(string(output)), nil, "")
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

	// urp orchestrate container <command> - Run in real containers
	var containerWorkers int
	var containerTimeout int
	containerCmd := &cobra.Command{
		Use:   "container <command>",
		Short: "Run command in containerized workers",
		Long: `Execute tasks using Docker/Podman containers as workers.

Each worker runs in an isolated container with read-write access
to the project directory. Communication uses the envelope protocol.

Examples:
  urp orchestrate container "go test ./..."
  urp orchestrate container "urp code dead" -n 3`,
		Args: cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			ctx, cancel := context.WithTimeout(context.Background(), time.Duration(containerTimeout)*time.Second)
			defer cancel()

			// Get absolute project path
			projectPath, _ := filepath.Abs(".")

			orch := orchestrator.New()
			defer orch.Shutdown()

			orch.OnWorkerReady = func(workerID string, caps []string) {
				fmt.Printf("  ◉ [%s] ready\n", workerID)
			}

			orch.OnTaskStarted = func(workerID, taskID string) {
				fmt.Printf("  ► [%s] started %s\n", workerID, taskID)
			}

			orch.OnTaskComplete = func(workerID, taskID string, result *orchestrator.TaskResult) {
				if result.Success {
					fmt.Printf("  ✓ [%s] completed (%.1fs)\n", workerID, result.Duration.Seconds())
				} else {
					fmt.Printf("  ✗ [%s] failed: %s\n", workerID, result.Error)
				}
			}

			fmt.Printf("CONTAINER ORCHESTRATION: %d workers\n", containerWorkers)
			fmt.Printf("Command: %s\n", args[0])
			fmt.Printf("Project: %s\n", projectPath)
			fmt.Println()

			// Spawn container workers
			for i := 1; i <= containerWorkers; i++ {
				workerID := fmt.Sprintf("urp-orch-w%d", i)
				if err := orch.SpawnWorkerContainer(ctx, workerID, projectPath); err != nil {
					fmt.Fprintf(os.Stderr, "Error spawning worker %s: %v\n", workerID, err)
					os.Exit(1)
				}
			}

			// Wait for workers to be ready
			fmt.Println("Waiting for workers...")
			readyCtx, readyCancel := context.WithTimeout(ctx, 30*time.Second)
			defer readyCancel()

			readyCount := 0
			for readyCount < containerWorkers {
				select {
				case <-readyCtx.Done():
					fmt.Fprintf(os.Stderr, "Error: timeout waiting for workers (%d/%d ready)\n", readyCount, containerWorkers)
					os.Exit(1)
				case <-orch.WorkerReadyCh():
					readyCount++
				}
			}
			fmt.Println("All workers ready")
			fmt.Println()

			// Assign tasks
			workerIDs := orch.ListWorkers()
			taskIDs := make([]string, len(workerIDs))
			for i, wID := range workerIDs {
				taskID := fmt.Sprintf("task-%d", i+1)
				taskIDs[i] = taskID
				if err := orch.AssignTask(taskID, wID, args[0]); err != nil {
					fmt.Fprintf(os.Stderr, "Error assigning task: %v\n", err)
				}
			}

			// Wait for completion
			results, err := orch.WaitForAll(ctx, taskIDs)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error: %v\n", err)
				os.Exit(1)
			}

			// Summary
			fmt.Println()
			fmt.Println("RESULTS:")
			successes := 0
			for _, taskID := range taskIDs {
				result := results[taskID]
				if result == nil {
					fmt.Printf("  ? %s: no result\n", taskID)
					continue
				}
				if result.Success {
					successes++
					fmt.Printf("  ✓ %s:\n", taskID)
					for _, line := range strings.Split(result.Output, "\n") {
						if line != "" {
							fmt.Printf("      %s\n", line)
						}
					}
				} else {
					fmt.Printf("  ✗ %s: %s\n", taskID, result.Error)
				}
			}
			fmt.Println()
			fmt.Printf("Summary: %d/%d succeeded\n", successes, containerWorkers)

			if successes < containerWorkers {
				os.Exit(1)
			}
		},
	}
	containerCmd.Flags().IntVarP(&containerWorkers, "workers", "n", 1, "Number of container workers")
	containerCmd.Flags().IntVarP(&containerTimeout, "timeout", "t", 120, "Timeout in seconds")

	cmd.AddCommand(runCmd, shellCmd, demoCmd, historyCmd, statsCmd, containerCmd)
	return cmd
}

// ══════════════════════════════════════════════════════════════════════════════
// AUDIT COMMANDS
// ══════════════════════════════════════════════════════════════════════════════

