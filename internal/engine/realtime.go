package engine

import (
	"sync"

	"github.com/voiddb/void/internal/engine/types"
)

// EventType defines the kind of database event.
type EventType string

const (
	EventInsert EventType = "insert"
	EventUpdate EventType = "update"
	EventDelete EventType = "delete"
)

// Event represents a database modification.
type Event struct {
	Type       EventType       `json:"type"`
	Database   string          `json:"database"`
	Collection string          `json:"collection"`
	DocID      string          `json:"doc_id"`
	Doc        *types.Document `json:"doc,omitempty"`
}

// Hub manages pub/sub subscriptions for real-time events.
type Hub struct {
	mu   sync.RWMutex
	subs map[chan Event]struct{}
}

// NewHub creates a new realtime event hub.
func NewHub() *Hub {
	return &Hub{
		subs: make(map[chan Event]struct{}),
	}
}

// Subscribe adds a channel to receive events.
func (h *Hub) Subscribe(ch chan Event) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.subs[ch] = struct{}{}
}

// Unsubscribe removes a channel.
func (h *Hub) Unsubscribe(ch chan Event) {
	h.mu.Lock()
	defer h.mu.Unlock()
	delete(h.subs, ch)
}

// Broadcast sends an event to all subscribers.
func (h *Hub) Broadcast(evt Event) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	for ch := range h.subs {
		// Non-blocking send; if a client is too slow, we drop the event for them.
		select {
		case ch <- evt:
		default:
		}
	}
}
