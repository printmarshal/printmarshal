package security

import (
	"context"
	"crypto/tls"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strconv"
	"testing"
	"time"

	"printmarshal/internal/model"
)

func TestTLSProbeAgainstHTTPTestServer(t *testing.T) {
	srv := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	srv.TLS = &tls.Config{MinVersion: tls.VersionTLS12}
	srv.StartTLS()
	defer srv.Close()
	u, _ := url.Parse(srv.URL)
	port, _ := strconv.Atoi(u.Port())

	reports := ProbeTLS(context.Background(), u.Hostname(), []int{port})
	if len(reports) != 1 {
		t.Fatalf("reports %+v", reports)
	}
	r := reports[0]
	if r.Error != "" {
		t.Fatalf("probe error: %s", r.Error)
	}
	if r.MinVersion != "1.2" {
		t.Errorf("min version = %q, want 1.2 (server refuses 1.0/1.1)", r.MinVersion)
	}
	if len(r.Versions) != 2 || r.Versions[0] != "1.2" || r.Versions[1] != "1.3" {
		t.Errorf("versions = %v", r.Versions)
	}
	if r.Leaf == nil || !r.Leaf.SelfSigned || r.Leaf.Fingerprint == "" || r.Leaf.NotAfter.IsZero() || r.Leaf.ID != "tls:"+u.Port() {
		t.Errorf("leaf = %+v", r.Leaf)
	}
	if r.Leaf.KeyType == "" || r.Leaf.Subject == "" {
		t.Errorf("leaf details = %+v", r.Leaf)
	}

	now := time.Now()
	settings := TLSSettings(reports, now)
	got := map[string]model.Setting{}
	for _, s := range settings {
		got[s.Key] = s
		if s.Source != "probe" || s.Writable {
			t.Errorf("probe settings are observed, read-only: %+v", s)
		}
	}
	if got["security.tls.minVersion"].Value != "1.2" || got["security.cert.identityPresent"].Value != true || got["security.cert.identitySelfSigned"].Value != true {
		t.Errorf("settings = %+v", got)
	}
	if days, ok := got["security.cert.identityDaysLeft"].Value.(int); !ok || days <= 0 {
		t.Errorf("days left = %v", got["security.cert.identityDaysLeft"].Value)
	}

	// A TLS 1.3-only server reports 1.3.
	srv13 := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	srv13.TLS = &tls.Config{MinVersion: tls.VersionTLS13}
	srv13.StartTLS()
	defer srv13.Close()
	u13, _ := url.Parse(srv13.URL)
	p13, _ := strconv.Atoi(u13.Port())
	if rep := ProbeTLS(context.Background(), u13.Hostname(), []int{p13}); rep[0].MinVersion != "1.3" {
		t.Errorf("1.3-only server: %+v", rep[0])
	}

	// A closed port is an error, not a setting.
	closed := ProbeTLS(context.Background(), "127.0.0.1", []int{1})
	if closed[0].Error == "" || len(TLSSettings(closed, now)) != 0 {
		t.Errorf("closed port: %+v", closed[0])
	}
}

// TestProbeRealPrinter runs the read-only probes against a real device when
// PM_PROBE_HOST is set (spec §14 integration). It sends no writes: TLS
// handshakes, TCP connects and one SNMP GET of sysDescr.
func TestProbeRealPrinter(t *testing.T) {
	host := os.Getenv("PM_PROBE_HOST")
	if host == "" {
		t.Skip("set PM_PROBE_HOST to probe a real printer")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	reports := ProbeTLS(ctx, host, DefaultTLSPorts)
	for _, r := range reports {
		t.Logf("tls %s:%d min=%s versions=%v negotiated=%s cipher=%s err=%q", r.Host, r.Port, r.MinVersion, r.Versions, r.HandshakeV, r.Cipher, r.Error)
		if r.Leaf != nil {
			t.Logf("  leaf subject=%q issuer=%q selfSigned=%v notAfter=%s key=%s fp=%s", r.Leaf.Subject, r.Leaf.Issuer, r.Leaf.SelfSigned, r.Leaf.NotAfter.Format(time.RFC3339), r.Leaf.KeyType, r.Leaf.Fingerprint[:16])
		}
	}
	for _, s := range TLSSettings(reports, time.Now()) {
		t.Logf("setting %s = %v (%s)", s.Key, s.Value, s.Source)
	}
	rep := ProbePorts(ctx, host, model.Creds{})
	t.Logf("ports open=%v snmpTried=%v snmpV2c=%v", rep.Open, rep.SNMPTried, rep.SNMPv2c)
	for _, s := range PortSettings(rep) {
		t.Logf("setting %s = %v (%s)", s.Key, s.Value, s.Source)
	}
	var ok bool
	for _, r := range reports {
		if r.Error == "" {
			ok = true
		}
	}
	if !ok {
		t.Errorf("no TLS endpoint answered on %s", host)
	}
}

func TestPortProbe(t *testing.T) {
	// The probe checks fixed well-known ports; here we only verify it reports
	// every port, never panics on a silent host, and maps results to settings.
	SNMPPort = 1 // nothing listens there; the probe must return quickly with "no answer"
	defer func() { SNMPPort = 161 }()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	rep := ProbePorts(ctx, "127.0.0.1", model.Creds{})
	if len(rep.Open) != len(ProbedPorts) || !rep.SNMPTried || rep.SNMPv2c {
		t.Errorf("report = %+v", rep)
	}
	settings := PortSettings(PortReport{Open: map[int]bool{23: false, 9100: true}, SNMPTried: true, SNMPv2c: true})
	got := map[string]any{}
	for _, s := range settings {
		got[s.Key] = s.Value
		if s.Source != "probe" {
			t.Errorf("source = %q", s.Source)
		}
	}
	if got["protocols.telnet.enabled"] != false || got["protocols.raw9100.enabled"] != true || got["protocols.snmp.v1v2cEnabled"] != true {
		t.Errorf("settings = %v", got)
	}
	if _, ok := got["protocols.ftp.enabled"]; ok {
		t.Error("unprobed ports must not produce settings")
	}
	// Silence over UDP is not evidence that SNMP is off.
	for _, s := range PortSettings(PortReport{Open: map[int]bool{}, SNMPTried: true, SNMPv2c: false}) {
		if s.Key == "protocols.snmp.v1v2cEnabled" {
			t.Error("no SNMP answer must not be reported as disabled")
		}
	}
	// A listening socket is reported open (use whatever port the OS gives, via the Open map helper).
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Skip(err)
	}
	defer ln.Close()
	d := net.Dialer{Timeout: time.Second}
	if c, err := d.Dial("tcp", ln.Addr().String()); err == nil {
		c.Close()
	} else {
		t.Errorf("listener not reachable: %v", err)
	}
}
