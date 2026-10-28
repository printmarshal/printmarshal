package discovery

import (
	"net"
	"testing"

	"github.com/miekg/dns"
)

func TestParseMDNSResponse(t *testing.T) {
	inst := "HP\\032Color\\032LaserJet\\032MFP\\032M283fdw\\032(0A1B2C)-4._ipp._tcp.local."
	m := new(dns.Msg)
	m.Response = true
	m.Answer = []dns.RR{
		&dns.PTR{Hdr: dns.RR_Header{Name: "_ipp._tcp.local.", Rrtype: dns.TypePTR, Class: dns.ClassINET}, Ptr: inst},
	}
	m.Extra = []dns.RR{
		&dns.SRV{Hdr: dns.RR_Header{Name: inst, Rrtype: dns.TypeSRV, Class: dns.ClassINET}, Port: 631, Target: "NPI0A1B2C.local."},
		&dns.TXT{Hdr: dns.RR_Header{Name: inst, Rrtype: dns.TypeTXT, Class: dns.ClassINET}, Txt: []string{"txtvers=1", "rp=ipp/print", "ty=HP ColorLaserJet MFP M282-M285", "UUID=564e4230-5445-5354-3031-02005e0a1b2c", "Scan=T", "usb_MFG=HP"}},
		&dns.A{Hdr: dns.RR_Header{Name: "NPI0A1B2C.local.", Rrtype: dns.TypeA, Class: dns.ClassINET}, A: net.IPv4(198, 51, 100, 5)},
	}
	b, err := m.Pack()
	if err != nil {
		t.Fatal(err)
	}
	var resp dns.Msg
	if err := resp.Unpack(b); err != nil {
		t.Fatal(err)
	}
	cands := parseMDNS(&resp, net.IPv4(198, 51, 100, 5))
	if len(cands) != 1 {
		t.Fatalf("candidates = %+v", cands)
	}
	c := cands[0]
	if c.Address != "198.51.100.5" || c.Hostname != "NPI0A1B2C.local" || c.Services[0] != "_ipp._tcp" {
		t.Errorf("candidate = %+v", c)
	}
	if c.TXT["ty"] != "HP ColorLaserJet MFP M282-M285" || c.TXT["uuid"] != "564e4230-5445-5354-3031-02005e0a1b2c" || c.TXT["usb_mfg"] != "HP" {
		t.Errorf("txt = %v", c.TXT)
	}
	if c.TXT["instance"] != "HP Color LaserJet MFP M283fdw (0A1B2C)-4" {
		t.Errorf("instance = %q", c.TXT["instance"])
	}
	// Without an A record the sender's address is used.
	resp.Extra = resp.Extra[:2]
	cands = parseMDNS(&resp, net.IPv4(10, 0, 0, 9))
	if len(cands) != 1 || cands[0].Address != "10.0.0.9" {
		t.Errorf("fallback = %+v", cands)
	}
}
