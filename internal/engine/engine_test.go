package engine

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"testing"

	"printmarshal/internal/model"
)

// fake is a configurable driver for registry/dispatch tests.
type fake struct {
	name     string
	caps     []model.Capability
	prio     int
	identity *model.Identity
	identErr error
	status   *model.Status
	statErr  error
	supplies []model.Supply
	calls    int
	applies  func(Preflight) bool
}

// Applies implements Gated; a fake without an applies rule always runs.
func (f *fake) Applies(p Preflight) bool {
	if f.applies == nil {
		return true
	}
	return f.applies(p)
}

func (f *fake) Name() string                     { return f.name }
func (f *fake) Capabilities() []model.Capability { return f.caps }
func (f *fake) Supports(d *model.Device, c model.Capability) bool {
	return ProfileSupports(d, f.name, c)
}
func (f *fake) Priority(model.Capability) int { return f.prio }
func (f *fake) Identify(context.Context, string, model.Creds) (*model.Identity, error) {
	f.calls++
	if f.identErr != nil {
		return nil, f.identErr
	}
	id := *f.identity
	return &id, nil
}
func (f *fake) ReadStatus(context.Context, *model.Device, model.Creds) (*model.Status, error) {
	f.calls++
	if f.statErr != nil {
		return nil, f.statErr
	}
	s := *f.status
	return &s, nil
}
func (f *fake) ReadSupplies(context.Context, *model.Device, model.Creds) ([]model.Supply, error) {
	return f.supplies, nil
}

func newEngine(drivers ...Driver) *Engine {
	reg := NewRegistry()
	for _, d := range drivers {
		reg.Register(d)
	}
	return New(reg, slog.New(slog.NewTextHandler(io.Discard, nil)), 2)
}

func TestIdentifyMergesByPriorityAndResolvesCapabilities(t *testing.T) {
	snmp := &fake{name: "snmp", prio: 50, caps: []model.Capability{model.CapIdentify, model.CapReadStatus, model.CapReadSupplies},
		identity: &model.Identity{Serial: "S1", Model: "Acme 1", DeviceType: "printer", Capabilities: []model.Capability{model.CapIdentify, model.CapReadStatus, model.CapReadSupplies}}}
	ipp := &fake{name: "ipp", prio: 45, caps: []model.Capability{model.CapIdentify, model.CapReadStatus, model.CapFirmwareRead},
		identity: &model.Identity{Model: "Acme One (IPP)", Firmware: "1.2", UUID: "u-1", DeviceType: "mfp", Capabilities: []model.Capability{model.CapIdentify, model.CapReadStatus, model.CapFirmwareRead}}}
	broken := &fake{name: "vendor", prio: 60, caps: []model.Capability{model.CapIdentify}, identErr: errors.New("not mine")}
	e := newEngine(snmp, ipp, broken)
	id, caps, err := e.Identify(context.Background(), "10.0.0.5", model.Creds{})
	if err != nil {
		t.Fatal(err)
	}
	if id.Serial != "S1" || id.Model != "Acme 1" || id.Firmware != "1.2" || id.UUID != "u-1" {
		t.Errorf("merge by priority failed: %+v", id)
	}
	if id.DeviceType != "mfp" {
		t.Errorf("mfp should outrank printer, got %q", id.DeviceType)
	}
	if id.Method != "snmp,ipp" {
		t.Errorf("method = %q", id.Method)
	}
	if caps[model.CapReadStatus] != "snmp" || caps[model.CapFirmwareRead] != "ipp" || caps[model.CapReadSupplies] != "snmp" {
		t.Errorf("capability profile = %v", caps)
	}
	if _, ok := caps[model.CapIdentify]; !ok {
		t.Error("identify capability missing")
	}
}

func TestIdentifyNothingAnswers(t *testing.T) {
	e := newEngine(&fake{name: "snmp", prio: 50, caps: []model.Capability{model.CapIdentify}, identErr: errors.New("timeout")})
	_, _, err := e.Identify(context.Background(), "10.0.0.9", model.Creds{})
	if !errors.Is(err, ErrNotIdentified) {
		t.Fatalf("expected ErrNotIdentified, got %v", err)
	}
}

func TestPollFallsBackToNextDriver(t *testing.T) {
	primary := &fake{name: "snmp", prio: 50, caps: []model.Capability{model.CapReadStatus}, statErr: errors.New("boom")}
	backup := &fake{name: "ipp", prio: 45, caps: []model.Capability{model.CapReadStatus}, status: &model.Status{Online: true, State: "idle", Severity: model.SeverityOK}}
	e := newEngine(primary, backup)
	dev := &model.Device{ID: "d", Address: "10.0.0.5", Capabilities: map[model.Capability]string{model.CapReadStatus: "snmp"}, IdentifyMethods: []string{"snmp", "ipp"}}
	res := e.Poll(context.Background(), dev, model.Creds{})
	if !res.Reachable || res.Status == nil || res.Status.State != "idle" {
		t.Fatalf("expected fallback status, got %+v", res)
	}
	if res.ServedBy[model.CapReadStatus] != "ipp" {
		t.Errorf("servedBy = %v", res.ServedBy)
	}
	if primary.calls != 1 || backup.calls != 1 {
		t.Errorf("calls primary=%d backup=%d", primary.calls, backup.calls)
	}
	if _, ok := res.Errors[model.CapReadSupplies]; !ok {
		t.Error("unsupported capability should be recorded as an error, not crash")
	}
}

func TestPollUnreachableProducesOfflineStatus(t *testing.T) {
	d := &fake{name: "snmp", prio: 50, caps: []model.Capability{model.CapReadStatus}, statErr: errors.New("timeout")}
	e := newEngine(d)
	dev := &model.Device{ID: "d", Capabilities: map[model.Capability]string{model.CapReadStatus: "snmp"}}
	res := e.Poll(context.Background(), dev, model.Creds{})
	if res.Reachable || res.Status == nil || res.Status.Online || res.Status.State != "offline" || res.Status.Severity != model.SeverityCritical {
		t.Errorf("offline status expected: %+v", res.Status)
	}
}

func TestRegistryOrderAndProfileSupports(t *testing.T) {
	a := &fake{name: "a", prio: 10, caps: []model.Capability{model.CapReadStatus}}
	b := &fake{name: "b", prio: 90, caps: []model.Capability{model.CapReadStatus}}
	reg := NewRegistry()
	reg.Register(a)
	reg.Register(b)
	dev := &model.Device{IdentifyMethods: []string{"a", "b"}}
	got := reg.For(dev, model.CapReadStatus)
	if len(got) != 2 || got[0].Name() != "b" {
		t.Errorf("highest priority first expected, got %v", got)
	}
	assigned := &model.Device{Capabilities: map[model.Capability]string{model.CapReadStatus: "a"}, IdentifyMethods: []string{"a", "b"}}
	if got := reg.For(assigned, model.CapReadStatus); len(got) != 1 || got[0].Name() != "a" {
		t.Errorf("profile assignment must pin the driver, got %v", got)
	}
	if reg.Named("b") == nil || reg.Named("zzz") != nil {
		t.Error("Named lookup")
	}
}

func TestIdentifyRecordsDriversThatDidNotAnswer(t *testing.T) {
	snmp := &fake{name: "snmp", caps: []model.Capability{model.CapIdentify}, prio: 10, identErr: errors.New("request timeout (after 1 retries)")}
	hp := &fake{name: "hp", caps: []model.Capability{model.CapIdentify}, prio: 5, identity: &model.Identity{Make: "HP", Model: "M283", Method: "hp"}}
	xerox := &fake{name: "xerox", caps: []model.Capability{model.CapIdentify}, prio: 5, identErr: fmt.Errorf("enterprise 11 is not xerox: %w", ErrNotApplicable)}
	id, _, err := newEngine(snmp, hp, xerox).Identify(context.Background(), "10.0.0.3", model.Creds{})
	if err != nil {
		t.Fatal(err)
	}
	if id.Method != "hp" || id.Model != "M283" {
		t.Fatalf("merged identity: %+v", id)
	}
	// The generic driver's failure is recorded; a plugin for another vendor is not.
	if len(id.Errors) != 1 || id.Errors["snmp"] != "request timeout (after 1 retries)" {
		t.Fatalf("errors: %+v", id.Errors)
	}
	// A fully answered identification carries no errors at all.
	snmp.identErr = nil
	snmp.identity = &model.Identity{Make: "HP", Method: "snmp"}
	id, _, err = newEngine(snmp, hp, xerox).Identify(context.Background(), "10.0.0.3", model.Creds{})
	if err != nil || id.Errors != nil {
		t.Fatalf("errors after a clean identification: %+v (%v)", id.Errors, err)
	}
}

func TestIdentifyPreflightSkipsPluginsForOtherVendors(t *testing.T) {
	snmp := &fake{name: "snmp", caps: []model.Capability{model.CapIdentify}, prio: 10, identity: &model.Identity{Make: "HP", Method: "snmp"}}
	hp := &fake{name: "hp", caps: []model.Capability{model.CapIdentify}, prio: 5, identity: &model.Identity{Make: "HP", Model: "M283", Method: "hp"},
		applies: func(p Preflight) bool { return !p.SNMP || p.Enterprise == 0 || p.Enterprise == 11 }}
	xerox := &fake{name: "xerox", caps: []model.Capability{model.CapIdentify}, prio: 5, identErr: errors.New("should not have been asked"),
		applies: func(p Preflight) bool { return !p.SNMP || p.Enterprise == 0 || p.Enterprise == 253 }}
	eng := newEngine(snmp, hp, xerox)
	eng.Preflight = func(context.Context, string, model.Creds) (Preflight, bool) {
		return Preflight{SNMP: true, Enterprise: 11, SysDescr: "HP Color LaserJet"}, true
	}
	id, _, err := eng.Identify(context.Background(), "10.0.0.3", model.Creds{})
	if err != nil {
		t.Fatal(err)
	}
	if xerox.calls != 0 || hp.calls != 1 || snmp.calls != 1 {
		t.Fatalf("calls: xerox=%d hp=%d snmp=%d", xerox.calls, hp.calls, snmp.calls)
	}
	if id.Model != "M283" || id.Errors != nil {
		t.Fatalf("identity: %+v", id)
	}

	// No enterprise reported: every plugin still runs (badge text / IPP matching).
	xerox.identErr = fmt.Errorf("no xerox evidence: %w", ErrNotApplicable)
	eng.Preflight = func(context.Context, string, model.Creds) (Preflight, bool) {
		return Preflight{SNMP: true}, true
	}
	if _, _, err := eng.Identify(context.Background(), "10.0.0.3", model.Creds{}); err != nil {
		t.Fatal(err)
	}
	if xerox.calls != 1 {
		t.Fatalf("xerox should run without an enterprise, calls=%d", xerox.calls)
	}

	// SNMP silent: plugins run for their IPP path.
	eng.Preflight = func(context.Context, string, model.Creds) (Preflight, bool) { return Preflight{}, true }
	if _, _, err := eng.Identify(context.Background(), "10.0.0.3", model.Creds{}); err != nil {
		t.Fatal(err)
	}
	if xerox.calls != 2 {
		t.Fatalf("xerox should run when SNMP is silent, calls=%d", xerox.calls)
	}
}
