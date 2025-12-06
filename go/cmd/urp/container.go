// Package main container orchestration commands.
package main

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"golang.org/x/term"

	"github.com/joss/urp/internal/config"
	"github.com/joss/urp/internal/container"
	"github.com/joss/urp/internal/selftest"
)

// ─────────────────────────────────────────────────────────────────────────────
// Service factory (DIP: abstracts container.Service creation)
// ─────────────────────────────────────────────────────────────────────────────

func newContainerSvc() *container.Service {
	return container.NewService(context.Background())
}

func newContainerSvcForProject() *container.Service {
	return container.NewServiceForProject(context.Background(), config.Env().Project)
}

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
			svc := newContainerSvc()
			status := svc.Status()
			renderInfraStatus(status)
		},
	}

	// urp infra start
	startCmd := &cobra.Command{
		Use:   "start",
		Short: "Start infrastructure (network, memgraph)",
		Run: func(cmd *cobra.Command, args []string) {
			svc := newContainerSvcForProject()
			result := svc.StartInfra()
			if result.Error != nil {
				fatalError(result.Error)
			}
			renderInfraStarted(result)
		},
	}

	// urp infra stop
	stopCmd := &cobra.Command{
		Use:   "stop",
		Short: "Stop all URP containers for current project",
		Run: func(cmd *cobra.Command, args []string) {
			svc := newContainerSvcForProject()
			result := svc.StopInfra()
			if result.Error != nil {
				fatalError(result.Error)
			}
			fmt.Println("✓ Containers stopped")
		},
	}

	// urp infra clean
	cleanCmd := &cobra.Command{
		Use:   "clean",
		Short: "Remove all URP containers, volumes, and network for current project",
		Run: func(cmd *cobra.Command, args []string) {
			svc := newContainerSvcForProject()
			fmt.Println("Cleaning URP infrastructure...")
			result := svc.CleanInfra()
			if result.Error != nil {
				fatalError(result.Error)
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
			svc := newContainerSvcForProject()
			name := ""
			if len(args) > 0 {
				name = args[0]
			}
			result := svc.Logs(name, tail)
			if result.Error != nil {
				fatalError(result.Error)
			}
			fmt.Printf("=== %s logs (last %d lines) ===\n", result.ContainerName, result.Tail)
			fmt.Println(result.Logs)
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

			svc := newContainerSvc()

			if !worker {
				result := svc.LaunchMaster(path)
				if result.Error != nil {
					fatalError(result.Error)
				}
				renderLaunchMaster(result)
			} else {
				fmt.Printf("Launching standalone for %s...\n", path)
				result := svc.LaunchStandalone(path, readOnly)
				if result.Error != nil {
					fatalError(result.Error)
				}
				renderLaunchStandalone(result)
			}
		},
	}

	cmd.Flags().BoolVarP(&worker, "worker", "w", false, "Launch as standalone daemon (no master/worker flow)")
	cmd.Flags().BoolVarP(&readOnly, "readonly", "r", false, "Read-only access (only with --worker)")

	return cmd
}

func spawnCmd() *cobra.Command {
	var (
		background bool
		parallel   int
	)

	cmd := &cobra.Command{
		Use:   "spawn [num]",
		Short: "Spawn a worker container (from master)",
		Long: `Spawn a worker container with read-write access.
Run from inside master. Send instructions via urp ask.

Examples:
  urp spawn              # Create worker 1
  urp spawn 2            # Create worker 2
  urp spawn --parallel 4 # Create workers 1-4 in parallel
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

			svc := newContainerSvc()
			path := container.ProjectPath()

			// Parallel spawn mode
			if parallel > 1 {
				results := svc.SpawnWorkersParallel(path, parallel)
				renderSpawnParallel(results)
				return
			}

			// Single worker spawn
			result := svc.SpawnWorker(path, workerNum)
			if result.Error != nil {
				fatalError(result.Error)
			}
			renderSpawnSingle(result)
		},
	}

	cmd.Flags().BoolVarP(&background, "detach", "d", false, "Ignored (workers always detached)")
	cmd.Flags().IntVarP(&parallel, "parallel", "n", 0, "Spawn N workers in parallel")

	return cmd
}

func workersCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "workers",
		Short: "List worker containers",
		Run: func(cmd *cobra.Command, args []string) {
			svc := newContainerSvcForProject()
			workers := svc.ListWorkers()
			renderWorkersList(workers)
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
			svc := newContainerSvcForProject()
			result := svc.CheckAllHealth()
			renderWorkersHealth(result, restart)

			if restart && result.Unhealthy > 0 {
				restarted := svc.RestartUnhealthy()
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
			svc := newContainerSvc()
			if err := svc.AttachWorker(args[0]); err != nil {
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
			svc := newContainerSvc()
			command := strings.Join(args[1:], " ")
			if err := svc.Exec(args[0], command); err != nil {
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
			svc := newContainerSvc()
			prompt := strings.Join(args[1:], " ")
			command := buildAskCommand(prompt, useClaude)
			if err := svc.Exec(args[0], command); err != nil {
				fatalError(err)
			}
		},
	}

	cmd.Flags().BoolVar(&useClaude, "claude", false, "Use Claude Code CLI instead of OpenCode agent")

	return cmd
}

// buildAskCommand constructs the shell command for urp ask.
func buildAskCommand(prompt string, useClaude bool) string {
	escaped := strings.ReplaceAll(prompt, `"`, `\"`)
	escaped = strings.ReplaceAll(escaped, `$`, `\$`)
	if useClaude {
		return fmt.Sprintf(`set -a && source /etc/urp/.env && set +a && su -c 'claude -p --dangerously-skip-permissions "%s"' urp`, escaped)
	}
	return fmt.Sprintf(`set -a && source /etc/urp/.env && set +a && cd /workspace && urp oc agent -p "%s"`, escaped)
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
			svc := newContainerSvcForProject()

			if all {
				if err := svc.KillAllWorkers(); err != nil {
					fatalError(err)
				}
				fmt.Println("✓ All workers killed")
				return
			}

			if len(args) == 0 {
				fatalErrorf("container name required (or use --all)")
			}

			if err := svc.KillWorker(args[0]); err != nil {
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
			svc := newContainerSvcForProject()
			result := svc.LaunchNeMo(container.ProjectPath())
			if result.Error != nil {
				fatalError(result.Error)
			}
			fmt.Printf("✓ NeMo started: %s\n", result.ContainerName)
			fmt.Printf("  Run commands: urp nemo exec \"python train.py\"\n")
		},
	}

	// urp nemo exec <command>
	execNemoCmd := &cobra.Command{
		Use:   "exec <command>",
		Short: "Execute command in NeMo container",
		Args:  cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			svc := newContainerSvcForProject()
			output, err := svc.ExecNeMo(args[0])
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
			svc := newContainerSvcForProject()
			name := svc.NeMoContainerName()
			if err := svc.KillNeMo(); err != nil {
				fatalError(err)
			}
			fmt.Printf("✓ NeMo stopped: %s\n", name)
		},
	}

	cmd.AddCommand(startCmd, execNemoCmd, stopCmd)
	return cmd
}

// ─────────────────────────────────────────────────────────────────────────────
// Render functions (presentation layer)
// ─────────────────────────────────────────────────────────────────────────────

func renderInfraStatus(status *container.InfraStatus) {
	fmt.Println("URP INFRASTRUCTURE")
	fmt.Println()

	if status.Runtime == container.RuntimeNone {
		fmt.Println("  Runtime:  NOT FOUND")
		fmt.Println()
		fmt.Println("  Install docker or podman to use URP containers")
		return
	}
	fmt.Printf("  Runtime:  %s\n", status.Runtime)

	project := config.Env().Project
	networkName := container.NetworkName(project)
	if status.Network {
		fmt.Printf("  Network:  %s ✓\n", networkName)
	} else {
		fmt.Printf("  Network:  %s (not created)\n", networkName)
	}

	if status.Memgraph != nil {
		fmt.Printf("  Memgraph: %s (%s)\n", status.Memgraph.Name, status.Memgraph.Status)
		if status.Memgraph.Ports != "" {
			fmt.Printf("            Ports: %s\n", status.Memgraph.Ports)
		}
	} else {
		fmt.Println("  Memgraph: not running")
	}

	fmt.Printf("  Volumes:  %d\n", len(status.Volumes))
	for _, v := range status.Volumes {
		fmt.Printf("            - %s\n", v)
	}

	fmt.Printf("  Workers:  %d\n", len(status.Workers))
	for _, w := range status.Workers {
		fmt.Printf("            - %s (%s)\n", w.Name, w.Status)
	}
}

func renderInfraStarted(result *container.InfraResult) {
	if result.Project != "" {
		fmt.Printf("Starting URP infrastructure for project: %s\n", result.Project)
	} else {
		fmt.Println("Starting URP infrastructure (default)...")
	}
	fmt.Printf("✓ Network: %s\n", result.NetworkName)
	fmt.Println("✓ Volumes created")
	fmt.Printf("✓ Memgraph: %s (no host ports)\n", result.MemgraphName)
	fmt.Println()
	fmt.Println("Infrastructure ready. Use 'urp launch' to start a master.")
}

func renderLaunchMaster(result *container.LaunchResult) {
	if term.IsTerminal(int(os.Stdin.Fd())) {
		fmt.Println("\n✓ Master session ended")
	} else {
		fmt.Printf("✓ Master started: %s\n", result.ContainerName)
		fmt.Println()
		fmt.Println("Container is running in detached mode (no TTY).")
		fmt.Printf("  Execute commands:  urp exec %s <command>\n", result.ContainerName)
		fmt.Printf("  View logs:         docker logs -f %s\n", result.ContainerName)
		fmt.Printf("  Stop:              docker rm -f %s\n", result.ContainerName)
	}
}

func renderLaunchStandalone(result *container.LaunchResult) {
	fmt.Printf("✓ Container started: %s\n", result.ContainerName)
	fmt.Println()
	fmt.Printf("Attach with: urp attach %s\n", result.ContainerName)
}

func renderSpawnSingle(result *container.SpawnResult) {
	fmt.Printf("✓ Worker spawned: %s\n", result.Name)
	fmt.Printf("  Send tasks: urp ask %s \"<instruction>\"\n", result.Name)
	fmt.Printf("  Attach:     urp attach %s\n", result.Name)
}

func renderSpawnParallel(results []*container.SpawnResult) {
	fmt.Printf("Spawning %d workers in parallel...\n", len(results))

	var spawned []string
	var failed int
	for _, r := range results {
		if r.Error != nil {
			fmt.Printf("  ✗ Worker %d: %v\n", r.Num, r.Error)
			failed++
		} else {
			fmt.Printf("  ✓ Worker %d: %s\n", r.Num, r.Name)
			spawned = append(spawned, r.Name)
		}
	}

	fmt.Println()
	fmt.Printf("Summary: %d/%d workers spawned\n", len(spawned), len(results))

	if len(spawned) > 0 {
		fmt.Println("\nSend tasks:")
		for _, name := range spawned {
			fmt.Printf("  urp ask %s \"<instruction>\"\n", name)
		}
	}

	if failed > 0 {
		os.Exit(1)
	}
}

func renderWorkersList(workers []container.ContainerStatus) {
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
}

func renderWorkersHealth(result *container.HealthResult, showRestart bool) {
	if len(result.Workers) == 0 {
		fmt.Println("No workers to check")
		return
	}

	for _, h := range result.Workers {
		status := h.Status
		if h.Health != "" {
			status = h.Health
		}

		icon := "✓"
		if h.Status == "unhealthy" || h.Status == "exited" {
			icon = "✗"
		} else if h.Status == "starting" {
			icon = "…"
		}

		fmt.Printf("%s %s: %s\n", icon, h.Name, status)
	}

	if showRestart && result.Unhealthy > 0 {
		fmt.Println()
	}
}
