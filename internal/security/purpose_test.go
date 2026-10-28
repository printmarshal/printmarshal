package security

import (
	"context"
	"strings"
	"testing"

	"printmarshal/internal/model"
)

// Audit A19: a real remediation on a device with only read credentials in
// scope fails every planned item with a clear reason and writes nothing; the
// dry run still works because it only reads. Rotation behaves the same.
func TestRemediationRefusedWithoutWriteCredential(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	if err := h.st.DeleteCredential(ctx, h.writeCred.ID); err != nil {
		t.Fatal(err)
	}
	if err := h.st.CreateCredential(ctx, &model.Credential{Name: "ro", Type: model.CredSNMPCommunity, Purpose: model.PurposeRead}, &model.Secret{Community: "public"}); err != nil {
		t.Fatal(err)
	}
	fake := newFake("fake", map[string]any{"protocols.telnet.enabled": true})
	h.reg.Register(fake)
	dev := h.device("10.0.0.21", map[model.Capability]string{model.CapReadSecurity: "fake", model.CapWriteSecurity: "fake"})
	p := policyWith(model.PolicyItem{ID: "telnet", Key: "protocols.telnet.enabled", Operator: model.OpEq, Value: false, Severity: "critical", Remediate: true})

	res, err := h.svc.RemediateDevice(ctx, dev, p, Options{DryRun: true}, "tester", "j1")
	if err != nil || res.Written != 1 || res.Items[0].Status != model.RemediationWouldWrite {
		t.Fatalf("dry run must work with read credentials: %+v %v", res, err)
	}
	res, err = h.svc.RemediateDevice(ctx, dev, p, Options{DryRun: false}, "tester", "j2")
	if err != nil {
		t.Fatal(err)
	}
	if res.Failed != 1 || res.Items[0].Status != model.RemediationFailed || !strings.Contains(res.Items[0].Message, "no write credential bound") {
		t.Fatalf("real remediation must fail per item: %+v", res.Items)
	}
	if len(fake.writes) != 0 || fake.value("protocols.telnet.enabled") != true {
		t.Fatal("the device was written to without a write credential")
	}
	audit, _ := h.st.ListAudit(ctx, 10)
	var refused bool
	for _, e := range audit {
		if e.Action == "security.remediate" && e.Result == "refused" && e.Target == dev.ID {
			refused = true
		}
	}
	if !refused {
		t.Errorf("refusal not audited: %+v", audit)
	}

	// Rotation: dry run reports, the real run fails without writing or storing.
	j, _ := h.svc.RotateSNMPCommunity(ctx, Target{DeviceIDs: []string{dev.ID}}, Options{DryRun: true}, "tester")
	if _, results := h.wait(j); results[0].Data["status"] != "would_rotate" {
		t.Fatalf("rotation dry run: %+v", results)
	}
	j, _ = h.svc.RotateSNMPCommunity(ctx, Target{DeviceIDs: []string{dev.ID}}, Options{DryRun: false}, "tester")
	done, results := h.wait(j)
	if done.Status != model.JobFailed || results[0].Data["status"] != "failed" || !strings.Contains(results[0].Message, "no write credential bound") {
		t.Fatalf("rotation without a write credential: %+v %+v", done, results)
	}
	if len(fake.writes) != 0 {
		t.Fatal("rotation wrote without a write credential")
	}
	if cs, _ := h.st.ListCredentials(ctx); len(cs) != 1 {
		t.Errorf("rotation stored a credential: %+v", cs)
	}

	// A device-scoped "both" credential unblocks the device.
	if err := h.st.CreateCredential(ctx, &model.Credential{Name: "dev rw", Type: model.CredAdminPassword, Username: "admin", Scope: "device:" + dev.ID, Purpose: model.PurposeBoth}, &model.Secret{Password: "devPw12345"}); err != nil {
		t.Fatal(err)
	}
	res, _ = h.svc.RemediateDevice(ctx, dev, p, Options{DryRun: false}, "tester", "j3")
	if res.Written != 1 || res.Verified != 1 || fake.value("protocols.telnet.enabled") != false {
		t.Errorf("remediation with a write credential: %+v", res)
	}
}

// Audit A20: when the device accepts the write but the re-assessment still
// fails, the item is diverged and carries the read-back value.
func TestRemediationDivergedOutcome(t *testing.T) {
	h := newHarness(t)
	fake := newFake("fake", map[string]any{"protocols.telnet.enabled": true, "protocols.ftp.enabled": true})
	fake.stickKeys["protocols.telnet.enabled"] = true // the device says ok and keeps telnet on
	h.reg.Register(fake)
	dev := h.device("10.0.0.22", map[model.Capability]string{model.CapReadSecurity: "fake", model.CapWriteSecurity: "fake"})
	p := policyWith(
		model.PolicyItem{ID: "telnet", Key: "protocols.telnet.enabled", Operator: model.OpEq, Value: false, Severity: "critical", Remediate: true},
		model.PolicyItem{ID: "ftp", Key: "protocols.ftp.enabled", Operator: model.OpEq, Value: false, Severity: "critical", Remediate: true},
	)
	res, err := h.svc.RemediateDevice(context.Background(), dev, p, Options{DryRun: false}, "tester", "j")
	if err != nil {
		t.Fatal(err)
	}
	byID := map[string]model.RemediationItem{}
	for _, it := range res.Items {
		byID[it.ItemID] = it
	}
	if it := byID["telnet"]; it.Status != model.RemediationWritten || it.Outcome != model.OutcomeDiverged || it.ReadBack != true {
		t.Errorf("telnet must be written but diverged with read-back true: %+v", it)
	}
	if it := byID["ftp"]; it.Outcome != model.OutcomeVerified {
		t.Errorf("ftp must be verified: %+v", it)
	}
	if res.Written != 2 || res.Verified != 1 || res.Diverged != 1 {
		t.Errorf("counts = %+v", res)
	}
	audit, _ := h.st.ListAudit(context.Background(), 5)
	var found bool
	for _, e := range audit {
		if e.Action == "security.remediate" && strings.Contains(e.Result, "1 diverged") {
			if oc, _ := e.After["outcomes"].(map[string]any); oc["protocols.telnet.enabled"] == "diverged" {
				found = true
			}
		}
	}
	if !found {
		t.Errorf("audit must carry the outcome: %+v", audit)
	}
}
