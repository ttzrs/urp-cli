package graph

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"sync"
	"time"
)

// CacheEntry holds a cached query result.
type CacheEntry struct {
	Records   []Record
	ExpiresAt time.Time
}

// QueryCache provides LRU caching for graph queries.
type QueryCache struct {
	mu      sync.RWMutex
	entries map[string]CacheEntry
	maxSize int
	ttl     time.Duration
	hits    int64
	misses  int64
}

// NewQueryCache creates a cache with given max entries and TTL.
func NewQueryCache(maxSize int, ttl time.Duration) *QueryCache {
	return &QueryCache{
		entries: make(map[string]CacheEntry),
		maxSize: maxSize,
		ttl:     ttl,
	}
}

// cacheKey generates a unique key for query + params.
func cacheKey(query string, params map[string]any) string {
	data, _ := json.Marshal(map[string]any{
		"q": query,
		"p": params,
	})
	hash := sha256.Sum256(data)
	return hex.EncodeToString(hash[:16])
}

// Get retrieves a cached result if valid.
func (c *QueryCache) Get(query string, params map[string]any) ([]Record, bool) {
	key := cacheKey(query, params)

	c.mu.RLock()
	entry, ok := c.entries[key]
	c.mu.RUnlock()

	if !ok {
		c.mu.Lock()
		c.misses++
		c.mu.Unlock()
		return nil, false
	}

	if time.Now().After(entry.ExpiresAt) {
		c.mu.Lock()
		delete(c.entries, key)
		c.misses++
		c.mu.Unlock()
		return nil, false
	}

	c.mu.Lock()
	c.hits++
	c.mu.Unlock()
	return entry.Records, true
}

// Set stores a result in the cache.
func (c *QueryCache) Set(query string, params map[string]any, records []Record) {
	key := cacheKey(query, params)

	c.mu.Lock()
	defer c.mu.Unlock()

	// Simple size management: clear half when full
	if len(c.entries) >= c.maxSize {
		count := 0
		for k := range c.entries {
			delete(c.entries, k)
			count++
			if count >= c.maxSize/2 {
				break
			}
		}
	}

	c.entries[key] = CacheEntry{
		Records:   records,
		ExpiresAt: time.Now().Add(c.ttl),
	}
}

// Clear removes all cached entries.
func (c *QueryCache) Clear() {
	c.mu.Lock()
	c.entries = make(map[string]CacheEntry)
	c.mu.Unlock()
}

// CacheStats holds cache statistics.
type CacheStats struct {
	Size     int     `json:"size"`
	Capacity int     `json:"capacity"`
	Hits     int64   `json:"hits"`
	Misses   int64   `json:"misses"`
	HitRate  float64 `json:"hit_rate"`
}

// Stats returns cache statistics.
func (c *QueryCache) Stats() CacheStats {
	c.mu.RLock()
	defer c.mu.RUnlock()

	total := c.hits + c.misses
	hitRate := 0.0
	if total > 0 {
		hitRate = float64(c.hits) / float64(total)
	}

	return CacheStats{
		Size:     len(c.entries),
		Capacity: c.maxSize,
		Hits:     c.hits,
		Misses:   c.misses,
		HitRate:  hitRate,
	}
}

// CachedDriver wraps a Driver with query caching.
type CachedDriver struct {
	Driver
	cache *QueryCache
}

// NewCachedDriver wraps a driver with caching.
func NewCachedDriver(d Driver, cache *QueryCache) *CachedDriver {
	return &CachedDriver{
		Driver: d,
		cache:  cache,
	}
}

// Execute with cache lookup.
func (d *CachedDriver) Execute(ctx context.Context, query string, params map[string]any) ([]Record, error) {
	// Check cache first
	if records, ok := d.cache.Get(query, params); ok {
		return records, nil
	}

	// Execute query
	records, err := d.Driver.Execute(ctx, query, params)
	if err != nil {
		return nil, err
	}

	// Cache result
	d.cache.Set(query, params, records)
	return records, nil
}

// ExecuteWrite invalidates cache (writes may change data).
func (d *CachedDriver) ExecuteWrite(ctx context.Context, query string, params map[string]any) error {
	// Clear cache on any write
	d.cache.Clear()
	return d.Driver.ExecuteWrite(ctx, query, params)
}

// Cache returns the underlying cache for stats.
func (d *CachedDriver) Cache() *QueryCache {
	return d.cache
}

