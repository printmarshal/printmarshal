package store

import (
	"context"
	"testing"
	"time"

	"printmarshal/internal/model"
)

func TestConfigStoreRoundTrip(t *testing.T) {
	s := openTest(t)
	ctx := context.Background()

	p := &model.Profile{Name: "site", Settings: []model.SettingChange{{Key: "identity.location", Value: "Lobby"}}, CreatedBy: "a"}
	if err := s.CreateProfile(ctx, p); err != nil {
		t.Fatal(err)
	}
	if err := s.CreateProfile(ctx, &model.Profile{Name: "site"}); err == nil {
		t.Error("duplicate profile name accepted")
	}
	p.Description = "d"
	if err := s.UpdateProfile(ctx, p); err != nil {
		t.Fatal(err)
	}
	got, err := s.GetProfile(ctx, p.ID)
	if err != nil || got.Description != "d" || len(got.Settings) != 1 || got.Settings[0].Value != "Lobby" {
		t.Fatalf("profile = %+v err=%v", got, err)
	}

	doc := &model.ConfigDocument{DeviceID: "dev", Settings: []model.Setting{{Key: "identity.location", Value: "A", Source: "snmp"}}, ReadAt: time.Now(), Sources: []string{"snmp"}}
	for i := 0; i < 3; i++ {
		doc.Settings[0].Value = i
		if err := s.SaveConfigSnapshot(ctx, &model.ConfigSnapshot{DeviceID: "dev", Document: doc, At: time.Now().Add(time.Duration(i) * time.Second)}); err != nil {
			t.Fatal(err)
		}
	}
	latest, err := s.LatestConfigSnapshot(ctx, "dev")
	if err != nil || !model.ValuesEqual(latest.Document.Settings[0].Value, 2) {
		t.Fatalf("latest = %+v err=%v", latest, err)
	}
	if n, _ := s.PruneConfigSnapshots(ctx, "dev", 2); n != 1 {
		t.Errorf("pruned %d", n)
	}
	if list, _ := s.ListConfigSnapshots(ctx, "dev", 0); len(list) != 2 {
		t.Errorf("snapshots = %d", len(list))
	}
	if _, err := s.LatestConfigSnapshot(ctx, "none"); err != ErrNotFound {
		t.Errorf("missing snapshot err = %v", err)
	}

	w := &model.WriteRecord{JobID: "j1", ProfileID: p.ID, Actor: "a", Changes: []model.SettingChange{{Key: "identity.location", Value: "Lobby"}},
		Result: model.WriteResult{DeviceID: "dev", Results: []model.SettingResult{
			{Key: "identity.location", Status: model.SettingWritten, Before: "A", After: "Lobby"},
			{Key: "x", Status: model.SettingSkipped},
		}, Rollback: []model.SettingChange{{Key: "identity.location", Value: "A"}}}}
	if err := s.SaveWriteRecord(ctx, w); err != nil {
		t.Fatal(err)
	}
	gw, err := s.GetWriteRecord(ctx, w.ID)
	if err != nil || gw.Written != 1 || gw.Skipped != 1 || !gw.Reversible() || gw.Result.DeviceID != "dev" || gw.Result.Rollback[0].Value != "A" {
		t.Fatalf("write record = %+v err=%v", gw, err)
	}
	if list, _ := s.ListWriteRecords(ctx, WriteRecordFilter{ProfileID: p.ID}); len(list) != 1 {
		t.Errorf("writes by profile = %d", len(list))
	}

	r := &model.DriftReport{DeviceID: "dev", ProfileID: p.ID, Items: []model.DriftItem{{Key: "identity.location", Desired: "Lobby", Current: "A"}}, Drifted: 1}
	if err := s.SaveDriftReport(ctx, r); err != nil {
		t.Fatal(err)
	}
	if err := s.SaveDriftReport(ctx, r); err != nil {
		t.Fatal(err)
	}
	if list, _ := s.ListDriftReports(ctx, p.ID, 0); len(list) != 1 || list[0].Drifted != 1 {
		t.Errorf("drift = %+v", list)
	}
	if err := s.DeleteProfile(ctx, p.ID); err != nil {
		t.Fatal(err)
	}
	if list, _ := s.ListDriftReports(ctx, p.ID, 0); len(list) != 0 {
		t.Error("drift reports must go with the profile")
	}
}
