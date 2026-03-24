// Package kvcache provides an in-memory Redis-like key-value store with TTLs.
package kvcache

import (
	"sync"
	"time"
)

type item struct {
	Value      []byte
	Expiration int64 // unix nano
}

// Cache is a simple concurrent-safe TTL cache.
type Cache struct {
	mu    sync.RWMutex
	items map[string]item
	done  chan struct{}
}

// New creates a new Cache and starts the cleanup goroutine.
func New() *Cache {
	c := &Cache{
		items: make(map[string]item),
		done:  make(chan struct{}),
	}
	go c.cleanupLoop()
	return c
}

// Set adds a value to the cache with an optional TTL.
func (c *Cache) Set(key string, value []byte, ttl time.Duration) {
	var exp int64
	if ttl > 0 {
		exp = time.Now().Add(ttl).UnixNano()
	}
	c.mu.Lock()
	c.items[key] = item{Value: value, Expiration: exp}
	c.mu.Unlock()
}

// Get retrieves a value from the cache. Returns nil if not found or expired.
func (c *Cache) Get(key string) []byte {
	c.mu.RLock()
	it, found := c.items[key]
	c.mu.RUnlock()
	
	if !found {
		return nil
	}
	if it.Expiration > 0 && time.Now().UnixNano() > it.Expiration {
		c.mu.Lock()
		delete(c.items, key)
		c.mu.Unlock()
		return nil
	}
	return it.Value
}

// Delete removes a key from the cache.
func (c *Cache) Delete(key string) {
	c.mu.Lock()
	delete(c.items, key)
	c.mu.Unlock()
}

// Close stops the cleanup goroutine.
func (c *Cache) Close() {
	close(c.done)
}

func (c *Cache) cleanupLoop() {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-c.done:
			return
		case now := <-ticker.C:
			nowNano := now.UnixNano()
			c.mu.Lock()
			for k, v := range c.items {
				if v.Expiration > 0 && nowNano > v.Expiration {
					delete(c.items, k)
				}
			}
			c.mu.Unlock()
		}
	}
}
