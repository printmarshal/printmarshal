package configmgmt

import (
	"context"
	"testing"
	"time"

	"printmarshal/internal/engine"
	"printmarshal/internal/model"
	"printmarshal/internal/store"
)

// richDriver stands in for IPP or a vendor plugin: it outranks SNMP for
// read_config/write_config, reports identity.hostname read-only and a key SNMP
// does not know, and skips every write.
type richDriver struct{ writes int }

func (d *richDriver) Name() string { return "rich" }
func (d *richDriver) Capabilities() []model.Capability {
	return []model.Capability{model.CapIdentify, model.CapReadConfig, model.CapWriteConfig}
}
func (d *richDriver) Supports(dev *model.Device, cap model.Capability) bool {
	return engine.ProfileSupports(dev, "rich", cap)
}
func (d *richDriver) Priority(model.Capability) int { return 90 }
func (d *richDriver) Identify(context.Context, string, model.Creds) (*model.Identity, error) {
	return &model.Identity{Method: "rich", Capabilities: []model.Capability{model.CapIdentify, model.CapReadConfig, model.CapWriteConfig}}, nil
}
func (d *richDriver) ReadConfig(context.Context, *model.Device, model.Creds, model.ConfigSelector) (*model.ConfigDocument, error) {
	return &model.ConfigDocument{ReadAt: time.Now(), Settings: []model.Setting{
		{Key: "identity.hostname", Area: "identity", Value: "RICH-NAME", Writable: false, ReadOnlyReason: "vendor says no"},
		{Key: "print.defaultCopies", Area: "print", Value: 3, Writable: false, ReadOnlyReason: "read-only here"},
	}}, nil
}
func (d *richDriver) WriteConfig(_ context.Context, dev *model.Device, _ model.Creds, changes []model.SettingChange, _ model.WriteOptions) (*model.WriteResult, error) {
	d.writes++
	res := &model.WriteResult{DeviceID: dev.ID, At: time.Now()}
	for _, ch := range changes {
		res.Results = append(res.Results, model.SettingResult{Key: ch.Key, Status: model.SettingSkipped, Error: "rich cannot write this", Driver: "rich"})
	}
	return res, nil
}

// Merged reads take the best driver's value per key, keep SNMP-only keys, and
// mark a key writable when a lower driver can write it; writes fall through
// from the winner to the next driver and combine the skip reasons.
func TestMergeAcrossDriversAndWriteFallback(t *testing.T) {
	env := newTestEnv(t)
	ctx := context.Background()
	rich := &richDriver{}
	env.svc.Engine.Registry.Register(rich)
	ident, caps, err := env.svc.Engine.Identify(ctx, "10.0.0.5", model.Creds{})
	if err != nil {
		t.Fatal(err)
	}
	dev, _, err := env.st.UpsertDevice(ctx, ident, caps, []string{"scan"})
	if err != nil {
		t.Fatal(err)
	}
	if dev.Capabilities[model.CapReadConfig] != "rich" {
		t.Fatalf("profile = %v", dev.Capabilities)
	}
	doc, err := env.svc.ReadConfig(ctx, dev.ID, model.ConfigSelector{})
	if err != nil {
		t.Fatal(err)
	}
	if len(doc.Sources) != 2 || doc.Sources[0] != "rich" || doc.Sources[1] != "snmp" {
		t.Errorf("sources = %v", doc.Sources)
	}
	host, _ := doc.Get("identity.hostname")
	if host.Value != "RICH-NAME" || host.Source != "rich" || !host.Writable || host.ReadOnlyReason != "" {
		t.Errorf("hostname should keep rich's value but become writable via snmp: %+v", host)
	}
	if loc, ok := doc.Get("identity.location"); !ok || loc.Source != "snmp" {
		t.Errorf("snmp-only key missing: %+v", loc)
	}
	if c, _ := doc.Get("print.defaultCopies"); c.Writable {
		t.Errorf("nobody can write copies: %+v", c)
	}

	j, err := env.svc.Apply(ctx, ApplyRequest{Changes: []model.SettingChange{
		{Key: "identity.location", Value: "Mezzanine"},
		{Key: "print.defaultCopies", Value: 2},
	}, Target: model.ConfigTarget{DeviceIDs: []string{dev.ID}}}, "alice")
	if err != nil {
		t.Fatal(err)
	}
	if done := env.wait(t, j); done.Status != model.JobSucceeded {
		t.Fatalf("job = %+v", done)
	}
	if rich.writes != 1 {
		t.Errorf("the winning driver must be asked first (writes=%d)", rich.writes)
	}
	writes, _ := env.st.ListWriteRecords(ctx, store.WriteRecordFilter{DeviceID: dev.ID})
	if len(writes) != 1 {
		t.Fatalf("writes = %d", len(writes))
	}
	got := map[string]model.SettingResult{}
	for _, r := range writes[0].Result.Results {
		got[r.Key] = r
	}
	if got["identity.location"].Status != model.SettingWritten || got["identity.location"].Driver != "snmp" {
		t.Errorf("location should fall through to snmp: %+v", got["identity.location"])
	}
	if r := got["print.defaultCopies"]; r.Status != model.SettingSkipped || r.Error != "rich: rich cannot write this; snmp: not writable over SNMP" {
		t.Errorf("copies = %+v", r)
	}
}
