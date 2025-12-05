package store

import (
	"errors"
	"fmt"
)

// Common store errors.
var (
	// ErrNotFound indicates the requested entity does not exist.
	ErrNotFound = errors.New("entity not found")

	// ErrAlreadyExists indicates an entity with the same ID already exists.
	ErrAlreadyExists = errors.New("entity already exists")

	// ErrConnection indicates a connection problem with the backing store.
	ErrConnection = errors.New("store connection error")

	// ErrClosed indicates the store has been closed.
	ErrClosed = errors.New("store is closed")

	// ErrInvalidID indicates the provided ID is invalid.
	ErrInvalidID = errors.New("invalid entity ID")
)

// NotFoundError wraps ErrNotFound with entity details.
type NotFoundError struct {
	Entity string
	ID     string
}

func (e *NotFoundError) Error() string {
	return fmt.Sprintf("%s not found: %s", e.Entity, e.ID)
}

func (e *NotFoundError) Unwrap() error {
	return ErrNotFound
}

// NewNotFoundError creates a typed not found error.
func NewNotFoundError(entity, id string) error {
	return &NotFoundError{Entity: entity, ID: id}
}

// IsNotFound checks if an error is a not found error.
func IsNotFound(err error) bool {
	return errors.Is(err, ErrNotFound)
}

// IsConnection checks if an error is a connection error.
func IsConnection(err error) bool {
	return errors.Is(err, ErrConnection)
}
