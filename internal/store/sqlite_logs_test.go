package store

import (
	"context"
	"strconv"
	"testing"
	"time"

	"printmarshal/internal/model"
)

// walk pages through a sync collection with the given page size and returns
// every row in order, checking hasMore along the way.
func walk(t *testing.T, fetch func(ctx context.Context, after SyncCursor, limit int) ([]SyncRow, bool, error), limit int) []SyncRow {
	t.Helper()
	var all []SyncRow
	var cur SyncCursor
	for i := 0; i < 100; i++ {
		rows, more, err := fetch(context.Background(), cur, limit)
		if err != nil {
			t.Fatal(err)
		}
		all = append(all, rows...)
		if len(rows) == 0 {
			if more {
				t.Fatal("hasMore with an empty page")
			}
			return all
		}
		cur = rows[len(rows)-1].Cursor
		if !more {
			return all
		}
	}
	t.Fatal("walk did not terminate")
	return nil
}

func TestSyncDevicesWithTombstones(t *testing.T) {
	s := openTest(t)
	ctx := context.Background()
	var ids []string
	for i := 0; i < 5; i++ {
		d, _, err := s.UpsertDevice(ctx, &model.Identity{Address: "10.0.0." + strconv.Itoa(i+1), Serial: "SN" + strconv.Itoa(i), Model: "Acme", Method: "snmp"}, nil, nil)
		if err != nil {
			t.Fatal(err)
		}
		ids = append(ids, d.ID)
	}
	rows := walk(t, s.SyncDevices, 2)
	if len(rows) != 5 {
		t.Fatalf("walk returned %d rows, want 5", len(rows))
	}
	seen := map[string]bool{}
	var last SyncCursor
	for _, r := range rows {
		d := r.Item.(*model.Device)
		if seen[d.ID] {
			t.Errorf("device %s returned twice", d.ID)
		}
		seen[d.ID] = true
		if r.Cursor.At < last.At || (r.Cursor.At == last.At && r.Cursor.ID <= last.ID) {
			t.Errorf("rows out of order: %+v after %+v", r.Cursor, last)
		}
		last = r.Cursor
	}
	// Nothing after the last cursor.
	if more, _, _ := s.SyncDevices(ctx, last, 10); len(more) != 0 {
		t.Errorf("expected an empty page after the last cursor, got %d", len(more))
	}
	// A later change moves the device after the cursor; a deletion yields a tombstone.
	name := "Lobby"
	time.Sleep(2 * time.Millisecond)
	if _, err := s.PatchDevice(ctx, ids[0], DevicePatch{DisplayName: &name}); err != nil {
		t.Fatal(err)
	}
	if err := s.DeleteDevice(ctx, ids[3]); err != nil {
		t.Fatal(err)
	}
	if err := s.AddTombstone(ctx, "devices", ids[3]); err != nil {
		t.Fatal(err)
	}
	page, more, err := s.SyncDevices(ctx, last, 10)
	if err != nil || more || len(page) != 2 {
		t.Fatalf("page after changes: %d rows more=%v err=%v", len(page), more, err)
	}
	if d, ok := page[0].Item.(*model.Device); !ok || d.ID != ids[0] || d.DisplayName != "Lobby" {
		t.Errorf("first item should be the patched device, got %#v", page[0].Item)
	}
	if ts, ok := page[1].Item.(*Tombstone); !ok || ts.ID != ids[3] || !ts.Deleted {
		t.Errorf("second item should be the tombstone, got %#v", page[1].Item)
	}
	// A full walk now returns 4 live devices and 1 tombstone.
	all := walk(t, s.SyncDevices, 3)
	live, dead := 0, 0
	for _, r := range all {
		switch r.Item.(type) {
		case *model.Device:
			live++
		case *Tombstone:
			dead++
		}
	}
	if live != 4 || dead != 1 {
		t.Errorf("live=%d dead=%d", live, dead)
	}
}

func TestSyncAuditAlertsJobs(t *testing.T) {
	s := openTest(t)
	ctx := context.Background()
	for i := 0; i < 5; i++ {
		if err := s.AddAudit(ctx, &model.AuditEntry{Actor: "kim", Action: "x", Target: strconv.Itoa(i), Result: "ok"}); err != nil {
			t.Fatal(err)
		}
	}
	rows := walk(t, s.SyncAudit, 2)
	if len(rows) != 5 {
		t.Fatalf("audit walk = %d", len(rows))
	}
	for i, r := range rows {
		if e := r.Item.(*model.AuditEntry); e.Target != strconv.Itoa(i) {
			t.Errorf("audit row %d = %s", i, e.Target)
		}
	}
	d, _, _ := s.UpsertDevice(ctx, &model.Identity{Address: "10.0.0.9", Serial: "Z", Model: "Acme", Method: "snmp"}, nil, nil)
	for i := 0; i < 3; i++ {
		if _, _, err := s.RaiseAlert(ctx, d.ID, "k"+strconv.Itoa(i), "poll", model.SeverityWarning, "w"); err != nil {
			t.Fatal(err)
		}
	}
	if len(walk(t, s.SyncAlerts, 2)) != 3 {
		t.Error("alerts walk")
	}
	for i := 0; i < 3; i++ {
		if err := s.CreateJob(ctx, &model.Job{Type: "poll", CreatedBy: "t"}); err != nil {
			t.Fatal(err)
		}
	}
	jobs := walk(t, s.SyncJobs, 2)
	if len(jobs) != 3 {
		t.Fatalf("jobs walk = %d", len(jobs))
	}
	// Finishing a job moves it after every cursor seen so far.
	last := jobs[len(jobs)-1].Cursor
	j := jobs[0].Item.(*model.Job)
	now := time.Now().UTC()
	j.Status, j.FinishedAt = model.JobSucceeded, &now
	if err := s.UpdateJob(ctx, j); err != nil {
		t.Fatal(err)
	}
	page, _, err := s.SyncJobs(ctx, last, 10)
	if err != nil || len(page) != 1 || page[0].Item.(*model.Job).ID != j.ID {
		t.Errorf("finished job not resurfaced: %d err=%v", len(page), err)
	}
	if rows, _, err := s.SyncConfigWrites(ctx, SyncCursor{}, 10); err != nil || len(rows) != 0 {
		t.Errorf("config writes: %d err=%v", len(rows), err)
	}
}

func TestDeviceLogs(t *testing.T) {
	s := openTest(t)
	ctx := context.Background()
	for i := 0; i < 3; i++ {
		if err := s.AddDeviceLog(ctx, &model.DeviceLog{DeviceID: "d1", At: time.Now().Add(time.Duration(-i) * time.Hour), Facility: 4, Severity: 2, Message: "m" + strconv.Itoa(i)}); err != nil {
			t.Fatal(err)
		}
	}
	rows, err := s.ListDeviceLogs(ctx, "d1", 2)
	if err != nil || len(rows) != 2 || rows[0].Message != "m2" {
		t.Fatalf("list = %d err=%v", len(rows), err)
	}
	if n, err := s.PruneDeviceLogs(ctx, time.Now().Add(-90*time.Minute)); err != nil || n != 1 {
		t.Errorf("prune = %d err=%v", n, err)
	}
}
