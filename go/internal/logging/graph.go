// Package logging provides graph persistence for container events.
package logging

import (
	"context"
	"sync"
	"time"
)

// GraphWriter persists events to the graph database.
// Uses interface to avoid import cycle with graph package.
type GraphWriter interface {
	ExecuteWrite(ctx context.Context, query string, params map[string]any) error
}

var (
	graphDriver GraphWriter
	graphMu     sync.RWMutex
)

// SetGraphDriver configures the graph database for event persistence.
// Call this at startup after connecting to Memgraph.
func SetGraphDriver(driver GraphWriter) {
	graphMu.Lock()
	defer graphMu.Unlock()
	graphDriver = driver
}

// PersistContainerEvent stores a container event in the graph.
// Creates (:ContainerEvent) node linked to project.
func PersistContainerEvent(event, containerName, project string, durationMs int64, success bool, errMsg string) {
	graphMu.RLock()
	driver := graphDriver
	graphMu.RUnlock()

	if driver == nil {
		return // No graph configured, skip persistence
	}

	query := `
		MERGE (p:Project {name: $project})
		CREATE (e:ContainerEvent {
			event: $event,
			container: $container,
			timestamp: $timestamp,
			duration_ms: $duration,
			success: $success,
			error: $error
		})
		CREATE (p)-[:HAS_EVENT]->(e)
	`

	params := map[string]any{
		"project":   project,
		"event":     event,
		"container": containerName,
		"timestamp": time.Now().UTC().Format(time.RFC3339),
		"duration":  durationMs,
		"success":   success,
		"error":     errMsg,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Best effort - don't fail operations if graph write fails
	_ = driver.ExecuteWrite(ctx, query, params)
}

// PersistWorkerSpawn records a worker spawn in the graph.
func PersistWorkerSpawn(workerName, project string, success bool, durationMs int64, errMsg string) {
	PersistContainerEvent("spawn", workerName, project, durationMs, success, errMsg)
}

// PersistNeMoEvent records a NeMo container event in the graph.
func PersistNeMoEvent(event, containerName, project string, durationMs int64, errMsg string) {
	success := errMsg == ""
	PersistContainerEvent(event, containerName, project, durationMs, success, errMsg)
}

// PersistHealthCheck records a health check result.
func PersistHealthCheck(workerName, status string, healthy bool) {
	graphMu.RLock()
	driver := graphDriver
	graphMu.RUnlock()

	if driver == nil {
		return
	}

	query := `
		MATCH (c:Container {name: $container})
		CREATE (h:HealthCheck {
			timestamp: $timestamp,
			status: $status,
			healthy: $healthy
		})
		CREATE (c)-[:HEALTH_CHECK]->(h)
	`

	params := map[string]any{
		"container": workerName,
		"timestamp": time.Now().UTC().Format(time.RFC3339),
		"status":    status,
		"healthy":   healthy,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_ = driver.ExecuteWrite(ctx, query, params)
}
