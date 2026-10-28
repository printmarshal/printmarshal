package siem

import (
	"context"
	"net"
	"strings"
	"testing"
	"time"

	"printmarshal/internal/config"
	"printmarshal/internal/model"
	"printmarshal/internal/secrets"
	"printmarshal/internal/store"
)

func TestParseSyslog(t *testing.T) {
	cases := []struct {
		in       string
		fac, sev int
		msg      string
	}{
		{`<34>1 2026-10-28T21:14:02.003Z printer-7 lpd 1234 ID47 [exampleSDID@32473 iut="3" eventSource="App\]"] \xef\xbb\xbfJob 42 printed`, 4, 2, "lpd: Job 42 printed"},
		{`<165>1 2026-10-28T21:14:02Z host app - - - no structured data`, 20, 5, "app: no structured data"},
		{`<13>Oct 28 21:14:02 printer-7 lpd[123]: paper jam tray 2`, 1, 5, "lpd[123]: paper jam tray 2"},
		{`<6>just a message`, 0, 6, "just a message"},
		{"no priority at all\n", 1, 5, "no priority at all"},
		{"<999>bad pri", 1, 5, "<999>bad pri"},
	}
	for _, c := range cases {
		in := strings.ReplaceAll(c.in, `\xef\xbb\xbf`, "\xef\xbb\xbf")
		l := ParseSyslog(in)
		if l.Facility != c.fac || l.Severity != c.sev || l.Message != c.msg {
			t.Errorf("%q -> fac=%d sev=%d msg=%q; want %d %d %q", c.in, l.Facility, l.Severity, l.Message, c.fac, c.sev, c.msg)
		}
	}
	long := ParseSyslog("<6>" + strings.Repeat("x", 5000))
	if len(long.Message) != maxMessage {
		t.Errorf("message not bounded: %d", len(long.Message))
	}
	if ctl := ParseSyslog("<6>a\x01b\x7fc"); ctl.Message != "a b\x7fc" && !strings.Contains(ctl.Message, "a b") {
		t.Errorf("control chars not stripped: %q", ctl.Message)
	}
}

func openStore(t *testing.T) *store.SQLite {
	t.Helper()
	box, _ := secrets.New(make([]byte, 32))
	st, err := store.OpenSQLite(t.TempDir()+"/t.db", box)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	return st
}

func TestIngestStoresLinesFromKnownDevicesOnly(t *testing.T) {
	st := openStore(t)
	ctx := context.Background()
	dev, _, err := st.UpsertDevice(ctx, &model.Identity{Address: "127.0.0.1", Serial: "S1", Model: "Acme 1", Method: "snmp"}, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	var c config.Export
	c.IngestListen, c.IngestRatePerMinute = "127.0.0.1:0", 3
	in := NewIngest(c, st, st, testLogger())

	in.HandleLine(ctx, "10.9.9.9", "<13>Oct 28 21:14:02 ghost lpd: hello")
	if s := in.Status(); s.Unknown != 1 || s.Stored != 0 {
		t.Fatalf("unknown source should not be stored: %+v", s)
	}
	rows, _ := st.ListDeviceLogs(ctx, dev.ID, 10)
	if len(rows) != 0 {
		t.Fatalf("rows for unknown source: %d", len(rows))
	}

	// Live UDP path: run the listener on an ephemeral port and send a datagram.
	rctx, cancel := context.WithCancel(ctx)
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- in.Run(rctx) }()
	deadline := time.Now().Add(3 * time.Second)
	for in.Addr() == "" && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	if in.Addr() == "" {
		t.Fatal("listener did not bind")
	}
	conn, err := net.Dial("udp", in.Addr())
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	if _, err := conn.Write([]byte(`<34>1 2026-10-28T21:14:02Z printer-7 lpd - - - Toner cartridge missing`)); err != nil {
		t.Fatal(err)
	}
	for in.Status().Stored < 1 && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	rows, err = st.ListDeviceLogs(ctx, dev.ID, 10)
	if err != nil || len(rows) != 1 {
		t.Fatalf("rows = %d err=%v", len(rows), err)
	}
	if rows[0].Facility != 4 || rows[0].Severity != 2 || rows[0].Message != "lpd: Toner cartridge missing" || rows[0].SeverityName() != "critical" {
		t.Errorf("row = %+v", rows[0])
	}
	cancel()
	if err := <-done; err != nil {
		t.Errorf("run: %v", err)
	}

	// Rate limit per source: 3 per minute including the one above.
	in.HandleLine(ctx, "127.0.0.1", "<6>two")
	in.HandleLine(ctx, "127.0.0.1", "<6>three")
	in.HandleLine(ctx, "127.0.0.1", "<6>four (dropped)")
	s := in.Status()
	if s.Stored != 3 || s.Dropped != 1 {
		t.Errorf("rate limit: %+v", s)
	}
	if n, err := st.PruneDeviceLogs(ctx, time.Now().Add(time.Minute)); err != nil || n != 3 {
		t.Errorf("prune = %d err=%v", n, err)
	}
}
