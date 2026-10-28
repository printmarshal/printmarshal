package trap

import (
	"context"
	"io"
	"log/slog"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/gosnmp/gosnmp"

	"printmarshal/internal/model"
)

type recorder struct {
	mu   sync.Mutex
	got  []string
	sevs []model.Severity
}

func (r *recorder) HandleTrap(_ context.Context, src, desc string, sev model.Severity, _ map[string]string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.got = append(r.got, src+" "+desc)
	r.sevs = append(r.sevs, sev)
}

func TestReceivesPrinterAlertTrap(t *testing.T) {
	rec := &recorder{}
	l := New("127.0.0.1:0", rec, slog.New(slog.NewTextHandler(io.Discard, nil)))
	// Pick a free UDP port first since the listener needs a concrete address.
	pc, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := pc.LocalAddr().String()
	pc.Close()
	l.addr = addr
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { _ = l.Run(ctx) }()
	time.Sleep(200 * time.Millisecond)

	host, port, _ := net.SplitHostPort(addr)
	var p uint16
	_, _ = net.LookupPort("udp", port)
	for _, c := range port {
		p = p*10 + uint16(c-'0')
	}
	g := &gosnmp.GoSNMP{Target: host, Port: p, Community: "public", Version: gosnmp.Version2c, Timeout: time.Second, Retries: 0}
	if err := g.Connect(); err != nil {
		t.Fatal(err)
	}
	defer g.Close()
	trap := gosnmp.SnmpTrap{Variables: []gosnmp.SnmpPDU{
		{Name: ".1.3.6.1.2.1.1.3.0", Type: gosnmp.TimeTicks, Value: uint32(1000)},
		{Name: ".1.3.6.1.6.3.1.1.4.1.0", Type: gosnmp.ObjectIdentifier, Value: ".1.3.6.1.2.1.43.18.2.0.1"},
		{Name: ".1.3.6.1.2.1.43.18.1.1.2.1.3", Type: gosnmp.Integer, Value: 3},
		{Name: ".1.3.6.1.2.1.43.18.1.1.4.1.3", Type: gosnmp.Integer, Value: 6},
		{Name: ".1.3.6.1.2.1.43.18.1.1.7.1.3", Type: gosnmp.Integer, Value: 3},
		{Name: ".1.3.6.1.2.1.43.18.1.1.8.1.3", Type: gosnmp.OctetString, Value: "Front cover open"},
	}}
	if _, err := g.SendTrap(trap); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		rec.mu.Lock()
		n := len(rec.got)
		rec.mu.Unlock()
		if n > 0 {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	rec.mu.Lock()
	defer rec.mu.Unlock()
	if len(rec.got) != 1 || rec.got[0] != "127.0.0.1 Front cover open" || rec.sevs[0] != model.SeverityCritical {
		t.Fatalf("trap not decoded: %v %v", rec.got, rec.sevs)
	}
}
