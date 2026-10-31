package firmware

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"printmarshal/internal/firmware/fwversion"
	"printmarshal/internal/jobs"
	"printmarshal/internal/model"
)

// runCampaign executes one campaign as a job. It is a loop over the persisted
// state: every iteration re-reads the campaign so operator actions (pause,
// confirm, resume, cancel) and the gates (schedule, maintenance window, canary
// confirmation, failure threshold) are honoured between batches. Devices in
// flight always finish and are verified; a firmware write is never interrupted.
func (s *Service) runCampaign(ctx context.Context, jc *jobs.Context) error {
	id, _ := jc.Job.Target["campaignId"].(string)
	if id == "" {
		return errors.New("job has no campaignId")
	}
	if c, err := s.Store.GetCampaign(ctx, id); err != nil {
		return err
	} else if c.JobID != jc.Job.ID {
		return fmt.Errorf("campaign %s is owned by job %s; this job (%s) is superseded", id, c.JobID, jc.Job.ID)
	}
	if s.isActive(id) {
		return fmt.Errorf("campaign %s already has a running worker", id)
	}
	s.setActive(id, true)
	defer s.setActive(id, false)

	devs, err := s.Store.ListCampaignDevices(ctx, id)
	if err != nil {
		return err
	}
	jc.SetTotal(len(devs))
	// Devices already decided (planning skips, or a previous run) are reported
	// up front so the job's results stand on their own.
	for _, d := range devs {
		if d.Status.Terminal() {
			s.report(jc, d, "")
		}
	}

	for {
		c, err := s.Store.GetCampaign(ctx, id)
		if err != nil {
			return err
		}
		if c.JobID != jc.Job.ID {
			return fmt.Errorf("campaign %s was handed to job %s; stopping", id, c.JobID)
		}
		devs, err := s.Store.ListCampaignDevices(ctx, id)
		if err != nil {
			return err
		}
		switch c.Status {
		case model.CampaignCancelled:
			s.finishCancelled(ctx, jc, c, devs)
			return nil
		case model.CampaignCompleted, model.CampaignFailed:
			return nil
		case model.CampaignDraft:
			return fmt.Errorf("campaign %s has not been started", id)
		case model.CampaignPaused:
			if err := s.wait(ctx, id, s.ControlPoll); err != nil {
				return err
			}
			continue
		}

		pending := rowsWith(devs, c.Wave, model.CampaignDevicePending)
		if len(pending) == 0 {
			if c.Wave == 1 && len(rowsWith(devs, 2, model.CampaignDevicePending)) > 0 {
				if c.CanaryCount > 0 && !c.Confirmed {
					s.pause(ctx, id, model.PauseCanary, "canary wave finished; review the results and confirm to release the remaining devices")
					continue
				}
				s.update(ctx, id, func(c *model.Campaign) { c.Wave = 2 })
				continue
			}
			s.finish(ctx, id, devs)
			return nil
		}

		// Gates: the scheduled time and the maintenance window.
		now := s.now()
		if (c.ScheduledAt != nil && now.Before(*c.ScheduledAt)) || !InWindow(c.Window, now) {
			if c.Status != model.CampaignScheduled {
				s.update(ctx, id, func(c *model.Campaign) { c.Status = model.CampaignScheduled })
			}
			if err := s.wait(ctx, id, s.ControlPoll); err != nil {
				return err
			}
			continue
		}
		if c.Status != model.CampaignRunning {
			if c = s.update(ctx, id, func(c *model.Campaign) { c.Status = model.CampaignRunning }); c == nil {
				continue
			}
		}

		pkg, err := s.Store.GetFirmwarePackage(ctx, c.PackageID)
		if err != nil {
			s.update(ctx, id, func(c *model.Campaign) {
				now := time.Now().UTC()
				c.Status, c.FinishedAt, c.Error = model.CampaignFailed, &now, "package no longer exists: "+err.Error()
			})
			return fmt.Errorf("package %s: %w", c.PackageID, err)
		}
		// Drivers open StoragePath directly: hand them the resolved location.
		resolved := *pkg
		resolved.StoragePath = s.PackagePath(pkg)
		pkg = &resolved

		// One batch of up to Concurrency devices; the loop then re-reads state.
		n := c.Concurrency
		if n <= 0 {
			n = DefaultConcurrency
		}
		if n > len(pending) {
			n = len(pending)
		}
		var wg sync.WaitGroup
		for _, row := range pending[:n] {
			wg.Add(1)
			go func(row *model.CampaignDevice) {
				defer wg.Done()
				s.updateDevice(ctx, jc, c, pkg, row)
			}(row)
		}
		wg.Wait()

		if c.FailureThresholdPercent > 0 {
			devs, _ := s.Store.ListCampaignDevices(ctx, id)
			sum := Summarize(devs)
			if attempted := sum.Succeeded + sum.Failed; sum.Failed > 0 && sum.Failed*100 > c.FailureThresholdPercent*attempted {
				if cur, err := s.Store.GetCampaign(ctx, id); err == nil && cur.Status == model.CampaignRunning {
					s.pause(ctx, id, model.PauseThreshold, fmt.Sprintf("failure threshold exceeded: %d of %d attempted devices failed (limit %d%%)", sum.Failed, attempted, c.FailureThresholdPercent))
				}
			}
		}
	}
}

func rowsWith(devs []*model.CampaignDevice, wave int, status model.CampaignDeviceStatus) []*model.CampaignDevice {
	var out []*model.CampaignDevice
	for _, d := range devs {
		if d.Wave == wave && d.Status == status {
			out = append(out, d)
		}
	}
	return out
}

// update re-reads, mutates and persists a campaign so a stale copy never
// overwrites an operator's concurrent change. It returns the saved campaign.
func (s *Service) update(ctx context.Context, id string, fn func(c *model.Campaign)) *model.Campaign {
	c, err := s.Store.GetCampaign(ctx, id)
	if err != nil {
		return nil
	}
	fn(c)
	if err := s.Store.UpdateCampaign(ctx, c); err != nil {
		s.Log.Error("firmware campaign update", "campaign", id, "err", err)
		return nil
	}
	s.emit(EventCampaignUpdated, c)
	return c
}

func (s *Service) pause(ctx context.Context, id, reason, msg string) {
	s.update(ctx, id, func(c *model.Campaign) {
		if c.Finished() {
			return
		}
		c.Status, c.PauseReason, c.Error = model.CampaignPaused, reason, msg
	})
	s.Log.Info("firmware campaign paused", "campaign", id, "reason", reason, "detail", msg)
}

func (s *Service) finish(ctx context.Context, id string, devs []*model.CampaignDevice) {
	sum := Summarize(devs)
	s.update(ctx, id, func(c *model.Campaign) {
		if c.Finished() {
			return
		}
		now := time.Now().UTC()
		c.FinishedAt, c.PauseReason, c.Error = &now, "", ""
		c.Status = model.CampaignCompleted
		if sum.Succeeded == 0 && sum.Failed > 0 {
			c.Status, c.Error = model.CampaignFailed, "every attempted device failed"
		}
	})
}

func (s *Service) finishCancelled(ctx context.Context, jc *jobs.Context, c *model.Campaign, devs []*model.CampaignDevice) {
	now := time.Now().UTC()
	for _, d := range devs {
		if !d.Status.Terminal() {
			d.FinishedAt = &now
			s.setRow(ctx, d, model.CampaignDeviceSkipped, "campaign cancelled")
			s.report(jc, d, "")
		}
	}
	s.update(ctx, c.ID, func(c *model.Campaign) {
		if c.FinishedAt == nil {
			c.FinishedAt = &now
		}
	})
}

// setRow persists a device row change and publishes it.
func (s *Service) setRow(ctx context.Context, row *model.CampaignDevice, status model.CampaignDeviceStatus, msg string) {
	row.Status, row.Message = status, msg
	if status.Terminal() && row.FinishedAt == nil {
		now := time.Now().UTC()
		row.FinishedAt = &now
	}
	if err := s.Store.UpsertCampaignDevice(ctx, row); err != nil {
		s.Log.Error("firmware campaign device update", "campaign", row.CampaignID, "device", row.DeviceID, "err", err)
	}
	s.emit(EventDeviceUpdated, row)
}

// report mirrors a terminal row into the job's per-device results. outcome,
// when set, is the post-delivery verification verdict (audit A20): verified,
// diverged or unverifiable, using the same vocabulary as configuration writes.
func (s *Service) report(jc *jobs.Context, row *model.CampaignDevice, outcome model.WriteOutcome) {
	status := "ok"
	switch row.Status {
	case model.CampaignDeviceFailed:
		status = "error"
	case model.CampaignDeviceSkipped:
		status = "skipped"
	}
	data := map[string]any{"wave": row.Wave, "fromVersion": row.FromVersion, "toVersion": row.ToVersion}
	if outcome != "" {
		data["outcome"] = string(outcome)
	}
	jc.Result(row.DeviceID, "", status, row.Message, data)
}

// updateDevice runs the full per-device sequence: compatibility, pre-flight,
// delivery (or dry run), audit and verification.
func (s *Service) updateDevice(ctx context.Context, jc *jobs.Context, c *model.Campaign, pkg *model.FirmwarePackage, row *model.CampaignDevice) {
	start := time.Now().UTC()
	row.StartedAt = &start
	done := func(status model.CampaignDeviceStatus, msg string) {
		s.setRow(ctx, row, status, msg)
		s.report(jc, row, "")
	}
	verdict := func(status model.CampaignDeviceStatus, outcome model.WriteOutcome, msg string) {
		s.setRow(ctx, row, status, string(outcome)+": "+msg)
		s.report(jc, row, outcome)
	}

	dev, err := s.Store.GetDevice(ctx, row.DeviceID)
	if err != nil {
		done(model.CampaignDeviceFailed, "device record no longer exists")
		return
	}
	row.FromVersion, row.ToVersion = dev.Firmware, pkg.Version
	if !s.claim(ctx, c, row) {
		return // paused or cancelled since the batch was planned; the loop re-reads and parks
	}
	if dev.Decommissioned() {
		// Planning leaves these out, but a campaign can wait at a gate for days.
		done(model.CampaignDeviceSkipped, model.ErrDecommissioned.Error())
		return
	}

	v := fwversion.Check(pkg, dev, c.AllowDowngrade)
	switch {
	case v.Current:
		done(model.CampaignDeviceSkipped, v.Reason)
		return
	case !v.Compatible:
		done(model.CampaignDeviceSkipped, "incompatible: "+v.Reason)
		return
	}

	polled, err := s.preflight(ctx, dev)
	if err != nil {
		done(model.CampaignDeviceFailed, "pre-flight: "+err.Error())
		return
	}
	dev = polled
	row.FromVersion = dev.Firmware
	if dev.Firmware != "" && fwversion.Equal(dev.Firmware, pkg.Version) {
		done(model.CampaignDeviceSkipped, fmt.Sprintf("already at version %s (confirmed by pre-flight poll)", pkg.Version))
		return
	}

	up, drv, err := s.updater(dev)
	if err != nil {
		done(model.CampaignDeviceFailed, err.Error())
		return
	}
	if err := s.VerifyPackage(pkg); err != nil {
		done(model.CampaignDeviceFailed, "package integrity: "+err.Error())
		return
	}

	mode := "delivering"
	if c.DryRun {
		mode = "dry run: validating"
	}
	// A dry run only validates, so read credentials suffice; a real delivery
	// needs a write credential bound to the device (audit A19).
	creds := s.Creds(ctx, dev)
	if !c.DryRun {
		wc, err := s.WriteCreds(ctx, dev)
		if err != nil {
			_ = s.Store.AddAudit(ctx, &model.AuditEntry{Actor: c.StartedBy, Action: "firmware.update", Target: dev.ID,
				After: map[string]any{"campaignId": c.ID, "packageId": pkg.ID, "version": pkg.Version, "driver": drv, "reason": err.Error()}, Result: "refused"})
			done(model.CampaignDeviceFailed, err.Error())
			return
		}
		creds = wc
	}
	s.setRow(ctx, row, model.CampaignDeviceUpdating, fmt.Sprintf("%s %s via %s", mode, pkg.Version, drv))
	res, err := up.UpdateFirmware(ctx, dev, creds, pkg, model.WriteOptions{DryRun: c.DryRun})
	if !c.DryRun {
		s.auditDelivery(ctx, c, dev, pkg, drv, res, err)
	}
	if err != nil {
		done(model.CampaignDeviceFailed, fmt.Sprintf("driver %s: %v", drv, err))
		return
	}
	if res == nil {
		done(model.CampaignDeviceFailed, fmt.Sprintf("driver %s returned no result", drv))
		return
	}
	if c.DryRun {
		msg := res.Message
		if msg == "" {
			msg = fmt.Sprintf("package %s validated; would deliver via %s", pkg.Version, drv)
		}
		if !res.Accepted {
			done(model.CampaignDeviceFailed, "dry run: "+msg)
			return
		}
		done(model.CampaignDeviceSucceeded, "dry run: "+msg)
		return
	}
	if !res.Accepted {
		msg := res.Message
		if msg == "" {
			msg = "no reason given"
		}
		done(model.CampaignDeviceFailed, fmt.Sprintf("device rejected the image via %s: %s", drv, msg))
		return
	}

	timeout := s.VerifyTimeout
	if c.VerifyTimeoutSecs > 0 {
		timeout = time.Duration(c.VerifyTimeoutSecs) * time.Second
	}
	s.setRow(ctx, row, model.CampaignDeviceVerifying, fmt.Sprintf("image accepted via %s; waiting up to %s for the device to report %s", drv, timeout, pkg.Version))
	got, outcome, err := s.verify(ctx, dev, pkg.Version, timeout)
	if err != nil {
		verdict(model.CampaignDeviceFailed, outcome, err.Error())
		return
	}
	verdict(model.CampaignDeviceSucceeded, outcome, fmt.Sprintf("device reports %s (was %s)", got, orNone(row.FromVersion)))
}

// preflight re-reads the device and requires it reachable and idle. Transient
// conditions are retried a few times.
func (s *Service) preflight(ctx context.Context, dev *model.Device) (*model.Device, error) {
	if s.Poll == nil {
		return nil, errors.New("no poller configured")
	}
	var last error
	for attempt := 0; attempt < 3; attempt++ {
		if attempt > 0 {
			if err := sleepCtx(ctx, s.RetryDelay); err != nil {
				return nil, err
			}
		}
		upd, err := s.Poll(ctx, dev)
		switch {
		case err != nil:
			last = fmt.Errorf("poll failed: %w", err)
		case upd == nil || upd.Status == nil || !upd.Status.Online:
			last = errors.New("device is unreachable")
		case upd.Status.State == "printing" || upd.Status.State == "processing":
			last = fmt.Errorf("device is busy (%s); updates are delivered only to idle devices", upd.Status.State)
		default:
			return upd, nil
		}
	}
	return nil, last
}

// verify polls the device until it reports want or timeout elapses. The
// outcome names the verdict (audit A20): verified when the device reports the
// package version, diverged when it reports another version at the deadline,
// unverifiable when it reported no version at all (offline, poll errors,
// interrupted).
func (s *Service) verify(ctx context.Context, dev *model.Device, want string, timeout time.Duration) (string, model.WriteOutcome, error) {
	if s.Poll == nil {
		return "", model.OutcomeUnverifiable, errors.New("no poller configured")
	}
	deadline := time.Now().Add(timeout)
	last := ""
	var lastErr error
	for {
		upd, err := s.Poll(ctx, dev)
		if err != nil {
			lastErr = err
		} else if upd != nil {
			if upd.Firmware != "" {
				last = upd.Firmware
			}
			if upd.Status != nil && upd.Status.Online && upd.Firmware != "" && fwversion.Equal(upd.Firmware, want) {
				return upd.Firmware, model.OutcomeVerified, nil
			}
		}
		if time.Now().After(deadline) {
			break
		}
		if err := sleepCtx(ctx, s.VerifyInterval); err != nil {
			return last, model.OutcomeUnverifiable, fmt.Errorf("verification interrupted: %w", err)
		}
	}
	msg := fmt.Sprintf("timed out after %s waiting for the device to report %s", timeout, want)
	outcome := model.OutcomeUnverifiable
	if last != "" {
		msg += "; device reports " + last
		outcome = model.OutcomeDiverged
	} else {
		msg += "; device reported no version"
	}
	if lastErr != nil {
		msg += " (last poll error: " + lastErr.Error() + ")"
	}
	return last, outcome, errors.New(msg)
}

// auditDelivery records every real delivery attempt (spec §11 "audit everything
// that changes device state"), whatever its outcome.
func (s *Service) auditDelivery(ctx context.Context, c *model.Campaign, dev *model.Device, pkg *model.FirmwarePackage, drv string, res *model.FirmwareResult, err error) {
	result := "accepted"
	switch {
	case err != nil:
		result = "error: " + err.Error()
	case res == nil || !res.Accepted:
		result = "rejected"
	}
	after := map[string]any{"version": pkg.Version, "packageId": pkg.ID, "fileName": pkg.FileName, "sha256": pkg.SHA256, "driver": drv, "campaignId": c.ID, "campaign": c.Name}
	if res != nil && res.Message != "" {
		after["message"] = res.Message
	}
	actor := c.StartedBy
	if actor == "" {
		actor = "firmware"
	}
	if err := s.Store.AddAudit(ctx, &model.AuditEntry{Actor: actor, Action: AuditUpdate, Target: dev.ID,
		Before: map[string]any{"firmware": dev.Firmware, "address": dev.Address}, After: after, Result: result}); err != nil {
		s.Log.Error("firmware audit", "device", dev.ID, "err", err)
	}
}

func sleepCtx(ctx context.Context, d time.Duration) error {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}

func orNone(s string) string {
	if s == "" {
		return "unknown"
	}
	return s
}
