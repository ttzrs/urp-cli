package store

import (
	"errors"
	"testing"
)

func TestFilter_WithMethods(t *testing.T) {
	f := DefaultFilter()

	if f.Limit != 100 {
		t.Errorf("DefaultFilter().Limit = %d, want 100", f.Limit)
	}

	f2 := f.WithLimit(50).WithOffset(10).WithOrder("created_at", true)
	if f2.Limit != 50 {
		t.Errorf("WithLimit(50).Limit = %d, want 50", f2.Limit)
	}
	if f2.Offset != 10 {
		t.Errorf("WithOffset(10).Offset = %d, want 10", f2.Offset)
	}
	if f2.OrderBy != "created_at" {
		t.Errorf("WithOrder().OrderBy = %q, want %q", f2.OrderBy, "created_at")
	}
	if !f2.OrderDesc {
		t.Error("WithOrder(_, true).OrderDesc = false, want true")
	}

	// Original should be unchanged (immutable)
	if f.Limit != 100 {
		t.Error("original filter was mutated")
	}
}

func TestFilter_WithWhere(t *testing.T) {
	f := DefaultFilter().WithWhere("status", "active").WithWhere("type", "error")

	if f.Where["status"] != "active" {
		t.Errorf("Where[status] = %v, want 'active'", f.Where["status"])
	}
	if f.Where["type"] != "error" {
		t.Errorf("Where[type] = %v, want 'error'", f.Where["type"])
	}
}

func TestRecord_GetMethods(t *testing.T) {
	r := Record{
		"name":    "test",
		"count":   int64(42),
		"rate":    3.14,
		"enabled": true,
	}

	if got := r.GetString("name"); got != "test" {
		t.Errorf("GetString(name) = %q, want %q", got, "test")
	}
	if got := r.GetString("missing"); got != "" {
		t.Errorf("GetString(missing) = %q, want empty", got)
	}

	if got := r.GetInt("count"); got != 42 {
		t.Errorf("GetInt(count) = %d, want 42", got)
	}
	if got := r.GetInt("missing"); got != 0 {
		t.Errorf("GetInt(missing) = %d, want 0", got)
	}

	if got := r.GetFloat("rate"); got != 3.14 {
		t.Errorf("GetFloat(rate) = %f, want 3.14", got)
	}

	if got := r.GetBool("enabled"); !got {
		t.Error("GetBool(enabled) = false, want true")
	}
	if got := r.GetBool("missing"); got {
		t.Error("GetBool(missing) = true, want false")
	}
}

func TestErrors(t *testing.T) {
	t.Run("NotFoundError", func(t *testing.T) {
		err := NewNotFoundError("Session", "abc123")
		if !IsNotFound(err) {
			t.Error("IsNotFound should return true")
		}
		if !errors.Is(err, ErrNotFound) {
			t.Error("should wrap ErrNotFound")
		}

		nfe := &NotFoundError{}
		if !errors.As(err, &nfe) {
			t.Fatal("should be NotFoundError")
		}
		if nfe.Entity != "Session" || nfe.ID != "abc123" {
			t.Error("wrong entity/id in error")
		}
	})

	t.Run("IsConnection", func(t *testing.T) {
		if IsConnection(nil) {
			t.Error("IsConnection(nil) should be false")
		}
		if !IsConnection(ErrConnection) {
			t.Error("IsConnection(ErrConnection) should be true")
		}
	})
}
