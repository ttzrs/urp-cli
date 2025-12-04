// Package logging provides request ID tracing for distributed debugging.
package logging

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"sync"
)

type contextKey string

const requestIDKey contextKey = "request_id"

var (
	// requestIDPool reuses byte slices for ID generation
	requestIDPool = sync.Pool{
		New: func() interface{} {
			return make([]byte, 8)
		},
	}
)

// NewRequestID generates a unique request ID (16 hex chars).
func NewRequestID() string {
	buf := requestIDPool.Get().([]byte)
	defer requestIDPool.Put(buf)

	_, _ = rand.Read(buf)
	return hex.EncodeToString(buf)
}

// WithRequestID adds a request ID to context.
// If id is empty, generates a new one.
func WithRequestID(ctx context.Context, id string) context.Context {
	if id == "" {
		id = NewRequestID()
	}
	return context.WithValue(ctx, requestIDKey, id)
}

// GetRequestID extracts request ID from context.
// Returns empty string if not present.
func GetRequestID(ctx context.Context) string {
	if v := ctx.Value(requestIDKey); v != nil {
		return v.(string)
	}
	return ""
}

// RequestIDFromContext is an alias for GetRequestID.
func RequestIDFromContext(ctx context.Context) string {
	return GetRequestID(ctx)
}
