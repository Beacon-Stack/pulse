package scraper

import (
	"sync"
	"time"
)

const defaultCacheTTL = 5 * time.Minute

type cacheEntry struct {
	results []SearchResult
	created time.Time
}

// ResultCache is a simple in-memory cache for search results.
type ResultCache struct {
	mu      sync.RWMutex
	entries map[string]cacheEntry
	ttl     time.Duration
}

// NewResultCache creates a result cache with the given TTL.
func NewResultCache(ttl time.Duration) *ResultCache {
	if ttl == 0 {
		ttl = defaultCacheTTL
	}
	return &ResultCache{
		entries: make(map[string]cacheEntry),
		ttl:     ttl,
	}
}

// Get returns cached results if they exist and are not expired.
func (c *ResultCache) Get(key string) ([]SearchResult, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	e, ok := c.entries[key]
	if !ok || time.Since(e.created) > c.ttl {
		return nil, false
	}
	return e.results, true
}

// Set stores results in the cache.
func (c *ResultCache) Set(key string, results []SearchResult) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.entries[key] = cacheEntry{results: results, created: time.Now()}
}

// Cleanup removes expired entries. Call periodically.
func (c *ResultCache) Cleanup() {
	c.mu.Lock()
	defer c.mu.Unlock()
	for k, e := range c.entries {
		if time.Since(e.created) > c.ttl {
			delete(c.entries, k)
		}
	}
}
