package configmgmt

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"strings"
	"sync"
	"testing"
	"time"

	"printmarshal/internal/config"
	"printmarshal/internal/driver/snmp"
	"printmarshal/internal/engine"
	"printmarshal/internal/fleet"
	"printmarshal/internal/jobs"
	"printmarshal/internal/model"
	"printmarshal/internal/secrets"
	"printmarshal/internal/store"
)

type testEnv struct {
	svc    *Service
	st     store.Store
	fx     *snmp.Fixture
	dev    *model.Device
	events *[]model.Event
	mu     *sync.Mutex
	// writeCred is the global write credential bound by newTestEnv (audit A19);
	// tests delete it to prove a real apply is refused without one.
	writeCred string
}

// fakeConfigDriver is an in-memory read_config/write_config driver whose
// read-back can disagree with what it accepted (audit A20): keys in stick are
// "accepted" by the device but never change, keys in hidden are writable but
// never reported by the reader.
type fakeConfigDriver struct {
	mu     sync.Mutex
	doc    map[string]any
	stick  map[string]bool
	hidden map[string]bool
	writes []model.SettingChange
}

func (f *fakeConfigDriver) Name() string { return "fake" }
func (f *fakeConfigDriver) Capabilities() []model.Capability {
	return []model.Capability{model.CapReadConfig, model.CapWriteConfig}
}
func (f *fakeConfigDriver) Supports(d *model.Device, c model.Capability) bool {
	return engine.ProfileSupports(d, "fake", c)
}
func (f *fakeConfigDriver) Priority(model.Capability) int { return 10 }

func (f *fakeConfigDriver) ReadConfig(_ context.Context, d *model.Device, _ model.Creds, _ model.ConfigSelector) (*model.ConfigDocument, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	doc := &model.ConfigDocument{DeviceID: d.ID, ReadAt: time.Now()}
	for k, v := range f.doc {
		if f.hidden[k] {
			continue
		}
		def := model.CatalogByKey[k]
		doc.Settings = append(doc.Settings, model.Setting{Key: k, Area: def.Area, Name: def.Name, Type: def.Type, Value: v, Writable: true, Source: "fake"})
	}
	return doc, nil
}

func (f *fakeConfigDriver) WriteConfig(_ context.Context, d *model.Device, _ model.Creds, changes []model.SettingChange, opts model.WriteOptions) (*model.WriteResult, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	res := &model.WriteResult{DeviceID: d.ID, DryRun: opts.DryRun, At: time.Now()}
	for _, c := range changes {
		r := model.SettingResult{Key: c.Key, Before: f.doc[c.Key], After: c.Value, Driver: "fake"}
		if opts.DryRun {
			r.Status = model.SettingWouldWrite
		} else {
			// The device says "ok" to everything; sticky keys silently keep their value.
			r.Status = model.SettingWritten
			res.Rollback = append(res.Rollback, model.SettingChange{Key: c.Key, Value: f.doc[c.Key]})
			if !f.stick[c.Key] {
				f.doc[c.Key] = c.Value
			}
			f.writes = append(f.writes, c)
		}
		res.Results = append(res.Results, r)
	}
	return res, nil
}

// bindWriteCredential stores a global write credential so real applies are allowed.
func bindWriteCredential(t *testing.T, st store.Store) string {
	t.Helper()
	c := &model.Credential{Name: "snmp write", Type: model.CredSNMPCommunity, Purpose: model.PurposeWrite, Priority: 5}
	if err := st.CreateCredential(context.Background(), c, &model.Secret{Community: "private"}); err != nil {
		t.Fatal(err)
	}
	return c.ID
}

// newTestEnv wires the service over a fixture-backed SNMP dialer, like
// internal/fleet/service_test.go, and identifies one device.
func newTestEnv(t *testing.T) *testEnv {
	t.Helper()
	fx, err := snmp.LoadFixture("../../testdata/fixtures/generic-laser.snmp")
	if err != nil {
		t.Fatal(err)
	}
	fx.Writable = true
	box, _ := secrets.New(make([]byte, 32))
	st, err := store.OpenSQLite(t.TempDir()+"/t.db", box)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	reg := engine.NewRegistry()
	reg.Register(snmp.New(func(ctx context.Context, address string, creds model.Creds) (snmp.Session, string, error) {
		return fx, "v2c", nil
	}))
	eng := engine.New(reg, log, 2)
	cfg := config.Default()
	var mu sync.Mutex
	var events []model.Event
	emit := func(e model.Event) {
		mu.Lock()
		events = append(events, e)
		mu.Unlock()
	}
	runner := jobs.New(st, log, 2, emit)
	t.Cleanup(runner.Stop)
	fl := fleet.New(st, eng, runner, cfg, log, emit, nil)
	svc := New(fl.DepsOf())
	ident, caps, err := eng.Identify(context.Background(), "10.0.0.5", model.Creds{})
	if err != nil {
		t.Fatal(err)
	}
	dev, _, err := st.UpsertDevice(context.Background(), ident, caps, []string{"scan"})
	if err != nil {
		t.Fatal(err)
	}
	if dev.Capabilities[model.CapReadConfig] != "snmp" || dev.Capabilities[model.CapWriteConfig] != "snmp" {
		t.Fatalf("capability profile = %v", dev.Capabilities)
	}
	return &testEnv{svc: svc, st: st, fx: fx, dev: dev, events: &events, mu: &mu, writeCred: bindWriteCredential(t, st)}
}

func (e *testEnv) wait(t *testing.T, j *model.Job) *model.Job {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	done, err := e.svc.Jobs.Wait(ctx, j.ID)
	if err != nil {
		t.Fatal(err)
	}
	return done
}

func (e *testEnv) eventTypes() map[string]int {
	e.mu.Lock()
	defer e.mu.Unlock()
	m := map[string]int{}
	for _, ev := range *e.events {
		m[ev.Type]++
	}
	return m
}

func TestReadConfigSnapshotAndDrift(t *testing.T) {
	env := newTestEnv(t)
	ctx := context.Background()
	doc, err := env.svc.ReadConfig(ctx, env.dev.ID, model.ConfigSelector{})
	if err != nil {
		t.Fatal(err)
	}
	if v, ok := doc.Get("identity.location"); !ok || v.Value != "Floor 2 east wing" || v.Source != "snmp" {
		t.Errorf("location = %+v", v)
	}
	if len(doc.Sources) != 1 || doc.Sources[0] != "snmp" {
		t.Errorf("sources = %v", doc.Sources)
	}
	sn, err := env.st.LatestConfigSnapshot(ctx, env.dev.ID)
	if err != nil || sn.Document == nil || len(sn.Document.Settings) != len(doc.Settings) {
		t.Fatalf("snapshot = %+v err=%v", sn, err)
	}
	// Drift against a profile: one match, one drifted, one missing.
	p := &model.Profile{Name: "site", Settings: []model.SettingChange{
		{Key: "identity.location", Value: "Floor 2 east wing"},
		{Key: "identity.hostname", Value: "OTHER"},
		{Key: "energy.sleepDelayMinutes", Value: 5},
	}}
	if err := env.st.CreateProfile(ctx, p); err != nil {
		t.Fatal(err)
	}
	r, err := env.svc.Drift(ctx, env.dev.ID, p.ID, false)
	if err != nil {
		t.Fatal(err)
	}
	if r.Matching != 1 || r.Drifted != 1 || len(r.Items) != 2 {
		t.Errorf("drift = %+v", r)
	}
	missing := 0
	for _, it := range r.Items {
		if it.Missing {
			missing++
		}
	}
	if missing != 1 {
		t.Errorf("expected one missing item: %+v", r.Items)
	}
	reports, err := env.st.ListDriftReports(ctx, p.ID, 0)
	if err != nil || len(reports) != 1 {
		t.Errorf("cached drift reports = %d err=%v", len(reports), err)
	}
	// Device-vs-device diff of the same device is empty.
	d := Diff(doc, doc)
	if len(d.Items) != 0 || d.Same != len(doc.Settings) {
		t.Errorf("self diff = %+v", d)
	}
}

// Safety (spec §14): dry run changes nothing on the device, reports per setting
// and is audited as a rehearsal; the real apply writes, audits with before/after
// (no secrets), records a rollback and can be reverted.
func TestApplyDryRunThenRealThenRevert(t *testing.T) {
	env := newTestEnv(t)
	ctx := context.Background()
	p := &model.Profile{Name: "lobby", Settings: []model.SettingChange{
		{Key: "identity.location", Value: "Lobby"},
		{Key: "identity.hostname", Value: "LP-FLOOR2-EAST"},
		{Key: "print.defaultCopies", Value: 2},
	}}
	if err := env.st.CreateProfile(ctx, p); err != nil {
		t.Fatal(err)
	}
	target := model.ConfigTarget{DeviceIDs: []string{env.dev.ID}}

	// Dry run.
	j, err := env.svc.Apply(ctx, ApplyRequest{ProfileID: p.ID, Target: target, DryRun: true}, "alice")
	if err != nil {
		t.Fatal(err)
	}
	done := env.wait(t, j)
	if done.Status != model.JobSucceeded || done.Done != 1 {
		t.Fatalf("dry-run job = %+v", done)
	}
	if n := len(env.fx.Sets()); n != 0 {
		t.Fatalf("dry run touched the device: %d sets", n)
	}
	writes, _ := env.st.ListWriteRecords(ctx, store.WriteRecordFilter{DeviceID: env.dev.ID})
	if len(writes) != 1 || !writes[0].Result.DryRun || writes[0].ProfileID != p.ID || writes[0].Actor != "alice" {
		t.Fatalf("dry-run record = %+v", writes)
	}
	if writes[0].Written != 1 || writes[0].Unchanged != 1 || writes[0].Skipped != 1 || writes[0].Reversible() {
		t.Errorf("dry-run counts = %+v", writes[0])
	}
	results, _ := env.st.ListJobResults(ctx, j.ID)
	if len(results) != 1 || results[0].Status != "ok" || !strings.Contains(results[0].Message, "dry run") {
		t.Errorf("job results = %+v", results)
	}

	// Real apply.
	j, err = env.svc.Apply(ctx, ApplyRequest{ProfileID: p.ID, Target: target}, "alice")
	if err != nil {
		t.Fatal(err)
	}
	done = env.wait(t, j)
	if done.Status != model.JobSucceeded {
		t.Fatalf("apply job = %+v", done)
	}
	if sets := env.fx.Sets(); len(sets) != 1 || sets[0].OID != snmp.OIDSysLocation {
		t.Fatalf("device sets = %+v", sets)
	}
	doc, _ := env.svc.ReadConfig(ctx, env.dev.ID, model.ConfigSelector{})
	if v, _ := doc.Get("identity.location"); v.Value != "Lobby" {
		t.Errorf("device location after apply = %v", v.Value)
	}
	writes, _ = env.st.ListWriteRecords(ctx, store.WriteRecordFilter{DeviceID: env.dev.ID})
	var real *model.WriteRecord
	for _, w := range writes {
		if !w.Result.DryRun {
			real = w
		}
	}
	if real == nil || !real.Reversible() || real.Result.Rollback[0].Value != "Floor 2 east wing" || real.JobID != j.ID {
		t.Fatalf("real record = %+v", real)
	}
	// The written setting was read back from the device and matches (audit A20).
	if real.Verified != 1 || real.Diverged != 0 {
		t.Errorf("verified/diverged = %d/%d: %+v", real.Verified, real.Diverged, real.Result.Results)
	}
	for _, r := range real.Result.Results {
		if r.Status == model.SettingWritten && r.Outcome != model.OutcomeVerified {
			t.Errorf("written setting %s outcome = %q", r.Key, r.Outcome)
		}
	}
	if results, _ := env.st.ListJobResults(ctx, j.ID); len(results) != 1 || !strings.Contains(results[0].Message, "1 verified, 0 diverged") {
		t.Errorf("job result message = %+v", results)
	}

	// Audit: one entry per apply, with before/after, actor and target.
	audit, _ := env.st.ListAudit(ctx, 50)
	var applies []*model.AuditEntry
	for _, a := range audit {
		if a.Action == JobType {
			applies = append(applies, a)
		}
	}
	if len(applies) != 2 {
		t.Fatalf("audit entries = %d", len(applies))
	}
	latest := applies[0]
	if latest.Actor != "alice" || latest.Target != env.dev.ID || latest.Result != "ok" ||
		latest.Before["identity.location"] != "Floor 2 east wing" || latest.After["identity.location"] != "Lobby" {
		t.Errorf("audit = %+v", latest)
	}
	if applies[1].Result != "dry-run" {
		t.Errorf("dry run audit = %+v", applies[1])
	}
	ev := env.eventTypes()
	if ev["config.applied"] != 2 || ev["job.updated"] == 0 {
		t.Errorf("events = %v", ev)
	}

	// Revert.
	j, err = env.svc.Revert(ctx, real.ID, false, "bob")
	if err != nil {
		t.Fatal(err)
	}
	done = env.wait(t, j)
	if done.Status != model.JobSucceeded {
		t.Fatalf("revert job = %+v", done)
	}
	doc, _ = env.svc.ReadConfig(ctx, env.dev.ID, model.ConfigSelector{})
	if v, _ := doc.Get("identity.location"); v.Value != "Floor 2 east wing" {
		t.Errorf("device location after revert = %v", v.Value)
	}
	writes, _ = env.st.ListWriteRecords(ctx, store.WriteRecordFilter{DeviceID: env.dev.ID})
	if writes[0].RevertOf != real.ID || writes[0].Actor != "bob" {
		t.Errorf("revert record = %+v", writes[0])
	}
	if _, err := env.svc.Revert(ctx, writes[len(writes)-1].ID, false, "bob"); err == nil {
		t.Error("a dry run must not be revertible")
	}
}

// Safety (spec §14): secret-typed values never reach the job table, write
// records or the audit log.
func TestSecretsNeverPersisted(t *testing.T) {
	env := newTestEnv(t)
	ctx := context.Background()
	j, err := env.svc.Apply(ctx, ApplyRequest{Changes: []model.SettingChange{
		{Key: "protocols.snmp.writeCommunity", Value: "s3cretWriteComm"},
		{Key: "identity.location", Value: "Lobby"},
	}, Target: model.ConfigTarget{DeviceIDs: []string{env.dev.ID}}}, "alice")
	if err != nil {
		t.Fatal(err)
	}
	env.wait(t, j)
	stored, _ := env.st.GetJob(ctx, j.ID)
	for _, blob := range []any{stored.Params, stored.Target} {
		if b, _ := json.Marshal(blob); strings.Contains(string(b), "s3cretWriteComm") {
			t.Errorf("secret in job record: %s", b)
		}
	}
	writes, _ := env.st.ListWriteRecords(ctx, store.WriteRecordFilter{DeviceID: env.dev.ID})
	if b, _ := json.Marshal(writes); strings.Contains(string(b), "s3cretWriteComm") {
		t.Errorf("secret in write record: %s", b)
	}
	audit, _ := env.st.ListAudit(ctx, 50)
	if b, _ := json.Marshal(audit); strings.Contains(string(b), "s3cretWriteComm") {
		t.Errorf("secret in audit log: %s", b)
	}
	results, _ := env.st.ListJobResults(ctx, j.ID)
	if b, _ := json.Marshal(results); strings.Contains(string(b), "s3cretWriteComm") {
		t.Errorf("secret in job results: %s", b)
	}
	// The unsupported secret key is reported as skipped, the location written.
	if len(writes) != 1 || writes[0].Skipped != 1 || writes[0].Written != 1 {
		t.Errorf("record = %+v", writes[0])
	}
}

func TestApplyValidation(t *testing.T) {
	env := newTestEnv(t)
	ctx := context.Background()
	if _, err := env.svc.Apply(ctx, ApplyRequest{Changes: []model.SettingChange{{Key: "identity.location", Value: "x"}}}, "a"); err != ErrNoTarget {
		t.Errorf("empty target: %v", err)
	}
	if _, err := env.svc.Apply(ctx, ApplyRequest{Target: model.ConfigTarget{DeviceIDs: []string{env.dev.ID}}}, "a"); err != ErrNoChanges {
		t.Errorf("no changes: %v", err)
	}
	if _, err := env.svc.Apply(ctx, ApplyRequest{Changes: []model.SettingChange{{Key: "print.defaultCopies", Value: "many"}}, Target: model.ConfigTarget{DeviceIDs: []string{env.dev.ID}}}, "a"); err == nil {
		t.Error("bad value accepted")
	}
	if _, err := env.svc.Apply(ctx, ApplyRequest{Changes: []model.SettingChange{{Key: "identity.location", Value: "x"}}, Target: model.ConfigTarget{DeviceIDs: []string{"nope"}}}, "a"); err == nil {
		t.Error("unknown device accepted")
	}
	// Group targets resolve members.
	g := &model.Group{Name: "floor2", Kind: "static"}
	if err := env.st.CreateGroup(ctx, g); err != nil {
		t.Fatal(err)
	}
	if err := env.st.SetGroupMembers(ctx, g.ID, []string{env.dev.ID}); err != nil {
		t.Fatal(err)
	}
	devs, err := env.svc.ResolveTarget(ctx, model.ConfigTarget{GroupID: g.ID})
	if err != nil || len(devs) != 1 {
		t.Errorf("group target = %v err=%v", devs, err)
	}
	reports, err := env.svc.DriftGroup(ctx, model.ConfigTarget{GroupID: g.ID}, mustProfile(t, env, "p").ID, true)
	if err != nil || len(reports) != 1 {
		t.Errorf("drift group = %v err=%v", reports, err)
	}
}

func mustProfile(t *testing.T, env *testEnv, name string) *model.Profile {
	t.Helper()
	p := &model.Profile{Name: name, Settings: []model.SettingChange{{Key: "identity.location", Value: "Lobby"}}}
	if err := env.st.CreateProfile(context.Background(), p); err != nil {
		t.Fatal(err)
	}
	return p
}
