// Package main multi-agent orchestration commands.
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

	"github.com/joss/urp/internal/graph"
	"github.com/joss/urp/internal/memory"
	"github.com/joss/urp/internal/orchestrator"
	"github.com/joss/urp/internal/protocol"
)

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
				fatalError(err)
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
				fatalError(err)
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
				dbConn, err := graph.Connect()
				if err == nil {
					defer dbConn.Close()
					sessCtx := memory.NewContext()
					po := orchestrator.NewPersistentOrchestrator(dbConn, sessCtx.SessionID)

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
			dbConn, err := graph.Connect()
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error: cannot connect to Memgraph: %v\n", err)
				fmt.Println("Run 'urp infra start' to start Memgraph")
				os.Exit(1)
			}
			defer dbConn.Close()

			// Get session ID
			sessCtx := memory.NewContext()

			// Create persistent orchestrator
			po := orchestrator.NewPersistentOrchestrator(dbConn, sessCtx.SessionID)

			runs, err := po.ListRuns(ctx, historyLimit)
			if err != nil {
				fatalError(err)
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

			dbConn, err := graph.Connect()
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error: cannot connect to Memgraph: %v\n", err)
				os.Exit(1)
			}
			defer dbConn.Close()

			sessCtx := memory.NewContext()
			po := orchestrator.NewPersistentOrchestrator(dbConn, sessCtx.SessionID)

			stats, err := po.GetRunStats(ctx)
			if err != nil {
				fatalError(err)
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
				fatalError(err)
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
