package protocol

import (
	"context"
	"fmt"
	"io"
	"os"
	"sync"
	"time"
)

// TaskHandler processes assigned tasks.
type TaskHandler func(ctx context.Context, task *AssignTaskPayload, reporter *TaskReporter) error

// Worker handles incoming messages from master.
type Worker struct {
	id       string
	enc      *Encoder
	dec      *Decoder
	handler  TaskHandler
	caps     []string
	exitFunc func(int) // override for testing

	mu          sync.Mutex
	currentTask string
	cancelFunc  context.CancelFunc
}

// NewWorker creates a worker that communicates via stdin/stdout.
func NewWorker(id string, caps []string) *Worker {
	return &Worker{
		id:       id,
		enc:      NewEncoder(os.Stdout),
		dec:      NewDecoder(os.Stdin),
		caps:     caps,
		exitFunc: os.Exit,
	}
}

// NewWorkerWithIO creates a worker with custom IO (for testing).
// In test mode, shutdown doesn't call os.Exit.
func NewWorkerWithIO(id string, caps []string, r io.Reader, w io.Writer) *Worker {
	return &Worker{
		id:       id,
		enc:      NewEncoder(w),
		dec:      NewDecoder(r),
		caps:     caps,
		exitFunc: func(int) {}, // no-op for testing
	}
}

// SetHandler sets the task handler function.
func (w *Worker) SetHandler(h TaskHandler) {
	w.handler = h
}

// Run starts the worker message loop.
func (w *Worker) Run(ctx context.Context) error {
	// Announce ready
	if err := w.enc.Send(MsgReady, &ReadyPayload{
		WorkerID:     w.id,
		Capabilities: w.caps,
	}); err != nil {
		return fmt.Errorf("send ready: %w", err)
	}

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		env, err := w.dec.Decode()
		if err == io.EOF {
			return nil // Master closed connection
		}
		if err != nil {
			return fmt.Errorf("decode: %w", err)
		}

		if err := w.handleMessage(ctx, env); err != nil {
			w.enc.Send(MsgError, &ErrorPayload{
				Code:    "handler_error",
				Message: err.Error(),
			})
		}
	}
}

func (w *Worker) handleMessage(ctx context.Context, env *Envelope) error {
	switch env.Type {
	case MsgAssignTask:
		return w.handleAssignTask(ctx, env)

	case MsgCancelTask:
		return w.handleCancelTask(env)

	case MsgPing:
		return w.enc.Send(MsgPong, nil)

	case MsgShutdown:
		w.cancelCurrentTask()
		if w.exitFunc != nil {
			w.exitFunc(0)
		}
		return io.EOF // signal clean exit

	default:
		return fmt.Errorf("unknown message type: %s", env.Type)
	}
}

func (w *Worker) handleAssignTask(ctx context.Context, env *Envelope) error {
	task, err := env.AsAssignTask()
	if err != nil {
		return err
	}

	if w.handler == nil {
		return fmt.Errorf("no task handler registered")
	}

	// Cancel any existing task
	w.cancelCurrentTask()

	// Create cancellable context for this task
	taskCtx, cancel := context.WithCancel(ctx)

	w.mu.Lock()
	w.currentTask = task.TaskID
	w.cancelFunc = cancel
	w.mu.Unlock()

	// Create reporter for this task
	reporter := &TaskReporter{
		enc:      w.enc,
		taskID:   task.TaskID,
		workerID: w.id,
		start:    time.Now(),
	}

	// Send started
	reporter.Started(task.Branch)

	// Run handler in goroutine
	go func() {
		defer func() {
			w.mu.Lock()
			w.currentTask = ""
			w.cancelFunc = nil
			w.mu.Unlock()
		}()

		if err := w.handler(taskCtx, task, reporter); err != nil {
			if taskCtx.Err() != nil {
				// Cancelled, not a failure
				return
			}
			reporter.Failed(err.Error(), 1)
		}
	}()

	return nil
}

func (w *Worker) handleCancelTask(env *Envelope) error {
	payload := &CancelTaskPayload{}
	if err := env.GetPayload(payload); err != nil {
		return err
	}

	w.mu.Lock()
	if w.currentTask == payload.TaskID && w.cancelFunc != nil {
		w.cancelFunc()
	}
	w.mu.Unlock()

	return nil
}

func (w *Worker) cancelCurrentTask() {
	w.mu.Lock()
	if w.cancelFunc != nil {
		w.cancelFunc()
	}
	w.mu.Unlock()
}

// ─────────────────────────────────────────────────────────────────────────────
// TaskReporter for sending progress updates
// ─────────────────────────────────────────────────────────────────────────────

// TaskReporter sends task updates to master.
type TaskReporter struct {
	enc      *Encoder
	taskID   string
	workerID string
	start    time.Time
}

// Started sends task_started message.
func (r *TaskReporter) Started(branch string) {
	r.enc.Send(MsgTaskStarted, &TaskStartedPayload{
		TaskID:   r.taskID,
		WorkerID: r.workerID,
		Branch:   branch,
	})
}

// Progress sends incremental progress update.
func (r *TaskReporter) Progress(pct float64, msg string) {
	r.enc.Send(MsgTaskProgress, &TaskProgressPayload{
		TaskID:   r.taskID,
		Progress: pct,
		Message:  msg,
	})
}

// Output sends streaming output.
func (r *TaskReporter) Output(stream, data string) {
	r.enc.Send(MsgTaskOutput, &TaskOutputPayload{
		TaskID: r.taskID,
		Stream: stream,
		Data:   data,
	})
}

// Stdout sends stdout output.
func (r *TaskReporter) Stdout(data string) {
	r.Output("stdout", data)
}

// Stderr sends stderr output.
func (r *TaskReporter) Stderr(data string) {
	r.Output("stderr", data)
}

// Complete sends task_complete message.
func (r *TaskReporter) Complete(output string, filesChanged []string, prURL string) {
	r.enc.Send(MsgTaskComplete, &TaskCompletePayload{
		TaskID:       r.taskID,
		WorkerID:     r.workerID,
		Output:       output,
		FilesChanged: filesChanged,
		PRUrl:        prURL,
		Duration:     time.Since(r.start).Milliseconds(),
	})
}

// Failed sends task_failed message.
func (r *TaskReporter) Failed(errMsg string, code int) {
	r.enc.Send(MsgTaskFailed, &TaskFailedPayload{
		TaskID:   r.taskID,
		WorkerID: r.workerID,
		Error:    errMsg,
		Code:     code,
	})
}
