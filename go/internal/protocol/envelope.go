// Package protocol defines the Master↔Worker communication protocol.
// Messages use JSON envelope format over stdin/stdout.
//
// # Usage Mode: Programmatic Automation
//
// This protocol is for direct process-to-process communication via pipes.
// It's an alternative to the primary Claude CLI-based flow.
//
// Primary flow (recommended):
//
//	urp ask urp-proj-w1 "run tests"
//	  → docker exec worker claude --print "..."
//	  → Claude CLI executes, human-readable output
//
// Protocol flow (automation):
//
//	urp orchestrate run "task1" "task2"
//	  → spawns workers with stdin/stdout pipes
//	  → sends JSON messages via protocol
//	  → structured progress/result payloads
//
// Use protocol mode for:
//   - CI/CD automation without human interaction
//   - Parallel task execution with progress tracking
//   - Custom orchestration tooling
package protocol

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"sync"
	"time"
)

// MessageType identifies the kind of message.
type MessageType string

const (
	// Master → Worker
	MsgAssignTask   MessageType = "assign_task"
	MsgCancelTask   MessageType = "cancel_task"
	MsgPing         MessageType = "ping"
	MsgShutdown     MessageType = "shutdown"

	// Worker → Master
	MsgTaskStarted  MessageType = "task_started"
	MsgTaskProgress MessageType = "task_progress"
	MsgTaskOutput   MessageType = "task_output"
	MsgTaskComplete MessageType = "task_complete"
	MsgTaskFailed   MessageType = "task_failed"
	MsgPong         MessageType = "pong"
	MsgReady        MessageType = "ready"

	// Bidirectional
	MsgError        MessageType = "error"
)

// Envelope wraps all protocol messages.
type Envelope struct {
	Type      MessageType `json:"type"`
	ID        string      `json:"id"`                  // Message ID for correlation
	Timestamp string      `json:"ts"`                  // ISO8601
	Payload   any         `json:"payload,omitempty"`   // Type-specific data
}

// NewEnvelope creates a new envelope with auto-generated ID and timestamp.
func NewEnvelope(msgType MessageType, payload any) *Envelope {
	return &Envelope{
		Type:      msgType,
		ID:        fmt.Sprintf("msg-%d", time.Now().UnixNano()),
		Timestamp: time.Now().UTC().Format(time.RFC3339),
		Payload:   payload,
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// Payload types for Master → Worker
// ─────────────────────────────────────────────────────────────────────────────

// AssignTaskPayload contains task assignment details.
type AssignTaskPayload struct {
	TaskID      string   `json:"task_id"`
	PlanID      string   `json:"plan_id"`
	Description string   `json:"description"`
	Branch      string   `json:"branch,omitempty"`
	Context     []string `json:"context,omitempty"`  // Files to focus on
	Prompt      string   `json:"prompt,omitempty"`   // Additional instructions
}

// CancelTaskPayload identifies which task to cancel.
type CancelTaskPayload struct {
	TaskID string `json:"task_id"`
	Reason string `json:"reason,omitempty"`
}

// ─────────────────────────────────────────────────────────────────────────────
// Payload types for Worker → Master
// ─────────────────────────────────────────────────────────────────────────────

// TaskStartedPayload confirms task execution began.
type TaskStartedPayload struct {
	TaskID   string `json:"task_id"`
	WorkerID string `json:"worker_id"`
	Branch   string `json:"branch,omitempty"`
}

// TaskProgressPayload reports incremental progress.
type TaskProgressPayload struct {
	TaskID   string  `json:"task_id"`
	Progress float64 `json:"progress"`  // 0.0 - 1.0
	Message  string  `json:"message"`
}

// TaskOutputPayload streams command/tool output.
type TaskOutputPayload struct {
	TaskID string `json:"task_id"`
	Stream string `json:"stream"`  // "stdout" | "stderr" | "tool"
	Data   string `json:"data"`
}

// TaskCompletePayload reports successful completion.
type TaskCompletePayload struct {
	TaskID       string   `json:"task_id"`
	WorkerID     string   `json:"worker_id"`
	Output       string   `json:"output"`
	FilesChanged []string `json:"files_changed,omitempty"`
	PRUrl        string   `json:"pr_url,omitempty"`
	Duration     int64    `json:"duration_ms"`
}

// TaskFailedPayload reports task failure.
type TaskFailedPayload struct {
	TaskID   string `json:"task_id"`
	WorkerID string `json:"worker_id"`
	Error    string `json:"error"`
	Code     int    `json:"code,omitempty"`  // Exit code if applicable
}

// ReadyPayload announces worker is ready for tasks.
type ReadyPayload struct {
	WorkerID     string   `json:"worker_id"`
	Capabilities []string `json:"capabilities,omitempty"`  // e.g., ["go", "python", "git"]
}

// ErrorPayload for protocol-level errors.
type ErrorPayload struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// ─────────────────────────────────────────────────────────────────────────────
// Encoder/Decoder for streaming JSON lines
// ─────────────────────────────────────────────────────────────────────────────

// Encoder writes envelopes as JSON lines.
type Encoder struct {
	w  io.Writer
	mu sync.Mutex
}

// NewEncoder creates an encoder for the given writer.
func NewEncoder(w io.Writer) *Encoder {
	return &Encoder{w: w}
}

// Encode writes an envelope as a single JSON line.
func (e *Encoder) Encode(env *Envelope) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	data, err := json.Marshal(env)
	if err != nil {
		return fmt.Errorf("marshal envelope: %w", err)
	}

	_, err = fmt.Fprintf(e.w, "%s\n", data)
	return err
}

// Send is a convenience method to create and encode an envelope.
func (e *Encoder) Send(msgType MessageType, payload any) error {
	return e.Encode(NewEnvelope(msgType, payload))
}

// Decoder reads envelopes from JSON lines.
type Decoder struct {
	scanner *bufio.Scanner
}

// NewDecoder creates a decoder for the given reader.
func NewDecoder(r io.Reader) *Decoder {
	scanner := bufio.NewScanner(r)
	// Allow large messages (up to 1MB)
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)
	return &Decoder{scanner: scanner}
}

// Decode reads the next envelope.
func (d *Decoder) Decode() (*Envelope, error) {
	if !d.scanner.Scan() {
		if err := d.scanner.Err(); err != nil {
			return nil, err
		}
		return nil, io.EOF
	}

	line := d.scanner.Bytes()
	if len(line) == 0 {
		return d.Decode() // Skip empty lines
	}

	var env Envelope
	if err := json.Unmarshal(line, &env); err != nil {
		return nil, fmt.Errorf("unmarshal envelope: %w", err)
	}

	return &env, nil
}

// ─────────────────────────────────────────────────────────────────────────────
// Payload extraction helpers
// ─────────────────────────────────────────────────────────────────────────────

// GetPayload extracts and unmarshals the payload into the target type.
func (e *Envelope) GetPayload(target any) error {
	if e.Payload == nil {
		return nil
	}

	// Payload comes as map[string]any from JSON, re-marshal to unmarshal into struct
	data, err := json.Marshal(e.Payload)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, target)
}

// AsAssignTask extracts AssignTaskPayload.
func (e *Envelope) AsAssignTask() (*AssignTaskPayload, error) {
	var p AssignTaskPayload
	if err := e.GetPayload(&p); err != nil {
		return nil, err
	}
	return &p, nil
}

// AsTaskComplete extracts TaskCompletePayload.
func (e *Envelope) AsTaskComplete() (*TaskCompletePayload, error) {
	var p TaskCompletePayload
	if err := e.GetPayload(&p); err != nil {
		return nil, err
	}
	return &p, nil
}

// AsTaskFailed extracts TaskFailedPayload.
func (e *Envelope) AsTaskFailed() (*TaskFailedPayload, error) {
	var p TaskFailedPayload
	if err := e.GetPayload(&p); err != nil {
		return nil, err
	}
	return &p, nil
}

// AsTaskOutput extracts TaskOutputPayload.
func (e *Envelope) AsTaskOutput() (*TaskOutputPayload, error) {
	var p TaskOutputPayload
	if err := e.GetPayload(&p); err != nil {
		return nil, err
	}
	return &p, nil
}

// AsReady extracts ReadyPayload.
func (e *Envelope) AsReady() (*ReadyPayload, error) {
	var p ReadyPayload
	if err := e.GetPayload(&p); err != nil {
		return nil, err
	}
	return &p, nil
}
