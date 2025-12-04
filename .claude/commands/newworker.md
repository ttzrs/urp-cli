# /newworker - Spawn a Claude Worker

You are inside a URP Master container (READ-ONLY). When the user runs `/newworker`, you must spawn a new worker container with WRITE access and launch Claude CLI inside it.

## Instructions

Execute this sequence:

1. **Spawn worker container** using `urp-spawn`:
```bash
urp-spawn 1
```

This creates a worker container named `urp-worker-${PROJECT_NAME}-1` with:
- WRITE access to `/codebase`
- Same Memgraph connection
- Claude API configured (ANTHROPIC_BASE_URL, ANTHROPIC_API_KEY)

2. **Launch Claude CLI in the worker**:
```bash
docker exec -it urp-worker-${PROJECT_NAME}-1 claude
```

3. **Report the worker status**:
```bash
urp-workers
```

## Worker Management Commands (available in master)

- `urp-spawn [n]` - Spawn worker n (default: 1)
- `urp-attach [n]` - Attach to worker n (bash shell)
- `urp-exec n cmd` - Execute command in worker n
- `urp-workers` - List all workers
- `urp-kill [n]` - Kill worker n
- `urp-kill-all` - Kill all workers

## Communication Pattern

As master, you can:
1. **Send tasks to worker Claude**: `urp-exec 1 claude -p "your task here"`
2. **Monitor worker output**: Check logs via `docker logs urp-worker-${PROJECT_NAME}-1`
3. **Verify changes**: Read files from `/codebase` (read-only in master)

## Response Format

After spawning, confirm:
```
Worker 1 spawned: urp-worker-{project}-1
Claude CLI launched. The worker has WRITE access to the codebase.

To interact with worker:
- urp-exec 1 claude -p "task"  # Send task to Claude
- urp-attach 1                  # Open bash in worker
- urp-workers                   # List workers
```
