// Package engine is the core of VoidDB.  It wires together the WAL, memtable
// (skip list), SSTable segments, LRU cache, and background compaction into a
// single LSM-tree storage engine.
//
// Write path:
//
//	Put(key, val) → WAL.Append → memtable.Put
//	               (when memtable > threshold) → flush goroutine → segment file
//
// Read path:
//
//	Get(key) → memtable.Get (hit?)
//	         → for each segment newest-first: bloom.Test + segment.Get
package engine

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/voiddb/void/internal/engine/cache"
	"github.com/voiddb/void/internal/engine/storage"
	"github.com/voiddb/void/internal/engine/wal"
)

// Options configure the Engine.
type Options struct {
	// DataDir is where segment files are written.
	DataDir string
	// WALDir is where the WAL file lives (defaults to DataDir/wal).
	WALDir string
	// MemTableSize is the threshold in bytes before a flush is triggered.
	MemTableSize int64
	// BlockCacheSize is the LRU cache capacity in bytes.
	BlockCacheSize int64
	// BloomFPRate is the Bloom filter false-positive rate (0 < p < 1).
	BloomFPRate float64
	// CompactionWorkers is the number of background compaction goroutines.
	CompactionWorkers int
	// SyncWAL forces fsync on every WAL append.
	SyncWAL bool
	// MaxLevels for LSM compaction.
	MaxLevels int
	// LevelSizeMultiplier controls level size growth.
	LevelSizeMultiplier int
}

// DefaultOptions returns sensible defaults for a development environment.
func DefaultOptions() Options {
	return Options{
		DataDir:             "./data",
		MemTableSize:        64 * 1024 * 1024,
		BlockCacheSize:      256 * 1024 * 1024,
		BloomFPRate:         0.01,
		CompactionWorkers:   2,
		SyncWAL:             false,
		MaxLevels:           7,
		LevelSizeMultiplier: 10,
	}
}

// Engine is the top-level VoidDB storage engine.
// It is safe for concurrent use by multiple goroutines.
type Engine struct {
	opts     Options
	mu       sync.RWMutex
	memtable *storage.SkipList
	// imm is the immutable memtable being flushed.
	imm      *storage.SkipList
	segments []*storage.Segment // sorted newest-first
	wal      *wal.WAL
	cache    *cache.Cache
	seqNum   atomic.Uint64

	flushCh    chan struct{}
	closeCh    chan struct{}
	wg         sync.WaitGroup
	closed     atomic.Bool
	compacting atomic.Bool
}

// Open initialises the Engine, replaying the WAL if the process was previously
// interrupted.
func Open(opts Options) (*Engine, error) {
	if err := os.MkdirAll(opts.DataDir, 0755); err != nil {
		return nil, fmt.Errorf("engine: mkdir data: %w", err)
	}
	walDir := opts.WALDir
	if walDir == "" {
		walDir = filepath.Join(opts.DataDir, "wal")
	}
	if err := os.MkdirAll(walDir, 0755); err != nil {
		return nil, fmt.Errorf("engine: mkdir wal: %w", err)
	}

	w, err := wal.Open(filepath.Join(walDir, "void.wal"), opts.SyncWAL)
	if err != nil {
		return nil, fmt.Errorf("engine: open wal: %w", err)
	}

	e := &Engine{
		opts:     opts,
		memtable: storage.NewSkipList(),
		wal:      w,
		cache:    cache.New(int(opts.BlockCacheSize)),
		flushCh:  make(chan struct{}, 1),
		closeCh:  make(chan struct{}),
	}

	// Load existing segments from disk.
	maxSegmentSeq, err := e.loadSegments()
	if err != nil {
		return nil, fmt.Errorf("engine: load segments: %w", err)
	}

	// Replay WAL to rebuild the memtable.
	if err := e.replayWAL(); err != nil {
		return nil, fmt.Errorf("engine: replay wal: %w", err)
	}
	if walSeq := e.wal.SeqNum(); walSeq > maxSegmentSeq {
		maxSegmentSeq = walSeq
	}
	e.seqNum.Store(maxSegmentSeq)

	// Start background goroutines.
	e.wg.Add(1)
	go e.flushLoop()

	for i := 0; i < opts.CompactionWorkers; i++ {
		e.wg.Add(1)
		go e.compactionLoop()
	}

	return e, nil
}

// Put inserts or updates the value for key within the given namespace (collection).
func (e *Engine) Put(namespace string, key, value []byte) error {
	if e.closed.Load() {
		return fmt.Errorf("engine: closed")
	}
	fullKey := makeKey(namespace, key)

	if _, err := e.wal.Append(wal.OpPut, fullKey, value); err != nil {
		return fmt.Errorf("engine: wal append: %w", err)
	}

	e.mu.Lock()
	e.memtable.Put(fullKey, value, false)
	memSize := e.memtable.MemSize()
	e.mu.Unlock()

	if memSize >= e.opts.MemTableSize {
		select {
		case e.flushCh <- struct{}{}:
		default:
		}
	}
	return nil
}

// Delete marks key as deleted (tombstone).
func (e *Engine) Delete(namespace string, key []byte) error {
	if e.closed.Load() {
		return fmt.Errorf("engine: closed")
	}
	fullKey := makeKey(namespace, key)
	if _, err := e.wal.Append(wal.OpDelete, fullKey, nil); err != nil {
		return fmt.Errorf("engine: wal append: %w", err)
	}
	e.mu.Lock()
	e.memtable.Delete(fullKey)
	e.mu.Unlock()
	return nil
}

// Get retrieves the value for key. Returns (nil, false, nil) if not found.
func (e *Engine) Get(namespace string, key []byte) ([]byte, bool, error) {
	if e.closed.Load() {
		return nil, false, fmt.Errorf("engine: closed")
	}
	fullKey := makeKey(namespace, key)

	// 1. Check cache.
	cacheKey := string(fullKey)
	if cached := e.cache.Get(cacheKey); cached != nil {
		// First byte is a deleted flag.
		if len(cached) > 0 && cached[0] == 1 {
			return nil, false, nil
		}
		return cached[1:], true, nil
	}

	e.mu.RLock()
	defer e.mu.RUnlock()

	// 2. Check memtable.
	if v, found, deleted := e.memtable.Get(fullKey); found {
		if deleted {
			e.cache.Set(cacheKey, []byte{1}) // cache tombstone
			return nil, false, nil
		}
		payload := make([]byte, 1+len(v))
		payload[0] = 0
		copy(payload[1:], v)
		e.cache.Set(cacheKey, payload)
		return v, true, nil
	}

	// 3. Check immutable memtable (being flushed).
	if e.imm != nil {
		if v, found, deleted := e.imm.Get(fullKey); found {
			if deleted {
				return nil, false, nil
			}
			return v, true, nil
		}
	}

	// 4. Check segments newest-first.
	for _, seg := range e.segments {
		val, found, deleted := seg.Get(fullKey)
		if !found {
			continue
		}
		if deleted {
			e.cache.Set(cacheKey, []byte{1})
			return nil, false, nil
		}
		payload := make([]byte, 1+len(val))
		payload[0] = 0
		copy(payload[1:], val)
		e.cache.Set(cacheKey, payload)
		return val, true, nil
	}
	return nil, false, nil
}

// Scan iterates over all keys with the given namespace prefix in sorted order.
// fn receives each key (without the namespace prefix) and value.
// Returning false from fn stops iteration.
func (e *Engine) Scan(namespace string, fn func(key, value []byte) bool) error {
	if e.closed.Load() {
		return fmt.Errorf("engine: closed")
	}
	prefix := []byte(namespace + ":")
	end := append([]byte(namespace+":"), 0xFF)

	// Collect all entries from memtable + segments into a merged sorted view.
	// For simplicity we use a map to deduplicate (last writer wins by seqNum).
	seen := make(map[string]struct{})

	collect := func(sl *storage.SkipList) {
		if sl == nil {
			return
		}
		sl.Scan(prefix, end, func(k, v []byte, deleted bool) bool {
			ks := string(k)
			if _, ok := seen[ks]; ok {
				return true
			}
			seen[ks] = struct{}{}
			if !deleted {
				// Strip namespace prefix before calling fn.
				shortKey := k[len(prefix):]
				fn(shortKey, v)
			}
			return true
		})
	}

	e.mu.RLock()
	defer e.mu.RUnlock()

	collect(e.memtable)
	collect(e.imm)

	// Then segments (newest-first so first-seen wins).
	for _, seg := range e.segments {
		for _, entry := range seg.Entries() {
			ks := string(entry.Key)
			if !strings.HasPrefix(ks, string(prefix)) {
				continue
			}
			if _, ok := seen[ks]; ok {
				continue
			}
			seen[ks] = struct{}{}
			if !entry.Deleted {
				shortKey := entry.Key[len(prefix):]
				if !fn(shortKey, entry.Value) {
					return nil
				}
			}
		}
	}
	return nil
}

// Close flushes the memtable and shuts down background workers.
func (e *Engine) Close() error {
	if !e.closed.CompareAndSwap(false, true) {
		return nil
	}
	close(e.closeCh)
	e.wg.Wait()

	// Final flush.
	e.mu.Lock()
	if e.memtable.Count() > 0 {
		_ = e.flushMemtable(e.memtable)
	}
	e.mu.Unlock()

	return e.wal.Close()
}

// Stats returns a snapshot of engine metrics.
func (e *Engine) Stats() map[string]interface{} {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return map[string]interface{}{
		"memtable_size":  e.memtable.MemSize(),
		"memtable_count": e.memtable.Count(),
		"segments":       len(e.segments),
		"cache_len":      e.cache.Len(),
		"cache_used":     e.cache.UsedBytes(),
		"wal_seq":        e.wal.SeqNum(),
	}
}

// Namespaces returns all namespaces currently visible in memtables and segments.
// It is used to reconstruct database metadata after process restarts.
func (e *Engine) Namespaces() []string {
	seen := make(map[string]struct{})

	collectKey := func(key []byte) {
		parts := strings.SplitN(string(key), ":", 2)
		if len(parts) != 2 || parts[0] == "" {
			return
		}
		seen[parts[0]] = struct{}{}
	}

	collectSkipList := func(sl *storage.SkipList) {
		if sl == nil {
			return
		}
		sl.Scan(nil, nil, func(key, _ []byte, _ bool) bool {
			collectKey(key)
			return true
		})
	}

	e.mu.RLock()
	collectSkipList(e.memtable)
	collectSkipList(e.imm)
	for _, seg := range e.segments {
		for _, entry := range seg.Entries() {
			collectKey(entry.Key)
		}
	}
	e.mu.RUnlock()

	namespaces := make([]string, 0, len(seen))
	for ns := range seen {
		namespaces = append(namespaces, ns)
	}
	sort.Strings(namespaces)
	return namespaces
}

// --- internal ----------------------------------------------------------------

// makeKey creates a namespaced storage key: "namespace:key".
func makeKey(namespace string, key []byte) []byte {
	ns := namespace + ":"
	full := make([]byte, len(ns)+len(key))
	copy(full, ns)
	copy(full[len(ns):], key)
	return full
}

// loadSegments scans DataDir for *.seg files and opens them.
func (e *Engine) loadSegments() (uint64, error) {
	entries, err := os.ReadDir(e.opts.DataDir)
	if err != nil {
		return 0, err
	}
	var paths []string
	var maxSeq uint64
	for _, de := range entries {
		if !de.IsDir() && strings.HasSuffix(de.Name(), ".seg") {
			paths = append(paths, filepath.Join(e.opts.DataDir, de.Name()))
			name := strings.TrimSuffix(de.Name(), ".seg")
			if seq, err := strconv.ParseUint(name, 10, 64); err == nil && seq > maxSeq {
				maxSeq = seq
			}
		}
	}
	// Sort by filename (seq number is encoded there) newest-first.
	sort.Sort(sort.Reverse(sort.StringSlice(paths)))

	for _, p := range paths {
		seg, err := storage.OpenSegment(p)
		if err != nil {
			return 0, fmt.Errorf("engine: open segment %s: %w", p, err)
		}
		e.segments = append(e.segments, seg)
	}
	return maxSeq, nil
}

// replayWAL reads the WAL and rebuilds the memtable.
func (e *Engine) replayWAL() error {
	_, err := e.wal.Replay(func(r wal.Record) error {
		switch r.Op {
		case wal.OpPut:
			e.memtable.Put(r.Key, r.Value, false)
		case wal.OpDelete:
			e.memtable.Delete(r.Key)
		}
		return nil
	})
	return err
}

// flushLoop waits for flush signals and flushes the memtable to disk.
func (e *Engine) flushLoop() {
	defer e.wg.Done()
	for {
		select {
		case <-e.closeCh:
			return
		case <-e.flushCh:
			e.mu.Lock()
			if e.memtable.MemSize() >= e.opts.MemTableSize && e.imm == nil {
				e.imm = e.memtable
				e.memtable = storage.NewSkipList()
			}
			imm := e.imm
			e.mu.Unlock()

			if imm != nil {
				if err := e.flushMemtable(imm); err == nil {
					e.mu.Lock()
					e.imm = nil
					e.mu.Unlock()
				}
			}
		}
	}
}

// flushMemtable writes the skip list to a new segment file.
func (e *Engine) flushMemtable(sl *storage.SkipList) error {
	entries := sl.All()
	if len(entries) == 0 {
		return nil
	}
	segmentSeq := e.nextSegmentSeq()
	path := filepath.Join(e.opts.DataDir, fmt.Sprintf("%020d.seg", segmentSeq))
	w, err := storage.NewSegmentWriter(path, len(entries), e.opts.BloomFPRate)
	if err != nil {
		return err
	}
	for _, en := range entries {
		w.Add(en.Key, en.Value, en.Deleted)
	}
	seg, err := w.Flush()
	if err != nil {
		return err
	}
	seg.SetSeqNum(segmentSeq)
	seg.SetLevel(0)

	e.mu.Lock()
	e.segments = append([]*storage.Segment{seg}, e.segments...)
	e.mu.Unlock()

	// Write checkpoint to WAL so we can truncate earlier records.
	_ = e.wal.Checkpoint(e.wal.SeqNum())
	return nil
}

// compactionLoop merges small L0 segments into larger L1+ segments.
// It sleeps between checks to avoid spinning the CPU.
func (e *Engine) compactionLoop() {
	defer e.wg.Done()
	// Check for compaction every 500 ms.  Using a ticker rather than a
	// bare default-select prevents the goroutine from consuming a full CPU
	// core when there is nothing to compact.
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-e.closeCh:
			return
		case <-ticker.C:
			e.mu.RLock()
			l0count := 0
			for _, s := range e.segments {
				if s.Level() == 0 {
					l0count++
				}
			}
			e.mu.RUnlock()
			if l0count >= 4 && e.compacting.CompareAndSwap(false, true) {
				func() {
					defer e.compacting.Store(false)
					_ = e.compact()
				}()
			}
		}
	}
}

// compact merges all L0 segments into a single L1 segment.
func (e *Engine) compact() error {
	e.mu.Lock()
	var l0 []*storage.Segment
	for _, s := range e.segments {
		if s.Level() == 0 {
			l0 = append(l0, s)
		}
	}
	e.mu.Unlock()

	if len(l0) < 2 {
		return nil
	}

	// Merge entries from all L0 segments (newest-first wins).
	merged := make(map[string]storage.SegmentEntry)
	for _, seg := range l0 {
		for _, en := range seg.Entries() {
			k := string(en.Key)
			if _, exists := merged[k]; !exists {
				merged[k] = en
			}
		}
	}

	// Sort keys.
	keys := make([]string, 0, len(merged))
	for k := range merged {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	segmentSeq := e.nextSegmentSeq()
	path := filepath.Join(e.opts.DataDir, fmt.Sprintf("%020d.seg", segmentSeq))
	w, err := storage.NewSegmentWriter(path, len(merged), e.opts.BloomFPRate)
	if err != nil {
		return err
	}
	for _, k := range keys {
		en := merged[k]
		w.Add(en.Key, en.Value, en.Deleted)
	}
	newSeg, err := w.Flush()
	if err != nil {
		return err
	}
	newSeg.SetLevel(1)
	newSeg.SetSeqNum(segmentSeq)

	// Replace only the segments that were compacted. Newer L0 segments that were
	// flushed while compaction was running must stay ahead of the compacted output.
	remove := make(map[string]struct{}, len(l0))
	for _, s := range l0 {
		remove[s.Path()] = struct{}{}
	}

	e.mu.Lock()
	newSegments := make([]*storage.Segment, 0, len(e.segments)-len(l0)+1)
	inserted := false
	for _, s := range e.segments {
		if _, ok := remove[s.Path()]; ok {
			if !inserted {
				newSegments = append(newSegments, newSeg)
				inserted = true
			}
			continue
		}
		newSegments = append(newSegments, s)
	}
	if !inserted {
		newSegments = append([]*storage.Segment{newSeg}, newSegments...)
	}
	e.segments = newSegments
	e.mu.Unlock()

	// Delete old L0 files.
	for _, s := range l0 {
		_ = os.Remove(s.Path())
	}
	return nil
}

func (e *Engine) nextSegmentSeq() uint64 {
	return e.seqNum.Add(1)
}
