package hp

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"printmarshal/internal/driver/snmp"
	"printmarshal/internal/model"
	"printmarshal/internal/plugins/vendorkit"
)

// lossySession drops the first `lose` replies for one OID, the way a busy scan
// (2 s timeout, many workers) loses an occasional SNMP answer.
type lossySession struct {
	snmp.Session
	oid  string
	lose int
}

func (l *lossySession) Get(ctx context.Context, oids []string) ([]snmp.PDU, error) {
	for _, o := range oids {
		if o == l.oid && l.lose > 0 {
			l.lose--
			return nil, errors.New("snmp timeout")
		}
	}
	return l.Session.Get(ctx, oids)
}

// jetdirectFixture is the recorded M283fdw reduced to what the affected E87760 printers
// send: the Jetdirect card's name in sysDescr (no PID field) and in hrDeviceDescr, so the
// model can only come from the PML model OID.
func jetdirectFixture(t *testing.T) *snmp.Fixture {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(fixtures, "hp-m283fdw-enterprise.snmp"))
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(string(b), "\n")
	n := 0
	for i, l := range lines {
		switch {
		case strings.HasPrefix(l, ".1.3.6.1.2.1.1.1.0 STRING "):
			lines[i] = ".1.3.6.1.2.1.1.1.0 STRING HP ETHERNET MULTI-ENVIRONMENT,ROM none,JETDIRECT,JD149,EEPROM JOL25100"
			n++
		case strings.HasPrefix(l, ".1.3.6.1.2.1.25.3.2.1.3.1 STRING "):
			lines[i] = ".1.3.6.1.2.1.25.3.2.1.3.1 STRING HP ETHERNET MULTI-ENVIRONMENT"
			n++
		}
	}
	if n != 2 {
		t.Fatalf("expected to rewrite sysDescr and hrDeviceDescr, rewrote %d lines", n)
	}
	fx, err := snmp.ParseFixture(strings.NewReader(strings.Join(lines, "\n")))
	if err != nil {
		t.Fatal(err)
	}
	return fx
}

func identifyLosing(t *testing.T, lose int) *model.Identity {
	t.Helper()
	sess := &lossySession{Session: jetdirectFixture(t), oid: oidModelName, lose: lose}
	dial := func(context.Context, string, model.Creds) (snmp.Session, string, error) { return sess, "v2c", nil }
	p := New(vendorkit.Options{SNMP: dial})
	id, err := p.Identify(ctx(), "198.51.100.5", model.Creds{})
	if err != nil {
		t.Fatal(err)
	}
	return id
}

// One lost reply for the model OID must not leave the print server's name as the model.
func TestIdentifyRetriesALostModelRead(t *testing.T) {
	if id := identifyLosing(t, 0); id.Model != "HP Color LaserJet MFP M283fdw" {
		t.Fatalf("baseline model = %q", id.Model)
	}
	if id := identifyLosing(t, 1); id.Model != "HP Color LaserJet MFP M283fdw" {
		t.Errorf("model after one lost reply = %q, want the PML model", id.Model)
	}
}

// When the model really cannot be read, the generic name is only a last resort; the
// store refuses to let it replace a specific model (store.TestUpsertDeviceKeepsSpecificModel).
func TestIdentifyFallsBackToGenericOnlyWhenModelUnreadable(t *testing.T) {
	id := identifyLosing(t, 2)
	if id.Model != "HP ETHERNET MULTI-ENVIRONMENT" || !model.IsGenericModel(id.Model) {
		t.Errorf("model with no PML answer = %q", id.Model)
	}
}
