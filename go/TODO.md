# URP TODO

## Completed (P0 - Reliability)

- [x] P0.1: Panic recovery middleware con stack trace (`internal/logging/recovery.go`)
- [x] P0.2: Graceful shutdown handler (`internal/runtime/shutdown.go`)
- [x] P0.3: Log rotation para alerts (`internal/alerts/alerts.go`)

## Pending - Orchestration System

### P1: `urp launch` - Master Container
- [ ] Create master container with project:ro mount
- [ ] Auto-ingest: `urp code ingest && urp git ingest`
- [ ] Open Claude CLI (interactive)
- [ ] SELinux :z labels for volumes

### P2: `urp spawn` - Ephemeral Workers
- [ ] Spawn worker containers (docker --rm)
- [ ] Workers have project:rw access
- [ ] Workers can install tools, modify code
- [ ] Worker types: default, browser, test

### P3: Graph Schema Plan/Task/Result
- [ ] `:Plan` node with status, project
- [ ] `:Task` node with description, worker_id
- [ ] `:TaskResult` with success, changes, summary
- [ ] Relationships: HAS_TASK, EXECUTED_BY, PRODUCED, DEPENDS_ON

### P4: Envelope Wrapper
- [ ] JSON-lines protocol over stdin/stdout
- [ ] Message types: instruction, status, result, error
- [ ] Intercept Claude CLI I/O
- [ ] Log envelopes to graph

### P5: X11 Browser Worker
- [ ] X11 forwarding setup
- [ ] Firefox/Chrome in container
- [ ] Visual testing support

## Implemented Features

- `urp code ingest|deps|impact|dead|cycles|hotspots|stats`
- `urp git ingest|history`
- `urp think wisdom|novelty|learn`
- `urp mem add|recall|list|stats|clear`
- `urp kb store|query|list|reject|promote|stats`
- `urp focus <target> [-d depth]`
- `urp sys vitals|topology|health|runtime`
- `urp events run|list|errors`
- `urp vec stats|search|add`
- `urp alert send|list|resolve|active`
- Alerts system with Claude hooks injection
- Vector store with local embeddings
