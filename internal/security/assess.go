package security

import (
	"context"
	"fmt"
	"time"

	"printmarshal/internal/jobs"
	"printmarshal/internal/model"
)

// Assess queues an assessment of the target against a policy (job
// "security.assess"). Reads only: nothing on the device changes.
func (s *Service) Assess(ctx context.Context, policyID string, t Target, actor string) (*model.Job, error) {
	if _, err := s.Store.GetPolicy(ctx, policyID); err != nil {
		return nil, err
	}
	if _, err := s.ResolveTarget(ctx, t); err != nil {
		return nil, err
	}
	target := t.toMap()
	target["policyId"] = policyID
	return s.Jobs.Submit(ctx, &model.Job{Type: JobAssess, CreatedBy: actor, Target: target, Params: map[string]any{"policyId": policyID}})
}

func (s *Service) runAssess(ctx context.Context, jc *jobs.Context) error {
	policy, err := s.Store.GetPolicy(ctx, jobString(jc.Job, "policyId"))
	if err != nil {
		return err
	}
	devs, err := s.ResolveTarget(ctx, targetFromJob(jc.Job))
	if err != nil {
		return err
	}
	// Certificate expiry is re-checked from stored observations before every
	// assessment so cert:expiry alerts never go stale (main should also call
	// CheckExpiry on a schedule).
	if err := s.CheckExpiry(ctx); err != nil {
		s.Log.Warn("security: expiry check", "err", err)
	}
	s.forEachDevice(ctx, jc, devs, 2*time.Minute, func(ctx context.Context, d *model.Device) (string, string, map[string]any) {
		a, err := s.AssessDevice(ctx, d, policy, jc.Job.ID)
		if err != nil {
			return "error", err.Error(), nil
		}
		return "ok", fmt.Sprintf("score %.1f: %d pass, %d fail, %d unknown", a.Score, a.Passed, a.Failed, a.Unknown),
			map[string]any{"assessmentId": a.ID, "score": a.Score, "passed": a.Passed, "failed": a.Failed, "unknown": a.Unknown, "compliant": a.Compliant}
	})
	return nil
}

// AssessDevice gathers a device's security document, evaluates the policy,
// stores the assessment, publishes security.assessed and reconciles the
// per-item alerts for failing critical items.
func (s *Service) AssessDevice(ctx context.Context, dev *model.Device, policy *model.Policy, jobID string) (*model.Assessment, error) {
	creds := s.creds(ctx, dev)
	doc, _ := s.Gather(ctx, dev, creds)
	a := Assess(policy, doc, dev.ID, jobID)
	a.At = s.Now()
	if err := s.Store.SaveAssessment(ctx, a); err != nil {
		return nil, err
	}
	s.reconcilePolicyAlerts(ctx, dev, policy, a)
	s.emit("security.assessed", a)
	return a, nil
}

// reconcilePolicyAlerts raises one alert per failing critical item (key
// policy:<policyId>:<itemId>) and clears the policy's alerts that no longer fail.
func (s *Service) reconcilePolicyAlerts(ctx context.Context, dev *model.Device, policy *model.Policy, a *model.Assessment) {
	prefix := "policy:" + policy.ID + ":"
	keep := map[string]bool{}
	for _, it := range a.Items {
		if it.Status != model.ItemFail || it.Severity != model.SeverityItemCritical {
			continue
		}
		key := prefix + it.ItemID
		keep[key] = true
		desc := fmt.Sprintf("Policy %q: %s", policy.Name, it.Name)
		if it.Message != "" {
			desc += " (" + it.Message + ")"
		}
		s.raiseAlert(ctx, dev, key, model.SeverityCritical, desc)
	}
	s.clearAlerts(ctx, dev, prefix, keep)
}

// ComplianceSummary is the fleet roll-up of the latest assessments.
type ComplianceSummary struct {
	Devices      int     `json:"devices"`
	Compliant    int     `json:"compliant"`
	NonCompliant int     `json:"nonCompliant"`
	AvgScore     float64 `json:"avgScore"`
	// Unknown is the number of items, across every assessed device, that the
	// device would not reveal. They are excluded from each score's denominator
	// and reported here so they are never mistaken for passes.
	Unknown int `json:"unknown"`
	// NotAssessed is the number of devices in the caller's view with no latest
	// assessment (only known when the caller passes the view's size).
	NotAssessed int `json:"notAssessed"`
	// FailingItems counts failures per item id across the fleet.
	FailingItems map[string]int `json:"failingItems"`
}

// Summarize rolls up assessments.
func Summarize(as []*model.Assessment) ComplianceSummary {
	return SummarizeFor(as, len(as))
}

// SummarizeFor rolls up assessments for a view of devicesInView devices, so
// the summary can also say how many devices have not been assessed.
func SummarizeFor(as []*model.Assessment, devicesInView int) ComplianceSummary {
	sum := ComplianceSummary{FailingItems: map[string]int{}}
	var total float64
	for _, a := range as {
		sum.Devices++
		if a.Compliant {
			sum.Compliant++
		} else {
			sum.NonCompliant++
		}
		total += a.Score
		sum.Unknown += a.Unknown
		for _, it := range a.Items {
			if it.Status == model.ItemFail {
				sum.FailingItems[it.ItemID]++
			}
		}
	}
	if sum.Devices > 0 {
		sum.AvgScore = float64(int(total/float64(sum.Devices)*10+0.5)) / 10
	}
	if devicesInView > sum.Devices {
		sum.NotAssessed = devicesInView - sum.Devices
	}
	return sum
}
