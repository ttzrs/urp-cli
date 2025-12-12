# URP Testing Guide

This guide covers running tests, debugging failures, and writing new tests for URP.

---

## Quick Start

### Run All Tests

```bash
cd go

# Run all tests
go test ./...

# Run with verbose output
go test -v ./...

# Run with coverage
go test -cover ./...

# Generate coverage report
go test -coverprofile=coverage.out ./...
go tool cover -html=coverage.out  # Open in browser
```

### Run Specific Tests

```bash
# Test single package
go test ./internal/logging/...
go test ./internal/graph/...
go test ./internal/memory/...

# Test single function
go test -run TestRecoveryHandler_WrapPanic ./internal/logging/

# Test with pattern matching
go test -run "TestRecover" ./...
go test -run "TestMemory.*Session" ./...
```

---

## Test Coverage

### Current Test Status

| Package | Tests | Status | Notes |
|---------|-------|--------|-------|
| `internal/logging` | 6+ | ✅ Passing | Panic recovery tests |
| `internal/graph` | 86+ | ✅ Passing | Graph database tests |
| `internal/memory` | Multiple | ✅ Passing | Session/Knowledge base |
| `internal/opencode/agent` | Multiple | ⚠️ Check | May have failures |
| `internal/cognitive` | Multiple | ✅ Passing | Wisdom/Learn tests |
| `internal/container` | Multiple | ✅ Passing | Container management |
| `internal/opencode/model` | Multiple | ✅ Passing | Model registry tests |

Run coverage analysis:
```bash
go test -cover ./...
```

### Test File Locations

Tests are co-located with source files:
```
internal/
├── logging/
│   ├── recovery.go
│   └── recovery_test.go      # Test file next to implementation
├── graph/
│   ├── memgraph.go
│   ├── cache.go
│   └── *_test.go             # Multiple test files
├── memory/
│   ├── session.go
│   ├── knowledge.go
│   └── *_test.go
```

---

## Testing Patterns

### Unit Tests

Test individual functions in isolation:

```go
// internal/logging/recovery_test.go
func TestRecoveryHandler_WrapPanic(t *testing.T) {
    handler := &RecoveryHandler{}

    // Test that panic is caught and logged
    assert.NotPanics(t, func() {
        handler.Wrap(func() {
            panic("test panic")
        })
    })
}
```

Run:
```bash
go test -run TestRecoveryHandler_WrapPanic ./internal/logging/
```

### Integration Tests

Test multiple components working together:

```go
// Test agent with real graph database
func TestAgent_WithGraph(t *testing.T) {
    // Setup: connect to Memgraph
    db, err := graph.Connect(...)
    require.NoError(t, err)

    // Run agent
    agent := opencode.NewAgent(db, ...)
    result, err := agent.Execute(...)

    // Verify: check graph state
    assert.NoError(t, err)
    assert.NotNil(t, result)
}
```

Run with services:
```bash
# Start Memgraph first
docker compose up -d memgraph

# Run integration tests
go test -v ./internal/opencode/agent/...
```

### Table-Driven Tests

Test multiple scenarios efficiently:

```go
func TestCompiler_Modes(t *testing.T) {
    tests := []struct {
        name     string
        mode     string
        expected int  // Token count
    }{
        {"Full mode", "full", 10000},
        {"Minimal mode", "minimal", 1000},
        {"Delta mode", "delta", 2000},
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            c := NewCompiler()
            result := c.Compile(tt.mode)
            assert.Equal(t, tt.expected, len(result))
        })
    }
}
```

Run:
```bash
go test -v -run TestCompiler_Modes ./internal/compiler/
```

---

## Running Tests by Category

### Logging & Recovery

```bash
# Panic recovery tests
go test -v ./internal/logging/

# These tests intentionally panic - that's normal!
# Output should show recovered panics being logged
```

**Expected output:**
```
✓ TestRecoveryHandler_WrapPanic
✓ TestRecoveryHandler_WrapError
✓ TestSafeGo
✓ TestRecover
```

### Graph Database Tests

```bash
# Requires Memgraph running
docker compose up -d memgraph

# Run graph tests
go test -v ./internal/graph/...

# Test caching specifically
go test -run "Cache" -v ./internal/graph/
```

### Memory (Session & Knowledge Base)

```bash
# Session memory (fast, no dependencies)
go test -v ./internal/memory/

# Knowledge base tests (may require setup)
go test -run "Knowledge" -v ./internal/memory/
```

### Agent & Tools

```bash
# Agent executor tests
go test -v ./internal/opencode/agent/

# Specific tool tests
go test -run "Bash\|Read\|Write" -v ./internal/opencode/tool/

# Model registry
go test -v ./internal/opencode/model/
```

### Cognitive System

```bash
# Wisdom (vector search)
go test -run "Wisdom" -v ./internal/cognitive/

# Learning system
go test -run "Learn" -v ./internal/cognitive/

# Novelty detection
go test -run "Novelty" -v ./internal/cognitive/
```

---

## Debugging Test Failures

### Common Issues

#### 1. Memgraph Not Running
```
Error: cannot connect to bolt://localhost:7687
```

**Fix:**
```bash
docker compose up -d memgraph
docker logs urp-memgraph  # Check status

# Try running test again
go test -v ./internal/graph/...
```

#### 2. Port Already in Use
```
Error: address already in use :7687
```

**Fix:**
```bash
# Find and kill existing process
lsof -i :7687
kill -9 <PID>

# Or use different port
NEO4J_URI=bolt://localhost:7688 go test -v ./...
```

#### 3. Test Timeout
```
timeout: test execution timed out
```

**Fix:**
```bash
# Increase timeout
go test -timeout 30s ./...  # 30 seconds

# Or disable timeout for debugging
go test -timeout 0 ./...
```

#### 4. Race Conditions
```
WARNING: DATA RACE
```

**Fix:**
```bash
# Run with race detector
go test -race ./...

# This catches concurrent access issues
```

---

## Test Output Flags

### Useful Flags

```bash
# Verbose output (show test names)
go test -v ./...

# Only failed tests
go test -v ./... 2>&1 | grep -E "FAIL|RUN"

# Show test duration
go test -v -bench . ./...

# Keep test cache disabled
go test -count=1 ./...

# Run in parallel (faster)
go test -parallel 4 ./...

# Limit parallelism for flaky tests
go test -parallel 1 ./...
```

### Analyzing Output

```bash
# Run and save output
go test -v ./... > test_output.log 2>&1

# Find failed tests
grep "FAIL" test_output.log

# Find slowest tests
grep -E "--- PASS:|--- FAIL:" test_output.log | sort -k5 -rn | head -10

# Extract panic stack traces
grep -A 20 "panic" test_output.log
```

---

## Writing Tests

### Test File Naming

```go
// Source file:
// internal/auth/provider.go

// Test file (same package):
// internal/auth/provider_test.go  ← MUST end with _test.go

package auth_test  // Can use _test suffix for isolation

import "testing"

func TestProvider_Authenticate(t *testing.T) {
    // Test implementation
}
```

### Test Function Signature

```go
func TestComponentName_Action(t *testing.T) {
    // Arrange: set up test data
    input := "test"

    // Act: run the code
    result := Component.Do(input)

    // Assert: verify results
    if result != "expected" {
        t.Errorf("Expected 'expected', got '%s'", result)
    }
}
```

### Using `require` and `assert`

```go
import "github.com/stretchr/testify/require"
import "github.com/stretchr/testify/assert"

func TestWithAssert(t *testing.T) {
    // assert: continue on failure
    assert.NoError(t, err)
    assert.Equal(t, expected, actual)

    // require: stop on failure
    require.NoError(t, err)
    require.NotNil(t, result)
}
```

### Mocking Dependencies

```go
type MockDB struct {
    mock.Mock
}

func (m *MockDB) Query(ctx context.Context, q string) ([]Record, error) {
    args := m.Called(ctx, q)
    return args.Get(0).([]Record), args.Error(1)
}

func TestWithMock(t *testing.T) {
    mockDB := new(MockDB)
    mockDB.On("Query", mock.Anything, "SELECT...").Return([]Record{}, nil)

    // Test with mock
    result := Component.Process(mockDB)

    // Verify mock was called
    mockDB.AssertCalled(t, "Query", mock.Anything, mock.MatchedBy(func(q string) bool {
        return strings.Contains(q, "SELECT")
    }))
}
```

---

## Continuous Integration

### Pre-Commit Checks

```bash
#!/bin/bash
# .git/hooks/pre-commit

# Run tests before commit
go test ./... || exit 1

# Run linter
go vet ./... || exit 1

# Format code
go fmt ./...
```

### Build Pipeline

```bash
# Full pre-release checklist
go fmt ./...         # Format
go vet ./...         # Lint
go test -v ./...     # Test
go test -race ./...  # Race detector
go build -o urp ./cmd/urp  # Build

echo "✅ All checks passed!"
```

---

## Performance Testing

### Benchmarking

```bash
# Run benchmarks
go test -bench=. ./internal/compiler/

# Compare benchmarks
go test -bench=. -benchmem ./internal/graph/

# Run specific benchmark
go test -bench=BenchmarkQuery -run=^$ ./internal/graph/
```

### Profiling

```bash
# CPU profile
go test -cpuprofile=cpu.prof ./internal/compiler/
go tool pprof cpu.prof

# Memory profile
go test -memprofile=mem.prof ./internal/memory/
go tool pprof mem.prof

# Trace analysis
go test -trace=trace.out ./internal/agent/
go tool trace trace.out
```

---

## Testing Best Practices

### ✅ Do

- **Name tests clearly**: `TestCompiler_ModeSelection_ReturnsMinimalContext`
- **Test one thing**: Each test should verify one behavior
- **Use subtests**: `t.Run("case1", func(t *testing.T) {...})`
- **Clean up**: Use `t.Cleanup()` or `defer` for teardown
- **Mock external dependencies**: Don't hit real APIs in tests
- **Use table-driven tests**: For multiple scenarios
- **Document test purpose**: Add comments for non-obvious tests

### ❌ Don't

- **Test implementation details**: Test behavior, not internals
- **Create test interdependencies**: Each test should be independent
- **Use global state**: Use test fixtures instead
- **Ignore test output**: Read failure messages carefully
- **Skip cleanup**: Memory leaks and port conflicts
- **Hard-code timeouts**: Use `context.WithTimeout()`

---

## Quick Reference

```bash
# Run all tests
go test ./...

# Run with verbose output
go test -v ./...

# Run specific package
go test ./internal/graph/

# Run specific test
go test -run TestName ./internal/package/

# Run with coverage
go test -cover ./...

# Generate HTML coverage
go test -coverprofile=coverage.out ./...
go tool cover -html=coverage.out

# Run with race detector
go test -race ./...

# Increase timeout
go test -timeout 60s ./...

# Keep test cache disabled
go test -count=1 ./...

# Run benchmarks
go test -bench=. ./internal/package/

# View pprof profile
go test -cpuprofile=cpu.prof ./internal/package/
go tool pprof cpu.prof
```

---

## Troubleshooting

| Problem | Command | Solution |
|---------|---------|----------|
| Tests hang | `go test -timeout 30s ./...` | Increase timeout or add context |
| Race condition detected | `go test -race ./...` | Fix concurrent access |
| Flaky test | `go test -count=10 -run Test ./...` | Run multiple times to find races |
| Test cache issue | `go test -count=1 ./...` | Bypass cache |
| Port conflict | `docker ps \| grep -i urp` | Stop conflicting containers |
| Memgraph disconnect | `docker logs urp-memgraph` | Check DB health |

---

## See Also

- [QUICKSTART.md](./QUICKSTART.md) - First-time setup
- [ARCHITECTURE.md](./ARCHITECTURE.md) - System design
- [CLAUDE.md](../CLAUDE.md) - Development commands
- [Go Testing Docs](https://golang.org/doc/effective_go#testing)
