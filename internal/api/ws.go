package api

import (
	"context"
	"encoding/json"
	"net/http"
	"sync"
	"time"

	"github.com/coder/websocket"

	"printmarshal/internal/model"
)

// Hub fans events out to WebSocket subscribers (spec §10 "WS /stream").
type Hub struct {
	mu   sync.RWMutex
	subs map[chan []byte]struct{}
}

// NewHub returns an empty hub.
func NewHub() *Hub { return &Hub{subs: map[chan []byte]struct{}{}} }

// Publish broadcasts an event; slow subscribers drop messages rather than block.
func (h *Hub) Publish(e model.Event) {
	if e.At.IsZero() {
		e.At = time.Now().UTC()
	}
	b, err := json.Marshal(e)
	if err != nil {
		return
	}
	h.mu.RLock()
	defer h.mu.RUnlock()
	for ch := range h.subs {
		select {
		case ch <- b:
		default:
		}
	}
}

func (h *Hub) subscribe() chan []byte {
	ch := make(chan []byte, 256)
	h.mu.Lock()
	h.subs[ch] = struct{}{}
	h.mu.Unlock()
	return ch
}

func (h *Hub) unsubscribe(ch chan []byte) {
	h.mu.Lock()
	delete(h.subs, ch)
	h.mu.Unlock()
}

// ServeHTTP upgrades to WebSocket and streams events until the client leaves.
func (h *Hub) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	c, err := websocket.Accept(w, r, &websocket.AcceptOptions{OriginPatterns: []string{"*"}})
	if err != nil {
		return
	}
	defer c.CloseNow()
	ctx := c.CloseRead(r.Context())
	ch := h.subscribe()
	defer h.unsubscribe(ch)
	ping := time.NewTicker(25 * time.Second)
	defer ping.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ping.C:
			pctx, cancel := context.WithTimeout(ctx, 5*time.Second)
			err := c.Ping(pctx)
			cancel()
			if err != nil {
				return
			}
		case msg := <-ch:
			wctx, cancel := context.WithTimeout(ctx, 5*time.Second)
			err := c.Write(wctx, websocket.MessageText, msg)
			cancel()
			if err != nil {
				return
			}
		}
	}
}
