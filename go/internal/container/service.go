// Package container service provides high-level container operations.
// Service wraps Manager and returns structured results for CLI rendering.
package container

import (
	"context"
	"fmt"
	"path/filepath"
	"sync"

	"github.com/joss/urp/internal/config"
)

// Service provides high-level container operations with structured results.
// It separates business logic from CLI presentation concerns.
type Service struct {
	mgr *Manager
}

// NewService creates a new container service.
func NewService(ctx context.Context) *Service {
	return &Service{mgr: NewManager(ctx)}
}

// NewServiceForProject creates a service with project scope.
func NewServiceForProject(ctx context.Context, project string) *Service {
	return &Service{mgr: NewManagerForProject(ctx, project)}
}

// ─────────────────────────────────────────────────────────────────────────────
// Infrastructure Operations
// ─────────────────────────────────────────────────────────────────────────────

// InfraResult holds the result of an infrastructure operation.
type InfraResult struct {
	Project      string
	NetworkName  string
	MemgraphName string
	Error        error
}

// StartInfra starts infrastructure and returns structured result.
func (s *Service) StartInfra() *InfraResult {
	project := s.mgr.Project()
	result := &InfraResult{
		Project:      project,
		NetworkName:  NetworkName(project),
		MemgraphName: MemgraphName(project),
	}
	result.Error = s.mgr.StartInfra()
	return result
}

// StopInfra stops infrastructure for the project.
func (s *Service) StopInfra() *InfraResult {
	project := s.mgr.Project()
	result := &InfraResult{
		Project:      project,
		NetworkName:  NetworkName(project),
		MemgraphName: MemgraphName(project),
	}
	result.Error = s.mgr.StopInfra()
	return result
}

// CleanInfra removes all infrastructure for the project.
func (s *Service) CleanInfra() *InfraResult {
	project := s.mgr.Project()
	result := &InfraResult{
		Project:      project,
		NetworkName:  NetworkName(project),
		MemgraphName: MemgraphName(project),
	}
	result.Error = s.mgr.CleanInfra()
	return result
}

// Status returns the current infrastructure status.
func (s *Service) Status() *InfraStatus {
	return s.mgr.Status()
}

// ─────────────────────────────────────────────────────────────────────────────
// Launch Operations
// ─────────────────────────────────────────────────────────────────────────────

// LaunchResult holds the result of a launch operation.
type LaunchResult struct {
	ContainerName string
	Interactive   bool
	Error         error
}

// LaunchMaster starts a master container.
func (s *Service) LaunchMaster(path string) *LaunchResult {
	name, err := s.mgr.LaunchMaster(path)
	return &LaunchResult{
		ContainerName: name,
		Interactive:   true,
		Error:         err,
	}
}

// LaunchStandalone starts a standalone container.
func (s *Service) LaunchStandalone(path string, readOnly bool) *LaunchResult {
	name, err := s.mgr.LaunchStandalone(path, readOnly)
	return &LaunchResult{
		ContainerName: name,
		Interactive:   false,
		Error:         err,
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// Worker Operations
// ─────────────────────────────────────────────────────────────────────────────

// SpawnResult holds the result of a spawn operation.
type SpawnResult struct {
	Num   int
	Name  string
	Error error
}

// SpawnWorker spawns a single worker.
func (s *Service) SpawnWorker(path string, num int) *SpawnResult {
	name, err := s.mgr.SpawnWorker(path, num)
	return &SpawnResult{Num: num, Name: name, Error: err}
}

// SpawnWorkersParallel spawns multiple workers concurrently.
func (s *Service) SpawnWorkersParallel(path string, count int) []*SpawnResult {
	results := make([]*SpawnResult, count)
	var wg sync.WaitGroup
	var mu sync.Mutex

	for i := 1; i <= count; i++ {
		wg.Add(1)
		go func(num int) {
			defer wg.Done()
			name, err := s.mgr.SpawnWorker(path, num)
			mu.Lock()
			results[num-1] = &SpawnResult{Num: num, Name: name, Error: err}
			mu.Unlock()
		}(i)
	}

	wg.Wait()
	return results
}

// ListWorkers returns all worker containers.
func (s *Service) ListWorkers() []ContainerStatus {
	return s.mgr.ListWorkers(s.mgr.Project())
}

// AttachWorker opens interactive shell in a worker.
func (s *Service) AttachWorker(name string) error {
	return s.mgr.AttachWorker(name)
}

// Exec runs a command in a container.
func (s *Service) Exec(name, command string) error {
	return s.mgr.Exec(name, command)
}

// KillWorker stops and removes a worker.
func (s *Service) KillWorker(name string) error {
	return s.mgr.KillWorker(name)
}

// KillAllWorkers stops all workers for the project.
func (s *Service) KillAllWorkers() error {
	return s.mgr.KillAllWorkers(s.mgr.Project())
}

// ─────────────────────────────────────────────────────────────────────────────
// Health Operations
// ─────────────────────────────────────────────────────────────────────────────

// HealthResult holds health check results for all workers.
type HealthResult struct {
	Workers  []*WorkerHealth
	Healthy  int
	Unhealthy int
}

// CheckAllHealth returns health status of all workers.
func (s *Service) CheckAllHealth() *HealthResult {
	workers := s.ListWorkers()
	result := &HealthResult{
		Workers: make([]*WorkerHealth, 0, len(workers)),
	}

	for _, w := range workers {
		health := s.mgr.CheckWorkerHealth(w.Name)
		result.Workers = append(result.Workers, health)
		if health.Status == "healthy" || health.Status == "running" {
			result.Healthy++
		} else {
			result.Unhealthy++
		}
	}
	return result
}

// RestartUnhealthy restarts unhealthy workers and returns names of restarted.
func (s *Service) RestartUnhealthy() []string {
	return s.mgr.MonitorAndRestartUnhealthy(s.mgr.Project())
}

// ─────────────────────────────────────────────────────────────────────────────
// Logs Operations
// ─────────────────────────────────────────────────────────────────────────────

// LogsResult holds container logs.
type LogsResult struct {
	ContainerName string
	Logs          string
	Tail          int
	Error         error
}

// Logs returns container logs.
func (s *Service) Logs(name string, tail int) *LogsResult {
	// Default to project memgraph if no name given
	if name == "" {
		name = MemgraphName(s.mgr.Project())
	}
	logs, err := s.mgr.Logs(name, tail)
	return &LogsResult{
		ContainerName: name,
		Logs:          logs,
		Tail:          tail,
		Error:         err,
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// NeMo Operations
// ─────────────────────────────────────────────────────────────────────────────

// NeMoContainerName returns the NeMo container name for current project.
func (s *Service) NeMoContainerName() string {
	project := s.mgr.Project()
	if project == "" {
		// Fallback to cwd basename
		if cwd, err := filepath.Abs("."); err == nil {
			project = filepath.Base(cwd)
		}
	}
	return fmt.Sprintf("urp-nemo-%s", project)
}

// LaunchNeMo starts a NeMo container.
func (s *Service) LaunchNeMo(path string) *LaunchResult {
	name, err := s.mgr.LaunchNeMo(path, "")
	return &LaunchResult{ContainerName: name, Error: err}
}

// ExecNeMo runs a command in NeMo container.
func (s *Service) ExecNeMo(command string) (string, error) {
	return s.mgr.ExecNeMo(s.NeMoContainerName(), command)
}

// KillNeMo stops the NeMo container.
func (s *Service) KillNeMo() error {
	return s.mgr.KillNeMo(s.NeMoContainerName())
}

// ─────────────────────────────────────────────────────────────────────────────
// Helper Functions
// ─────────────────────────────────────────────────────────────────────────────

// ProjectPath returns the resolved project path for worker mounts.
func ProjectPath() string {
	if path := config.Env().HostPath; path != "" {
		return path
	}
	if cwd, err := filepath.Abs("."); err == nil {
		return cwd
	}
	return "."
}
