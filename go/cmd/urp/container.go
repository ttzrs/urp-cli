// Package main container orchestration commands.
package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
	"golang.org/x/term"

	"github.com/joss/urp/internal/config"
	"github.com/joss/urp/internal/container"
	"github.com/joss/urp/internal/selftest"
)

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
				fatalError(err)
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
				fatalError(err)
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
				fatalError(err)
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
				fatalError(err)
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
					fatalError(err)
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
					fatalError(err)
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
				fatalErrorf("spawn must be run from inside a master container\nUse 'urp launch' first")
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
				fatalError(err)
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
				fatalError(err)
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
				fatalError(err)
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
				fatalError(err)
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
					fatalError(err)
				}
				fmt.Println("✓ All workers killed")
				return
			}

			if len(args) == 0 {
				fatalErrorf("container name required (or use --all)")
			}

			if err := mgr.KillWorker(args[0]); err != nil {
				fatalError(err)
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
				fatalError(err)
			}

			fmt.Printf("✓ NeMo started: %s\n", name)
			fmt.Printf("  Run commands: urp nemo exec \"python train.py\"\n")
		},
	}

	// urp nemo exec <command>
	execNemoCmd := &cobra.Command{
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
				fatalError(err)
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
				fatalError(err)
			}

			fmt.Printf("✓ NeMo stopped: %s\n", containerName)
		},
	}

	cmd.AddCommand(startCmd, execNemoCmd, stopCmd)
	return cmd
}
