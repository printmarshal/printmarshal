package security

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"testing"
	"time"

	"printmarshal/internal/alerts"
	"printmarshal/internal/config"
	"printmarshal/internal/engine"
	"printmarshal/internal/fleet"
	"printmarshal/internal/jobs"
	"printmarshal/internal/model"
	"printmarshal/internal/secrets"
	"printmarshal/internal/store"
)

// fakeDriver is an in-memory security reader/writer that records every call,
// so tests can prove what was (not) written to the "device".
type fakeDriver struct {
	name     string
	mu       sync.Mutex
	doc      map[string]any
	writes   []model.SettingChange // real writes only
	dryRuns  int
	failKeys map[string]bool
	skipKeys map[string]bool
}

func newFake(name string, doc map[string]any) *fakeDriver {
	return &fakeDriver{name: name, doc: doc, failKeys: map[string]bool{}, skipKeys: map[string]bool{}}
}

func (f *fakeDriver) Name() string { return f.name }
func (f *fakeDriver) Capabilities() []model.Capability {
	return []model.Capability{model.CapIdentify, model.CapReadSecurity, model.CapWriteSecurity}
}
func (f *fakeDriver) Supports(d *model.Device, cap model.Capability) bool {
	return engine.ProfileSupports(d, f.name, cap)
}
func (f *fakeDriver) Priority(model.Capability) int { return 10 }

func (f *fakeDriver) ReadSecurity(_ context.Context, d *model.Device, _ model.Creds) (*model.ConfigDocument, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	doc := &model.ConfigDocument{DeviceID: d.ID, ReadAt: time.Now()}
	for k, v := range f.doc {
		def := model.CatalogByKey[k]
		doc.Settings = append(doc.Settings, model.Setting{Key: k, Area: def.Area, Name: def.Name, Type: def.Type, Value: v, Writable: true, Source: f.name})
	}
	return doc, nil
}

func (f *fakeDriver) WriteSecurity(_ context.Context, d *model.Device, _ model.Creds, changes []model.SettingChange, opts model.WriteOptions) (*model.WriteResult, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	res := &model.WriteResult{DeviceID: d.ID, DryRun: opts.DryRun, At: time.Now()}
	if opts.DryRun {
		f.dryRuns++
	}
	for _, c := range changes {
		r := model.SettingResult{Key: c.Key, Before: f.doc[c.Key], After: c.Value}
		switch {
		case f.skipKeys[c.Key]:
			r.Status, r.Error = model.SettingSkipped, "unsupported"
		case f.failKeys[c.Key]:
			r.Status, r.Error = model.SettingFailed, "device said no"
		case opts.DryRun:
			r.Status = model.SettingWouldWrite
		default:
			r.Status = model.SettingWritten
			res.Rollback = append(res.Rollback, model.SettingChange{Key: c.Key, Value: f.doc[c.Key]})
			f.doc[c.Key] = c.Value
			f.writes = append(f.writes, c)
		}
		res.Results = append(res.Results, r)
	}
	return res, nil
}

func (f *fakeDriver) value(key string) any {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.doc[key]
}

type harness struct {
	t      *testing.T
	st     store.Store
	svc    *Service
	runner *jobs.Runner
	reg    *engine.Registry
	logBuf *bytes.Buffer
}

func newHarness(t *testing.T) *harness {
	t.Helper()
	box, _ := secrets.New(make([]byte, 32))
	st, err := store.OpenSQLite(t.TempDir()+"/t.db", box)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	buf := &bytes.Buffer{}
	log := slog.New(slog.NewTextHandler(buf, &slog.HandlerOptions{Level: slog.LevelDebug}))
	cfg := config.Default()
	runner := jobs.New(st, log, 2, nil)
	t.Cleanup(runner.Stop)
	reg := engine.NewRegistry()
	eng := engine.New(reg, log, 4)
	fl := fleet.New(st, eng, runner, cfg, log, nil, alerts.NewNotifier(nil, st, log), nil)
	svc := New(fl.DepsOf())
	svc.TLSProbe, svc.PortProbe = nil, nil
	return &harness{t: t, st: st, svc: svc, runner: runner, reg: reg, logBuf: buf}
}

// device creates a device whose capability profile names the given drivers.
func (h *harness) device(addr string, caps map[model.Capability]string) *model.Device {
	h.t.Helper()
	d, _, err := h.st.UpsertDevice(context.Background(), &model.Identity{Address: addr, Serial: "SN-" + addr, Model: "Test", Method: "test"}, caps, []string{"test"})
	if err != nil {
		h.t.Fatal(err)
	}
	return d
}

func (h *harness) wait(j *model.Job) (*model.Job, []*model.JobResult) {
	h.t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	done, err := h.runner.Wait(ctx, j.ID)
	if err != nil {
		h.t.Fatal(err)
	}
	res, _ := h.st.ListJobResults(ctx, j.ID)
	return done, res
}

func policyWith(items ...model.PolicyItem) *model.Policy {
	p := &model.Policy{Name: "test-" + fmt.Sprint(time.Now().UnixNano()), Items: items}
	if err := ValidatePolicy(p); err != nil {
		panic(err)
	}
	return p
}

func TestBaselineCreatedOnFirstRun(t *testing.T) {
	h := newHarness(t)
	ps, err := h.st.ListPolicies(context.Background())
	if err != nil || len(ps) != 1 || ps[0].Name != BaselineName {
		t.Fatalf("baseline missing: %v %v", ps, err)
	}
	// A second service start must not duplicate it.
	if err := h.svc.EnsureBaseline(context.Background()); err != nil {
		t.Fatal(err)
	}
	ps, _ = h.st.ListPolicies(context.Background())
	if len(ps) != 1 {
		t.Fatalf("baseline duplicated: %d", len(ps))
	}
}

func TestAssessJobScoresAlertsAndEvents(t *testing.T) {
	h := newHarness(t)
	fake := newFake("fake", map[string]any{"protocols.telnet.enabled": true, "protocols.ftp.enabled": false, "security.tls.minVersion": "1.0"})
	h.reg.Register(fake)
	dev := h.device("10.0.0.1", map[model.Capability]string{model.CapReadSecurity: "fake", model.CapWriteSecurity: "fake"})
	var events []model.Event
	var mu sync.Mutex
	h.svc.Events = func(e model.Event) { mu.Lock(); events = append(events, e); mu.Unlock() }
	p := policyWith(
		model.PolicyItem{ID: "telnet", Key: "protocols.telnet.enabled", Operator: model.OpEq, Value: false, Severity: "critical", Remediate: true},
		model.PolicyItem{ID: "ftp", Key: "protocols.ftp.enabled", Operator: model.OpEq, Value: false, Severity: "critical", Remediate: true},
		model.PolicyItem{ID: "tls", Key: "security.tls.minVersion", Operator: model.OpGte, Value: "1.2", Severity: "high"},
		model.PolicyItem{ID: "v3", Key: "protocols.snmp.v3Enabled", Operator: model.OpEq, Value: true, Severity: "high"},
	)
	if err := h.st.CreatePolicy(context.Background(), p); err != nil {
		t.Fatal(err)
	}
	j, err := h.svc.Assess(context.Background(), p.ID, Target{DeviceIDs: []string{dev.ID}}, "tester")
	if err != nil {
		t.Fatal(err)
	}
	done, results := h.wait(j)
	if done.Status != model.JobSucceeded || len(results) != 1 || results[0].Status != "ok" {
		t.Fatalf("job %+v results %+v", done, results)
	}
	latest, err := h.st.LatestAssessments(context.Background(), p.ID, []string{dev.ID})
	if err != nil || len(latest) != 1 {
		t.Fatal(err, latest)
	}
	a := latest[0]
	// pass: ftp (3). fail: telnet (3), tls (2). unknown: v3.
	if a.Passed != 1 || a.Failed != 2 || a.Unknown != 1 || a.Compliant || a.Score != 37.5 {
		t.Errorf("assessment %+v", a)
	}
	alertsNow, _ := h.st.ListAlerts(context.Background(), store.AlertFilter{DeviceID: dev.ID, ActiveOnly: true})
	if len(alertsNow) != 1 || alertsNow[0].Key != "policy:"+p.ID+":telnet" || alertsNow[0].Source != AlertSource || alertsNow[0].Severity != model.SeverityCritical {
		t.Fatalf("expected one critical telnet alert, got %+v", alertsNow)
	}
	mu.Lock()
	var sawAssessed bool
	for _, e := range events {
		if e.Type == "security.assessed" {
			sawAssessed = true
		}
	}
	mu.Unlock()
	if !sawAssessed {
		t.Error("security.assessed event not published")
	}
	// Device fixed: alert clears on the next assessment, unrelated security alerts survive.
	_, _, _ = h.st.RaiseAlert(context.Background(), dev.ID, "cert:expiry", AlertSource, model.SeverityWarning, "unrelated")
	fake.doc["protocols.telnet.enabled"] = false
	if _, err := h.svc.AssessDevice(context.Background(), dev, p, ""); err != nil {
		t.Fatal(err)
	}
	alertsNow, _ = h.st.ListAlerts(context.Background(), store.AlertFilter{DeviceID: dev.ID, ActiveOnly: true})
	if len(alertsNow) != 1 || alertsNow[0].Key != "cert:expiry" {
		t.Fatalf("telnet alert should be cleared and cert:expiry kept, got %+v", alertsNow)
	}
}

// Spec §14 safety: a dry-run remediation changes nothing on the device and
// leaves no audit entry for a write.
func TestDryRunPerformsNoWrite(t *testing.T) {
	h := newHarness(t)
	fake := newFake("fake", map[string]any{"protocols.telnet.enabled": true, "protocols.ftp.enabled": true})
	h.reg.Register(fake)
	dev := h.device("10.0.0.2", map[model.Capability]string{model.CapReadSecurity: "fake", model.CapWriteSecurity: "fake", model.CapReadStatus: "snmp"})
	p := policyWith(
		model.PolicyItem{ID: "telnet", Key: "protocols.telnet.enabled", Operator: model.OpEq, Value: false, Severity: "critical", Remediate: true},
		model.PolicyItem{ID: "ftp", Key: "protocols.ftp.enabled", Operator: model.OpEq, Value: false, Severity: "critical", Remediate: true},
	)
	res, err := h.svc.RemediateDevice(context.Background(), dev, p, Options{DryRun: true}, "tester", "job1")
	if err != nil {
		t.Fatal(err)
	}
	if res.Written != 2 || res.Failed != 0 {
		t.Fatalf("dry run result %+v", res)
	}
	for _, it := range res.Items {
		if it.Status != model.RemediationWouldWrite {
			t.Errorf("item %s status %s", it.ItemID, it.Status)
		}
	}
	if len(fake.writes) != 0 || fake.dryRuns != 1 || fake.value("protocols.telnet.enabled") != true {
		t.Errorf("dry run touched the device: writes=%v dryRuns=%d", fake.writes, fake.dryRuns)
	}
	audit, _ := h.st.ListAudit(context.Background(), 100)
	for _, e := range audit {
		if e.Action == "security.remediate" {
			t.Errorf("dry run must not audit a write: %+v", e)
		}
	}
	// The real thing writes, audits and re-assesses.
	res, err = h.svc.RemediateDevice(context.Background(), dev, p, Options{DryRun: false}, "tester", "job2")
	if err != nil {
		t.Fatal(err)
	}
	if res.Written != 2 || len(fake.writes) != 2 || fake.value("protocols.ftp.enabled") != false {
		t.Errorf("real remediation: %+v writes=%v", res, fake.writes)
	}
	audit, _ = h.st.ListAudit(context.Background(), 100)
	var found bool
	for _, e := range audit {
		if e.Action == "security.remediate" && e.Actor == "tester" && e.Target == dev.ID {
			found = true
		}
	}
	if !found {
		t.Error("real write not audited")
	}
	latest, _ := h.st.LatestAssessments(context.Background(), p.ID, []string{dev.ID})
	if len(latest) != 1 || !latest[0].Compliant {
		t.Errorf("device should be compliant after remediation: %+v", latest)
	}
}

// Spec §11 / §14 safety: a remediation never removes the last management path.
func TestManagementPathRuleKeepsLastPath(t *testing.T) {
	h := newHarness(t)
	fake := newFake("fake", map[string]any{"protocols.http.enabled": true, "protocols.snmp.v1v2cEnabled": true, "protocols.ipp.enabled": true})
	h.reg.Register(fake)
	p := policyWith(
		model.PolicyItem{ID: "http-off", Key: "protocols.http.enabled", Operator: model.OpEq, Value: false, Severity: "high", Remediate: true, ManagementPath: "http"},
		model.PolicyItem{ID: "snmp-off", Key: "protocols.snmp.v1v2cEnabled", Operator: model.OpEq, Value: false, Severity: "high", Remediate: true, ManagementPath: "snmp"},
		model.PolicyItem{ID: "ipp-off", Key: "protocols.ipp.enabled", Operator: model.OpEq, Value: false, Severity: "low", Remediate: true, ManagementPath: "ipp"},
	)

	// Device A is managed only through the plugin (an HTTP path): disabling HTTP would lock it out.
	devA := h.device("10.0.0.3", map[model.Capability]string{model.CapReadSecurity: "fake", model.CapWriteSecurity: "fake"})
	res, err := h.svc.RemediateDevice(context.Background(), devA, p, Options{DryRun: false}, "tester", "j")
	if err != nil {
		t.Fatal(err)
	}
	byID := map[string]model.RemediationItem{}
	for _, it := range res.Items {
		byID[it.ItemID] = it
	}
	if it := byID["http-off"]; it.Status != model.RemediationSkipped || !strings.Contains(it.Message, "management-path") {
		t.Errorf("http-off must be skipped by the management-path rule: %+v", it)
	}
	// SNMP/IPP are not management paths of A, so they are plain writes.
	if byID["snmp-off"].Status != model.RemediationWritten || byID["ipp-off"].Status != model.RemediationWritten {
		t.Errorf("snmp/ipp should be written on A: %+v", byID)
	}
	if fake.value("protocols.http.enabled") != true {
		t.Fatal("HTTP was disabled on a device managed only over HTTP")
	}

	// Device B is managed over SNMP, IPP and the plugin: two paths may go, the last must stay.
	fake.doc["protocols.http.enabled"], fake.doc["protocols.snmp.v1v2cEnabled"], fake.doc["protocols.ipp.enabled"] = true, true, true
	fake.writes = nil
	devB := h.device("10.0.0.4", map[model.Capability]string{model.CapReadSecurity: "fake", model.CapWriteSecurity: "fake", model.CapReadStatus: "snmp", model.CapReadSupplies: "ipp"})
	res, err = h.svc.RemediateDevice(context.Background(), devB, p, Options{DryRun: false}, "tester", "j")
	if err != nil {
		t.Fatal(err)
	}
	byID = map[string]model.RemediationItem{}
	for _, it := range res.Items {
		byID[it.ItemID] = it
	}
	if byID["http-off"].Status != model.RemediationWritten || byID["snmp-off"].Status != model.RemediationWritten {
		t.Errorf("first two paths may be disabled on B: %+v", byID)
	}
	if byID["ipp-off"].Status != model.RemediationSkipped {
		t.Errorf("the last path (ipp) must survive on B: %+v", byID["ipp-off"])
	}
	if fake.value("protocols.ipp.enabled") != true {
		t.Fatal("the last management path was disabled")
	}
	// PlanRemediation is the unit under test for the rule itself.
	plan := PlanRemediation(&model.Device{Capabilities: map[model.Capability]string{model.CapReadStatus: "snmp"}}, p,
		map[string]model.ItemResult{"http-off": {Status: model.ItemFail}, "snmp-off": {Status: model.ItemFail}, "ipp-off": {Status: model.ItemFail}})
	if plan[0].Change == nil || plan[1].Change != nil || plan[2].Change == nil {
		t.Errorf("plan for an SNMP-only device: http ok, snmp blocked, ipp ok; got %+v", plan)
	}
}

func TestRemediateSkipsAssessOnlyUnknownAndNoWriter(t *testing.T) {
	h := newHarness(t)
	fake := newFake("fake", map[string]any{"protocols.raw9100.enabled": true, "protocols.ftp.enabled": true})
	fake.failKeys["protocols.ftp.enabled"] = true
	h.reg.Register(fake)
	dev := h.device("10.0.0.5", map[model.Capability]string{model.CapReadSecurity: "fake", model.CapWriteSecurity: "fake", model.CapReadStatus: "snmp"})
	p := policyWith(
		model.PolicyItem{ID: "raw", Key: "protocols.raw9100.enabled", Operator: model.OpEq, Value: false, Remediate: false},
		model.PolicyItem{ID: "wsd", Key: "protocols.wsd.enabled", Operator: model.OpEq, Value: false, Remediate: true},
		model.PolicyItem{ID: "ftp", Key: "protocols.ftp.enabled", Operator: model.OpEq, Value: false, Remediate: true},
	)
	res, err := h.svc.RemediateDevice(context.Background(), dev, p, Options{}, "t", "j")
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]model.RemediationStatus{"raw": model.RemediationSkipped, "wsd": model.RemediationSkipped, "ftp": model.RemediationFailed}
	for _, it := range res.Items {
		if it.Status != want[it.ItemID] {
			t.Errorf("%s: %s (%s), want %s", it.ItemID, it.Status, it.Message, want[it.ItemID])
		}
	}
	// No writer at all: everything that would be written is skipped with a clear message.
	devNo := h.device("10.0.0.6", map[model.Capability]string{model.CapReadSecurity: "fake"})
	res, err = h.svc.RemediateDevice(context.Background(), devNo, p, Options{}, "t", "j")
	if err != nil {
		t.Fatal(err)
	}
	for _, it := range res.Items {
		if it.ItemID == "ftp" && (it.Status != model.RemediationSkipped || !strings.Contains(it.Message, "no driver")) {
			t.Errorf("no-writer item: %+v", it)
		}
	}
}

// Spec §14 safety: rotation secrets never appear in results, audit or logs,
// and the device keeps a working credential.
func TestRotationNeverLeaksSecret(t *testing.T) {
	h := newHarness(t)
	fake := newFake("fake", map[string]any{"protocols.snmp.readCommunity": "public", "security.adminPassword": ""})
	h.reg.Register(fake)
	dev := h.device("10.0.0.7", map[model.Capability]string{model.CapReadSecurity: "fake", model.CapWriteSecurity: "fake"})
	ctx := context.Background()

	// Dry run: reports but changes nothing and stores nothing.
	j, err := h.svc.RotateSNMPCommunity(ctx, Target{DeviceIDs: []string{dev.ID}}, Options{DryRun: true}, "tester")
	if err != nil {
		t.Fatal(err)
	}
	done, results := h.wait(j)
	if done.Status != model.JobSucceeded || len(results) != 1 || results[0].Data["status"] != "would_rotate" {
		t.Fatalf("dry run: %+v %+v", done, results)
	}
	if fake.value("protocols.snmp.readCommunity") != "public" || len(fake.writes) != 0 {
		t.Fatal("dry run wrote to the device")
	}
	if cs, _ := h.st.ListCredentials(ctx); len(cs) != 0 {
		t.Fatal("dry run stored a credential")
	}

	j, err = h.svc.RotateSNMPCommunity(ctx, Target{DeviceIDs: []string{dev.ID}}, Options{DryRun: false}, "tester")
	if err != nil {
		t.Fatal(err)
	}
	done, results = h.wait(j)
	if done.Status != model.JobSucceeded || len(results) != 1 || results[0].Data["status"] != "rotated" {
		t.Fatalf("rotate: %+v %+v", done, results)
	}
	secret, _ := fake.value("protocols.snmp.readCommunity").(string)
	if len(secret) != 24 || secret == "public" {
		t.Fatalf("device did not receive a new community: %q", secret)
	}
	creds, err := h.st.ResolveCreds(ctx, dev.ID, nil)
	if err != nil || len(creds.Communities) == 0 || creds.Communities[0] != secret {
		t.Fatalf("new community must resolve first for the device: %+v %v", creds.Communities, err)
	}
	// Nothing observable carries the secret.
	jr, _ := json.Marshal(results)
	audit, _ := h.st.ListAudit(ctx, 100)
	ar, _ := json.Marshal(audit)
	cl, _ := h.st.ListCredentials(ctx)
	cr, _ := json.Marshal(cl)
	jobJSON, _ := json.Marshal(done)
	for name, blob := range map[string]string{"job results": string(jr), "audit": string(ar), "credentials": string(cr), "job": string(jobJSON), "log": h.logBuf.String()} {
		if strings.Contains(blob, secret) {
			t.Errorf("secret leaked into %s", name)
		}
	}
	var audited bool
	for _, e := range audit {
		if e.Action == "security.rotate" && e.Result == "rotated" && e.After["credentialId"] != "" {
			audited = true
		}
	}
	if !audited {
		t.Error("rotation not audited")
	}

	// Admin password rotation stores an admin credential with a username.
	j, _ = h.svc.RotateAdminPassword(ctx, Target{DeviceIDs: []string{dev.ID}}, Options{DryRun: false}, "tester")
	done, results = h.wait(j)
	if done.Status != model.JobSucceeded || results[0].Data["status"] != "rotated" {
		t.Fatalf("admin rotate: %+v %+v", done, results)
	}
	pw, _ := fake.value("security.adminPassword").(string)
	creds, _ = h.st.ResolveCreds(ctx, dev.ID, nil)
	if creds.AdminPass != pw || creds.AdminUser != "admin" || pw == "" {
		t.Errorf("admin credential not resolved: user=%q", creds.AdminUser)
	}
	if strings.Contains(h.logBuf.String(), pw) {
		t.Error("admin password leaked into the log")
	}
}

func TestRotationSkipsWhenNoWriterAndFailsWithoutStoring(t *testing.T) {
	h := newHarness(t)
	fake := newFake("fake", map[string]any{"protocols.snmp.readCommunity": "public"})
	fake.failKeys["protocols.snmp.readCommunity"] = true
	h.reg.Register(fake)
	dev := h.device("10.0.0.8", map[model.Capability]string{model.CapReadSecurity: "fake", model.CapWriteSecurity: "fake"})
	j, _ := h.svc.RotateSNMPCommunity(context.Background(), Target{DeviceIDs: []string{dev.ID}}, Options{DryRun: false}, "t")
	done, results := h.wait(j)
	if done.Status != model.JobFailed || results[0].Data["status"] != "failed" {
		t.Fatalf("failed write should fail the job: %+v %+v", done, results)
	}
	if cs, _ := h.st.ListCredentials(context.Background()); len(cs) != 0 {
		t.Error("a credential was stored although the device rejected the write")
	}
	devNo := h.device("10.0.0.9", nil)
	j, _ = h.svc.RotateSNMPCommunity(context.Background(), Target{DeviceIDs: []string{devNo.ID}}, Options{DryRun: false}, "t")
	_, results = h.wait(j)
	if results[0].Status != "skipped" {
		t.Errorf("no writer should skip: %+v", results[0])
	}
}

func TestCheckExpiryAlerts(t *testing.T) {
	h := newHarness(t)
	dev := h.device("10.0.0.10", nil)
	ctx := context.Background()
	now := time.Now().UTC()
	set := func(days int) {
		if err := h.st.SaveDeviceCertificates(ctx, dev.ID, "probe", []model.Certificate{{ID: "tls:443", Use: "identity", Subject: "CN=printer", NotAfter: now.Add(time.Duration(days) * 24 * time.Hour)}}, now); err != nil {
			t.Fatal(err)
		}
		if err := h.svc.CheckExpiry(ctx); err != nil {
			t.Fatal(err)
		}
	}
	sev := func() string {
		as, _ := h.st.ListAlerts(ctx, store.AlertFilter{DeviceID: dev.ID, ActiveOnly: true})
		for _, a := range as {
			if a.Key == "cert:expiry" {
				return string(a.Severity)
			}
		}
		return "none"
	}
	set(20)
	if sev() != "warning" {
		t.Errorf("20 days: %s", sev())
	}
	set(5)
	if sev() != "critical" {
		t.Errorf("5 days: %s", sev())
	}
	set(-1)
	if sev() != "critical" {
		t.Errorf("expired: %s", sev())
	}
	set(90)
	if sev() != "none" {
		t.Errorf("renewed: %s", sev())
	}
}

func TestGatherMergesSourcesAndTargetsResolve(t *testing.T) {
	h := newHarness(t)
	a := newFake("alpha", map[string]any{"protocols.telnet.enabled": true, "identity.hostname": "x"})
	b := newFake("beta", map[string]any{"protocols.telnet.enabled": false, "protocols.ftp.enabled": true})
	h.reg.Register(a)
	h.reg.Register(b)
	dev := h.device("10.0.0.11", nil)
	dev.IdentifyMethods = []string{"alpha", "beta"}
	d, _ := h.svc.Gather(context.Background(), dev, model.Creds{})
	if v, ok := d.Get("protocols.telnet.enabled"); !ok || v.Value != true || v.Source != "alpha" {
		t.Errorf("first source must win: %+v", v)
	}
	if v, ok := d.Get("protocols.ftp.enabled"); !ok || v.Source != "beta" {
		t.Errorf("second source fills gaps: %+v", v)
	}
	if _, ok := d.Get("identity.hostname"); ok {
		t.Error("non-security keys must be dropped from a security document")
	}
	// Targets: group (static), rule group, ids, bad.
	g := &model.Group{Name: "g", Kind: "static"}
	if err := h.st.CreateGroup(context.Background(), g); err != nil {
		t.Fatal(err)
	}
	_ = h.st.SetGroupMembers(context.Background(), g.ID, []string{dev.ID})
	if devs, err := h.svc.ResolveTarget(context.Background(), Target{GroupID: g.ID}); err != nil || len(devs) != 1 {
		t.Errorf("static group: %v %v", devs, err)
	}
	rg := &model.Group{Name: "r", Kind: "rule", Rule: &model.GroupRule{Subnet: "10.0.0.0/24"}}
	_ = h.st.CreateGroup(context.Background(), rg)
	if devs, err := h.svc.ResolveTarget(context.Background(), Target{GroupID: rg.ID}); err != nil || len(devs) != 1 {
		t.Errorf("rule group: %v %v", devs, err)
	}
	if _, err := h.svc.ResolveTarget(context.Background(), Target{DeviceIDs: []string{"nope"}}); err == nil {
		t.Error("unknown device must error")
	}
	if _, err := h.svc.ResolveTarget(context.Background(), Target{}); err == nil {
		t.Error("empty target must error")
	}
}
