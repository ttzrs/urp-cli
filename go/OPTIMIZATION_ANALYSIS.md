# URP Optimization Analysis

## Benchmark Results (2025-12-04)

### Performance Summary

| Category | Avg Time | Status |
|----------|----------|--------|
| Infrastructure | 25ms | Good |
| Code Analysis | 9ms | Excellent |
| Git Operations | 102ms | Needs work |
| Cognitive | 9ms | Excellent |
| OpenCode | 9ms | Excellent |
| Focus | 12ms | Excellent |
| Events | 10ms | Excellent |
| Audit | 5ms | Excellent |

### Bottlenecks Identified

1. **git ingest (102ms)** - Main bottleneck
   - Spawns `git log` subprocess
   - Parses output line by line
   - Multiple DB writes

2. **infra status (43ms)** - Container checks
   - Calls docker/podman ps
   - Network inspection

3. **sys vitals (25ms)** - System queries
   - Multiple graph queries

### Optimization Targets

1. **Batch Graph Writes** - Already done for GetStats
2. **Connection Pooling** - Single connection reused ✓
3. **Lazy Loading** - Only connect when needed ✓
4. **Parallel Operations** - git/code ingest could parallelize

## Architecture Review

### Current Structure
```
cmd/urp/main.go          # 1400+ lines - TOO BIG
internal/
  ├── graph/             # DB abstraction ✓
  ├── ingest/            # Code parsing ✓
  ├── query/             # Graph queries ✓
  ├── memory/            # Session memory ✓
  ├── cognitive/         # Think/KB ✓
  ├── opencode/          # Integrated ✓
  └── ... 15+ packages
```

### Issues
1. main.go is monolithic (1400+ lines)
2. Each command creates new DB connection
3. No query result caching
4. Audit logging on every command

## Proposed Optimizations

### V2 Architecture

```
cmd/urp/
  ├── main.go           # Entry + root cmd only
  ├── code.go           # Code subcommands
  ├── git.go            # Git subcommands
  ├── oc.go             # OpenCode subcommands
  └── ...

internal/
  ├── core/             # Shared context, DB pool
  ├── graph/            # Unchanged
  ├── ingest/           # + parallel parsing
  └── ...
```

### Key Changes

1. **Lazy audit** - Only log on explicit flag or errors
2. **Query cache** - LRU cache for frequent queries
3. **Parallel git** - Parse commits in goroutines
4. **Single main.go split** - By command group
5. **Precompiled queries** - Common Cypher as constants

### Expected Improvements

| Operation | Current | Target | Improvement |
|-----------|---------|--------|-------------|
| git ingest | 102ms | 40ms | 60% |
| infra status | 43ms | 20ms | 53% |
| sys vitals | 25ms | 10ms | 60% |
| Average | 15ms | 8ms | 47% |
