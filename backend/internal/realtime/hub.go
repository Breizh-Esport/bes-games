package realtime

import (
	"sync"
	"time"
)

// Hub is a simple fan-out pub/sub for room-scoped realtime events.
//
// Design goals:
// - Minimal dependencies and easy to reason about.
// - Non-blocking broadcasts: slow subscribers drop events rather than backpressure the whole room.
// - Explicit subscribe/unsubscribe lifecycle.
// - Room registry to create hubs on-demand.
//
// This is intended to be used by the HTTP/WebSocket layer:
// - When a client connects to a room WS endpoint, call Registry.Room(roomID).Subscribe(...)
// - When state changes (DB mutations), publish events via Registry.Room(roomID).Broadcast(...)
type Hub struct {
	mu   sync.RWMutex
	subs map[uint64]chan Event
	seq  uint64
}

// Event is a generic room event envelope.
// Payload should be JSON-marshalable by the caller.
// Timestamp defaults to time.Now().UTC() if zero.
type Event struct {
	Type      string    `json:"type"`
	RoomID    string    `json:"roomId"`
	Timestamp time.Time `json:"ts"`
	Payload   any       `json:"payload,omitempty"`
}

// NewHub creates a new Hub instance.
func NewHub() *Hub {
	return &Hub{
		subs: make(map[uint64]chan Event),
	}
}

// Subscribe registers a subscriber and returns:
// - a receive-only channel that will carry events
// - a cancel function that unregisters the subscriber and closes the channel
//
// buffer defines the channel buffer; if <= 0 a default is used.
func (h *Hub) Subscribe(buffer int) (<-chan Event, func()) {
	if buffer <= 0 {
		buffer = 64
	}

	h.mu.Lock()
	h.seq++
	id := h.seq
	ch := make(chan Event, buffer)
	h.subs[id] = ch
	h.mu.Unlock()

	cancel := func() {
		h.mu.Lock()
		c, ok := h.subs[id]
		if ok {
			delete(h.subs, id)
			close(c)
		}
		h.mu.Unlock()
	}

	return ch, cancel
}

// Broadcast fan-outs an event to all current subscribers.
//
// If a subscriber is slow and its buffer is full, the event is dropped for that subscriber.
func (h *Hub) Broadcast(ev Event) {
	if ev.Timestamp.IsZero() {
		ev.Timestamp = time.Now().UTC()
	}

	h.mu.RLock()
	defer h.mu.RUnlock()

	for _, ch := range h.subs {
		select {
		case ch <- ev:
		default:
			// Drop for this subscriber to avoid blocking.
		}
	}
}

// SubscriberCount returns the current number of subscribers.
func (h *Hub) SubscriberCount() int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.subs)
}

// Close closes all subscriber channels and resets the hub.
// After Close, the hub can be used again (new subscribers can be added).
func (h *Hub) Close() {
	h.mu.Lock()
	defer h.mu.Unlock()

	for id, ch := range h.subs {
		delete(h.subs, id)
		close(ch)
	}
}

// Registry manages per-room hubs.
type Registry struct {
	mu    sync.RWMutex
	rooms map[string]*Hub
}

// NewRegistry creates a new hub registry.
func NewRegistry() *Registry {
	return &Registry{
		rooms: make(map[string]*Hub),
	}
}

// Room returns the hub for the given roomID, creating it if necessary.
func (r *Registry) Room(roomID string) *Hub {
	r.mu.RLock()
	h := r.rooms[roomID]
	r.mu.RUnlock()
	if h != nil {
		return h
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	// Double-check under write lock.
	h = r.rooms[roomID]
	if h != nil {
		return h
	}

	h = NewHub()
	r.rooms[roomID] = h
	return h
}

// Remove deletes a room hub and closes all subscribers.
// Use this if you want to clean up after a room is deleted.
func (r *Registry) Remove(roomID string) {
	r.mu.Lock()
	h := r.rooms[roomID]
	if h != nil {
		delete(r.rooms, roomID)
	}
	r.mu.Unlock()

	if h != nil {
		h.Close()
	}
}

// Rooms returns the number of room hubs currently tracked.
func (r *Registry) Rooms() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.rooms)
}
