// Package security implements security policies (spec §8.5): assess,
// remediate, report; certificate enrollment and credential rotation.
//
// Reads come from every driver that serves read_security (falling back to
// read_config restricted to the security keys) plus two vendor-neutral
// network probes (TLS and service ports). Writes go through write_security
// (falling back to write_config), are dry-runnable, audited, and obey the
// management-path rule of spec §11: a remediation never disables the last
// protocol PrintMarshal manages the device through.
package security

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"math/big"
	"strings"
	"sync"
	"time"

	"printmarshal/internal/engine"
	"printmarshal/internal/fleet"
	"printmarshal/internal/jobs"
	"printmarshal/internal/model"
	"printmarshal/internal/store"
)

// Job types registered by the service.
const (
	JobAssess    = "security.assess"
	JobRemediate = "security.remediate"
	JobRotate    = "security.rotate"
	JobEnroll    = "security.enroll"
	JobInstall   = "security.install"
)

// AlertSource is the alert source of everything this package raises.
const AlertSource = "security"

// Target selects the devices a security job acts on: explicit ids or a group.
type Target struct {
	DeviceIDs []string `json:"deviceIds,omitempty"`
	GroupID   string   `json:"groupId,omitempty"`
}

// Options gate a write job.
type Options struct {
	DryRun bool `json:"dryRun"`
}

// Service is the security façade.
type Service struct {
	fleet.Deps
	// TLSProbe and PortProbe are the vendor-neutral network probes; nil
	// disables them (tests).
	TLSProbe  TLSProber
	PortProbe PortProber
	// Now is the clock (tests).
	Now func() time.Time

	mu sync.Mutex
}

// New builds the service, registers its job types and creates the built-in
// Baseline policy when no policy exists yet.
func New(d fleet.Deps) *Service {
	s := &Service{Deps: d, TLSProbe: ProbeTLS, PortProbe: ProbePorts, Now: func() time.Time { return time.Now().UTC() }}
	if d.Jobs != nil {
		d.Jobs.Register(JobAssess, s.runAssess)
		d.Jobs.Register(JobRemediate, s.runRemediate)
		d.Jobs.Register(JobRotate, s.runRotate)
		d.Jobs.Register(JobEnroll, s.runEnroll)
		d.Jobs.Register(JobInstall, s.runInstall)
	}
	if d.Store != nil {
		if err := s.EnsureBaseline(context.Background()); err != nil && d.Log != nil {
			d.Log.Warn("security: baseline policy", "err", err)
		}
	}
	return s
}

// EnsureBaseline creates the built-in Baseline policy if no policies exist.
func (s *Service) EnsureBaseline(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	ps, err := s.Store.ListPolicies(ctx)
	if err != nil {
		return err
	}
	if len(ps) > 0 {
		return nil
	}
	p := Baseline()
	if err := ValidatePolicy(p); err != nil {
		return err
	}
	return s.Store.CreatePolicy(ctx, p)
}

func (s *Service) emit(typ string, data any) {
	if s.Events != nil {
		s.Events(model.Event{Type: typ, At: s.Now(), Data: data})
	}
}

func (s *Service) creds(ctx context.Context, dev *model.Device) model.Creds {
	if s.Fleet != nil {
		return s.Fleet.Creds(ctx, dev)
	}
	groups, _ := s.Store.GroupsForDevice(ctx, dev.ID)
	c, _ := s.Store.ResolveCreds(ctx, dev.ID, groups)
	return c
}

func (s *Service) concurrency() int {
	if s.Cfg != nil && s.Cfg.Poll.Concurrency > 0 {
		return s.Cfg.Poll.Concurrency
	}
	return 8
}

// --- targets --------------------------------------------------------------------

// ResolveTarget lists the devices a target names (static or rule groups expanded).
func (s *Service) ResolveTarget(ctx context.Context, t Target) ([]*model.Device, error) {
	switch {
	case t.GroupID != "":
		g, err := s.Store.GetGroup(ctx, t.GroupID)
		if err != nil {
			return nil, err
		}
		if g.Kind == "rule" {
			all, err := s.Store.ListDevices(ctx, store.DeviceFilter{})
			if err != nil {
				return nil, err
			}
			var out []*model.Device
			for _, d := range all {
				if store.MatchRule(g.Rule, d) {
					out = append(out, d)
				}
			}
			return out, nil
		}
		return s.Store.ListDevices(ctx, store.DeviceFilter{GroupIDs: []string{g.ID}})
	case len(t.DeviceIDs) > 0:
		devs, err := s.Store.ListDevices(ctx, store.DeviceFilter{IDs: t.DeviceIDs, IncludeDecommissioned: true})
		if err != nil {
			return nil, err
		}
		if len(devs) != len(t.DeviceIDs) {
			seen := map[string]bool{}
			for _, d := range devs {
				seen[d.ID] = true
			}
			for _, id := range t.DeviceIDs {
				if !seen[id] {
					return nil, fmt.Errorf("device %s: %w", id, store.ErrNotFound)
				}
			}
		}
		return devs, nil
	}
	return nil, errors.New("target must name deviceIds or a groupId")
}

func (t Target) toMap() map[string]any {
	m := map[string]any{}
	if t.GroupID != "" {
		m["groupId"] = t.GroupID
	}
	if len(t.DeviceIDs) > 0 {
		ids := make([]any, len(t.DeviceIDs))
		for i, id := range t.DeviceIDs {
			ids[i] = id
		}
		m["deviceIds"] = ids
	}
	return m
}

func targetFromJob(j *model.Job) Target {
	var t Target
	if g, ok := j.Target["groupId"].(string); ok {
		t.GroupID = g
	}
	if ids, ok := j.Target["deviceIds"].([]any); ok {
		for _, id := range ids {
			if sid, ok := id.(string); ok {
				t.DeviceIDs = append(t.DeviceIDs, sid)
			}
		}
	}
	return t
}

func jobString(j *model.Job, key string) string {
	if v, ok := j.Params[key].(string); ok {
		return v
	}
	if v, ok := j.Target[key].(string); ok {
		return v
	}
	return ""
}

func jobBool(j *model.Job, key string, def bool) bool {
	if v, ok := j.Params[key].(bool); ok {
		return v
	}
	return def
}

// forEachDevice runs fn for every device of a job's target with bounded
// concurrency and a per-device timeout, recording one job result each.
func (s *Service) forEachDevice(ctx context.Context, jc *jobs.Context, devs []*model.Device, timeout time.Duration, fn func(ctx context.Context, d *model.Device) (status, msg string, data map[string]any)) {
	jc.SetTotal(len(devs))
	var wg sync.WaitGroup
	sem := make(chan struct{}, s.concurrency())
	for _, d := range devs {
		if ctx.Err() != nil {
			break
		}
		wg.Add(1)
		sem <- struct{}{}
		go func(d *model.Device) {
			defer wg.Done()
			defer func() { <-sem }()
			dctx, cancel := context.WithTimeout(ctx, timeout)
			defer cancel()
			status, msg, data := fn(dctx, d)
			jc.Result(d.ID, d.Address, status, msg, data)
		}(d)
	}
	wg.Wait()
}

// --- security document ------------------------------------------------------------

// Gather builds a device's security document: every read_security driver,
// then read_config drivers restricted to the security keys, then the
// certificate managers, then the network probes. Earlier sources win per key;
// probe values only fill keys no driver reported.
func (s *Service) Gather(ctx context.Context, dev *model.Device, creds model.Creds) (*model.ConfigDocument, []TLSReport) {
	now := s.Now()
	doc := &model.ConfigDocument{DeviceID: dev.ID, ReadAt: now, Settings: []model.Setting{}, Sources: []string{}}
	have := map[string]bool{}
	add := func(source string, settings []model.Setting) {
		added := false
		for _, st := range settings {
			if have[st.Key] || !isSecurityKey(st.Key) {
				continue
			}
			if st.Source == "" {
				st.Source = source
			}
			have[st.Key] = true
			doc.Settings = append(doc.Settings, st)
			added = true
		}
		if added {
			doc.Sources = append(doc.Sources, source)
		}
	}
	if s.Engine != nil {
		for _, drv := range s.Engine.Resolve(dev, model.CapReadSecurity) {
			r, ok := drv.(engine.SecurityReader)
			if !ok {
				continue
			}
			d, err := r.ReadSecurity(ctx, dev, creds)
			if err != nil {
				s.warn("read security", dev, drv.Name(), err)
				continue
			}
			add(drv.Name(), d.Settings)
		}
		for _, drv := range s.Engine.Resolve(dev, model.CapReadConfig) {
			r, ok := drv.(engine.ConfigReader)
			if !ok {
				continue
			}
			d, err := r.ReadConfig(ctx, dev, creds, model.ConfigSelector{Keys: model.SecurityKeys()})
			if err != nil {
				s.warn("read config", dev, drv.Name(), err)
				continue
			}
			add(drv.Name(), d.Settings)
		}
		for _, drv := range s.Engine.Resolve(dev, model.CapManageCerts) {
			cm, ok := drv.(engine.CertManager)
			if !ok {
				continue
			}
			certs, err := cm.ListCertificates(ctx, dev, creds)
			if err != nil {
				s.warn("list certificates", dev, drv.Name(), err)
				continue
			}
			_ = s.Store.SaveDeviceCertificates(ctx, dev.ID, drv.Name(), certs, now)
			add(drv.Name(), certSettings(certs, now, drv.Name()))
		}
	}
	var reports []TLSReport
	if s.TLSProbe != nil && dev.Address != "" {
		reports = s.TLSProbe(ctx, dev.Address, DefaultTLSPorts)
		add("probe", TLSSettings(reports, now))
		var leafs []model.Certificate
		for _, r := range reports {
			if r.Leaf != nil {
				leafs = append(leafs, *r.Leaf)
			}
		}
		if len(leafs) > 0 {
			_ = s.Store.SaveDeviceCertificates(ctx, dev.ID, "probe", leafs, now)
		}
	}
	if s.PortProbe != nil && dev.Address != "" {
		add("probe", PortSettings(s.PortProbe(ctx, dev.Address, creds)))
	}
	return doc, reports
}

// certSettings derives the identity-certificate facts from a driver's list.
func certSettings(certs []model.Certificate, now time.Time, source string) []model.Setting {
	var identity *model.Certificate
	for i := range certs {
		if certs[i].Use == "identity" {
			identity = &certs[i]
			break
		}
	}
	if identity == nil {
		return []model.Setting{driverSetting("security.cert.identityPresent", false, source)}
	}
	days := int(identity.NotAfter.Sub(now).Hours() / 24)
	return []model.Setting{
		driverSetting("security.cert.identityPresent", true, source),
		driverSetting("security.cert.identitySelfSigned", identity.SelfSigned, source),
		driverSetting("security.cert.identityExpiresAt", identity.NotAfter.Format(time.RFC3339), source),
		driverSetting("security.cert.identityDaysLeft", days, source),
	}
}

func driverSetting(key string, v any, source string) model.Setting {
	def := model.CatalogByKey[key]
	return model.Setting{Key: key, Area: def.Area, Name: def.Name, Value: v, Type: def.Type, Source: source}
}

var securityKeySet = func() map[string]bool {
	m := map[string]bool{}
	for _, k := range model.SecurityKeys() {
		m[k] = true
	}
	return m
}()

func isSecurityKey(key string) bool {
	return securityKeySet[key] || strings.HasPrefix(key, "vendor.")
}

func (s *Service) warn(what string, dev *model.Device, driver string, err error) {
	if s.Log != nil {
		s.Log.Warn("security: "+what, "device", dev.ID, "driver", driver, "err", err)
	}
}

// --- management paths (spec §11) ----------------------------------------------------

// ManagementPaths returns the protocols PrintMarshal currently manages a
// device through, derived from the drivers in its capability profile:
// "snmp", "ipp", or "http" for any vendor plugin (they talk to the device's
// web service).
func ManagementPaths(dev *model.Device) map[string]bool {
	paths := map[string]bool{}
	if dev == nil {
		return paths
	}
	for _, drv := range dev.Capabilities {
		paths[pathOfDriver(drv)] = true
	}
	if len(paths) == 0 {
		for _, m := range dev.IdentifyMethods {
			paths[pathOfDriver(m)] = true
		}
	}
	return paths
}

func pathOfDriver(name string) string {
	switch strings.ToLower(name) {
	case "snmp", "ipp":
		return strings.ToLower(name)
	}
	return "http"
}

// --- writers ---------------------------------------------------------------------

// settingWriter unifies SecurityWriter and ConfigWriter.
type settingWriter struct {
	name  string
	write func(ctx context.Context, d *model.Device, creds model.Creds, changes []model.SettingChange, opts model.WriteOptions) (*model.WriteResult, error)
}

// writers returns the drivers that can write security settings for a
// device: write_security first, then write_config as a fallback.
func (s *Service) writers(dev *model.Device) []settingWriter {
	var out []settingWriter
	seen := map[string]bool{}
	if s.Engine == nil {
		return out
	}
	for _, drv := range s.Engine.Resolve(dev, model.CapWriteSecurity) {
		if w, ok := drv.(engine.SecurityWriter); ok && !seen[drv.Name()] {
			seen[drv.Name()] = true
			out = append(out, settingWriter{name: drv.Name(), write: w.WriteSecurity})
		}
	}
	for _, drv := range s.Engine.Resolve(dev, model.CapWriteConfig) {
		if w, ok := drv.(engine.ConfigWriter); ok && !seen[drv.Name()] {
			seen[drv.Name()] = true
			out = append(out, settingWriter{name: drv.Name(), write: w.WriteConfig})
		}
	}
	return out
}

// applyChanges writes changes through the writers in order; settings a
// writer skips are offered to the next one. Results are keyed by setting
// and redacted for secret keys.
func (s *Service) applyChanges(ctx context.Context, dev *model.Device, creds model.Creds, changes []model.SettingChange, opts model.WriteOptions) (map[string]model.SettingResult, error) {
	results := map[string]model.SettingResult{}
	pending := changes
	ws := s.writers(dev)
	if len(ws) == 0 {
		return results, engine.ErrUnsupported
	}
	var lastErr error
	for _, w := range ws {
		if len(pending) == 0 {
			break
		}
		res, err := w.write(ctx, dev, creds, pending, opts)
		if err != nil {
			lastErr = fmt.Errorf("%s: %w", w.name, err)
			s.warn("write security", dev, w.name, err)
			continue
		}
		var next []model.SettingChange
		got := map[string]model.SettingResult{}
		for _, r := range res.Results {
			r.Driver = w.name
			got[r.Key] = r
		}
		for _, c := range pending {
			r, ok := got[c.Key]
			if !ok || r.Status == model.SettingSkipped {
				next = append(next, c)
				continue
			}
			results[c.Key] = redactResult(r)
		}
		pending = next
	}
	for _, c := range pending {
		if _, ok := results[c.Key]; !ok {
			results[c.Key] = model.SettingResult{Key: c.Key, Status: model.SettingSkipped, Error: "no driver accepted this setting"}
		}
	}
	if len(results) == 0 && lastErr != nil {
		return results, lastErr
	}
	return results, nil
}

// redactResult strips secret values from a setting result (spec §11).
func redactResult(r model.SettingResult) model.SettingResult {
	if IsSecretKey(r.Key) {
		if r.Before != nil {
			r.Before = "<redacted>"
		}
		if r.After != nil {
			r.After = "<redacted>"
		}
	}
	return r
}

// redactChanges returns a key -> value map safe for audit entries.
func redactChanges(changes []model.SettingChange) map[string]any {
	m := map[string]any{}
	for _, c := range changes {
		if IsSecretKey(c.Key) {
			m[c.Key] = "<redacted>"
		} else {
			m[c.Key] = c.Value
		}
	}
	return m
}

// --- alerts ------------------------------------------------------------------------

func (s *Service) raiseAlert(ctx context.Context, dev *model.Device, key string, sev model.Severity, desc string) {
	a, raised, err := s.Store.RaiseAlert(ctx, dev.ID, key, AlertSource, sev, desc)
	if err != nil {
		s.warn("raise alert", dev, "", err)
		return
	}
	if raised {
		s.emit("alert.raised", a)
		if s.Fleet != nil && s.Fleet.Notifier != nil {
			s.Fleet.Notifier.Notify("alert.raised", fmt.Sprintf("[%s] %s: %s", strings.ToUpper(string(sev)), dev.Name(), desc),
				fmt.Sprintf("%s (%s, %s)\n%s", dev.Name(), dev.Model, dev.Address, desc), map[string]any{"alert": a, "device": dev})
		}
	}
}

// clearAlerts clears the device's active security alerts whose key matches
// prefix and is not in keep.
func (s *Service) clearAlerts(ctx context.Context, dev *model.Device, prefix string, keep map[string]bool) {
	active, err := s.Store.ListAlerts(ctx, store.AlertFilter{DeviceID: dev.ID, ActiveOnly: true})
	if err != nil {
		return
	}
	for _, a := range active {
		if a.Source != AlertSource || !strings.HasPrefix(a.Key, prefix) || keep[a.Key] {
			continue
		}
		if cleared, err := s.Store.ClearAlert(ctx, a.ID); err == nil {
			s.emit("alert.cleared", cleared)
		}
	}
}

// --- misc ------------------------------------------------------------------------

const secretAlphabet = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789"

// randomSecret returns n characters of [A-Za-z0-9] from crypto/rand.
func randomSecret(n int) (string, error) {
	b := make([]byte, n)
	max := big.NewInt(int64(len(secretAlphabet)))
	for i := range b {
		k, err := rand.Int(rand.Reader, max)
		if err != nil {
			return "", err
		}
		b[i] = secretAlphabet[k.Int64()]
	}
	return string(b), nil
}
