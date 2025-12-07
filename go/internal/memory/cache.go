// Package memory provides knowledge store management.
package memory

import (
	"crypto/sha256"
	"encoding/hex"
	"sync"
	"sync/atomic"
	"time"
)

// CacheStats holds cache performance metrics.
type CacheStats struct {
	Hits      int64   `json:"hits"`
	Misses    int64   `json:"misses"`
	HitRate   float64 `json:"hit_rate"`
	Size      int     `json:"size"`
	MaxSize   int     `json:"max_size"`
	Evictions int64   `json:"evictions"`
}

// cacheEntry wraps cached data with TTL.
type cacheEntry struct {
	results   []KnowledgeEntry
	createdAt time.Time
}

// QueryCache provides LRU caching for knowledge queries.
type QueryCache struct {
	mu        sync.RWMutex
	entries   map[string]*cacheEntry
	order     []string // LRU order: oldest first
	maxSize   int
	ttl       time.Duration
	hits      atomic.Int64
	misses    atomic.Int64
	evictions atomic.Int64
}

// NewQueryCache creates a new query cache.
func NewQueryCache(maxSize int, ttl time.Duration) *QueryCache {
	if maxSize <= 0 {
		maxSize = 1000 // Default
	}
	if ttl <= 0 {
		ttl = 5 * time.Minute // Default TTL
	}
	return &QueryCache{
		entries: make(map[string]*cacheEntry, maxSize),
		order:   make([]string, 0, maxSize),
		maxSize: maxSize,
		ttl:     ttl,
	}
}

// hashQuery creates a cache key from query parameters.
func hashQuery(queryText, level, kind string, limit int) string {
	h := sha256.New()
	h.Write([]byte(queryText))
	h.Write([]byte("|"))
	h.Write([]byte(level))
	h.Write([]byte("|"))
	h.Write([]byte(kind))
	h.Write([]byte("|"))
	h.Write([]byte{byte(limit)})
	return hex.EncodeToString(h.Sum(nil))[:16] // First 16 chars
}

// Get retrieves cached results if valid.
func (c *QueryCache) Get(queryText, level, kind string, limit int) ([]KnowledgeEntry, bool) {
	key := hashQuery(queryText, level, kind, limit)

	c.mu.RLock()
	entry, exists := c.entries[key]
	c.mu.RUnlock()

	if !exists {
		c.misses.Add(1)
		return nil, false
	}

	// Check TTL
	if time.Since(entry.createdAt) > c.ttl {
		c.misses.Add(1)
		// Don't delete here - let Set handle eviction
		return nil, false
	}

	c.hits.Add(1)

	// Move to end of LRU (most recently used)
	c.mu.Lock()
	c.moveToEnd(key)
	c.mu.Unlock()

	// Return copy to avoid mutation
	result := make([]KnowledgeEntry, len(entry.results))
	copy(result, entry.results)
	return result, true
}

// Set stores results in cache, evicting old entries if needed.
func (c *QueryCache) Set(queryText, level, kind string, limit int, results []KnowledgeEntry) {
	key := hashQuery(queryText, level, kind, limit)

	c.mu.Lock()
	defer c.mu.Unlock()

	// Evict expired entries first
	c.evictExpired()

	// Evict LRU if at capacity
	for len(c.entries) >= c.maxSize {
		c.evictOldest()
	}

	// Store copy
	resultsCopy := make([]KnowledgeEntry, len(results))
	copy(resultsCopy, results)

	c.entries[key] = &cacheEntry{
		results:   resultsCopy,
		createdAt: time.Now(),
	}
	c.order = append(c.order, key)
}

// Invalidate removes entries matching a pattern.
func (c *QueryCache) Invalidate(pattern string) int {
	c.mu.Lock()
	defer c.mu.Unlock()

	count := 0
	for key := range c.entries {
		// Simple prefix match for now
		if pattern == "" || pattern == "*" {
			delete(c.entries, key)
			count++
		}
	}

	if pattern == "" || pattern == "*" {
		c.order = c.order[:0]
	}

	return count
}

// InvalidateAll clears the entire cache.
func (c *QueryCache) InvalidateAll() {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.entries = make(map[string]*cacheEntry, c.maxSize)
	c.order = c.order[:0]
}

// Stats returns cache performance metrics.
func (c *QueryCache) Stats() CacheStats {
	c.mu.RLock()
	size := len(c.entries)
	c.mu.RUnlock()

	hits := c.hits.Load()
	misses := c.misses.Load()
	total := hits + misses

	hitRate := 0.0
	if total > 0 {
		hitRate = float64(hits) / float64(total)
	}

	return CacheStats{
		Hits:      hits,
		Misses:    misses,
		HitRate:   hitRate,
		Size:      size,
		MaxSize:   c.maxSize,
		Evictions: c.evictions.Load(),
	}
}

// moveToEnd moves a key to the end of the LRU order (most recently used).
// Caller must hold write lock.
func (c *QueryCache) moveToEnd(key string) {
	for i, k := range c.order {
		if k == key {
			// Remove from current position
			c.order = append(c.order[:i], c.order[i+1:]...)
			// Add to end
			c.order = append(c.order, key)
			return
		}
	}
}

// evictOldest removes the least recently used entry.
// Caller must hold write lock.
func (c *QueryCache) evictOldest() {
	if len(c.order) == 0 {
		return
	}

	oldestKey := c.order[0]
	c.order = c.order[1:]
	delete(c.entries, oldestKey)
	c.evictions.Add(1)
}

// evictExpired removes all expired entries.
// Caller must hold write lock.
func (c *QueryCache) evictExpired() {
	now := time.Now()
	keysToRemove := make([]string, 0)

	for key, entry := range c.entries {
		if now.Sub(entry.createdAt) > c.ttl {
			keysToRemove = append(keysToRemove, key)
		}
	}

	for _, key := range keysToRemove {
		delete(c.entries, key)
		c.evictions.Add(1)
	}

	// Rebuild order without expired keys
	if len(keysToRemove) > 0 {
		expired := make(map[string]bool, len(keysToRemove))
		for _, k := range keysToRemove {
			expired[k] = true
		}

		newOrder := make([]string, 0, len(c.order))
		for _, k := range c.order {
			if !expired[k] {
				newOrder = append(newOrder, k)
			}
		}
		c.order = newOrder
	}
}
