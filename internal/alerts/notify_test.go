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
	"printmarshal/internal/secrets"
	"printmarshal/internal/store"
)

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
	box, _ := secrets.New(make([]byte, 32))
	st, err := store.OpenSQLite(t.TempDir()+"/t.db", box)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	n := NewNotifier([]config.Notifier{
		{Name: "slack", Type: "slack", URL: srv.URL + "/slack"},                                   // default: alert.* only
		{Name: "hook", Type: "webhook", URL: srv.URL + "/hook", Events: []string{"*"}},             // everything
		{Name: "bad", Type: "teams", URL: srv.URL + "/fail", Events: []string{"alert.raised"}},     // fails
		{Name: "jobs", Type: "discord", URL: srv.URL + "/discord", Events: []string{"job.finished"}}, // not subscribed to alerts
	}, st, slog.New(slog.NewTextHandler(io.Discard, nil)))
	n.Notify("alert.raised", "[CRITICAL] LP-1: Paper jam", "LP-1 (10.0.0.5)\nPaper jam", map[string]any{"device": "LP-1", "secret": "should-not-matter"})

	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		logs, _ := st.ListNotificationLog(context.Background(), 10)
		if len(logs) == 3 {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	logs, _ := st.ListNotificationLog(context.Background(), 10)
	if len(logs) != 3 {
		t.Fatalf("expected 3 delivery attempts (slack, hook, bad), got %d", len(logs))
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
	mu.Lock()
	defer mu.Unlock()
	for _, m := range got {
		if txt, ok := m["text"].(string); ok && txt == "" {
			t.Error("empty text payload")
		}
	}
}
