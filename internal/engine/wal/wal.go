// Package wal implements a Write-Ahead Log (WAL) for VoidDB.
// Every mutation is first written sequentially to the WAL file before being
// applied to the memtable. On crash recovery the WAL is replayed to reconstruct
// the in-memory state that had not yet been flushed to disk.
//
// WAL record format:
//
//	[length  uint32] – total record byte size (including this header)
//	[crc32   uint32] – checksum of the payload bytes
//	[seqNum  uint64] – monotonically increasing write sequence number
//	[opType  uint8 ] – 0=Put, 1=Delete, 2=Checkpoint
//	[keyLen  uint32]
//	[key     bytes ]
//	[valLen  uint32] – 0 for Delete
//	[val     bytes ]
package wal

import (
	"encoding/binary"
	"fmt"
	"hash/crc32"
	"io"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
)

// OpType classifies the WAL record.
type OpType uint8

const (
	// OpPut is a key/value upsert.
	OpPut OpType = 0
	// OpDelete is a tombstone record.
	OpDelete OpType = 1
	// OpCheckpoint marks the point up to which data has been flushed to SSTables.
	OpCheckpoint OpType = 2
)

// Record is a single decoded WAL entry.
type Record struct {
	SeqNum uint64
	Op     OpType
	Key    []byte
	Value  []byte
}

// WAL is a write-ahead log that appends records to a single file.
// It is safe for concurrent use – writes are serialised under a mutex.
type WAL struct {
	mu      sync.Mutex
	file    *os.File
	path    string
	seqNum  atomic.Uint64
	sync    bool   // if true, call fsync after every write
	buf     []byte // reusable encoding buffer
}

// Open opens (or creates) the WAL at path.
// If sync is true, every Append call will fsync the file before returning.
func Open(path string, syncOnWrite bool) (*WAL, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return nil, fmt.Errorf("wal: mkdir: %w", err)
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR|os.O_APPEND, 0644)
	if err != nil {
		return nil, fmt.Errorf("wal: open: %w", err)
	}
	w := &WAL{file: f, path: path, sync: syncOnWrite}
	// Determine the highest seqNum already in the file.
	maxSeq, err := scanMaxSeq(f)
	if err != nil {
		return nil, fmt.Errorf("wal: scan: %w", err)
	}
	w.seqNum.Store(maxSeq)
	return w, nil
}

// Append writes an Op record to the WAL and returns the assigned sequence number.
func (w *WAL) Append(op OpType, key, value []byte) (uint64, error) {
	w.mu.Lock()
	defer w.mu.Unlock()

	seq := w.seqNum.Add(1)
	rec := encodeRecord(seq, op, key, value, &w.buf)
	if _, err := w.file.Write(rec); err != nil {
		return 0, fmt.Errorf("wal: write: %w", err)
	}
	if w.sync {
		if err := w.file.Sync(); err != nil {
			return 0, fmt.Errorf("wal: sync: %w", err)
		}
	}
	return seq, nil
}

// Checkpoint writes a checkpoint record and flushes the file.
// All records with seqNum ≤ checkpointSeq have been durably written to SSTables
// and the WAL may be truncated up to this point.
func (w *WAL) Checkpoint(checkpointSeq uint64) error {
	w.mu.Lock()
	defer w.mu.Unlock()

	seq := w.seqNum.Add(1)
	rec := encodeRecord(seq, OpCheckpoint, uint64ToBytes(checkpointSeq), nil, &w.buf)
	if _, err := w.file.Write(rec); err != nil {
		return fmt.Errorf("wal: checkpoint write: %w", err)
	}
	return w.file.Sync()
}

// Replay reads the WAL from the beginning and calls fn for each non-checkpoint
// record. Used during crash recovery. Returns the highest seqNum seen.
func (w *WAL) Replay(fn func(r Record) error) (uint64, error) {
	w.mu.Lock()
	defer w.mu.Unlock()

	if _, err := w.file.Seek(0, io.SeekStart); err != nil {
		return 0, fmt.Errorf("wal: seek: %w", err)
	}
	return replayFile(w.file, fn)
}

// Close syncs and closes the underlying file.
func (w *WAL) Close() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if err := w.file.Sync(); err != nil {
		return err
	}
	return w.file.Close()
}

// Path returns the filesystem path of the WAL file.
func (w *WAL) Path() string { return w.path }

// SeqNum returns the current monotonic sequence counter.
func (w *WAL) SeqNum() uint64 { return w.seqNum.Load() }

// Truncate creates a new WAL file keeping only records with seqNum > afterSeq.
// This is called after a successful flush to reclaim disk space.
func (w *WAL) Truncate(afterSeq uint64) error {
	w.mu.Lock()
	defer w.mu.Unlock()

	// Collect records to keep.
	if _, err := w.file.Seek(0, io.SeekStart); err != nil {
		return err
	}
	var keep []Record
	_, err := replayFile(w.file, func(r Record) error {
		if r.SeqNum > afterSeq {
			keep = append(keep, r)
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("wal: truncate scan: %w", err)
	}

	// Rewrite the file.
	if err := w.file.Truncate(0); err != nil {
		return err
	}
	if _, err := w.file.Seek(0, io.SeekStart); err != nil {
		return err
	}
	for _, r := range keep {
		rec := encodeRecord(r.SeqNum, r.Op, r.Key, r.Value, &w.buf)
		if _, err := w.file.Write(rec); err != nil {
			return err
		}
	}
	return w.file.Sync()
}

// --- internal helpers --------------------------------------------------------

const recHeaderSize = 4 + 4 + 8 + 1 + 4 + 4 // length+crc+seq+op+keyLen+valLen = 25

// encodeRecord builds the binary record into *buf (reused across calls).
func encodeRecord(seq uint64, op OpType, key, value []byte, buf *[]byte) []byte {
	total := recHeaderSize + len(key) + len(value)
	if cap(*buf) < total {
		*buf = make([]byte, total, total*2)
	}
	b := (*buf)[:total]

	// Placeholder for length and crc (filled at the end).
	binary.LittleEndian.PutUint32(b[0:4], uint32(total))
	binary.LittleEndian.PutUint64(b[8:16], seq)
	b[16] = byte(op)
	binary.LittleEndian.PutUint32(b[17:21], uint32(len(key)))
	copy(b[21:], key)
	off := 21 + len(key)
	binary.LittleEndian.PutUint32(b[off:off+4], uint32(len(value)))
	copy(b[off+4:], value)

	// Compute and store CRC over everything except the first 8 bytes (length+crc).
	checksum := crc32.ChecksumIEEE(b[8:])
	binary.LittleEndian.PutUint32(b[4:8], checksum)
	return b
}

// replayFile reads records from r and calls fn for each valid non-checkpoint one.
func replayFile(r io.ReadSeeker, fn func(Record) error) (uint64, error) {
	var maxSeq uint64
	hdr := make([]byte, recHeaderSize)
	for {
		_, err := io.ReadFull(r, hdr)
		if err == io.EOF || err == io.ErrUnexpectedEOF {
			break
		}
		if err != nil {
			return maxSeq, fmt.Errorf("wal: read header: %w", err)
		}
		total := int(binary.LittleEndian.Uint32(hdr[0:4]))
		if total < recHeaderSize {
			break // corrupted; stop here
		}
		payload := make([]byte, total)
		copy(payload, hdr)
		rem := total - recHeaderSize
		if rem > 0 {
			if _, err := io.ReadFull(r, payload[recHeaderSize:]); err != nil {
				break
			}
		}

		storedCRC := binary.LittleEndian.Uint32(payload[4:8])
		computedCRC := crc32.ChecksumIEEE(payload[8:])
		if storedCRC != computedCRC {
			break // corrupted record; stop replaying
		}

		seq := binary.LittleEndian.Uint64(payload[8:16])
		op := OpType(payload[16])
		keyLen := int(binary.LittleEndian.Uint32(payload[17:21]))
		key := payload[21 : 21+keyLen]
		off := 21 + keyLen
		valLen := int(binary.LittleEndian.Uint32(payload[off : off+4]))
		val := payload[off+4 : off+4+valLen]

		if seq > maxSeq {
			maxSeq = seq
		}
		if op != OpCheckpoint {
			if err := fn(Record{SeqNum: seq, Op: op, Key: key, Value: val}); err != nil {
				return maxSeq, err
			}
		}
	}
	return maxSeq, nil
}

// scanMaxSeq fast-scans only the seqNum fields of all records.
func scanMaxSeq(f *os.File) (uint64, error) {
	if _, err := f.Seek(0, io.SeekStart); err != nil {
		return 0, err
	}
	var maxSeq uint64
	hdr := make([]byte, recHeaderSize)
	for {
		_, err := io.ReadFull(f, hdr)
		if err != nil {
			break
		}
		total := int(binary.LittleEndian.Uint32(hdr[0:4]))
		if total < recHeaderSize {
			break
		}
		seq := binary.LittleEndian.Uint64(hdr[8:16])
		if seq > maxSeq {
			maxSeq = seq
		}
		skip := int64(total - recHeaderSize)
		if skip > 0 {
			if _, err := f.Seek(skip, io.SeekCurrent); err != nil {
				break
			}
		}
	}
	return maxSeq, nil
}

// uint64ToBytes converts a uint64 to 8 little-endian bytes.
func uint64ToBytes(n uint64) []byte {
	b := make([]byte, 8)
	binary.LittleEndian.PutUint64(b, n)
	return b
}
