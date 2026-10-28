package printprovider

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"strings"
	"testing"
	"time"

	"printmarshal/internal/fleet"
	"printmarshal/internal/jobs"
	"printmarshal/internal/model"
	"printmarshal/internal/secrets"
	"printmarshal/internal/store"
)

// strictBackend fails the test on any call except Availability: it proves
// the module does nothing while disabled.
type strictBackend struct {
	t *testing.T
}

func (b strictBackend) Availability() Availability { return Availability{Backend: "dry", OS: "test"} }
func (b strictBackend) ListQueues(context.Context) ([]Queue, error) {
	b.t.Fatal("ListQueues called while disabled")
	return nil, nil
}
func (b strictBackend) CreateOrUpdate(context.Context, QueueSpec) error {
	b.t.Fatal("CreateOrUpdate called while disabled")
	return nil
}
func (b strictBackend) Remove(context.Context, string) error {
	b.t.Fatal("Remove called while disabled")
	return nil
}

type harness struct {
	st     *store.SQLite
	runner *jobs.Runner
	svc    *Service
	dry    *DryBackend
}

func newHarness(t *testing.T, b Backend) *harness {
	t.Helper()
	box, _ := secrets.New(make([]byte, 32))
	st, err := store.OpenSQLite(t.TempDir()+"/t.db", box)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	runner := jobs.New(st, log, 1, nil)
	t.Cleanup(runner.Stop)
	h := &harness{st: st, runner: runner}
	if d, ok := b.(*DryBackend); ok {
		h.dry = d
	}
	h.svc = NewWithBackend(fleet.Deps{Store: st, Jobs: runner, Log: log}, b)
	return h
}

func (h *harness) device(t *testing.T, addr, name, mdl string, caps map[model.Capability]string, disc []string) *model.Device {
	t.Helper()
	d, _, err := h.st.UpsertDevice(context.Background(), &model.Identity{Address: addr, Model: mdl, Make: "Acme"}, caps, disc)
	if err != nil {
		t.Fatal(err)
	}
	if name != "" {
		if d, err = h.st.PatchDevice(context.Background(), d.ID, store.DevicePatch{DisplayName: &name}); err != nil {
			t.Fatal(err)
		}
	}
	return d
}

func (h *harness) enable(t *testing.T, mut func(*Settings)) {
	t.Helper()
	s := DefaultSettings()
	s.Enabled = true
	if mut != nil {
		mut(&s)
	}
	if _, err := h.svc.SaveSettings(context.Background(), s); err != nil {
		t.Fatal(err)
	}
}

func (h *harness) runJob(t *testing.T, dry bool) (*model.Job, []*model.JobResult) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	j, err := h.svc.Sync(ctx, "tester", dry)
	if err != nil {
		t.Fatal(err)
	}
	j, err = h.runner.Wait(ctx, j.ID)
	if err != nil {
		t.Fatal(err)
	}
	if j.Status != model.JobSucceeded {
		t.Fatalf("job %s: %s", j.Status, j.Error)
	}
	res, _ := h.st.ListJobResults(ctx, j.ID)
	return j, res
}

func queueAudits(t *testing.T, st store.Store) []*model.AuditEntry {
	t.Helper()
	all, _ := st.ListAudit(context.Background(), 200)
	var out []*model.AuditEntry
	for _, a := range all {
		if strings.HasPrefix(a.Action, "printprovider.queue.") {
			out = append(out, a)
		}
	}
	return out
}

var ippCaps = map[model.Capability]string{model.CapIdentify: "ipp", model.CapReadStatus: "ipp"}
var snmpCaps = map[model.Capability]string{model.CapIdentify: "snmp", model.CapReadStatus: "snmp"}

func TestSanitizeAndNaming(t *testing.T) {
	cases := map[string]string{
		"Front Desk HP":         "front-desk-hp",
		"  --Café / #3 (Lobby)": "caf-3-lobby",
		"___":                   "___",
		"!!!":                   "",
		"ALLCAPS_name-1":        "allcaps_name-1",
	}
	for in, want := range cases {
		if got := Sanitize(in); got != want {
			t.Errorf("Sanitize(%q)=%q want %q", in, got, want)
		}
	}
	if got := Sanitize(strings.Repeat("a", 200)); len(got) != maxNameLen {
		t.Errorf("long name not capped: %d", len(got))
	}
	d := &model.Device{ID: "0123456789abcdef", Address: "10.0.0.9"}
	if got := QueueName("pm-", d); got != "pm-printer-01234567" {
		t.Errorf("fallback name %q", got)
	}
	d.Model = "LaserJet 400"
	if got := QueueName("pm-", d); got != "pm-laserjet-400" {
		t.Errorf("model name %q", got)
	}
	d.SysName = "NPI12AB"
	if got := QueueName("pm-", d); got != "pm-npi12ab" {
		t.Errorf("sysName %q", got)
	}
	d.DisplayName = "Lobby printer"
	if got := QueueName("pm-", d); got != "pm-lobby-printer" {
		t.Errorf("display name %q", got)
	}
	if got := DeviceURI(d); got != "ipp://10.0.0.9:631/ipp/print" {
		t.Errorf("uri %q", got)
	}
	d.Address = "fe80::1"
	if got := DeviceURI(d); got != "ipp://[fe80::1]:631/ipp/print" {
		t.Errorf("ipv6 uri %q", got)
	}
}

func TestSettingsValidate(t *testing.T) {
	s := Settings{}
	if err := s.Validate(); err == nil {
		t.Fatal("empty prefix accepted")
	}
	s = Settings{QueuePrefix: "pm-", Mode: "", DefaultOptions: "  media=iso_a4_210x297mm   sides=two-sided-long-edge "}
	if err := s.Validate(); err != nil {
		t.Fatal(err)
	}
	if s.Mode != ModeCUPS || s.DefaultOptions != "media=iso_a4_210x297mm sides=two-sided-long-edge" {
		t.Errorf("normalised %+v", s)
	}
	for _, bad := range []Settings{{QueuePrefix: "pm-", Mode: "spool"}, {QueuePrefix: "has space"}, {QueuePrefix: "pm-", DefaultOptions: "novalue"}, {QueuePrefix: "pm-", DefaultOptions: "bad key=1"}} {
		if err := bad.Validate(); err == nil {
			t.Errorf("accepted %+v", bad)
		}
	}
}

func TestPlanNamesAndDiff(t *testing.T) {
	h := newHarness(t, NewDryBackend("test"))
	a := h.device(t, "10.20.0.1", "Front Desk HP", "LaserJet", ippCaps, []string{"mdns"})
	b := h.device(t, "10.20.0.2", "Front Desk HP", "LaserJet", ippCaps, []string{"scan"})
	c := h.device(t, "10.20.0.3", "SNMP only", "Dot", snmpCaps, []string{"scan"})
	// Existing: one managed queue pointing at a stale address, one orphan, one unrelated.
	h.dry.Seed(Queue{Name: "pm-front-desk-hp", URI: "ipp://10.20.0.99:631/ipp/print"}, Queue{Name: "pm-old", URI: "ipp://10.20.0.50:631/ipp/print"}, Queue{Name: "office", URI: "socket://10.20.0.60"})
	h.enable(t, nil)

	p, err := h.svc.Plan(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !p.InventoryRead {
		t.Error("inventory not read while enabled")
	}
	byDev := map[string]Entry{}
	byQueue := map[string]Entry{}
	for _, e := range p.Entries {
		if e.DeviceID != "" {
			byDev[e.DeviceID] = e
		}
		byQueue[e.QueueName] = e
	}
	// Same display name: one keeps the base name (and matches the stale
	// queue, so it is an update), the other gets a -2 suffix (create).
	ea, eb := byDev[a.ID], byDev[b.ID]
	names := map[string]Entry{ea.QueueName: ea, eb.QueueName: eb}
	if e, ok := names["pm-front-desk-hp"]; !ok || e.Action != ActionUpdate || e.Current == nil {
		t.Errorf("base name: %+v", e)
	}
	if e, ok := names["pm-front-desk-hp-2"]; !ok || e.Action != ActionCreate || e.Current != nil {
		t.Errorf("suffixed name: %+v", e)
	}
	if ea.Reason != "" || eb.Reason == "" {
		t.Errorf("DNS-SD hint: a=%q b=%q", ea.Reason, eb.Reason)
	}
	if e := byDev[c.ID]; e.Action != ActionSkip || e.QueueName != "" {
		t.Errorf("c: %+v", e)
	}
	if e := byQueue["pm-old"]; e.Action != ActionRemove {
		t.Errorf("orphan: %+v", e)
	}
	if _, touched := byQueue["office"]; touched {
		t.Error("unmanaged queue appears in the plan")
	}
	if p.Summary[string(ActionCreate)] != 1 || p.Summary[string(ActionUpdate)] != 1 || p.Summary[string(ActionRemove)] != 1 || p.Summary[string(ActionSkip)] != 1 {
		t.Errorf("summary %v", p.Summary)
	}
	created := names["pm-front-desk-hp-2"]
	cmd := strings.Join(created.Commands, "\n")
	if !strings.Contains(cmd, "lpadmin -p pm-front-desk-hp-2 -E -v "+created.URI+" -m everywhere") || !strings.Contains(cmd, "cupsenable pm-front-desk-hp-2") || !strings.Contains(cmd, "cupsaccept pm-front-desk-hp-2") {
		t.Errorf("commands:\n%s", cmd)
	}
	if !strings.Contains(cmd, "-D 'Front Desk HP (LaserJet)'") {
		t.Errorf("description not quoted:\n%s", cmd)
	}
}

func TestSyncDryRunThenReal(t *testing.T) {
	h := newHarness(t, NewDryBackend("test"))
	a := h.device(t, "10.20.1.1", "Lobby", "MFP", ippCaps, []string{"mdns"})
	h.dry.Seed(Queue{Name: "pm-gone", URI: "ipp://10.20.1.77:631/ipp/print"})
	h.enable(t, func(s *Settings) { s.DefaultOptions = "media=iso_a4_210x297mm" })

	_, res := h.runJob(t, true)
	if len(res) != 2 {
		t.Fatalf("results %d", len(res))
	}
	for _, r := range res {
		if r.Status != "ok" || !strings.HasPrefix(r.Message, "would ") || r.Data["dryRun"] != true {
			t.Errorf("dry result %+v", r)
		}
	}
	for _, c := range h.dry.Commands() {
		if strings.HasPrefix(c, "lpadmin") || strings.HasPrefix(c, "cups") {
			t.Errorf("dry run executed %q", c)
		}
	}
	if n := len(queueAudits(t, h.st)); n != 0 {
		t.Errorf("dry run wrote %d queue audit entries", n)
	}

	_, res = h.runJob(t, false)
	if len(res) != 2 {
		t.Fatalf("results %d", len(res))
	}
	cmds := strings.Join(h.dry.Commands(), "\n")
	if !strings.Contains(cmds, "lpadmin -p pm-lobby -E -v ipp://10.20.1.1:631/ipp/print -m everywhere -D 'Lobby (MFP)' -o media=iso_a4_210x297mm") || !strings.Contains(cmds, "lpadmin -x pm-gone") {
		t.Errorf("commands:\n%s", cmds)
	}
	audits := queueAudits(t, h.st)
	if len(audits) != 2 {
		t.Fatalf("audits %d", len(audits))
	}
	seen := map[string]string{}
	for _, e := range audits {
		seen[e.Action] = e.Target
		if e.Actor != "tester" || e.Result != "ok" {
			t.Errorf("audit %+v", e)
		}
	}
	if seen["printprovider.queue.create"] != a.ID || seen["printprovider.queue.remove"] != "queue:pm-gone" {
		t.Errorf("audit targets %v", seen)
	}

	p, _ := h.svc.Plan(context.Background())
	if p.Summary[string(ActionUnchanged)] != 1 || len(p.Entries) != 1 {
		t.Errorf("after sync: %+v", p.Entries)
	}
	inv, _ := h.svc.Inventory(context.Background())
	if len(inv) != 1 || inv[0].Name != "pm-lobby" || !inv[0].Managed {
		t.Errorf("inventory %+v", inv)
	}
}

func TestRemoveDeviceQueue(t *testing.T) {
	h := newHarness(t, NewDryBackend("test"))
	a := h.device(t, "10.20.2.1", "Annex", "X", ippCaps, []string{"mdns"})
	b := h.device(t, "10.20.2.2", "Basement", "X", ippCaps, []string{"mdns"})
	h.dry.Seed(Queue{Name: "pm-annex", URI: "ipp://10.20.2.1:631/ipp/print", Description: "Annex (X)"})
	h.enable(t, nil)
	name, err := h.svc.Remove(context.Background(), "op", a.ID)
	if err != nil || name != "pm-annex" {
		t.Fatalf("remove: %q %v", name, err)
	}
	if _, err := h.svc.Remove(context.Background(), "op", b.ID); !errors.Is(err, ErrNoQueue) {
		t.Errorf("no-queue remove: %v", err)
	}
	if _, err := h.svc.Remove(context.Background(), "op", "nope"); !errors.Is(err, store.ErrNotFound) {
		t.Errorf("unknown device: %v", err)
	}
	au := queueAudits(t, h.st)
	if len(au) != 1 || au[0].Action != "printprovider.queue.remove" || au[0].Target != a.ID || au[0].Actor != "op" {
		t.Errorf("audit %+v", au)
	}
	if inv, _ := h.svc.Inventory(context.Background()); len(inv) != 0 {
		t.Errorf("queue still present: %+v", inv)
	}
}

func TestDisabledDoesNothing(t *testing.T) {
	h := newHarness(t, strictBackend{t: t})
	a := h.device(t, "10.20.3.1", "Idle", "X", ippCaps, []string{"mdns"})
	ctx := context.Background()
	st, err := h.svc.Settings(ctx)
	if err != nil || st.Enabled {
		t.Fatalf("default settings %+v %v", st, err)
	}
	p, err := h.svc.Plan(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if p.Enabled || p.InventoryRead || len(p.Entries) != 1 || p.Entries[0].DeviceID != a.ID || p.Entries[0].Action != ActionCreate {
		t.Errorf("disabled plan %+v", p)
	}
	if inv, err := h.svc.Inventory(ctx); err != nil || len(inv) != 0 {
		t.Errorf("inventory %+v %v", inv, err)
	}
	if _, err := h.svc.Sync(ctx, "x", false); !errors.Is(err, ErrDisabled) {
		t.Errorf("sync: %v", err)
	}
	if _, err := h.svc.Remove(ctx, "x", a.ID); !errors.Is(err, ErrDisabled) {
		t.Errorf("remove: %v", err)
	}
	// A job that was queued before the module was turned off must also stop.
	j, err := h.runner.Submit(ctx, &model.Job{Type: JobType, CreatedBy: "x", Target: map[string]any{"all": true}, Params: map[string]any{"dryRun": false}})
	if err != nil {
		t.Fatal(err)
	}
	wctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	j, _ = h.runner.Wait(wctx, j.ID)
	if j.Status != model.JobFailed || !strings.Contains(j.Error, "disabled") {
		t.Errorf("job %s %q", j.Status, j.Error)
	}
}

func TestListModeNeverTouchesHost(t *testing.T) {
	h := newHarness(t, strictBackend{t: t})
	h.device(t, "10.20.4.1", "Listed", "X", ippCaps, []string{"mdns"})
	h.enable(t, func(s *Settings) { s.Mode = ModeList })
	ctx := context.Background()
	p, err := h.svc.Plan(ctx)
	if err != nil || len(p.Entries) != 1 || p.Entries[0].Action != ActionList || p.Entries[0].URI != "ipp://10.20.4.1:631/ipp/print" {
		t.Errorf("list plan %+v %v", p, err)
	}
	if _, err := h.svc.Sync(ctx, "x", true); !errors.Is(err, ErrListMode) {
		t.Errorf("sync: %v", err)
	}
	if inv, _ := h.svc.Inventory(ctx); len(inv) != 0 {
		t.Errorf("inventory %+v", inv)
	}
}

func TestGroupScope(t *testing.T) {
	h := newHarness(t, NewDryBackend("test"))
	a := h.device(t, "10.20.5.1", "In group", "X", ippCaps, []string{"mdns"})
	h.device(t, "10.20.5.2", "Outside", "X", ippCaps, []string{"mdns"})
	g := &model.Group{Name: "Floor 1", Kind: "static", MemberIDs: []string{a.ID}}
	if err := h.st.CreateGroup(context.Background(), g); err != nil {
		t.Fatal(err)
	}
	h.enable(t, func(s *Settings) { s.AutoCreateForGroupID = g.ID })
	p, err := h.svc.Plan(context.Background())
	if err != nil || len(p.Entries) != 1 || p.Entries[0].DeviceID != a.ID {
		t.Errorf("group plan %+v %v", p.Entries, err)
	}
}

func TestParseLpstat(t *testing.T) {
	qs := parseLpstatV("device for pm-lobby: ipp://10.0.0.5:631/ipp/print\ndevice for office: socket://10.0.0.6\njunk line\n")
	if len(qs) != 2 || qs[0].Name != "pm-lobby" || qs[0].URI != "ipp://10.0.0.5:631/ipp/print" || qs[1].Name != "office" {
		t.Errorf("%+v", qs)
	}
	meta := parseLpstatLP("printer pm-lobby is idle.  enabled since Tue\n\tDescription: Lobby (MFP)\n\tLocation: Ground floor\n\tForm mounted:\nprinter office disabled since x\n\tLocation: Annex\n")
	if m := meta["pm-lobby"]; m[0] != "Lobby (MFP)" || m[1] != "Ground floor" {
		t.Errorf("%+v", meta)
	}
	if m := meta["office"]; m[0] != "" || m[1] != "Annex" {
		t.Errorf("%+v", meta)
	}
	// A cupsBackend with a fake runner parses the same way and never execs.
	var ran [][]string
	b := &cupsBackend{tools: map[string]string{"lpadmin": "lpadmin", "lpstat": "lpstat", "cupsenable": "cupsenable", "cupsaccept": "cupsaccept"}, run: func(_ context.Context, name string, args ...string) (string, error) {
		ran = append(ran, append([]string{name}, args...))
		if name == "lpstat" && len(args) == 1 {
			return "lpstat: No destinations added.", errors.New("exit status 1")
		}
		return "", nil
	}}
	if qs, err := b.ListQueues(context.Background()); err != nil || len(qs) != 0 {
		t.Errorf("empty host: %+v %v", qs, err)
	}
	if err := b.CreateOrUpdate(context.Background(), QueueSpec{Name: "pm-x", URI: "ipp://h:631/ipp/print", Driverless: false}); err != nil {
		t.Fatal(err)
	}
	if err := b.Remove(context.Background(), "pm-x"); err != nil {
		t.Fatal(err)
	}
	joined := make([]string, len(ran))
	for i, r := range ran {
		joined[i] = strings.Join(r, " ")
	}
	all := strings.Join(joined, "\n")
	for _, want := range []string{"lpadmin -p pm-x -E -v ipp://h:631/ipp/print -m raw", "cupsenable pm-x", "cupsaccept pm-x", "lpadmin -x pm-x"} {
		if !strings.Contains(all, want) {
			t.Errorf("missing %q in\n%s", want, all)
		}
	}
}
