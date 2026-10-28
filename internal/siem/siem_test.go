package siem

import (
	"bufio"
	"context"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"printmarshal/internal/config"
	"printmarshal/internal/events"
	"printmarshal/internal/model"
)

func testLogger() *slog.Logger { return slog.New(slog.NewTextHandler(io.Discard, nil)) }

func alertEnv() events.Envelope {
	return events.Envelope{ID: "e1", Version: 1, Type: "alert.raised", At: time.Date(2026, 10, 28, 21, 14, 2, 0, time.UTC),
		Origin: events.Origin{Source: "system"},
		Data:   &model.Alert{ID: "a1", DeviceID: "dev-1", Severity: model.SeverityCritical, Source: "poll", Key: "k", Description: `Toner "black" empty]`}}
}

func auditEnv() events.Envelope {
	return events.Envelope{ID: "e2", Version: 1, Type: "audit.recorded", At: time.Date(2026, 10, 28, 21, 14, 3, 0, time.UTC),
		Origin: events.Origin{Source: "operator", Actor: "kim", IP: "10.0.0.7"},
		Data:   map[string]any{"actor": "kim", "action": "device.delete", "target": "dev-1", "result": "ok"}}
}

func syslogCfg(addr, format string) struct {
	Address     string `json:"address"`
	Format      string `json:"format"`
	Facility    int    `json:"facility,omitempty"`
	TLSInsecure bool   `json:"tlsInsecure,omitempty"`
	CAFile      string `json:"caFile,omitempty"`
} {
	var c config.Export
	c.Syslog.Address, c.Syslog.Format, c.Syslog.Facility = addr, format, 16
	return c.Syslog
}

func TestSyslogUDPRFC5424(t *testing.T) {
	pc, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer pc.Close()
	sink, err := newSyslogSink(syslogCfg("udp://"+pc.LocalAddr().String(), "syslog"), "1.0")
	if err != nil {
		t.Fatal(err)
	}
	defer sink.Close()
	if err := sink.Write(alertEnv()); err != nil {
		t.Fatal(err)
	}
	buf := make([]byte, 65536)
	_ = pc.SetReadDeadline(time.Now().Add(3 * time.Second))
	n, _, err := pc.ReadFrom(buf)
	if err != nil {
		t.Fatal(err)
	}
	msg := string(buf[:n])
	// local0 (16) * 8 + critical (2) = 130
	if !strings.HasPrefix(msg, "<130>1 2026-10-28T21:14:02Z ") {
		t.Fatalf("bad header: %q", msg)
	}
	for _, want := range []string{" printmarshal " + strconv.Itoa(os.Getpid()) + " alert.raised ", `[printmarshal@32473 id="e1" type="alert.raised" source="system" deviceId="dev-1"]`, `"description":"Toner \"black\" empty]"`} {
		if !strings.Contains(msg, want) {
			t.Errorf("message lacks %q:\n%s", want, msg)
		}
	}
	st := sink.Status()
	if st.Sent != 1 || !st.OK || st.Target != "udp://"+pc.LocalAddr().String() {
		t.Errorf("status = %+v", st)
	}
}

func TestSyslogTCPOctetCounting(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	got := make(chan string, 2)
	go func() {
		c, err := ln.Accept()
		if err != nil {
			return
		}
		defer c.Close()
		r := bufio.NewReader(c)
		for i := 0; i < 2; i++ {
			lenStr, err := r.ReadString(' ')
			if err != nil {
				return
			}
			n, _ := strconv.Atoi(strings.TrimSpace(lenStr))
			body := make([]byte, n)
			if _, err := io.ReadFull(r, body); err != nil {
				return
			}
			got <- string(body)
		}
	}()
	sink, err := newSyslogSink(syslogCfg("tcp://"+ln.Addr().String(), "cef"), "1.0")
	if err != nil {
		t.Fatal(err)
	}
	defer sink.Close()
	if err := sink.Write(alertEnv()); err != nil {
		t.Fatal(err)
	}
	if err := sink.Write(auditEnv()); err != nil {
		t.Fatal(err)
	}
	for i, want := range []string{"CEF:0|PrintMarshal|PrintMarshal|1.0|alert.raised|An alert was raised|9|", "CEF:0|PrintMarshal|PrintMarshal|1.0|audit.recorded|An audit entry was written|1|"} {
		select {
		case m := <-got:
			if !strings.Contains(m, want) {
				t.Errorf("frame %d = %q, want it to contain %q", i, m, want)
			}
			if !strings.HasPrefix(m, "<1") || !strings.Contains(m, " - CEF:0|") {
				t.Errorf("frame %d lacks a syslog header before CEF: %q", i, m)
			}
		case <-time.After(3 * time.Second):
			t.Fatalf("frame %d not received", i)
		}
	}
}

func TestCEFFieldsAndEscaping(t *testing.T) {
	env := auditEnv()
	env.Data.(map[string]any)["target"] = "a=b|c\\d"
	rec := formatCEF(env, "2.0")
	for _, want := range []string{"rt=1793222043000", "externalId=e2", "cat=audit.recorded", "suser=kim", "src=10.0.0.7", "act=device.delete", "outcome=ok", `cs6=a\=b|c\\d`, "flexString1Label=data"} {
		if !strings.Contains(rec, want) {
			t.Errorf("CEF lacks %q:\n%s", want, rec)
		}
	}
	env.Type = "pipe|in|type"
	if !strings.Contains(formatCEF(env, "2.0"), `|pipe\|in\|type|`) {
		t.Error("header pipes not escaped")
	}
}

func TestFileSinkRotation(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "export.jsonl")
	s := &fileSink{path: path, maxSize: 600, keep: 2}
	if err := s.open(); err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	for i := 0; i < 12; i++ {
		if err := s.Write(auditEnv()); err != nil {
			t.Fatal(err)
		}
	}
	for _, p := range []string{path, path + ".1", path + ".2"} {
		if _, err := os.Stat(p); err != nil {
			t.Errorf("%s missing: %v", p, err)
		}
	}
	if _, err := os.Stat(path + ".3"); !os.IsNotExist(err) {
		t.Errorf("more than keep files were kept")
	}
	b, _ := os.ReadFile(path + ".1")
	for _, line := range strings.Split(strings.TrimSpace(string(b)), "\n") {
		if !strings.HasPrefix(line, `{"id":"e2","version":1,"type":"audit.recorded"`) {
			t.Errorf("bad line %q", line)
		}
	}
	if st := s.Status(); st.Sent != 12 || st.Failed != 0 {
		t.Errorf("status = %+v", st)
	}
}

func TestHTTPSinkBatchesWithBearer(t *testing.T) {
	var mu sync.Mutex
	var bodies []string
	var auths []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		mu.Lock()
		bodies = append(bodies, string(b))
		auths = append(auths, r.Header.Get("Authorization"))
		mu.Unlock()
		if r.Header.Get("Content-Type") != "application/x-ndjson" {
			w.WriteHeader(415)
		}
	}))
	defer srv.Close()
	t.Setenv("PM_TEST_EXPORT_TOKEN", "s3cret")
	var c config.Export
	c.HTTP.URL, c.HTTP.TokenEnv, c.HTTP.BatchSize = srv.URL+"/ingest", "PM_TEST_EXPORT_TOKEN", 2
	c.HTTP.FlushInterval = config.Duration(time.Hour)
	sink, err := newHTTPSink(c.HTTP, testLogger())
	if err != nil {
		t.Fatal(err)
	}
	_ = sink.Write(alertEnv())
	if st := sink.Status(); st.Pending != 1 || st.Sent != 0 {
		t.Fatalf("expected 1 pending before the batch fills, got %+v", st)
	}
	_ = sink.Write(auditEnv())
	mu.Lock()
	n := len(bodies)
	mu.Unlock()
	if n != 1 {
		t.Fatalf("expected one POST when the batch filled, got %d", n)
	}
	lines := strings.Split(strings.TrimSpace(bodies[0]), "\n")
	if len(lines) != 2 || !strings.Contains(lines[0], `"type":"alert.raised"`) || !strings.Contains(lines[1], `"type":"audit.recorded"`) {
		t.Errorf("bad batch: %q", bodies[0])
	}
	if auths[0] != "Bearer s3cret" {
		t.Errorf("authorization = %q", auths[0])
	}
	_ = sink.Write(alertEnv())
	if err := sink.Close(); err != nil { // close flushes regardless of the interval
		t.Fatal(err)
	}
	if st := sink.Status(); st.Sent != 3 || st.Pending != 0 {
		t.Errorf("status after close = %+v", st)
	}
}

func TestHTTPSinkRetriesThenDrops(t *testing.T) {
	var calls int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls++; w.WriteHeader(503) }))
	defer srv.Close()
	var c config.Export
	c.HTTP.URL, c.HTTP.BatchSize = srv.URL, 1
	sink, err := newHTTPSink(c.HTTP, testLogger())
	if err != nil {
		t.Fatal(err)
	}
	sink.Backoff = time.Millisecond
	if err := sink.Write(alertEnv()); err == nil {
		t.Fatal("expected an error after retries")
	}
	if calls != 3 {
		t.Errorf("attempts = %d, want 3", calls)
	}
	if st := sink.Status(); st.Failed != 1 || st.OK || !strings.Contains(st.LastError, "503") {
		t.Errorf("status = %+v", st)
	}
}

// memSink records envelopes for exporter tests.
type memSink struct {
	mu   sync.Mutex
	got  []events.Envelope
	slow chan struct{}
}

func (m *memSink) Name() string { return "mem" }
func (m *memSink) Write(env events.Envelope) error {
	if m.slow != nil {
		<-m.slow
	}
	m.mu.Lock()
	m.got = append(m.got, env)
	m.mu.Unlock()
	return nil
}
func (m *memSink) Flush() error       { return nil }
func (m *memSink) Close() error       { return nil }
func (m *memSink) Status() SinkStatus { return SinkStatus{Name: "mem", Type: "mem", OK: true} }
func (m *memSink) count() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.got)
}

func TestExporterFiltersAndDropsWhenFull(t *testing.T) {
	var c config.Export
	c.Buffer = 2
	e := New(c, nil, testLogger(), "t")
	sink := &memSink{}
	e.AddSink(sink)
	// not exported: not a security event, not alert.*, not notification.dead
	e.Handle(events.Envelope{ID: "x", Type: "device.updated"})
	e.Handle(events.Envelope{ID: "y", Type: "job.updated"})
	if st := e.Status(); st.Received != 0 || st.Queued != 0 {
		t.Fatalf("non-security events were queued: %+v", st)
	}
	// three exported events into a queue of two: one is dropped and counted
	for i := 0; i < 3; i++ {
		env := alertEnv()
		env.ID = "a" + strconv.Itoa(i)
		e.Handle(env)
	}
	e.Handle(events.Envelope{ID: "d", Type: "notification.dead"}) // dropped too
	st := e.Status()
	if st.Received != 2 || st.Dropped != 2 || st.Queued != 2 || st.QueueSize != 2 || !st.Enabled {
		t.Fatalf("status = %+v", st)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { e.Run(ctx); close(done) }()
	deadline := time.Now().Add(3 * time.Second)
	for sink.count() < 2 && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	if sink.count() != 2 {
		t.Fatalf("sink got %d events, want 2", sink.count())
	}
	cancel()
	<-done
	// After the worker has stopped, late events (server.stopping) are written synchronously.
	e.Handle(events.Envelope{ID: "stop", Type: "server.stopping"})
	if sink.count() != 3 {
		t.Errorf("sink got %d events after stop, want 3", sink.count())
	}
}

func TestExportedTypes(t *testing.T) {
	for _, typ := range []string{"auth.login", "auth.failed", "audit.recorded", "device.deleted", "config.applied", "alert.raised", "alert.cleared", "notification.dead", "server.stopping"} {
		if !Exported(typ) {
			t.Errorf("%s should be exported", typ)
		}
	}
	for _, typ := range []string{"device.updated", "device.discovered", "job.updated", "consumable.low", "notification.test"} {
		if Exported(typ) {
			t.Errorf("%s should not be exported", typ)
		}
	}
	if len(ExportedTypes()) == 0 {
		t.Error("no exported types")
	}
}
