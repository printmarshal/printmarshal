package fleet

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"printmarshal/internal/alerts"
	"printmarshal/internal/config"
	"printmarshal/internal/discovery"
	"printmarshal/internal/driver/snmp"
	"printmarshal/internal/engine"
	"printmarshal/internal/jobs"
	"printmarshal/internal/model"
	"printmarshal/internal/secrets"
	"printmarshal/internal/store"
)

type fakeResolver struct {
	ptrs  map[string][]string
	hosts map[string][]string
	delay time.Duration
	calls atomic.Int64
}

func (f *fakeResolver) LookupAddr(ctx context.Context, addr string) ([]string, error) {
	f.calls.Add(1)
	if f.delay > 0 {
		select {
		case <-time.After(f.delay):
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	if f.ptrs == nil {
		return nil, errors.New("nxdomain")
	}
	names, ok := f.ptrs[addr]
	if !ok {
		return nil, errors.New("nxdomain")
	}
	return names, nil
}

func (f *fakeResolver) LookupHost(ctx context.Context, host string) ([]string, error) {
	if f.delay > 0 {
		select {
		case <-time.After(f.delay):
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	if f.hosts == nil {
		return nil, errors.New("nxdomain")
	}
	addrs, ok := f.hosts[host]
	if !ok {
		return nil, errors.New("nxdomain")
	}
	return addrs, nil
}

func newTestService(t *testing.T, fixture string) (*Service, store.Store) {
	t.Helper()
	fx, err := snmp.LoadFixture("../../testdata/fixtures/" + fixture)
	if err != nil {
		t.Fatal(err)
	}
	box, _ := secrets.New(make([]byte, 32))
	st, err := store.OpenSQLite(t.TempDir()+"/t.db", box)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	reg := engine.NewRegistry()
	reg.Register(snmp.New(func(ctx context.Context, address string, creds model.Creds) (snmp.Session, string, error) {
		if address == "10.0.0.250" {
			return nil, "", context.DeadlineExceeded
		}
		return fx, "v2c", nil
	}))
	eng := engine.New(reg, log, 4)
	cfg := config.Default()
	cfg.Alerts.OfflineAfterFailures = 1
	runner := jobs.New(st, log, 2, func(e model.Event) {})
	t.Cleanup(runner.Stop)
	svc := New(st, eng, runner, cfg, log, func(e model.Event) {}, func(ctx context.Context, a string) bool { return true })
	svc.Resolver = &fakeResolver{}
	return svc, st
}

// Integration: identify + poll a fixture-backed device end to end, read-only.
func TestIdentifyAndPoll(t *testing.T) {
	svc, st := newTestService(t, "generic-laser.snmp")
	ctx := context.Background()
	dev, created, err := svc.IdentifyAndUpsert(ctx, "10.0.0.5", []string{"scan"}, discoveryCandidate("10.0.0.5"))
	if err != nil || !created {
		t.Fatalf("identify: %v created=%v", err, created)
	}
	if dev.Serial != "ACM4200X0917" || dev.Capabilities[model.CapReadSupplies] != "snmp" {
		t.Errorf("device = %+v", dev)
	}
	upd, res, err := svc.PollDevice(ctx, dev)
	if err != nil {
		t.Fatal(err)
	}
	if !res.Reachable || upd.Status == nil || upd.Status.State != "idle" || len(upd.Supplies) != 2 || upd.Counters == nil || upd.Counters.Total != 184233 {
		t.Errorf("poll = %+v / %+v", upd, res)
	}
	if res.ServedBy[model.CapReadStatus] != "snmp" {
		t.Errorf("servedBy = %v", res.ServedBy)
	}
	active, _ := st.ListAlerts(ctx, store.AlertFilter{DeviceID: dev.ID, ActiveOnly: true})
	keys := map[string]model.Severity{}
	for _, a := range active {
		keys[a.Key] = a.Severity
	}
	if keys["supply:1"] != model.SeverityWarning {
		t.Errorf("toner at 12%% should be a warning: %+v", keys)
	}
	if keys["device:markerSupplies:markerTonerAlmostEmpty:1"] != model.SeverityWarning {
		t.Errorf("device-reported alert missing: %+v", keys)
	}
	if _, ok := keys["paper:210x297mm"]; ok {
		t.Error("only one of two A4 trays is empty; no fleet paper alert expected")
	}
	samples, _ := st.QueryTelemetry(ctx, store.TelemetryQuery{DeviceID: dev.ID, Metric: "supply.percent"})
	if len(samples) != 2 {
		t.Errorf("telemetry samples = %d", len(samples))
	}
	// Second poll must not duplicate alerts.
	_, _, _ = svc.PollDevice(ctx, upd)
	active2, _ := st.ListAlerts(ctx, store.AlertFilter{DeviceID: dev.ID, ActiveOnly: true})
	if len(active2) != len(active) {
		t.Errorf("alerts duplicated: %d -> %d", len(active), len(active2))
	}
}

func TestOfflineDetection(t *testing.T) {
	svc, st := newTestService(t, "generic-laser.snmp")
	ctx := context.Background()
	dev, _, err := svc.IdentifyAndUpsert(ctx, "10.0.0.5", []string{"scan"}, discoveryCandidate("10.0.0.5"))
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := svc.PollDevice(ctx, dev); err != nil {
		t.Fatal(err)
	}
	// Simulate the device moving to an address that does not answer.
	moved, _, _ := st.UpsertDevice(ctx, &model.Identity{Address: "10.0.0.250", Serial: dev.Serial, Method: "snmp"}, nil, nil)
	upd, res, err := svc.PollDevice(ctx, moved)
	if err != nil {
		t.Fatal(err)
	}
	if res.Reachable || upd.Status == nil || upd.Status.Online {
		t.Errorf("expected offline: %+v", upd.Status)
	}
	active, _ := st.ListAlerts(ctx, store.AlertFilter{DeviceID: dev.ID, ActiveOnly: true})
	found := false
	for _, a := range active {
		if a.Key == "offline" {
			found = true
		}
	}
	if !found {
		t.Errorf("offline alert expected, got %+v", active)
	}
	if len(upd.Supplies) != 2 {
		t.Error("last known supplies must be kept while offline")
	}
}

func TestPollJob(t *testing.T) {
	svc, st := newTestService(t, "color-mfp.snmp")
	ctx := context.Background()
	if _, _, err := svc.IdentifyAndUpsert(ctx, "10.0.0.9", []string{"mdns"}, discoveryCandidate("10.0.0.9")); err != nil {
		t.Fatal(err)
	}
	j, err := svc.SubmitPoll(ctx, nil, "test")
	if err != nil {
		t.Fatal(err)
	}
	wctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	done, err := svc.Jobs.Wait(wctx, j.ID)
	if err != nil {
		t.Fatal(err)
	}
	if done.Status != model.JobSucceeded || done.Done != 1 {
		t.Errorf("job = %+v", done)
	}
	results, _ := st.ListJobResults(ctx, j.ID)
	b, _ := json.Marshal(results)
	if len(results) != 1 || !strings.Contains(string(b), "down") {
		t.Errorf("results = %s", b)
	}
	devs, _ := st.ListDevices(ctx, store.DeviceFilter{Severity: "critical"})
	if len(devs) != 1 {
		t.Errorf("jammed MFP should be critical: %d", len(devs))
	}
}

// Consumables: a poll auto-links installed supplies to stock items, manual
// links survive, and inferred replacements are recorded per the setting.
func TestPollLinksSuppliesAndDetectsReplacement(t *testing.T) {
	svc, st := newTestService(t, "generic-laser.snmp")
	ctx := context.Background()
	toner := &model.Consumable{Name: "Black toner", Type: model.ConsumableToner, Color: "black", CompatibleModels: []string{"LaserPrint 4200"}, OnHand: 3, WarningAt: 1}
	if err := st.CreateConsumable(ctx, toner); err != nil {
		t.Fatal(err)
	}
	drum := &model.Consumable{Name: "Drum unit", Type: model.ConsumableDrum, OnHand: 1}
	_ = st.CreateConsumable(ctx, drum)
	dev, _, err := svc.IdentifyAndUpsert(ctx, "10.0.0.5", []string{"scan"}, discoveryCandidate("10.0.0.5"))
	if err != nil {
		t.Fatal(err)
	}
	upd, _, err := svc.PollDevice(ctx, dev)
	if err != nil {
		t.Fatal(err)
	}
	links, _ := st.ListSupplyLinks(ctx, store.SupplyLinkFilter{DeviceID: dev.ID})
	byIdx := map[int]*model.SupplyLink{}
	for _, l := range links {
		byIdx[l.SupplyIndex] = l
	}
	if byIdx[1] == nil || byIdx[1].ConsumableID != toner.ID || byIdx[1].Source != "auto" {
		t.Errorf("toner slot should auto-link: %+v", links)
	}
	if byIdx[2] == nil || byIdx[2].ConsumableID != drum.ID {
		t.Errorf("sole drum item should auto-link: %+v", links)
	}
	ins, err := svc.Consumables.Installed(ctx, []*model.Device{upd}, false)
	if err != nil || len(ins) != 2 || ins[0].Consumable == nil || ins[0].Consumable.OnHand != 3 {
		t.Errorf("installed = %v %+v", err, ins)
	}

	// Replacement: the toner level jumps from 12% to 100% on the next reading.
	_ = st.SetSetting(ctx, "consumables", model.ConsumableSettings{AutoDecrement: model.AutoDecrementPrompt, ReplacementJumpPct: 30, ForecastMinDays: 3, ReorderLeadDays: 14})
	full := make([]model.Supply, len(upd.Supplies))
	copy(full, upd.Supplies)
	p := 100
	full[0].Percent, full[0].Level = &p, 100
	after, _ := st.ApplyPoll(ctx, dev.ID, store.PollUpdate{Supplies: full, Reachable: true, At: time.Now()})
	events := svc.Consumables.AfterPoll(ctx, upd, after)
	if len(events) != 1 || events[0].Status != model.StockEventPending || events[0].ConsumableID != toner.ID {
		t.Fatalf("prompt mode should record a pending replacement: %+v", events)
	}
	if c, _ := st.GetConsumable(ctx, toner.ID); c.OnHand != 3 {
		t.Errorf("pending must not change stock: %d", c.OnHand)
	}
	ev, c, err := svc.Consumables.ConfirmReplacement(ctx, events[0].ID, "op")
	if err != nil || c.OnHand != 2 || ev.Status != model.StockEventApplied {
		t.Errorf("confirm: %v %+v %+v", err, c, ev)
	}
	_ = st.SetSetting(ctx, "consumables", model.ConsumableSettings{AutoDecrement: model.AutoDecrementAutomatic, ReplacementJumpPct: 30, ForecastMinDays: 3, ReorderLeadDays: 14})
	events = svc.Consumables.AfterPoll(ctx, upd, after)
	if len(events) != 1 || events[0].Status != model.StockEventApplied {
		t.Fatalf("automatic mode should apply: %+v", events)
	}
	if c, _ := st.GetConsumable(ctx, toner.ID); c.OnHand != 1 {
		t.Errorf("automatic decrement: %d", c.OnHand)
	}
	// No jump, no event.
	if events = svc.Consumables.AfterPoll(ctx, after, after); len(events) != 0 {
		t.Errorf("unchanged levels must not record replacements: %+v", events)
	}
	// Stock alerts follow the inventory.
	svc.ReconcileConsumables(ctx)
	active, _ := st.ListAlerts(ctx, store.AlertFilter{ActiveOnly: true})
	found := false
	for _, a := range active {
		if a.Source == "consumable" && a.Key == "consumable:"+toner.ID && a.Severity == model.SeverityWarning {
			found = true
		}
	}
	if !found {
		t.Errorf("expected a consumable stock alert, got %+v", active)
	}
}

// TestPartialPollKeepsFaults (code review H-012/H-013): alerts of sources a
// poll did not observe survive it; trap alerts clear after the hold, and a
// recovery trap clears its counterpart at once.
func TestPartialPollKeepsFaults(t *testing.T) {
	svc, st := newTestService(t, "generic-laser.snmp")
	ctx := context.Background()
	dev, _, err := svc.IdentifyAndUpsert(ctx, "10.0.0.5", []string{"scan"}, discoveryCandidate("10.0.0.5"))
	if err != nil {
		t.Fatal(err)
	}
	jam, _, _ := st.RaiseAlert(ctx, dev.ID, "error:jammed", "poll", model.SeverityCritical, "Paper jam")
	low, _, _ := st.RaiseAlert(ctx, dev.ID, "supply:1", "supply", model.SeverityWarning, "Black low")
	active := func() map[string]bool {
		as, _ := st.ListAlerts(ctx, store.AlertFilter{DeviceID: dev.ID, ActiveOnly: true})
		m := map[string]bool{}
		for _, a := range as {
			m[a.Key] = true
		}
		return m
	}
	// Unreachable: only the offline source is judged.
	desired, observed := alerts.Evaluate(dev, &engine.PollResult{Reachable: false}, 1, svc.thresholds())
	svc.reconcileAlerts(ctx, dev, desired, observed)
	if m := active(); !m["error:jammed"] || !m["supply:1"] {
		t.Fatalf("unreachable poll cleared faults: %v", m)
	}
	// Partial: counters only.
	desired, observed = alerts.Evaluate(dev, &engine.PollResult{Reachable: true, Counters: &model.Counters{Total: 5}}, 0, svc.thresholds())
	svc.reconcileAlerts(ctx, dev, desired, observed)
	if m := active(); !m["error:jammed"] || !m["supply:1"] {
		t.Fatalf("partial poll cleared faults: %v", m)
	}
	// Status observed, supplies not: the jam clears, the supply stays.
	desired, observed = alerts.Evaluate(dev, &engine.PollResult{Reachable: true, Status: &model.Status{State: "idle"}}, 0, svc.thresholds())
	svc.reconcileAlerts(ctx, dev, desired, observed)
	if m := active(); m["error:jammed"] || !m["supply:1"] {
		t.Fatalf("status-only poll: %v", m)
	}
	_ = jam
	_ = low

	// Traps: a link-down alert survives a status poll inside the hold ...
	svc.HandleTrap(ctx, "10.0.0.5", "Link down", model.SeverityCritical, nil)
	if m := active(); !m["trap:link-down"] {
		t.Fatalf("trap alert not raised: %v", m)
	}
	svc.reconcileTrapAlerts(ctx, dev)
	if m := active(); !m["trap:link-down"] {
		t.Fatal("trap alert cleared inside the hold")
	}
	// ... a link-up trap clears it at once ...
	svc.HandleTrap(ctx, "10.0.0.5", "Link up", model.SeverityOK, nil)
	if m := active(); m["trap:link-down"] {
		t.Fatal("link up did not clear link down")
	}
	// ... and any other trap alert clears once the hold has passed and a poll read the status.
	svc.HandleTrap(ctx, "10.0.0.5", "Device cold start", model.SeverityWarning, nil)
	old := TrapAlertHold
	TrapAlertHold = 0
	defer func() { TrapAlertHold = old }()
	// The trap queued its own reconciling poll; retry until that one is done.
	deadline := time.Now().Add(5 * time.Second)
	for {
		_, _, err := svc.PollDevice(ctx, dev)
		if err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal(err)
		}
		time.Sleep(20 * time.Millisecond)
	}
	if m := active(); m["trap:device-cold-start"] {
		t.Fatalf("trap alert not reconciled after the hold: %v", m)
	}
}

func TestReverseDNSLookupAndShortNames(t *testing.T) {
	svc, st := newTestService(t, "generic-laser.snmp")
	svc.Resolver = &fakeResolver{
		ptrs:  map[string][]string{"10.0.0.5": {"printer-7.corp.example.com."}},
		hosts: map[string][]string{"printer-7.corp.example.com": {"10.0.0.5"}},
	}
	ctx := context.Background()
	dev, created, err := svc.IdentifyAndUpsert(ctx, "10.0.0.5", []string{"scan"}, discoveryCandidate("10.0.0.5"))
	if err != nil || !created {
		t.Fatalf("identify: %v created=%v", err, created)
	}
	if dev.Hostname != "printer-7" || dev.HostnameSource != "dns" {
		t.Fatalf("hostname=%q source=%q, want printer-7 / dns", dev.Hostname, dev.HostnameSource)
	}
	if dev.Name() != "printer-7" {
		t.Errorf("dev.Name() = %q, want printer-7 (DNS hostname before advertised sysName)", dev.Name())
	}
	stored, err := st.GetDevice(ctx, dev.ID)
	if err != nil {
		t.Fatal(err)
	}
	if stored.Hostname != "printer-7" || stored.HostnameSource != "dns" {
		t.Errorf("stored hostname=%q source=%q", stored.Hostname, stored.HostnameSource)
	}

	// ShortNames=false keeps the FQDN (without trailing dot).
	svc.Cfg.Discovery.ReverseDNS.ShortNames = false
	upd, err := svc.RefreshHostname(ctx, stored)
	if err != nil {
		t.Fatal(err)
	}
	if upd.Hostname != "printer-7.corp.example.com" || upd.HostnameSource != "dns" {
		t.Errorf("ShortNames=false: hostname=%q source=%q", upd.Hostname, upd.HostnameSource)
	}
}

func TestReverseDNSMDNSHostnameNotOverwritten(t *testing.T) {
	svc, _ := newTestService(t, "generic-laser.snmp")
	fake := &fakeResolver{
		ptrs:  map[string][]string{"10.0.0.5": {"dns-name.corp.example.com."}},
		hosts: map[string][]string{"dns-name.corp.example.com": {"10.0.0.5"}},
	}
	svc.Resolver = fake
	ctx := context.Background()

	cand := discovery.Candidate{Address: "10.0.0.5", Hostname: "mdns-printer.local"}
	dev, _, err := svc.IdentifyAndUpsert(ctx, "10.0.0.5", []string{"mdns"}, cand)
	if err != nil {
		t.Fatal(err)
	}
	if dev.Hostname != "mdns-printer.local" || dev.HostnameSource != "mdns" {
		t.Fatalf("mDNS hostname must win: hostname=%q source=%q", dev.Hostname, dev.HostnameSource)
	}
	if fake.calls.Load() != 0 {
		t.Errorf("expected 0 reverse DNS lookups when mDNS hostname is present, got %d", fake.calls.Load())
	}

	// Polling or on-demand refresh must not overwrite the mDNS hostname either.
	polled, _, err := svc.PollDevice(ctx, dev)
	if err != nil {
		t.Fatal(err)
	}
	if polled.Hostname != "mdns-printer.local" || polled.HostnameSource != "mdns" {
		t.Errorf("poll overwrote mDNS hostname: %q (%q)", polled.Hostname, polled.HostnameSource)
	}
	refreshed, err := svc.RefreshHostname(ctx, polled)
	if err != nil {
		t.Fatal(err)
	}
	if refreshed.Hostname != "mdns-printer.local" || refreshed.HostnameSource != "mdns" {
		t.Errorf("RefreshHostname overwrote mDNS hostname: %q (%q)", refreshed.Hostname, refreshed.HostnameSource)
	}
}

func TestReverseDNSForwardConfirmRejectsMismatch(t *testing.T) {
	svc, st := newTestService(t, "generic-laser.snmp")
	fake := &fakeResolver{
		ptrs:  map[string][]string{"10.0.0.5": {"spoofed.corp.example.com."}},
		hosts: map[string][]string{"spoofed.corp.example.com": {"10.0.0.99"}}, // mismatch!
	}
	svc.Resolver = fake
	ctx := context.Background()

	dev, _, err := svc.IdentifyAndUpsert(ctx, "10.0.0.5", []string{"scan"}, discoveryCandidate("10.0.0.5"))
	if err != nil {
		t.Fatal(err)
	}
	if dev.Hostname != "" || dev.HostnameSource != "" {
		t.Fatalf("forward-confirm mismatch should be rejected, got hostname=%q source=%q", dev.Hostname, dev.HostnameSource)
	}

	// Pre-populate an existing DNS hostname; a later mismatched PTR lookup must keep the old value.
	prev, err := st.SetDeviceHostname(ctx, dev.ID, "good-name", "dns")
	if err != nil {
		t.Fatal(err)
	}
	after, err := svc.RefreshHostname(ctx, prev)
	if err != nil {
		t.Fatal(err)
	}
	if after.Hostname != "good-name" || after.HostnameSource != "dns" {
		t.Errorf("expected old value good-name to be kept on mismatch, got %q (%q)", after.Hostname, after.HostnameSource)
	}

	// Disabling ForwardConfirm accepts the PTR name without checking LookupHost.
	svc.Cfg.Discovery.ReverseDNS.ForwardConfirm = false
	accepted, err := svc.RefreshHostname(ctx, prev)
	if err != nil {
		t.Fatal(err)
	}
	if accepted.Hostname != "spoofed" || accepted.HostnameSource != "dns" {
		t.Errorf("ForwardConfirm=false: got %q (%q), want spoofed / dns", accepted.Hostname, accepted.HostnameSource)
	}
}

func TestReverseDNSTimeoutReturnsQuickly(t *testing.T) {
	svc, _ := newTestService(t, "generic-laser.snmp")
	svc.Cfg.Discovery.ReverseDNS.Timeout = config.Duration(30 * time.Millisecond)
	svc.Resolver = &fakeResolver{
		delay: 5 * time.Second,
		ptrs:  map[string][]string{"10.0.0.5": {"slow.corp.example.com."}},
		hosts: map[string][]string{"slow.corp.example.com": {"10.0.0.5"}},
	}
	ctx := context.Background()
	start := time.Now()
	dev, _, err := svc.IdentifyAndUpsert(ctx, "10.0.0.5", []string{"scan"}, discoveryCandidate("10.0.0.5"))
	elapsed := time.Since(start)
	if err != nil {
		t.Fatal(err)
	}
	if elapsed > 500*time.Millisecond {
		t.Errorf("lookup blocked for %v, expected ~30ms timeout", elapsed)
	}
	if dev.Hostname != "" {
		t.Errorf("timed-out lookup should leave hostname empty, got %q", dev.Hostname)
	}
}

func TestReverseDNSPollRefreshIntervalAndRetry(t *testing.T) {
	svc, _ := newTestService(t, "generic-laser.snmp")
	fake := &fakeResolver{
		ptrs:  map[string][]string{},
		hosts: map[string][]string{},
	}
	svc.Resolver = fake
	ctx := context.Background()

	// Initial identify fails PTR lookup (no record yet).
	dev, _, err := svc.IdentifyAndUpsert(ctx, "10.0.0.5", []string{"scan"}, discoveryCandidate("10.0.0.5"))
	if err != nil {
		t.Fatal(err)
	}
	if dev.Hostname != "" || fake.calls.Load() != 1 {
		t.Fatalf("after failed identify: hostname=%q calls=%d", dev.Hostname, fake.calls.Load())
	}

	// DNS record appears; next poll retries because the previous lookup failed.
	fake.ptrs["10.0.0.5"] = []string{"printer-5.corp.example.com."}
	fake.hosts["printer-5.corp.example.com"] = []string{"10.0.0.5"}
	polled, _, err := svc.PollDevice(ctx, dev)
	if err != nil {
		t.Fatal(err)
	}
	if polled.Hostname != "printer-5" || polled.HostnameSource != "dns" || fake.calls.Load() != 2 {
		t.Fatalf("poll retry: hostname=%q source=%q calls=%d", polled.Hostname, polled.HostnameSource, fake.calls.Load())
	}

	// Second poll within refreshInterval (24h) does not re-query DNS.
	polled2, _, err := svc.PollDevice(ctx, polled)
	if err != nil {
		t.Fatal(err)
	}
	if fake.calls.Load() != 2 || polled2.Hostname != "printer-5" {
		t.Errorf("expected poll within refreshInterval to skip DNS, calls=%d hostname=%q", fake.calls.Load(), polled2.Hostname)
	}

	// Once refreshInterval has elapsed, the next poll refreshes the hostname.
	fake.ptrs["10.0.0.5"] = []string{"printer-5-renamed.corp.example.com."}
	fake.hosts["printer-5-renamed.corp.example.com"] = []string{"10.0.0.5"}
	svc.dnsMu.Lock()
	svc.dnsRefreshed[polled.ID] = time.Now().Add(-25 * time.Hour)
	svc.dnsMu.Unlock()
	polled3, _, err := svc.PollDevice(ctx, polled2)
	if err != nil {
		t.Fatal(err)
	}
	if polled3.Hostname != "printer-5-renamed" || fake.calls.Load() != 3 {
		t.Errorf("expected poll after refreshInterval to update hostname, got %q (calls=%d)", polled3.Hostname, fake.calls.Load())
	}
}

func TestNewResolverCustomServer(t *testing.T) {
	if r := NewResolver(""); r != net.DefaultResolver {
		t.Error("empty server should return net.DefaultResolver")
	}
	r := NewResolver("192.0.2.53")
	if r == net.DefaultResolver || !r.PreferGo || r.Dial == nil {
		t.Error("custom server should construct a PreferGo resolver with custom Dial")
	}
}
