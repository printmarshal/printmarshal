package api

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"testing"

	"printmarshal/internal/config"
	"printmarshal/internal/events"
	"printmarshal/internal/model"
	"printmarshal/internal/siem"
)

type nullSink struct{ n int }

func (s *nullSink) Name() string                { return "null" }
func (s *nullSink) Write(events.Envelope) error { s.n++; return nil }
func (s *nullSink) Flush() error                { return nil }
func (s *nullSink) Close() error                { return nil }
func (s *nullSink) Status() siem.SinkStatus {
	return siem.SinkStatus{Name: "null", Type: "test", Target: "memory", OK: true, Sent: int64(s.n)}
}
func discard() *slog.Logger { return slog.New(slog.NewTextHandler(io.Discard, nil)) }
func decode(t *testing.T, body string, dst any) {
	t.Helper()
	if err := json.Unmarshal([]byte(body), dst); err != nil {
		t.Fatalf("decode %q: %v", body, err)
	}
}

func TestExportStatusEndpoint(t *testing.T) {
	ts, _ := newTestServer(t)
	admin := login(t, ts, "admin", "adminpass123")
	code, body := do(t, ts, admin, "GET", "/api/export/status", "")
	if code != 200 || !strings.Contains(body, `"enabled":false`) || !strings.Contains(body, `"syncCollections"`) {
		t.Fatalf("status without exporter: %d %s", code, body)
	}
	e := siem.New(config.Default().Export, nil, discard(), "test")
	e.AddSink(&nullSink{})
	SetExport(e)
	t.Cleanup(func() { SetExport(nil) })
	code, body = do(t, ts, admin, "GET", "/api/export/status", "")
	var st siem.Status
	decode(t, body, &st)
	if code != 200 || !st.Enabled || len(st.Sinks) != 1 || st.Sinks[0].Target != "memory" || st.QueueSize != 4096 {
		t.Fatalf("status with exporter: %d %s", code, body)
	}
	if code, _ := do(t, ts, "", "GET", "/api/export/status", ""); code != 401 {
		t.Errorf("unauthenticated = %d", code)
	}
}

func TestSyncWalkAndTombstones(t *testing.T) {
	ts, s := newTestServer(t)
	admin := login(t, ts, "admin", "adminpass123")
	viewer := login(t, ts, "viewer", "viewerpass1")
	ctx := context.Background()
	var ids []string
	for _, addr := range []string{"10.0.0.1", "10.0.0.2", "10.0.0.3"} {
		d, _, err := s.Store.UpsertDevice(ctx, &model.Identity{Address: addr, Serial: "S" + addr, Model: "Acme", Method: "snmp"}, nil, nil)
		if err != nil {
			t.Fatal(err)
		}
		ids = append(ids, d.ID)
	}
	type page struct {
		Collection string            `json:"collection"`
		Items      []json.RawMessage `json:"items"`
		Cursor     string            `json:"cursor"`
		HasMore    bool              `json:"hasMore"`
	}
	var got []string
	cursor := ""
	for i := 0; i < 10; i++ {
		code, body := do(t, ts, viewer, "GET", "/api/sync/devices?limit=1&since="+cursor, "")
		if code != 200 {
			t.Fatalf("sync: %d %s", code, body)
		}
		var p page
		decode(t, body, &p)
		for _, it := range p.Items {
			var d struct {
				ID string `json:"id"`
			}
			decode(t, string(it), &d)
			got = append(got, d.ID)
		}
		cursor = p.Cursor
		if !p.HasMore {
			break
		}
	}
	if len(got) != 3 {
		t.Fatalf("walk returned %v", got)
	}
	// Nothing new: same cursor, empty page, and the ETag lets the client get a 304.
	req, _ := http.NewRequest("GET", ts.URL+"/api/sync/devices?since="+cursor, nil)
	req.Header.Set("Authorization", "Bearer "+viewer)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	etag := resp.Header.Get("ETag")
	b, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	var p page
	decode(t, string(b), &p)
	if len(p.Items) != 0 || p.HasMore || p.Cursor != cursor || etag == "" {
		t.Fatalf("empty page: %s etag=%q", b, etag)
	}
	req.Header.Set("If-None-Match", etag)
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != 304 {
		t.Errorf("If-None-Match = %d", resp.StatusCode)
	}
	// Delete through the API: the next page carries a tombstone.
	if code, body := do(t, ts, admin, "DELETE", "/api/devices/"+ids[1], ""); code != 200 {
		t.Fatalf("delete: %d %s", code, body)
	}
	_, body := do(t, ts, viewer, "GET", "/api/sync/devices?since="+cursor, "")
	decode(t, body, &p)
	if len(p.Items) != 1 || !strings.Contains(string(p.Items[0]), `"deleted":true`) || !strings.Contains(string(p.Items[0]), `"id":"`+ids[1]+`"`) {
		t.Fatalf("tombstone page: %s", body)
	}
	// Bad cursor and unknown collection.
	if code, _ := do(t, ts, viewer, "GET", "/api/sync/devices?since=%21%21", ""); code != 400 {
		t.Errorf("bad cursor = %d", code)
	}
	if code, _ := do(t, ts, viewer, "GET", "/api/sync/nothing", ""); code != 404 {
		t.Errorf("unknown collection = %d", code)
	}
	// Audit is operator-only, like GET /api/audit.
	if code, _ := do(t, ts, viewer, "GET", "/api/sync/audit", ""); code != 403 {
		t.Errorf("viewer audit sync = %d", code)
	}
	code, body := do(t, ts, admin, "GET", "/api/sync/audit", "")
	if code != 200 || !strings.Contains(body, `"action":"device.delete"`) {
		t.Errorf("admin audit sync: %d %s", code, body)
	}
	for _, c := range []string{"alerts", "jobs", "config-writes"} {
		if code, body := do(t, ts, viewer, "GET", "/api/sync/"+c, ""); code != 200 || !strings.Contains(body, `"collection":"`+c+`"`) {
			t.Errorf("%s: %d %s", c, code, body)
		}
	}
}

func TestDeviceLogsEndpoint(t *testing.T) {
	ts, s := newTestServer(t)
	viewer := login(t, ts, "viewer", "viewerpass1")
	ctx := context.Background()
	d, _, _ := s.Store.UpsertDevice(ctx, &model.Identity{Address: "10.0.0.5", Serial: "S1", Model: "Acme", Method: "snmp"}, nil, nil)
	if err := s.logStore().AddDeviceLog(ctx, &model.DeviceLog{DeviceID: d.ID, Facility: 4, Severity: 2, Message: "Toner missing"}); err != nil {
		t.Fatal(err)
	}
	code, body := do(t, ts, viewer, "GET", "/api/devices/"+d.ID+"/logs?limit=10", "")
	if code != 200 || !strings.Contains(body, `"message":"Toner missing"`) || !strings.Contains(body, `"severityName":"critical"`) || !strings.Contains(body, `"facilityName":"auth"`) {
		t.Fatalf("logs: %d %s", code, body)
	}
	if code, _ := do(t, ts, viewer, "GET", "/api/devices/nope/logs", ""); code != 404 {
		t.Errorf("unknown device = %d", code)
	}
}
