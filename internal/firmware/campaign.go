package firmware

import (
	"context"
	"errors"
	"sort"
	"strings"
	"time"

	"printmarshal/internal/firmware/fwversion"
	"printmarshal/internal/model"
	"printmarshal/internal/store"
)

// Defaults applied when a campaign leaves a field unset.
const (
	DefaultCanaryCount = 3 // or 10% of the eligible devices when that is larger
	DefaultConcurrency = 2
	// CanaryDefault as a CanaryCount value means "resolve the default at start".
	CanaryDefault = -1
)

// Summary counts a campaign's devices by status.
type Summary struct {
	Total     int `json:"total"`
	Pending   int `json:"pending"`
	Skipped   int `json:"skipped"`
	Updating  int `json:"updating"`
	Verifying int `json:"verifying"`
	Succeeded int `json:"succeeded"`
	Failed    int `json:"failed"`
}

// Summarize counts devices by status.
func Summarize(devs []*model.CampaignDevice) Summary {
	var s Summary
	for _, d := range devs {
		s.Total++
		switch d.Status {
		case model.CampaignDevicePending:
			s.Pending++
		case model.CampaignDeviceSkipped:
			s.Skipped++
		case model.CampaignDeviceUpdating:
			s.Updating++
		case model.CampaignDeviceVerifying:
			s.Verifying++
		case model.CampaignDeviceSucceeded:
			s.Succeeded++
		case model.CampaignDeviceFailed:
			s.Failed++
		}
	}
	return s
}

// View is a campaign with its device summary and recovery guidance, as the API returns it.
type View struct {
	*model.Campaign
	Summary       Summary `json:"summary"`
	RecoveryNotes string  `json:"recoveryNotes"`
	// PackageVersion is denormalised for lists.
	PackageVersion string `json:"packageVersion,omitempty"`
}

// View builds the API view of a campaign.
func (s *Service) View(ctx context.Context, c *model.Campaign) (*View, error) {
	devs, err := s.Store.ListCampaignDevices(ctx, c.ID)
	if err != nil {
		return nil, err
	}
	v := &View{Campaign: c, Summary: Summarize(devs)}
	if p, err := s.Store.GetFirmwarePackage(ctx, c.PackageID); err == nil {
		v.PackageVersion = p.Version
		v.RecoveryNotes = RecoveryNotes(p.Vendor)
	} else {
		v.RecoveryNotes = RecoveryNotes("")
	}
	return v, nil
}

func (s *Service) validate(ctx context.Context, c *model.Campaign) error {
	c.Name = strings.TrimSpace(c.Name)
	if c.Name == "" {
		return invalidf("name is required")
	}
	if c.PackageID == "" {
		return invalidf("packageId is required")
	}
	if _, err := s.Store.GetFirmwarePackage(ctx, c.PackageID); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return invalidf("package %s does not exist", c.PackageID)
		}
		return err
	}
	if c.GroupID == "" && len(c.DeviceIDs) == 0 {
		return invalidf("a target is required: groupId or deviceIds")
	}
	if c.GroupID != "" {
		if _, err := s.Store.GetGroup(ctx, c.GroupID); err != nil {
			if errors.Is(err, store.ErrNotFound) {
				return invalidf("group %s does not exist", c.GroupID)
			}
			return err
		}
	}
	if c.Concurrency <= 0 {
		c.Concurrency = DefaultConcurrency
	}
	if c.CanaryCount < 0 {
		c.CanaryCount = CanaryDefault
	}
	if c.FailureThresholdPercent < 0 || c.FailureThresholdPercent > 100 {
		return invalidf("failureThresholdPercent must be 0..100")
	}
	if c.VerifyTimeoutSecs < 0 {
		return invalidf("verifyTimeoutSecs must be >= 0")
	}
	if c.DeviceIDs == nil {
		c.DeviceIDs = []string{}
	}
	return ValidateWindow(c.Window)
}

// CreateCampaign validates and stores a draft. Nothing starts here.
func (s *Service) CreateCampaign(ctx context.Context, c *model.Campaign, by string) error {
	if err := s.validate(ctx, c); err != nil {
		return err
	}
	c.Status, c.Wave, c.Confirmed, c.CreatedBy = model.CampaignDraft, 0, false, by
	c.StartedAt, c.FinishedAt, c.JobID, c.Error, c.PauseReason = nil, nil, "", "", ""
	if err := s.Store.CreateCampaign(ctx, c); err != nil {
		return err
	}
	s.emit(EventCampaignUpdated, c)
	return nil
}

// UpdateCampaign changes a draft's definition. Started campaigns are immutable.
func (s *Service) UpdateCampaign(ctx context.Context, c *model.Campaign) error {
	cur, err := s.Store.GetCampaign(ctx, c.ID)
	if err != nil {
		return err
	}
	if cur.Status != model.CampaignDraft {
		return conflictf("only draft campaigns can be edited (status is %s)", cur.Status)
	}
	if err := s.validate(ctx, c); err != nil {
		return err
	}
	c.Status, c.Wave, c.Confirmed, c.CreatedBy, c.CreatedAt = cur.Status, cur.Wave, cur.Confirmed, cur.CreatedBy, cur.CreatedAt
	c.StartedAt, c.FinishedAt, c.JobID, c.Error, c.PauseReason, c.StartedBy = nil, nil, "", "", "", ""
	if err := s.Store.UpdateCampaign(ctx, c); err != nil {
		return err
	}
	s.emit(EventCampaignUpdated, c)
	return nil
}

// GetCampaign returns one campaign.
func (s *Service) GetCampaign(ctx context.Context, id string) (*model.Campaign, error) {
	return s.Store.GetCampaign(ctx, id)
}

// ListCampaigns lists campaigns, newest first.
func (s *Service) ListCampaigns(ctx context.Context, f store.CampaignFilter) ([]*model.Campaign, error) {
	return s.Store.ListCampaigns(ctx, f)
}

// Devices returns the per-device state of a campaign.
func (s *Service) Devices(ctx context.Context, id string) ([]*model.CampaignDevice, error) {
	if _, err := s.Store.GetCampaign(ctx, id); err != nil {
		return nil, err
	}
	return s.Store.ListCampaignDevices(ctx, id)
}

// DeleteCampaign removes a draft or finished campaign and its device rows.
func (s *Service) DeleteCampaign(ctx context.Context, id string) error {
	c, err := s.Store.GetCampaign(ctx, id)
	if err != nil {
		return err
	}
	if c.Status != model.CampaignDraft && !c.Finished() {
		return conflictf("campaign is %s; cancel it before deleting", c.Status)
	}
	return s.Store.DeleteCampaign(ctx, id)
}

// Targets resolves the devices a campaign addresses (group members, rule
// matches or the explicit list), in a stable order.
func (s *Service) Targets(ctx context.Context, c *model.Campaign) ([]*model.Device, error) {
	var devs []*model.Device
	var err error
	switch {
	case c.GroupID != "":
		g, gerr := s.Store.GetGroup(ctx, c.GroupID)
		if gerr != nil {
			return nil, gerr
		}
		if g.Kind == "rule" {
			all, lerr := s.Store.ListDevices(ctx, store.DeviceFilter{})
			if lerr != nil {
				return nil, lerr
			}
			for _, d := range all {
				if store.MatchRule(g.Rule, d) {
					devs = append(devs, d)
				}
			}
		} else {
			devs, err = s.Store.ListDevices(ctx, store.DeviceFilter{GroupIDs: []string{c.GroupID}})
		}
	default:
		devs, err = s.Store.ListDevices(ctx, store.DeviceFilter{IDs: c.DeviceIDs, IncludeDecommissioned: true})
	}
	if err != nil {
		return nil, err
	}
	out := devs[:0]
	for _, d := range devs {
		if d.State != model.StateDecommissioned {
			out = append(out, d)
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		if a, b := strings.ToLower(out[i].Name()), strings.ToLower(out[j].Name()); a != b {
			return a < b
		}
		return out[i].ID < out[j].ID
	})
	return out, nil
}

// Plan computes the per-device rows for a campaign: compatibility verdicts and
// wave assignment. It does not write anything; Start persists the result.
func (s *Service) Plan(ctx context.Context, c *model.Campaign) ([]*model.CampaignDevice, int, error) {
	pkg, err := s.Store.GetFirmwarePackage(ctx, c.PackageID)
	if err != nil {
		return nil, 0, err
	}
	targets, err := s.Targets(ctx, c)
	if err != nil {
		return nil, 0, err
	}
	var rows []*model.CampaignDevice
	var eligible []*model.CampaignDevice
	for _, d := range targets {
		row := &model.CampaignDevice{CampaignID: c.ID, DeviceID: d.ID, FromVersion: d.Firmware, ToVersion: pkg.Version, Status: model.CampaignDevicePending}
		v := fwversion.Check(pkg, d, c.AllowDowngrade)
		switch {
		case v.Current:
			row.Status, row.Message = model.CampaignDeviceSkipped, v.Reason
		case !v.Compatible:
			row.Status, row.Message = model.CampaignDeviceSkipped, "incompatible: "+v.Reason
		default:
			eligible = append(eligible, row)
		}
		rows = append(rows, row)
	}
	canary := c.CanaryCount
	if canary < 0 {
		canary = DefaultCanaryCount
		if pct := (len(eligible) + 9) / 10; pct > canary {
			canary = pct
		}
	}
	for i, row := range eligible {
		row.Wave = 2
		if canary == 0 || i < canary {
			row.Wave = 1
		}
	}
	return rows, canary, nil
}

// StartOptions gate the initial start.
type StartOptions struct {
	// DryRun validates, pre-flights and reports; no driver delivers anything.
	DryRun bool
}

// Start is the explicit confirmation gate for wave 1 (spec §8.6). It plans the
// campaign, persists the device rows, verifies the package integrity and
// submits the job. Only drafts can start.
func (s *Service) Start(ctx context.Context, id, by string, opts StartOptions) (*model.Campaign, error) {
	c, err := s.Store.GetCampaign(ctx, id)
	if err != nil {
		return nil, err
	}
	if c.Status != model.CampaignDraft {
		return nil, conflictf("campaign is %s; only drafts can be started", c.Status)
	}
	if s.Jobs == nil {
		return nil, errors.New("no job runner")
	}
	pkg, err := s.Store.GetFirmwarePackage(ctx, c.PackageID)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return nil, invalidf("package %s no longer exists", c.PackageID)
		}
		return nil, err
	}
	if err := s.VerifyPackage(pkg); err != nil {
		return nil, invalidf("%v", err)
	}
	rows, canary, err := s.Plan(ctx, c)
	if err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return nil, invalidf("the campaign targets no devices")
	}
	if err := s.Store.DeleteCampaignDevices(ctx, id); err != nil {
		return nil, err
	}
	for _, r := range rows {
		if err := s.Store.UpsertCampaignDevice(ctx, r); err != nil {
			return nil, err
		}
	}
	now := time.Now().UTC()
	c.CanaryCount, c.DryRun, c.StartedBy, c.StartedAt = canary, opts.DryRun, by, &now
	c.Status, c.Wave, c.Confirmed, c.PauseReason, c.Error, c.FinishedAt = model.CampaignScheduled, 1, false, "", "", nil
	if err := s.submit(ctx, c); err != nil {
		return nil, err
	}
	return c, nil
}

// submit persists the campaign and queues its job.
func (s *Service) submit(ctx context.Context, c *model.Campaign) error {
	j := &model.Job{Type: JobType, CreatedBy: c.StartedBy,
		Target: map[string]any{"campaignId": c.ID, "name": c.Name, "dryRun": c.DryRun},
		Params: map[string]any{"campaignId": c.ID}}
	if err := s.Store.UpdateCampaign(ctx, c); err != nil {
		return err
	}
	j, err := s.Jobs.Submit(ctx, j)
	if err != nil {
		return err
	}
	c.JobID = j.ID
	if err := s.Store.UpdateCampaign(ctx, c); err != nil {
		return err
	}
	s.emit(EventCampaignUpdated, c)
	return nil
}

// ensureJob resubmits the job when no live job runs the campaign (after a
// restart) and otherwise wakes it.
func (s *Service) ensureJob(ctx context.Context, c *model.Campaign) error {
	if s.isActive(c.ID) {
		if err := s.Store.UpdateCampaign(ctx, c); err != nil {
			return err
		}
		s.emit(EventCampaignUpdated, c)
		s.wake(c.ID)
		return nil
	}
	if s.Jobs == nil {
		return errors.New("no job runner")
	}
	return s.submit(ctx, c)
}

// Confirm is the second explicit gate: it releases wave 2 after the canary
// wave finished and the campaign paused for review.
func (s *Service) Confirm(ctx context.Context, id, by string) (*model.Campaign, error) {
	c, err := s.Store.GetCampaign(ctx, id)
	if err != nil {
		return nil, err
	}
	if c.Status != model.CampaignPaused || c.PauseReason != model.PauseCanary {
		return nil, conflictf("campaign is not waiting for canary confirmation (status %s%s)", c.Status, reasonSuffix(c))
	}
	c.Confirmed, c.Status, c.PauseReason, c.Error = true, model.CampaignRunning, "", ""
	if err := s.ensureJob(ctx, c); err != nil {
		return nil, err
	}
	s.Log.Info("firmware campaign confirmed", "campaign", c.ID, "by", by)
	return c, nil
}

// Pause stops further devices from being dispatched; devices already updating
// finish, because a firmware write cannot be safely interrupted.
func (s *Service) Pause(ctx context.Context, id, by string) (*model.Campaign, error) {
	c, err := s.Store.GetCampaign(ctx, id)
	if err != nil {
		return nil, err
	}
	if c.Status != model.CampaignRunning && c.Status != model.CampaignScheduled {
		return nil, conflictf("campaign is %s; only running or scheduled campaigns can be paused", c.Status)
	}
	c.Status, c.PauseReason = model.CampaignPaused, model.PauseOperator
	if err := s.Store.UpdateCampaign(ctx, c); err != nil {
		return nil, err
	}
	s.emit(EventCampaignUpdated, c)
	s.wake(c.ID)
	s.Log.Info("firmware campaign paused", "campaign", c.ID, "by", by)
	return c, nil
}

// Resume continues a campaign paused by an operator, by the failure threshold
// or by a restart. A canary pause needs Confirm instead.
func (s *Service) Resume(ctx context.Context, id, by string) (*model.Campaign, error) {
	c, err := s.Store.GetCampaign(ctx, id)
	if err != nil {
		return nil, err
	}
	if c.Status != model.CampaignPaused {
		return nil, conflictf("campaign is %s; only paused campaigns can be resumed", c.Status)
	}
	if c.PauseReason == model.PauseCanary {
		return nil, conflictf("campaign is waiting for canary confirmation; confirm it instead")
	}
	c.Status, c.PauseReason, c.Error = model.CampaignRunning, "", ""
	if err := s.ensureJob(ctx, c); err != nil {
		return nil, err
	}
	s.Log.Info("firmware campaign resumed", "campaign", c.ID, "by", by)
	return c, nil
}

// Cancel stops a campaign. Pending devices are skipped; devices already
// updating finish and are verified.
func (s *Service) Cancel(ctx context.Context, id, by string) (*model.Campaign, error) {
	c, err := s.Store.GetCampaign(ctx, id)
	if err != nil {
		return nil, err
	}
	if c.Finished() {
		return nil, conflictf("campaign is already %s", c.Status)
	}
	c.Status, c.PauseReason = model.CampaignCancelled, ""
	if !s.isActive(c.ID) {
		// No job will do the bookkeeping.
		now := time.Now().UTC()
		c.FinishedAt = &now
		devs, _ := s.Store.ListCampaignDevices(ctx, c.ID)
		for _, d := range devs {
			if !d.Status.Terminal() {
				d.Status, d.Message, d.FinishedAt = model.CampaignDeviceSkipped, "campaign cancelled", &now
				_ = s.Store.UpsertCampaignDevice(ctx, d)
				s.emit(EventDeviceUpdated, d)
			}
		}
	}
	if err := s.Store.UpdateCampaign(ctx, c); err != nil {
		return nil, err
	}
	s.emit(EventCampaignUpdated, c)
	s.wake(c.ID)
	s.Log.Info("firmware campaign cancelled", "campaign", c.ID, "by", by)
	return c, nil
}

func reasonSuffix(c *model.Campaign) string {
	if c.PauseReason == "" {
		return ""
	}
	return ", paused: " + c.PauseReason
}
