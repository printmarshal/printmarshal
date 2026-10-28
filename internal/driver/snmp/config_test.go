package snmp

import (
	"context"
	"testing"

	"github.com/gosnmp/gosnmp"

	"printmarshal/internal/model"
)

func loadFx(t *testing.T, name string) *Fixture {
	t.Helper()
	fx, err := LoadFixture("../../../testdata/fixtures/" + name)
	if err != nil {
		t.Fatal(err)
	}
	return fx
}

func settingMap(doc *model.ConfigDocument) map[string]model.Setting {
	m := map[string]model.Setting{}
	for _, s := range doc.Settings {
		m[s.Key] = s
	}
	return m
}

// Unit (spec §14): the SNMP mapping against the recorded fixtures.
func TestReadConfigFixtures(t *testing.T) {
	fx := loadFx(t, "generic-laser.snmp")
	doc, err := ReadConfig(context.Background(), fx, "v2c")
	if err != nil {
		t.Fatal(err)
	}
	s := settingMap(doc)
	want := map[string]any{
		"identity.hostname":           "LP-FLOOR2-EAST",
		"identity.location":           "Floor 2 east wing",
		"identity.printerName":        "LP-FLOOR2-EAST",
		"protocols.snmp.v1v2cEnabled": true,
	}
	for k, v := range want {
		got, ok := s[k]
		if !ok || !model.ValuesEqual(got.Value, v) {
			t.Errorf("%s = %+v, want %v", k, got, v)
		}
	}
	if !s["identity.hostname"].Writable || s["protocols.snmp.v1v2cEnabled"].Writable {
		t.Error("sysName is writable, the observed SNMP version is not")
	}
	if _, ok := s["identity.contact"]; ok {
		t.Error("sysContact is not in this fixture and must be absent, not invented")
	}
	if _, ok := s["energy.sleepDelayMinutes"]; ok {
		t.Error("keys SNMP cannot express must be absent")
	}

	// The HP fixture has no system group; only the Printer MIB objects appear.
	hp := loadFx(t, "hp-m283fdw.snmp")
	doc, err = ReadConfig(context.Background(), hp, "v2c")
	if err != nil {
		t.Fatal(err)
	}
	s = settingMap(doc)
	if _, ok := s["identity.hostname"]; ok {
		t.Error("no sysName in the HP fixture")
	}
}

// Safety (spec §14): a dry run never issues a SetRequest; results are per setting.
func TestWriteConfigDryRunNoSet(t *testing.T) {
	fx := loadFx(t, "generic-laser.snmp")
	changes := []model.SettingChange{
		{Key: "identity.location", Value: "Floor 3"},
		{Key: "identity.hostname", Value: "LP-FLOOR2-EAST"}, // same value
		{Key: "identity.contact", Value: "ops@example.com"}, // not exposed
		{Key: "print.defaultCopies", Value: 2},              // not an SNMP object
		{Key: "trays.defaultTray", Value: "Tray 2"},
	}
	res, err := WriteConfig(context.Background(), fx, "v2c", "dev", changes, model.WriteOptions{DryRun: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(fx.Sets()) != 0 {
		t.Fatalf("dry run issued a SetRequest: %+v", fx.Sets())
	}
	got := map[string]model.SettingResult{}
	for _, r := range res.Results {
		got[r.Key] = r
	}
	if got["identity.location"].Status != model.SettingWouldWrite || got["identity.location"].Before != "Floor 2 east wing" {
		t.Errorf("location = %+v", got["identity.location"])
	}
	if got["identity.hostname"].Status != model.SettingUnchanged {
		t.Errorf("hostname = %+v", got["identity.hostname"])
	}
	if got["identity.contact"].Status != model.SettingSkipped || got["print.defaultCopies"].Status != model.SettingSkipped {
		t.Errorf("contact/copies = %+v / %+v", got["identity.contact"], got["print.defaultCopies"])
	}
	if _, ok := got["trays.defaultTray"]; !ok {
		t.Errorf("no result for trays.defaultTray: %+v", res.Results)
	}
}

// A read-only community (fixture not Writable) fails the setting; a writable
// agent applies it, the read-back confirms it and the rollback holds the old value.
func TestWriteConfigAppliesAndRollsBack(t *testing.T) {
	fx := loadFx(t, "generic-laser.snmp")
	changes := []model.SettingChange{{Key: "identity.location", Value: "Floor 3"}}
	res, err := WriteConfig(context.Background(), fx, "v2c", "dev", changes, model.WriteOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if res.Results[0].Status != model.SettingFailed || len(res.Rollback) != 0 || len(fx.Sets()) != 1 {
		t.Fatalf("read-only agent: %+v sets=%v", res.Results[0], fx.Sets())
	}
	fx.ResetSets()
	fx.Writable = true
	res, err = WriteConfig(context.Background(), fx, "v2c", "dev", changes, model.WriteOptions{})
	if err != nil {
		t.Fatal(err)
	}
	r := res.Results[0]
	if r.Status != model.SettingWritten || r.Before != "Floor 2 east wing" || r.After != "Floor 3" {
		t.Fatalf("written = %+v", r)
	}
	sets := fx.Sets()
	if len(sets) != 1 || sets[0].OID != OIDSysLocation || sets[0].Type != gosnmp.OctetString || string(sets[0].Value.([]byte)) != "Floor 3" {
		t.Errorf("set = %+v", sets)
	}
	if len(res.Rollback) != 1 || res.Rollback[0].Value != "Floor 2 east wing" {
		t.Errorf("rollback = %+v", res.Rollback)
	}
	// Revert by applying the rollback.
	res, err = WriteConfig(context.Background(), fx, "v2c", "dev", res.Rollback, model.WriteOptions{})
	if err != nil || res.Results[0].Status != model.SettingWritten {
		t.Fatalf("revert: %v %+v", err, res)
	}
	doc, _ := ReadConfig(context.Background(), fx, "v2c")
	if v, _ := doc.Get("identity.location"); v.Value != "Floor 2 east wing" {
		t.Errorf("after revert location = %v", v.Value)
	}
}

// trays.defaultTray resolves a tray name to prtInputDefaultIndex.
func TestWriteDefaultTrayByName(t *testing.T) {
	fx := loadFx(t, "generic-laser.snmp")
	fx.Writable = true
	fx.Put(OIDPrtInputDefaultIndex+".1", 1)
	doc, _ := ReadConfig(context.Background(), fx, "v2c")
	if v, ok := doc.Get("trays.defaultTray"); !ok || v.Value != "Tray 1" {
		t.Fatalf("default tray = %+v", v)
	}
	res, err := WriteConfig(context.Background(), fx, "v2c", "dev", []model.SettingChange{{Key: "trays.defaultTray", Value: "tray 2"}}, model.WriteOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if res.Results[0].Status != model.SettingWritten || res.Results[0].After != "Tray 2" {
		t.Errorf("result = %+v", res.Results[0])
	}
	if sets := fx.Sets(); len(sets) != 1 || sets[0].Value != 2 || sets[0].Type != gosnmp.Integer {
		t.Errorf("sets = %+v", sets)
	}
	res, _ = WriteConfig(context.Background(), fx, "v2c", "dev", []model.SettingChange{{Key: "trays.defaultTray", Value: "Tray 9"}}, model.WriteOptions{})
	if res.Results[0].Status != model.SettingFailed {
		t.Errorf("unknown tray must fail: %+v", res.Results[0])
	}
}
