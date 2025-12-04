package metrics

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestMetricsGlobal(t *testing.T) {
	m1 := Global()
	m2 := Global()

	if m1 != m2 {
		t.Error("Global() should return same instance")
	}
}

func TestRecordSpawn(t *testing.T) {
	m := &Metrics{startTime: time.Now()}

	m.RecordSpawn(true, 100)
	if m.WorkerSpawns.Load() != 1 {
		t.Errorf("expected 1 spawn, got %d", m.WorkerSpawns.Load())
	}
	if m.WorkerSpawnErrors.Load() != 0 {
		t.Errorf("expected 0 errors, got %d", m.WorkerSpawnErrors.Load())
	}
	if m.LastSpawnDurationMs.Load() != 100 {
		t.Errorf("expected duration 100, got %d", m.LastSpawnDurationMs.Load())
	}

	m.RecordSpawn(false, 50)
	if m.WorkerSpawns.Load() != 2 {
		t.Errorf("expected 2 spawns, got %d", m.WorkerSpawns.Load())
	}
	if m.WorkerSpawnErrors.Load() != 1 {
		t.Errorf("expected 1 error, got %d", m.WorkerSpawnErrors.Load())
	}
}

func TestRecordNeMo(t *testing.T) {
	m := &Metrics{startTime: time.Now()}

	m.RecordNeMo(true, 2000)
	if m.NeMoLaunches.Load() != 1 {
		t.Errorf("expected 1 launch, got %d", m.NeMoLaunches.Load())
	}
	if m.NeMoLaunchErrors.Load() != 0 {
		t.Errorf("expected 0 errors, got %d", m.NeMoLaunchErrors.Load())
	}
	if m.LastNeMoDurationMs.Load() != 2000 {
		t.Errorf("expected duration 2000, got %d", m.LastNeMoDurationMs.Load())
	}

	m.RecordNeMo(false, 500)
	if m.NeMoLaunches.Load() != 2 {
		t.Errorf("expected 2 launches, got %d", m.NeMoLaunches.Load())
	}
	if m.NeMoLaunchErrors.Load() != 1 {
		t.Errorf("expected 1 error, got %d", m.NeMoLaunchErrors.Load())
	}
}

func TestRecordHealthCheck(t *testing.T) {
	m := &Metrics{startTime: time.Now()}

	m.RecordHealthCheck(true)
	if m.HealthChecks.Load() != 1 {
		t.Errorf("expected 1 check, got %d", m.HealthChecks.Load())
	}
	if m.HealthCheckFailures.Load() != 0 {
		t.Errorf("expected 0 failures, got %d", m.HealthCheckFailures.Load())
	}

	m.RecordHealthCheck(false)
	if m.HealthChecks.Load() != 2 {
		t.Errorf("expected 2 checks, got %d", m.HealthChecks.Load())
	}
	if m.HealthCheckFailures.Load() != 1 {
		t.Errorf("expected 1 failure, got %d", m.HealthCheckFailures.Load())
	}
}

func TestRecordGraphWrite(t *testing.T) {
	m := &Metrics{startTime: time.Now()}

	m.RecordGraphWrite(true)
	if m.GraphWrites.Load() != 1 {
		t.Errorf("expected 1 write, got %d", m.GraphWrites.Load())
	}
	if m.GraphWriteErrors.Load() != 0 {
		t.Errorf("expected 0 errors, got %d", m.GraphWriteErrors.Load())
	}

	m.RecordGraphWrite(false)
	if m.GraphWrites.Load() != 2 {
		t.Errorf("expected 2 writes, got %d", m.GraphWrites.Load())
	}
	if m.GraphWriteErrors.Load() != 1 {
		t.Errorf("expected 1 error, got %d", m.GraphWriteErrors.Load())
	}
}

func TestMetricsHandler(t *testing.T) {
	m := &Metrics{startTime: time.Now()}
	m.RecordSpawn(true, 150)
	m.RecordSpawn(false, 50)
	m.RecordNeMo(true, 3000)
	m.RecordHealthCheck(true)
	m.RecordHealthCheck(false)

	handler := m.Handler()

	req := httptest.NewRequest("GET", "/metrics", nil)
	rec := httptest.NewRecorder()

	handler(rec, req)

	resp := rec.Result()
	body, _ := io.ReadAll(resp.Body)
	output := string(body)

	// Check content type
	if resp.Header.Get("Content-Type") != "text/plain; version=0.0.4" {
		t.Errorf("wrong content type: %s", resp.Header.Get("Content-Type"))
	}

	// Check metrics are present
	expectedMetrics := []string{
		"urp_uptime_seconds",
		"urp_worker_spawns_total 2",
		"urp_worker_spawn_errors_total 1",
		"urp_nemo_launches_total 1",
		"urp_nemo_launch_errors_total 0",
		"urp_health_checks_total 2",
		"urp_health_check_failures_total 1",
		"urp_last_spawn_duration_ms 50",
		"urp_last_nemo_duration_ms 3000",
	}

	for _, expected := range expectedMetrics {
		if !strings.Contains(output, expected) {
			t.Errorf("missing metric: %s\nOutput:\n%s", expected, output)
		}
	}
}

func TestMetricsHandlerPrometheusFormat(t *testing.T) {
	m := &Metrics{startTime: time.Now()}
	handler := m.Handler()

	req := httptest.NewRequest("GET", "/metrics", nil)
	rec := httptest.NewRecorder()

	handler(rec, req)

	body, _ := io.ReadAll(rec.Result().Body)
	output := string(body)

	// Check Prometheus format (# HELP, # TYPE lines)
	if !strings.Contains(output, "# HELP urp_uptime_seconds") {
		t.Error("missing HELP comment for uptime")
	}
	if !strings.Contains(output, "# TYPE urp_uptime_seconds gauge") {
		t.Error("missing TYPE comment for uptime")
	}
	if !strings.Contains(output, "# TYPE urp_worker_spawns_total counter") {
		t.Error("missing TYPE comment for spawns counter")
	}
}

func TestNewServer(t *testing.T) {
	srv := NewServer(9999)
	if srv == nil {
		t.Fatal("NewServer returned nil")
	}
	if srv.srv.Addr != ":9999" {
		t.Errorf("expected addr ':9999', got '%s'", srv.srv.Addr)
	}
}

func TestHealthEndpoint(t *testing.T) {
	// Create a test server with the same mux as NewServer
	mux := http.NewServeMux()
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok"))
	})

	req := httptest.NewRequest("GET", "/health", nil)
	rec := httptest.NewRecorder()

	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", rec.Code)
	}
	if rec.Body.String() != "ok" {
		t.Errorf("expected 'ok', got '%s'", rec.Body.String())
	}
}

func TestConcurrentMetricsRecording(t *testing.T) {
	m := &Metrics{startTime: time.Now()}

	done := make(chan bool)

	// Spawn multiple goroutines recording metrics
	for i := 0; i < 100; i++ {
		go func() {
			m.RecordSpawn(true, 100)
			m.RecordNeMo(true, 200)
			m.RecordHealthCheck(true)
			m.RecordGraphWrite(true)
			done <- true
		}()
	}

	// Wait for all goroutines
	for i := 0; i < 100; i++ {
		<-done
	}

	// All should have been recorded
	if m.WorkerSpawns.Load() != 100 {
		t.Errorf("expected 100 spawns, got %d", m.WorkerSpawns.Load())
	}
	if m.NeMoLaunches.Load() != 100 {
		t.Errorf("expected 100 launches, got %d", m.NeMoLaunches.Load())
	}
	if m.HealthChecks.Load() != 100 {
		t.Errorf("expected 100 health checks, got %d", m.HealthChecks.Load())
	}
	if m.GraphWrites.Load() != 100 {
		t.Errorf("expected 100 graph writes, got %d", m.GraphWrites.Load())
	}
}
