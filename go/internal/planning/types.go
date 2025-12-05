// Package planning provides task orchestration for master/worker pattern.
package planning

// PlanStatus represents the state of a plan.
type PlanStatus string

const (
	PlanPending    PlanStatus = "pending"
	PlanInProgress PlanStatus = "in_progress"
	PlanCompleted  PlanStatus = "completed"
	PlanFailed     PlanStatus = "failed"
)

// TaskStatus represents the state of a task.
type TaskStatus string

const (
	TaskPending    TaskStatus = "pending"
	TaskAssigned   TaskStatus = "assigned"
	TaskInProgress TaskStatus = "in_progress"
	TaskCompleted  TaskStatus = "completed"
	TaskFailed     TaskStatus = "failed"
	TaskBlocked    TaskStatus = "blocked"
)

// Plan represents a high-level plan with tasks.
type Plan struct {
	PlanID      string     `json:"plan_id"`
	Description string     `json:"description"`
	Status      PlanStatus `json:"status"`
	CreatedAt   string     `json:"created_at"`
	UpdatedAt   string     `json:"updated_at"`
	SessionID   string     `json:"session_id"`
	Tasks       []Task     `json:"tasks,omitempty"`
}

// Task represents a unit of work assignable to a worker.
type Task struct {
	TaskID      string     `json:"task_id"`
	PlanID      string     `json:"plan_id"`
	Description string     `json:"description"`
	Status      TaskStatus `json:"status"`
	WorkerID    string     `json:"worker_id,omitempty"`
	Order       int        `json:"order"`
	DependsOn   []string   `json:"depends_on,omitempty"`
	CreatedAt   string     `json:"created_at"`
	StartedAt   string     `json:"started_at,omitempty"`
	CompletedAt string     `json:"completed_at,omitempty"`
}

// Result represents the outcome of a task.
type Result struct {
	ResultID     string   `json:"result_id"`
	TaskID       string   `json:"task_id"`
	WorkerID     string   `json:"worker_id"`
	Status       string   `json:"status"` // success, failure, partial
	Output       string   `json:"output"`
	Error        string   `json:"error,omitempty"`
	FilesChanged []string `json:"files_changed,omitempty"`
	CreatedAt    string   `json:"created_at"`
}
