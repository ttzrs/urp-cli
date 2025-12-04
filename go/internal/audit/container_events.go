package audit

import (
	"context"
	"fmt"
	"time"

	"github.com/joss/urp/internal/metrics"
)

// GetRequestID extracts request ID from context or returns empty.
func GetRequestID(ctx context.Context) string {
	// This is a placeholder. In a real HTTP server, you'd pull from context.
	// For CLI, we might not have one.
	return ""
}

// SpawnEvent logs a container spawn event.
func SpawnEvent(workerName, project string, success bool, duration time.Duration, err error) {
	SpawnEventCtx(context.Background(), workerName, project, success, duration, err)
}

// SpawnEventCtx logs a container spawn event with context.
func SpawnEventCtx(ctx context.Context, workerName, project string, success bool, duration time.Duration, err error) {
	l := Global()
	event := l.Start(CategorySystem, "spawn")
	event.Command = workerName // abusing command field for worker name
	event.Project = project
	
	if err != nil {
		l.LogError(event, err)
	} else {
		l.LogSuccess(event)
	}

	// Update metrics
	metrics.Global().RecordSpawn(success, duration.Milliseconds())

	// Persist to graph
	errMsg := ""
	if err != nil {
		errMsg = err.Error()
	}
	PersistWorkerSpawn(workerName, project, success, duration.Milliseconds(), errMsg)
}

// NeMoEvent logs a NeMo container event.
func NeMoEvent(eventOp, containerName, project string, duration time.Duration, err error) {
	NeMoEventCtx(context.Background(), eventOp, containerName, project, duration, err)
}

// NeMoEventCtx logs a NeMo container event with context.
func NeMoEventCtx(ctx context.Context, eventOp, containerName, project string, duration time.Duration, err error) {
	l := Global()
	event := l.Start(CategorySystem, "nemo_"+eventOp)
	event.Command = containerName
	event.Project = project

	if err != nil {
		l.LogError(event, err)
	} else {
		l.LogSuccess(event)
	}

	errMsg := ""
	if err != nil {
		errMsg = err.Error()
	}

	// Update metrics (only for launch events)
	if eventOp == "launch" {
		metrics.Global().RecordNeMo(errMsg == "", duration.Milliseconds())
	}

	// Persist to graph
	PersistNeMoEvent(eventOp, containerName, project, duration.Milliseconds(), errMsg)
}

// HealthEvent logs a health check event.
func HealthEvent(workerName, status string, healthy bool) {
	HealthEventCtx(context.Background(), workerName, status, healthy)
}

// HealthEventCtx logs a health check event with context.
func HealthEventCtx(ctx context.Context, workerName, status string, healthy bool) {
	l := Global()
	
	// Use a specialized event or just generic system event
	// Since audit logger is focused on operations, health check is an operation
	event := l.Start(CategorySystem, "health_check")
	event.Command = workerName
	
	if !healthy {
		l.LogWarning(event, fmt.Sprintf("unhealthy: %s", status))
	} else {
		l.LogSuccess(event)
	}

	// Update metrics
	metrics.Global().RecordHealthCheck(healthy)

	// Persist to graph
	PersistHealthCheck(workerName, status, healthy)
}
