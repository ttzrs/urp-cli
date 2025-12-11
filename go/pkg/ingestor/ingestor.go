package ingestor

import (
	"context"
)

// KnowledgeChunk represents a piece of theoretical knowledge extracted from a document.
type KnowledgeChunk struct {
	Source      string  // e.g., "VFD_Manual_v2.pdf - Page 40"
	Proposition string  // e.g., "IF LI1=TRUE THEN Motor=RUN"
	Entities    []string // e.g., ["LI1", "Motor"]
	Confidence  float64 // 0.0 to 1.0
}

// Ingestor defines the contract for processing raw documentation.
type Ingestor interface {
	// ProcessDocument reads a file and returns a list of theoretical rules/facts.
	ProcessDocument(ctx context.Context, filePath string) ([]KnowledgeChunk, error)
}
