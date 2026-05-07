package stream

import (
	"sync"
	"time"
)

// EventMsg is the payload published to SSE subscribers when a new event is
// created. It mirrors the subset of fields needed to render an event row in
// the UI plus enough metadata for server-side filtering.
type EventMsg struct {
	ID        uint           `json:"id"`
	TokenID   *uint          `json:"token_id,omitempty"`
	Source    string         `json:"source,omitempty"`
	Event     string         `json:"event,omitempty"`
	Title     string         `json:"title"`
	Message   string         `json:"message,omitempty"`
	Level     string         `json:"level"`
	Metadata  map[string]any `json:"metadata,omitempty"`
	CreatedAt time.Time      `json:"created_at"`
}

// Hub is a tiny in-process pub/sub for live-tail subscribers.
// One Hub is shared between the emit handler (publisher) and SSE handler
// (subscriber). Drops messages on slow consumers rather than blocking emit.
type Hub struct {
	mu   sync.RWMutex
	subs map[chan EventMsg]struct{}
}

func NewHub() *Hub {
	return &Hub{subs: make(map[chan EventMsg]struct{})}
}

func (h *Hub) Subscribe() (<-chan EventMsg, func()) {
	ch := make(chan EventMsg, 16)
	h.mu.Lock()
	h.subs[ch] = struct{}{}
	h.mu.Unlock()

	cancel := func() {
		h.mu.Lock()
		if _, ok := h.subs[ch]; ok {
			delete(h.subs, ch)
			close(ch)
		}
		h.mu.Unlock()
	}
	return ch, cancel
}

func (h *Hub) Publish(ev EventMsg) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	for ch := range h.subs {
		select {
		case ch <- ev:
		default:
			// Slow consumer; drop rather than block emit.
		}
	}
}
