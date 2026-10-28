package kyocera

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
	p := New(vendorkit.Options{SNMP: vendorkit.FixtureDialer(load(t, "kyocera-taskalfa-3554ci.snmp"))})
	id, err := p.Identify(context.Background(), "10.0.0.9", model.Creds{})
	if err != nil {
		t.Fatal(err)
	}
	if id.Make != "Kyocera" || id.Model != "TASKalfa 3554ci" || id.Serial != "R2V1234567" || id.EnterpriseID != 1347 || id.MAC != "00:17:c8:7f:3a:2b" {
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
	if s, _ := doc.Get("vendor.kyocera.family"); s == nil || s.Value != "TASKalfa" {
		t.Errorf("family = %+v", s)
	}
	if s, _ := doc.Get("identity.location"); s == nil || s.Value != "2nd floor" {
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
	if d.Name != Name || d.Vendor != "Kyocera" || len(d.Families) == 0 || d.Management.Read == "" {
		t.Errorf("descriptor = %+v", d)
	}
	if len(d.Declared()) != 2 {
		t.Errorf("declared = %v", d.Declared())
	}
}
