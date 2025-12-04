# URP Operations Runbook

## Quick Diagnostics

```bash
# Full health check
urp selftest

# Quick check
urp selftest --quick

# System status
urp sys vitals
urp infra status
```

## Common Errors

### 1. Memgraph Connection Timeout

**Symptom:**
```
Error: connection timeout after 30s
```

**Diagnosis:**
```bash
# Check if memgraph is running
docker ps | grep memgraph
podman ps | grep memgraph

# Check connection
nc -zv localhost 7687
```

**Solution:**
```bash
# Start memgraph
urp infra start

# Or manually
docker run -d --name urp-memgraph -p 7687:7687 memgraph/memgraph
```

### 2. Docker Socket Permission Denied

**Symptom:**
```
Error: permission denied while trying to connect to Docker daemon
```

**Diagnosis:**
```bash
ls -la /var/run/docker.sock
groups $USER | grep docker
```

**Solution:**
```bash
# Add user to docker group
sudo usermod -aG docker $USER
newgrp docker

# Or use podman (rootless)
export URP_RUNTIME=podman
```

### 3. SELinux Blocking Mounts

**Symptom:**
```
Error: permission denied on /workspace
```

**Diagnosis:**
```bash
getenforce
ausearch -m avc -ts recent | grep urp
```

**Solution:**
```bash
# Option 1: Use :z suffix (shared label)
# Already handled by URP automatically on SELinux systems

# Option 2: Set permissive for testing
sudo setenforce 0

# Option 3: Create proper policy (production)
# Contact sysadmin
```

### 4. Worker Spawn Fails

**Symptom:**
```
Error: worker container failed to start
```

**Diagnosis:**
```bash
# Check worker logs
urp infra logs

# Check container state
docker ps -a | grep urp-worker
docker logs urp-worker-1
```

**Solution:**
```bash
# Clean and retry
urp kill --all
urp infra clean
urp launch .
```

### 5. GPU Not Detected

**Symptom:**
```
GPU: Not available (nvidia-smi not found)
```

**Diagnosis:**
```bash
# Check nvidia driver
nvidia-smi

# Check container toolkit
docker run --rm --gpus all nvidia/cuda:12.0-base nvidia-smi
```

**Solution:**
```bash
# Install nvidia-container-toolkit
# Fedora:
sudo dnf install nvidia-container-toolkit
sudo systemctl restart docker

# Force CPU mode
export URP_GPU_MODE=cpu
```

### 6. Vector Store Errors

**Symptom:**
```
Error: vector store not available
```

**Diagnosis:**
```bash
# Check vector stats
urp vec stats

# Check volume
ls -la ~/.urp-go/data/vector/
```

**Solution:**
```bash
# Reinitialize vector store
rm -rf ~/.urp-go/data/vector/
urp code ingest
```

### 7. Out of Memory

**Symptom:**
```
Error: OOM killed
```

**Diagnosis:**
```bash
# Check memory usage
docker stats
free -h
```

**Solution:**
```bash
# Limit container memory
export URP_WORKER_MEMORY=4g

# Or reduce workers
urp kill --all
urp spawn 1  # Only one worker
```

## Health Endpoint Response

```json
{
  "status": "healthy|degraded|unhealthy",
  "uptime": "2h15m30s",
  "components": {
    "memgraph": {"status": "ok", "latency_ms": 5},
    "docker": {"status": "ok", "latency_ms": 10},
    "vector_store": {"status": "ok", "latency_ms": 1}
  },
  "last_error": "",
  "timestamp": "2025-12-04T12:00:00Z"
}
```

**Status meanings:**
- `healthy`: All components OK
- `degraded`: Some components slow (latency > 100ms)
- `unhealthy`: Critical component failed

## Log Analysis

```bash
# Filter by component
urp infra logs 2>&1 | grep '"component":"container"'

# Filter by level
urp infra logs 2>&1 | grep '"level":"error"'

# Filter by request ID (for tracing)
urp infra logs 2>&1 | grep '"request_id":"abc123"'
```

## Emergency Procedures

### Full Reset
```bash
urp kill --all
urp infra stop
urp infra clean
docker system prune -f
urp infra start
```

### Backup Before Reset
```bash
urp backup export ~/urp-backup.tar.gz
# ... reset ...
urp backup import ~/urp-backup.tar.gz
```
