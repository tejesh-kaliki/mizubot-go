package anilist

import (
	"strings"
	"sync"
	"time"
)

// TTLCache is a tiny concurrency-safe in-memory cache whose entries expire
// after a fixed duration. Expired entries are dropped lazily on access.
type TTLCache[V any] struct {
	ttl time.Duration
	now func() time.Time

	mu      sync.Mutex
	entries map[string]cacheEntry[V]
}

type cacheEntry[V any] struct {
	value   V
	expires time.Time
}

func NewTTLCache[V any](ttl time.Duration) *TTLCache[V] {
	return &TTLCache[V]{ttl: ttl, now: time.Now, entries: make(map[string]cacheEntry[V])}
}

func (c *TTLCache[V]) Get(key string) (V, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	entry, ok := c.entries[key]
	if !ok {
		var zero V
		return zero, false
	}
	if !c.now().Before(entry.expires) {
		delete(c.entries, key)
		var zero V
		return zero, false
	}
	return entry.value, true
}

func (c *TTLCache[V]) Set(key string, value V) {
	c.mu.Lock()
	defer c.mu.Unlock()
	now := c.now()
	// Sweep occasionally so a long-running bot doesn't accumulate keys.
	if len(c.entries) > 256 {
		for k, e := range c.entries {
			if !now.Before(e.expires) {
				delete(c.entries, k)
			}
		}
	}
	c.entries[key] = cacheEntry[V]{value: value, expires: now.Add(c.ttl)}
}

// NormalizeKey lowercases and collapses whitespace so "Attack  on Titan"
// and "attack on titan" share a cache entry.
func NormalizeKey(s string) string {
	return strings.ToLower(strings.Join(strings.Fields(s), " "))
}
