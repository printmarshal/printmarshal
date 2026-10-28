package ricoh

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"printmarshal/internal/driver/snmp"
	"printmarshal/internal/model"
	"printmarshal/internal/plugins/vendorkit"
)

const fixtures = "../../../testdata/fixtures"

func load(t *testing.T, name string) *snmp.Fixture {
	t.Helper()
	fx, err := snmp.LoadFixture(filepath.Join(fixtures, name))
	if err != nil {
		t.Fatal(err)
	}
	return fx
}

func TestIdentifyIMC3010(t *testing.T) {
	p := New(vendorkit.Options{SNMP: vendorkit.FixtureDialer(load(t, "ricoh-imc3010.snmp"))})
	id, err := p.Identify(context.Background(), "10.0.0.5", model.Creds{})
	if err != nil {
		t.Fatal(err)
	}
	if id.Make != "Ricoh" || id.Model != "RICOH IM C3010" || id.Serial != "3101MC00042" || id.EnterpriseID != 367 || id.MAC != "00:26:73:4f:3a:1b" {
		t.Errorf("identity = %+v", id)
	}
	if id.DeviceType != "mfp" {
		t.Errorf("sysDescr names a scanner and facsimile: want mfp, got %s", id.DeviceType)
	}
	caps := map[model.Capability]bool{}
	for _, c := range id.Capabilities {
		caps[c] = true
	}
	if !caps[model.CapIdentify] || !caps[model.CapReadConfig] || !caps[model.CapReadCounters] || caps[model.CapReadSecurity] {
		t.Errorf("capabilities = %v", id.Capabilities)
	}
}

func TestBadges(t *testing.T) {
	for _, badge := range []string{"SAVIN", "LANIER", "Gestetner", "NASHUATEC", "Rex-Rotary", "NRG", "IKON"} {
		fx, err := snmp.ParseFixture(strings.NewReader(`
.1.3.6.1.2.1.1.1.0 STRING ` + badge + ` IM C3010 1.00
.1.3.6.1.2.1.1.2.0 OID .1.3.6.1.4.1.367.1.1
.1.3.6.1.2.1.25.3.2.1.2.1 OID .1.3.6.1.2.1.25.3.1.5
.1.3.6.1.2.1.25.3.2.1.3.1 STRING ` + badge + ` IM C3010
`))
		if err != nil {
			t.Fatal(err)
		}
		p := New(vendorkit.Options{SNMP: vendorkit.FixtureDialer(fx)})
		id, err := p.Identify(context.Background(), "10.0.0.5", model.Creds{})
		if err != nil {
			t.Fatalf("%s: %v", badge, err)
		}
		if id.Make != "Ricoh" || !strings.HasPrefix(id.Model, badge) {
			t.Errorf("%s: identity = %+v", badge, id)
		}
		doc, err := p.ReadConfig(context.Background(), &model.Device{Address: "x", Model: id.Model}, model.Creds{}, model.ConfigSelector{})
		if err != nil {
			t.Fatal(err)
		}
		if s, _ := doc.Get("vendor.ricoh.badge"); s == nil || !strings.EqualFold(s.Value.(string), badge) {
			t.Errorf("%s: badge setting = %+v", badge, s)
		}
		if s, _ := doc.Get("vendor.ricoh.family"); s == nil || s.Value != "IM C" {
			t.Errorf("%s: family = %+v", badge, s)
		}
	}
	// A badge word alone, with another vendor's enterprise number, never matches.
	fx, _ := snmp.ParseFixture(strings.NewReader(`
.1.3.6.1.2.1.1.1.0 STRING SAVIN badge on a foreign agent
.1.3.6.1.2.1.1.2.0 OID .1.3.6.1.4.1.99999.1
`))
	p := New(vendorkit.Options{SNMP: vendorkit.FixtureDialer(fx)})
	if _, err := p.Identify(context.Background(), "x", model.Creds{}); !errors.Is(err, vendorkit.ErrNotVendor) {
		t.Errorf("want ErrNotVendor, got %v", err)
	}
}

func TestRejectsOtherVendors(t *testing.T) {
	for _, f := range []string{"generic-laser.snmp", "color-mfp.snmp", "hp-m283fdw-enterprise.snmp", "hp-m283fdw.snmp"} {
		p := New(vendorkit.Options{SNMP: vendorkit.FixtureDialer(load(t, f))})
		if _, err := p.Identify(context.Background(), "10.0.0.1", model.Creds{}); !errors.Is(err, vendorkit.ErrNotVendor) {
			t.Errorf("%s: want ErrNotVendor, got %v", f, err)
		}
	}
}

func TestCounters(t *testing.T) {
	p := New(vendorkit.Options{SNMP: vendorkit.FixtureDialer(load(t, "ricoh-imc3010.snmp"))})
	c, err := p.ReadCounters(context.Background(), &model.Device{Address: "x"}, model.Creds{})
	if err != nil {
		t.Fatal(err)
	}
	if c.Total != 184230 || c.Mono == nil || *c.Mono != 42110+90118 || c.Color == nil || *c.Color != 18002+31000 || c.Duplex == nil || *c.Duplex != 61240 {
		t.Errorf("counters = %+v mono=%v color=%v duplex=%v", c, c.Mono, c.Color, c.Duplex)
	}
	if c.ByMarker["1"] != 184230 || c.PowerOn == nil || *c.PowerOn != 1203 {
		t.Errorf("standard counters not merged: %+v", c)
	}
}

func TestCountersAreGated(t *testing.T) {
	// No Total row: nothing is reported and read_counters is not declared.
	fx, _ := snmp.ParseFixture(strings.NewReader(`
.1.3.6.1.2.1.1.1.0 STRING RICOH IM 3500
.1.3.6.1.2.1.1.2.0 OID .1.3.6.1.4.1.367.1.1
.1.3.6.1.2.1.25.3.2.1.2.1 OID .1.3.6.1.2.1.25.3.1.5
.1.3.6.1.2.1.25.3.2.1.3.1 STRING RICOH IM 3500
.1.3.6.1.4.1.367.3.2.1.2.19.5.1.5.1 STRING Copier: B & W
.1.3.6.1.4.1.367.3.2.1.2.19.5.1.9.1 INTEGER 5
`))
	p := New(vendorkit.Options{SNMP: vendorkit.FixtureDialer(fx)})
	id, err := p.Identify(context.Background(), "x", model.Creds{})
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range id.Capabilities {
		if c == model.CapReadCounters {
			t.Error("read_counters must not be declared without a Total row")
		}
	}
	if _, err := p.ReadCounters(context.Background(), &model.Device{Address: "x"}, model.Creds{}); err == nil {
		t.Error("ReadCounters must fail without a Total row")
	}
	// Mono/colour larger than the total are dropped.
	rows := []snmp.PDU{
		{OID: oidEngCounterTable + ".5.1", Value: []byte("Total")}, {OID: oidEngCounterTable + ".9.1", Value: 100},
		{OID: oidEngCounterTable + ".5.2", Value: []byte("Printer: Color")}, {OID: oidEngCounterTable + ".9.2", Value: 90},
		{OID: oidEngCounterTable + ".5.3", Value: []byte("Printer: B & W")}, {OID: oidEngCounterTable + ".9.3", Value: 90},
	}
	for i := range rows {
		if _, ok := rows[i].Value.([]byte); ok {
			rows[i].Type = 4 // OctetString
		} else {
			rows[i].Type = 2 // Integer
		}
	}
	c := ParseCounters(rows, time.Now())
	if c == nil || c.Total != 100 || c.Mono != nil || c.Color != nil {
		t.Errorf("gated counters = %+v", c)
	}
}

func TestFamily(t *testing.T) {
	cases := map[string]string{
		"RICOH IM C3010": "IM C", "SAVIN IM 3500": "IM", "RICOH MP C3004ex": "MP C", "RICOH Pro C7500": "Pro C",
		"RICOH P C600": "P C", "RICOH IM 460F": "IM", "LANIER SP 5300DN": "SP", "RICOH Aficio MP 2501": "Aficio", "Unknown": "",
	}
	for in, want := range cases {
		if got := family(in); got != want {
			t.Errorf("family(%q) = %q, want %q", in, got, want)
		}
	}
}
