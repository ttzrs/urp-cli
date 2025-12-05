// Package planning provides task orchestration for master/worker pattern.
package planning

import (
	"context"
	"fmt"
	"time"

	"github.com/joss/urp/internal/graph"
)

// Planner manages plans and tasks in the graph.
type Planner struct {
	db        graph.Driver
	sessionID string
}

// NewPlanner creates a new planner.
func NewPlanner(db graph.Driver, sessionID string) *Planner {
	return &Planner{db: db, sessionID: sessionID}
}

// CreatePlan creates a new plan with tasks.
func (p *Planner) CreatePlan(ctx context.Context, description string, taskDescriptions []string) (*Plan, error) {
	planID := fmt.Sprintf("plan-%d", time.Now().UnixNano())
	now := time.Now().UTC().Format(time.RFC3339)

	// Create plan node
	query := `
		MERGE (sess:Session {session_id: $session_id})
		CREATE (plan:Plan {
			plan_id: $plan_id,
			description: $description,
			status: $status,
			created_at: $now,
			updated_at: $now
		})
		CREATE (sess)-[:CREATED_PLAN {at: $now}]->(plan)
	`

	err := p.db.ExecuteWrite(ctx, query, map[string]any{
		"session_id":  p.sessionID,
		"plan_id":     planID,
		"description": description,
		"status":      string(PlanPending),
		"now":         now,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create plan: %w", err)
	}

	// Create tasks
	var tasks []Task
	for i, desc := range taskDescriptions {
		taskID := fmt.Sprintf("task-%d-%d", time.Now().UnixNano(), i)
		task := Task{
			TaskID:      taskID,
			PlanID:      planID,
			Description: desc,
			Status:      TaskPending,
			Order:       i + 1,
			CreatedAt:   now,
		}

		taskQuery := `
			MATCH (plan:Plan {plan_id: $plan_id})
			CREATE (task:Task {
				task_id: $task_id,
				description: $description,
				status: $status,
				task_order: $order,
				created_at: $now
			})
			CREATE (plan)-[:HAS_TASK]->(task)
		`

		err := p.db.ExecuteWrite(ctx, taskQuery, map[string]any{
			"plan_id":     planID,
			"task_id":     taskID,
			"description": desc,
			"status":      string(TaskPending),
			"order":       i + 1,
			"now":         now,
		})
		if err != nil {
			return nil, fmt.Errorf("failed to create task %d: %w", i, err)
		}

		tasks = append(tasks, task)
	}

	return &Plan{
		PlanID:      planID,
		Description: description,
		Status:      PlanPending,
		CreatedAt:   now,
		UpdatedAt:   now,
		SessionID:   p.sessionID,
		Tasks:       tasks,
	}, nil
}

// GetPlan retrieves a plan with its tasks.
func (p *Planner) GetPlan(ctx context.Context, planID string) (*Plan, error) {
	query := `
		MATCH (plan:Plan {plan_id: $plan_id})
		OPTIONAL MATCH (plan)-[:HAS_TASK]->(task:Task)
		RETURN plan.plan_id as plan_id,
		       plan.description as description,
		       plan.status as status,
		       plan.created_at as created_at,
		       plan.updated_at as updated_at,
		       collect({
		           task_id: task.task_id,
		           description: task.description,
		           status: task.status,
		           worker_id: task.worker_id,
		           task_order: task.task_order,
		           created_at: task.created_at,
		           started_at: task.started_at,
		           completed_at: task.completed_at
		       }) as tasks
	`

	records, err := p.db.Execute(ctx, query, map[string]any{
		"plan_id": planID,
	})
	if err != nil {
		return nil, err
	}

	if len(records) == 0 {
		return nil, fmt.Errorf("plan not found: %s", planID)
	}

	r := records[0]
	plan := &Plan{
		PlanID:      graph.GetString(r, "plan_id"),
		Description: graph.GetString(r, "description"),
		Status:      PlanStatus(graph.GetString(r, "status")),
		CreatedAt:   graph.GetString(r, "created_at"),
		UpdatedAt:   graph.GetString(r, "updated_at"),
	}

	// Parse tasks
	if tasksRaw, ok := r["tasks"].([]any); ok {
		for _, t := range tasksRaw {
			if tm, ok := t.(map[string]any); ok {
				if tm["task_id"] == nil {
					continue
				}
				plan.Tasks = append(plan.Tasks, Task{
					TaskID:      getStringFrom(tm, "task_id"),
					Description: getStringFrom(tm, "description"),
					Status:      TaskStatus(getStringFrom(tm, "status")),
					WorkerID:    getStringFrom(tm, "worker_id"),
					Order:       getIntFrom(tm, "task_order"),
					CreatedAt:   getStringFrom(tm, "created_at"),
					StartedAt:   getStringFrom(tm, "started_at"),
					CompletedAt: getStringFrom(tm, "completed_at"),
				})
			}
		}
	}

	return plan, nil
}

// AssignTask assigns a task to a worker.
func (p *Planner) AssignTask(ctx context.Context, taskID, workerID string) error {
	now := time.Now().UTC().Format(time.RFC3339)

	query := `
		MATCH (task:Task {task_id: $task_id})
		SET task.status = $status,
		    task.worker_id = $worker_id,
		    task.started_at = $now
	`

	return p.db.ExecuteWrite(ctx, query, map[string]any{
		"task_id":   taskID,
		"worker_id": workerID,
		"status":    string(TaskAssigned),
		"now":       now,
	})
}

// StartTask marks a task as in progress.
func (p *Planner) StartTask(ctx context.Context, taskID string) error {
	now := time.Now().UTC().Format(time.RFC3339)

	query := `
		MATCH (task:Task {task_id: $task_id})
		SET task.status = $status,
		    task.started_at = CASE WHEN task.started_at IS NULL THEN $now ELSE task.started_at END
	`

	return p.db.ExecuteWrite(ctx, query, map[string]any{
		"task_id": taskID,
		"status":  string(TaskInProgress),
		"now":     now,
	})
}

// CompleteTask marks a task as completed and records the result.
func (p *Planner) CompleteTask(ctx context.Context, taskID, workerID, output string, filesChanged []string) (*Result, error) {
	now := time.Now().UTC().Format(time.RFC3339)
	resultID := fmt.Sprintf("result-%d", time.Now().UnixNano())

	query := `
		MATCH (task:Task {task_id: $task_id})
		SET task.status = $status,
		    task.completed_at = $now
		CREATE (result:Result {
			result_id: $result_id,
			worker_id: $worker_id,
			status: 'success',
			output: $output,
			files_changed: $files_changed,
			created_at: $now
		})
		CREATE (task)-[:HAS_RESULT]->(result)
	`

	err := p.db.ExecuteWrite(ctx, query, map[string]any{
		"task_id":       taskID,
		"result_id":     resultID,
		"worker_id":     workerID,
		"status":        string(TaskCompleted),
		"output":        output,
		"files_changed": filesChanged,
		"now":           now,
	})
	if err != nil {
		return nil, err
	}

	// Update plan status
	p.updatePlanStatus(ctx, taskID)

	return &Result{
		ResultID:     resultID,
		TaskID:       taskID,
		WorkerID:     workerID,
		Status:       "success",
		Output:       output,
		FilesChanged: filesChanged,
		CreatedAt:    now,
	}, nil
}

// FailTask marks a task as failed.
func (p *Planner) FailTask(ctx context.Context, taskID, workerID, errorMsg string) (*Result, error) {
	now := time.Now().UTC().Format(time.RFC3339)
	resultID := fmt.Sprintf("result-%d", time.Now().UnixNano())

	query := `
		MATCH (task:Task {task_id: $task_id})
		SET task.status = $status,
		    task.completed_at = $now
		CREATE (result:Result {
			result_id: $result_id,
			worker_id: $worker_id,
			status: 'failure',
			error: $error,
			created_at: $now
		})
		CREATE (task)-[:HAS_RESULT]->(result)
	`

	err := p.db.ExecuteWrite(ctx, query, map[string]any{
		"task_id":   taskID,
		"result_id": resultID,
		"worker_id": workerID,
		"status":    string(TaskFailed),
		"error":     errorMsg,
		"now":       now,
	})
	if err != nil {
		return nil, err
	}

	// Update plan status
	p.updatePlanStatus(ctx, taskID)

	return &Result{
		ResultID:  resultID,
		TaskID:    taskID,
		WorkerID:  workerID,
		Status:    "failure",
		Error:     errorMsg,
		CreatedAt: now,
	}, nil
}

// GetNextTask returns the next pending task for assignment.
func (p *Planner) GetNextTask(ctx context.Context, planID string) (*Task, error) {
	query := `
		MATCH (plan:Plan {plan_id: $plan_id})-[:HAS_TASK]->(task:Task)
		WHERE task.status = 'pending'
		RETURN task.task_id as task_id,
		       task.description as description,
		       task.status as status,
		       task.task_order as task_order,
		       task.created_at as created_at
		ORDER BY task.task_order
		LIMIT 1
	`

	records, err := p.db.Execute(ctx, query, map[string]any{
		"plan_id": planID,
	})
	if err != nil {
		return nil, err
	}

	if len(records) == 0 {
		return nil, nil // No pending tasks
	}

	r := records[0]
	return &Task{
		TaskID:      graph.GetString(r, "task_id"),
		PlanID:      planID,
		Description: graph.GetString(r, "description"),
		Status:      TaskStatus(graph.GetString(r, "status")),
		Order:       graph.GetInt(r, "task_order"),
		CreatedAt:   graph.GetString(r, "created_at"),
	}, nil
}

// ListPlans returns all plans for the session.
func (p *Planner) ListPlans(ctx context.Context, limit int) ([]Plan, error) {
	query := `
		MATCH (sess:Session {session_id: $session_id})-[:CREATED_PLAN]->(plan:Plan)
		RETURN plan.plan_id as plan_id,
		       plan.description as description,
		       plan.status as status,
		       plan.created_at as created_at,
		       plan.updated_at as updated_at
		ORDER BY plan.created_at DESC
		LIMIT $limit
	`

	records, err := p.db.Execute(ctx, query, map[string]any{
		"session_id": p.sessionID,
		"limit":      limit,
	})
	if err != nil {
		return nil, err
	}

	var plans []Plan
	for _, r := range records {
		plans = append(plans, Plan{
			PlanID:      graph.GetString(r, "plan_id"),
			Description: graph.GetString(r, "description"),
			Status:      PlanStatus(graph.GetString(r, "status")),
			CreatedAt:   graph.GetString(r, "created_at"),
			UpdatedAt:   graph.GetString(r, "updated_at"),
		})
	}

	return plans, nil
}

// GetTask retrieves a single task by ID.
func (p *Planner) GetTask(ctx context.Context, taskID string) (*Task, error) {
	query := `
		MATCH (task:Task {task_id: $task_id})
		RETURN task.task_id as task_id,
		       task.description as description,
		       task.status as status,
		       task.worker_id as worker_id,
		       task.task_order as task_order
	`

	records, err := p.db.Execute(ctx, query, map[string]any{
		"task_id": taskID,
	})
	if err != nil {
		return nil, err
	}

	if len(records) == 0 {
		return nil, fmt.Errorf("task not found: %s", taskID)
	}

	r := records[0]
	return &Task{
		TaskID:      graph.GetString(r, "task_id"),
		Description: graph.GetString(r, "description"),
		Status:      TaskStatus(graph.GetString(r, "status")),
		WorkerID:    graph.GetString(r, "worker_id"),
		Order:       graph.GetInt(r, "task_order"),
	}, nil
}

// CompleteTaskWithPR marks task as completed and creates a PR if there are commits.
func (p *Planner) CompleteTaskWithPR(ctx context.Context, taskID, workerID, output string, filesChanged []string, repoPath, baseBranch string) (*Result, *PRResult, error) {
	// First complete the task in the graph
	result, err := p.CompleteTask(ctx, taskID, workerID, output, filesChanged)
	if err != nil {
		return nil, nil, err
	}

	// Check if we have a branch (from env or task metadata)
	branch := getBranchForTask(repoPath)
	if branch == "" || branch == baseBranch {
		// No separate branch, no PR needed
		return result, nil, nil
	}

	// Check if there are commits to PR
	if !hasCommits(repoPath, baseBranch, branch) {
		return result, nil, nil
	}

	// Get task description for PR title
	task, err := p.GetTask(ctx, taskID)
	if err != nil {
		return result, nil, nil // Task completed, PR failed is acceptable
	}

	title := fmt.Sprintf("[URP] %s", task.Description)
	body := fmt.Sprintf("## Task Completed\n\n%s\n\n**Worker:** %s\n**Output:** %s\n\n---\n*Created by URP planning system*",
		task.Description, workerID, output)

	pr, err := CreatePR(repoPath, branch, baseBranch, title, body)
	if err != nil {
		// Task completed but PR failed - return result anyway
		return result, nil, fmt.Errorf("task completed but PR failed: %w", err)
	}

	// Store PR URL in result metadata
	p.storePRInfo(ctx, taskID, pr.URL)

	return result, pr, nil
}

// updatePlanStatus updates the plan status based on task completion.
func (p *Planner) updatePlanStatus(ctx context.Context, taskID string) {
	// Get plan ID from task
	query := `
		MATCH (plan:Plan)-[:HAS_TASK]->(task:Task {task_id: $task_id})
		OPTIONAL MATCH (plan)-[:HAS_TASK]->(allTasks:Task)
		WITH plan,
		     count(allTasks) as total,
		     sum(CASE WHEN allTasks.status = 'completed' THEN 1 ELSE 0 END) as completed,
		     sum(CASE WHEN allTasks.status = 'failed' THEN 1 ELSE 0 END) as failed
		SET plan.status = CASE
			WHEN failed > 0 THEN 'failed'
			WHEN completed = total THEN 'completed'
			ELSE 'in_progress'
		END,
		plan.updated_at = $now
	`

	p.db.ExecuteWrite(ctx, query, map[string]any{
		"task_id": taskID,
		"now":     time.Now().UTC().Format(time.RFC3339),
	})
}

// storePRInfo stores the PR URL in the task's result.
func (p *Planner) storePRInfo(ctx context.Context, taskID, prURL string) {
	query := `
		MATCH (task:Task {task_id: $task_id})-[:HAS_RESULT]->(r:Result)
		SET r.pr_url = $pr_url
	`
	p.db.ExecuteWrite(ctx, query, map[string]any{
		"task_id": taskID,
		"pr_url":  prURL,
	})
}

func getStringFrom(m map[string]any, key string) string {
	if v, ok := m[key]; ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}

func getIntFrom(m map[string]any, key string) int {
	if v, ok := m[key]; ok {
		switch n := v.(type) {
		case int64:
			return int(n)
		case int:
			return n
		case float64:
			return int(n)
		}
	}
	return 0
}
