// Package runtime provides graceful shutdown handling for URP processes.
package runtime

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"
)

// ShutdownFunc is a cleanup function called during shutdown
type ShutdownFunc func(ctx context.Context) error

// ShutdownManager handles graceful shutdown of the application
type ShutdownManager struct {
	mu          sync.Mutex
	handlers    []namedHandler
	timeout     time.Duration
	shutdownCtx context.Context
	cancel      context.CancelFunc
	done        chan struct{}
	once        sync.Once
}

type namedHandler struct {
	name string
	fn   ShutdownFunc
}

// DefaultShutdownTimeout is the default timeout for cleanup operations
const DefaultShutdownTimeout = 30 * time.Second

var (
	globalManager *ShutdownManager
	managerOnce   sync.Once
)

// Global returns the global shutdown manager
func Global() *ShutdownManager {
	managerOnce.Do(func() {
		globalManager = NewShutdownManager(DefaultShutdownTimeout)
	})
	return globalManager
}

// NewShutdownManager creates a new shutdown manager with specified timeout
func NewShutdownManager(timeout time.Duration) *ShutdownManager {
	ctx, cancel := context.WithCancel(context.Background())
	return &ShutdownManager{
		handlers:    make([]namedHandler, 0),
		timeout:     timeout,
		shutdownCtx: ctx,
		cancel:      cancel,
		done:        make(chan struct{}),
	}
}

// Register adds a cleanup handler to be called during shutdown
// Handlers are called in reverse order (LIFO) - last registered, first called
func (m *ShutdownManager) Register(name string, fn ShutdownFunc) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.handlers = append(m.handlers, namedHandler{name: name, fn: fn})
}

// RegisterSimple adds a simple cleanup function (no error return)
func (m *ShutdownManager) RegisterSimple(name string, fn func()) {
	m.Register(name, func(ctx context.Context) error {
		fn()
		return nil
	})
}

// Context returns a context that is cancelled when shutdown begins
func (m *ShutdownManager) Context() context.Context {
	return m.shutdownCtx
}

// Done returns a channel that's closed when shutdown is complete
func (m *ShutdownManager) Done() <-chan struct{} {
	return m.done
}

// ListenForSignals starts listening for shutdown signals (SIGTERM, SIGINT)
// This is non-blocking and should be called once at startup
func (m *ShutdownManager) ListenForSignals() {
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGTERM, syscall.SIGINT)

	go func() {
		sig := <-sigChan
		fmt.Fprintf(os.Stderr, "\nReceived signal: %v, initiating graceful shutdown...\n", sig)
		m.Shutdown()
	}()
}

// Shutdown initiates graceful shutdown - can only be called once
func (m *ShutdownManager) Shutdown() {
	m.once.Do(func() {
		m.performShutdown()
	})
}

// performShutdown executes all cleanup handlers
func (m *ShutdownManager) performShutdown() {
	defer close(m.done)

	// Cancel the main context to signal all operations to stop
	m.cancel()

	// Create timeout context for cleanup
	ctx, cancel := context.WithTimeout(context.Background(), m.timeout)
	defer cancel()

	m.mu.Lock()
	handlers := make([]namedHandler, len(m.handlers))
	copy(handlers, m.handlers)
	m.mu.Unlock()

	// Execute handlers in reverse order (LIFO)
	var wg sync.WaitGroup
	var errorCount int32
	var errorMu sync.Mutex
	var errors []string

	fmt.Fprintf(os.Stderr, "Running %d shutdown handlers...\n", len(handlers))

	for i := len(handlers) - 1; i >= 0; i-- {
		h := handlers[i]
		wg.Add(1)
		go func(handler namedHandler) {
			defer wg.Done()

			start := time.Now()
			err := handler.fn(ctx)
			duration := time.Since(start)

			if err != nil {
				fmt.Fprintf(os.Stderr, "  [FAIL] %s (%v): %v\n", handler.name, duration, err)
				errorMu.Lock()
				errorCount++
				errors = append(errors, fmt.Sprintf("%s: %v", handler.name, err))
				errorMu.Unlock()
			} else {
				fmt.Fprintf(os.Stderr, "  [OK] %s (%v)\n", handler.name, duration)
			}
		}(h)
	}

	// Wait for all handlers or timeout
	doneChan := make(chan struct{})
	go func() {
		wg.Wait()
		close(doneChan)
	}()

	select {
	case <-doneChan:
		fmt.Fprintf(os.Stderr, "Shutdown complete\n")
	case <-ctx.Done():
		fmt.Fprintf(os.Stderr, "Shutdown timed out after %v\n", m.timeout)
	}

	if errorCount > 0 {
		fmt.Fprintf(os.Stderr, "Shutdown completed with %d error(s)\n", errorCount)
	}
}

// WaitForShutdown blocks until shutdown is complete
func (m *ShutdownManager) WaitForShutdown() {
	<-m.done
}

// Convenience functions for global manager

// OnShutdown registers a cleanup handler with the global manager
func OnShutdown(name string, fn ShutdownFunc) {
	Global().Register(name, fn)
}

// OnShutdownSimple registers a simple cleanup function with the global manager
func OnShutdownSimple(name string, fn func()) {
	Global().RegisterSimple(name, fn)
}

// ListenForSignals starts signal listening on the global manager
func ListenForSignals() {
	Global().ListenForSignals()
}

// ShutdownContext returns the global shutdown context
func ShutdownContext() context.Context {
	return Global().Context()
}
