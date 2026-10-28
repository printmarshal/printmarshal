package canon

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"

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

func TestIdentifyAndCounters(t *testing.T) {
	p := New(vendorkit.Options{SNMP: vendorkit.FixtureDialer(load(t, "canon-irdxc3930i.snmp"))})
	id, err := p.Identify(context.Background(), "10.0.0.7", model.Creds{})
	if err != nil {
		t.Fatal(err)
	}
	if id.Make != "Canon" || id.Model != "Canon imageRUNNER ADVANCE DX C3930i" || id.Serial != "2YK01234" || id.EnterpriseID != 1602 {
		t.Errorf("identity = %+v", id)
	}
	hasCounters := false
	for _, c := range id.Capabilities {
		if c == model.CapReadCounters {
			hasCounters = true
		}
	}
	if !hasCounters {
		t.Errorf("capabilities = %v", id.Capabilities)
	}
	c, err := p.ReadCounters(context.Background(), &model.Device{Address: "x"}, model.Creds{})
	if err != nil {
		t.Fatal(err)
	}
	if c.Total != 77410 || c.Mono == nil || *c.Mono != 50100 || c.Color == nil || *c.Color != 27310 || c.Duplex == nil || *c.Duplex != 20005 {
		t.Errorf("counters = %+v", c)
	}
	doc, err := p.ReadConfig(context.Background(), &model.Device{Address: "x", Model: id.Model}, model.Creds{}, model.ConfigSelector{})
	if err != nil {
		t.Fatal(err)
	}
	if s, _ := doc.Get("vendor.canon.family"); s == nil || s.Value != "imageRUNNER ADVANCE DX" {
		t.Errorf("family = %+v", s)
	}
}

func TestCountersGated(t *testing.T) {
	fx, _ := snmp.ParseFixture(strings.NewReader(`
.1.3.6.1.2.1.1.1.0 STRING Canon imageCLASS MF644Cdw
.1.3.6.1.2.1.1.2.0 OID .1.3.6.1.4.1.1602.4.7
.1.3.6.1.2.1.25.3.2.1.2.1 OID .1.3.6.1.2.1.25.3.1.5
.1.3.6.1.2.1.25.3.2.1.3.1 STRING Canon imageCLASS MF644Cdw
.1.3.6.1.4.1.1602.1.11.1.3.1.4.108 COUNTER32 50
`))
	p := New(vendorkit.Options{SNMP: vendorkit.FixtureDialer(fx)})
	id, err := p.Identify(context.Background(), "x", model.Creds{})
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range id.Capabilities {
		if c == model.CapReadCounters {
			t.Error("read_counters declared without a Total 1 row")
		}
	}
	if _, err := p.ReadCounters(context.Background(), &model.Device{Address: "x"}, model.Creds{}); err == nil {
		t.Error("ReadCounters must fail without a Total 1 row")
	}
}

func TestRejectsOtherVendors(t *testing.T) {
	for _, f := range []string{"generic-laser.snmp", "hp-m283fdw-enterprise.snmp", "ricoh-imc3010.snmp"} {
		p := New(vendorkit.Options{SNMP: vendorkit.FixtureDialer(load(t, f))})
		if _, err := p.Identify(context.Background(), "10.0.0.1", model.Creds{}); !errors.Is(err, vendorkit.ErrNotVendor) {
			t.Errorf("%s: want ErrNotVendor, got %v", f, err)
		}
	}
}
