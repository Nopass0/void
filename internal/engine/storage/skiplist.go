// Package storage – skiplist.go implements a concurrent skip list used as
// the in-memory write buffer (memtable). It supports O(log n) reads and writes
// and allows concurrent readers while a single writer holds the lock.
package storage

import (
	"math/rand"
	"sync"
	"sync/atomic"
)

const (
	// maxLevel is the max height of a skip list node.
	maxLevel = 24
	// probability used when choosing a new node's level.
	skipProbability = 0.25
)

// skipNode is a single node in the skip list.
type skipNode struct {
	key     []byte
	value   []byte
	deleted bool
	// next is an array of atomic pointers to the next node at each level.
	next [maxLevel]atomic.Pointer[skipNode]
}

// SkipList is a concurrent, sorted key-value store backed by a skip list.
// It is used as the memtable in VoidDB's LSM tree.
// Only one goroutine may write at a time (protected by mu).
// Multiple goroutines may read concurrently.
type SkipList struct {
	mu      sync.Mutex
	head    *skipNode
	level   int
	count   int64  // atomic
	memSize int64  // approximate byte size, atomic
}

// NewSkipList creates an empty skip list.
func NewSkipList() *SkipList {
	head := &skipNode{key: nil}
	return &SkipList{head: head, level: 1}
}

// Put inserts or updates the value for key.
// A tombstone can be set by passing deleted=true (value is ignored).
func (sl *SkipList) Put(key, value []byte, deleted bool) {
	sl.mu.Lock()
	defer sl.mu.Unlock()

	update := [maxLevel]*skipNode{}
	current := sl.head

	for i := sl.level - 1; i >= 0; i-- {
		for {
			next := current.next[i].Load()
			if next == nil || string(next.key) >= string(key) {
				break
			}
			current = next
		}
		update[i] = current
	}

	// Check if key already exists at level 0.
	next := current.next[0].Load()
	if next != nil && string(next.key) == string(key) {
		// Update in place.
		next.value = append([]byte(nil), value...)
		next.deleted = deleted
		return
	}

	// Generate level for new node.
	newLevel := sl.randomLevel()
	if newLevel > sl.level {
		for i := sl.level; i < newLevel; i++ {
			update[i] = sl.head
		}
		sl.level = newLevel
	}

	newNode := &skipNode{
		key:     append([]byte(nil), key...),
		value:   append([]byte(nil), value...),
		deleted: deleted,
	}
	for i := 0; i < newLevel; i++ {
		newNode.next[i].Store(update[i].next[i].Load())
		update[i].next[i].Store(newNode)
	}
	atomic.AddInt64(&sl.count, 1)
	atomic.AddInt64(&sl.memSize, int64(len(key)+len(value)+64))
}

// Get retrieves the value for key.
// Returns (value, found, deleted).
func (sl *SkipList) Get(key []byte) ([]byte, bool, bool) {
	current := sl.head
	for i := sl.level - 1; i >= 0; i-- {
		for {
			next := current.next[i].Load()
			if next == nil || string(next.key) >= string(key) {
				break
			}
			current = next
		}
	}
	next := current.next[0].Load()
	if next != nil && string(next.key) == string(key) {
		return next.value, true, next.deleted
	}
	return nil, false, false
}

// Delete marks key as deleted (tombstone).
func (sl *SkipList) Delete(key []byte) {
	sl.Put(key, nil, true)
}

// Scan calls fn for every key in [start, end).
// If start is nil the scan begins at the first key.
// If end is nil the scan runs to the last key.
// Returning false from fn stops the iteration.
func (sl *SkipList) Scan(start, end []byte, fn func(key, value []byte, deleted bool) bool) {
	current := sl.head
	if start != nil {
		// Fast-forward to start.
		for i := sl.level - 1; i >= 0; i-- {
			for {
				next := current.next[i].Load()
				if next == nil || string(next.key) >= string(start) {
					break
				}
				current = next
			}
		}
	}
	node := current.next[0].Load()
	for node != nil {
		if end != nil && string(node.key) >= string(end) {
			break
		}
		if !fn(node.key, node.value, node.deleted) {
			return
		}
		node = node.next[0].Load()
	}
}

// Count returns the number of entries (including tombstones).
func (sl *SkipList) Count() int64 { return atomic.LoadInt64(&sl.count) }

// MemSize returns the approximate memory used in bytes.
func (sl *SkipList) MemSize() int64 { return atomic.LoadInt64(&sl.memSize) }

// All returns all entries in sorted order (used during flush to disk).
func (sl *SkipList) All() []SegmentEntry {
	result := make([]SegmentEntry, 0, sl.Count())
	node := sl.head.next[0].Load()
	for node != nil {
		result = append(result, SegmentEntry{
			Key:     append([]byte(nil), node.key...),
			Value:   append([]byte(nil), node.value...),
			Deleted: node.deleted,
		})
		node = node.next[0].Load()
	}
	return result
}

// randomLevel returns a random height for a new node using geometric distribution.
func (sl *SkipList) randomLevel() int {
	level := 1
	for level < maxLevel && rand.Float64() < skipProbability {
		level++
	}
	return level
}
