// Package configmgmt implements configuration management (spec §8.4):
// normalised config reads, profiles, drift reports and gated writes.
//
// Safety rules (spec §3 principle 4, §11):
//   - every apply runs as a job with per-device, per-setting results;
//   - a dry run performs no write I/O on any device;
//   - every real write is audited (actor, device, before/after) and keeps the
//     previous values so it can be reverted;
//   - secret-typed catalogue values are redacted before anything is persisted;
//   - device I/O goes through the engine's limiter.
package configmgmt

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"printmarshal/internal/engine"
	"printmarshal/internal/fleet"
	"printmarshal/internal/jobs"
	"printmarshal/internal/model"
	"printmarshal/internal/store"
)

// JobType is the apply job's type.
const JobType = "config.apply"

// SnapshotsKept is how many snapshots per device are retained.
const SnapshotsKept = 50

// Service is the configuration-management façade.
type Service struct {
	fleet.Deps
	pending pendingChanges
}

// New builds the service and registers its job types.
func New(d fleet.Deps) *Service {
	s := &Service{Deps: d}
	if d.Jobs != nil {
		d.Jobs.Register(JobType, s.runApply)
	}
	return s
}

// ApplyRequest describes one apply: a profile's settings or ad-hoc changes,
// against a set of devices, as a dry run or for real.
type ApplyRequest struct {
	ProfileID string                `json:"profileId,omitempty"`
	Changes   []model.SettingChange `json:"changes,omitempty"`
	Target    model.ConfigTarget    `json:"target"`
	DryRun    bool                  `json:"dryRun"`
	// RevertOf marks a revert of an earlier write record.
	RevertOf string `json:"revertOf,omitempty"`
}

// ErrNoTarget is returned when an apply names no device.
var ErrNoTarget = errors.New("target selects no device")

// ErrNoChanges is returned when there is nothing to apply.
var ErrNoChanges = errors.New("no settings to apply")

func (s *Service) emit(typ string, data any) {
	if s.Events != nil {
		s.Events(model.Event{Type: typ, At: time.Now().UTC(), Data: data})
	}
}

func (s *Service) acquire(ctx context.Context) (func(), error) {
	if s.Engine == nil || s.Engine.Limiter == nil {
		return func() {}, nil
	}
	select {
	case s.Engine.Limiter <- struct{}{}:
		return func() { <-s.Engine.Limiter }, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// credsFor resolves a device's credentials for a purpose (audit A19): reads
// and dry runs use read credentials, real writes use write credentials and
// fail with store.ErrNoWriteCredential when none is bound.
func (s *Service) credsFor(ctx context.Context, dev *model.Device, purpose model.CredentialPurpose) (model.Creds, error) {
	if s.Fleet != nil {
		return s.Fleet.CredsFor(ctx, dev, purpose)
	}
	return store.CredsFor(ctx, s.Store, dev, purpose)
}

// writeCredsError is the reason a per-device result carries when the write
// credentials could not be resolved (audit A19): a readable device is never
// written to with a read credential. store.ErrNoWriteCredential already says
// what to bind.
func writeCredsError(err error) error {
	if errors.Is(err, store.ErrNoWriteCredential) {
		return err
	}
	return fmt.Errorf("resolve write credentials: %w", err)
}

// Catalog returns the canonical setting catalogue.
func (s *Service) Catalog() []model.SettingDef { return model.Catalog }

// --- reads -------------------------------------------------------------------

// ReadConfig reads a device's configuration through every driver that serves
// read_config, merges the documents (the best driver's value wins per key,
// each setting keeps its Source) and stores the result as a snapshot.
func (s *Service) ReadConfig(ctx context.Context, deviceID string, sel model.ConfigSelector) (*model.ConfigDocument, error) {
	dev, err := s.Store.GetDevice(ctx, deviceID)
	if err != nil {
		return nil, err
	}
	if dev.Decommissioned() {
		return nil, model.ErrDecommissioned
	}
	doc, err := s.readDevice(ctx, dev, sel)
	if err != nil {
		return nil, err
	}
	if sel.Areas == nil && sel.Keys == nil {
		if err := s.Store.SaveConfigSnapshot(ctx, &model.ConfigSnapshot{DeviceID: dev.ID, Document: doc, At: doc.ReadAt}); err != nil {
			s.Log.Warn("save config snapshot", "device", dev.ID, "err", err)
		} else {
			_, _ = s.Store.PruneConfigSnapshots(ctx, dev.ID, SnapshotsKept)
		}
	}
	return doc, nil
}

// configDrivers returns the drivers to consult for cap on dev, best first: the
// profile's winner (engine.Resolve) followed by every other driver that declares
// cap and identified the device, so a merged read can take identity.contact from
// SNMP while IPP wins the keys both report.
func (s *Service) configDrivers(dev *model.Device, cap model.Capability) []engine.Driver {
	out := s.Engine.Resolve(dev, cap)
	seen := map[string]bool{}
	for _, d := range out {
		seen[d.Name()] = true
	}
	identified := map[string]bool{}
	for _, m := range dev.IdentifyMethods {
		identified[m] = true
	}
	var extra []engine.Driver
	for _, d := range s.Engine.Registry.Drivers() {
		if seen[d.Name()] || !identified[d.Name()] {
			continue
		}
		for _, c := range d.Capabilities() {
			if c == cap {
				extra = append(extra, d)
				break
			}
		}
	}
	sort.SliceStable(extra, func(i, j int) bool { return extra[i].Priority(cap) > extra[j].Priority(cap) })
	return append(out, extra...)
}

func (s *Service) readDevice(ctx context.Context, dev *model.Device, sel model.ConfigSelector) (*model.ConfigDocument, error) {
	drivers := s.configDrivers(dev, model.CapReadConfig)
	if len(drivers) == 0 {
		return nil, engine.ErrUnsupported
	}
	release, err := s.acquire(ctx)
	if err != nil {
		return nil, err
	}
	defer release()
	creds, _ := s.credsFor(ctx, dev, model.PurposeRead)
	merged := &model.ConfigDocument{DeviceID: dev.ID, Settings: []model.Setting{}, ReadAt: time.Now().UTC(), Sources: []string{}}
	seen := map[string]int{} // key -> index in merged.Settings
	var errs []error
	for _, drv := range drivers {
		r, ok := drv.(engine.ConfigReader)
		if !ok {
			continue
		}
		doc, err := r.ReadConfig(ctx, dev, creds, sel)
		if err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", drv.Name(), err))
			continue
		}
		merged.Sources = append(merged.Sources, drv.Name())
		for _, st := range doc.Settings {
			if i, ok := seen[st.Key]; ok {
				// A lower-priority driver that can write the key makes it writable:
				// the apply falls through to it when the winner skips the key.
				if st.Writable && !merged.Settings[i].Writable {
					merged.Settings[i].Writable = true
					merged.Settings[i].ReadOnlyReason = ""
				}
				continue
			}
			if st.Source == "" {
				st.Source = drv.Name()
			}
			if model.IsSecretKey(st.Key) {
				st.Value = model.RedactedValue
			}
			seen[st.Key] = len(merged.Settings)
			merged.Settings = append(merged.Settings, st)
		}
	}
	if len(merged.Sources) == 0 {
		return nil, errors.Join(errs...)
	}
	sortSettings(merged.Settings)
	return merged, nil
}

// sortSettings orders by catalogue area then key, unknown keys last.
func sortSettings(in []model.Setting) {
	rank := map[string]int{}
	for i, a := range model.Areas {
		rank[a] = i
	}
	sort.SliceStable(in, func(i, j int) bool {
		ri, rj := rank[in[i].Area], rank[in[j].Area]
		if ri != rj {
			return ri < rj
		}
		return in[i].Key < in[j].Key
	})
}

// CurrentConfig returns the latest snapshot's document, reading from the
// device when refresh is set or no snapshot exists yet. A decommissioned device
// is never read: those two cases return model.ErrDecommissioned.
func (s *Service) CurrentConfig(ctx context.Context, deviceID string, refresh bool) (*model.ConfigDocument, *model.ConfigSnapshot, error) {
	if !refresh {
		if sn, err := s.Store.LatestConfigSnapshot(ctx, deviceID); err == nil {
			return sn.Document, sn, nil
		} else if !errors.Is(err, store.ErrNotFound) {
			return nil, nil, err
		}
	}
	doc, err := s.ReadConfig(ctx, deviceID, model.ConfigSelector{})
	if err != nil {
		return nil, nil, err
	}
	sn, _ := s.Store.LatestConfigSnapshot(ctx, deviceID)
	return doc, sn, nil
}

// --- diff and drift ------------------------------------------------------------

// Diff compares two documents key by key.
func Diff(a, b *model.ConfigDocument) *model.ConfigDiff {
	out := &model.ConfigDiff{Items: []model.ConfigDiffItem{}}
	if a != nil {
		out.A = a.DeviceID
	}
	if b != nil {
		out.B = b.DeviceID
	}
	av, bv := index(a), index(b)
	keys := map[string]bool{}
	for k := range av {
		keys[k] = true
	}
	for k := range bv {
		keys[k] = true
	}
	for _, k := range sortedKeys(keys) {
		def := model.CatalogByKey[k]
		x, inA := av[k]
		y, inB := bv[k]
		switch {
		case inA && inB && model.ValuesEqual(x, y):
			out.Same++
		case inA && inB:
			out.Items = append(out.Items, model.ConfigDiffItem{Key: k, Name: def.Name, Area: def.Area, A: x, B: y})
		case inA:
			out.Items = append(out.Items, model.ConfigDiffItem{Key: k, Name: def.Name, Area: def.Area, A: x, OnlyA: true})
		default:
			out.Items = append(out.Items, model.ConfigDiffItem{Key: k, Name: def.Name, Area: def.Area, B: y, OnlyB: true})
		}
	}
	return out
}

func index(d *model.ConfigDocument) map[string]any {
	m := map[string]any{}
	if d == nil {
		return m
	}
	for _, s := range d.Settings {
		m[s.Key] = s.Value
	}
	return m
}

func sortedKeys(m map[string]bool) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// DriftOf compares a document with a profile's desired settings.
func DriftOf(doc *model.ConfigDocument, p *model.Profile) *model.DriftReport {
	r := &model.DriftReport{ProfileID: p.ID, Items: []model.DriftItem{}, At: time.Now().UTC()}
	if doc != nil {
		r.DeviceID = doc.DeviceID
	}
	cur := index(doc)
	for _, want := range p.Settings {
		def := model.CatalogByKey[want.Key]
		desired, err := model.CoerceValue(want.Key, want.Value)
		if err != nil {
			desired = want.Value
		}
		got, ok := cur[want.Key]
		switch {
		case !ok:
			r.Items = append(r.Items, model.DriftItem{Key: want.Key, Name: def.Name, Desired: desired, Missing: true})
		case model.ValuesEqual(got, desired):
			r.Matching++
		default:
			r.Items = append(r.Items, model.DriftItem{Key: want.Key, Name: def.Name, Desired: desired, Current: got})
			r.Drifted++
		}
	}
	return r
}

// Drift compares one device with a profile, using the latest snapshot unless
// refresh is set or none exists. The report is cached in the store.
func (s *Service) Drift(ctx context.Context, deviceID, profileID string, refresh bool) (*model.DriftReport, error) {
	p, err := s.Store.GetProfile(ctx, profileID)
	if err != nil {
		return nil, err
	}
	doc, _, err := s.CurrentConfig(ctx, deviceID, refresh)
	if err != nil {
		return nil, err
	}
	r := DriftOf(doc, p)
	r.DeviceID = deviceID
	if err := s.Store.SaveDriftReport(ctx, r); err != nil {
		s.Log.Warn("save drift report", "device", deviceID, "err", err)
	}
	return r, nil
}

// DriftGroup runs Drift for every device of a target. Devices that cannot be
// read produce a report with no items and the error in its only item's name.
func (s *Service) DriftGroup(ctx context.Context, target model.ConfigTarget, profileID string, refresh bool) ([]*model.DriftReport, error) {
	devs, err := s.ResolveTarget(ctx, target)
	if err != nil {
		return nil, err
	}
	if _, err := s.Store.GetProfile(ctx, profileID); err != nil {
		return nil, err
	}
	out := make([]*model.DriftReport, 0, len(devs))
	for _, d := range devs {
		r, err := s.Drift(ctx, d.ID, profileID, refresh)
		if err != nil {
			r = &model.DriftReport{DeviceID: d.ID, ProfileID: profileID, At: time.Now().UTC(),
				Items: []model.DriftItem{{Key: "", Name: "read failed: " + err.Error(), Missing: true}}}
		}
		out = append(out, r)
	}
	return out, nil
}

// ResolveTarget expands a target into devices: the listed ids plus every
// member of the group (static membership or rule), de-duplicated and sorted.
func (s *Service) ResolveTarget(ctx context.Context, t model.ConfigTarget) ([]*model.Device, error) {
	if t.Empty() {
		return nil, ErrNoTarget
	}
	byID := map[string]*model.Device{}
	if len(t.DeviceIDs) > 0 {
		devs, err := s.Store.ListDevices(ctx, store.DeviceFilter{IDs: t.DeviceIDs, IncludeDecommissioned: true})
		if err != nil {
			return nil, err
		}
		for _, d := range devs {
			byID[d.ID] = d
		}
		for _, id := range t.DeviceIDs {
			if _, ok := byID[id]; !ok {
				return nil, fmt.Errorf("device %s: %w", id, store.ErrNotFound)
			}
		}
	}
	if t.GroupID != "" {
		g, err := s.Store.GetGroup(ctx, t.GroupID)
		if err != nil {
			return nil, err
		}
		var devs []*model.Device
		if g.Kind == "rule" {
			all, err := s.Store.ListDevices(ctx, store.DeviceFilter{})
			if err != nil {
				return nil, err
			}
			for _, d := range all {
				if store.MatchRule(g.Rule, d) {
					devs = append(devs, d)
				}
			}
		} else {
			devs, err = s.Store.ListDevices(ctx, store.DeviceFilter{GroupIDs: []string{g.ID}})
			if err != nil {
				return nil, err
			}
		}
		for _, d := range devs {
			byID[d.ID] = d
		}
	}
	out := make([]*model.Device, 0, len(byID))
	for _, d := range byID {
		out = append(out, d)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name() < out[j].Name() })
	if len(out) == 0 {
		return nil, ErrNoTarget
	}
	return out, nil
}

// --- apply ---------------------------------------------------------------------

// Apply validates the request and submits a config.apply job. The API accepts a
// single device as well as a group; starting with one device before a group is
// the operator's (UI's) discipline, the service enforces dry-run semantics.
func (s *Service) Apply(ctx context.Context, req ApplyRequest, actor string) (*model.Job, error) {
	changes := req.Changes
	if req.ProfileID != "" {
		p, err := s.Store.GetProfile(ctx, req.ProfileID)
		if err != nil {
			return nil, err
		}
		changes = append(append([]model.SettingChange{}, p.Settings...), changes...)
	}
	if len(changes) == 0 {
		return nil, ErrNoChanges
	}
	for _, ch := range changes {
		if ch.Key == "" {
			return nil, errors.New("a change has no key")
		}
		if _, err := model.CoerceValue(ch.Key, ch.Value); err != nil {
			return nil, err
		}
	}
	devs, err := s.ResolveTarget(ctx, req.Target)
	if err != nil {
		return nil, err
	}
	if model.AllDecommissioned(devs) {
		if !req.DryRun {
			for _, d := range devs {
				s.auditDecommissionedRefusal(ctx, d, actor)
			}
		}
		return nil, model.ErrDecommissioned
	}
	ids := make([]any, 0, len(devs))
	for _, d := range devs {
		ids = append(ids, d.ID)
	}
	target := map[string]any{"deviceIds": ids}
	if req.Target.GroupID != "" {
		target["groupId"] = req.Target.GroupID
	}
	chs := make([]any, 0, len(changes))
	for _, ch := range changes {
		v := ch.Value
		if model.IsSecretKey(ch.Key) {
			// Secrets are carried in memory only; the persisted job shows a marker.
			v = model.RedactedValue
		}
		chs = append(chs, map[string]any{"key": ch.Key, "value": v})
	}
	params := map[string]any{"dryRun": req.DryRun, "changes": chs}
	if req.ProfileID != "" {
		params["profileId"] = req.ProfileID
	}
	if req.RevertOf != "" {
		params["revertOf"] = req.RevertOf
	}
	j := &model.Job{Type: JobType, CreatedBy: actor, Target: target, Params: params}
	s.pending.Store(j, changes)
	job, err := s.Jobs.Submit(ctx, j)
	if err != nil {
		s.pending.Delete(j)
		return nil, err
	}
	return job, nil
}

func (s *Service) runApply(ctx context.Context, jc *jobs.Context) error {
	j := jc.Job
	dryRun, _ := j.Params["dryRun"].(bool)
	profileID, _ := j.Params["profileId"].(string)
	revertOf, _ := j.Params["revertOf"].(string)
	changes := s.takePending(j)
	if changes == nil {
		changes = changesFromParams(j)
	}
	if len(changes) == 0 {
		return ErrNoChanges
	}
	var ids []string
	if raw, ok := j.Target["deviceIds"].([]any); ok {
		for _, v := range raw {
			if id, ok := v.(string); ok {
				ids = append(ids, id)
			}
		}
	}
	devs, err := s.Store.ListDevices(ctx, store.DeviceFilter{IDs: ids, IncludeDecommissioned: true})
	if err != nil {
		return err
	}
	jc.SetTotal(len(devs))
	conc := 4
	if s.Cfg != nil && s.Cfg.Poll.Concurrency > 0 {
		conc = s.Cfg.Poll.Concurrency
	}
	sem := make(chan struct{}, conc)
	var wg sync.WaitGroup
	for _, d := range devs {
		if ctx.Err() != nil {
			break
		}
		wg.Add(1)
		sem <- struct{}{}
		go func(d *model.Device) {
			defer wg.Done()
			defer func() { <-sem }()
			dctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
			defer cancel()
			rec, err := s.applyDevice(dctx, d, changes, model.WriteOptions{DryRun: dryRun}, j.CreatedBy, j.ID, profileID, revertOf)
			if errors.Is(err, model.ErrDecommissioned) {
				jc.Result(d.ID, d.Address, "skipped", err.Error(), nil)
				return
			}
			if err != nil {
				jc.Result(d.ID, d.Address, "error", err.Error(), nil)
				return
			}
			status := "ok"
			if rec.Failed > 0 {
				status = "error"
			}
			msg := fmt.Sprintf("%d written, %d unchanged, %d skipped, %d failed", rec.Written, rec.Unchanged, rec.Skipped, rec.Failed)
			if dryRun {
				// A dry run writes nothing: say what would be written (#51).
				msg = fmt.Sprintf("dry run: %d would be written, %d unchanged, %d skipped, %d failed", rec.Written, rec.Unchanged, rec.Skipped, rec.Failed)
			} else if rec.Written > 0 {
				msg += fmt.Sprintf(" (%d verified, %d diverged)", rec.Verified, rec.Diverged)
			}
			jc.Result(d.ID, d.Address, status, msg, map[string]any{"writeId": rec.ID, "dryRun": dryRun, "results": rec.Result.Results,
				"verified": rec.Verified, "diverged": rec.Diverged})
		}(d)
	}
	wg.Wait()
	return nil
}

// applyDevice writes the changes on one device: the best write_config driver
// first, and settings it skips are offered to the next driver. The merged
// result is persisted, audited and announced. A decommissioned device is
// refused, dry run included, since a dry run reads the device.
func (s *Service) applyDevice(ctx context.Context, dev *model.Device, changes []model.SettingChange, opts model.WriteOptions, actor, jobID, profileID, revertOf string) (*model.WriteRecord, error) {
	if dev.Decommissioned() {
		if !opts.DryRun {
			s.auditDecommissionedRefusal(ctx, dev, actor)
		}
		return nil, model.ErrDecommissioned
	}
	drivers := s.configDrivers(dev, model.CapWriteConfig)
	var writers []engine.ConfigWriter
	var names []string
	for _, drv := range drivers {
		if w, ok := drv.(engine.ConfigWriter); ok {
			writers = append(writers, w)
			names = append(names, drv.Name())
		}
	}
	if len(writers) == 0 {
		return nil, engine.ErrUnsupported
	}
	// A dry run only reads, so read credentials suffice; a real apply needs a
	// write credential bound to the device (audit A19) and is refused otherwise.
	purpose := model.PurposeWrite
	if opts.DryRun {
		purpose = model.PurposeRead
	}
	creds, err := s.credsFor(ctx, dev, purpose)
	if err != nil && !opts.DryRun {
		err = writeCredsError(err)
		_ = s.Store.AddAudit(ctx, &model.AuditEntry{Actor: actor, Action: JobType, Target: dev.ID,
			After: map[string]any{"jobId": jobID, "reason": err.Error()}, Result: "refused"})
		return nil, err
	}
	release, err := s.acquire(ctx)
	if err != nil {
		return nil, err
	}
	// The permit covers the writes only: the verification read below goes
	// through readDevice, which takes its own permit, so holding this one
	// across it deadlocks at concurrency 1 (or when a batch fills every
	// slot) until the device context expires (code review H-005).
	released := false
	releaseOnce := func() {
		if !released {
			released = true
			release()
		}
	}
	defer releaseOnce()

	final := map[string]model.SettingResult{}
	skipReasons := map[string][]string{} // key -> "<driver>: <reason>" from every driver that skipped it
	var rollback []model.SettingChange
	remaining := changes
	var errs []error
	for i, w := range writers {
		if len(remaining) == 0 {
			break
		}
		res, err := w.WriteConfig(ctx, dev, creds, remaining, opts)
		if err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", names[i], err))
			continue
		}
		var next []model.SettingChange
		byKey := map[string]model.SettingResult{}
		for _, r := range res.Results {
			byKey[r.Key] = r
		}
		for _, ch := range remaining {
			r, ok := byKey[ch.Key]
			if ok && r.Status == model.SettingSkipped {
				skipReasons[ch.Key] = append(skipReasons[ch.Key], names[i]+": "+r.Error)
				if i < len(writers)-1 {
					next = append(next, ch) // offer it to the next driver
					continue
				}
				r.Error, r.Driver = strings.Join(skipReasons[ch.Key], "; "), ""
			}
			if !ok {
				next = append(next, ch)
				continue
			}
			final[ch.Key] = r
		}
		rollback = append(rollback, res.Rollback...)
		remaining = next
	}
	releaseOnce()
	if len(final) == 0 && len(errs) > 0 {
		return nil, errors.Join(errs...)
	}
	results := make([]model.SettingResult, 0, len(changes))
	for _, ch := range changes {
		r, ok := final[ch.Key]
		if !ok {
			r = model.SettingResult{Key: ch.Key, Status: model.SettingSkipped, Error: "no driver serves this setting"}
			if rs := skipReasons[ch.Key]; len(rs) > 0 {
				r.Error = strings.Join(rs, "; ")
			}
			if len(errs) > 0 {
				r.Status, r.Error = model.SettingFailed, errors.Join(errs...).Error()
			}
		}
		results = append(results, redactResult(r))
	}
	if !opts.DryRun && anyWritten(results) {
		// Re-read the device (audit A20): a write the device accepted is only
		// "verified" once the read-back equals the desired value. This also
		// refreshes the snapshot so the stored document reflects the device.
		doc, err := s.ReadConfig(ctx, dev.ID, model.ConfigSelector{})
		if err != nil {
			s.Log.Warn("config re-read after write", "device", dev.ID, "err", err)
		}
		VerifyOutcomes(results, changes, doc)
	}
	rec := &model.WriteRecord{JobID: jobID, ProfileID: profileID, RevertOf: revertOf, Actor: actor, Changes: redactChanges(changes),
		Result: model.WriteResult{DeviceID: dev.ID, DryRun: opts.DryRun, Results: results, At: time.Now().UTC(), Rollback: redactChanges(rollback)}}
	rec.Count()
	// The device may already hold the new settings, so the record (with its
	// rollback data) and the audit entry must be written even when the
	// device context has since expired: use a bounded context of their own.
	pctx, pcancel := context.WithTimeout(context.WithoutCancel(ctx), 30*time.Second)
	defer pcancel()
	if err := s.Store.SaveWriteRecord(pctx, rec); err != nil {
		return nil, err
	}
	s.audit(pctx, dev, rec)
	s.emit("config.applied", map[string]any{"deviceId": dev.ID, "jobId": jobID, "writeId": rec.ID, "dryRun": opts.DryRun,
		"written": rec.Written, "failed": rec.Failed, "skipped": rec.Skipped, "unchanged": rec.Unchanged,
		"verified": rec.Verified, "diverged": rec.Diverged})
	return rec, nil
}

// auditDecommissionedRefusal records a real write that was refused, like the
// refusal for a missing write credential.
func (s *Service) auditDecommissionedRefusal(ctx context.Context, dev *model.Device, actor string) {
	_ = s.Store.AddAudit(ctx, &model.AuditEntry{Actor: actor, Action: JobType, Target: dev.ID,
		After: map[string]any{"reason": model.ErrDecommissioned.Error()}, Result: "refused"})
}

func anyWritten(results []model.SettingResult) bool {
	for _, r := range results {
		if r.Status == model.SettingWritten {
			return true
		}
	}
	return false
}

// VerifyOutcomes fills the Outcome of every written setting from a re-read of
// the device (audit A20): verified when the read-back equals the desired
// value, diverged (with ReadBack) when it differs, unverifiable when no reader
// reports the key or the key is a secret (its value is never compared or
// stored), and written when the re-read itself failed (doc == nil), meaning
// accepted but not verified.
func VerifyOutcomes(results []model.SettingResult, changes []model.SettingChange, doc *model.ConfigDocument) {
	desired := map[string]any{}
	for _, ch := range changes {
		v, err := model.CoerceValue(ch.Key, ch.Value)
		if err != nil {
			v = ch.Value
		}
		desired[ch.Key] = v
	}
	for i := range results {
		r := &results[i]
		if r.Status != model.SettingWritten {
			continue
		}
		switch {
		case doc == nil:
			r.Outcome = model.OutcomeWritten
		case model.IsSecretKey(r.Key):
			r.Outcome = model.OutcomeUnverifiable
		default:
			st, ok := doc.Get(r.Key)
			if !ok {
				r.Outcome = model.OutcomeUnverifiable
				continue
			}
			if model.ValuesEqual(st.Value, desired[r.Key]) {
				r.Outcome = model.OutcomeVerified
			} else {
				r.Outcome, r.ReadBack = model.OutcomeDiverged, st.Value
			}
		}
	}
}

// audit records one entry per apply. Real writes carry before/after per key;
// a dry run is recorded with result "dry-run" so the trail shows the rehearsal.
func (s *Service) audit(ctx context.Context, dev *model.Device, rec *model.WriteRecord) {
	before, after := map[string]any{}, map[string]any{}
	for _, r := range rec.Result.Results {
		if r.Status == model.SettingWritten || r.Status == model.SettingWouldWrite || r.Status == model.SettingFailed {
			before[r.Key] = r.Before
			after[r.Key] = r.After
		}
	}
	result := "ok"
	switch {
	case rec.Result.DryRun:
		result = "dry-run"
	case rec.Failed > 0 && rec.Written == 0:
		result = "failed"
	case rec.Failed > 0:
		result = "partial"
	}
	after["writeId"] = rec.ID
	if rec.ProfileID != "" {
		after["profileId"] = rec.ProfileID
	}
	if !rec.Result.DryRun {
		// The read-back verdict per written key (audit A20), so the trail says
		// what is in effect, not only what the device accepted.
		outcomes := map[string]any{}
		for _, r := range rec.Result.Results {
			if r.Outcome != "" {
				outcomes[r.Key] = string(r.Outcome)
			}
		}
		if len(outcomes) > 0 {
			after["outcomes"] = outcomes
		}
		if rec.Diverged > 0 && result == "ok" {
			result = "diverged"
		}
	}
	if rec.RevertOf != "" {
		after["revertOf"] = rec.RevertOf
	}
	_ = s.Store.AddAudit(ctx, &model.AuditEntry{Actor: rec.Actor, Action: JobType, Target: dev.ID, Before: before, After: after, Result: result})
}

func redactResult(r model.SettingResult) model.SettingResult {
	if model.IsSecretKey(r.Key) {
		if r.Before != nil {
			r.Before = model.RedactedValue
		}
		if r.After != nil {
			r.After = model.RedactedValue
		}
	}
	return r
}

func redactChanges(in []model.SettingChange) []model.SettingChange {
	out := make([]model.SettingChange, 0, len(in))
	for _, ch := range in {
		if model.IsSecretKey(ch.Key) {
			ch.Value = model.RedactedValue
		}
		out = append(out, ch)
	}
	return out
}

func changesFromParams(j *model.Job) []model.SettingChange {
	raw, _ := j.Params["changes"].([]any)
	var out []model.SettingChange
	for _, v := range raw {
		m, ok := v.(map[string]any)
		if !ok {
			continue
		}
		k, _ := m["key"].(string)
		if k != "" {
			out = append(out, model.SettingChange{Key: k, Value: m["value"]})
		}
	}
	return out
}

// Revert submits an apply of a write record's rollback values.
func (s *Service) Revert(ctx context.Context, writeID string, dryRun bool, actor string) (*model.Job, error) {
	rec, err := s.Store.GetWriteRecord(ctx, writeID)
	if err != nil {
		return nil, err
	}
	if !rec.Reversible() {
		return nil, errors.New("this write has nothing to revert (dry run, or nothing was written)")
	}
	for _, ch := range rec.Result.Rollback {
		if model.IsSecretKey(ch.Key) {
			return nil, fmt.Errorf("%s: secret values are not stored, revert it manually", ch.Key)
		}
	}
	return s.Apply(ctx, ApplyRequest{Changes: rec.Result.Rollback, Target: model.ConfigTarget{DeviceIDs: []string{rec.Result.DeviceID}}, DryRun: dryRun, RevertOf: rec.ID}, actor)
}
