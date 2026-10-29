// Package firmware implements firmware inventory and staged update campaigns
// (spec §8.6) with safeguards: explicit start and canary confirmation gates,
// dry runs that never deliver, compatibility and pre-flight checks before
// every delivery, verification by re-reading the device afterwards, failure
// thresholds, maintenance windows and an audit entry per real delivery.
//
// Delivery itself is a capability (firmware_update) served by drivers: the
// standard simulator driver (internal/driver/fwsim) or a vendor plugin. The
// service never knows how an image reaches a device.
package firmware

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"path/filepath"
	"sync"
	"time"

	"printmarshal/internal/engine"
	"printmarshal/internal/fleet"
	"printmarshal/internal/model"
	"printmarshal/internal/store"
)

// JobType is the job that executes a campaign.
const JobType = "firmware.campaign"

// Audit actions written by the service and its API.
const (
	AuditUpdate = "firmware.update" // one real delivery to one device
)

// Event types published on the live stream.
const (
	EventCampaignUpdated = "firmware.campaign.updated"
	EventDeviceUpdated   = "firmware.device.updated"
)

var (
	// ErrInvalid marks a request the caller can fix (HTTP 400).
	ErrInvalid = errors.New("invalid request")
	// ErrConflict marks an operation the campaign's state does not allow (HTTP 409).
	ErrConflict = errors.New("conflict")
)

func invalidf(format string, args ...any) error {
	return fmt.Errorf("%w: %s", ErrInvalid, fmt.Sprintf(format, args...))
}

func conflictf(format string, args ...any) error {
	return fmt.Errorf("%w: %s", ErrConflict, fmt.Sprintf(format, args...))
}

// Poller re-reads a device and returns its refreshed record.
type Poller func(ctx context.Context, dev *model.Device) (*model.Device, error)

// Service is the firmware façade.
type Service struct {
	fleet.Deps

	// Clock is the time source for schedules and maintenance windows.
	Clock func() time.Time
	// VerifyTimeout bounds post-update verification per device when the
	// campaign sets none (default 15 minutes).
	VerifyTimeout time.Duration
	// VerifyInterval is the pause between verification polls (default 20s).
	VerifyInterval time.Duration
	// ControlPoll is how often a waiting job re-reads its campaign while
	// scheduled, paused or outside the maintenance window (default 1 minute).
	ControlPoll time.Duration
	// RetryDelay is the pause between pre-flight poll attempts (default 2s).
	RetryDelay time.Duration
	// Poll re-reads a device (defaults to Fleet.PollDevice).
	Poll Poller
	// Creds resolves a device's read credentials, used by dry runs (defaults to Fleet.Creds).
	Creds func(ctx context.Context, dev *model.Device) model.Creds
	// WriteCreds resolves the write credentials a real delivery uses (audit
	// A19); it returns store.ErrNoWriteCredential when none is bound, and the
	// device then fails instead of being written to with a read credential
	// (defaults to Fleet.CredsFor with PurposeWrite, else the store).
	WriteCreds func(ctx context.Context, dev *model.Device) (model.Creds, error)
	// DataDir holds package files under <DataDir>/firmware/<id>/ (defaults to Cfg.DataDir).
	DataDir string

	mu     sync.Mutex
	wakes  map[string]chan struct{}
	active map[string]bool
	// lifecycle serialises Start/Confirm/Pause/Resume/Cancel so two
	// overlapping requests cannot both read "draft" and each queue a worker
	// (code review H-008). A campaign row also records the id of the one job
	// allowed to run it (JobID); a worker whose job is not that id exits.
	lifecycle sync.Mutex
}

// New builds the service, registers its job type and recovers campaigns a
// previous process left running (they pause until an operator resumes them).
func New(d fleet.Deps) *Service {
	s := &Service{Deps: d, Clock: func() time.Time { return time.Now().UTC() },
		VerifyTimeout: 15 * time.Minute, VerifyInterval: 20 * time.Second, ControlPoll: time.Minute, RetryDelay: 2 * time.Second,
		wakes: map[string]chan struct{}{}, active: map[string]bool{}}
	if d.Log == nil {
		s.Log = slog.Default()
	}
	if d.Cfg != nil {
		s.DataDir = d.Cfg.DataDir
	}
	if d.Fleet != nil {
		s.Poll = func(ctx context.Context, dev *model.Device) (*model.Device, error) {
			upd, _, err := d.Fleet.PollDevice(ctx, dev)
			return upd, err
		}
		s.Creds = d.Fleet.Creds
		s.WriteCreds = func(ctx context.Context, dev *model.Device) (model.Creds, error) {
			return d.Fleet.CredsFor(ctx, dev, model.PurposeWrite)
		}
	}
	if s.Creds == nil {
		s.Creds = func(context.Context, *model.Device) model.Creds { return model.Creds{} }
	}
	if s.WriteCreds == nil {
		s.WriteCreds = func(ctx context.Context, dev *model.Device) (model.Creds, error) {
			if d.Store == nil {
				return model.Creds{}, store.ErrNoWriteCredential
			}
			return store.CredsFor(ctx, d.Store, dev, model.PurposeWrite)
		}
	}
	if d.Jobs != nil {
		d.Jobs.Register(JobType, s.runCampaign)
	}
	if d.Store != nil {
		s.recoverInterrupted(context.Background())
	}
	return s
}

// Dir is the package storage directory.
func (s *Service) Dir() string { return filepath.Join(s.DataDir, "firmware") }

func (s *Service) emit(typ string, data any) {
	if s.Events != nil {
		s.Events(model.Event{Type: typ, At: time.Now().UTC(), Data: data})
	}
}

func (s *Service) now() time.Time {
	if s.Clock != nil {
		return s.Clock()
	}
	return time.Now().UTC()
}

// updater resolves the best firmware_update driver for a device.
func (s *Service) updater(dev *model.Device) (engine.FirmwareUpdater, string, error) {
	if s.Engine == nil {
		return nil, "", errors.New("no engine")
	}
	for _, drv := range s.Engine.Resolve(dev, model.CapFirmwareUpdate) {
		if u, ok := drv.(engine.FirmwareUpdater); ok {
			return u, drv.Name(), nil
		}
	}
	return nil, "", errors.New("no registered driver can deliver firmware to this device (capability firmware_update)")
}

// --- job control -------------------------------------------------------------

func (s *Service) wakeCh(id string) chan struct{} {
	s.mu.Lock()
	defer s.mu.Unlock()
	ch, ok := s.wakes[id]
	if !ok {
		ch = make(chan struct{}, 1)
		s.wakes[id] = ch
	}
	return ch
}

// wake nudges the job running campaign id, if any, to re-read its state.
func (s *Service) wake(id string) {
	select {
	case s.wakeCh(id) <- struct{}{}:
	default:
	}
}

func (s *Service) setActive(id string, on bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if on {
		s.active[id] = true
		return
	}
	delete(s.active, id)
	delete(s.wakes, id)
}

func (s *Service) isActive(id string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.active[id]
}

// wait blocks until d elapses, the campaign is woken, or ctx ends.
func (s *Service) wait(ctx context.Context, id string, d time.Duration) error {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-s.wakeCh(id):
	case <-t.C:
	}
	return nil
}

// recoverInterrupted pauses campaigns a previous process left running so an
// operator decides whether to resume; devices caught mid-update are failed
// because their state is unknown (spec §8.6: an interrupted update can
// disable a device).
func (s *Service) recoverInterrupted(ctx context.Context) {
	cs, err := s.Store.ListCampaigns(ctx, store.CampaignFilter{Active: true})
	if err != nil {
		return
	}
	for _, c := range cs {
		if c.Status != model.CampaignRunning && c.Status != model.CampaignScheduled {
			continue
		}
		devs, _ := s.Store.ListCampaignDevices(ctx, c.ID)
		for _, d := range devs {
			if d.Status == model.CampaignDeviceUpdating || d.Status == model.CampaignDeviceVerifying {
				d.Message = "interrupted by restart while " + string(d.Status) + ": verify the device manually before retrying"
				d.Status = model.CampaignDeviceFailed
				fin := time.Now().UTC()
				d.FinishedAt = &fin
				_ = s.Store.UpsertCampaignDevice(ctx, d)
			}
		}
		c.Status, c.PauseReason = model.CampaignPaused, model.PauseInterrupted
		c.Error = "interrupted by server restart; review device states and resume or cancel"
		_ = s.Store.UpdateCampaign(ctx, c)
		s.Log.Warn("firmware campaign interrupted by restart", "campaign", c.ID, "name", c.Name)
	}
}
