package alerts

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"printmarshal/internal/config"
	"printmarshal/internal/events"
	"printmarshal/internal/model"
	"printmarshal/internal/secrets"
	"printmarshal/internal/store"
)

func newTestNotifier(t *testing.T, dests []config.Notifier) (*Notifier, *store.SQLite, *events.Bus) {
	t.Helper()
	box, _ := secrets.New(make([]byte, 32))
	st, err := store.OpenSQLite(t.TempDir()+"/t.db", box)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	n := NewNotifier(dests, st, slog.New(slog.NewTextHandler(io.Discard, nil)))
	n.Backoff, n.MaxBackoff, n.MaxAttempts = time.Millisecond, 5*time.Millisecond, 3
	bus := events.NewBus()
	n.Attach(bus)
	return n, st, bus
}

func TestWebhookDeliveryAndLog(t *testing.T) {
	var mu sync.Mutex
	var got []map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		var m map[string]any
		_ = json.Unmarshal(b, &m)
		mu.Lock()
		got = append(got, m)
		mu.Unlock()
		if r.URL.Path == "/fail" {
			w.WriteHeader(500)
		}
	}))
	defer srv.Close()
	n, st, bus := newTestNotifier(t, []config.Notifier{
		{Name: "slack", Type: "slack", URL: srv.URL + "/slack"},                                      // default: alert.* only
		{Name: "hook", Type: "webhook", URL: srv.URL + "/hook", Events: []string{"*"}},               // every notifiable event
		{Name: "bad", Type: "teams", URL: srv.URL + "/fail", Events: []string{"alert.raised"}},       // fails
		{Name: "jobs", Type: "discord", URL: srv.URL + "/discord", Events: []string{"job.finished"}}, // not subscribed to alerts
	})
	ctx := context.Background()
	bus.Publish(model.Event{Type: "alert.raised", Data: &model.Alert{DeviceID: "dev-1", Severity: model.SeverityCritical, Description: "Paper jam", Source: "poll"}})
	bus.Publish(model.Event{Type: "device.updated", Data: map[string]any{"x": 1}}) // not notifiable: nobody gets it

	pending, _ := st.ListOutbox(ctx, "pending", 50)
	if len(pending) != 3 {
		t.Fatalf("expected 3 outbox rows (slack, hook, bad), got %d", len(pending))
	}
	n.Drain(ctx) // first attempt for all three
	logs, _ := st.ListNotificationLog(ctx, 10)
	if len(logs) != 3 {
		t.Fatalf("expected 3 delivery attempts, got %d", len(logs))
	}
	okCount := 0
	for _, l := range logs {
		if l.Notifier == "jobs" {
			t.Error("discord notifier is not subscribed to alert.raised")
		}
		if l.OK {
			okCount++
		} else if l.Notifier != "bad" || l.Error == "" {
			t.Errorf("unexpected failure record: %+v", l)
		}
	}
	if okCount != 2 {
		t.Errorf("ok deliveries = %d", okCount)
	}
	// Retries with backoff until dead, then a notification.dead event.
	var dead []events.Envelope
	bus.Subscribe(events.SubscriberFunc(func(e events.Envelope) {
		if e.Type == "notification.dead" {
			dead = append(dead, e)
		}
	}))
	for i := 0; i < 10 && len(dead) == 0; i++ {
		time.Sleep(10 * time.Millisecond)
		n.Drain(ctx)
	}
	rows, _ := st.ListOutbox(ctx, "dead", 10)
	if len(rows) != 1 || rows[0].Notifier != "bad" || rows[0].Attempts != 3 {
		t.Fatalf("dead rows = %+v", rows)
	}
	if len(dead) != 1 {
		t.Fatalf("notification.dead events = %d", len(dead))
	}
	// The dead event is itself notifiable, but never sent back to the failing destination.
	pending, _ = st.ListOutbox(ctx, "pending", 50)
	for _, p := range pending {
		if p.Notifier == "bad" {
			t.Errorf("dead notice queued to the failed destination: %+v", p)
		}
	}
	if err := st.RequeueDead(ctx, rows[0].ID); err != nil {
		t.Fatal(err)
	}
	mu.Lock()
	defer mu.Unlock()
	for _, m := range got {
		if txt, ok := m["text"].(string); ok && txt == "" {
			t.Error("empty text payload")
		}
	}
}
