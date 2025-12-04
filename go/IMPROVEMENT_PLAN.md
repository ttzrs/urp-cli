# URP Improvement Plan

## Current State Analysis

```
VERDICT: do - address real pain points (SELinux, socket fragility)
DATA: host_socket → container → NeMo (fragile chain)
REMOVE: --privileged, label=disable hacks
RISK: workers spawn but can't use docker (silent fail)
ACTION: dind-rootless → healthchecks → validation → observability
```

### Pain Points Identified
1. **SELinux blocks docker.sock** → required `--privileged` to work
2. **Silent failures** → entrypoint doesn't exit on docker group creation fail
3. **No validation** → SpawnWorker() doesn't verify socket access works
4. **Inconsistent mounts** → master/worker mount things differently
5. **No healthchecks** → zombie workers stay "running"

---

## Phase 0: Critical Fixes (1 day)

### 0.1 Stop Silent Failures in Entrypoint

**File:** `worker-entrypoint.sh`

```bash
# BEFORE (silent fail)
groupadd -g "$DOCKER_GID" docker 2>/dev/null || true
usermod -aG docker urp 2>/dev/null || true

# AFTER (fail fast with warning)
DOCKER_HEALTHY=false
if [[ -S /var/run/docker.sock ]]; then
    DOCKER_GID=$(stat -c '%g' /var/run/docker.sock 2>/dev/null)
    if [[ -n "$DOCKER_GID" ]]; then
        getent group docker >/dev/null 2>&1 || groupadd -g "$DOCKER_GID" docker 2>/dev/null
        usermod -aG docker urp 2>/dev/null
        # VERIFY it worked
        if docker ps >/dev/null 2>&1; then
            DOCKER_HEALTHY=true
            log "Docker access: OK"
        else
            log "WARN: Docker socket mounted but not accessible"
        fi
    fi
fi
export URP_DOCKER_HEALTHY=$DOCKER_HEALTHY
```

### 0.2 Add Pre-Spawn Validation in Go

**File:** `internal/container/manager.go`

```go
// ValidateSpawnRequirements checks environment before spawning
func (m *Manager) ValidateSpawnRequirements(projectPath string) error {
    // Check docker.sock exists
    if _, err := os.Stat("/var/run/docker.sock"); os.IsNotExist(err) {
        return fmt.Errorf("docker.sock not found: workers won't be able to control NeMo")
    }

    // Check project path exists
    if _, err := os.Stat(projectPath); os.IsNotExist(err) {
        return fmt.Errorf("project path does not exist: %s", projectPath)
    }

    // Check .env file exists
    homeDir := os.Getenv("URP_HOST_HOME")
    if homeDir == "" {
        homeDir, _ = os.UserHomeDir()
    }
    envFile := filepath.Join(homeDir, ".urp-go", ".env")
    if _, err := os.Stat(envFile); os.IsNotExist(err) {
        return fmt.Errorf("env file not found: %s", envFile)
    }

    return nil
}
```

### 0.3 Add Post-Spawn Health Check

```go
// VerifyWorkerHealth checks if spawned worker is functional
func (m *Manager) VerifyWorkerHealth(containerName string, timeout time.Duration) error {
    deadline := time.Now().Add(timeout)

    for time.Now().Before(deadline) {
        // Check container is running
        out, err := m.run("ps", "-q", "--filter", fmt.Sprintf("name=^%s$", containerName))
        if err != nil || out == "" {
            time.Sleep(500 * time.Millisecond)
            continue
        }

        // Check docker access inside container
        out, err = m.run("exec", containerName, "docker", "ps", "--format", "{{.Names}}")
        if err == nil {
            return nil // Healthy
        }

        time.Sleep(500 * time.Millisecond)
    }

    return fmt.Errorf("worker %s failed health check", containerName)
}
```

---

## Phase 1: SELinux-Safe Docker Access (2 days)

### 1.1 Option A: Docker-in-Docker Rootless (Recommended)

Instead of mounting host socket, each worker runs its own Docker daemon.

**New Dockerfile target: `worker-dind`**

```dockerfile
# ═══════════════════════════════════════════════════════════════
# Target: worker-dind - Worker with rootless Docker-in-Docker
# ═══════════════════════════════════════════════════════════════
FROM docker:27-dind-rootless AS dind-base

FROM worker AS worker-dind

# Copy rootless docker from official image
COPY --from=dind-base /usr/local/bin/dockerd-rootless.sh /usr/local/bin/
COPY --from=dind-base /usr/local/bin/docker /usr/local/bin/
COPY --from=dind-base /usr/local/bin/docker-init /usr/local/bin/

# Required for rootless
RUN apk add --no-cache \
    iptables ip6tables \
    fuse-overlayfs \
    && echo "urp:100000:65536" >> /etc/subuid \
    && echo "urp:100000:65536" >> /etc/subgid

ENV DOCKER_HOST=unix:///run/user/1000/docker.sock

# Start dockerd-rootless on entrypoint
```

**Benefits:**
- No host socket access needed
- Works with SELinux enforcing
- Each worker isolated
- Can spawn NeMo without host privileges

**New SpawnWorker mode:**

```go
func (m *Manager) SpawnWorker(projectPath string, workerNum int, opts SpawnOptions) (string, error) {
    // ...

    if opts.DindRootless {
        // Use worker-dind image, no host socket
        image = "urp:worker-dind"
        // No -v /var/run/docker.sock mount
    } else {
        // Current behavior
        image = URPWorkerImage
        args = append(args, "-v", "/var/run/docker.sock:/var/run/docker.sock")
    }
}
```

### 1.2 Option B: SELinux Context Labels (Simpler)

If dind-rootless is too complex, use proper SELinux labels:

```go
// For Podman with SELinux
if m.runtime == RuntimePodman && isSelinuxEnforcing() {
    args = append(args, "-v", "/var/run/docker.sock:/var/run/docker.sock:Z")
} else {
    args = append(args, "-v", "/var/run/docker.sock:/var/run/docker.sock")
}
```

Helper function:

```go
func isSelinuxEnforcing() bool {
    out, err := exec.Command("getenforce").Output()
    if err != nil {
        return false
    }
    return strings.TrimSpace(string(out)) == "Enforcing"
}
```

### 1.3 Auto-Detect Best Mode

```go
type DockerAccessMode string

const (
    ModeHostSocket   DockerAccessMode = "host"      // Direct socket mount
    ModeDindRootless DockerAccessMode = "dind"      // Docker-in-Docker rootless
    ModeNone         DockerAccessMode = "none"      // No docker access
)

func (m *Manager) DetectDockerMode() DockerAccessMode {
    // Check if running inside container
    if _, err := os.Stat("/.dockerenv"); err == nil {
        // Inside container - check if socket accessible
        if out, _ := exec.Command("docker", "ps").Output(); len(out) > 0 {
            return ModeHostSocket
        }
        return ModeDindRootless
    }

    // On host - direct socket
    if _, err := os.Stat("/var/run/docker.sock"); err == nil {
        return ModeHostSocket
    }

    return ModeNone
}
```

---

## Phase 2: Volume Management Refactor (1 day)

### 2.1 VolumeSpec Type

```go
// VolumeSpec defines a container volume mount
type VolumeSpec struct {
    Source      string
    Target      string
    ReadOnly    bool
    SELinuxMode string // "", "z", "Z"
}

func (v VolumeSpec) String() string {
    mode := "rw"
    if v.ReadOnly {
        mode = "ro"
    }
    if v.SELinuxMode != "" {
        mode += ":" + v.SELinuxMode
    }
    return fmt.Sprintf("%s:%s:%s", v.Source, v.Target, mode)
}

// StandardVolumes returns common volume mounts for URP containers
func StandardVolumes(projectPath, envFile string, readOnly bool) []VolumeSpec {
    return []VolumeSpec{
        {Source: projectPath, Target: "/workspace", ReadOnly: readOnly},
        {Source: VectorVolume, Target: "/var/lib/urp/vector", ReadOnly: false},
        {Source: envFile, Target: "/etc/urp/.env", ReadOnly: true},
    }
}
```

### 2.2 Unified Container Launcher

```go
type LaunchConfig struct {
    Name        string
    Image       string
    Network     string
    Volumes     []VolumeSpec
    Env         map[string]string
    DockerMode  DockerAccessMode
    Detached    bool
    Interactive bool
    Remove      bool
}

func (m *Manager) Launch(cfg LaunchConfig) (string, error) {
    args := []string{"run"}

    if cfg.Detached {
        args = append(args, "-d")
    }
    if cfg.Interactive {
        args = append(args, "-it")
    }
    if cfg.Remove {
        args = append(args, "--rm")
    }

    args = append(args, "--name", cfg.Name)
    args = append(args, "--network", cfg.Network)

    // SELinux handling
    if m.needsSelinuxDisable(cfg) {
        args = append(args, "--security-opt", "label=disable")
    }

    // Docker access
    if cfg.DockerMode == ModeHostSocket {
        args = append(args, "-v", "/var/run/docker.sock:/var/run/docker.sock")
    }

    // Volumes
    for _, v := range cfg.Volumes {
        args = append(args, "-v", v.String())
    }

    // Environment
    for k, v := range cfg.Env {
        args = append(args, "-e", fmt.Sprintf("%s=%s", k, v))
    }

    args = append(args, cfg.Image)

    return m.run(args...)
}
```

---

## Phase 3: Healthchecks & Auto-Recovery (1 day)

### 3.1 Docker Healthcheck in Dockerfile

```dockerfile
# Worker healthcheck
HEALTHCHECK --interval=30s --timeout=10s --start-period=5s --retries=3 \
    CMD urp selftest --quick || exit 1
```

### 3.2 Go Healthcheck Implementation

```go
// internal/selftest/docker.go

// CheckDockerAccess verifies the container can use docker
func CheckDockerAccess() error {
    cmd := exec.Command("docker", "ps", "--format", "{{.Names}}")
    out, err := cmd.Output()
    if err != nil {
        return fmt.Errorf("docker access failed: %w", err)
    }
    // If we get here, docker works
    return nil
}

// CheckNeMoReady verifies NeMo container can be spawned
func CheckNeMoReady(projectName string) error {
    // Check image exists
    cmd := exec.Command("docker", "image", "inspect", NeMoImage)
    if err := cmd.Run(); err != nil {
        return fmt.Errorf("NeMo image not found: %s", NeMoImage)
    }
    return nil
}
```

### 3.3 Worker Auto-Recovery

```go
// MonitorWorkers checks worker health and restarts unhealthy ones
func (m *Manager) MonitorWorkers(projectName string) {
    ticker := time.NewTicker(30 * time.Second)
    defer ticker.Stop()

    for range ticker.C {
        workers := m.ListWorkers(projectName)
        for _, w := range workers {
            if !strings.Contains(w.Status, "healthy") {
                log.Printf("Worker %s unhealthy, restarting...", w.Name)
                m.RestartWorker(w.Name)
            }
        }
    }
}
```

---

## Phase 4: Security Hardening (1 day)

### 4.1 Drop Capabilities

```go
// Add to LaunchConfig
SecurityOpts []string

// In Launch()
if !cfg.Privileged {
    args = append(args,
        "--cap-drop", "ALL",
        "--cap-add", "NET_BIND_SERVICE", // If needed
    )
}
```

### 4.2 Read-Only Root Filesystem

```go
if cfg.ReadOnlyRoot {
    args = append(args,
        "--read-only",
        "--tmpfs", "/tmp:rw,noexec,nosuid",
        "--tmpfs", "/run:rw,noexec,nosuid",
    )
}
```

### 4.3 Non-Root User in NeMo

**File:** Add to NeMo launcher

```go
func (m *Manager) LaunchNeMo(projectPath, containerName string) (string, error) {
    // ...
    args = append(args,
        // Run as non-root user if possible
        "--user", "1000:1000",
    )
    // ...
}
```

---

## Phase 5: Observability (2 days)

### 5.1 Structured JSON Logging

```go
// internal/logging/logger.go

type Event struct {
    Time      string `json:"ts"`
    Level     string `json:"level"`
    Component string `json:"component"`
    Event     string `json:"event"`
    Worker    string `json:"worker,omitempty"`
    Duration  int64  `json:"duration_ms,omitempty"`
    Error     string `json:"error,omitempty"`
    Extra     map[string]interface{} `json:"extra,omitempty"`
}

func LogEvent(e Event) {
    e.Time = time.Now().UTC().Format(time.RFC3339)
    data, _ := json.Marshal(e)
    fmt.Fprintln(os.Stderr, string(data))
}

// Usage:
LogEvent(Event{
    Level:     "info",
    Component: "worker",
    Event:     "spawn",
    Worker:    "postllm-w1",
    Extra:     map[string]interface{}{"image": "urp:worker"},
})
```

### 5.2 Prometheus Metrics

```go
// internal/metrics/prometheus.go

import "github.com/prometheus/client_golang/prometheus"

var (
    workersActive = prometheus.NewGaugeVec(
        prometheus.GaugeOpts{
            Name: "urp_workers_active",
            Help: "Number of active workers",
        },
        []string{"project"},
    )

    workerSpawnDuration = prometheus.NewHistogramVec(
        prometheus.HistogramOpts{
            Name:    "urp_worker_spawn_duration_seconds",
            Help:    "Time to spawn a worker",
            Buckets: []float64{0.5, 1, 2, 5, 10, 30},
        },
        []string{"project", "status"},
    )

    nemoExecDuration = prometheus.NewHistogramVec(
        prometheus.HistogramOpts{
            Name:    "urp_nemo_exec_duration_seconds",
            Help:    "Time to execute commands in NeMo",
            Buckets: []float64{1, 5, 10, 30, 60, 120, 300},
        },
        []string{"project", "status"},
    )
)
```

### 5.3 Events Log to Graph

```go
// Log spawn events to Memgraph
func (m *Manager) LogSpawnEvent(workerName, projectName string, success bool, duration time.Duration) {
    query := `
        CREATE (e:WorkerEvent {
            type: 'spawn',
            worker: $worker,
            project: $project,
            success: $success,
            duration_ms: $duration,
            timestamp: datetime()
        })
    `
    // Execute via graph client
}
```

---

## Phase 6: NeMo Fallback Mode (0.5 day)

### 6.1 CPU Fallback

```go
type NeMoConfig struct {
    GPU       bool
    CPUOnly   bool
    ShmSize   string
    Image     string
}

func (m *Manager) LaunchNeMo(projectPath, containerName string, cfg NeMoConfig) (string, error) {
    args := []string{
        "run", "-d",
        "--name", containerName,
        "--network", NetworkName,
    }

    if cfg.GPU {
        // Check if GPU available
        if m.hasNvidiaGPU() {
            args = append(args, "--gpus", "all")
            args = append(args, "--shm-size", cfg.ShmSize)
        } else if cfg.CPUOnly {
            log.Println("GPU not available, falling back to CPU mode")
        } else {
            return "", fmt.Errorf("GPU requested but not available")
        }
    }

    // ...
}

func (m *Manager) hasNvidiaGPU() bool {
    cmd := exec.Command("nvidia-smi")
    return cmd.Run() == nil
}
```

### 6.2 CLI Flag

```go
// In nemo start command
nemoStartCmd.Flags().Bool("cpu", false, "Run in CPU-only mode (no GPU required)")
```

---

## Implementation Order

```
Week 1:
├── Phase 0.1: Stop silent failures [4h]
├── Phase 0.2: Pre-spawn validation [2h]
├── Phase 0.3: Post-spawn health check [2h]
└── Phase 1.2: SELinux context labels [4h]

Week 2:
├── Phase 2.1: VolumeSpec type [2h]
├── Phase 2.2: Unified launcher [4h]
├── Phase 3.1: Dockerfile healthcheck [1h]
└── Phase 3.2: Go healthcheck [2h]

Week 3:
├── Phase 1.1: DinD rootless (optional) [8h]
├── Phase 4: Security hardening [4h]
└── Phase 6: NeMo fallback [4h]

Week 4:
├── Phase 5.1: JSON logging [2h]
├── Phase 5.2: Prometheus metrics [4h]
└── Phase 5.3: Events to graph [2h]
```

---

## Files to Create/Modify

### New Files
- `internal/container/volume.go` - VolumeSpec and helpers
- `internal/container/health.go` - Health check logic
- `internal/container/docker_mode.go` - Auto-detection
- `internal/logging/structured.go` - JSON logging
- `internal/metrics/prometheus.go` - Metrics

### Modified Files
- `internal/container/manager.go` - Validation, unified launcher
- `worker-entrypoint.sh` - Fail-fast, health export
- `master-entrypoint.sh` - Same improvements
- `Dockerfile` - Healthcheck, dind target
- `cmd/urp/main.go` - New flags (--cpu, --dind)

---

## Testing Matrix

| Scenario | Current | After Phase 0 | After Phase 1 |
|----------|---------|---------------|---------------|
| SELinux Enforcing + host socket | FAIL | WARN + retry | Auto-detect DinD |
| SELinux Permissive | OK | OK | OK |
| Docker Desktop (Mac/Win) | OK | OK | OK |
| Rootless Podman | FAIL | FAIL | Auto-detect |
| No GPU + NeMo | FAIL | FAIL | CPU fallback |

---

## Success Metrics

1. **Zero silent failures** - Every spawn issue produces clear error
2. **SELinux compatible** - Works without `--privileged` or `label=disable`
3. **Self-healing** - Unhealthy workers auto-restart within 60s
4. **Observable** - All events logged in JSON, metrics exposed
5. **Reproducible** - Same config produces same behavior across hosts
