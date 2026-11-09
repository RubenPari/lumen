// Package events is a tiny in-process pub/sub used to push changes to clients.
package events

import "sync"

// Event is a notification sent to connected clients.
type Event struct {
	Type string `json:"type"` // library.changed, scan.progress, userdata.changed, livetv.changed
	Data any    `json:"data,omitempty"`
	User string `json:"-"` // when set, only delivered to this user
}

// Hub fans out events to subscribers.
type Hub struct {
	mu   sync.Mutex
	subs map[chan Event]struct{}
}

// New creates a hub.
func New() *Hub { return &Hub{subs: map[chan Event]struct{}{}} }

// Subscribe returns a channel of events and a cancel function.
func (h *Hub) Subscribe() (<-chan Event, func()) {
	ch := make(chan Event, 32)
	h.mu.Lock()
	h.subs[ch] = struct{}{}
	h.mu.Unlock()
	return ch, func() {
		h.mu.Lock()
		if _, ok := h.subs[ch]; ok {
			delete(h.subs, ch)
			close(ch)
		}
		h.mu.Unlock()
	}
}

// Publish sends an event without blocking (slow subscribers drop events).
func (h *Hub) Publish(e Event) {
	if h == nil {
		return
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	for ch := range h.subs {
		select {
		case ch <- e:
		default:
		}
	}
}
