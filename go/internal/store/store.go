// Package store provides common interfaces for data persistence.
// All stores in URP should implement these interfaces for consistency.
package store

import (
	"context"
)

// Store is the minimal interface all stores must implement.
type Store interface {
	// Ping verifies the connection is alive.
	Ping(ctx context.Context) error
	// Close releases any resources held by the store.
	Close() error
}

// Filter defines query parameters for listing entities.
type Filter struct {
	Limit     int            // Maximum results (0 = no limit)
	Offset    int            // Skip first N results
	OrderBy   string         // Field to sort by
	OrderDesc bool           // Sort descending if true
	Where     map[string]any // Field conditions
}

// DefaultFilter returns a filter with sensible defaults.
func DefaultFilter() Filter {
	return Filter{
		Limit:     100,
		Offset:    0,
		OrderDesc: true,
	}
}

// WithLimit returns a copy of the filter with a new limit.
func (f Filter) WithLimit(n int) Filter {
	f.Limit = n
	return f
}

// WithOffset returns a copy of the filter with a new offset.
func (f Filter) WithOffset(n int) Filter {
	f.Offset = n
	return f
}

// WithOrder returns a copy of the filter with ordering.
func (f Filter) WithOrder(field string, desc bool) Filter {
	f.OrderBy = field
	f.OrderDesc = desc
	return f
}

// WithWhere returns a copy of the filter with an added condition.
func (f Filter) WithWhere(field string, value any) Filter {
	if f.Where == nil {
		f.Where = make(map[string]any)
	}
	f.Where[field] = value
	return f
}

// Reader provides read-only access to entities.
type Reader[T any] interface {
	// Get retrieves an entity by ID.
	Get(ctx context.Context, id string) (*T, error)
	// List retrieves entities matching the filter.
	List(ctx context.Context, filter Filter) ([]*T, error)
	// Count returns the number of entities matching the filter.
	Count(ctx context.Context, filter Filter) (int, error)
}

// Writer provides write access to entities.
type Writer[T any] interface {
	// Create stores a new entity.
	Create(ctx context.Context, entity *T) error
	// Update modifies an existing entity.
	Update(ctx context.Context, entity *T) error
	// Delete removes an entity by ID.
	Delete(ctx context.Context, id string) error
}

// EntityStore combines read and write operations for a single entity type.
// Implements Interface Segregation: consumers can depend on Reader or Writer only.
type EntityStore[T any] interface {
	Store
	Reader[T]
	Writer[T]
}

// QueryStore adds raw query capability for complex operations.
type QueryStore interface {
	Store
	// Execute runs a read query and returns records.
	Execute(ctx context.Context, query string, params map[string]any) ([]Record, error)
	// ExecuteWrite runs a write query.
	ExecuteWrite(ctx context.Context, query string, params map[string]any) error
}

// Record is a generic query result row.
type Record map[string]any

// GetString extracts a string value from a record.
func (r Record) GetString(key string) string {
	if v, ok := r[key].(string); ok {
		return v
	}
	return ""
}

// GetInt extracts an int value from a record.
func (r Record) GetInt(key string) int {
	switch v := r[key].(type) {
	case int:
		return v
	case int64:
		return int(v)
	case float64:
		return int(v)
	}
	return 0
}

// GetFloat extracts a float64 value from a record.
func (r Record) GetFloat(key string) float64 {
	switch v := r[key].(type) {
	case float64:
		return v
	case int:
		return float64(v)
	case int64:
		return float64(v)
	}
	return 0
}

// GetBool extracts a bool value from a record.
func (r Record) GetBool(key string) bool {
	if v, ok := r[key].(bool); ok {
		return v
	}
	return false
}

// Tx represents a transaction scope.
type Tx interface {
	// Commit commits the transaction.
	Commit(ctx context.Context) error
	// Rollback aborts the transaction.
	Rollback(ctx context.Context) error
}

// TxStore adds transaction support to a store.
type TxStore interface {
	Store
	// Begin starts a new transaction.
	Begin(ctx context.Context) (Tx, error)
}
