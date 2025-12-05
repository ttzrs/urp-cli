// Package domain defines core entities for URP.
// These are the D (Domain) primitives from PRU theory.
package domain

import "time"

// EntityType represents the type of code entity.
type EntityType string

const (
	EntityFile      EntityType = "File"
	EntityFunction  EntityType = "Function"
	EntityClass     EntityType = "Class"
	EntityStruct    EntityType = "Struct"
	EntityInterface EntityType = "Interface"
	EntityMethod    EntityType = "Method"
)

// entityMeta provides metadata for entity types (OCP - extend via map, not switch).
var entityMeta = map[EntityType]struct {
	Label   string
	StatKey string
}{
	EntityFile:      {"File", "files"},
	EntityFunction:  {"Function", "functions"},
	EntityMethod:    {"Method", "functions"}, // Methods count as functions
	EntityStruct:    {"Struct", "structs"},
	EntityInterface: {"Interface", "interfaces"},
	EntityClass:     {"Class", "classes"},
}

// GraphLabel returns the Cypher node label for this entity type.
func (e EntityType) GraphLabel() string {
	if m, ok := entityMeta[e]; ok {
		return m.Label
	}
	return "Entity"
}

// StatKey returns the stats counter key for this entity type.
func (e EntityType) StatKey() string {
	if m, ok := entityMeta[e]; ok {
		return m.StatKey
	}
	return "other"
}

// Entity represents a code entity in the graph (D primitive).
type Entity struct {
	ID        string            `json:"id"`
	Type      EntityType        `json:"type"`
	Name      string            `json:"name"`
	Path      string            `json:"path"`
	Signature string            `json:"signature,omitempty"`
	StartLine int               `json:"start_line,omitempty"`
	EndLine   int               `json:"end_line,omitempty"`
	Metadata  map[string]string `json:"metadata,omitempty"`
}

// Relationship represents an edge in the graph (Φ primitive).
type Relationship struct {
	From string `json:"from"`
	To   string `json:"to"`
	Type string `json:"type"` // CALLS, CONTAINS, IMPORTS, etc.
}

// Commit represents a git commit (τ primitive).
type Commit struct {
	Hash      string    `json:"hash"`
	Author    string    `json:"author"`
	Email     string    `json:"email"`
	Message   string    `json:"message"`
	Timestamp time.Time `json:"timestamp"`
	Files     []string  `json:"files,omitempty"`
}

// Author represents a code author.
type Author struct {
	Name       string `json:"name"`
	Email      string `json:"email"`
	Commits    int    `json:"commits"`
	LinesAdded int    `json:"lines_added"`
}
