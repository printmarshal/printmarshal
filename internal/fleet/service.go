// Package fleet wires the engine, store, jobs and alerts into the operations
// the API and scheduler call: discovery runs, polls, alert reconciliation and
// trap handling. Everything here is read-only against devices.
package fleet

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"runtime/debug"
	"sort"
	"strings"
	"sync"
	"time"

	"printmarshal/internal/alerts"
	"printmarshal/internal/config"
	"printmarshal/internal/consumables"
	"printmarshal/internal/discovery"
	"printmarshal/internal/engine"
	"printmarshal/internal/jobs"
	"printmarshal/internal/model"
	"printmarshal/internal/store"
)

// Service is the fleet façade.
type Service struct {
	Store  store.Store
	Engine *engine.Engine
	Jobs   *jobs.Runner
	Cfg    *config.Config
	Log    *slog.Logger
	Events func(model.Event)
	// Consumables links installed supplies to stock items and forecasts them.
	Consumables *consumables.Service
	// Probe is the cheap liveness check used by active scans.
	Probe discovery.Prober
	// Resolver performs reverse and forward DNS lookups for device hostnames.
	Resolver Resolver
	// WebClient performs the device web-page probe (weburl.go); nil means a
	// default client with short timeouts and no certificate verification.
	WebClient *http.Client

	pollMu  sync.Mutex
	polling map[string]bool

	dnsMu        sync.Mutex
	dnsRefreshed map[string]time.Time

	// The resolver built for the effective DNS server (rdns.go).
	rdnsMu       sync.Mutex
	rdnsServer   string
	rdnsResolver Resolver
}

// New builds a service and registers its job types.
func New(st store.Store, eng *engine.Engine, runner *jobs.Runner, cfg *config.Config, log *slog.Logger, events func(model.Event), probe discovery.Prober) *Service {
	// Resolver stays nil so lookups follow the DNS server set at runtime
	// (rdns.go builds one per effective server); tests inject a fake.
	s := &Service{Store: st, Engine: eng, Jobs: runner, Cfg: cfg, Log: log, Events: events, Probe: probe,
		polling: map[string]bool{}, dnsRefreshed: map[string]time.Time{},
		Consumables: consumables.New(st, log)}
	runner.Register("discovery", s.runDiscovery)
	runner.Register("poll", s.runPoll)
	return s
}

func (s *Service) emit(typ string, data any) {
	if s.Events != nil {
		s.Events(model.Event{Type: typ, At: time.Now().UTC(), Data: data})
	}
}

func (s *Service) thresholds() alerts.Thresholds {
	r := s.Runtime(context.Background())
	return alerts.Thresholds{
		SupplyWarningPercent:  r.SupplyWarningPercent,
		SupplyCriticalPercent: r.SupplyCriticalPercent,
		OfflineAfterFailures:  r.OfflineAfterFailures,
	}
}

// Creds resolves the read credentials for a device (device scope > group
// scope > global). Everything in this package is read-only, so this is the
// only purpose it needs; write-side services call CredsFor with PurposeWrite.
func (s *Service) Creds(ctx context.Context, dev *model.Device) model.Creds {
	creds, _ := s.CredsFor(ctx, dev, model.PurposeRead)
	return creds
}

// CredsFor resolves a device's credentials for a purpose (audit A19). Read
// resolution logs and swallows store errors (an empty Creds falls back to the
// driver defaults); write resolution returns store.ErrNoWriteCredential when
// nothing write-capable is bound, and callers must fail the device rather
// than fall back to a read credential.
func (s *Service) CredsFor(ctx context.Context, dev *model.Device, purpose model.CredentialPurpose) (model.Creds, error) {
	creds, err := store.CredsFor(ctx, s.Store, dev, purpose)
	if err != nil && !errors.Is(err, store.ErrNoWriteCredential) {
		id := ""
		if dev != nil {
			id = dev.ID
		}
		s.Log.Warn("resolve credentials", "device", id, "purpose", purpose, "err", err)
	}
	return creds, err
}

// IdentifyAndUpsert identifies an address and records it (spec §8.1).
func (s *Service) IdentifyAndUpsert(ctx context.Context, address string, methods []string, hints discovery.Candidate) (*model.Device, bool, error) {
	return s.identifyWith(ctx, s.Creds(ctx, &model.Device{Address: address}), address, methods, hints)
}

// identifyLaterTimeout bounds the identification of an address-only record
// within a poll, so a poll's own read and store writes keep their share of
// the caller's deadline.
const identifyLaterTimeout = 45 * time.Second

// needsIdentification reports whether a record has never been identified: no
// make, model or serial and no capability profile. The CSV import leaves such
// a record for a printer that did not answer.
func needsIdentification(d *model.Device) bool {
	return d.Make == "" && d.Model == "" && d.Serial == "" && len(d.Capabilities) == 0
}

// identifyLater tries the read-only identification of an address-only record
// and returns the record to poll: identified when the printer answered, as it
// was otherwise. It merges through UpsertDevice, which matches the record by
// its address and leaves operator fields alone.
func (s *Service) identifyLater(ctx context.Context, dev *model.Device) *model.Device {
	ictx, cancel := context.WithTimeout(ctx, identifyLaterTimeout)
	defer cancel()
	upd, _, err := s.identifyWith(ictx, s.Creds(ctx, dev), dev.Address, nil, discovery.Candidate{Address: dev.Address, Hostname: dev.Hostname})
	if err != nil {
		s.Log.Debug("address-only record not identified yet", "device", dev.ID, "err", err)
		return dev
	}
	if upd.ID != dev.ID {
		// The printer is already recorded (by serial) and has moved to this
		// address: that record now holds the address; this one is left for
		// an administrator to remove.
		s.Log.Warn("address-only record is a printer already recorded", "device", dev.ID, "recorded", upd.ID)
		return dev
	}
	return upd
}

func (s *Service) identifyWith(ctx context.Context, creds model.Creds, address string, methods []string, hints discovery.Candidate) (*model.Device, bool, error) {
	ident, caps, err := s.Engine.Identify(ctx, address, creds)
	if err != nil {
		return nil, false, err
	}
	applyHints(ident, hints)
	dev, created, err := s.Store.UpsertDevice(ctx, ident, caps, methods)
	if err != nil {
		return nil, false, err
	}
	if dev.Decommissioned() {
		return dev, created, nil // the sweep found it; nothing more goes to it
	}
	dev = s.refreshHostname(ctx, dev)
	s.ensureWebURL(ctx, dev, true)
	if created {
		s.emit("device.discovered", dev)
	}
	return dev, created, nil
}

// applyHints fills identity gaps from DNS-SD TXT records (model, make, UUID, admin URL).
func applyHints(id *model.Identity, c discovery.Candidate) {
	if c.Hostname != "" && id.Hostname == "" {
		id.Hostname = c.Hostname
	}
	if c.TXT == nil {
		return
	}
	set := func(dst *string, keys ...string) {
		if *dst != "" {
			return
		}
		for _, k := range keys {
			if v := strings.TrimSpace(c.TXT[k]); v != "" {
				*dst = v
				return
			}
		}
	}
	set(&id.Model, "ty", "product", "usb_mdl")
	set(&id.Make, "usb_mfg", "mfg")
	set(&id.UUID, "uuid")
	set(&id.AdminURL, "adminurl")
	set(&id.SysLocation, "note")
	for _, svc := range c.Services {
		if strings.Contains(svc, "scan") {
			id.DeviceType = "mfp"
		}
	}
}

// PollDevice reads a device, stores the result, records telemetry and
// reconciles alerts. It never writes to the device, and it never touches a
// decommissioned one.
func (s *Service) PollDevice(ctx context.Context, dev *model.Device) (*model.Device, *engine.PollResult, error) {
	if dev.Decommissioned() {
		return dev, nil, model.ErrDecommissioned
	}
	s.pollMu.Lock()
	if s.polling[dev.ID] {
		s.pollMu.Unlock()
		return dev, nil, fmt.Errorf("poll already in progress")
	}
	s.polling[dev.ID] = true
	s.pollMu.Unlock()
	defer func() {
		s.pollMu.Lock()
		delete(s.polling, dev.ID)
		s.pollMu.Unlock()
	}()

	if needsIdentification(dev) {
		dev = s.identifyLater(ctx, dev)
	}
	creds := s.Creds(ctx, dev)
	res := s.Engine.Poll(ctx, dev, creds)
	updated, err := s.Store.ApplyPoll(ctx, dev.ID, store.PollUpdate{
		Status: res.Status, Supplies: res.Supplies, Trays: res.Trays, Counters: res.Counters,
		Firmware: res.Firmware, Reachable: res.Reachable, At: res.At,
	})
	if err != nil {
		return dev, res, err
	}
	if res.Reachable {
		if err := s.Store.AddSamples(ctx, samples(updated, res)); err != nil {
			s.Log.Warn("telemetry write", "device", dev.ID, "err", err)
		}
		updated = s.maybeRefreshHostname(ctx, updated)
	} else {
		_ = s.Store.AddSamples(ctx, []model.TelemetrySample{{DeviceID: dev.ID, Metric: "status.online", Value: 0, At: res.At}})
	}
	failures, _ := s.Store.DeviceFailures(ctx, dev.ID)
	desired, observed := alerts.Evaluate(updated, res, failures, s.thresholds())
	s.reconcileAlerts(ctx, updated, desired, observed)
	if res.Reachable && res.Status != nil {
		s.reconcileTrapAlerts(ctx, updated)
	}
	if res.Reachable && res.Supplies != nil {
		s.afterSupplies(ctx, dev, updated)
	}
	if res.Reachable {
		s.ensureWebURL(ctx, updated, false)
	}
	// The alerts above are newer than the poll stored at the top: read the
	// device back so the result and the event carry their severity (#47).
	if fresh, err := s.Store.GetDevice(ctx, updated.ID); err == nil {
		updated = fresh
	}
	s.emit("device.updated", updated)
	return updated, res, nil
}

func samples(d *model.Device, res *engine.PollResult) []model.TelemetrySample {
	at := res.At
	var out []model.TelemetrySample
	add := func(metric, label string, v float64) {
		out = append(out, model.TelemetrySample{DeviceID: d.ID, Metric: metric, Label: label, Value: v, At: at})
	}
	add("status.online", "", 1)
	if res.Status != nil {
		sev := 0.0
		switch res.Status.Severity {
		case model.SeverityWarning:
			sev = 1
		case model.SeverityCritical:
			sev = 2
		}
		add("status.severity", "", sev)
	}
	for _, sp := range res.Supplies {
		if sp.Percent != nil {
			add("supply.percent", fmt.Sprintf("%d:%s", sp.Index, sp.Description), float64(*sp.Percent))
		}
	}
	for _, t := range res.Trays {
		if t.Percent != nil {
			add("tray.percent", fmt.Sprintf("%s:%d:%s", t.Kind, t.Index, t.Name), float64(*t.Percent))
		}
	}
	if res.Counters != nil {
		add("counter.total", "", float64(res.Counters.Total))
		if res.Counters.Mono != nil {
			add("counter.mono", "", float64(*res.Counters.Mono))
		}
		if res.Counters.Color != nil {
			add("counter.color", "", float64(*res.Counters.Color))
		}
	}
	return out
}

// reconcileAlerts raises desired alerts and clears alerts of the given sources
// that are no longer desired, emitting events and notifications for changes.
func (s *Service) reconcileAlerts(ctx context.Context, dev *model.Device, desired []alerts.Desired, sources []string) {
	keep := map[string][]string{}
	for _, d := range desired {
		a, raised, err := s.Store.RaiseAlert(ctx, dev.ID, d.Key, d.Source, d.Severity, d.Description)
		if err != nil {
			s.Log.Warn("raise alert", "device", dev.ID, "key", d.Key, "err", err)
			continue
		}
		keep[d.Source] = append(keep[d.Source], d.Key)
		if raised {
			s.emit("alert.raised", a)
		}
	}
	for _, src := range sources {
		cleared, err := s.Store.ClearAlertsExcept(ctx, dev.ID, src, keep[src])
		if err != nil {
			s.Log.Warn("clear alerts", "device", dev.ID, "err", err)
			continue
		}
		for _, a := range cleared {
			s.emit("alert.cleared", a)
		}
	}
}

// afterSupplies runs the consumables bookkeeping once a poll has stored new
// supply readings: auto links are refreshed and inferred replacements recorded.
func (s *Service) afterSupplies(ctx context.Context, before, after *model.Device) {
	events := s.Consumables.AfterPoll(ctx, before, after)
	for _, ev := range events {
		s.emit("consumable.replacement", ev)
		if ev.Status == model.StockEventApplied {
			s.emit("consumable.updated", map[string]any{"id": ev.ConsumableID})
		}
	}
	if len(events) > 0 {
		s.ReconcileConsumables(ctx)
	}
}

// ReconcileConsumables re-evaluates stock alerts (global, no device).
func (s *Service) ReconcileConsumables(ctx context.Context) {
	items, err := s.Store.ListConsumables(ctx, store.ConsumableFilter{})
	if err != nil {
		return
	}
	s.reconcileAlerts(ctx, &model.Device{ID: "", DisplayName: "Consumables"}, alerts.ConsumableDesired(items), []string{alerts.ConsumableSource})
}

// --- jobs -------------------------------------------------------------------

// DiscoveryParams are the discovery job parameters.
type DiscoveryParams struct {
	Targets    []string `json:"targets"`
	ZeroConf   bool     `json:"zeroconf"`
	AllowLarge bool     `json:"allowLarge"`
}

func paramsFromJob(j *model.Job) DiscoveryParams {
	p := DiscoveryParams{ZeroConf: true}
	if v, ok := j.Params["zeroconf"].(bool); ok {
		p.ZeroConf = v
	}
	if v, ok := j.Params["allowLarge"].(bool); ok {
		p.AllowLarge = v
	}
	if ts, ok := j.Params["targets"].([]any); ok {
		for _, t := range ts {
			if s, ok := t.(string); ok && strings.TrimSpace(s) != "" {
				p.Targets = append(p.Targets, strings.TrimSpace(s))
			}
		}
	}
	return p
}

// SubmitDiscovery queues a discovery run.
func (s *Service) SubmitDiscovery(ctx context.Context, p DiscoveryParams, by string) (*model.Job, error) {
	targets := make([]any, len(p.Targets))
	for i, t := range p.Targets {
		targets[i] = t
	}
	j := &model.Job{Type: "discovery", CreatedBy: by,
		Target: map[string]any{"targets": targets, "zeroconf": p.ZeroConf},
		Params: map[string]any{"targets": targets, "zeroconf": p.ZeroConf, "allowLarge": p.AllowLarge}}
	return s.Jobs.Submit(ctx, j)
}

func (s *Service) runDiscovery(ctx context.Context, jc *jobs.Context) error {
	p := paramsFromJob(jc.Job)
	var lists [][]discovery.Candidate
	if p.ZeroConf {
		if c, err := discovery.BrowseMDNS(ctx, s.Cfg.Discovery.MDNSWait.D()); err != nil {
			s.Log.Warn("mdns browse", "err", err)
			jc.Result("", "", "skipped", "mDNS browse failed: "+err.Error(), nil)
		} else {
			lists = append(lists, c)
		}
		if c, err := discovery.ProbeWSD(ctx, s.Cfg.Discovery.WSDWait.D()); err != nil {
			s.Log.Warn("ws-discovery probe", "err", err)
			jc.Result("", "", "skipped", "WS-Discovery probe failed: "+err.Error(), nil)
		} else {
			lists = append(lists, c)
		}
	}
	if len(p.Targets) > 0 {
		max := s.Cfg.Discovery.MaxAddresses
		if p.AllowLarge {
			max = 0
		}
		addrs, skipped, err := discovery.ExpandTargets(p.Targets, max)
		if err != nil {
			return err
		}
		if skipped > 0 {
			msg := fmt.Sprintf("scan capped at %d addresses; %d skipped (set allowLarge to scan them)", max, skipped)
			s.Log.Warn("discovery scan capped", "max", max, "skipped", skipped)
			jc.Result("", "", "skipped", msg, map[string]any{"skipped": skipped})
		}
		lists = append(lists, discovery.ActiveScan(ctx, addrs, s.Cfg.Discovery.ScanWorkers, s.Probe))
	}
	cands := discovery.MergeCandidates(lists...)
	jc.SetTotal(len(cands) + jc.Job.Done)

	var wg sync.WaitGroup
	sem := make(chan struct{}, maxInt(1, s.Cfg.Poll.Concurrency))
	for _, c := range cands {
		if ctx.Err() != nil {
			break
		}
		wg.Add(1)
		sem <- struct{}{}
		go func(c discovery.Candidate) {
			defer wg.Done()
			defer func() { <-sem }()
			ictx, cancel := context.WithTimeout(ctx, 60*time.Second)
			defer cancel()
			dev, created, err := s.IdentifyAndUpsert(ictx, c.Address, c.Methods, c)
			if err != nil {
				status := "error"
				if errors.Is(err, engine.ErrNotIdentified) {
					status = "skipped" // responded to discovery but is not a manageable printer
				}
				jc.Result("", c.Address, status, err.Error(), map[string]any{"methods": c.Methods, "services": c.Services})
				return
			}
			msg := "updated"
			if created {
				msg = "discovered"
			}
			data := map[string]any{"methods": c.Methods, "model": dev.Model, "created": created}
			if len(dev.IdentifyErrors) > 0 {
				// Say which drivers did not answer rather than silently recording a partial read (issue #19).
				msg += "; no answer from " + strings.Join(sortedKeys(dev.IdentifyErrors), ", ")
				data["identifyErrors"] = dev.IdentifyErrors
			}
			// Read-only first poll so the record has status straight away.
			if dev.Decommissioned() {
				msg += "; decommissioned, not polled"
			} else if _, _, perr := s.PollDevice(ictx, dev); perr != nil {
				msg += "; poll: " + perr.Error()
			}
			jc.Result(dev.ID, c.Address, "ok", msg, data)
		}(c)
	}
	wg.Wait()
	return nil
}

// SubmitPoll queues a poll of the given devices (all pollable devices if empty).
// A list with only decommissioned devices is refused; in a longer list they are
// skipped when the job runs.
func (s *Service) SubmitPoll(ctx context.Context, deviceIDs []string, by string) (*model.Job, error) {
	if len(deviceIDs) > 0 {
		devs, err := s.Store.ListDevices(ctx, store.DeviceFilter{IDs: deviceIDs, IncludeDecommissioned: true})
		if err != nil {
			return nil, err
		}
		if model.AllDecommissioned(devs) {
			return nil, model.ErrDecommissioned
		}
	}
	ids := make([]any, len(deviceIDs))
	for i, id := range deviceIDs {
		ids[i] = id
	}
	target := map[string]any{"all": len(deviceIDs) == 0}
	if len(deviceIDs) > 0 {
		target["deviceIds"] = ids
	}
	return s.Jobs.Submit(ctx, &model.Job{Type: "poll", CreatedBy: by, Target: target, Params: map[string]any{}})
}

func (s *Service) runPoll(ctx context.Context, jc *jobs.Context) error {
	var devs []*model.Device
	var err error
	if ids, ok := jc.Job.Target["deviceIds"].([]any); ok && len(ids) > 0 {
		var sids []string
		for _, id := range ids {
			if sid, ok := id.(string); ok {
				sids = append(sids, sid)
			}
		}
		devs, err = s.Store.ListDevices(ctx, store.DeviceFilter{IDs: sids, IncludeDecommissioned: true})
	} else {
		devs, err = s.Store.ListDevices(ctx, store.DeviceFilter{})
	}
	if err != nil {
		return err
	}
	jc.SetTotal(len(devs))
	var wg sync.WaitGroup
	sem := make(chan struct{}, maxInt(1, s.Cfg.Poll.Concurrency))
	for _, d := range devs {
		if ctx.Err() != nil {
			break
		}
		if d.Decommissioned() {
			jc.Result(d.ID, d.Address, "skipped", model.ErrDecommissioned.Error(), nil)
			continue
		}
		wg.Add(1)
		sem <- struct{}{}
		go func(d *model.Device) {
			defer wg.Done()
			defer func() { <-sem }()
			// A panic in one device's poll must not take the process down
			// with every other job (code review H-004).
			defer func() {
				if p := recover(); p != nil {
					s.Log.Error("poll panic", "device", d.ID, "panic", p, "stack", string(debug.Stack()))
					jc.Result(d.ID, d.Address, "error", fmt.Sprintf("internal error: %v", p), nil)
				}
			}()
			pctx, cancel := context.WithTimeout(ctx, 90*time.Second)
			defer cancel()
			upd, res, err := s.PollDevice(pctx, d)
			if err != nil {
				jc.Result(d.ID, d.Address, "error", err.Error(), nil)
				return
			}
			data := map[string]any{"servedBy": res.ServedBy}
			if len(res.Errors) > 0 {
				data["errors"] = res.Errors
			}
			if !res.Reachable {
				jc.Result(d.ID, d.Address, "error", "unreachable", data)
				return
			}
			msg := "reachable, status not read"
			if upd.Status != nil {
				msg = fmt.Sprintf("%s (%s)", upd.Status.State, upd.Status.Severity)
			}
			jc.Result(upd.ID, upd.Address, "ok", msg, data)
		}(d)
	}
	wg.Wait()
	return nil
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

// --- scheduler --------------------------------------------------------------

// RunScheduler runs scheduled polls, optional scheduled discovery and
// telemetry pruning until ctx ends. Intervals are re-read from the runtime
// settings on every tick so operator changes apply without a restart.
func (s *Service) RunScheduler(ctx context.Context) {
	nextPoll := time.Now().Add(10 * time.Second) // first poll shortly after start
	nextDisc := time.Now().Add(s.Runtime(ctx).DiscoveryInterval.D())
	nextPrune := time.Now().Add(6 * time.Hour)
	tick := time.NewTicker(5 * time.Second)
	defer tick.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case now := <-tick.C:
			r := s.Runtime(ctx)
			if now.After(nextPoll) {
				s.scheduledPoll(ctx)
				nextPoll = now.Add(maxDur(r.PollInterval.D(), 30*time.Second))
			}
			if now.After(nextDisc) {
				if r.DiscoveryEnabled {
					if _, err := s.SubmitDiscovery(ctx, s.scheduledDiscoveryParams(r), "scheduler"); err != nil {
						s.Log.Warn("scheduled discovery", "err", err)
					}
				}
				nextDisc = now.Add(maxDur(r.DiscoveryInterval.D(), 5*time.Minute))
			}
			if now.After(nextPrune) {
				if n, err := s.Store.PruneTelemetry(ctx, now.Add(-r.TelemetryRetention.D())); err == nil && n > 0 {
					s.Log.Info("pruned telemetry", "rows", n)
				}
				nextPrune = now.Add(6 * time.Hour)
			}
		}
	}
}

// scheduledDiscoveryParams are the parameters of a scheduled discovery run. The
// zeroconf setting applies here only: a manual run states its own.
func (s *Service) scheduledDiscoveryParams(r RuntimeSettings) DiscoveryParams {
	return DiscoveryParams{Targets: r.DiscoveryTargets, ZeroConf: s.Cfg.Discovery.ZeroConf}
}

func (s *Service) scheduledPoll(ctx context.Context) {
	n, err := s.Store.CountDevices(ctx, store.DeviceFilter{})
	if err != nil || n == 0 {
		return
	}
	if running, _ := s.Store.ListJobs(ctx, store.JobFilter{Type: "poll", Status: string(model.JobRunning), Limit: 1}); len(running) > 0 {
		s.Log.Warn("skipping scheduled poll: previous poll still running")
		return
	}
	if _, err := s.SubmitPoll(ctx, nil, "scheduler"); err != nil {
		s.Log.Warn("scheduled poll", "err", err)
	}
}

func maxDur(a, b time.Duration) time.Duration {
	if a > b {
		return a
	}
	return b
}

// --- traps ------------------------------------------------------------------

// TrapAlertHold is how long a trap-raised alert survives successful polls.
// Traps report moments (a link flapped, the device restarted, an alert
// fired) rather than states the poller can confirm, so a trap alert is kept
// at least this long and then cleared by the next poll that reads the
// device's status without finding the condition (code review H-013). A
// recovery trap (link up) clears its counterpart at once.
var TrapAlertHold = 5 * time.Minute

// trapRecoveries maps a recovery trap's alert key to the key it resolves.
var trapRecoveries = map[string]string{"trap:link-up": "trap:link-down"}

func trapKey(description string) string {
	key := "trap:" + strings.ToLower(strings.ReplaceAll(description, " ", "-"))
	if len(key) > 120 {
		key = key[:120]
	}
	return key
}

// reconcileTrapAlerts clears trap alerts older than TrapAlertHold after a
// poll that read the device's status: the poll rules now carry whatever the
// device still reports, so the trap has served its purpose.
func (s *Service) reconcileTrapAlerts(ctx context.Context, dev *model.Device) {
	active, err := s.Store.ListAlerts(ctx, store.AlertFilter{DeviceID: dev.ID, ActiveOnly: true})
	if err != nil {
		return
	}
	var keep []string
	hold := false
	cutoff := time.Now().UTC().Add(-TrapAlertHold)
	for _, a := range active {
		if a.Source != "trap" {
			continue
		}
		if a.RaisedAt.After(cutoff) {
			keep = append(keep, a.Key)
		}
		hold = true
	}
	if !hold {
		return
	}
	cleared, err := s.Store.ClearAlertsExcept(ctx, dev.ID, "trap", keep)
	if err != nil {
		s.Log.Warn("clear trap alerts", "device", dev.ID, "err", err)
		return
	}
	for _, a := range cleared {
		s.emit("alert.cleared", a)
	}
}

// HandleTrap records a device-pushed event and schedules a reconciling poll
// (spec §8.3). Informational traps (severity ok) raise nothing but may clear
// the alert of the condition they end.
func (s *Service) HandleTrap(ctx context.Context, sourceIP string, description string, severity model.Severity, varbinds map[string]string) {
	devs, err := s.Store.ListDevices(ctx, store.DeviceFilter{Q: sourceIP, IncludeDecommissioned: true})
	if err != nil {
		return
	}
	// An address can have a retired record and its replacement's: the printer
	// in service is the one that sent this.
	var dev *model.Device
	for _, d := range devs {
		if d.Address == sourceIP && (dev == nil || dev.Decommissioned()) {
			dev = d
		}
	}
	if dev == nil {
		s.Log.Info("trap from unknown device", "source", sourceIP, "description", description)
		return
	}
	if dev.Decommissioned() {
		s.Log.Info("trap from a decommissioned device's address ignored", "source", sourceIP, "description", description)
		return
	}
	s.Log.Info("trap", "device", dev.Name(), "description", description, "severity", severity)
	key := trapKey(description)
	if severity == model.SeverityOK {
		if resolves, ok := trapRecoveries[key]; ok {
			active, _ := s.Store.ListAlerts(ctx, store.AlertFilter{DeviceID: dev.ID, ActiveOnly: true})
			for _, a := range active {
				if a.Source == "trap" && a.Key == resolves {
					if c, err := s.Store.ClearAlert(ctx, a.ID); err == nil {
						s.emit("alert.cleared", c)
					}
				}
			}
		}
		if _, err := s.SubmitPoll(ctx, []string{dev.ID}, "trap"); err != nil {
			s.Log.Warn("trap poll", "err", err)
		}
		return
	}
	if severity == "" {
		severity = model.SeverityWarning
	}
	a, raised, err := s.Store.RaiseAlert(ctx, dev.ID, key, "trap", severity, description)
	if err == nil && raised {
		s.emit("alert.raised", a)
	}
	// Reconcile trap state with polled state.
	if _, err := s.SubmitPoll(ctx, []string{dev.ID}, "trap"); err != nil {
		s.Log.Warn("trap poll", "err", err)
	}
}

func sortedKeys(m map[string]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
