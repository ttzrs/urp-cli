package logging

import (
	"strings"
	"testing"
)

func TestRecoveryHandler_Wrap(t *testing.T) {
	handler := NewRecoveryHandler("test-component")

	// Should not panic
	executed := false
	handler.Wrap(func() {
		executed = true
	})

	if !executed {
		t.Error("function was not executed")
	}
}

func TestRecoveryHandler_WrapPanic(t *testing.T) {
	handler := NewRecoveryHandler("test-component")

	var capturedErr interface{}
	var capturedStack string

	handler.OnPanic = func(err interface{}, stack string) {
		capturedErr = err
		capturedStack = stack
	}

	// Should recover from panic
	handler.Wrap(func() {
		panic("test panic")
	})

	if capturedErr == nil {
		t.Error("panic was not captured")
	}

	if capturedErr != "test panic" {
		t.Errorf("expected 'test panic', got %v", capturedErr)
	}

	if !strings.Contains(capturedStack, "TestRecoveryHandler_WrapPanic") {
		t.Error("stack trace should contain test function name")
	}
}

func TestRecoveryHandler_WrapError(t *testing.T) {
	handler := NewRecoveryHandler("test-component")

	// Should return nil on success
	err := handler.WrapError(func() error {
		return nil
	})

	if err != nil {
		t.Errorf("expected nil, got %v", err)
	}

	// Should return error on panic
	err = handler.WrapError(func() error {
		panic("wrapped panic")
	})

	if err == nil {
		t.Error("expected error from panic")
	}

	if !strings.Contains(err.Error(), "wrapped panic") {
		t.Errorf("error should contain panic message, got: %v", err)
	}
}

func TestSafeGo(t *testing.T) {
	done := make(chan bool, 1)

	// Should not panic even if goroutine panics
	SafeGo("test-goroutine", func() {
		defer func() { done <- true }()
		panic("goroutine panic")
	})

	<-done // Wait for goroutine
}

func TestRecover(t *testing.T) {
	executed := false

	func() {
		defer Recover("test-defer")
		executed = true
		panic("deferred panic")
	}()

	if !executed {
		t.Error("function should have executed before panic")
	}
}

func TestMust(t *testing.T) {
	// Should return value on success
	val := Must(42, nil)
	if val != 42 {
		t.Errorf("expected 42, got %d", val)
	}

	// Should panic on error
	defer func() {
		if r := recover(); r == nil {
			t.Error("Must should panic on error")
		}
	}()

	Must(0, &testError{"test error"})
}

type testError struct {
	msg string
}

func (e *testError) Error() string {
	return e.msg
}
