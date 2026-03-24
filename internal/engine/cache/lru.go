// Package cache provides the LRU block cache used by VoidDB to keep hot
// SSTable blocks in memory, avoiding repeated disk reads.
package cache

import (
	"container/list"
	"sync"
)

// entry is stored in both the doubly-linked list and the lookup map.
type entry struct {
	key   string
	value []byte
	size  int
}

// Cache is a thread-safe LRU cache with a byte-size budget.
type Cache struct {
	mu       sync.Mutex
	maxBytes int
	usedBytes int
	list     *list.List
	items    map[string]*list.Element
}

// New creates a Cache whose total byte usage will not exceed maxBytes.
func New(maxBytes int) *Cache {
	return &Cache{
		maxBytes: maxBytes,
		list:     list.New(),
		items:    make(map[string]*list.Element),
	}
}

// Get retrieves the value for key.  Returns nil if not found.
func (c *Cache) Get(key string) []byte {
	c.mu.Lock()
	defer c.mu.Unlock()

	el, ok := c.items[key]
	if !ok {
		return nil
	}
	c.list.MoveToFront(el)
	return el.Value.(*entry).value
}

// Set inserts or replaces key→value.  If the value is larger than maxBytes the
// cache is not modified (the item would immediately be evicted).
func (c *Cache) Set(key string, value []byte) {
	size := len(key) + len(value) + 64 // ~64 B overhead per entry
	if size > c.maxBytes {
		return
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	if el, ok := c.items[key]; ok {
		c.list.MoveToFront(el)
		old := el.Value.(*entry)
		c.usedBytes -= old.size
		old.value = value
		old.size = size
		c.usedBytes += size
		return
	}

	el := c.list.PushFront(&entry{key: key, value: value, size: size})
	c.items[key] = el
	c.usedBytes += size
	c.evict()
}

// Delete removes an entry from the cache.
func (c *Cache) Delete(key string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if el, ok := c.items[key]; ok {
		c.removeElement(el)
	}
}

// Len returns the current number of cached entries.
func (c *Cache) Len() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.items)
}

// UsedBytes returns current total memory usage.
func (c *Cache) UsedBytes() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.usedBytes
}

// Purge empties the cache entirely.
func (c *Cache) Purge() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.list.Init()
	c.items = make(map[string]*list.Element)
	c.usedBytes = 0
}

// evict removes least-recently-used entries until usedBytes ≤ maxBytes.
// Must be called with mu held.
func (c *Cache) evict() {
	for c.usedBytes > c.maxBytes && c.list.Len() > 0 {
		c.removeElement(c.list.Back())
	}
}

// removeElement unlinks el from the list and map.
// Must be called with mu held.
func (c *Cache) removeElement(el *list.Element) {
	c.list.Remove(el)
	e := el.Value.(*entry)
	delete(c.items, e.key)
	c.usedBytes -= e.size
}
