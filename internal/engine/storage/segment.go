// Package storage – segment.go implements SSTable-style immutable segments.
// Each segment is a sorted, read-only file produced by flushing the memtable
// or merging smaller segments during compaction.
//
// Segment file layout:
//
//	[Header  32 B]
//	[Data blocks …]
//	[Bloom filter block]
//	[Index block]
//	[Footer  16 B]  ← contains offsets to bloom+index
package storage

import (
	"encoding/binary"
	"fmt"
	"io"
	"os"
	"sort"
	"sync"
)

// segmentMagic is written at the start and end of every segment file.
const segmentMagic = uint32(0x5345474D) // "SEGM"

// segmentVersion is the current file format version.
const segmentVersion = uint16(1)

// SegmentEntry is a single key/value record within a segment.
type SegmentEntry struct {
	Key   []byte
	Value []byte
	// Deleted marks a tombstone record (value is nil when true).
	Deleted bool
}

// Segment is an immutable, on-disk sorted string table.
// Multiple goroutines may read it concurrently without locking.
type Segment struct {
	mu      sync.RWMutex
	path    string
	entries []SegmentEntry // kept in memory for small segments; for large ones use index
	bloom   *BloomFilter
	size    int64
	level   int
	seqNum  uint64
}

// SegmentWriter writes a new immutable segment file from a sorted list of entries.
type SegmentWriter struct {
	path    string
	file    *os.File
	entries []SegmentEntry
	bloom   *BloomFilter
}

// NewSegmentWriter opens path for writing and returns a SegmentWriter.
// The bloom filter is pre-sized for expectedKeys with the given false-positive rate.
func NewSegmentWriter(path string, expectedKeys int, fpRate float64) (*SegmentWriter, error) {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
	if err != nil {
		return nil, fmt.Errorf("segment: open for write: %w", err)
	}
	return &SegmentWriter{
		path:    path,
		file:    f,
		entries: make([]SegmentEntry, 0, expectedKeys),
		bloom:   NewBloomFilter(expectedKeys, fpRate),
	}, nil
}

// Add appends a key/value pair. Keys must be added in sorted order.
func (w *SegmentWriter) Add(key, value []byte, deleted bool) {
	w.bloom.Add(key)
	w.entries = append(w.entries, SegmentEntry{
		Key:     append([]byte(nil), key...),
		Value:   append([]byte(nil), value...),
		Deleted: deleted,
	})
}

// Flush writes all entries to disk and closes the file.
// Returns the finished Segment ready for reads.
func (w *SegmentWriter) Flush() (*Segment, error) {
	defer w.file.Close()

	var buf []byte

	// Header: magic(4) + version(2) + numEntries(8) + reserved(18) = 32 bytes
	hdr := make([]byte, 32)
	binary.LittleEndian.PutUint32(hdr[0:4], segmentMagic)
	binary.LittleEndian.PutUint16(hdr[4:6], segmentVersion)
	binary.LittleEndian.PutUint64(hdr[6:14], uint64(len(w.entries)))
	if _, err := w.file.Write(hdr); err != nil {
		return nil, fmt.Errorf("segment: write header: %w", err)
	}
	written := int64(len(hdr))

	// Data block: sequence of [keyLen(4)][key][deleted(1)][valLen(4)][val]
	// Index: slice of (key, fileOffset) pairs for binary-search at read time.
	type indexEntry struct {
		key    []byte
		offset int64
	}
	index := make([]indexEntry, 0, len(w.entries))

	for _, e := range w.entries {
		index = append(index, indexEntry{key: e.Key, offset: written})
		recSize := 4 + len(e.Key) + 1 + 4 + len(e.Value)
		buf = ensureBuf(buf, recSize)
		binary.LittleEndian.PutUint32(buf[0:4], uint32(len(e.Key)))
		copy(buf[4:], e.Key)
		off := 4 + len(e.Key)
		if e.Deleted {
			buf[off] = 1
		} else {
			buf[off] = 0
		}
		off++
		binary.LittleEndian.PutUint32(buf[off:], uint32(len(e.Value)))
		off += 4
		copy(buf[off:], e.Value)

		if _, err := w.file.Write(buf[:recSize]); err != nil {
			return nil, fmt.Errorf("segment: write record: %w", err)
		}
		written += int64(recSize)
	}

	// Bloom filter block.
	bloomOff := written
	bloomBytes := w.bloom.Bytes()
	bloomHdr := make([]byte, 8)
	binary.LittleEndian.PutUint64(bloomHdr, uint64(len(bloomBytes)))
	if _, err := w.file.Write(bloomHdr); err != nil {
		return nil, fmt.Errorf("segment: write bloom header: %w", err)
	}
	if _, err := w.file.Write(bloomBytes); err != nil {
		return nil, fmt.Errorf("segment: write bloom: %w", err)
	}
	written += 8 + int64(len(bloomBytes))

	// Index block: numEntries(8) then for each: offset(8) + keyLen(4) + key
	indexOff := written
	idxHdr := make([]byte, 8)
	binary.LittleEndian.PutUint64(idxHdr, uint64(len(index)))
	if _, err := w.file.Write(idxHdr); err != nil {
		return nil, fmt.Errorf("segment: write index header: %w", err)
	}
	written += 8
	for _, ie := range index {
		entry := make([]byte, 8+4+len(ie.key))
		binary.LittleEndian.PutUint64(entry[0:8], uint64(ie.offset))
		binary.LittleEndian.PutUint32(entry[8:12], uint32(len(ie.key)))
		copy(entry[12:], ie.key)
		if _, err := w.file.Write(entry); err != nil {
			return nil, fmt.Errorf("segment: write index entry: %w", err)
		}
		written += int64(len(entry))
	}

	// Footer: bloomOff(8) + indexOff(8) = 16 bytes
	footer := make([]byte, 16)
	binary.LittleEndian.PutUint64(footer[0:8], uint64(bloomOff))
	binary.LittleEndian.PutUint64(footer[8:16], uint64(indexOff))
	if _, err := w.file.Write(footer); err != nil {
		return nil, fmt.Errorf("segment: write footer: %w", err)
	}
	written += 16

	if err := w.file.Sync(); err != nil {
		return nil, fmt.Errorf("segment: sync: %w", err)
	}

	return &Segment{
		path:    w.path,
		entries: w.entries,
		bloom:   w.bloom,
		size:    written,
	}, nil
}

// OpenSegment reads an existing segment file from disk.
func OpenSegment(path string) (*Segment, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("segment: open: %w", err)
	}
	defer f.Close()

	stat, err := f.Stat()
	if err != nil {
		return nil, err
	}
	size := stat.Size()

	// Read footer.
	footer := make([]byte, 16)
	if _, err := f.ReadAt(footer, size-16); err != nil {
		return nil, fmt.Errorf("segment: read footer: %w", err)
	}
	bloomOff := int64(binary.LittleEndian.Uint64(footer[0:8]))
	indexOff := int64(binary.LittleEndian.Uint64(footer[8:16]))

	// Read bloom filter.
	bloomHdr := make([]byte, 8)
	if _, err := f.ReadAt(bloomHdr, bloomOff); err != nil {
		return nil, fmt.Errorf("segment: read bloom header: %w", err)
	}
	bloomLen := int64(binary.LittleEndian.Uint64(bloomHdr))
	bloomBytes := make([]byte, bloomLen)
	if _, err := f.ReadAt(bloomBytes, bloomOff+8); err != nil {
		return nil, fmt.Errorf("segment: read bloom: %w", err)
	}
	bloom := BloomFromBytes(bloomBytes)

	// Read index.
	idxHdr := make([]byte, 8)
	if _, err := f.ReadAt(idxHdr, indexOff); err != nil {
		return nil, fmt.Errorf("segment: read index header: %w", err)
	}
	numEntries := int(binary.LittleEndian.Uint64(idxHdr))

	// Read all entries using the index.
	entries := make([]SegmentEntry, 0, numEntries)
	cursor := int64(32) // skip file header

	for i := 0; i < numEntries; i++ {
		// Read key length.
		klenBuf := make([]byte, 4)
		if _, err := f.ReadAt(klenBuf, cursor); err != nil {
			if err == io.EOF {
				break
			}
			return nil, fmt.Errorf("segment: read key len at %d: %w", cursor, err)
		}
		klen := int(binary.LittleEndian.Uint32(klenBuf))
		cursor += 4

		key := make([]byte, klen)
		if _, err := f.ReadAt(key, cursor); err != nil {
			return nil, fmt.Errorf("segment: read key: %w", err)
		}
		cursor += int64(klen)

		delBuf := make([]byte, 1)
		if _, err := f.ReadAt(delBuf, cursor); err != nil {
			return nil, fmt.Errorf("segment: read deleted flag: %w", err)
		}
		deleted := delBuf[0] == 1
		cursor++

		vlenBuf := make([]byte, 4)
		if _, err := f.ReadAt(vlenBuf, cursor); err != nil {
			return nil, fmt.Errorf("segment: read val len: %w", err)
		}
		vlen := int(binary.LittleEndian.Uint32(vlenBuf))
		cursor += 4

		val := make([]byte, vlen)
		if vlen > 0 {
			if _, err := f.ReadAt(val, cursor); err != nil {
				return nil, fmt.Errorf("segment: read val: %w", err)
			}
		}
		cursor += int64(vlen)

		entries = append(entries, SegmentEntry{Key: key, Value: val, Deleted: deleted})
	}

	return &Segment{
		path:    path,
		entries: entries,
		bloom:   bloom,
		size:    size,
	}, nil
}

// Get looks up a key in the segment.
// Returns (value, found, tombstone).
func (s *Segment) Get(key []byte) ([]byte, bool, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if !s.bloom.Test(key) {
		return nil, false, false
	}
	idx := sort.Search(len(s.entries), func(i int) bool {
		return string(s.entries[i].Key) >= string(key)
	})
	if idx < len(s.entries) && string(s.entries[idx].Key) == string(key) {
		e := s.entries[idx]
		return e.Value, true, e.Deleted
	}
	return nil, false, false
}

// Path returns the filesystem path of this segment.
func (s *Segment) Path() string { return s.path }

// Size returns the on-disk file size in bytes.
func (s *Segment) Size() int64 { return s.size }

// Level returns the compaction level (0 = freshly flushed).
func (s *Segment) Level() int { return s.level }

// SetLevel assigns a compaction level.
func (s *Segment) SetLevel(l int) { s.level = l }

// SeqNum is the write sequence number at time of flush.
func (s *Segment) SeqNum() uint64 { return s.seqNum }

// SetSeqNum stores the sequence number.
func (s *Segment) SetSeqNum(n uint64) { s.seqNum = n }

// Entries returns a read-only view of all records (for compaction/merge).
func (s *Segment) Entries() []SegmentEntry { return s.entries }

// ensureBuf grows buf to at least n bytes without zeroing.
func ensureBuf(buf []byte, n int) []byte {
	if cap(buf) >= n {
		return buf[:n]
	}
	return make([]byte, n, n*2)
}
