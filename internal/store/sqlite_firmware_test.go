package store

import (
	"context"
	"testing"
	"time"

	"printmarshal/internal/model"
)

func TestFirmwareStoreRoundTrip(t *testing.T) {
	s := openTest(t)
	ctx := context.Background()

	p := &model.FirmwarePackage{Vendor: "Acme", Models: []string{"4200"}, Version: "2.15.0", MinVersion: "2.10", FileName: "fw.bin", Size: 12,
		SHA256: "abc", StoragePath: "/tmp/x/fw.bin", Notes: "n", CreatedBy: "admin"}
	if err := s.CreateFirmwarePackage(ctx, p); err != nil {
		t.Fatal(err)
	}
	got, err := s.GetFirmwarePackage(ctx, p.ID)
	if err != nil || got.Vendor != "Acme" || len(got.Models) != 1 || got.StoragePath != p.StoragePath || got.MinVersion != "2.10" {
		t.Fatalf("package %+v err=%v", got, err)
	}
	if list, _ := s.ListFirmwarePackages(ctx); len(list) != 1 {
		t.Fatalf("list = %d", len(list))
	}

	at := time.Date(2026, 11, 1, 22, 0, 0, 0, time.UTC)
	c := &model.Campaign{Name: "c", Description: "d", PackageID: p.ID, DeviceIDs: []string{"d1", "d2"}, CanaryCount: 1, Concurrency: 2,
		Window: &model.MaintenanceWindow{Days: []string{"sat"}, Start: "22:00", End: "06:00", TZ: "UTC"}, FailureThresholdPercent: 25,
		Status: model.CampaignDraft, ScheduledAt: &at, DryRun: true, AllowDowngrade: true, VerifyTimeoutSecs: 60, CreatedBy: "op"}
	if err := s.CreateCampaign(ctx, c); err != nil {
		t.Fatal(err)
	}
	gc, err := s.GetCampaign(ctx, c.ID)
	if err != nil || gc.Window == nil || gc.Window.Start != "22:00" || !gc.DryRun || !gc.AllowDowngrade || gc.VerifyTimeoutSecs != 60 ||
		gc.ScheduledAt == nil || !gc.ScheduledAt.Equal(at) || len(gc.DeviceIDs) != 2 || gc.Status != model.CampaignDraft {
		t.Fatalf("campaign %+v err=%v", gc, err)
	}
	gc.Status, gc.PauseReason, gc.Wave, gc.Confirmed, gc.JobID, gc.Window = model.CampaignPaused, model.PauseCanary, 1, true, "job1", nil
	if err := s.UpdateCampaign(ctx, gc); err != nil {
		t.Fatal(err)
	}
	gc, _ = s.GetCampaign(ctx, c.ID)
	if gc.Status != model.CampaignPaused || gc.PauseReason != model.PauseCanary || gc.Window != nil || !gc.Confirmed || gc.JobID != "job1" {
		t.Fatalf("updated %+v", gc)
	}
	if l, _ := s.ListCampaigns(ctx, CampaignFilter{Active: true}); len(l) != 1 {
		t.Errorf("active = %d", len(l))
	}
	if l, _ := s.ListCampaigns(ctx, CampaignFilter{Status: "completed"}); len(l) != 0 {
		t.Errorf("completed = %d", len(l))
	}
	if l, _ := s.ListCampaigns(ctx, CampaignFilter{PackageID: p.ID}); len(l) != 1 {
		t.Errorf("by package = %d", len(l))
	}
	if err := s.UpdateCampaign(ctx, &model.Campaign{ID: "nope"}); err != ErrNotFound {
		t.Errorf("update unknown: %v", err)
	}

	now := time.Now().UTC()
	rows := []*model.CampaignDevice{
		{CampaignID: c.ID, DeviceID: "d1", Wave: 1, Status: model.CampaignDeviceSucceeded, FromVersion: "2.14", ToVersion: "2.15.0", Message: "ok", StartedAt: &now, FinishedAt: &now},
		{CampaignID: c.ID, DeviceID: "d2", Wave: 2, Status: model.CampaignDevicePending, ToVersion: "2.15.0"},
	}
	for _, r := range rows {
		if err := s.UpsertCampaignDevice(ctx, r); err != nil {
			t.Fatal(err)
		}
	}
	rows[1].Status, rows[1].Message = model.CampaignDeviceFailed, "boom"
	if err := s.UpsertCampaignDevice(ctx, rows[1]); err != nil {
		t.Fatal(err)
	}
	devs, err := s.ListCampaignDevices(ctx, c.ID)
	if err != nil || len(devs) != 2 || devs[0].DeviceID != "d1" || devs[0].FinishedAt == nil || devs[1].Status != model.CampaignDeviceFailed || devs[1].Message != "boom" {
		t.Fatalf("devices %+v err=%v", devs, err)
	}
	hist, err := s.FirmwareHistory(ctx, "d2", 10)
	if err != nil || len(hist) != 1 || hist[0].CampaignName != "c" || hist[0].PackageID != p.ID || !hist[0].DryRun || hist[0].Status != model.CampaignDeviceFailed {
		t.Fatalf("history %+v err=%v", hist, err)
	}

	// Deleting the campaign cascades to its device rows; the package keeps its history semantics.
	if err := s.DeleteCampaign(ctx, c.ID); err != nil {
		t.Fatal(err)
	}
	if devs, _ := s.ListCampaignDevices(ctx, c.ID); len(devs) != 0 {
		t.Errorf("rows survived campaign delete: %d", len(devs))
	}
	if err := s.DeleteCampaign(ctx, c.ID); err != ErrNotFound {
		t.Errorf("delete twice: %v", err)
	}
	if err := s.DeleteFirmwarePackage(ctx, p.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.GetFirmwarePackage(ctx, p.ID); err != ErrNotFound {
		t.Errorf("after delete: %v", err)
	}
}
