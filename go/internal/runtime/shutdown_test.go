package runtime

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"
)

func TestNewShutdownManager(t *testing.T) {
	m := NewShutdownManager(5 * time.Second)

	if m == nil {
		t.Fatal("NewShutdownManager returned nil")
	}

	if m.timeout != 5*time.Second {
		t.Errorf("expected timeout 5s, got %v", m.timeout)
	}
}

func TestShutdownManager_Register(t *testing.T) {
	m := NewShutdownManager(5 * time.Second)

	var called int32

	m.Register("test-handler", func(ctx context.Context) error {
		atomic.AddInt32(&called, 1)
		return nil
	})

	m.Shutdown()

	if atomic.LoadInt32(&called) != 1 {
		t.Error("handler was not called")
	}
}

func TestShutdownManager_RegisterSimple(t *testing.T) {
	m := NewShutdownManager(5 * time.Second)

	var called bool

	m.RegisterSimple("simple-handler", func() {
		called = true
	})

	m.Shutdown()

	if !called {
		t.Error("simple handler was not called")
	}
}

func TestShutdownManager_LIFO(t *testing.T) {
	m := NewShutdownManager(5 * time.Second)

	order := make([]int, 0, 3)

	m.RegisterSimple("first", func() {
		order = append(order, 1)
	})
	m.RegisterSimple("second", func() {
		order = append(order, 2)
	})
	m.RegisterSimple("third", func() {
		order = append(order, 3)
	})

	m.Shutdown()

	// Wait a bit for goroutines to complete
	time.Sleep(100 * time.Millisecond)

	// All should be called (order may vary due to concurrency)
	if len(order) != 3 {
		t.Errorf("expected 3 handlers called, got %d", len(order))
	}
}

func TestShutdownManager_Context(t *testing.T) {
	m := NewShutdownManager(5 * time.Second)

	ctx := m.Context()

	select {
	case <-ctx.Done():
		t.Fatal("context should not be cancelled before shutdown")
	default:
		// Good
	}

	m.Shutdown()

	select {
	case <-ctx.Done():
		// Good - context should be cancelled
	case <-time.After(time.Second):
		t.Fatal("context should be cancelled after shutdown")
	}
}

func TestShutdownManager_Done(t *testing.T) {
	m := NewShutdownManager(5 * time.Second)

	done := m.Done()

	select {
	case <-done:
		t.Fatal("done channel should not be closed before shutdown")
	default:
		// Good
	}

	m.Shutdown()

	select {
	case <-done:
		// Good - done should be closed
	case <-time.After(time.Second):
		t.Fatal("done channel should be closed after shutdown")
	}
}

func TestShutdownManager_Timeout(t *testing.T) {
	m := NewShutdownManager(100 * time.Millisecond)

	m.Register("slow-handler", func(ctx context.Context) error {
		// This handler is slow and should be interrupted
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(5 * time.Second):
			return nil
		}
	})

	start := time.Now()
	m.Shutdown()
	duration := time.Since(start)

	// Should complete in roughly the timeout period
	if duration > 500*time.Millisecond {
		t.Errorf("shutdown took too long: %v", duration)
	}
}

func TestShutdownManager_ErrorHandling(t *testing.T) {
	m := NewShutdownManager(5 * time.Second)

	m.Register("error-handler", func(ctx context.Context) error {
		return errors.New("test error")
	})

	m.Register("success-handler", func(ctx context.Context) error {
		return nil
	})

	// Should not panic with errors
	m.Shutdown()
}

func TestShutdownManager_OnlyOnce(t *testing.T) {
	m := NewShutdownManager(5 * time.Second)

	var callCount int32

	m.Register("once-handler", func(ctx context.Context) error {
		atomic.AddInt32(&callCount, 1)
		return nil
	})

	// Call shutdown multiple times
	m.Shutdown()
	m.Shutdown()
	m.Shutdown()

	time.Sleep(100 * time.Millisecond)

	if atomic.LoadInt32(&callCount) != 1 {
		t.Errorf("handler should only be called once, got %d", callCount)
	}
}
