package security

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"printmarshal/internal/engine"
	"printmarshal/internal/jobs"
	"printmarshal/internal/model"
)

// Remediate queues a remediation of the target against a policy (job
// "security.remediate"). By API convention the first call is a dry run;
// callers must pass DryRun=false explicitly to write.
func (s *Service) Remediate(ctx context.Context, policyID string, t Target, opts Options, actor string) (*model.Job, error) {
	if _, err := s.Store.GetPolicy(ctx, policyID); err != nil {
		return nil, err
	}
	if _, err := s.ResolveTarget(ctx, t); err != nil {
		return nil, err
	}
	target := t.toMap()
	target["policyId"] = policyID
	return s.Jobs.Submit(ctx, &model.Job{Type: JobRemediate, CreatedBy: actor, Target: target,
		Params: map[string]any{"policyId": policyID, "dryRun": opts.DryRun}})
}

func (s *Service) runRemediate(ctx context.Context, jc *jobs.Context) error {
	policy, err := s.Store.GetPolicy(ctx, jobString(jc.Job, "policyId"))
	if err != nil {
		return err
	}
	devs, err := s.ResolveTarget(ctx, targetFromJob(jc.Job))
	if err != nil {
		return err
	}
	dryRun := jobBool(jc.Job, "dryRun", true)
	s.forEachDevice(ctx, jc, devs, 3*time.Minute, func(ctx context.Context, d *model.Device) (string, string, map[string]any) {
		res, err := s.RemediateDevice(ctx, d, policy, Options{DryRun: dryRun}, jc.Job.CreatedBy, jc.Job.ID)
		if err != nil {
			return "error", err.Error(), nil
		}
		verb := "written"
		if dryRun {
			verb = "would write"
		}
		status := "ok"
		if res.Failed > 0 {
			status = "error"
		}
		return status, fmt.Sprintf("%d %s, %d skipped, %d failed", res.Written, verb, res.Skipped, res.Failed),
			map[string]any{"dryRun": dryRun, "items": res.Items, "written": res.Written, "skipped": res.Skipped, "failed": res.Failed}
	})
	return nil
}

// RemediateDevice assesses a device, then writes the remediation of every
// failing item that allows it, honouring the management-path rule (spec §11)
// and dry run. Every real write is audited; secrets never appear in the
// result or the audit entry.
func (s *Service) RemediateDevice(ctx context.Context, dev *model.Device, policy *model.Policy, opts Options, actor, jobID string) (*model.RemediationResult, error) {
	creds := s.creds(ctx, dev)
	doc, _ := s.Gather(ctx, dev, creds)
	assessment := Assess(policy, doc, dev.ID, jobID)
	res := &model.RemediationResult{DeviceID: dev.ID, PolicyID: policy.ID, DryRun: opts.DryRun, At: s.Now(), Items: []model.RemediationItem{}}

	results := map[string]model.ItemResult{}
	for _, r := range assessment.Items {
		results[r.ItemID] = r
	}
	// Plan: which items get a change, and which the management-path rule blocks.
	plan := PlanRemediation(dev, policy, results)
	var changes []model.SettingChange
	byKey := map[string]string{} // setting key -> item id
	for _, p := range plan {
		it := model.RemediationItem{ItemID: p.Item.ID, Key: p.Item.Key, Name: p.Item.Name, Status: p.Status, Message: p.Message}
		if p.Change != nil {
			it.Key = p.Change.Key
			it.Before = displayValue(p.Change.Key, results[p.Item.ID].Current)
			it.After = displayValue(p.Change.Key, p.Change.Value)
			changes = append(changes, *p.Change)
			byKey[p.Change.Key] = p.Item.ID
		}
		res.Items = append(res.Items, it)
	}
	if len(changes) == 0 {
		count(res)
		return res, nil
	}

	written, err := s.applyChanges(ctx, dev, creds, changes, model.WriteOptions{DryRun: opts.DryRun})
	if err != nil && len(written) == 0 {
		msg := err.Error()
		if errors.Is(err, engine.ErrUnsupported) {
			msg = "no driver can write security settings on this device"
		}
		for i := range res.Items {
			if res.Items[i].Status == "" {
				res.Items[i].Status, res.Items[i].Message = model.RemediationSkipped, msg
			}
		}
		count(res)
		return res, nil
	}
	before, after := map[string]any{}, map[string]any{}
	for i := range res.Items {
		it := &res.Items[i]
		if it.Status != "" {
			continue
		}
		r, ok := written[it.Key]
		if !ok {
			it.Status, it.Message = model.RemediationSkipped, "no driver accepted this setting"
			continue
		}
		it.Driver = r.Driver
		switch r.Status {
		case model.SettingWritten:
			it.Status = model.RemediationWritten
			before[it.Key], after[it.Key] = displayValue(it.Key, r.Before), displayValue(it.Key, r.After)
		case model.SettingWouldWrite:
			it.Status = model.RemediationWouldWrite
		case model.SettingUnchanged:
			it.Status = model.RemediationUnchanged
		case model.SettingFailed:
			it.Status, it.Message = model.RemediationFailed, r.Error
		default:
			it.Status, it.Message = model.RemediationSkipped, r.Error
		}
	}
	count(res)
	if !opts.DryRun && len(after) > 0 {
		_ = s.Store.AddAudit(ctx, &model.AuditEntry{Actor: actor, Action: "security.remediate", Target: dev.ID,
			Before: before, After: map[string]any{"policyId": policy.ID, "jobId": jobID, "changes": after},
			Result: fmt.Sprintf("%d written, %d failed", res.Written, res.Failed)})
		s.emit("security.remediated", res)
		// Refresh compliance and alerts after a real change.
		if _, err := s.AssessDevice(ctx, dev, policy, jobID); err != nil {
			s.warn("re-assess after remediation", dev, "", err)
		}
	}
	return res, nil
}

func count(res *model.RemediationResult) {
	res.Written, res.Skipped, res.Failed = 0, 0, 0
	for _, it := range res.Items {
		switch it.Status {
		case model.RemediationWritten, model.RemediationWouldWrite:
			res.Written++
		case model.RemediationSkipped:
			res.Skipped++
		case model.RemediationFailed:
			res.Failed++
		}
	}
}

// PlannedItem is one policy item's remediation plan.
type PlannedItem struct {
	Item    model.PolicyItem
	Change  *model.SettingChange // nil = nothing to write
	Status  model.RemediationStatus
	Message string
}

// PlanRemediation decides, per policy item, whether a change will be written.
// It enforces the management-path rule of spec §11: of the protocols the
// device is currently managed through (ManagementPaths), at least one must
// survive the whole batch, so a change whose ManagementPath is the only one
// left is skipped with a clear message. Items that pass are reported
// unchanged; unknown or assess-only items are skipped.
func PlanRemediation(dev *model.Device, policy *model.Policy, results map[string]model.ItemResult) []PlannedItem {
	remaining := ManagementPaths(dev)
	var out []PlannedItem
	for _, it := range policy.Items {
		p := PlannedItem{Item: it}
		r, ok := results[it.ID]
		switch {
		case !ok || r.Status == model.ItemUnknown:
			p.Status, p.Message = model.RemediationSkipped, "setting not readable on this device"
		case r.Status == model.ItemPass:
			p.Status = model.RemediationUnchanged
		case !it.Remediate:
			p.Status, p.Message = model.RemediationSkipped, "assess-only item (remediate=false)"
		default:
			change := RemediationFor(it)
			if change == nil {
				p.Status, p.Message = model.RemediationSkipped, fmt.Sprintf("no remediation defined for operator %s", it.Operator)
				break
			}
			if mp := it.ManagementPath; mp != "" && remaining[mp] {
				if len(remaining) == 1 {
					p.Status = model.RemediationSkipped
					p.Message = fmt.Sprintf("management-path rule (spec §11): %s is the only protocol PrintMarshal manages this device through; the change would lock it out", strings.ToUpper(mp))
					break
				}
				delete(remaining, mp)
			}
			p.Change = change
		}
		out = append(out, p)
	}
	return out
}
