package store

import (
	"context"
	"strings"
	"testing"
	"time"

	"printmarshal/internal/model"
)

// Users, sessions, the notification outbox, logs and the audit trail: the
// parts of the base interface the older tests did not touch.
func testUsersSessionsOutboxAudit(t *testing.T, s testStore) {
	ctx := context.Background()
	u := &model.User{Username: "alice", Role: model.RoleOperator}
	if err := s.CreateUser(ctx, u, "hash1"); err != nil {
		t.Fatal(err)
	}
	if err := s.CreateUser(ctx, &model.User{Username: "alice", Role: model.RoleViewer}, "x"); err == nil {
		t.Error("duplicate username accepted")
	}
	if got, hash, err := s.GetUserByName(ctx, "alice"); err != nil || hash != "hash1" || got.ID != u.ID || got.Role != model.RoleOperator {
		t.Fatalf("by name: %+v %q %v", got, hash, err)
	}
	if n, _ := s.CountUsers(ctx); n != 1 {
		t.Errorf("count = %d", n)
	}
	if err := s.SetPassword(ctx, u.ID, "hash2"); err != nil {
		t.Fatal(err)
	}
	if err := s.SetPassword(ctx, "nope", "h"); err != ErrNotFound {
		t.Errorf("set password unknown: %v", err)
	}
	if err := s.CreateSession(ctx, "tok1", u.ID, time.Now().Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	if err := s.CreateSession(ctx, "tok2", u.ID, time.Now().Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	if err := s.CreateSession(ctx, "old", u.ID, time.Now().Add(-time.Hour)); err != nil {
		t.Fatal(err)
	}
	if got, err := s.GetSession(ctx, "tok1"); err != nil || got.Username != "alice" {
		t.Errorf("session: %+v %v", got, err)
	}
	if _, err := s.GetSession(ctx, "old"); err != ErrNotFound {
		t.Errorf("expired session: %v", err)
	}
	if n, err := s.DeleteUserSessions(ctx, u.ID, "tok1"); err != nil || n != 1 {
		t.Errorf("revoke others: %d %v", n, err)
	}
	if _, err := s.GetSession(ctx, "tok2"); err != ErrNotFound {
		t.Errorf("revoked session still valid: %v", err)
	}
	if err := s.DeleteSession(ctx, "tok1"); err != nil {
		t.Fatal(err)
	}
	if list, _ := s.ListUsers(ctx); len(list) != 1 || list[0].CreatedAt.IsZero() {
		t.Errorf("users = %+v", list)
	}
	if err := s.DeleteUser(ctx, u.ID); err != nil {
		t.Fatal(err)
	}
	if err := s.DeleteUser(ctx, u.ID); err != ErrNotFound {
		t.Errorf("double delete: %v", err)
	}

	// Outbox: de-duplicated per (event, notifier), due rows in next_at order, dead rows requeue.
	o1 := &model.Outbox{EventID: "e1", Event: "alert.raised", Notifier: "slack", Subject: "s", Body: "b", Payload: map[string]any{"k": "v"}}
	if err := s.EnqueueNotification(ctx, o1); err != nil || o1.ID == 0 {
		t.Fatalf("enqueue: %d %v", o1.ID, err)
	}
	dup := &model.Outbox{EventID: "e1", Event: "alert.raised", Notifier: "slack"}
	if err := s.EnqueueNotification(ctx, dup); err != nil {
		t.Fatalf("duplicate enqueue must be ignored: %v", err)
	}
	later := &model.Outbox{EventID: "e2", Event: "alert.raised", Notifier: "slack", NextAt: time.Now().Add(time.Hour)}
	if err := s.EnqueueNotification(ctx, later); err != nil {
		t.Fatal(err)
	}
	due, err := s.DueNotifications(ctx, 10)
	if err != nil || len(due) != 1 || due[0].ID != o1.ID || due[0].Payload.(map[string]any)["k"] != "v" {
		t.Fatalf("due = %+v %v", due, err)
	}
	if all, _ := s.ListOutbox(ctx, "", 10); len(all) != 2 || all[0].ID != later.ID {
		t.Errorf("list all = %+v", all)
	}
	now := time.Now().UTC()
	o1.State, o1.Attempts, o1.LastError, o1.DoneAt = "dead", 3, "boom", &now
	if err := s.UpdateNotification(ctx, o1); err != nil {
		t.Fatal(err)
	}
	if dead, _ := s.ListOutbox(ctx, "dead", 10); len(dead) != 1 || dead[0].Attempts != 3 || dead[0].DoneAt == nil {
		t.Errorf("dead = %+v", dead)
	}
	o1.Subject = "edited"
	if err := s.UpdateNotificationText(ctx, o1); err != nil {
		t.Fatal(err)
	}
	if err := s.RequeueDead(ctx, o1.ID); err != nil {
		t.Fatal(err)
	}
	if err := s.RequeueDead(ctx, o1.ID); err != ErrNotFound {
		t.Errorf("requeue pending: %v", err)
	}
	if due, _ := s.DueNotifications(ctx, 10); len(due) != 1 || due[0].Subject != "edited" || due[0].Attempts != 0 || due[0].DoneAt != nil {
		t.Errorf("requeued = %+v", due)
	}

	// Notification log and audit trail, newest first.
	_ = s.AddNotificationLog(ctx, &model.NotificationLog{Notifier: "slack", Event: "alert.raised", OK: false, Error: "500"})
	_ = s.AddNotificationLog(ctx, &model.NotificationLog{Notifier: "slack", Event: "alert.raised", OK: true})
	if l, _ := s.ListNotificationLog(ctx, 0); len(l) != 2 || !l[0].OK || l[1].Error != "500" || l[0].At.IsZero() {
		t.Errorf("notification log = %+v", l)
	}
	e := &model.AuditEntry{Actor: "admin", Action: "device.patch", Target: "d1", Before: map[string]any{"a": 1.0}, After: map[string]any{"a": 2.0}, Result: "ok"}
	if err := s.AddAudit(ctx, e); err != nil || e.ID == 0 {
		t.Fatalf("audit: %d %v", e.ID, err)
	}
	_ = s.AddAudit(ctx, &model.AuditEntry{Actor: "admin", Action: "login", Result: "ok"})
	audit, _ := s.ListAudit(ctx, 1)
	if len(audit) != 1 || audit[0].Action != "login" || audit[0].Before != nil {
		t.Errorf("audit limit = %+v", audit)
	}
	audit, _ = s.ListAudit(ctx, 0)
	if len(audit) != 2 || audit[1].Before["a"] != 1.0 || audit[1].After["a"] != 2.0 {
		t.Errorf("audit = %+v", audit)
	}

	// Settings round-trip and job listing.
	if err := s.SetSetting(ctx, "k", map[string]any{"x": 1.0}); err != nil {
		t.Fatal(err)
	}
	var got map[string]any
	if err := s.GetSetting(ctx, "k", &got); err != nil || got["x"] != 1.0 {
		t.Errorf("setting = %+v %v", got, err)
	}
	if err := s.GetSetting(ctx, "missing", &got); err != ErrNotFound {
		t.Errorf("missing setting: %v", err)
	}
	j := &model.Job{Type: "discovery", Target: map[string]any{}, Params: map[string]any{}}
	_ = s.CreateJob(ctx, j)
	started := time.Now().UTC()
	j.Status, j.StartedAt, j.Total = model.JobRunning, &started, 3
	if err := s.UpdateJob(ctx, j); err != nil {
		t.Fatal(err)
	}
	if jobs, _ := s.ListJobs(ctx, JobFilter{Type: "discovery", Status: "running"}); len(jobs) != 1 || jobs[0].StartedAt == nil || jobs[0].Total != 3 {
		t.Errorf("jobs = %+v", jobs)
	}
	if _, err := s.GetJob(ctx, "nope"); err != ErrNotFound {
		t.Errorf("missing job: %v", err)
	}
}

// Secret settings are sealed with the master key: the plaintext round-trips,
// the stored value is not the plaintext, and clearing returns ErrNotFound.
func testSecretSettings(t *testing.T, s testStore) {
	ctx := context.Background()
	if _, err := s.GetSecretSetting(ctx, "auth.test.secret"); err != ErrNotFound {
		t.Fatalf("unset: %v", err)
	}
	if err := s.SetSecretSetting(ctx, "auth.test.secret", "hunter2"); err != nil {
		t.Fatal(err)
	}
	if v, err := s.GetSecretSetting(ctx, "auth.test.secret"); err != nil || v != "hunter2" {
		t.Fatalf("round trip: %q %v", v, err)
	}
	var raw map[string]string
	if err := s.GetSetting(ctx, "auth.test.secret", &raw); err != nil || raw["sealed"] == "" || strings.Contains(raw["sealed"], "hunter2") {
		t.Fatalf("stored form: %v %v", raw, err)
	}
	if err := s.SetSecretSetting(ctx, "auth.test.secret", ""); err != nil {
		t.Fatal(err)
	}
	if _, err := s.GetSecretSetting(ctx, "auth.test.secret"); err != ErrNotFound {
		t.Fatalf("cleared: %v", err)
	}
}
