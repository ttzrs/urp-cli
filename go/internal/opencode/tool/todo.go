package tool

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/oklog/ulid/v2"
	"github.com/joss/urp/internal/opencode/domain"
)

// TodoItem represents a single task
type TodoItem struct {
	ID         string    `json:"id"`
	Content    string    `json:"content"`
	ActiveForm string    `json:"active_form"` // Present continuous form
	Status     string    `json:"status"`      // pending, in_progress, completed
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

// TodoStore manages todos in memory (session-scoped)
type TodoStore struct {
	mu    sync.RWMutex
	items []TodoItem
}

var globalTodoStore = &TodoStore{}

// GetTodoStore returns the global todo store
func GetTodoStore() *TodoStore {
	return globalTodoStore
}

// List returns all todos
func (s *TodoStore) List() []TodoItem {
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := make([]TodoItem, len(s.items))
	copy(result, s.items)
	return result
}

// Set replaces all todos
func (s *TodoStore) Set(items []TodoItem) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.items = make([]TodoItem, len(items))
	copy(s.items, items)
}

// Clear removes all todos
func (s *TodoStore) Clear() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.items = nil
}

// TodoWrite implements the todo_write tool
type TodoWrite struct{}

func NewTodoWrite() *TodoWrite {
	return &TodoWrite{}
}

func (t *TodoWrite) Info() domain.Tool {
	return domain.Tool{
		Name:        "todo_write",
		Description: "Update the todo list to track tasks and progress",
		Parameters: domain.JSONSchema{
			"type": "object",
			"properties": map[string]any{
				"todos": map[string]any{
					"type":        "array",
					"description": "Array of todos: [{content, status, activeForm}, ...]. Status: pending, in_progress, completed",
					"items": map[string]any{
						"type": "object",
						"properties": map[string]any{
							"content":    map[string]any{"type": "string"},
							"status":     map[string]any{"type": "string", "enum": []string{"pending", "in_progress", "completed"}},
							"activeForm": map[string]any{"type": "string"},
						},
					},
				},
			},
			"required": []string{"todos"},
		},
	}
}

func (t *TodoWrite) Execute(ctx context.Context, args map[string]any) (*Result, error) {
	todosRaw, _ := args["todos"].([]any)

	if len(todosRaw) == 0 {
		globalTodoStore.Clear()
		return &Result{
			Title:  "Cleared todo list",
			Output: "Todo list has been cleared",
		}, nil
	}

	var items []TodoItem
	now := time.Now()

	for i, raw := range todosRaw {
		var item TodoItem
		item.ID = ulid.Make().String()
		item.CreatedAt = now
		item.UpdatedAt = now

		switch v := raw.(type) {
		case map[string]any:
			item.Content, _ = v["content"].(string)
			item.Status, _ = v["status"].(string)
			item.ActiveForm, _ = v["activeForm"].(string)
			if item.ActiveForm == "" {
				item.ActiveForm, _ = v["active_form"].(string)
			}
		case string:
			if err := json.Unmarshal([]byte(v), &item); err != nil {
				item.Content = v
				item.Status = "pending"
			}
		default:
			return &Result{Error: fmt.Errorf("todo %d: unexpected type %T", i, raw)}, nil
		}

		if item.Content == "" {
			return &Result{Error: fmt.Errorf("todo %d: content is required", i)}, nil
		}
		if item.Status == "" {
			item.Status = "pending"
		}
		if item.ActiveForm == "" {
			item.ActiveForm = item.Content
		}

		// Validate status
		switch item.Status {
		case "pending", "in_progress", "completed":
			// valid
		default:
			return &Result{Error: fmt.Errorf("todo %d: invalid status '%s'", i, item.Status)}, nil
		}

		items = append(items, item)
	}

	globalTodoStore.Set(items)

	// Build summary
	var pending, inProgress, completed int
	for _, item := range items {
		switch item.Status {
		case "pending":
			pending++
		case "in_progress":
			inProgress++
		case "completed":
			completed++
		}
	}

	return &Result{
		Title:  fmt.Sprintf("Updated todos (%d items)", len(items)),
		Output: fmt.Sprintf("Todo list updated: %d pending, %d in progress, %d completed", pending, inProgress, completed),
		Metadata: map[string]any{
			"total":       len(items),
			"pending":     pending,
			"in_progress": inProgress,
			"completed":   completed,
		},
	}, nil
}

// TodoRead implements the todo_read tool
type TodoRead struct{}

func NewTodoRead() *TodoRead {
	return &TodoRead{}
}

func (t *TodoRead) Info() domain.Tool {
	return domain.Tool{
		Name:        "todo_read",
		Description: "Read the current todo list",
		Parameters: domain.JSONSchema{
			"type":       "object",
			"properties": map[string]any{},
		},
	}
}

func (t *TodoRead) Execute(ctx context.Context, args map[string]any) (*Result, error) {
	items := globalTodoStore.List()

	if len(items) == 0 {
		return &Result{
			Title:  "Todo list empty",
			Output: "No todos in the list",
		}, nil
	}

	var sb strings.Builder
	for i, item := range items {
		icon := "○"
		switch item.Status {
		case "in_progress":
			icon = "◐"
		case "completed":
			icon = "●"
		}
		sb.WriteString(fmt.Sprintf("%d. %s %s [%s]\n", i+1, icon, item.Content, item.Status))
	}

	return &Result{
		Title:  fmt.Sprintf("Todo list (%d items)", len(items)),
		Output: sb.String(),
		Metadata: map[string]any{
			"count": len(items),
		},
	}, nil
}
