// Package fleet wires the engine, store, jobs and alerts into the operations
// the API and scheduler call: discovery runs, polls, alert reconciliation and
// trap handling. Everything here is read-only against devices.
package fleet

import (
	"context"
	"fmt"
	"log/slog"
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
	Store    store.Store
	Engine   *engine.Engine
	Jobs     *jobs.Runner
	Cfg      *config.Config
	Log      *slog.Logger
	Events   func(model.Event)
	Notifier *alerts.Notifier
	// Consumables links installed supplies to stock items and forecasts them.
	Consumables *consumables.Service
	// Probe is the cheap liveness check used by active scans.
	Probe discovery.Prober

	pollMu  sync.Mutex
	polling map[string]bool
}

// New builds a service and registers its job types.
func New(st store.Store, eng *engine.Engine, runner *jobs.Runner, cfg *config.Config, log *slog.Logger, events func(model.Event), notifier *alerts.Notifier, probe discovery.Prober) *Service {
	s := &Service{Store: st, Engine: eng, Jobs: runner, Cfg: cfg, Log: log, Events: events, Notifier: notifier, Probe: probe, polling: map[string]bool{},
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
	return alerts.Thresholds{
		SupplyWarningPercent:  s.Cfg.Alerts.SupplyWarningPercent,
		SupplyCriticalPercent: s.Cfg.Alerts.SupplyCriticalPercent,
		OfflineAfterFailures:  s.Cfg.Alerts.OfflineAfterFailures,
	}
}

// Creds resolves credentials for a device (device scope > group scope > global).
func (s *Service) Creds(ctx context.Context, dev *model.Device) model.Creds {
	var groups []string
	if dev != nil && dev.ID != "" {
		groups, _ = s.Store.GroupsForDevice(ctx, dev.ID)
		// Rule-based groups also contribute scope.
		if all, err := s.Store.ListGroups(ctx); err == nil {
			for _, g := range all {
				if g.Kind == "rule" && store.MatchRule(g.Rule, dev) {
					groups = append(groups, g.ID)
				}
			}
		}
	}
	id := ""
	if dev != nil {
		id = dev.ID
	}
	creds, err := s.Store.ResolveCreds(ctx, id, groups)
	if err != nil {
		s.Log.Warn("resolve credentials", "device", id, "err", err)
	}
	return creds
}

// IdentifyAndUpsert identifies an address and records it (spec §8.1).
func (s *Service) IdentifyAndUpsert(ctx context.Context, address string, methods []string, hints discovery.Candidate) (*model.Device, bool, error) {
	creds := s.Creds(ctx, &model.Device{Address: address})
	ident, caps, err := s.Engine.Identify(ctx, address, creds)
	if err != nil {
		return nil, false, err
	}
	applyHints(ident, hints)
	dev, created, err := s.Store.UpsertDevice(ctx, ident, caps, methods)
	if err != nil {
		return nil, false, err
	}
	if created {
		s.emit("device.discovered", dev)
		if s.Notifier != nil {
			s.Notifier.Notify("device.discovered", "New device: "+dev.Name(), fmt.Sprintf("%s %s at %s (serial %s)", dev.Make, dev.Model, dev.Address, dev.Serial), dev)
		}
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
// reconciles alerts. It never writes to the device.
func (s *Service) PollDevice(ctx context.Context, dev *model.Device) (*model.Device, *engine.PollResult, error) {
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
	} else {
		_ = s.Store.AddSamples(ctx, []model.TelemetrySample{{DeviceID: dev.ID, Metric: "status.online", Value: 0, At: res.At}})
	}
	failures, _ := s.Store.DeviceFailures(ctx, dev.ID)
	s.reconcileAlerts(ctx, updated, alerts.Evaluate(updated, res, failures, s.thresholds()), alerts.Sources)
	if res.Reachable && res.Supplies != nil {
		s.afterSupplies(ctx, dev, updated)
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
			if s.Notifier != nil {
				s.Notifier.Notify("alert.raised", fmt.Sprintf("[%s] %s: %s", strings.ToUpper(string(a.Severity)), dev.Name(), a.Description),
					fmt.Sprintf("%s (%s, %s)\n%s", dev.Name(), dev.Model, dev.Address, a.Description), map[string]any{"alert": a, "device": dev})
			}
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
			if s.Notifier != nil {
				s.Notifier.Notify("alert.cleared", fmt.Sprintf("[CLEARED] %s: %s", dev.Name(), a.Description),
					fmt.Sprintf("%s (%s)\n%s is resolved", dev.Name(), dev.Address, a.Description), map[string]any{"alert": a, "device": dev})
			}
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
				jc.Result("", c.Address, "error", err.Error(), map[string]any{"methods": c.Methods})
				return
			}
			msg := "updated"
			if created {
				msg = "discovered"
			}
			// Read-only first poll so the record has status straight away.
			if _, _, perr := s.PollDevice(ictx, dev); perr != nil {
				msg += "; poll: " + perr.Error()
			}
			jc.Result(dev.ID, c.Address, "ok", msg, map[string]any{"methods": c.Methods, "model": dev.Model, "created": created})
		}(c)
	}
	wg.Wait()
	return nil
}

// SubmitPoll queues a poll of the given devices (all pollable devices if empty).
func (s *Service) SubmitPoll(ctx context.Context, deviceIDs []string, by string) (*model.Job, error) {
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
		wg.Add(1)
		sem <- struct{}{}
		go func(d *model.Device) {
			defer wg.Done()
			defer func() { <-sem }()
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
			jc.Result(upd.ID, upd.Address, "ok", fmt.Sprintf("%s (%s)", upd.Status.State, upd.Status.Severity), data)
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
// telemetry pruning until ctx ends.
func (s *Service) RunScheduler(ctx context.Context) {
	poll := time.NewTicker(maxDur(s.Cfg.Poll.Interval.D(), 30*time.Second))
	defer poll.Stop()
	disc := time.NewTicker(maxDur(s.Cfg.Discovery.Interval.D(), 5*time.Minute))
	defer disc.Stop()
	prune := time.NewTicker(6 * time.Hour)
	defer prune.Stop()

	// Stock alerts reflect the current inventory straight away.
	s.ReconcileConsumables(ctx)

	// First poll shortly after start.
	first := time.NewTimer(10 * time.Second)
	defer first.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-first.C:
			s.scheduledPoll(ctx)
		case <-poll.C:
			s.scheduledPoll(ctx)
		case <-disc.C:
			if s.Cfg.Discovery.Enabled {
				if _, err := s.SubmitDiscovery(ctx, DiscoveryParams{Targets: s.Cfg.Discovery.Targets, ZeroConf: true}, "scheduler"); err != nil {
					s.Log.Warn("scheduled discovery", "err", err)
				}
			}
		case <-prune.C:
			if n, err := s.Store.PruneTelemetry(ctx, time.Now().Add(-s.Cfg.Telemetry.Retention.D())); err == nil && n > 0 {
				s.Log.Info("pruned telemetry", "rows", n)
			}
		}
	}
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

// HandleTrap records a device-pushed event and schedules a reconciling poll (spec §8.3).
func (s *Service) HandleTrap(ctx context.Context, sourceIP string, description string, severity model.Severity, varbinds map[string]string) {
	devs, err := s.Store.ListDevices(ctx, store.DeviceFilter{Q: sourceIP, IncludeDecommissioned: true})
	if err != nil {
		return
	}
	var dev *model.Device
	for _, d := range devs {
		if d.Address == sourceIP {
			dev = d
			break
		}
	}
	if dev == nil {
		s.Log.Info("trap from unknown device", "source", sourceIP, "description", description)
		return
	}
	s.Log.Info("trap", "device", dev.Name(), "description", description, "severity", severity)
	key := "trap:" + strings.ToLower(strings.ReplaceAll(description, " ", "-"))
	if len(key) > 120 {
		key = key[:120]
	}
	if severity == "" {
		severity = model.SeverityWarning
	}
	a, raised, err := s.Store.RaiseAlert(ctx, dev.ID, key, "trap", severity, description)
	if err == nil && raised {
		s.emit("alert.raised", a)
		if s.Notifier != nil {
			s.Notifier.Notify("alert.raised", fmt.Sprintf("[TRAP] %s: %s", dev.Name(), description), fmt.Sprintf("%s (%s) sent a trap: %s", dev.Name(), dev.Address, description), map[string]any{"alert": a, "device": dev, "varbinds": varbinds})
		}
	}
	// Reconcile trap state with polled state.
	if _, err := s.SubmitPoll(ctx, []string{dev.ID}, "trap"); err != nil {
		s.Log.Warn("trap poll", "err", err)
	}
}
