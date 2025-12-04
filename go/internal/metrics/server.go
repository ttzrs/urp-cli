// Package metrics provides a simple Prometheus-compatible metrics endpoint.
package metrics

import (
	"context"
	"fmt"
	"net/http"
	"sync"
	"sync/atomic"
	"time"
)

// Metrics holds runtime metrics for URP
type Metrics struct {
	// Container operations
	WorkerSpawns      atomic.Int64
	WorkerSpawnErrors atomic.Int64
	NeMoLaunches      atomic.Int64
	NeMoLaunchErrors  atomic.Int64

	// Health checks
	HealthChecks        atomic.Int64
	HealthCheckFailures atomic.Int64

	// Graph operations
	GraphWrites      atomic.Int64
	GraphWriteErrors atomic.Int64

	// Timing (last operation duration in ms)
	LastSpawnDurationMs atomic.Int64
	LastNeMoDurationMs  atomic.Int64

	startTime time.Time
}

var (
	global     *Metrics
	globalOnce sync.Once
)

// Global returns the global metrics instance
func Global() *Metrics {
	globalOnce.Do(func() {
		global = &Metrics{
			startTime: time.Now(),
		}
	})
	return global
}

// RecordSpawn records a worker spawn attempt
func (m *Metrics) RecordSpawn(success bool, durationMs int64) {
	m.WorkerSpawns.Add(1)
	if !success {
		m.WorkerSpawnErrors.Add(1)
	}
	m.LastSpawnDurationMs.Store(durationMs)
}

// RecordNeMo records a NeMo launch attempt
func (m *Metrics) RecordNeMo(success bool, durationMs int64) {
	m.NeMoLaunches.Add(1)
	if !success {
		m.NeMoLaunchErrors.Add(1)
	}
	m.LastNeMoDurationMs.Store(durationMs)
}

// RecordHealthCheck records a health check
func (m *Metrics) RecordHealthCheck(healthy bool) {
	m.HealthChecks.Add(1)
	if !healthy {
		m.HealthCheckFailures.Add(1)
	}
}

// RecordGraphWrite records a graph write attempt
func (m *Metrics) RecordGraphWrite(success bool) {
	m.GraphWrites.Add(1)
	if !success {
		m.GraphWriteErrors.Add(1)
	}
}

// Handler returns an HTTP handler for /metrics endpoint
func (m *Metrics) Handler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain; version=0.0.4")

		uptime := time.Since(m.startTime).Seconds()

		fmt.Fprintf(w, "# HELP urp_uptime_seconds Time since URP started\n")
		fmt.Fprintf(w, "# TYPE urp_uptime_seconds gauge\n")
		fmt.Fprintf(w, "urp_uptime_seconds %.2f\n\n", uptime)

		fmt.Fprintf(w, "# HELP urp_worker_spawns_total Total worker spawn attempts\n")
		fmt.Fprintf(w, "# TYPE urp_worker_spawns_total counter\n")
		fmt.Fprintf(w, "urp_worker_spawns_total %d\n\n", m.WorkerSpawns.Load())

		fmt.Fprintf(w, "# HELP urp_worker_spawn_errors_total Total worker spawn failures\n")
		fmt.Fprintf(w, "# TYPE urp_worker_spawn_errors_total counter\n")
		fmt.Fprintf(w, "urp_worker_spawn_errors_total %d\n\n", m.WorkerSpawnErrors.Load())

		fmt.Fprintf(w, "# HELP urp_nemo_launches_total Total NeMo container launches\n")
		fmt.Fprintf(w, "# TYPE urp_nemo_launches_total counter\n")
		fmt.Fprintf(w, "urp_nemo_launches_total %d\n\n", m.NeMoLaunches.Load())

		fmt.Fprintf(w, "# HELP urp_nemo_launch_errors_total Total NeMo launch failures\n")
		fmt.Fprintf(w, "# TYPE urp_nemo_launch_errors_total counter\n")
		fmt.Fprintf(w, "urp_nemo_launch_errors_total %d\n\n", m.NeMoLaunchErrors.Load())

		fmt.Fprintf(w, "# HELP urp_health_checks_total Total health checks performed\n")
		fmt.Fprintf(w, "# TYPE urp_health_checks_total counter\n")
		fmt.Fprintf(w, "urp_health_checks_total %d\n\n", m.HealthChecks.Load())

		fmt.Fprintf(w, "# HELP urp_health_check_failures_total Total health check failures\n")
		fmt.Fprintf(w, "# TYPE urp_health_check_failures_total counter\n")
		fmt.Fprintf(w, "urp_health_check_failures_total %d\n\n", m.HealthCheckFailures.Load())

		fmt.Fprintf(w, "# HELP urp_graph_writes_total Total graph write operations\n")
		fmt.Fprintf(w, "# TYPE urp_graph_writes_total counter\n")
		fmt.Fprintf(w, "urp_graph_writes_total %d\n\n", m.GraphWrites.Load())

		fmt.Fprintf(w, "# HELP urp_graph_write_errors_total Total graph write failures\n")
		fmt.Fprintf(w, "# TYPE urp_graph_write_errors_total counter\n")
		fmt.Fprintf(w, "urp_graph_write_errors_total %d\n\n", m.GraphWriteErrors.Load())

		fmt.Fprintf(w, "# HELP urp_last_spawn_duration_ms Last worker spawn duration\n")
		fmt.Fprintf(w, "# TYPE urp_last_spawn_duration_ms gauge\n")
		fmt.Fprintf(w, "urp_last_spawn_duration_ms %d\n\n", m.LastSpawnDurationMs.Load())

		fmt.Fprintf(w, "# HELP urp_last_nemo_duration_ms Last NeMo launch duration\n")
		fmt.Fprintf(w, "# TYPE urp_last_nemo_duration_ms gauge\n")
		fmt.Fprintf(w, "urp_last_nemo_duration_ms %d\n", m.LastNeMoDurationMs.Load())
	}
}

// Server wraps the metrics HTTP server
type Server struct {
	srv *http.Server
}

// NewServer creates a metrics server on the given port
func NewServer(port int) *Server {
	mux := http.NewServeMux()
	mux.HandleFunc("/metrics", Global().Handler())
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok"))
	})

	return &Server{
		srv: &http.Server{
			Addr:    fmt.Sprintf(":%d", port),
			Handler: mux,
		},
	}
}

// Start starts the metrics server in background
func (s *Server) Start() error {
	go s.srv.ListenAndServe()
	return nil
}

// Stop gracefully shuts down the metrics server
func (s *Server) Stop(ctx context.Context) error {
	return s.srv.Shutdown(ctx)
}
