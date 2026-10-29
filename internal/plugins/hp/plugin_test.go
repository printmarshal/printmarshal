package hp

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"printmarshal/internal/driver/snmp"
	"printmarshal/internal/model"
	"printmarshal/internal/plugins/vendorkit"
)

const fixtures = "../../../testdata/fixtures"

func loadFixture(t *testing.T, name string) *snmp.Fixture {
	t.Helper()
	fx, err := snmp.LoadFixture(filepath.Join(fixtures, name))
	if err != nil {
		t.Fatal(err)
	}
	return fx
}

func ledmPages(t *testing.T) vendorkit.StaticHTTP {
	t.Helper()
	dir := filepath.Join(fixtures, "hp-m283fdw-ledm")
	read := func(n string) []byte {
		b, err := os.ReadFile(filepath.Join(dir, n))
		if err != nil {
			t.Fatal(err)
		}
		return b
	}
	return vendorkit.StaticHTTP{Pages: map[string][]byte{
		ledmDiscoveryTree: read("DiscoveryTree.xml"),
		ledmProductConfig: read("ProductConfigDyn.xml"),
		ledmProductUsage:  read("ProductUsageDyn.xml"),
		ledmProductStatus: read("ProductStatusDyn.xml"),
		ledmNetApps:       read("NetAppsDyn.xml"),
		ledmNetAppsSecure: read("NetAppsSecureDyn.xml"),
		ledmSecurity:      read("SecurityDyn.xml"),
		ledmAdapters:      read("IoMgmt-Adapters.xml"),
	}}
}

func ctx() context.Context { return context.Background() }

func TestIdentifyRecordedM283fdw(t *testing.T) {
	p := New(vendorkit.Options{SNMP: vendorkit.FixtureDialer(loadFixture(t, "hp-m283fdw-enterprise.snmp"))})
	id, err := p.Identify(ctx(), "198.51.100.5", model.Creds{})
	if err != nil {
		t.Fatal(err)
	}
	if id.Make != "HP" || id.Model != "HP Color LaserJet MFP M283fdw" || id.Serial != "VNB0TEST01" || id.Firmware != "20260622" {
		t.Errorf("identity = %+v", id)
	}
	if id.EnterpriseID != 11 || id.DeviceType != "mfp" || id.MAC != "02:00:5e:0a:1b:2c" || id.SysName != "NPI0A1B2C" {
		t.Errorf("identity = %+v", id)
	}
	want := map[model.Capability]bool{model.CapIdentify: true, model.CapReadStatus: true, model.CapReadCounters: true,
		model.CapFirmwareRead: true, model.CapReadConfig: true, model.CapReadSecurity: true}
	if len(id.Capabilities) != len(want) {
		t.Errorf("capabilities = %v", id.Capabilities)
	}
	for _, c := range id.Capabilities {
		if !want[c] {
			t.Errorf("unexpected capability %s", c)
		}
	}
}

func TestRejectsOtherVendors(t *testing.T) {
	for _, f := range []string{"generic-laser.snmp", "color-mfp.snmp"} {
		p := New(vendorkit.Options{SNMP: vendorkit.FixtureDialer(loadFixture(t, f))})
		if _, err := p.Identify(ctx(), "10.0.0.1", model.Creds{}); !errors.Is(err, vendorkit.ErrNotVendor) {
			t.Errorf("%s: want ErrNotVendor, got %v", f, err)
		}
	}
}

// The first recording of the M283fdw (hp-m283fdw.snmp) carries no system
// group at all, so SNMP alone gives no vendor evidence; the IPP recording
// taken at the same time identifies it.
func TestIdentifyViaIPPWhenSNMPHasNoSysObjectID(t *testing.T) {
	ippResp, err := os.ReadFile(filepath.Join(fixtures, "hp-m283fdw.ipp"))
	if err != nil {
		t.Fatal(err)
	}
	snmpOnly := New(vendorkit.Options{SNMP: vendorkit.FixtureDialer(loadFixture(t, "hp-m283fdw.snmp"))})
	if _, err := snmpOnly.Identify(ctx(), "198.51.100.5", model.Creds{}); !errors.Is(err, vendorkit.ErrNotVendor) {
		t.Errorf("snmp-only identify of a system-group-less recording: want ErrNotVendor, got %v", err)
	}
	p := New(vendorkit.Options{SNMP: vendorkit.FixtureDialer(loadFixture(t, "hp-m283fdw.snmp")), IPP: vendorkit.StaticIPP{Resp: ippResp}})
	id, err := p.Identify(ctx(), "198.51.100.5", model.Creds{})
	if err != nil {
		t.Fatal(err)
	}
	if id.Make != "HP" || id.Model != "HP ColorLaserJet MFP M282-M285" || id.UUID != "564e4230-5445-5354-3031-02005e0a1b2c" || id.DeviceType != "mfp" {
		t.Errorf("identity = %+v", id)
	}
	if id.Method != "hp/v2c" {
		t.Errorf("method = %s", id.Method)
	}
}

func TestSamsungLegacyIsIdentityOnly(t *testing.T) {
	fx, err := snmp.ParseFixture(stringsReader(`
.1.3.6.1.2.1.1.1.0 STRING Samsung X4300 Series; V4.00.02.19; SyncThru
.1.3.6.1.2.1.1.2.0 OID .1.3.6.1.4.1.236.11.5.1
.1.3.6.1.2.1.25.3.2.1.2.1 OID .1.3.6.1.2.1.25.3.1.5
.1.3.6.1.2.1.25.3.2.1.3.1 STRING Samsung X4300 Series
.1.3.6.1.2.1.43.5.1.1.17.1 STRING ZDAA001234
`))
	if err != nil {
		t.Fatal(err)
	}
	p := New(vendorkit.Options{SNMP: vendorkit.FixtureDialer(fx)})
	id, err := p.Identify(ctx(), "10.0.0.2", model.Creds{})
	if err != nil {
		t.Fatal(err)
	}
	if id.Make != "Samsung" || id.Serial != "ZDAA001234" || len(id.Capabilities) != 3 {
		t.Errorf("identity = %+v", id)
	}
}

func TestCountersFirmwareStatus(t *testing.T) {
	p := New(vendorkit.Options{SNMP: vendorkit.FixtureDialer(loadFixture(t, "hp-m283fdw-enterprise.snmp")), HTTP: ledmPages(t)})
	dev := &model.Device{ID: "d", Address: "198.51.100.5", EnterpriseID: 11, Make: "HP"}
	c, err := p.ReadCounters(ctx(), dev, model.Creds{})
	if err != nil {
		t.Fatal(err)
	}
	if c.Total != 1396 || c.Mono == nil || *c.Mono != 312 || c.Color == nil || *c.Color != 1084 || c.Duplex == nil || *c.Duplex != 263 {
		t.Errorf("counters = %+v", c)
	}
	if c.ByMarker["1"] != 1396 || c.PowerOn == nil || *c.PowerOn != 8 {
		t.Errorf("standard counters not merged: %+v", c)
	}
	fw, err := p.ReadFirmware(ctx(), dev, model.Creds{})
	if err != nil || fw != "20260622" {
		t.Errorf("firmware = %q %v", fw, err)
	}
	st, err := p.ReadStatus(ctx(), dev, model.Creds{})
	if err != nil {
		t.Fatal(err)
	}
	if !st.Online || st.State != "idle" || st.Message != "Sleep mode is on." {
		t.Errorf("status = %+v", st)
	}
	// Four LEDM "genuineHP" info alerts translated into readable text.
	if len(st.Alerts) != 4 || st.Alerts[0].Description != "Genuine HP supply installed (Yellow toner)" || st.Alerts[0].Severity != model.SeverityOK {
		t.Errorf("alerts = %+v", st.Alerts)
	}
	if st.Severity != model.SeverityOK {
		t.Errorf("severity = %s", st.Severity)
	}
}

func TestMonoColorDroppedWhenTheyDoNotSum(t *testing.T) {
	fx := loadFixture(t, "hp-m283fdw-enterprise.snmp")
	// Rewrite the colour counter so mono+colour != total.
	text := ""
	for _, pdu := range fx.All() {
		if pdu.OID == oidColorPages {
			text += pdu.OID + " INTEGER 5\n"
			continue
		}
		text += fixtureLine(pdu)
	}
	fx2, err := snmp.ParseFixture(stringsReader(text))
	if err != nil {
		t.Fatal(err)
	}
	p := New(vendorkit.Options{SNMP: vendorkit.FixtureDialer(fx2)})
	c, err := p.ReadCounters(ctx(), &model.Device{Address: "x"}, model.Creds{})
	if err != nil {
		t.Fatal(err)
	}
	if c.Total != 1396 || c.Mono != nil || c.Color != nil || c.Duplex == nil {
		t.Errorf("counters = %+v", c)
	}
}

func TestConfigFromLEDM(t *testing.T) {
	p := New(vendorkit.Options{SNMP: vendorkit.FixtureDialer(loadFixture(t, "hp-m283fdw-enterprise.snmp")), HTTP: ledmPages(t)})
	dev := &model.Device{ID: "d", Address: "198.51.100.5", EnterpriseID: 11, Make: "HP", Model: "HP Color LaserJet MFP M283fdw"}
	doc, err := p.ReadConfig(ctx(), dev, model.Creds{}, model.ConfigSelector{})
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]any{
		"identity.hostname":              "NPI0A1B2C",
		"identity.location":              "Home Office",
		"identity.contact":               "Fleet Admin",
		"vendor.hp.badge":                "HP",
		"vendor.hp.family":               "LaserJet",
		"vendor.hp.productNumber":        "7KW75A",
		"vendor.hp.firmwareDateCode":     "20260622",
		"vendor.hp.firmwareDate":         "2026-06-22",
		"vendor.hp.ledm":                 true,
		"security.adminPassword.set":     false,
		"energy.sleepDelayMinutes":       1,
		"energy.autoOffMinutes":          240,
		"panel.language":                 "en",
		"print.defaultDuplex":            false,
		"protocols.snmp.v1v2cEnabled":    true,
		"protocols.snmp.v2cWriteEnabled": true,
		"protocols.mdns.enabled":         true,
		"protocols.wsd.enabled":          true,
		"protocols.lpd.enabled":          true,
		"protocols.llmnr.enabled":        true,
		"protocols.slp.enabled":          true,
		"protocols.https.redirect":       false,
		"protocols.raw9100.enabled":      true,
		"protocols.ipp.enabled":          true,
		"network.ipv4.mode":              "dhcp",
		"network.wifiDirect.enabled":     true,
		"network.wifi.enabled":           true,
		"vendor.hp.encryptionStrength":   "low",
		"vendor.hp.wifi.encryption":      "WPA2",
	}
	for k, v := range want {
		s, ok := doc.Get(k)
		if !ok {
			t.Errorf("missing %s", k)
			continue
		}
		if s.Value != v {
			t.Errorf("%s = %#v, want %#v", k, s.Value, v)
		}
		if s.Writable {
			t.Errorf("%s must be read-only", k)
		}
		if s.Source != "hp" {
			t.Errorf("%s source = %s", k, s.Source)
		}
	}
	if s, ok := doc.Get("vendor.hp.firmware.components"); !ok || len(s.Value.([]string)) != 2 {
		t.Errorf("components = %+v", s)
	}
	seen := map[string]int{}
	for _, s := range doc.Settings {
		seen[s.Key]++
		if seen[s.Key] > 1 {
			t.Errorf("duplicate key %s", s.Key)
		}
	}
	sec, err := p.ReadSecurity(ctx(), dev, model.Creds{})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := sec.Get("protocols.raw9100.enabled"); !ok {
		t.Error("security document lacks protocols.raw9100.enabled")
	}
	if _, ok := sec.Get("panel.language"); ok {
		t.Error("security document must not carry panel settings")
	}
	// Selector by area.
	doc, err = p.ReadConfig(ctx(), dev, model.Creds{}, model.ConfigSelector{Areas: []string{"energy"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(doc.Settings) != 2 {
		t.Errorf("energy area = %+v", doc.Settings)
	}
}

func TestConfigWithoutLEDM(t *testing.T) {
	p := New(vendorkit.Options{SNMP: vendorkit.FixtureDialer(loadFixture(t, "hp-m283fdw-enterprise.snmp")), HTTP: vendorkit.NoHTTP{}})
	doc, err := p.ReadConfig(ctx(), &model.Device{Address: "x", Make: "HP"}, model.Creds{}, model.ConfigSelector{})
	if err != nil {
		t.Fatal(err)
	}
	if s, ok := doc.Get("vendor.hp.ledm"); !ok || s.Value != false {
		t.Errorf("ledm flag = %+v", s)
	}
	if s, ok := doc.Get("vendor.hp.productNumber"); !ok || s.Value != "7KW75A" {
		t.Errorf("PML product number = %+v", s)
	}
	if _, ok := doc.Get("protocols.ipp.enabled"); ok {
		t.Error("no LEDM: protocol settings must be absent, not guessed")
	}
}

func TestParseLEDMUsage(t *testing.T) {
	b, err := os.ReadFile(filepath.Join(fixtures, "hp-m283fdw-ledm", "ProductUsageDyn.xml"))
	if err != nil {
		t.Fatal(err)
	}
	u, err := ParseLEDMUsage(b)
	if err != nil {
		t.Fatal(err)
	}
	if u.Total != 1396 || u.Mono != 312 || u.Color != 1084 || u.DuplexSheets != 263 || !u.HasMonoColor {
		t.Errorf("usage = %+v", u)
	}
}

func TestPMLHelpers(t *testing.T) {
	if got := pmlString(snmp.PDU{Value: []byte{0x01, 0x15, 'A', 'B', 0}}); got != "AB" {
		t.Errorf("pmlString = %q", got)
	}
	// Newer HP firmware (E87760, E47528) sends FD E8 instead of 01 15; unstripped it
	// is invalid UTF-8 and shows as "��" before the model name.
	if got := pmlString(snmp.PDU{Value: []byte("\xfd\xe8HP Color LaserJet Flow E87760\x00")}); got != "HP Color LaserJet Flow E87760" {
		t.Errorf("pmlString with FD E8 header = %q", got)
	}
	jd := jetdirectSysDescr("HP ETHERNET MULTI-ENVIRONMENT,SN:VNB0TEST01,FN:0000001,SVCID:10084,PID:HP Color LaserJet MFP M283fdw")
	if jd["SN"] != "VNB0TEST01" || jd["PID"] != "HP Color LaserJet MFP M283fdw" {
		t.Errorf("jetdirect = %v", jd)
	}
	if m, ok := ledmMinutes("4hours"); !ok || m != 240 {
		t.Error("ledmMinutes hours")
	}
	if m, ok := ledmMinutes("15minutes"); !ok || m != 15 {
		t.Error("ledmMinutes minutes")
	}
	if _, ok := ledmMinutes("never"); ok {
		t.Error("ledmMinutes never")
	}
	if TranslateHPCode("trayEmptyOrOpen") != "Tray empty or open" || TranslateHPCode("someNewThing") != "Some new thing" {
		t.Error("translate")
	}
	if family("HP LaserJet Managed MFP E87740") != "LaserJet Managed" || family("HP Color LaserJet Pro MFP M283fdw") != "LaserJet Pro" || family("HP Color LaserJet MFP M283fdw") != "LaserJet" {
		t.Error("family")
	}
}
