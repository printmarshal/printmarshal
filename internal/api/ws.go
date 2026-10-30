package api

import (
	"context"
	"encoding/json"
	"net/http"
	"sync"
	"time"

	"github.com/coder/websocket"

	"printmarshal/internal/events"
	"printmarshal/internal/model"
)

// Hub fans events out to WebSocket subscribers (spec §10 "WS /stream").
type Hub struct {
	mu   sync.RWMutex
	subs map[*subscriber]struct{}
	// Scope, when set, returns the filter for a subscriber's request: nil for
	// an unscoped principal, otherwise a predicate over the event payload. A
	// group-scoped principal receives only events about devices in its
	// groups (audit NV-06).
	Scope func(r *http.Request) func(data any) bool
	// Origins are extra Origin host patterns accepted besides same-origin
	// (config auth.allowedOrigins). Empty means same-origin only.
	Origins []string
	// Authorize re-checks the caller's session while a stream is open; a
	// revoked or expired session closes the stream within a minute (audit A05).
	Authorize func(r *http.Request) bool
	// RecheckEvery overrides the re-authorization interval (tests).
	RecheckEvery time.Duration
}

type subscriber struct {
	ch    chan []byte
	mu    sync.Mutex
	allow func(data any) bool
}

func (sub *subscriber) wants(data any) bool {
	sub.mu.Lock()
	allow := sub.allow
	sub.mu.Unlock()
	return allow == nil || allow(data)
}

func (sub *subscriber) setAllow(allow func(data any) bool) {
	sub.mu.Lock()
	sub.allow = allow
	sub.mu.Unlock()
}

// NewHub returns an empty hub.
func NewHub() *Hub { return &Hub{subs: map[*subscriber]struct{}{}} }

// Publish broadcasts an event; slow subscribers drop messages rather than block.
func (h *Hub) Publish(e model.Event) {
	if e.At.IsZero() {
		e.At = time.Now().UTC()
	}
	b, err := json.Marshal(e)
	if err != nil {
		return
	}
	h.fanout(b, e.Data)
}

func (h *Hub) fanout(b []byte, data any) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	for sub := range h.subs {
		if !sub.wants(data) {
			continue
		}
		select {
		case sub.ch <- b:
		default:
		}
	}
}

func (h *Hub) subscribe(allow func(data any) bool) *subscriber {
	sub := &subscriber{ch: make(chan []byte, 256), allow: allow}
	h.mu.Lock()
	h.subs[sub] = struct{}{}
	h.mu.Unlock()
	return sub
}

func (h *Hub) unsubscribe(sub *subscriber) {
	h.mu.Lock()
	delete(h.subs, sub)
	h.mu.Unlock()
}

// ServeHTTP upgrades to WebSocket and streams events until the client leaves.
func (h *Hub) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	c, err := websocket.Accept(w, r, &websocket.AcceptOptions{OriginPatterns: h.Origins})
	if err != nil {
		return
	}
	defer c.CloseNow()
	ctx := c.CloseRead(r.Context())
	var allow func(any) bool
	if h.Scope != nil {
		allow = h.Scope(r)
	}
	sub := h.subscribe(allow)
	defer h.unsubscribe(sub)
	ch := sub.ch
	ping := time.NewTicker(25 * time.Second)
	defer ping.Stop()
	every := h.RecheckEvery
	if every <= 0 {
		every = time.Minute
	}
	recheck := time.NewTicker(every)
	defer recheck.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-recheck.C:
			if h.Authorize != nil && !h.Authorize(r) {
				_ = c.Close(websocket.StatusPolicyViolation, "session ended")
				return
			}
			if h.Scope != nil {
				sub.setAllow(h.Scope(r)) // group membership may have changed
			}
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

// Handle implements events.Subscriber: every bus event reaches the live stream.
func (h *Hub) Handle(env events.Envelope) {
	b, err := json.Marshal(env)
	if err != nil {
		return
	}
	h.fanout(b, env.Data)
}
