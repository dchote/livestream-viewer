package events

import (
	"encoding/json"
	"sync"
)

// Hub fans out SSE payloads to subscribers.
type Hub struct {
	mu     sync.Mutex
	closed bool
	subs   map[chan []byte]struct{}
}

func NewHub() *Hub {
	return &Hub{subs: map[chan []byte]struct{}{}}
}

// Subscribe returns a buffered channel of already-formatted SSE chunks.
// The unsubscribe function is safe to call after Close.
func (h *Hub) Subscribe() (<-chan []byte, func()) {
	ch := make(chan []byte, 8)
	h.mu.Lock()
	if h.closed {
		h.mu.Unlock()
		close(ch)
		return ch, func() {}
	}
	h.subs[ch] = struct{}{}
	h.mu.Unlock()
	return ch, func() {
		h.mu.Lock()
		defer h.mu.Unlock()
		if _, ok := h.subs[ch]; !ok {
			return
		}
		delete(h.subs, ch)
		close(ch)
	}
}

// Close closes every subscriber channel so SSE handlers unblock during shutdown.
// Further Publish calls are no-ops; further Subscribe calls return a closed channel.
func (h *Hub) Close() {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.closed {
		return
	}
	h.closed = true
	for ch := range h.subs {
		delete(h.subs, ch)
		close(ch)
	}
}

// PublishDisplayState sends a display.state event.
func (h *Hub) PublishDisplayState(v any) {
	payload, err := json.Marshal(v)
	if err != nil {
		return
	}
	chunk := []byte("event: display.state\ndata: " + string(payload) + "\n\n")
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.closed {
		return
	}
	for ch := range h.subs {
		select {
		case ch <- chunk:
		default:
		}
	}
}
