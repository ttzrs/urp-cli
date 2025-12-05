// Package main planning commands for master/worker orchestration.
package main

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/joss/urp/internal/config"
	"github.com/joss/urp/internal/memory"
	"github.com/joss/urp/internal/planning"
)

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
			requireDBSimple()

			description := args[0]
			tasks := args[1:]

			planner := getPlanner()
			plan, err := planner.CreatePlan(context.Background(), description, tasks)
			if err != nil {
				fatalError(err)
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
			requireDBSimple()

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
				fatalError(err)
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
			requireDBSimple()

			planner := getPlanner()
			plans, err := planner.ListPlans(context.Background(), limit)
			if err != nil {
				fatalError(err)
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
			requireDBSimple()

			planner := getPlanner()
			task, err := planner.GetNextTask(context.Background(), args[0])
			if err != nil {
				fatalError(err)
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
			requireDBSimple()

			planner := getPlanner()
			if err := planner.AssignTask(context.Background(), args[0], args[1]); err != nil {
				fatalError(err)
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
			requireDBSimple()

			planner := getPlanner()
			if err := planner.StartTask(context.Background(), args[0]); err != nil {
				fatalError(err)
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
			requireDBSimple()

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
				fatalError(err)
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
			requireDBSimple()

			workerID := config.Env().WorkerID
			if workerID == "" {
				workerID = "manual"
			}

			planner := getPlanner()
			result, err := planner.FailTask(context.Background(), args[0], workerID, args[1])
			if err != nil {
				fatalError(err)
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
			requireDBSimple()

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
				fatalError(err)
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
			requireDBSimple()

			// Get PR URL from task result
			query := `
				MATCH (task:Task {task_id: $task_id})-[:HAS_RESULT]->(r:Result)
				RETURN r.pr_url as pr_url
			`
			records, err := db.Execute(context.Background(), query, map[string]any{
				"task_id": args[0],
			})
			if err != nil || len(records) == 0 {
				fatalErrorf("Task or PR not found")
			}

			prURL, ok := records[0]["pr_url"].(string)
			if !ok || prURL == "" {
				fatalErrorf("No PR associated with this task")
			}

			// Extract PR number from URL (format: .../pull/123)
			parts := strings.Split(prURL, "/")
			if len(parts) < 2 {
				fatalErrorf("Invalid PR URL")
			}
			var prNum int
			fmt.Sscanf(parts[len(parts)-1], "%d", &prNum)

			repoPath := getCwd()
			if err := planning.MergePR(repoPath, prNum, squash); err != nil {
				fatalError(err)
			}

			fmt.Printf("✓ PR #%d merged\n", prNum)
			fmt.Printf("  Task: %s\n", args[0])
		},
	}
	mergeCmd.Flags().BoolVarP(&squash, "squash", "s", false, "Squash merge")

	cmd.AddCommand(createCmd, showCmd, listCmd, nextCmd, assignCmd, startCmd, completeCmd, failCmd, doneCmd, mergeCmd)
	return cmd
}
