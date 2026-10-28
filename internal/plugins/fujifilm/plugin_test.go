package fujifilm

import (
	"context"
	"errors"
	"path/filepath"
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

func TestIdentify(t *testing.T) {
	p := New(vendorkit.Options{SNMP: vendorkit.FixtureDialer(load(t, "fujifilm-apeos-c3570.snmp"))})
	id, err := p.Identify(context.Background(), "10.0.0.9", model.Creds{})
	if err != nil {
		t.Fatal(err)
	}
	if id.Make != "Fujifilm" || id.Model != "FUJIFILM Apeos C3570" || id.Serial != "400123" || id.EnterpriseID != 297 || id.MAC != "08:00:37:a1:b2:c3" {
		t.Errorf("identity = %+v", id)
	}
	// Device type comes from text evidence only ("MFP", "scanner", ...); a bare
	// model name stays "printer" rather than guessing.
	if id.DeviceType != "printer" && id.DeviceType != "mfp" {
		t.Errorf("device type = %s", id.DeviceType)
	}
	if len(id.Capabilities) != 2 {
		t.Errorf("skeleton must declare identify + read_config only, got %v", id.Capabilities)
	}
	doc, err := p.ReadConfig(context.Background(), &model.Device{Address: "x", Model: id.Model}, model.Creds{}, model.ConfigSelector{})
	if err != nil {
		t.Fatal(err)
	}
	if s, _ := doc.Get("vendor.fujifilm.family"); s == nil || s.Value != "Apeos" {
		t.Errorf("family = %+v", s)
	}
	if s, _ := doc.Get("identity.location"); s == nil || s.Value != "HR" {
		t.Errorf("location = %+v", s)
	}
}

func TestRejectsOtherVendors(t *testing.T) {
	for _, f := range []string{"generic-laser.snmp", "color-mfp.snmp", "hp-m283fdw-enterprise.snmp", "ricoh-imc3010.snmp", "canon-irdxc3930i.snmp"} {
		p := New(vendorkit.Options{SNMP: vendorkit.FixtureDialer(load(t, f))})
		if _, err := p.Identify(context.Background(), "10.0.0.1", model.Creds{}); !errors.Is(err, vendorkit.ErrNotVendor) {
			t.Errorf("%s: want ErrNotVendor, got %v", f, err)
		}
	}
}

func TestDescriptor(t *testing.T) {
	d := New(vendorkit.Options{}).Descriptor()
	if d.Name != Name || d.Vendor != "Fujifilm" || len(d.Families) == 0 || d.Management.Read == "" {
		t.Errorf("descriptor = %+v", d)
	}
	if len(d.Declared()) != 2 {
		t.Errorf("declared = %v", d.Declared())
	}
}
