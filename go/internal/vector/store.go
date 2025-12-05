// Package vector provides vector database operations.
// Uses JSON + binary format for persistence (LanceDB-compatible design).
package vector

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/joss/urp/internal/config"
)

// VectorEntry represents a stored vector with metadata.
type VectorEntry struct {
	ID        string            `json:"id"`
	Text      string            `json:"text"`
	Vector    []float32         `json:"vector"`
	Kind      string            `json:"kind"` // "error", "code", "solution", "knowledge"
	Metadata  map[string]string `json:"metadata,omitempty"`
	CreatedAt int64             `json:"created_at"`
}

// SearchResult represents a similarity search result.
type SearchResult struct {
	Entry    VectorEntry
	Score    float32 // cosine similarity (0-1)
	Distance float32 // L2 distance
}

// VectorSearcher provides read-only vector search (ISP - Interface Segregation).
type VectorSearcher interface {
	// Search finds similar vectors.
	Search(ctx context.Context, vector []float32, limit int, kind string) ([]SearchResult, error)
}

// VectorWriter provides write operations for vectors.
type VectorWriter interface {
	// Add stores a vector entry.
	Add(ctx context.Context, entry VectorEntry) error

	// Delete removes an entry by ID.
	Delete(ctx context.Context, id string) error
}

// Store defines the full vector store interface.
// Composes VectorSearcher + VectorWriter + lifecycle methods.
type Store interface {
	VectorSearcher
	VectorWriter

	// Count returns total entries.
	Count(ctx context.Context) (int, error)

	// Close closes the store.
	Close() error
}

// LanceStore implements Store with file-based persistence.
// Uses JSON for metadata and binary format for vectors (efficient storage).
type LanceStore struct {
	mu       sync.RWMutex
	entries  map[string]VectorEntry
	dbPath   string
	metaFile string // JSON metadata
	vecFile  string // Binary vectors
	dirty    bool   // Needs persist
}

// NewLanceStore creates a new vector store with file persistence.
func NewLanceStore(dbPath string) (*LanceStore, error) {
	if dbPath == "" {
		dbPath = config.GetPaths().Vectors
	}

	if err := os.MkdirAll(dbPath, 0755); err != nil {
		return nil, fmt.Errorf("create vector dir: %w", err)
	}

	store := &LanceStore{
		entries:  make(map[string]VectorEntry),
		dbPath:   dbPath,
		metaFile: filepath.Join(dbPath, "index.json"),
		vecFile:  filepath.Join(dbPath, "vectors.bin"),
	}

	// Load existing entries
	if err := store.load(); err != nil {
		// Ignore load errors, start fresh
	}

	return store, nil
}

// Add stores a vector entry.
func (s *LanceStore) Add(ctx context.Context, entry VectorEntry) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if entry.ID == "" {
		entry.ID = generateID(entry.Text)
	}
	if entry.CreatedAt == 0 {
		entry.CreatedAt = time.Now().Unix()
	}

	s.entries[entry.ID] = entry
	s.dirty = true
	return s.persist()
}

// Search finds similar vectors using cosine similarity.
func (s *LanceStore) Search(ctx context.Context, vector []float32, limit int, kind string) ([]SearchResult, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var results []SearchResult

	for _, entry := range s.entries {
		// Filter by kind if specified
		if kind != "" && entry.Kind != kind {
			continue
		}

		// Skip entries without vectors
		if len(entry.Vector) == 0 {
			continue
		}

		score := cosineSimilarity(vector, entry.Vector)
		results = append(results, SearchResult{
			Entry:    entry,
			Score:    score,
			Distance: 1 - score,
		})
	}

	// Sort by score descending
	sortByScore(results)

	// Limit results
	if limit > 0 && len(results) > limit {
		results = results[:limit]
	}

	return results, nil
}

// Delete removes an entry by ID.
func (s *LanceStore) Delete(ctx context.Context, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.entries[id]; exists {
		delete(s.entries, id)
		s.dirty = true
		return s.persist()
	}
	return nil
}

// Count returns total entries.
func (s *LanceStore) Count(ctx context.Context) (int, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.entries), nil
}

// Close closes the store.
func (s *LanceStore) Close() error {
	return s.persist()
}

// cosineSimilarity calculates cosine similarity between two vectors.
func cosineSimilarity(a, b []float32) float32 {
	if len(a) != len(b) || len(a) == 0 {
		return 0
	}

	var dotProduct, normA, normB float32
	for i := range a {
		dotProduct += a[i] * b[i]
		normA += a[i] * a[i]
		normB += b[i] * b[i]
	}

	if normA == 0 || normB == 0 {
		return 0
	}

	return dotProduct / (sqrt(normA) * sqrt(normB))
}

// sqrt is a simple float32 square root.
func sqrt(x float32) float32 {
	if x <= 0 {
		return 0
	}
	// Newton's method
	z := x / 2
	for i := 0; i < 10; i++ {
		z = z - (z*z-x)/(2*z)
	}
	return z
}

// sortByScore sorts results by score descending.
func sortByScore(results []SearchResult) {
	for i := 0; i < len(results)-1; i++ {
		for j := i + 1; j < len(results); j++ {
			if results[j].Score > results[i].Score {
				results[i], results[j] = results[j], results[i]
			}
		}
	}
}

// generateID creates a unique ID from text.
func generateID(text string) string {
	h := sha256.Sum256([]byte(text))
	return hex.EncodeToString(h[:8])
}

// metaEntry is the JSON-serializable part of VectorEntry (without vector).
type metaEntry struct {
	ID        string            `json:"id"`
	Text      string            `json:"text"`
	Kind      string            `json:"kind"`
	Metadata  map[string]string `json:"metadata,omitempty"`
	CreatedAt int64             `json:"created_at"`
	VecDims   int               `json:"vec_dims"`
	VecOffset int64             `json:"vec_offset"` // Offset in binary file
}

// persist saves entries to disk using JSON + binary format.
// Metadata goes to index.json, vectors go to vectors.bin (more efficient).
func (s *LanceStore) persist() error {
	if !s.dirty {
		return nil
	}

	// Build metadata and binary vectors
	var metas []metaEntry
	var vectors []byte

	for _, entry := range s.entries {
		offset := int64(len(vectors))
		dims := len(entry.Vector)

		metas = append(metas, metaEntry{
			ID:        entry.ID,
			Text:      entry.Text,
			Kind:      entry.Kind,
			Metadata:  entry.Metadata,
			CreatedAt: entry.CreatedAt,
			VecDims:   dims,
			VecOffset: offset,
		})

		// Append vector as binary (little-endian float32)
		for _, v := range entry.Vector {
			buf := make([]byte, 4)
			binary.LittleEndian.PutUint32(buf, floatBits(v))
			vectors = append(vectors, buf...)
		}
	}

	// Write metadata JSON
	metaData, err := json.MarshalIndent(metas, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal metadata: %w", err)
	}
	if err := os.WriteFile(s.metaFile, metaData, 0644); err != nil {
		return fmt.Errorf("write metadata: %w", err)
	}

	// Write binary vectors
	if err := os.WriteFile(s.vecFile, vectors, 0644); err != nil {
		return fmt.Errorf("write vectors: %w", err)
	}

	s.dirty = false
	return nil
}

// load loads entries from disk.
func (s *LanceStore) load() error {
	// Read metadata
	metaData, err := os.ReadFile(s.metaFile)
	if err != nil {
		if os.IsNotExist(err) {
			return nil // No data yet
		}
		return fmt.Errorf("read metadata: %w", err)
	}

	var metas []metaEntry
	if err := json.Unmarshal(metaData, &metas); err != nil {
		return fmt.Errorf("unmarshal metadata: %w", err)
	}

	// Read vectors
	vecFile, err := os.Open(s.vecFile)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("open vectors: %w", err)
	}
	defer vecFile.Close()

	// Reconstruct entries
	for _, meta := range metas {
		// Seek to vector offset
		if _, err := vecFile.Seek(meta.VecOffset, io.SeekStart); err != nil {
			continue
		}

		// Read vector
		vector := make([]float32, meta.VecDims)
		for i := 0; i < meta.VecDims; i++ {
			var buf [4]byte
			if _, err := io.ReadFull(vecFile, buf[:]); err != nil {
				break
			}
			vector[i] = floatFromBits(binary.LittleEndian.Uint32(buf[:]))
		}

		s.entries[meta.ID] = VectorEntry{
			ID:        meta.ID,
			Text:      meta.Text,
			Vector:    vector,
			Kind:      meta.Kind,
			Metadata:  meta.Metadata,
			CreatedAt: meta.CreatedAt,
		}
	}

	return nil
}

// floatBits converts float32 to uint32 bits using math package.
func floatBits(f float32) uint32 {
	return math.Float32bits(f)
}

// floatFromBits converts uint32 bits to float32 using math package.
func floatFromBits(b uint32) float32 {
	return math.Float32frombits(b)
}

// DefaultStore is the global vector store instance.
var (
	defaultStore     Store
	defaultStoreOnce sync.Once
)

// SetDefaultStore sets the default vector store.
func SetDefaultStore(s Store) {
	defaultStore = s
	// Ensure subsequent calls to Default() don't overwrite it
	defaultStoreOnce.Do(func() {})
}

// Default returns the default vector store.
func Default() Store {
	defaultStoreOnce.Do(func() {
		store, err := NewLanceStore("")
		if err != nil {
			// Fallback to in-memory only
			store = &LanceStore{entries: make(map[string]VectorEntry)}
		}
		defaultStore = store
	})
	return defaultStore
}
