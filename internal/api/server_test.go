package api

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"printmarshal/internal/auth"
	"printmarshal/internal/config"
	"printmarshal/internal/engine"
	"printmarshal/internal/fleet"
	"printmarshal/internal/jobs"
	"printmarshal/internal/model"
	"printmarshal/internal/secrets"
	"printmarshal/internal/store"
)

func newTestServer(t *testing.T) (*httptest.Server, *Server) {
	t.Helper()
	box, _ := secrets.New(make([]byte, 32))
	st, err := store.OpenSQLite(t.TempDir()+"/t.db", box)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	cfg := config.Default()
	hub := NewHub()
	runner := jobs.New(st, log, 1, hub.Publish)
	t.Cleanup(runner.Stop)
	eng := engine.New(engine.NewRegistry(), log, 2)
	fl := fleet.New(st, eng, runner, cfg, log, hub.Publish, nil)
	a := auth.New(st, time.Hour, false, log)
	if err := a.Bootstrap(context.Background(), "adminpass123"); err != nil {
		t.Fatal(err)
	}
	if _, err := a.CreateUser(context.Background(), "viewer", "viewerpass1", model.RoleViewer); err != nil {
		t.Fatal(err)
	}
	s := &Server{Store: st, Fleet: fl, Auth: a, Hub: hub, Cfg: cfg, Log: log, Version: "test"}
	ts := httptest.NewServer(s.Handler())
	t.Cleanup(ts.Close)
	return ts, s
}

func login(t *testing.T, ts *httptest.Server, user, pass string) string {
	t.Helper()
	resp, err := http.Post(ts.URL+"/api/auth/login", "application/json", strings.NewReader(`{"username":"`+user+`","password":"`+pass+`"}`))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("login status %d", resp.StatusCode)
	}
	var body struct {
		Token string `json:"token"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&body)
	return body.Token
}

func do(t *testing.T, ts *httptest.Server, token, method, path, body string) (int, string) {
	t.Helper()
	req, _ := http.NewRequest(method, ts.URL+path, strings.NewReader(body))
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, string(b)
}

// Spec §14 safety: secrets never appear in API output; writes are role-gated.
func TestCredentialsNeverLeakAndRolesEnforced(t *testing.T) {
	ts, _ := newTestServer(t)
	if code, _ := do(t, ts, "", "GET", "/api/devices", ""); code != 401 {
		t.Errorf("unauthenticated = %d", code)
	}
	admin := login(t, ts, "admin", "adminpass123")
	viewer := login(t, ts, "viewer", "viewerpass1")

	code, body := do(t, ts, admin, "POST", "/api/credentials", `{"name":"site","type":"snmp_community","secret":{"community":"topSecretComm"}}`)
	if code != 201 || strings.Contains(body, "topSecretComm") {
		t.Fatalf("create credential: %d %s", code, body)
	}
	code, body = do(t, ts, admin, "GET", "/api/credentials", "")
	if code != 200 || strings.Contains(body, "topSecretComm") || !strings.Contains(body, `"name":"site"`) {
		t.Fatalf("list credentials: %d %s", code, body)
	}
	code, body = do(t, ts, admin, "GET", "/api/audit", "")
	if code != 200 || strings.Contains(body, "topSecretComm") || !strings.Contains(body, "credential.create") {
		t.Fatalf("audit: %d %s", code, body)
	}
	if code, _ := do(t, ts, viewer, "POST", "/api/credentials", `{"name":"x","type":"snmp_community","secret":{"community":"c"}}`); code != 403 {
		t.Errorf("viewer create credential = %d", code)
	}
	if code, _ := do(t, ts, viewer, "GET", "/api/credentials", ""); code != 403 {
		t.Errorf("viewer list credentials = %d", code)
	}
	if code, _ := do(t, ts, viewer, "POST", "/api/discovery/runs", `{"targets":["10.0.0.1"]}`); code != 403 {
		t.Errorf("viewer discovery = %d", code)
	}
	if code, _ := do(t, ts, viewer, "GET", "/api/dashboard", ""); code != 200 {
		t.Errorf("viewer dashboard = %d", code)
	}
}

func TestDevicePatchAndState(t *testing.T) {
	ts, s := newTestServer(t)
	admin := login(t, ts, "admin", "adminpass123")
	d, _, err := s.Store.UpsertDevice(context.Background(), &model.Identity{Address: "10.0.0.5", Serial: "S1", Model: "Acme 1", Method: "snmp"}, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	code, body := do(t, ts, admin, "PATCH", "/api/devices/"+d.ID, `{"displayName":"Lobby printer","assetTag":"A1"}`)
	if code != 200 || !strings.Contains(body, "Lobby printer") {
		t.Fatalf("patch: %d %s", code, body)
	}
	if code, body := do(t, ts, admin, "PATCH", "/api/devices/"+d.ID, `{"serial":"HACK"}`); code != 400 {
		t.Errorf("automatic fields must not be patchable: %d %s", code, body)
	}
	if code, _ := do(t, ts, admin, "POST", "/api/devices/"+d.ID+"/state", `{"state":"needs_attention"}`); code != 400 {
		t.Error("discovered -> needs_attention is not a valid transition")
	}
	if code, _ := do(t, ts, admin, "POST", "/api/devices/"+d.ID+"/state", `{"state":"active"}`); code != 200 {
		t.Error("discovered -> active should be allowed")
	}
	code, body = do(t, ts, admin, "GET", "/api/devices.csv", "")
	if code != 200 || !strings.Contains(body, "Lobby printer") || !strings.Contains(body, "active") {
		t.Errorf("csv: %d %s", code, body)
	}
}
