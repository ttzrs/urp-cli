package protocol

import (
	"context"
	"fmt"
	"io"
	"sync"
	"time"
)

// WorkerConnection represents a connection to a worker.
type WorkerConnection struct {
	ID           string
	Capabilities []string
	enc          *Encoder
	dec          *Decoder

	mu          sync.RWMutex
	currentTask string
	lastSeen    time.Time
}

// Master coordinates multiple workers.
type Master struct {
	mu      sync.RWMutex
	workers map[string]*WorkerConnection

	// Callbacks
	OnWorkerReady    func(workerID string, caps []string)
	OnTaskStarted    func(workerID, taskID, branch string)
	OnTaskProgress   func(workerID, taskID string, progress float64, msg string)
	OnTaskOutput     func(workerID, taskID, stream, data string)
	OnTaskComplete   func(workerID, taskID string, result *TaskCompletePayload)
	OnTaskFailed     func(workerID, taskID string, result *TaskFailedPayload)
	OnWorkerDisconnect func(workerID string)
}

// NewMaster creates a new master coordinator.
func NewMaster() *Master {
	return &Master{
		workers: make(map[string]*WorkerConnection),
	}
}

// AddWorker registers a worker connection.
func (m *Master) AddWorker(workerID string, r io.Reader, w io.Writer) *WorkerConnection {
	conn := &WorkerConnection{
		ID:       workerID,
		enc:      NewEncoder(w),
		dec:      NewDecoder(r),
		lastSeen: time.Now(),
	}

	m.mu.Lock()
	m.workers[workerID] = conn
	m.mu.Unlock()

	return conn
}

// RemoveWorker unregisters a worker.
func (m *Master) RemoveWorker(workerID string) {
	m.mu.Lock()
	delete(m.workers, workerID)
	m.mu.Unlock()

	if m.OnWorkerDisconnect != nil {
		m.OnWorkerDisconnect(workerID)
	}
}

// GetWorker returns a worker connection by ID.
func (m *Master) GetWorker(workerID string) *WorkerConnection {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.workers[workerID]
}

// ListWorkers returns all connected worker IDs.
func (m *Master) ListWorkers() []string {
	m.mu.RLock()
	defer m.mu.RUnlock()

	ids := make([]string, 0, len(m.workers))
	for id := range m.workers {
		ids = append(ids, id)
	}
	return ids
}

// ListIdleWorkers returns workers not currently executing a task.
func (m *Master) ListIdleWorkers() []string {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var idle []string
	for id, w := range m.workers {
		w.mu.RLock()
		if w.currentTask == "" {
			idle = append(idle, id)
		}
		w.mu.RUnlock()
	}
	return idle
}

// AssignTask sends a task to a specific worker.
func (m *Master) AssignTask(workerID string, task *AssignTaskPayload) error {
	m.mu.RLock()
	w, ok := m.workers[workerID]
	m.mu.RUnlock()

	if !ok {
		return fmt.Errorf("worker not found: %s", workerID)
	}

	w.mu.Lock()
	if w.currentTask != "" {
		w.mu.Unlock()
		return fmt.Errorf("worker %s is busy with task %s", workerID, w.currentTask)
	}
	w.currentTask = task.TaskID
	w.mu.Unlock()

	return w.enc.Send(MsgAssignTask, task)
}

// CancelTask cancels a task on a worker.
func (m *Master) CancelTask(workerID, taskID, reason string) error {
	m.mu.RLock()
	w, ok := m.workers[workerID]
	m.mu.RUnlock()

	if !ok {
		return fmt.Errorf("worker not found: %s", workerID)
	}

	return w.enc.Send(MsgCancelTask, &CancelTaskPayload{
		TaskID: taskID,
		Reason: reason,
	})
}

// Ping sends a ping to a worker.
func (m *Master) Ping(workerID string) error {
	m.mu.RLock()
	w, ok := m.workers[workerID]
	m.mu.RUnlock()

	if !ok {
		return fmt.Errorf("worker not found: %s", workerID)
	}

	return w.enc.Send(MsgPing, nil)
}

// Shutdown sends shutdown to a worker.
func (m *Master) Shutdown(workerID string) error {
	m.mu.RLock()
	w, ok := m.workers[workerID]
	m.mu.RUnlock()

	if !ok {
		return fmt.Errorf("worker not found: %s", workerID)
	}

	return w.enc.Send(MsgShutdown, nil)
}

// ShutdownAll sends shutdown to all workers.
func (m *Master) ShutdownAll() {
	m.mu.RLock()
	defer m.mu.RUnlock()

	for _, w := range m.workers {
		w.enc.Send(MsgShutdown, nil)
	}
}

// HandleWorker processes messages from a single worker.
// Run this in a goroutine for each worker.
func (m *Master) HandleWorker(ctx context.Context, workerID string) error {
	m.mu.RLock()
	w, ok := m.workers[workerID]
	m.mu.RUnlock()

	if !ok {
		return fmt.Errorf("worker not found: %s", workerID)
	}

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		env, err := w.dec.Decode()
		if err == io.EOF {
			m.RemoveWorker(workerID)
			return nil
		}
		if err != nil {
			m.RemoveWorker(workerID)
			return fmt.Errorf("decode from %s: %w", workerID, err)
		}

		w.mu.Lock()
		w.lastSeen = time.Now()
		w.mu.Unlock()

		m.handleWorkerMessage(w, env)
	}
}

func (m *Master) handleWorkerMessage(w *WorkerConnection, env *Envelope) {
	switch env.Type {
	case MsgReady:
		if p, err := env.AsReady(); err == nil {
			w.mu.Lock()
			w.Capabilities = p.Capabilities
			w.mu.Unlock()

			if m.OnWorkerReady != nil {
				m.OnWorkerReady(w.ID, p.Capabilities)
			}
		}

	case MsgTaskStarted:
		var p TaskStartedPayload
		if err := env.GetPayload(&p); err == nil {
			if m.OnTaskStarted != nil {
				m.OnTaskStarted(w.ID, p.TaskID, p.Branch)
			}
		}

	case MsgTaskProgress:
		var p TaskProgressPayload
		if err := env.GetPayload(&p); err == nil {
			if m.OnTaskProgress != nil {
				m.OnTaskProgress(w.ID, p.TaskID, p.Progress, p.Message)
			}
		}

	case MsgTaskOutput:
		if p, err := env.AsTaskOutput(); err == nil {
			if m.OnTaskOutput != nil {
				m.OnTaskOutput(w.ID, p.TaskID, p.Stream, p.Data)
			}
		}

	case MsgTaskComplete:
		if p, err := env.AsTaskComplete(); err == nil {
			w.mu.Lock()
			w.currentTask = ""
			w.mu.Unlock()

			if m.OnTaskComplete != nil {
				m.OnTaskComplete(w.ID, p.TaskID, p)
			}
		}

	case MsgTaskFailed:
		if p, err := env.AsTaskFailed(); err == nil {
			w.mu.Lock()
			w.currentTask = ""
			w.mu.Unlock()

			if m.OnTaskFailed != nil {
				m.OnTaskFailed(w.ID, p.TaskID, p)
			}
		}

	case MsgPong:
		// Just update lastSeen (already done)

	case MsgError:
		var p ErrorPayload
		if err := env.GetPayload(&p); err == nil {
			fmt.Printf("[%s] error: %s - %s\n", w.ID, p.Code, p.Message)
		}
	}
}
