// Package main provides the URP CLI entrypoint.
package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/joss/urp/internal/audit"
	"github.com/joss/urp/internal/config"
	"github.com/joss/urp/internal/graph"
	"github.com/joss/urp/internal/memory"
	"github.com/joss/urp/internal/render"
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
				db = nil
			}

			// Configure logging to persist events to graph
			if db != nil {
				audit.SetGraphDriver(db)
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
			workDir, _ := os.Getwd()
			if len(args) > 0 {
				path := args[0]
				if !filepath.IsAbs(path) {
					path = filepath.Join(workDir, path)
				}
				workDir = path
			}

			// Validate workDir exists and is a directory
			info, err := os.Stat(workDir)
			if err != nil {
				if os.IsNotExist(err) {
					fatalErrorf("path does not exist: %s", workDir)
				}
				fatalErrorf("cannot access path: %v", err)
			}
			if !info.IsDir() {
				fatalErrorf("path is not a directory: %s", workDir)
			}

			useTUI, _ := cmd.Flags().GetBool("tui")
			if useTUI {
				runInteractiveAgent(workDir)
				return
			}
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

	// Register all commands
	registerInfraCommands(rootCmd)
	registerAnalysisCommands(rootCmd)
	registerCognitiveCommands(rootCmd)
	registerRuntimeCommands(rootCmd)

	// Ungrouped
	rootCmd.AddCommand(versionCmd())
	rootCmd.AddCommand(statusCmd())
	rootCmd.AddCommand(tuiCmd())

	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

// registerInfraCommands adds infrastructure group commands
func registerInfraCommands(root *cobra.Command) {
	cmds := []*cobra.Command{
		infraCmd(),
		launchCmd(),
		spawnCmd(),
		workersCmd(),
		attachCmd(),
		execCmd(),
		killCmd(),
		askCmd(),
		nemoCmd(),
		workerCmd(),
		orchestrateCmd(),
		doctorCmd(),
	}
	for _, c := range cmds {
		c.GroupID = "infra"
		root.AddCommand(c)
	}
}

// registerAnalysisCommands adds analysis group commands
func registerAnalysisCommands(root *cobra.Command) {
	cmds := []*cobra.Command{
		codeCmd(),
		gitCmd(),
		focusCmd(),
	}
	for _, c := range cmds {
		c.GroupID = "analysis"
		root.AddCommand(c)
	}
}

// registerCognitiveCommands adds cognitive group commands
func registerCognitiveCommands(root *cobra.Command) {
	cmds := []*cobra.Command{
		thinkCmd(),
		memCmd(),
		kbCmd(),
		vecCmd(),
		planCmd(),
		opencodeCmd(),
		skillsCmd(),
		backupCmd(),
		specCmd(),
	}
	for _, c := range cmds {
		c.GroupID = "cognitive"
		root.AddCommand(c)
	}
}

// registerRuntimeCommands adds runtime group commands
func registerRuntimeCommands(root *cobra.Command) {
	cmds := []*cobra.Command{
		sysCmd(),
		eventsCmd(),
		sessionCmd(),
		auditCmd(),
		alertCmd(),
	}
	for _, c := range cmds {
		c.GroupID = "runtime"
		root.AddCommand(c)
	}
}

// statusCmd shows infrastructure status
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
				fatalError(err)
			}
		},
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
	var verbose, quick bool

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

func focusCmd() *cobra.Command {
	var depth int

	cmd := &cobra.Command{
		Use:   "focus <target>",
		Short: "Load focused context around a target",
		Long:  "Load minimal context for surgical precision (reduces hallucination)",
		Args:  cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			requireDBSimple()

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

// runInteractiveAgent starts an interactive OpenCode session
func runInteractiveAgent(workDir string) {
	if _, err := os.Stat(workDir); os.IsNotExist(err) {
		fatalErrorf("directory does not exist: %s", workDir)
	}

	if config.Env().ContainerMode {
		cmd := exec.Command("claude")
		cmd.Dir = workDir
		cmd.Stdin = os.Stdin
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		if err := cmd.Run(); err != nil {
			fatalError(err)
		}
		return
	}

	if err := tui.RunAgent(workDir); err != nil {
		fatalError(err)
	}
}

// runInteractiveAgentDebug runs agent with static output (debug mode)
func runInteractiveAgentDebug(workDir string) {
	if _, err := os.Stat(workDir); os.IsNotExist(err) {
		fatalErrorf("directory does not exist: %s", workDir)
	}

	fmt.Println("🔧 URP Debug Mode")
	fmt.Printf("📁 Working directory: %s\n", workDir)

	if err := tui.RunAgentDebug(workDir); err != nil {
		fatalError(err)
	}
}

func showStatus() {
	project := config.Env().Project
	if project == "" {
		project = "unknown"
	}

	connected := false
	eventCount := 0

	if db != nil {
		if err := db.Ping(context.Background()); err == nil {
			connected = true
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
