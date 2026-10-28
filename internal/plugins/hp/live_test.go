package hp

import (
	"context"
	"os"
	"testing"
	"time"

	"printmarshal/internal/driver/ipp"
	"printmarshal/internal/driver/snmp"
	"printmarshal/internal/model"
	"printmarshal/internal/plugins/vendorkit"
)

// TestLiveHP exercises the plugin read-only against a real HP device when
// PM_LIVE_HP=<address> is set (e.g. the Color LaserJet MFP M283fdw used for
// this build's validation). It only issues SNMP GET/walk, IPP
// Get-Printer-Attributes and HTTP GET; it never writes.
func TestLiveHP(t *testing.T) {
	addr := os.Getenv("PM_LIVE_HP")
	if addr == "" {
		t.Skip("set PM_LIVE_HP=<address> to run against real hardware (read-only)")
	}
	opts := vendorkit.Options{
		SNMP: snmp.NewDialer(snmp.DefaultOptions()),
		IPP:  ipp.NewTransport(ipp.DefaultOptions()),
		HTTP: vendorkit.NewHTTP(vendorkit.DefaultHTTPOptions()),
	}
	p := New(opts)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	creds := model.Creds{Communities: []string{os.Getenv("PM_LIVE_COMMUNITY")}}
	if creds.Communities[0] == "" {
		creds.Communities = nil
	}
	id, err := p.Identify(ctx, addr, creds)
	if err != nil {
		t.Fatalf("identify: %v", err)
	}
	t.Logf("identity: make=%q model=%q serial=%q firmware=%q type=%s caps=%v", id.Make, id.Model, id.Serial, id.Firmware, id.DeviceType, id.Capabilities)
	if id.Make != "HP" || id.Serial == "" || id.Firmware == "" {
		t.Errorf("identity incomplete: %+v", id)
	}
	dev := &model.Device{ID: "live", Address: addr, Make: id.Make, Model: id.Model, EnterpriseID: id.EnterpriseID}
	c, err := p.ReadCounters(ctx, dev, creds)
	if err != nil {
		t.Fatalf("counters: %v", err)
	}
	t.Logf("counters: total=%d mono=%v color=%v duplex=%v", c.Total, deref(c.Mono), deref(c.Color), deref(c.Duplex))
	if c.Total <= 0 {
		t.Error("total must be positive")
	}
	st, err := p.ReadStatus(ctx, dev, creds)
	if err != nil {
		t.Fatalf("status: %v", err)
	}
	t.Logf("status: state=%s severity=%s message=%q alerts=%d", st.State, st.Severity, st.Message, len(st.Alerts))
	fw, err := p.ReadFirmware(ctx, dev, creds)
	if err != nil {
		t.Fatalf("firmware: %v", err)
	}
	t.Logf("firmware: %s", fw)
	doc, err := p.ReadConfig(ctx, dev, creds, model.ConfigSelector{})
	if err != nil {
		t.Fatalf("config: %v", err)
	}
	for _, s := range doc.Settings {
		t.Logf("config: %-36s = %v", s.Key, s.Value)
		if s.Writable {
			t.Errorf("%s reported writable; this plugin is read-only", s.Key)
		}
	}
	if len(doc.Settings) < 5 {
		t.Errorf("config document too small: %d settings", len(doc.Settings))
	}
}

func deref(p *int64) any {
	if p == nil {
		return "n/a"
	}
	return *p
}
