package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"printmarshal/internal/model"
	"printmarshal/internal/store"
)

type tokenResp struct {
	Token  store.APIToken `json:"token"`
	Secret string         `json:"secret"`
}

func mkToken(t *testing.T, ts *httptest.Server, admin, body string) tokenResp {
	t.Helper()
	code, out := do(t, ts, admin, "POST", "/api/tokens", body)
	if code != 201 {
		t.Fatalf("create token: %d %s", code, out)
	}
	var tr tokenResp
	if err := json.Unmarshal([]byte(out), &tr); err != nil {
		t.Fatal(err)
	}
	return tr
}

// Audit A10: API tokens are admin-managed service accounts whose secret is
// shown once, that cannot touch /api/auth or mint tokens, and that stop
// working the moment they are revoked.
func TestAPITokenLifecycle(t *testing.T) {
	ts, s := newTestServer(t)
	admin := login(t, ts, "admin", "adminpass123")
	viewer := login(t, ts, "viewer", "viewerpass1")

	if code, _ := do(t, ts, viewer, "POST", "/api/tokens", `{"name":"x"}`); code != 403 {
		t.Fatalf("viewer created a token: %d", code)
	}
	if code, body := do(t, ts, admin, "POST", "/api/tokens", `{"name":"x","role":"root"}`); code != 400 {
		t.Fatalf("bad role accepted: %d %s", code, body)
	}
	if code, body := do(t, ts, admin, "POST", "/api/tokens", `{"name":"x","scope":["nope"]}`); code != 400 || !strings.Contains(body, "unknown group") {
		t.Fatalf("bad scope accepted: %d %s", code, body)
	}

	tr := mkToken(t, ts, admin, `{"name":"ci-bot","role":"operator","ttl":"1h"}`)
	if !strings.HasPrefix(tr.Secret, "pmt_") || len(tr.Secret) < 40 {
		t.Fatalf("secret %q", tr.Secret)
	}
	if tr.Token.Role != model.RoleOperator || tr.Token.ExpiresAt == nil || tr.Token.CreatedBy != "admin" {
		t.Fatalf("token %+v", tr.Token)
	}
	// The list never carries the secret.
	code, body := do(t, ts, admin, "GET", "/api/tokens", "")
	if code != 200 || strings.Contains(body, tr.Secret) || !strings.Contains(body, `"name":"ci-bot"`) {
		t.Fatalf("list: %d %s", code, body)
	}

	// The token authenticates as an operator principal.
	if code, body := do(t, ts, tr.Secret, "GET", "/api/devices", ""); code != 200 {
		t.Fatalf("token GET /api/devices: %d %s", code, body)
	}
	if code, _ := do(t, ts, tr.Secret, "POST", "/api/credentials", `{}`); code != 403 {
		t.Fatalf("operator token reached an admin route: %d", code)
	}
	// Tokens cannot manage authentication, even with the admin role.
	root := mkToken(t, ts, admin, `{"name":"root-bot","role":"admin"}`)
	for _, c := range []struct{ m, p string }{{"GET", "/api/auth/me"}, {"POST", "/api/auth/password"}, {"POST", "/api/tokens"}, {"DELETE", "/api/tokens/" + tr.Token.ID}} {
		if code, body := do(t, ts, root.Secret, c.m, c.p, `{}`); code != 403 || !strings.Contains(body, "cannot manage authentication") {
			t.Fatalf("token %s %s: %d %s", c.m, c.p, code, body)
		}
	}
	if code, _ := do(t, ts, root.Secret, "GET", "/api/tokens", ""); code != 200 {
		t.Fatalf("admin token cannot list tokens: %d", code)
	}
	// A pmt_ secret in the session cookie is not accepted.
	req, _ := http.NewRequest("GET", ts.URL+"/api/devices", nil)
	req.AddCookie(&http.Cookie{Name: "pm_session", Value: tr.Secret})
	if resp, err := http.DefaultClient.Do(req); err != nil || resp.StatusCode != 401 {
		t.Fatalf("token via cookie: %v %v", err, resp.StatusCode)
	}
	// Last-used is recorded.
	got, err := s.Store.(store.TokenStore).GetToken(context.Background(), tr.Token.ID)
	if err != nil || got.LastUsedAt == nil {
		t.Fatalf("last used not recorded: %+v %v", got, err)
	}

	// Revoke: the next request fails with 401, the audit trail has both events.
	if code, body := do(t, ts, admin, "DELETE", "/api/tokens/"+tr.Token.ID, ""); code != 200 || !strings.Contains(body, "revokedAt") {
		t.Fatalf("revoke: %d %s", code, body)
	}
	if code, _ := do(t, ts, tr.Secret, "GET", "/api/devices", ""); code != 401 {
		t.Fatalf("revoked token still works: %d", code)
	}
	if code, _ := do(t, ts, admin, "DELETE", "/api/tokens/"+tr.Token.ID, ""); code != 404 {
		t.Fatalf("double revoke: %d", code)
	}
	_, audit := do(t, ts, admin, "GET", "/api/audit?limit=50", "")
	for _, want := range []string{`"action":"token.create"`, `"action":"token.revoke"`, `"action":"auth.token.created"`, `"action":"auth.token.revoked"`, `"action":"auth.denied"`} {
		if !strings.Contains(audit, want) {
			t.Errorf("audit lacks %s", want)
		}
	}
	if strings.Contains(audit, tr.Secret) {
		t.Error("audit leaks the secret")
	}

	// Expired tokens are refused like unknown ones.
	past := time.Now().Add(-time.Hour)
	exp := &store.APIToken{Name: "old", Role: model.RoleViewer, ExpiresAt: &past}
	if err := s.Store.(store.TokenStore).CreateToken(context.Background(), exp, "deadbeef"); err != nil {
		t.Fatal(err)
	}
	if code, _ := do(t, ts, "pmt_whatever", "GET", "/api/devices", ""); code != 401 {
		t.Fatalf("unknown token: %d", code)
	}
}

// Audit A11: a group-scoped principal only sees and acts on devices in its
// groups, on the core routes and on every /api/devices/{id}/... feature route.
func TestGroupScopedToken(t *testing.T) {
	ts, s := newTestServer(t)
	ctx := context.Background()
	admin := login(t, ts, "admin", "adminpass123")
	in, _, err := s.Store.UpsertDevice(ctx, &model.Identity{Address: "10.0.0.10", Serial: "IN1", Make: "Acme", Model: "Acme 100", Method: "snmp"}, map[model.Capability]string{model.CapReadStatus: "snmp"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	out, _, err := s.Store.UpsertDevice(ctx, &model.Identity{Address: "10.0.0.11", Serial: "OUT1", Make: "Acme", Model: "Acme 100", Method: "snmp"}, map[model.Capability]string{model.CapReadStatus: "snmp"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	g := &model.Group{Name: "site-a", Kind: "static"}
	if err := s.Store.CreateGroup(ctx, g); err != nil {
		t.Fatal(err)
	}
	if err := s.Store.SetGroupMembers(ctx, g.ID, []string{in.ID}); err != nil {
		t.Fatal(err)
	}
	tr := mkToken(t, ts, admin, `{"name":"site-a-bot","role":"operator","scope":["`+g.ID+`"]}`)
	if len(tr.Token.Scope) != 1 || tr.Token.Scope[0] != g.ID {
		t.Fatalf("scope %+v", tr.Token.Scope)
	}
	tok := tr.Secret

	// Lists are filtered; the admin still sees everything.
	code, body := do(t, ts, tok, "GET", "/api/devices", "")
	if code != 200 || !strings.Contains(body, in.ID) || strings.Contains(body, out.ID) || !strings.Contains(body, `"total":1`) {
		t.Fatalf("scoped list: %d %s", code, body)
	}
	if _, body := do(t, ts, admin, "GET", "/api/devices", ""); !strings.Contains(body, out.ID) {
		t.Fatalf("admin list filtered: %s", body)
	}
	if _, body := do(t, ts, tok, "GET", "/api/devices.csv", ""); strings.Contains(body, out.Serial) || !strings.Contains(body, in.Serial) {
		t.Fatalf("csv not filtered: %s", body)
	}
	if _, body := do(t, ts, tok, "GET", "/api/dashboard", ""); strings.Contains(body, out.ID) {
		t.Fatalf("dashboard not filtered: %s", body)
	}

	// Per-device routes: in scope ok, out of scope 403, on core and feature paths alike.
	if code, body := do(t, ts, tok, "GET", "/api/devices/"+in.ID, ""); code != 200 {
		t.Fatalf("in-scope get: %d %s", code, body)
	}
	for _, p := range []string{"/api/devices/" + out.ID, "/api/devices/" + out.ID + "/alerts", "/api/devices/" + out.ID + "/config", "/api/devices/" + out.ID + "/supplies", "/api/devices/" + out.ID + "/firmware"} {
		if code, body := do(t, ts, tok, "GET", p, ""); code != 403 || !strings.Contains(body, "outside your group scope") {
			t.Fatalf("out-of-scope %s: %d %s", p, code, body)
		}
	}
	if code, _ := do(t, ts, tok, "PATCH", "/api/devices/"+out.ID, `{"location":"x"}`); code != 403 {
		t.Fatalf("out-of-scope patch: %d", code)
	}
	if code, body := do(t, ts, tok, "PATCH", "/api/devices/"+in.ID, `{"location":"rack 1"}`); code != 200 {
		t.Fatalf("in-scope patch: %d %s", code, body)
	}
	if code, _ := do(t, ts, tok, "GET", "/api/telemetry?device="+out.ID, ""); code != 403 {
		t.Fatalf("telemetry out of scope: %d", code)
	}
	// Whole-fleet and cross-scope bulk actions are refused; in-scope ones pass.
	if code, body := do(t, ts, tok, "POST", "/api/poll", ``); code != 403 || !strings.Contains(body, "outside your group scope") {
		t.Fatalf("poll all: %d %s", code, body)
	}
	if code, _ := do(t, ts, tok, "POST", "/api/poll", `{"deviceIds":["`+out.ID+`"]}`); code != 403 {
		t.Fatalf("poll out of scope: %d", code)
	}
	if code, body := do(t, ts, tok, "POST", "/api/poll", `{"deviceIds":["`+in.ID+`"]}`); code != 202 {
		t.Fatalf("poll in scope: %d %s", code, body)
	}
	// Session users are unscoped (existing behaviour).
	if code, _ := do(t, ts, admin, "GET", "/api/devices/"+out.ID, ""); code != 200 {
		t.Fatalf("admin out-of-scope get: %d", code)
	}
	// A rule group scope resolves through the rule.
	rule := &model.Group{Name: "acme", Kind: "rule", Rule: &model.GroupRule{MakeContains: "acme"}}
	if err := s.Store.CreateGroup(ctx, rule); err != nil {
		t.Fatal(err)
	}
	tr2 := mkToken(t, ts, admin, `{"name":"acme-bot","role":"viewer","scope":["`+rule.ID+`"]}`)
	if code, body := do(t, ts, tr2.Secret, "GET", "/api/devices", ""); code != 200 || !strings.Contains(body, `"total":2`) {
		t.Fatalf("rule scope list: %d %s", code, body)
	}
}
