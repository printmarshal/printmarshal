package firmware

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"printmarshal/internal/config"
	"printmarshal/internal/engine"
	"printmarshal/internal/fleet"
	"printmarshal/internal/jobs"
	"printmarshal/internal/model"
	"printmarshal/internal/secrets"
	"printmarshal/internal/store"
)

// --- fakes -------------------------------------------------------------------

type call struct {
	DeviceID string
	Version  string
	DryRun   bool
}

// fakeUpdater is a firmware_update driver that records every call.
type fakeUpdater struct {
	mu      sync.Mutex
	calls   []call
	reject  map[string]string // device id -> rejection message
	fail    map[string]error  // device id -> driver error
	devices *fakeDevices
}

func (f *fakeUpdater) Name() string { return "fake" }
func (f *fakeUpdater) Capabilities() []model.Capability {
	return []model.Capability{model.CapFirmwareUpdate}
}
func (f *fakeUpdater) Priority(model.Capability) int { return 10 }
func (f *fakeUpdater) Supports(d *model.Device, c model.Capability) bool {
	return engine.ProfileSupports(d, "fake", c)
}

func (f *fakeUpdater) UpdateFirmware(ctx context.Context, d *model.Device, creds model.Creds, pkg *model.FirmwarePackage, opts model.WriteOptions) (*model.FirmwareResult, error) {
	f.mu.Lock()
	f.calls = append(f.calls, call{d.ID, pkg.Version, opts.DryRun})
	rej, rejected := f.reject[d.ID]
	err := f.fail[d.ID]
	f.mu.Unlock()
	if opts.DryRun {
		return &model.FirmwareResult{DeviceID: d.ID, DryRun: true, Accepted: true, Message: "validated " + pkg.FileName, At: time.Now()}, nil
	}
	if err != nil {
		return nil, err
	}
	if rejected {
		return &model.FirmwareResult{DeviceID: d.ID, Accepted: false, Message: rej, At: time.Now()}, nil
	}
	f.devices.accept(d.ID, pkg.Version)
	return &model.FirmwareResult{DeviceID: d.ID, Accepted: true, Message: "image accepted", At: time.Now()}, nil
}

func (f *fakeUpdater) Calls() []call {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]call(nil), f.calls...)
}

// fakeDevices simulates the devices' reported state for the poller.
type fakeDevices struct {
	mu       sync.Mutex
	fw       map[string]string
	pending  map[string]string // accepted version not yet reported
	polls    map[string]int    // polls before a pending version becomes visible (per device)
	never    map[string]bool   // accepted but never applies (verification timeout)
	offline  map[string]bool
	state    map[string]string
	polled   map[string]int
	storeCtx store.Store
}

func newFakeDevices(st store.Store) *fakeDevices {
	return &fakeDevices{fw: map[string]string{}, pending: map[string]string{}, polls: map[string]int{}, never: map[string]bool{},
		offline: map[string]bool{}, state: map[string]string{}, polled: map[string]int{}, storeCtx: st}
}

func (f *fakeDevices) accept(id, version string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.never[id] {
		return
	}
	f.pending[id] = version
}

func (f *fakeDevices) Poll(ctx context.Context, d *model.Device) (*model.Device, error) {
	f.mu.Lock()
	f.polled[d.ID]++
	if v, ok := f.pending[d.ID]; ok {
		if f.polled[d.ID] > f.polls[d.ID] {
			f.fw[d.ID] = v
			delete(f.pending, d.ID)
		}
	}
	fw := f.fw[d.ID]
	offline := f.offline[d.ID]
	state := f.state[d.ID]
	f.mu.Unlock()
	if state == "" {
		state = "idle"
	}
	cp := *d
	cp.Firmware = fw
	cp.Status = &model.Status{Online: !offline, State: state, At: time.Now()}
	if offline {
		cp.Status.State = "offline"
	}
	// Mirror what a real poll stores so the inventory follows.
	if !offline {
		_, _ = f.storeCtx.ApplyPoll(ctx, d.ID, store.PollUpdate{Status: cp.Status, Firmware: fw, Reachable: true, At: time.Now()})
	}
	return &cp, nil
}

// --- harness -----------------------------------------------------------------

type harness struct {
	t      *testing.T
	svc    *Service
	st     store.Store
	upd    *fakeUpdater
	dev    *fakeDevices
	runner *jobs.Runner
	mu     sync.Mutex
	events []model.Event
	n      int
	// writeCred is the global write credential newHarness binds.
	writeCred *model.Credential
}

func newHarness(t *testing.T) *harness {
	t.Helper()
	box, _ := secrets.New(make([]byte, 32))
	st, err := store.OpenSQLite(t.TempDir()+"/t.db", box)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	h := &harness{t: t, st: st, dev: newFakeDevices(st)}
	h.upd = &fakeUpdater{devices: h.dev, reject: map[string]string{}, fail: map[string]error{}}
	reg := engine.NewRegistry()
	reg.Register(h.upd)
	eng := engine.New(reg, log, 4)
	cfg := config.Default()
	cfg.DataDir = t.TempDir()
	events := func(e model.Event) {
		h.mu.Lock()
		h.events = append(h.events, e)
		h.mu.Unlock()
	}
	h.runner = jobs.New(st, log, 2, events)
	t.Cleanup(h.runner.Stop)
	h.svc = New(fleet.Deps{Store: st, Engine: eng, Jobs: h.runner, Cfg: cfg, Log: log, Events: events})
	h.svc.Poll = h.dev.Poll
	h.svc.VerifyTimeout, h.svc.VerifyInterval, h.svc.ControlPoll, h.svc.RetryDelay = 300*time.Millisecond, 5*time.Millisecond, 10*time.Millisecond, time.Millisecond
	// Real deliveries need a write credential bound (audit A19).
	h.writeCred = &model.Credential{Name: "fw write", Type: model.CredAdminPassword, Username: "admin", Purpose: model.PurposeWrite}
	if err := st.CreateCredential(context.Background(), h.writeCred, &model.Secret{Password: "harnessWritePw"}); err != nil {
		t.Fatal(err)
	}
	return h
}

func (h *harness) device(name, mk, mdl, fw string) *model.Device {
	h.t.Helper()
	h.n++
	id := &model.Identity{Address: fmt.Sprintf("10.0.0.%d", h.n), Serial: "SN" + name, Make: mk, Model: mdl, Firmware: fw, SysName: name, Method: "snmp"}
	d, _, err := h.st.UpsertDevice(context.Background(), id, map[model.Capability]string{model.CapFirmwareUpdate: "fake", model.CapFirmwareRead: "snmp"}, nil)
	if err != nil {
		h.t.Fatal(err)
	}
	h.dev.mu.Lock()
	h.dev.fw[d.ID] = fw
	h.dev.mu.Unlock()
	return d
}

func (h *harness) pkg(vendor string, models []string, version, min string) *model.FirmwarePackage {
	h.t.Helper()
	p, err := h.svc.RegisterPackage(context.Background(), PackageMeta{Vendor: vendor, Models: models, Version: version, MinVersion: min, FileName: "fw-" + version + ".bin"},
		bytes.NewReader([]byte("firmware image "+version)), "admin")
	if err != nil {
		h.t.Fatal(err)
	}
	return p
}

func (h *harness) campaign(pkg *model.FirmwarePackage, devs []*model.Device, mutate func(c *model.Campaign)) *model.Campaign {
	h.t.Helper()
	c := &model.Campaign{Name: "test", PackageID: pkg.ID, CanaryCount: CanaryDefault, Concurrency: 2}
	for _, d := range devs {
		c.DeviceIDs = append(c.DeviceIDs, d.ID)
	}
	if mutate != nil {
		mutate(c)
	}
	if err := h.svc.CreateCampaign(context.Background(), c, "operator"); err != nil {
		h.t.Fatal(err)
	}
	return c
}

func (h *harness) start(c *model.Campaign, dryRun bool) *model.Campaign {
	h.t.Helper()
	c, err := h.svc.Start(context.Background(), c.ID, "operator", StartOptions{DryRun: dryRun})
	if err != nil {
		h.t.Fatal(err)
	}
	return c
}

// waitFor polls until cond holds or the test times out.
func (h *harness) waitFor(cond func(c *model.Campaign, devs []*model.CampaignDevice) bool) (*model.Campaign, []*model.CampaignDevice) {
	h.t.Helper()
	return h.waitForID("", cond)
}

func (h *harness) waitForID(id string, cond func(c *model.Campaign, devs []*model.CampaignDevice) bool) (*model.Campaign, []*model.CampaignDevice) {
	h.t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	var c *model.Campaign
	var devs []*model.CampaignDevice
	for time.Now().Before(deadline) {
		if id == "" {
			cs, _ := h.st.ListCampaigns(context.Background(), store.CampaignFilter{Limit: 1})
			if len(cs) > 0 {
				id = cs[0].ID
			}
		}
		if id != "" {
			c, _ = h.st.GetCampaign(context.Background(), id)
			devs, _ = h.st.ListCampaignDevices(context.Background(), id)
			if c != nil && cond(c, devs) {
				return c, devs
			}
		}
		time.Sleep(5 * time.Millisecond)
	}
	h.t.Fatalf("condition not met in time; campaign=%+v summary=%+v", c, Summarize(devs))
	return nil, nil
}

func status(s model.CampaignStatus) func(c *model.Campaign, devs []*model.CampaignDevice) bool {
	return func(c *model.Campaign, _ []*model.CampaignDevice) bool { return c.Status == s }
}

func byDevice(devs []*model.CampaignDevice) map[string]*model.CampaignDevice {
	m := map[string]*model.CampaignDevice{}
	for _, d := range devs {
		m[d.DeviceID] = d
	}
	return m
}

func (h *harness) audits(action string) []*model.AuditEntry {
	all, _ := h.st.ListAudit(context.Background(), 500)
	var out []*model.AuditEntry
	for _, a := range all {
		if a.Action == action {
			out = append(out, a)
		}
	}
	return out
}

func (h *harness) eventCount(typ string) int {
	h.mu.Lock()
	defer h.mu.Unlock()
	n := 0
	for _, e := range h.events {
		if e.Type == typ {
			n++
		}
	}
	return n
}

// --- tests -------------------------------------------------------------------

// Canary wave -> paused for confirmation -> confirm -> wave 2 -> completed.
func TestCanaryPauseConfirmWave2(t *testing.T) {
	h := newHarness(t)
	pkg := h.pkg("Acme", []string{"4200"}, "2.15.0", "")
	var devs []*model.Device
	for i := 0; i < 5; i++ {
		devs = append(devs, h.device(fmt.Sprintf("p%d", i), "Acme", "Acme LaserPro 4200", "2.14.7"))
	}
	c := h.campaign(pkg, devs, func(c *model.Campaign) { c.CanaryCount = 2; c.Concurrency = 2 })
	if c.Status != model.CampaignDraft {
		t.Fatalf("created status = %s", c.Status)
	}
	// A draft cannot be confirmed, paused or resumed; creating never starts anything.
	if _, err := h.svc.Confirm(context.Background(), c.ID, "op"); !errors.Is(err, ErrConflict) {
		t.Errorf("confirm draft: %v", err)
	}
	if calls := h.upd.Calls(); len(calls) != 0 {
		t.Fatalf("draft must not deliver: %+v", calls)
	}

	h.start(c, false)
	if _, err := h.svc.Start(context.Background(), c.ID, "op", StartOptions{}); !errors.Is(err, ErrConflict) {
		t.Errorf("second start must be refused: %v", err)
	}
	c, rows := h.waitFor(func(c *model.Campaign, _ []*model.CampaignDevice) bool {
		return c.Status == model.CampaignPaused && c.PauseReason == model.PauseCanary
	})
	sum := Summarize(rows)
	if sum.Succeeded != 2 || sum.Pending != 3 || c.Wave != 1 || c.CanaryCount != 2 {
		t.Fatalf("after canary: %+v wave=%d canary=%d", sum, c.Wave, c.CanaryCount)
	}
	if len(h.upd.Calls()) != 2 {
		t.Fatalf("canary must deliver to 2 devices only: %+v", h.upd.Calls())
	}
	if _, err := h.svc.Resume(context.Background(), c.ID, "op"); !errors.Is(err, ErrConflict) {
		t.Errorf("resume must not bypass the canary gate: %v", err)
	}
	if err := h.svc.UpdateCampaign(context.Background(), c); !errors.Is(err, ErrConflict) {
		t.Errorf("started campaigns are immutable: %v", err)
	}
	if _, err := h.svc.Confirm(context.Background(), c.ID, "op"); err != nil {
		t.Fatal(err)
	}
	c, rows = h.waitFor(status(model.CampaignCompleted))
	sum = Summarize(rows)
	if sum.Succeeded != 5 || sum.Failed != 0 || c.Wave != 2 || !c.Confirmed || c.FinishedAt == nil {
		t.Fatalf("final: %+v %+v", sum, c)
	}
	for _, r := range rows {
		if r.FromVersion != "2.14.7" || r.ToVersion != "2.15.0" || !strings.Contains(r.Message, "verified") {
			t.Errorf("row %+v", r)
		}
	}
	for _, call := range h.upd.Calls() {
		if call.DryRun {
			t.Errorf("real campaign made a dry-run call: %+v", call)
		}
	}
	if n := len(h.audits(AuditUpdate)); n != 5 {
		t.Errorf("audit entries = %d, want 5", n)
	}
	for _, a := range h.audits(AuditUpdate) {
		if a.Actor != "operator" || a.Result != "accepted" || a.After["version"] != "2.15.0" {
			t.Errorf("audit %+v", a)
		}
	}
	if h.eventCount(EventCampaignUpdated) == 0 || h.eventCount(EventDeviceUpdated) < 5 {
		t.Errorf("events: campaign=%d device=%d", h.eventCount(EventCampaignUpdated), h.eventCount(EventDeviceUpdated))
	}
	j, err := h.runner.Wait(context.Background(), c.JobID)
	if err != nil || j.Status != model.JobSucceeded || j.Done != 5 {
		t.Errorf("job %+v err=%v", j, err)
	}
	// The inventory follows the verified versions.
	inv, err := h.svc.Inventory(context.Background(), InventoryFilter{})
	if err != nil || inv.Devices != 5 || inv.Outdated != 0 || len(inv.Entries) != 1 || inv.Entries[0].Version != "2.15.0" {
		t.Errorf("inventory %+v err=%v", inv, err)
	}
}

func TestCanaryZeroRunsWithoutPause(t *testing.T) {
	h := newHarness(t)
	pkg := h.pkg("Acme", nil, "3.0", "")
	devs := []*model.Device{h.device("a", "Acme", "X1", "2.0"), h.device("b", "Acme", "X1", "2.0"), h.device("c", "Acme", "X1", "2.0")}
	c := h.campaign(pkg, devs, func(c *model.Campaign) { c.CanaryCount = 0; c.Concurrency = 3 })
	h.start(c, false)
	c, rows := h.waitFor(status(model.CampaignCompleted))
	if sum := Summarize(rows); sum.Succeeded != 3 || c.Confirmed {
		t.Fatalf("%+v confirmed=%v", sum, c.Confirmed)
	}
	for _, r := range rows {
		if r.Wave != 1 {
			t.Errorf("canary 0 puts every device in wave 1: %+v", r)
		}
	}
}

func TestDefaultCanaryIsThreeOrTenPercent(t *testing.T) {
	h := newHarness(t)
	pkg := h.pkg("Acme", nil, "3.0", "")
	var devs []*model.Device
	for i := 0; i < 45; i++ {
		devs = append(devs, h.device(fmt.Sprintf("d%02d", i), "Acme", "X1", "2.0"))
	}
	c := h.campaign(pkg, devs, nil)
	rows, canary, err := h.svc.Plan(context.Background(), c)
	if err != nil || canary != 5 || len(rows) != 45 {
		t.Fatalf("canary=%d rows=%d err=%v", canary, len(rows), err)
	}
	small := h.campaign(pkg, devs[:4], nil)
	if _, canary, _ := h.svc.Plan(context.Background(), small); canary != 3 {
		t.Errorf("small fleet canary = %d, want 3", canary)
	}
}

func TestFailureThresholdPausesAndResumeContinues(t *testing.T) {
	h := newHarness(t)
	pkg := h.pkg("Acme", nil, "3.0", "")
	devs := []*model.Device{h.device("a", "Acme", "X1", "2.0"), h.device("b", "Acme", "X1", "2.0"), h.device("c", "Acme", "X1", "2.0"), h.device("d", "Acme", "X1", "2.0")}
	h.upd.reject[devs[0].ID] = "image signature invalid"
	c := h.campaign(pkg, devs, func(c *model.Campaign) { c.CanaryCount = 0; c.Concurrency = 1; c.FailureThresholdPercent = 50 })
	h.start(c, false)
	c, rows := h.waitFor(func(c *model.Campaign, _ []*model.CampaignDevice) bool {
		return c.Status == model.CampaignPaused && c.PauseReason == model.PauseThreshold
	})
	if sum := Summarize(rows); sum.Failed != 1 || sum.Pending != 3 || !strings.Contains(c.Error, "threshold") {
		t.Fatalf("%+v %s", sum, c.Error)
	}
	if r := byDevice(rows)[devs[0].ID]; r.Status != model.CampaignDeviceFailed || !strings.Contains(r.Message, "rejected") || !strings.Contains(r.Message, "signature") {
		t.Errorf("rejected row %+v", r)
	}
	if n := len(h.audits(AuditUpdate)); n != 1 || h.audits(AuditUpdate)[0].Result != "rejected" {
		t.Errorf("rejection must be audited: %d", n)
	}
	if _, err := h.svc.Confirm(context.Background(), c.ID, "op"); !errors.Is(err, ErrConflict) {
		t.Errorf("confirm is only for the canary gate: %v", err)
	}
	if _, err := h.svc.Resume(context.Background(), c.ID, "op"); err != nil {
		t.Fatal(err)
	}
	c, rows = h.waitFor(status(model.CampaignCompleted))
	if sum := Summarize(rows); sum.Succeeded != 3 || sum.Failed != 1 {
		t.Fatalf("%+v", sum)
	}
}

func TestPauseAndCancelSkipPending(t *testing.T) {
	h := newHarness(t)
	pkg := h.pkg("Acme", nil, "3.0", "")
	var devs []*model.Device
	for i := 0; i < 6; i++ {
		devs = append(devs, h.device(fmt.Sprintf("p%d", i), "Acme", "X1", "2.0"))
	}
	// Make verification slow enough to pause mid-flight.
	for _, d := range devs {
		h.dev.polls[d.ID] = 5
	}
	c := h.campaign(pkg, devs, func(c *model.Campaign) { c.CanaryCount = 0; c.Concurrency = 1 })
	h.start(c, false)
	h.waitFor(func(c *model.Campaign, devs []*model.CampaignDevice) bool { return Summarize(devs).Succeeded >= 1 })
	if _, err := h.svc.Pause(context.Background(), c.ID, "op"); err != nil {
		t.Fatal(err)
	}
	c, rows := h.waitFor(func(c *model.Campaign, devs []*model.CampaignDevice) bool {
		return c.Status == model.CampaignPaused && Summarize(devs).Updating == 0 && Summarize(devs).Verifying == 0
	})
	paused := Summarize(rows)
	if paused.Pending == 0 || c.PauseReason != model.PauseOperator {
		t.Fatalf("paused: %+v reason=%s", paused, c.PauseReason)
	}
	// Nothing moves while paused.
	time.Sleep(50 * time.Millisecond)
	if _, rows = h.waitFor(status(model.CampaignPaused)); Summarize(rows) != paused {
		t.Fatalf("paused campaign kept working: %+v -> %+v", paused, Summarize(rows))
	}
	if _, err := h.svc.Cancel(context.Background(), c.ID, "op"); err != nil {
		t.Fatal(err)
	}
	c, rows = h.waitFor(func(c *model.Campaign, devs []*model.CampaignDevice) bool {
		return c.Status == model.CampaignCancelled && c.FinishedAt != nil && Summarize(devs).Pending == 0
	})
	sum := Summarize(rows)
	if sum.Skipped != paused.Pending || sum.Succeeded != paused.Succeeded {
		t.Fatalf("cancel: %+v (paused %+v)", sum, paused)
	}
	if _, err := h.svc.Cancel(context.Background(), c.ID, "op"); !errors.Is(err, ErrConflict) {
		t.Errorf("cancel twice: %v", err)
	}
	if err := h.svc.DeleteCampaign(context.Background(), c.ID); err != nil {
		t.Errorf("finished campaigns are deletable: %v", err)
	}
	if j, _ := h.runner.Wait(context.Background(), c.JobID); j == nil || j.FinishedAt == nil {
		t.Errorf("job should finish after cancel: %+v", j)
	}
}

func TestMaintenanceWindowGatesDelivery(t *testing.T) {
	h := newHarness(t)
	pkg := h.pkg("Acme", nil, "3.0", "")
	devs := []*model.Device{h.device("a", "Acme", "X1", "2.0"), h.device("b", "Acme", "X1", "2.0")}
	var mu sync.Mutex
	now := time.Date(2026, 10, 27, 12, 0, 0, 0, time.UTC) // Tuesday noon
	h.svc.Clock = func() time.Time { mu.Lock(); defer mu.Unlock(); return now }
	c := h.campaign(pkg, devs, func(c *model.Campaign) {
		c.CanaryCount = 0
		c.Window = &model.MaintenanceWindow{Days: []string{"tue", "wed"}, Start: "22:00", End: "06:00", TZ: "UTC"}
	})
	h.start(c, false)
	h.waitFor(status(model.CampaignScheduled))
	time.Sleep(60 * time.Millisecond)
	if n := len(h.upd.Calls()); n != 0 {
		t.Fatalf("outside the window nothing may be delivered: %d calls", n)
	}
	if _, rows := h.waitFor(status(model.CampaignScheduled)); Summarize(rows).Pending != 2 {
		t.Fatalf("devices must wait: %+v", Summarize(rows))
	}
	mu.Lock()
	now = time.Date(2026, 10, 28, 1, 30, 0, 0, time.UTC) // Wednesday 01:30, inside Tuesday's overnight window
	mu.Unlock()
	c, rows := h.waitFor(status(model.CampaignCompleted))
	if sum := Summarize(rows); sum.Succeeded != 2 || c.StartedAt == nil {
		t.Fatalf("%+v", sum)
	}
}

func TestScheduledAtWaits(t *testing.T) {
	h := newHarness(t)
	pkg := h.pkg("Acme", nil, "3.0", "")
	devs := []*model.Device{h.device("a", "Acme", "X1", "2.0")}
	var mu sync.Mutex
	now := time.Date(2026, 10, 27, 12, 0, 0, 0, time.UTC)
	h.svc.Clock = func() time.Time { mu.Lock(); defer mu.Unlock(); return now }
	at := now.Add(time.Hour)
	c := h.campaign(pkg, devs, func(c *model.Campaign) { c.CanaryCount = 0; c.ScheduledAt = &at })
	h.start(c, false)
	h.waitFor(status(model.CampaignScheduled))
	time.Sleep(40 * time.Millisecond)
	if len(h.upd.Calls()) != 0 {
		t.Fatal("delivered before the scheduled time")
	}
	mu.Lock()
	now = at.Add(time.Minute)
	mu.Unlock()
	h.waitFor(status(model.CampaignCompleted))
}

func TestInWindow(t *testing.T) {
	w := &model.MaintenanceWindow{Days: []string{"Mon", "tue"}, Start: "22:00", End: "06:00", TZ: "UTC"}
	cases := []struct {
		at   time.Time
		want bool
	}{
		{time.Date(2026, 10, 26, 23, 0, 0, 0, time.UTC), true},  // Monday 23:00
		{time.Date(2026, 10, 27, 2, 0, 0, 0, time.UTC), true},   // Tuesday 02:00 (Monday's window)
		{time.Date(2026, 10, 27, 12, 0, 0, 0, time.UTC), false}, // Tuesday noon
		{time.Date(2026, 10, 28, 2, 0, 0, 0, time.UTC), true},   // Wednesday 02:00 (Tuesday's window)
		{time.Date(2026, 10, 28, 23, 0, 0, 0, time.UTC), false}, // Wednesday night: not a window day
		{time.Date(2026, 10, 29, 2, 0, 0, 0, time.UTC), false},  // Thursday 02:00: Wednesday is not a window day
		{time.Date(2026, 10, 26, 21, 59, 0, 0, time.UTC), false},
		{time.Date(2026, 10, 27, 6, 0, 0, 0, time.UTC), false}, // end is exclusive
	}
	for _, c := range cases {
		if got := InWindow(w, c.at); got != c.want {
			t.Errorf("InWindow(%s) = %v, want %v", c.at.Format(time.RFC1123), got, c.want)
		}
	}
	day := &model.MaintenanceWindow{Start: "09:00", End: "17:00", TZ: "UTC"}
	if !InWindow(day, time.Date(2026, 10, 27, 10, 0, 0, 0, time.UTC)) || InWindow(day, time.Date(2026, 10, 27, 18, 0, 0, 0, time.UTC)) {
		t.Error("same-day window")
	}
	if !InWindow(nil, time.Now()) || !InWindow(&model.MaintenanceWindow{}, time.Now()) {
		t.Error("empty window is always open")
	}
	if err := ValidateWindow(&model.MaintenanceWindow{Start: "22:00"}); !errors.Is(err, ErrInvalid) {
		t.Errorf("half window: %v", err)
	}
	if err := ValidateWindow(&model.MaintenanceWindow{Days: []string{"funday"}}); !errors.Is(err, ErrInvalid) {
		t.Errorf("bad day: %v", err)
	}
	if err := ValidateWindow(&model.MaintenanceWindow{Start: "25:00", End: "06:00"}); !errors.Is(err, ErrInvalid) {
		t.Errorf("bad time: %v", err)
	}
}

// Spec §14 safety: a dry run changes nothing on any device.
func TestDryRunDeliversNothing(t *testing.T) {
	h := newHarness(t)
	pkg := h.pkg("Acme", nil, "3.0", "")
	devs := []*model.Device{h.device("a", "Acme", "X1", "2.0"), h.device("b", "Acme", "X1", "2.0"), h.device("c", "Acme", "X1", "2.0"), h.device("d", "Acme", "X1", "2.0")}
	c := h.campaign(pkg, devs, func(c *model.Campaign) { c.CanaryCount = 1 })
	h.start(c, true)
	c, _ = h.waitFor(func(c *model.Campaign, _ []*model.CampaignDevice) bool {
		return c.Status == model.CampaignPaused && c.PauseReason == model.PauseCanary
	})
	if !c.DryRun {
		t.Fatal("dry run flag lost")
	}
	if _, err := h.svc.Confirm(context.Background(), c.ID, "op"); err != nil {
		t.Fatal(err)
	}
	_, rows := h.waitFor(status(model.CampaignCompleted))
	calls := h.upd.Calls()
	if len(calls) != 4 {
		t.Fatalf("calls = %+v", calls)
	}
	for _, call := range calls {
		if !call.DryRun {
			t.Fatalf("dry run made a real delivery call: %+v", call)
		}
	}
	for _, r := range rows {
		if r.Status != model.CampaignDeviceSucceeded || !strings.HasPrefix(r.Message, "dry run:") {
			t.Errorf("row %+v", r)
		}
	}
	for _, d := range devs {
		got, _ := h.st.GetDevice(context.Background(), d.ID)
		if got.Firmware != "2.0" {
			t.Errorf("device %s changed to %s during a dry run", d.ID, got.Firmware)
		}
	}
	if n := len(h.audits(AuditUpdate)); n != 0 {
		t.Errorf("dry run must not audit deliveries: %d", n)
	}
}

func TestVerificationOutcomes(t *testing.T) {
	h := newHarness(t)
	pkg := h.pkg("Acme", nil, "3.0", "")
	slow := h.device("slow", "Acme", "X1", "2.0")     // reports the new version after several polls
	never := h.device("never", "Acme", "X1", "2.0")   // accepts but never applies -> timeout
	down := h.device("down", "Acme", "X1", "2.0")     // offline at pre-flight
	busy := h.device("busy", "Acme", "X1", "2.0")     // printing at pre-flight
	broken := h.device("broken", "Acme", "X1", "2.0") // driver error
	h.dev.polls[slow.ID] = 4
	h.dev.never[never.ID] = true
	h.dev.offline[down.ID] = true
	h.dev.state[busy.ID] = "printing"
	h.upd.fail[broken.ID] = errors.New("connection refused")
	c := h.campaign(pkg, []*model.Device{slow, never, down, busy, broken}, func(c *model.Campaign) { c.CanaryCount = 0; c.Concurrency = 5; c.VerifyTimeoutSecs = 1 })
	h.start(c, false)
	c, rows := h.waitFor(status(model.CampaignCompleted))
	r := byDevice(rows)
	if r[slow.ID].Status != model.CampaignDeviceSucceeded || !strings.Contains(r[slow.ID].Message, "3.0") {
		t.Errorf("slow: %+v", r[slow.ID])
	}
	if r[never.ID].Status != model.CampaignDeviceFailed || !strings.Contains(r[never.ID].Message, "timed out") || !strings.Contains(r[never.ID].Message, "device reports 2.0") {
		t.Errorf("never: %+v", r[never.ID])
	}
	if r[down.ID].Status != model.CampaignDeviceFailed || !strings.Contains(r[down.ID].Message, "pre-flight") || !strings.Contains(r[down.ID].Message, "unreachable") {
		t.Errorf("down: %+v", r[down.ID])
	}
	if r[busy.ID].Status != model.CampaignDeviceFailed || !strings.Contains(r[busy.ID].Message, "busy") {
		t.Errorf("busy: %+v", r[busy.ID])
	}
	if r[broken.ID].Status != model.CampaignDeviceFailed || !strings.Contains(r[broken.ID].Message, "connection refused") {
		t.Errorf("broken: %+v", r[broken.ID])
	}
	// Pre-flight failures never reach the driver; driver errors are audited.
	for _, call := range h.upd.Calls() {
		if call.DeviceID == down.ID || call.DeviceID == busy.ID {
			t.Errorf("pre-flight failure reached the driver: %+v", call)
		}
	}
	audits := h.audits(AuditUpdate)
	if len(audits) != 3 {
		t.Fatalf("audits = %d, want 3 (slow, never, broken)", len(audits))
	}
	results := map[string]string{}
	for _, a := range audits {
		results[a.Target] = a.Result
	}
	if results[slow.ID] != "accepted" || results[never.ID] != "accepted" || !strings.HasPrefix(results[broken.ID], "error:") {
		t.Errorf("audit results %v", results)
	}
	j, _ := h.runner.Wait(context.Background(), c.JobID)
	if j.Status != model.JobPartial || j.Failed != 4 || j.Done != 5 {
		t.Errorf("job %+v", j)
	}
	// The verification verdict uses the configuration vocabulary (audit A20):
	// verified / diverged / unverifiable, in the row message and the job result.
	if !strings.HasPrefix(r[slow.ID].Message, "verified:") || !strings.HasPrefix(r[never.ID].Message, "diverged:") {
		t.Errorf("verdict prefixes: slow=%q never=%q", r[slow.ID].Message, r[never.ID].Message)
	}
	outcomes := map[string]any{}
	jr, _ := h.st.ListJobResults(context.Background(), c.JobID)
	for _, res := range jr {
		outcomes[res.DeviceID] = res.Data["outcome"]
	}
	if outcomes[slow.ID] != "verified" || outcomes[never.ID] != "diverged" || outcomes[down.ID] != nil {
		t.Errorf("job result outcomes = %v", outcomes)
	}
}

// Audit A19: a real delivery needs a write credential bound to the device; a
// dry run only validates and works with read credentials.
func TestDeliveryRefusedWithoutWriteCredential(t *testing.T) {
	h := newHarness(t)
	if err := h.st.DeleteCredential(context.Background(), h.writeCred.ID); err != nil {
		t.Fatal(err)
	}
	pkg := h.pkg("Acme", nil, "3.0", "")
	dev := h.device("ro", "Acme", "X1", "2.0")
	c := h.campaign(pkg, []*model.Device{dev}, func(c *model.Campaign) { c.CanaryCount = 0 })
	h.start(c, true)
	_, rows := h.waitFor(status(model.CampaignCompleted))
	if r := byDevice(rows)[dev.ID]; r.Status != model.CampaignDeviceSucceeded || !strings.HasPrefix(r.Message, "dry run") {
		t.Fatalf("dry run must work without a write credential: %+v", r)
	}
	c2 := h.campaign(pkg, []*model.Device{dev}, func(c *model.Campaign) { c.CanaryCount = 0 })
	h.start(c2, false)
	_, rows = h.waitForID(c2.ID, status(model.CampaignFailed))
	r := byDevice(rows)[dev.ID]
	if r.Status != model.CampaignDeviceFailed || !strings.Contains(r.Message, "no write credential bound") {
		t.Fatalf("delivery without a write credential: %+v", r)
	}
	for _, call := range h.upd.Calls() {
		if !call.DryRun {
			t.Errorf("the driver was asked to deliver without a write credential: %+v", call)
		}
	}
	var refused bool
	for _, a := range h.audits(AuditUpdate) {
		if a.Target == dev.ID && a.Result == "refused" {
			refused = true
		}
	}
	if !refused {
		t.Error("refusal not audited")
	}
}

func TestCompatibilityAndSkips(t *testing.T) {
	h := newHarness(t)
	pkg := h.pkg("Acme", []string{"4200"}, "2.15.0", "2.10")
	ok := h.device("ok", "Acme", "Acme LaserPro 4200", "2.14.7")
	current := h.device("current", "Acme", "Acme 4200", "v2.15.0")
	other := h.device("other", "Globex", "Acme 4200", "2.14.7")
	wrongModel := h.device("wrongmodel", "Acme", "Acme InkJet 100", "2.14.7")
	old := h.device("old", "Acme", "Acme 4200", "2.0")
	newer := h.device("newer", "Acme", "Acme 4200", "2.16")
	all := []*model.Device{ok, current, other, wrongModel, old, newer}
	c := h.campaign(pkg, all, func(c *model.Campaign) { c.CanaryCount = 0 })
	h.start(c, false)
	_, rows := h.waitFor(status(model.CampaignCompleted))
	r := byDevice(rows)
	if r[ok.ID].Status != model.CampaignDeviceSucceeded {
		t.Errorf("ok: %+v", r[ok.ID])
	}
	for _, d := range []*model.Device{current, other, wrongModel, old, newer} {
		if r[d.ID].Status != model.CampaignDeviceSkipped || r[d.ID].Message == "" {
			t.Errorf("%s: %+v", d.Name(), r[d.ID])
		}
	}
	if !strings.Contains(r[newer.ID].Message, "downgrade") || !strings.Contains(r[old.ID].Message, "minimum") || !strings.Contains(r[current.ID].Message, "already") {
		t.Errorf("messages: newer=%q old=%q current=%q", r[newer.ID].Message, r[old.ID].Message, r[current.ID].Message)
	}
	if calls := h.upd.Calls(); len(calls) != 1 || calls[0].DeviceID != ok.ID {
		t.Errorf("only the compatible device may be delivered to: %+v", calls)
	}
	// Downgrade is allowed explicitly (rollback).
	back := h.pkg("Acme", []string{"4200"}, "2.14.7", "")
	c2 := h.campaign(back, []*model.Device{newer}, func(c *model.Campaign) { c.CanaryCount = 0; c.AllowDowngrade = true })
	h.start(c2, false)
	_, rows = h.waitForID(c2.ID, status(model.CampaignCompleted))
	if r := byDevice(rows)[newer.ID]; r.Status != model.CampaignDeviceSucceeded {
		t.Errorf("downgrade: %+v", r)
	}
}

func TestPackagesIntegrityAndDeleteRefusal(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	if _, err := h.svc.RegisterPackage(ctx, PackageMeta{Vendor: "Acme", FileName: "x.bin"}, strings.NewReader("abc"), "admin"); !errors.Is(err, ErrInvalid) {
		t.Errorf("version required: %v", err)
	}
	if _, err := h.svc.RegisterPackage(ctx, PackageMeta{Version: "1", FileName: "x.bin"}, strings.NewReader(""), "admin"); !errors.Is(err, ErrInvalid) {
		t.Errorf("empty file: %v", err)
	}
	pkg, err := h.svc.RegisterPackage(ctx, PackageMeta{Vendor: "Acme", Models: []string{" 4200 ", ""}, Version: "2.15.0", FileName: "../../evil/fw 2.15.bin"}, strings.NewReader("image bytes"), "admin")
	if err != nil {
		t.Fatal(err)
	}
	if pkg.FileName != "fw_2.15.bin" || pkg.Size != 11 || len(pkg.SHA256) != 64 || len(pkg.Models) != 1 || pkg.Models[0] != "4200" {
		t.Errorf("package %+v", pkg)
	}
	// The stored path is relative to the firmware root (H-018) so a restore
	// into another data directory still resolves it; PackagePath is absolute
	// and inside the root.
	if filepath.IsAbs(pkg.StoragePath) || !strings.HasPrefix(h.svc.PackagePath(pkg), h.svc.Dir()) {
		t.Errorf("storage path %q, resolved %q", pkg.StoragePath, h.svc.PackagePath(pkg))
	}
	if err := h.svc.VerifyPackage(pkg); err != nil {
		t.Fatal(err)
	}
	// A legacy absolute path that no longer exists is rebased onto the root.
	legacy := *pkg
	legacy.StoragePath = "/var/lib/old-printmarshal/firmware/" + pkg.ID + "/" + pkg.FileName
	if got := h.svc.PackagePath(&legacy); got != h.svc.PackagePath(pkg) {
		t.Errorf("legacy path resolved to %q", got)
	}
	dev := h.device("a", "Acme", "4200", "2.14")
	c := h.campaign(pkg, []*model.Device{dev}, nil)
	if err := h.svc.DeletePackage(ctx, pkg.ID); !errors.Is(err, ErrConflict) {
		t.Errorf("delete with draft campaign: %v", err)
	}
	// Tampering is caught before anything starts.
	if err := os.WriteFile(h.svc.PackagePath(pkg), []byte("tampered"), 0o640); err != nil {
		t.Fatal(err)
	}
	if err := h.svc.VerifyPackage(pkg); err == nil || !strings.Contains(err.Error(), "integrity") {
		t.Errorf("tampered file passed: %v", err)
	}
	if _, err := h.svc.Start(ctx, c.ID, "op", StartOptions{}); !errors.Is(err, ErrInvalid) {
		t.Errorf("start with a tampered package: %v", err)
	}
	if _, err := h.svc.Cancel(ctx, c.ID, "op"); err != nil {
		t.Fatal(err)
	}
	if err := h.svc.DeletePackage(ctx, pkg.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(pkg.StoragePath); !os.IsNotExist(err) {
		t.Error("package file not removed")
	}
	if _, err := h.svc.GetPackage(ctx, pkg.ID); !errors.Is(err, store.ErrNotFound) {
		t.Errorf("after delete: %v", err)
	}
}

func TestInventoryAndOutdated(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	h.device("a", "Acme", "Acme 4200", "2.14.7")
	h.device("b", "Acme", "Acme 4200", "2.14.7")
	h.device("c", "Acme", "Acme 4200", "2.15.0")
	h.device("d", "Globex", "G1", "1.0")
	h.device("e", "Globex", "G1", "")
	h.pkg("Acme", []string{"4200"}, "2.15.0", "")
	h.pkg("Acme", []string{"4200"}, "2.14.0", "")
	inv, err := h.svc.Inventory(ctx, InventoryFilter{})
	if err != nil {
		t.Fatal(err)
	}
	if inv.Devices != 5 || inv.Outdated != 2 || inv.Unknown != 1 || len(inv.Entries) != 4 {
		t.Fatalf("%+v", inv)
	}
	for _, e := range inv.Entries {
		switch {
		case e.Model == "Acme 4200" && e.Version == "2.14.7":
			if e.Count != 2 || !e.Outdated || e.Latest != "2.15.0" {
				t.Errorf("entry %+v", e)
			}
		case e.Model == "Acme 4200" && e.Version == "2.15.0":
			if e.Count != 1 || e.Outdated || e.Latest != "2.15.0" {
				t.Errorf("entry %+v", e)
			}
		case e.Make == "Globex":
			if e.Outdated || e.Latest != "" {
				t.Errorf("entry %+v", e)
			}
		default:
			t.Errorf("unexpected entry %+v", e)
		}
	}
	out, err := h.svc.Outdated(ctx)
	if err != nil || len(out) != 2 || out[0].Installed != "2.14.7" || out[0].Latest != "2.15.0" || out[0].PackageID == "" {
		t.Errorf("outdated %+v err=%v", out, err)
	}
	if inv, _ := h.svc.Inventory(ctx, InventoryFilter{Make: "Globex"}); inv.Devices != 2 {
		t.Errorf("filtered inventory %+v", inv)
	}
}

func TestRestartRecovery(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	pkg := h.pkg("Acme", nil, "3.0", "")
	a := h.device("a", "Acme", "X1", "2.0")
	b := h.device("b", "Acme", "X1", "2.0")
	// Fake what a crashed process leaves behind: a running campaign with one
	// device mid-update and one still pending.
	c := &model.Campaign{Name: "crashed", PackageID: pkg.ID, DeviceIDs: []string{a.ID, b.ID}, CanaryCount: 0, Concurrency: 1,
		Status: model.CampaignRunning, Wave: 1, StartedBy: "operator"}
	if err := h.st.CreateCampaign(ctx, c); err != nil {
		t.Fatal(err)
	}
	_ = h.st.UpsertCampaignDevice(ctx, &model.CampaignDevice{CampaignID: c.ID, DeviceID: a.ID, Wave: 1, Status: model.CampaignDeviceVerifying, ToVersion: "3.0"})
	_ = h.st.UpsertCampaignDevice(ctx, &model.CampaignDevice{CampaignID: c.ID, DeviceID: b.ID, Wave: 1, Status: model.CampaignDevicePending, ToVersion: "3.0"})

	svc2 := New(h.svc.Deps) // a new process starts
	svc2.Poll, svc2.VerifyInterval, svc2.ControlPoll, svc2.RetryDelay = h.dev.Poll, 5*time.Millisecond, 10*time.Millisecond, time.Millisecond
	got, _ := h.st.GetCampaign(ctx, c.ID)
	if got.Status != model.CampaignPaused || got.PauseReason != model.PauseInterrupted {
		t.Fatalf("after restart: %+v", got)
	}
	rows, _ := h.st.ListCampaignDevices(ctx, c.ID)
	r := byDevice(rows)
	if r[a.ID].Status != model.CampaignDeviceFailed || !strings.Contains(r[a.ID].Message, "interrupted") || r[b.ID].Status != model.CampaignDevicePending {
		t.Fatalf("rows %+v / %+v", r[a.ID], r[b.ID])
	}
	// Resume submits a fresh job (none is live) and finishes the pending device.
	if _, err := svc2.Resume(ctx, c.ID, "op"); err != nil {
		t.Fatal(err)
	}
	h.waitForID(c.ID, status(model.CampaignCompleted))
	rows, _ = h.st.ListCampaignDevices(ctx, c.ID)
	if r := byDevice(rows); r[b.ID].Status != model.CampaignDeviceSucceeded || r[a.ID].Status != model.CampaignDeviceFailed {
		t.Errorf("after resume %+v / %+v", r[a.ID], r[b.ID])
	}
	hist, _ := h.st.FirmwareHistory(ctx, b.ID, 10)
	if len(hist) != 1 || hist[0].CampaignName != "crashed" || hist[0].Status != model.CampaignDeviceSucceeded {
		t.Errorf("history %+v", hist)
	}
}

func TestValidationAndGroups(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	pkg := h.pkg("Acme", nil, "3.0", "")
	bad := []*model.Campaign{
		{Name: "", PackageID: pkg.ID, DeviceIDs: []string{"x"}},
		{Name: "n", PackageID: "nope", DeviceIDs: []string{"x"}},
		{Name: "n", PackageID: pkg.ID},
		{Name: "n", PackageID: pkg.ID, GroupID: "nope"},
		{Name: "n", PackageID: pkg.ID, DeviceIDs: []string{"x"}, FailureThresholdPercent: 101},
		{Name: "n", PackageID: pkg.ID, DeviceIDs: []string{"x"}, Window: &model.MaintenanceWindow{Start: "1"}},
	}
	for i, c := range bad {
		if err := h.svc.CreateCampaign(ctx, c, "op"); !errors.Is(err, ErrInvalid) {
			t.Errorf("case %d: %v", i, err)
		}
	}
	// Unknown device ids resolve to no targets and the start is refused.
	ghost := h.campaign(pkg, nil, func(c *model.Campaign) { c.DeviceIDs = []string{"does-not-exist"} })
	if _, err := h.svc.Start(ctx, ghost.ID, "op", StartOptions{}); !errors.Is(err, ErrInvalid) {
		t.Errorf("no targets: %v", err)
	}
	// Rule groups and static groups both resolve; decommissioned devices are excluded.
	a := h.device("a", "Acme", "X1", "2.0")
	b := h.device("b", "Acme", "X1", "2.0")
	gone := h.device("gone", "Acme", "X1", "2.0")
	_, _ = h.st.SetDeviceState(ctx, gone.ID, model.StateDecommissioned)
	rule := &model.Group{Name: "acme", Kind: "rule", Rule: &model.GroupRule{MakeContains: "acme"}}
	if err := h.st.CreateGroup(ctx, rule); err != nil {
		t.Fatal(err)
	}
	c := h.campaign(pkg, nil, func(c *model.Campaign) { c.GroupID = rule.ID; c.Concurrency = 0; c.CanaryCount = -5 })
	if c.Concurrency != DefaultConcurrency || c.CanaryCount != CanaryDefault {
		t.Errorf("defaults %+v", c)
	}
	targets, err := h.svc.Targets(ctx, c)
	if err != nil || len(targets) != 2 || targets[0].ID != a.ID || targets[1].ID != b.ID {
		t.Errorf("rule targets %v err=%v", targets, err)
	}
	static := &model.Group{Name: "static", Kind: "static"}
	_ = h.st.CreateGroup(ctx, static)
	_ = h.st.SetGroupMembers(ctx, static.ID, []string{b.ID, gone.ID})
	c2 := h.campaign(pkg, nil, func(c *model.Campaign) { c.GroupID = static.ID })
	if targets, _ := h.svc.Targets(ctx, c2); len(targets) != 1 || targets[0].ID != b.ID {
		t.Errorf("static targets %v", targets)
	}
	v, err := h.svc.View(ctx, c)
	if err != nil || !strings.Contains(v.RecoveryNotes, "vendor") || v.PackageVersion != "3.0" {
		t.Errorf("view %+v err=%v", v, err)
	}
	if !strings.Contains(RecoveryNotes("Unknown Corp"), "no verified recovery procedure") || !strings.Contains(RecoveryNotes("sim"), "pm-sim") {
		t.Error("recovery notes")
	}
}
