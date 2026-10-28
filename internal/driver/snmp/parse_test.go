package snmp

import (
	"context"
	"testing"

	"printmarshal/internal/model"
)

func load(t *testing.T, name string) *Fixture {
	t.Helper()
	fx, err := LoadFixture("../../../testdata/fixtures/" + name)
	if err != nil {
		t.Fatalf("load fixture: %v", err)
	}
	return fx
}

func fixtureDialer(fx *Fixture) Dialer {
	return func(ctx context.Context, address string, creds model.Creds) (Session, string, error) {
		return fx, "v2c", nil
	}
}

func TestIdentifyGenericLaser(t *testing.T) {
	d := New(fixtureDialer(load(t, "generic-laser.snmp")))
	id, err := d.Identify(context.Background(), "10.0.0.5", model.Creds{})
	if err != nil {
		t.Fatal(err)
	}
	if id.Serial != "ACM4200X0917" || id.MAC != "00:1b:a9:42:00:17" || id.Make != "Acme" || id.Model != "Acme LaserPrint 4200dn" {
		t.Errorf("identity = %+v", id)
	}
	if id.EnterpriseID != 99999 || id.SysName != "LP-FLOOR2-EAST" || id.SysLocation != "Floor 2 east wing" {
		t.Errorf("identity = %+v", id)
	}
	want := map[model.Capability]bool{model.CapReadSupplies: true, model.CapReadTrays: true, model.CapReadCounters: true, model.CapReadStatus: true}
	for _, c := range id.Capabilities {
		delete(want, c)
	}
	if len(want) > 0 {
		t.Errorf("missing capabilities: %v", want)
	}
}

func TestSuppliesAndTrays(t *testing.T) {
	fx := load(t, "generic-laser.snmp")
	ctx := context.Background()
	sup, _ := fx.Walk(ctx, OIDPrtMarkerSuppliesTable)
	col, _ := fx.Walk(ctx, OIDPrtMarkerColorantValue)
	supplies := ParseSupplies(sup, col)
	if len(supplies) != 2 {
		t.Fatalf("supplies = %d", len(supplies))
	}
	if supplies[0].Description != "Black Toner Cartridge" || supplies[0].Type != "toner" || supplies[0].Colorant != "black" || supplies[0].Percent == nil || *supplies[0].Percent != 12 {
		t.Errorf("toner = %+v", supplies[0])
	}
	if supplies[1].Type != "opc" || supplies[1].Percent == nil || *supplies[1].Percent != 71 {
		t.Errorf("drum = %+v", supplies[1])
	}
	in, _ := fx.Walk(ctx, OIDPrtInputTable)
	trays := ParseInputTrays(in)
	if len(trays) != 2 {
		t.Fatalf("trays = %d", len(trays))
	}
	if trays[0].Name != "Tray 1" || trays[0].Status != "ok" || trays[0].MediaSize != "210x297mm" || trays[0].Percent == nil || *trays[0].Percent != 72 {
		t.Errorf("tray1 = %+v", trays[0])
	}
	if trays[1].Status != "empty" || trays[1].MediaName != "A4" {
		t.Errorf("tray2 = %+v", trays[1])
	}
	out, _ := fx.Walk(ctx, OIDPrtOutputTable)
	if o := ParseOutputTrays(out); len(o) != 1 || o[0].Name != "Face Down Bin" || o[0].Status != "ok" {
		t.Errorf("output = %+v", o)
	}
}

func TestStatusGenericLaser(t *testing.T) {
	st, err := ReadStatus(context.Background(), load(t, "generic-laser.snmp"))
	if err != nil {
		t.Fatal(err)
	}
	if st.State != "idle" || st.Severity != model.SeverityWarning || st.Message != "Toner Low" {
		t.Errorf("status = %+v", st)
	}
	if len(st.Errors) != 1 || st.Errors[0] != "lowToner" {
		t.Errorf("errors = %v", st.Errors)
	}
	if len(st.Alerts) != 1 || st.Alerts[0].Code != "markerTonerAlmostEmpty" || st.Alerts[0].Severity != model.SeverityWarning || st.Alerts[0].Group != "markerSupplies" {
		t.Errorf("alerts = %+v", st.Alerts)
	}
	if st.UptimeSecs != 864123 {
		t.Errorf("uptime = %d", st.UptimeSecs)
	}
}

func TestColorMFP(t *testing.T) {
	fx := load(t, "color-mfp.snmp")
	ctx := context.Background()
	st, err := ReadStatus(ctx, fx)
	if err != nil {
		t.Fatal(err)
	}
	if st.State != "down" || st.Severity != model.SeverityCritical {
		t.Errorf("status = %+v", st)
	}
	if len(st.Errors) != 1 || st.Errors[0] != "jammed" {
		t.Errorf("errors = %v", st.Errors)
	}
	if st.Message != "Paper jam in duplex unit / Open rear door" {
		t.Errorf("message = %q", st.Message)
	}
	if len(st.Alerts) != 2 || st.Alerts[0].Description != "Jam" || st.Alerts[0].Severity != model.SeverityCritical {
		t.Errorf("alerts = %+v", st.Alerts)
	}
	id, err := New(fixtureDialer(fx)).Identify(ctx, "10.0.0.9", model.Creds{})
	if err != nil {
		t.Fatal(err)
	}
	if id.MAC != "3c:2a:f4:77:00:01" {
		t.Errorf("mac = %q (all-zero interface must be skipped)", id.MAC)
	}
	sup, _ := fx.Walk(ctx, OIDPrtMarkerSuppliesTable)
	col, _ := fx.Walk(ctx, OIDPrtMarkerColorantValue)
	supplies := ParseSupplies(sup, col)
	if len(supplies) != 5 {
		t.Fatalf("supplies = %d", len(supplies))
	}
	if supplies[3].Percent != nil {
		t.Errorf("level -3 (some remaining) must give nil percent, got %d", *supplies[3].Percent)
	}
	if supplies[4].Class != "filled" || *supplies[4].Percent != 97 {
		t.Errorf("waste = %+v", supplies[4])
	}
	c := ParseCounters(mustWalk(t, fx, OIDPrtMarkerTable), st.At)
	if c == nil || c.Total != 91002 {
		t.Errorf("counters = %+v", c)
	}
}

func mustWalk(t *testing.T, fx *Fixture, oid string) []PDU {
	t.Helper()
	p, err := fx.Walk(context.Background(), oid)
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func TestNotAPrinter(t *testing.T) {
	fx, err := ParseFixture(stringsReader(".1.3.6.1.2.1.1.1.0 STRING Linux router\n.1.3.6.1.2.1.1.2.0 OID .1.3.6.1.4.1.8072.3.2.10\n"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := New(fixtureDialer(fx)).Identify(context.Background(), "10.0.0.1", model.Creds{}); err == nil {
		t.Fatal("expected a non-printer to be rejected")
	}
}

func TestCompareOID(t *testing.T) {
	if CompareOID(".1.3.6.1.2.1.43.11.1.1.9.1.2", ".1.3.6.1.2.1.43.11.1.1.9.1.10") >= 0 {
		t.Error("numeric component order expected")
	}
	fx := load(t, "generic-laser.snmp")
	if p, ok := fx.Next(".1.3.6.1.2.1.1.1.0"); !ok || p.OID != ".1.3.6.1.2.1.1.2.0" {
		t.Errorf("next = %+v", p)
	}
}
