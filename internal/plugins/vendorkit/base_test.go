package vendorkit

import (
	"context"
	"errors"
	"strings"
	"testing"

	"printmarshal/internal/driver/ipp"
	"printmarshal/internal/driver/snmp"
	"printmarshal/internal/model"
)

const acmeFixture = `
.1.3.6.1.2.1.1.1.0 STRING ACME NetPrinter 9000; Firmware 1.2
.1.3.6.1.2.1.1.2.0 OID .1.3.6.1.4.1.4242.1.1
.1.3.6.1.2.1.1.4.0 STRING ops@example.org
.1.3.6.1.2.1.1.5.0 STRING ACME-1
.1.3.6.1.2.1.1.6.0 STRING Lab
.1.3.6.1.2.1.2.2.1.6.1 HEX 00:11:22:33:44:55
.1.3.6.1.2.1.25.3.2.1.2.1 OID .1.3.6.1.2.1.25.3.1.5
.1.3.6.1.2.1.25.3.2.1.3.1 STRING Acme NetPrinter 9000 MFP
.1.3.6.1.2.1.43.5.1.1.17.1 STRING ACM0001
`

const badgedFixture = `
.1.3.6.1.2.1.1.1.0 STRING BRANDX 9000 (OEM of ACME)
.1.3.6.1.2.1.1.2.0 OID .1.3.6.1.4.1.4242.1.1
.1.3.6.1.2.1.25.3.2.1.2.1 OID .1.3.6.1.2.1.25.3.1.5
.1.3.6.1.2.1.25.3.2.1.3.1 STRING BRANDX 9000
`

const noSysObjectIDFixture = `
.1.3.6.1.2.1.1.1.0 STRING something generic
.1.3.6.1.2.1.25.3.2.1.2.1 OID .1.3.6.1.2.1.25.3.1.5
.1.3.6.1.2.1.25.3.2.1.3.1 STRING unbranded
`

func spec() Spec {
	return Spec{
		Name: "acme", Vendor: "Acme", Enterprises: []int{4242},
		Badges:           []Badge{{Name: "BrandX", Match: []string{"BRANDX"}}},
		IPPManufacturers: []string{"ACME"},
		Capabilities:     []model.Capability{model.CapIdentify, model.CapReadConfig, model.CapReadSecurity, model.CapReadCounters},
		Enrich: func(ctx context.Context, p *Probe, id *model.Identity) {
			if p.Session != nil {
				id.Capabilities = append(id.Capabilities, model.CapReadCounters)
			}
		},
		Family: func(m string) string {
			if strings.Contains(m, "9000") {
				return "9000 series"
			}
			return ""
		},
	}
}

func fx(t *testing.T, text string) *snmp.Fixture {
	t.Helper()
	f, err := snmp.ParseFixture(strings.NewReader(text))
	if err != nil {
		t.Fatal(err)
	}
	return f
}

func TestIdentifyMatchesEnterprise(t *testing.T) {
	b := New(spec(), Options{SNMP: FixtureDialer(fx(t, acmeFixture))})
	id, err := b.Identify(context.Background(), "10.0.0.1", model.Creds{})
	if err != nil {
		t.Fatal(err)
	}
	if id.Make != "Acme" || id.Model != "Acme NetPrinter 9000 MFP" || id.Serial != "ACM0001" || id.EnterpriseID != 4242 || id.MAC != "00:11:22:33:44:55" {
		t.Errorf("identity = %+v", id)
	}
	if id.DeviceType != "mfp" || id.Method != "acme/v2c" {
		t.Errorf("type/method = %s %s", id.DeviceType, id.Method)
	}
	want := []model.Capability{model.CapIdentify, model.CapReadConfig, model.CapReadSecurity, model.CapReadCounters}
	if len(id.Capabilities) != len(want) {
		t.Errorf("caps = %v", id.Capabilities)
	}
}

func TestIdentifyRejectsOtherEnterprise(t *testing.T) {
	other := strings.ReplaceAll(acmeFixture, "4242", "99999")
	b := New(spec(), Options{SNMP: FixtureDialer(fx(t, other)), IPP: StaticIPP{Resp: acmeIPP(t, "ACME")}})
	_, err := b.Identify(context.Background(), "10.0.0.1", model.Creds{})
	if !errors.Is(err, ErrNotVendor) {
		t.Fatalf("want ErrNotVendor even though IPP would match, got %v", err)
	}
}

func TestIdentifyBadgeOnlyWhenEnterpriseMatches(t *testing.T) {
	b := New(spec(), Options{SNMP: FixtureDialer(fx(t, badgedFixture))})
	id, err := b.Identify(context.Background(), "10.0.0.1", model.Creds{})
	if err != nil {
		t.Fatal(err)
	}
	if id.Make != "Acme" || id.Model != "BRANDX 9000" {
		t.Errorf("identity = %+v", id)
	}
	doc, err := b.ReadConfig(context.Background(), &model.Device{Address: "10.0.0.1", Model: id.Model}, model.Creds{}, model.ConfigSelector{})
	if err != nil {
		t.Fatal(err)
	}
	if s, ok := doc.Get("vendor.acme.badge"); !ok || s.Value != "BrandX" {
		t.Errorf("badge setting = %+v", s)
	}
	if s, ok := doc.Get("vendor.acme.family"); !ok || s.Value != "9000 series" {
		t.Errorf("family setting = %+v", s)
	}
}

func TestIdentifyFallsBackToIPP(t *testing.T) {
	// SNMP gives no vendor evidence at all: IPP decides.
	b := New(spec(), Options{SNMP: FixtureDialer(fx(t, noSysObjectIDFixture)), IPP: StaticIPP{Resp: acmeIPP(t, "ACME")}})
	id, err := b.Identify(context.Background(), "10.0.0.1", model.Creds{})
	if err != nil {
		t.Fatal(err)
	}
	if id.Make != "Acme" || id.Serial != "IPP-1" || id.UUID != "11111111-2222-3333-4444-555555555555" {
		t.Errorf("identity = %+v", id)
	}
	// A different manufacturer over IPP is rejected.
	b = New(spec(), Options{SNMP: FixtureDialer(fx(t, noSysObjectIDFixture)), IPP: StaticIPP{Resp: acmeIPP(t, "Other")}})
	if _, err := b.Identify(context.Background(), "10.0.0.1", model.Creds{}); !errors.Is(err, ErrNotVendor) {
		t.Errorf("want ErrNotVendor, got %v", err)
	}
	// No SNMP at all, IPP only.
	b = New(spec(), Options{SNMP: FailingDialer(nil), IPP: StaticIPP{Resp: acmeIPP(t, "ACME")}})
	id, err = b.Identify(context.Background(), "10.0.0.1", model.Creds{})
	if err != nil || id.Method != "acme/ipp" {
		t.Errorf("ipp-only identify: %v %+v", err, id)
	}
	// Nothing answers.
	b = New(spec(), Options{SNMP: FailingDialer(nil), IPP: StaticIPP{}})
	if _, err := b.Identify(context.Background(), "10.0.0.1", model.Creds{}); err == nil || errors.Is(err, ErrNotVendor) {
		t.Errorf("unreachable device must be a transport error, got %v", err)
	}
}

func TestReadConfigAndSecurity(t *testing.T) {
	b := New(spec(), Options{SNMP: FixtureDialer(fx(t, acmeFixture))})
	dev := &model.Device{ID: "d1", Address: "10.0.0.1"}
	doc, err := b.ReadConfig(context.Background(), dev, model.Creds{}, model.ConfigSelector{Areas: []string{"identity"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(doc.Settings) != 3 {
		t.Errorf("identity area = %+v", doc.Settings)
	}
	if s, _ := doc.Get("identity.contact"); s == nil || s.Value != "ops@example.org" || s.Writable {
		t.Errorf("contact = %+v", s)
	}
	sec, err := b.ReadSecurity(context.Background(), dev, model.Creds{})
	if err != nil {
		t.Fatal(err)
	}
	if len(sec.Settings) != 0 {
		t.Errorf("base plugin has no security settings, got %+v", sec.Settings)
	}
	// Config refuses foreign devices.
	other := New(spec(), Options{SNMP: FixtureDialer(fx(t, strings.ReplaceAll(acmeFixture, "4242", "99999")))})
	if _, err := other.ReadConfig(context.Background(), dev, model.Creds{}, model.ConfigSelector{}); !errors.Is(err, ErrNotVendor) {
		t.Errorf("want ErrNotVendor, got %v", err)
	}
}

func TestPriorityAndDescriptor(t *testing.T) {
	s := spec()
	s.Priorities = map[model.Capability]int{model.CapReadCounters: 61}
	s.Evidence = []CapabilityEvidence{
		{Capability: model.CapIdentify, Status: EvidenceDocumented},
		{Capability: model.CapFirmwareUpdate, Status: EvidenceUnsupported},
	}
	b := New(s, Options{})
	if b.Priority(model.CapIdentify) != 60 || b.Priority(model.CapReadConfig) != 20 || b.Priority(model.CapReadCounters) != 61 {
		t.Error("priorities")
	}
	d := b.Descriptor()
	if d.Name != "acme" || len(d.Badges) != 1 || d.Badges[0] != "BrandX" || len(d.Declared()) != 1 {
		t.Errorf("descriptor = %+v", d)
	}
}

func TestWordPrefix(t *testing.T) {
	cases := map[[2]string]bool{
		{"RICOH IM C3010", "ricoh"}:               true,
		{"Ricoh-Aficio", "ricoh"}:                 true,
		{"Ricohx", "ricoh"}:                       false,
		{"HP ETHERNET", "hp"}:                     true,
		{"HPE Thing", "hp"}:                       false,
		{"", "hp"}:                                false,
		{"KONICA MINOLTA", "konica"}:              true,
		{"Hewlett-Packard LJ", "hewlett-packard"}: true,
	}
	for in, want := range cases {
		if got := wordPrefix(in[0], in[1]); got != want {
			t.Errorf("wordPrefix(%q,%q) = %v", in[0], in[1], got)
		}
	}
}

func acmeIPP(t *testing.T, mfg string) []byte {
	t.Helper()
	resp := &ipp.Message{Version: [2]byte{2, 0}, RequestID: 1, Groups: []ipp.Group{{Tag: ipp.TagPrinter, Attributes: []ipp.Attribute{
		{Tag: ipp.TagText, Name: "printer-make-and-model", Values: []any{mfg + " NetPrinter 9000"}},
		{Tag: ipp.TagText, Name: "printer-device-id", Values: []any{"MFG:" + mfg + ";MDL:NetPrinter 9000;CMD:PDF;SN:IPP-1;"}},
		{Tag: ipp.TagURI, Name: "printer-uuid", Values: []any{"urn:uuid:11111111-2222-3333-4444-555555555555"}},
		{Tag: ipp.TagText, Name: "printer-firmware-string-version", Values: []any{"9.1.2"}},
	}}}}
	b, err := ipp.Encode(resp)
	if err != nil {
		t.Fatal(err)
	}
	return b
}
