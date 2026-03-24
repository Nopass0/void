package logs

import (
	"sync"
	"time"
)

// LogEntry represents a single structured log line.
type LogEntry struct {
	Level   string                 `json:"level"`
	Time    time.Time              `json:"time"`
	Message string                 `json:"message"`
	Fields  map[string]interface{} `json:"fields,omitempty"`
}

// RingBuffer stores the last N logs in memory.
type RingBuffer struct {
	mu     sync.RWMutex
	logs   []LogEntry
	max    int
	cursor int
}

// GlobalRing holds the latest server logs for the /v1/logs endpoint.
var GlobalRing = NewRingBuffer(1000)

var listenersMu sync.RWMutex
var Listeners = make(map[chan LogEntry]struct{})

func Subscribe() chan LogEntry {
	ch := make(chan LogEntry, 100)
	listenersMu.Lock()
	defer listenersMu.Unlock()
	Listeners[ch] = struct{}{}
	return ch
}

func Unsubscribe(ch chan LogEntry) {
	listenersMu.Lock()
	defer listenersMu.Unlock()
	delete(Listeners, ch)
	close(ch)
}

// NewRingBuffer creates a fixed-size circular buffer.
func NewRingBuffer(max int) *RingBuffer {
	return &RingBuffer{
		logs: make([]LogEntry, 0, max),
		max:  max,
	}
}

// Add appends a new log, overriding old ones if max is reached.
func (r *RingBuffer) Add(entry LogEntry) {
	r.mu.Lock()
	if len(r.logs) < r.max {
		r.logs = append(r.logs, entry)
	} else {
		r.logs[r.cursor] = entry
		r.cursor = (r.cursor + 1) % r.max
	}
	r.mu.Unlock()

	listenersMu.RLock()
	defer listenersMu.RUnlock()
	for ch := range Listeners {
		select {
		case ch <- entry:
		default:
		}
	}
}

// Get returns all logs ordered chronologically. Limit and skip can be applied.
// limit=-1 means no limit.
func (r *RingBuffer) Get(limit, skip int) []LogEntry {
	r.mu.RLock()
	defer r.mu.RUnlock()

	n := len(r.logs)
	out := make([]LogEntry, 0, n)
	
	// Reorder from oldest to newest based on cursor
	if n == r.max {
		out = append(out, r.logs[r.cursor:]...)
		out = append(out, r.logs[:r.cursor]...)
	} else {
		out = append(out, r.logs...)
	}

	// Reverse (newest first)
	for i, j := 0, len(out)-1; i < j; i, j = i+1, j-1 {
		out[i], out[j] = out[j], out[i]
	}

	if skip > 0 {
		if skip >= len(out) {
			return nil
		}
		out = out[skip:]
	}
	if limit >= 0 && limit < len(out) {
		out = out[:limit]
	}
	
	return out
}

// Len returns the total number of buffered entries.
func (r *RingBuffer) Len() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.logs)
}
