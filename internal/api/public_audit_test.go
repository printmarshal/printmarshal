package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync"
	"testing"

	"printmarshal/internal/events"
	"printmarshal/internal/model"
)

// auditRows reads the audit trail through the route an operator uses and
// returns the rows for one action, oldest first, plus the raw response.
func auditRows(t *testing.T, ts *httptest.Server, token, action string) ([]model.AuditEntry, string) {
	t.Helper()
	code, body := do(t, ts, token, "GET", "/api/audit?limit=300", "")
	if code != 200 {
		t.Fatalf("audit = %d %s", code, body)
	}
	var all []model.AuditEntry
	if err := json.Unmarshal([]byte(body), &all); err != nil {
		t.Fatalf("audit is not JSON: %v: %s", err, body)
	}
	var out []model.AuditEntry
	for i := len(all) - 1; i >= 0; i-- { // the route answers newest first
		if all[i].Action == action {
			out = append(out, all[i])
		}
	}
	return out, body
}

// wantField compares one recorded value on one side of an audit entry.
func wantField(t *testing.T, side string, m map[string]any, key string, want any) {
	t.Helper()
	got, ok := m[key]
	if !ok {
		t.Errorf("%s has no %q: %v", side, key, m)
		return
	}
	gb, _ := json.Marshal(got)
	wb, _ := json.Marshal(want)
	if !reflect.DeepEqual(gb, wb) {
		t.Errorf("%s[%q] = %s, want %s", side, key, gb, wb)
	}
}

// assertNoSlug fails if any part of the entry carries a slug value or a slug
// field. slugRegenerated is the one permitted key, and it is a boolean.
func assertNoSlug(t *testing.T, e model.AuditEntry, slugs ...string) {
	t.Helper()
	for _, side := range []struct {
		name string
		m    map[string]any
	}{{"before", e.Before}, {"after", e.After}} {
		if _, ok := side.m["slug"]; ok {
			t.Errorf("%s has a slug key: %v", side.name, side.m)
		}
		if v, ok := side.m["slugRegenerated"]; ok {
			if _, isBool := v.(bool); !isBool {
				t.Errorf("%s slugRegenerated = %v, want a boolean", side.name, v)
			}
		}
	}
	raw, err := json.Marshal(e)
	if err != nil {
		t.Fatal(err)
	}
	for _, slug := range slugs {
		if slug == "" {
			t.Fatal("assertNoSlug needs the real slug")
		}
		for what, text := range map[string]string{"entry": string(raw), "target": e.Target} {
			if strings.Contains(text, slug) {
				t.Errorf("slug %q appears in the %s: %s", slug, what, text)
			}
		}
	}
}

// boardSlug reads the current slug through the admin settings route.
func boardSlug(t *testing.T, ts *httptest.Server, admin string) string {
	t.Helper()
	code, body := do(t, ts, admin, "GET", "/api/settings/public-status", "")
	if code != 200 {
		t.Fatalf("settings = %d %s", code, body)
	}
	var cfg struct {
		Slug string `json:"slug"`
	}
	if err := json.Unmarshal([]byte(body), &cfg); err != nil {
		t.Fatal(err)
	}
	return cfg.Slug
}

// Issue 72: a saved change is reviewable in the audit trail, field by field,
// and the slug is in none of it, because operators read this log.
func TestPublicStatusAuditRecordsChanges(t *testing.T) {
	ts, s := newTestServer(t)
	admin := login(t, ts, "admin", "adminpass123")
	ctx := context.Background()
	if _, err := s.Auth.CreateUser(ctx, "op", "operatorpass1", model.RoleOperator); err != nil {
		t.Fatal(err)
	}
	operator := login(t, ts, "op", "operatorpass1")

	a := seedBoardDevice(t, s.Store, 1, "A")
	b := seedBoardDevice(t, s.Store, 2, "B")

	// First save: the before side is the default configuration.
	first := map[string]any{
		"enabled": false, "unlisted": false, "title": "Library printers",
		"frameOrigins": []string{"https://intranet.example"},
		"devices":      []map[string]any{{"id": a.ID, "label": "Ground floor"}, {"id": b.ID}},
		"show":         map[string]any{"location": true, "online": true},
	}
	if code, body := putBoard(t, ts, admin, first); code != 200 {
		t.Fatalf("first save: %d %s", code, body)
	}
	rows, _ := auditRows(t, ts, operator, "public.status.changed")
	if len(rows) != 1 {
		t.Fatalf("rows after one save = %d, want 1: %+v", len(rows), rows)
	}
	e := rows[0]
	if e.Actor != "admin" || e.Result != "ok" {
		t.Errorf("actor, result = %q, %q", e.Actor, e.Result)
	}
	if e.Before == nil || e.After == nil {
		t.Fatalf("entry has no before and after: %+v", e)
	}
	wantField(t, "before", e.Before, "title", publicDefaultTitle)
	wantField(t, "after", e.After, "title", "Library printers")
	wantField(t, "before", e.Before, "frameOrigins", []string{})
	wantField(t, "after", e.After, "frameOrigins", []string{"https://intranet.example"})
	wantField(t, "before", e.Before, "deviceIds", []string{})
	wantField(t, "after", e.After, "deviceIds", []string{a.ID, b.ID})
	wantField(t, "before", e.Before, "deviceLabels", map[string]string{})
	wantField(t, "after", e.After, "deviceLabels", map[string]string{a.ID: "Ground floor"})
	wantField(t, "before", e.Before, "show.online", false)
	wantField(t, "after", e.After, "show.online", true)
	wantField(t, "after", e.After, "show.location", true)
	wantField(t, "after", e.After, "show.paper", false)
	wantField(t, "after", e.After, "enabled", false)
	wantField(t, "after", e.After, "unlisted", false)
	wantField(t, "after", e.After, "slugRegenerated", false)

	// Second save: reorder, relabel, publish, unlist. The first slug is issued
	// here, and it must not be recorded even though this save created it.
	second := map[string]any{
		"enabled": true, "unlisted": true, "title": "Library printers",
		"frameOrigins": []string{},
		"devices":      []map[string]any{{"id": b.ID, "label": "First floor"}, {"id": a.ID}},
		"show":         map[string]any{"location": true, "toner": true},
	}
	if code, body := putBoard(t, ts, admin, second); code != 200 {
		t.Fatalf("second save: %d %s", code, body)
	}
	slug1 := boardSlug(t, ts, admin)
	if slug1 == "" {
		t.Fatal("unlisting issued no slug")
	}
	rows, raw := auditRows(t, ts, operator, "public.status.changed")
	if len(rows) != 2 {
		t.Fatalf("rows after two saves = %d, want 2", len(rows))
	}
	e = rows[1]
	wantField(t, "before", e.Before, "enabled", false)
	wantField(t, "after", e.After, "enabled", true)
	wantField(t, "before", e.Before, "unlisted", false)
	wantField(t, "after", e.After, "unlisted", true)
	wantField(t, "before", e.Before, "deviceIds", []string{a.ID, b.ID})
	wantField(t, "after", e.After, "deviceIds", []string{b.ID, a.ID})
	wantField(t, "before", e.Before, "deviceLabels", map[string]string{a.ID: "Ground floor"})
	wantField(t, "after", e.After, "deviceLabels", map[string]string{b.ID: "First floor"})
	wantField(t, "before", e.Before, "frameOrigins", []string{"https://intranet.example"})
	wantField(t, "after", e.After, "frameOrigins", []string{})
	wantField(t, "before", e.Before, "show.online", true)
	wantField(t, "after", e.After, "show.online", false)
	wantField(t, "after", e.After, "show.toner", true)
	wantField(t, "before", e.Before, "slugRegenerated", false)
	wantField(t, "after", e.After, "slugRegenerated", true)
	assertNoSlug(t, e, slug1)
	if strings.Contains(raw, slug1) {
		t.Error("the slug appears in the audit response")
	}

	// Publishing keeps its own row, as the spec's R9 asks: one, not a copy.
	enabled, _ := auditRows(t, ts, operator, "public.status.enabled")
	if len(enabled) != 1 {
		t.Fatalf("public.status.enabled rows = %d, want 1", len(enabled))
	}
	wantField(t, "before", enabled[0].Before, "enabled", false)
	wantField(t, "after", enabled[0].After, "enabled", true)
	assertNoSlug(t, enabled[0], slug1)

	// Regenerating: neither the old slug nor the new one is recorded.
	second["regenerateSlug"] = true
	if code, body := putBoard(t, ts, admin, second); code != 200 {
		t.Fatalf("regenerate: %d %s", code, body)
	}
	slug2 := boardSlug(t, ts, admin)
	if slug2 == "" || slug2 == slug1 {
		t.Fatalf("slug after regenerate = %q (was %q)", slug2, slug1)
	}
	rows, raw = auditRows(t, ts, operator, "public.status.changed")
	if len(rows) != 3 {
		t.Fatalf("rows after three saves = %d, want 3", len(rows))
	}
	e = rows[2]
	wantField(t, "before", e.Before, "slugRegenerated", false)
	wantField(t, "after", e.After, "slugRegenerated", true)
	assertNoSlug(t, e, slug1, slug2)
	for _, slug := range []string{slug1, slug2} {
		if strings.Contains(raw, slug) {
			t.Errorf("slug %q appears in the audit response", slug)
		}
	}

	// A save that changes nothing still leaves its row, with equal sides, so
	// the trail says so instead of showing an empty change.
	delete(second, "regenerateSlug")
	if code, body := putBoard(t, ts, admin, second); code != 200 {
		t.Fatalf("no-op save: %d %s", code, body)
	}
	rows, _ = auditRows(t, ts, operator, "public.status.changed")
	if len(rows) != 4 {
		t.Fatalf("rows after four saves = %d, want 4", len(rows))
	}
	e = rows[3]
	if !reflect.DeepEqual(e.Before, e.After) {
		t.Errorf("no-op save differs:\nbefore %v\nafter  %v", e.Before, e.After)
	}
	assertNoSlug(t, e, slug1, slug2)
}

// The security event and its export keep their shape: one event per save,
// with the same keys as every other security event and no slug and no
// configuration values added to what leaves the system.
func TestPublicStatusSecurityEventUnchanged(t *testing.T) {
	ts, s := newTestServer(t)
	var mu sync.Mutex
	var seen []events.Envelope
	bus := events.NewBus()
	bus.Subscribe(events.SubscriberFunc(func(env events.Envelope) {
		mu.Lock()
		defer mu.Unlock()
		if strings.HasPrefix(env.Type, "public.status.") {
			seen = append(seen, env)
		}
	}))
	s.Bus = bus
	admin := login(t, ts, "admin", "adminpass123")
	d := seedBoardDevice(t, s.Store, 1, "A")
	cfg := map[string]any{"enabled": true, "unlisted": true, "regenerateSlug": true, "title": "Secret-free title", "devices": []map[string]any{{"id": d.ID}}, "show": map[string]any{"online": true}}
	if code, body := putBoard(t, ts, admin, cfg); code != 200 {
		t.Fatalf("save: %d %s", code, body)
	}
	slug := boardSlug(t, ts, admin)
	mu.Lock()
	defer mu.Unlock()
	var types []string
	for _, ev := range seen {
		types = append(types, ev.Type)
		data, ok := ev.Data.(map[string]any)
		if !ok {
			t.Fatalf("%s data = %T", ev.Type, ev.Data)
		}
		wantKeys(t, ev.Type+" event data", data, "user", "detail", "result", "ip", "summary")
		if b := fmt.Sprint(data); strings.Contains(b, slug) {
			t.Errorf("%s event carries the slug: %s", ev.Type, b)
		}
		if !strings.Contains(fmt.Sprint(data["detail"]), "slugRegenerated=true") {
			t.Errorf("%s detail lost slugRegenerated: %v", ev.Type, data["detail"])
		}
	}
	if want := []string{"public.status.changed", "public.status.enabled"}; !reflect.DeepEqual(types, want) {
		t.Errorf("events = %v, want %v", types, want)
	}
}
