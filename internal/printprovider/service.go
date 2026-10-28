package printprovider

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sort"
	"strings"
	"sync"
	"time"

	"printmarshal/internal/fleet"
	"printmarshal/internal/jobs"
	"printmarshal/internal/model"
	"printmarshal/internal/store"
)

// Errors the API maps to 4xx responses.
var (
	ErrDisabled = errors.New("print provider is disabled")
	ErrListMode = errors.New("print provider is in ipp-everywhere-list mode: it publishes URIs and does not create queues")
	ErrNoQueue  = errors.New("device has no queue on this host")
)

// Action is what the planner wants to do with one queue.
type Action string

// Plan actions.
const (
	ActionCreate    Action = "create"
	ActionUpdate    Action = "update"
	ActionRemove    Action = "remove"
	ActionUnchanged Action = "unchanged"
	ActionSkip      Action = "skip"
	// ActionList is used in ipp-everywhere-list mode: the URI is published,
	// nothing is created.
	ActionList Action = "list"
)

// Entry is one row of a plan: a device and the queue it maps to.
type Entry struct {
	DeviceID   string   `json:"deviceId,omitempty"`
	DeviceName string   `json:"deviceName,omitempty"`
	Address    string   `json:"address,omitempty"`
	QueueName  string   `json:"queueName"`
	URI        string   `json:"uri,omitempty"`
	Action     Action   `json:"action"`
	Reason     string   `json:"reason,omitempty"`
	Current    *Queue   `json:"current,omitempty"`
	Commands   []string `json:"commands,omitempty"`
	spec       *QueueSpec
}

// Plan is the diff between the devices PrintMarshal knows and this host's queues.
type Plan struct {
	Enabled bool   `json:"enabled"`
	Mode    string `json:"mode"`
	Backend string `json:"backend"`
	// InventoryRead is false when existing queues were not inspected (the
	// module is disabled or in list mode), so "create" may mean "already there".
	InventoryRead bool           `json:"inventoryRead"`
	Entries       []Entry        `json:"entries"`
	Summary       map[string]int `json:"summary"`
	GeneratedAt   time.Time      `json:"generatedAt"`
}

// Service is the print-provider module.
type Service struct {
	fleet.Deps

	mu      sync.Mutex
	backend Backend
	now     func() time.Time
}

// New builds the service from the dependency bundle, choosing the real CUPS
// backend on a Linux host that has it and the recording dry backend otherwise,
// and registers the sync job type.
func New(d fleet.Deps) *Service {
	return NewWithBackend(d, detectBackend())
}

// NewWithBackend is New with an explicit backend (tests, previews).
func NewWithBackend(d fleet.Deps, b Backend) *Service {
	s := &Service{Deps: d, backend: b, now: func() time.Time { return time.Now().UTC() }}
	if s.Log == nil {
		s.Log = slog.Default()
	}
	if d.Jobs != nil {
		d.Jobs.Register(JobType, s.runSync)
	}
	return s
}

// Backend exposes the backend in use (tests).
func (s *Service) Backend() Backend { return s.backend }

// Availability reports whether real queues can be created on this host.
func (s *Service) Availability() Availability { return s.backend.Availability() }

// Settings loads the persisted settings, or the defaults when none are saved.
func (s *Service) Settings(ctx context.Context) (Settings, error) {
	st := DefaultSettings()
	if s.Store == nil {
		return st, nil
	}
	err := s.Store.GetSetting(ctx, SettingsKey, &st)
	if errors.Is(err, store.ErrNotFound) {
		return DefaultSettings(), nil
	}
	if err != nil {
		return st, err
	}
	if verr := st.Validate(); verr != nil {
		// A stored value that no longer validates must not brick the module.
		s.Log.Warn("printprovider: stored settings invalid, using defaults", "err", verr)
		return DefaultSettings(), nil
	}
	return st, nil
}

// SaveSettings validates and persists new settings.
func (s *Service) SaveSettings(ctx context.Context, st Settings) (Settings, error) {
	if err := st.Validate(); err != nil {
		return st, err
	}
	if err := s.Store.SetSetting(ctx, SettingsKey, st); err != nil {
		return st, err
	}
	return st, nil
}

// Inventory lists this host's queues. It returns nothing — and never calls the
// backend — while the module is disabled or in list mode.
func (s *Service) Inventory(ctx context.Context) ([]Queue, error) {
	st, err := s.Settings(ctx)
	if err != nil {
		return nil, err
	}
	if !st.Enabled || st.Mode != ModeCUPS {
		return []Queue{}, nil
	}
	qs, err := s.backend.ListQueues(ctx)
	if err != nil {
		return nil, err
	}
	if qs == nil {
		qs = []Queue{}
	}
	cands, err := s.candidates(ctx, st)
	if err != nil {
		return nil, err
	}
	byQueue := map[string]string{}
	for _, e := range cands {
		if e.QueueName != "" {
			byQueue[strings.ToLower(e.QueueName)] = e.DeviceID
		}
	}
	for i := range qs {
		qs[i].Managed = hasPrefixFold(qs[i].Name, st.QueuePrefix)
		qs[i].DeviceID = byQueue[strings.ToLower(qs[i].Name)]
	}
	return qs, nil
}

// Plan computes which devices get which queues and diffs that against the
// host's inventory. While disabled it still derives names and URIs (so the
// UI can show what enabling would do) but never touches the backend.
func (s *Service) Plan(ctx context.Context) (*Plan, error) {
	st, err := s.Settings(ctx)
	if err != nil {
		return nil, err
	}
	return s.plan(ctx, st)
}

// candidates derives, without touching the backend, the queue each device
// would get: name, URI and spec, with skips for ineligible devices.
func (s *Service) candidates(ctx context.Context, st Settings) ([]Entry, error) {
	f := store.DeviceFilter{Sort: "name"}
	if st.AutoCreateForGroupID != "" {
		f.GroupIDs = []string{st.AutoCreateForGroupID}
	}
	devs, err := s.Store.ListDevices(ctx, f)
	if err != nil {
		return nil, err
	}
	sort.SliceStable(devs, func(i, j int) bool {
		if a, b := strings.ToLower(devs[i].Name()), strings.ToLower(devs[j].Name()); a != b {
			return a < b
		}
		// Same name: the device known longest keeps the base name.
		if !devs[i].CreatedAt.Equal(devs[j].CreatedAt) {
			return devs[i].CreatedAt.Before(devs[j].CreatedAt)
		}
		return devs[i].ID < devs[j].ID
	})
	opts, _ := ParseOptions(st.DefaultOptions)
	taken := map[string]bool{}
	entries := make([]Entry, 0, len(devs))
	for _, d := range devs {
		e := Entry{DeviceID: d.ID, DeviceName: d.Name(), Address: d.Address}
		if d.State == model.StateDecommissioned {
			e.Action, e.Reason = ActionSkip, "decommissioned"
			entries = append(entries, e)
			continue
		}
		if !HasIPP(d) {
			e.Action, e.Reason = ActionSkip, "no IPP capability (driverless queues need IPP)"
			entries = append(entries, e)
			continue
		}
		base := QueueName(st.QueuePrefix, d)
		name := base
		for n := 2; taken[strings.ToLower(name)]; n++ {
			name = fmt.Sprintf("%s-%d", base, n)
		}
		taken[strings.ToLower(name)] = true
		e.QueueName = name
		e.URI = DeviceURI(d)
		e.spec = &QueueSpec{Name: name, URI: e.URI, Description: description(d), Location: d.Location, Driverless: st.Driverless, Options: opts}
		if !AdvertisesDNSSD(d) {
			e.Reason = "not seen via DNS-SD; the printer may still accept IPP Everywhere"
		}
		if st.Mode == ModeList {
			e.Action = ActionList
		} else {
			e.Action = ActionCreate
			e.Commands = RenderCommands(CreateCommands(*e.spec))
		}
		entries = append(entries, e)
	}
	return entries, nil
}

func (s *Service) plan(ctx context.Context, st Settings) (*Plan, error) {
	p := &Plan{Enabled: st.Enabled, Mode: st.Mode, Backend: s.backend.Availability().Backend, Summary: map[string]int{}, GeneratedAt: s.now()}
	entries, err := s.candidates(ctx, st)
	if err != nil {
		return nil, err
	}
	p.Entries = entries

	if st.Enabled && st.Mode == ModeCUPS {
		existing, err := s.backend.ListQueues(ctx)
		if err != nil {
			return nil, fmt.Errorf("list queues: %w", err)
		}
		p.InventoryRead = true
		byName := map[string]Queue{}
		for _, q := range existing {
			q.Managed = hasPrefixFold(q.Name, st.QueuePrefix)
			byName[strings.ToLower(q.Name)] = q
		}
		claimed := map[string]bool{}
		for i := range p.Entries {
			e := &p.Entries[i]
			if e.spec == nil {
				continue
			}
			cur, ok := byName[strings.ToLower(e.QueueName)]
			if !ok {
				continue
			}
			claimed[strings.ToLower(cur.Name)] = true
			cur.DeviceID = e.DeviceID
			c := cur
			e.Current = &c
			if cur.URI == e.spec.URI && cur.Description == e.spec.Description && cur.Location == e.spec.Location {
				e.Action, e.Commands = ActionUnchanged, nil
			} else {
				e.Action = ActionUpdate
			}
		}
		for _, q := range existing {
			lk := strings.ToLower(q.Name)
			if claimed[lk] || !hasPrefixFold(q.Name, st.QueuePrefix) {
				continue
			}
			c := byName[lk]
			p.Entries = append(p.Entries, Entry{QueueName: q.Name, URI: q.URI, Action: ActionRemove, Reason: "no matching device", Current: &c, Commands: RenderCommands(RemoveCommands(q.Name))})
		}
	}
	for _, e := range p.Entries {
		p.Summary[string(e.Action)]++
	}
	return p, nil
}

// description is the queue's human-readable name: the device name plus model.
func description(d *model.Device) string {
	n := d.Name()
	if d.Model != "" && !strings.Contains(strings.ToLower(n), strings.ToLower(d.Model)) {
		return n + " (" + d.Model + ")"
	}
	return n
}

func hasPrefixFold(name, prefix string) bool {
	return len(name) >= len(prefix) && strings.EqualFold(name[:len(prefix)], prefix)
}

// Sync queues a synchronisation job. dryRun records what would change without
// touching the host. It refuses while disabled or in list mode.
func (s *Service) Sync(ctx context.Context, actor string, dryRun bool) (*model.Job, error) {
	st, err := s.Settings(ctx)
	if err != nil {
		return nil, err
	}
	if !st.Enabled {
		return nil, ErrDisabled
	}
	if st.Mode != ModeCUPS {
		return nil, ErrListMode
	}
	target := map[string]any{"all": st.AutoCreateForGroupID == ""}
	if st.AutoCreateForGroupID != "" {
		target["groupId"] = st.AutoCreateForGroupID
	}
	return s.Jobs.Submit(ctx, &model.Job{Type: JobType, CreatedBy: actor, Target: target, Params: map[string]any{"dryRun": dryRun}})
}

// runSync is the job handler: one result per plan entry, one audit entry per
// real queue change.
func (s *Service) runSync(ctx context.Context, jc *jobs.Context) error {
	dry, _ := jc.Job.Params["dryRun"].(bool)
	st, err := s.Settings(ctx)
	if err != nil {
		return err
	}
	if !st.Enabled {
		return ErrDisabled
	}
	if st.Mode != ModeCUPS {
		return ErrListMode
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	p, err := s.plan(ctx, st)
	if err != nil {
		return err
	}
	jc.SetTotal(len(p.Entries))
	for _, e := range p.Entries {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		data := map[string]any{"action": e.Action, "queue": e.QueueName, "uri": e.URI, "dryRun": dry}
		if len(e.Commands) > 0 {
			data["commands"] = e.Commands
		}
		switch e.Action {
		case ActionCreate, ActionUpdate:
			if dry {
				jc.Result(e.DeviceID, e.Address, "ok", fmt.Sprintf("would %s queue %s", e.Action, e.QueueName), data)
				continue
			}
			err := s.backend.CreateOrUpdate(ctx, *e.spec)
			s.auditQueue(ctx, jc.Job.CreatedBy, "printprovider.queue."+string(e.Action), e, err)
			if err != nil {
				jc.Result(e.DeviceID, e.Address, "error", err.Error(), data)
				continue
			}
			jc.Result(e.DeviceID, e.Address, "ok", fmt.Sprintf("%sd queue %s", e.Action, e.QueueName), data)
		case ActionRemove:
			if dry {
				jc.Result("", "", "ok", "would remove queue "+e.QueueName, data)
				continue
			}
			err := s.backend.Remove(ctx, e.QueueName)
			s.auditQueue(ctx, jc.Job.CreatedBy, "printprovider.queue.remove", e, err)
			if err != nil {
				jc.Result("", "", "error", err.Error(), data)
				continue
			}
			jc.Result("", "", "ok", "removed queue "+e.QueueName, data)
		case ActionUnchanged:
			jc.Result(e.DeviceID, e.Address, "skipped", "queue "+e.QueueName+" is up to date", data)
		default:
			jc.Result(e.DeviceID, e.Address, "skipped", e.Reason, data)
		}
	}
	return nil
}

func (s *Service) auditQueue(ctx context.Context, actor, action string, e Entry, err error) {
	target := e.DeviceID
	if target == "" {
		target = "queue:" + e.QueueName
	}
	var before map[string]any
	if e.Current != nil {
		before = map[string]any{"queue": e.Current.Name, "uri": e.Current.URI}
	}
	after := map[string]any{"queue": e.QueueName, "uri": e.URI}
	if action == "printprovider.queue.remove" {
		after = nil
	}
	result := "ok"
	if err != nil {
		result = "error: " + err.Error()
	}
	_ = s.Store.AddAudit(ctx, &model.AuditEntry{Actor: actor, Action: action, Target: target, Before: before, After: after, Result: result})
}

// Remove deletes the queue serving one device, if this host has one.
func (s *Service) Remove(ctx context.Context, actor, deviceID string) (string, error) {
	st, err := s.Settings(ctx)
	if err != nil {
		return "", err
	}
	if !st.Enabled {
		return "", ErrDisabled
	}
	if st.Mode != ModeCUPS {
		return "", ErrListMode
	}
	if _, err := s.Store.GetDevice(ctx, deviceID); err != nil {
		return "", err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	p, err := s.plan(ctx, st)
	if err != nil {
		return "", err
	}
	for _, e := range p.Entries {
		if e.DeviceID != deviceID || e.Current == nil {
			continue
		}
		rerr := s.backend.Remove(ctx, e.Current.Name)
		e.QueueName = e.Current.Name
		s.auditQueue(ctx, actor, "printprovider.queue.remove", e, rerr)
		if rerr != nil {
			return "", rerr
		}
		return e.Current.Name, nil
	}
	return "", ErrNoQueue
}
