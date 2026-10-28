package configmgmt

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"printmarshal/internal/model"
	"printmarshal/internal/store"
)

// Audit A19: a device that only has read credentials in scope can never be
// written to. The dry run (reads only) still works; the real apply is refused
// per device with a clear reason, touches nothing and is audited as refused.
func TestRealApplyRefusedWithoutWriteCredential(t *testing.T) {
	env := newTestEnv(t)
	ctx := context.Background()
	if err := env.st.DeleteCredential(ctx, env.writeCred); err != nil {
		t.Fatal(err)
	}
	// A read credential alone does not qualify.
	if err := env.st.CreateCredential(ctx, &model.Credential{Name: "ro", Type: model.CredSNMPCommunity, Purpose: model.PurposeRead}, &model.Secret{Community: "public"}); err != nil {
		t.Fatal(err)
	}
	changes := []model.SettingChange{{Key: "identity.location", Value: "Lobby"}}
	target := model.ConfigTarget{DeviceIDs: []string{env.dev.ID}}

	j, err := env.svc.Apply(ctx, ApplyRequest{Changes: changes, Target: target, DryRun: true}, "alice")
	if err != nil {
		t.Fatal(err)
	}
	if done := env.wait(t, j); done.Status != model.JobSucceeded {
		t.Fatalf("dry run must work with read credentials: %+v", done)
	}
	results, _ := env.st.ListJobResults(ctx, j.ID)
	if len(results) != 1 || results[0].Status != "ok" {
		t.Fatalf("dry-run results = %+v", results)
	}

	j, err = env.svc.Apply(ctx, ApplyRequest{Changes: changes, Target: target}, "alice")
	if err != nil {
		t.Fatal(err)
	}
	done := env.wait(t, j)
	if done.Status != model.JobFailed {
		t.Fatalf("real apply without a write credential must fail: %+v", done)
	}
	results, _ = env.st.ListJobResults(ctx, j.ID)
	if len(results) != 1 || results[0].Status != "error" || !strings.Contains(results[0].Message, "no write credential bound") {
		t.Fatalf("results = %+v", results)
	}
	if n := len(env.fx.Sets()); n != 0 {
		t.Fatalf("the device was written to without a write credential: %d sets", n)
	}
	writes, _ := env.st.ListWriteRecords(ctx, store.WriteRecordFilter{DeviceID: env.dev.ID})
	for _, w := range writes {
		if !w.Result.DryRun {
			t.Errorf("a real write record was stored: %+v", w)
		}
	}
	audit, _ := env.st.ListAudit(ctx, 10)
	var refused bool
	for _, a := range audit {
		if a.Action == JobType && a.Result == "refused" && a.Target == env.dev.ID {
			refused = true
		}
	}
	if !refused {
		t.Errorf("refusal not audited: %+v", audit)
	}

	// Binding a write credential to the device's group unblocks it.
	g := &model.Group{Name: "writable", Kind: "static"}
	if err := env.st.CreateGroup(ctx, g); err != nil {
		t.Fatal(err)
	}
	_ = env.st.SetGroupMembers(ctx, g.ID, []string{env.dev.ID})
	if err := env.st.CreateCredential(ctx, &model.Credential{Name: "rw", Type: model.CredSNMPCommunity, Scope: "group:" + g.ID, Purpose: model.PurposeBoth}, &model.Secret{Community: "private"}); err != nil {
		t.Fatal(err)
	}
	j, _ = env.svc.Apply(ctx, ApplyRequest{Changes: changes, Target: target}, "alice")
	if done := env.wait(t, j); done.Status != model.JobSucceeded {
		t.Fatalf("apply with a group write credential: %+v", done)
	}
	if n := len(env.fx.Sets()); n != 1 {
		t.Errorf("expected one set, got %d", n)
	}
}

// Audit A20: a device accepting a write is not the same as the value being in
// effect. The read-back decides: verified, diverged (with what the device
// reports) or unverifiable (no reader for the key).
func TestReadBackOutcomes(t *testing.T) {
	env := newTestEnv(t)
	ctx := context.Background()
	fake := &fakeConfigDriver{
		doc:    map[string]any{"identity.location": "Old", "identity.contact": "nobody", "print.defaultCopies": 1},
		stick:  map[string]bool{"identity.contact": true},
		hidden: map[string]bool{"print.defaultCopies": true},
	}
	env.svc.Engine.Registry.Register(fake)
	dev, _, err := env.st.UpsertDevice(ctx, &model.Identity{Address: "10.0.0.9", Serial: "SN-9", Model: "Fake", Method: "fake"},
		map[model.Capability]string{model.CapReadConfig: "fake", model.CapWriteConfig: "fake"}, []string{"test"})
	if err != nil {
		t.Fatal(err)
	}
	changes := []model.SettingChange{
		{Key: "identity.location", Value: "Lobby"},
		{Key: "identity.contact", Value: "ops@example.test"},
		{Key: "print.defaultCopies", Value: 2},
	}
	j, err := env.svc.Apply(ctx, ApplyRequest{Changes: changes, Target: model.ConfigTarget{DeviceIDs: []string{dev.ID}}}, "alice")
	if err != nil {
		t.Fatal(err)
	}
	done := env.wait(t, j)
	if done.Status != model.JobSucceeded {
		t.Fatalf("apply = %+v", done)
	}
	writes, _ := env.st.ListWriteRecords(ctx, store.WriteRecordFilter{DeviceID: dev.ID})
	if len(writes) != 1 {
		t.Fatalf("writes = %+v", writes)
	}
	rec := writes[0]
	if rec.Written != 3 || rec.Verified != 1 || rec.Diverged != 1 {
		t.Errorf("counts written/verified/diverged = %d/%d/%d", rec.Written, rec.Verified, rec.Diverged)
	}
	got := map[string]model.SettingResult{}
	for _, r := range rec.Result.Results {
		got[r.Key] = r
	}
	if r := got["identity.location"]; r.Outcome != model.OutcomeVerified || r.ReadBack != nil {
		t.Errorf("location = %+v", r)
	}
	if r := got["identity.contact"]; r.Status != model.SettingWritten || r.Outcome != model.OutcomeDiverged || r.ReadBack != "nobody" {
		t.Errorf("contact must be written but diverged with the read-back value: %+v", r)
	}
	if r := got["print.defaultCopies"]; r.Outcome != model.OutcomeUnverifiable {
		t.Errorf("copies (no reader) = %+v", r)
	}
	results, _ := env.st.ListJobResults(ctx, j.ID)
	if len(results) != 1 || !strings.Contains(results[0].Message, "1 verified, 1 diverged") {
		t.Errorf("job result = %+v", results)
	}
	audit, _ := env.st.ListAudit(ctx, 5)
	if len(audit) == 0 || audit[0].Action != JobType || audit[0].Result != "diverged" {
		t.Fatalf("audit = %+v", audit)
	}
	if oc, _ := audit[0].After["outcomes"].(map[string]any); oc["identity.contact"] != "diverged" || oc["identity.location"] != "verified" {
		t.Errorf("audit outcomes = %v", audit[0].After["outcomes"])
	}

	// A secret key is accepted but its value is never compared or stored.
	fake.mu.Lock()
	fake.doc["protocols.snmp.readCommunity"] = "public"
	fake.mu.Unlock()
	j, _ = env.svc.Apply(ctx, ApplyRequest{Changes: []model.SettingChange{{Key: "protocols.snmp.readCommunity", Value: "s3cretNew"}}, Target: model.ConfigTarget{DeviceIDs: []string{dev.ID}}}, "alice")
	env.wait(t, j)
	writes, _ = env.st.ListWriteRecords(ctx, store.WriteRecordFilter{DeviceID: dev.ID, Limit: 1})
	if b, _ := json.Marshal(writes); strings.Contains(string(b), "s3cretNew") {
		t.Errorf("secret in write record: %s", b)
	}
	if r := writes[0].Result.Results[0]; r.Outcome != model.OutcomeUnverifiable || r.ReadBack != nil {
		t.Errorf("secret outcome = %+v", r)
	}
}
