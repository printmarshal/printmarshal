package api

import (
	"context"
	"encoding/json"
	"fmt"
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

type testResolver struct {
	ptrs  map[string][]string
	hosts map[string][]string
}

func (r *testResolver) LookupAddr(_ context.Context, addr string) ([]string, error) {
	return r.ptrs[addr], nil
}

func (r *testResolver) LookupHost(_ context.Context, host string) ([]string, error) {
	return r.hosts[host], nil
}

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
	fl.Resolver = &testResolver{}
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
	if code != 200 || strings.Contains(body, "topSecretComm") || !strings.Contains(body, `"name":"site"`) || !strings.Contains(body, `"purpose":"read"`) {
		t.Fatalf("list credentials: %d %s", code, body)
	}
	// Purpose (audit A19): defaults to read, accepted on create, changed by PATCH (admin), audited.
	if code, body := do(t, ts, admin, "POST", "/api/credentials", `{"name":"bad","type":"snmp_community","purpose":"root","secret":{"community":"c"}}`); code != 400 {
		t.Errorf("bad purpose = %d %s", code, body)
	}
	code, body = do(t, ts, admin, "POST", "/api/credentials", `{"name":"rw","type":"snmp_community","purpose":"write","secret":{"community":"writeComm"}}`)
	if code != 201 || !strings.Contains(body, `"purpose":"write"`) || strings.Contains(body, "writeComm") {
		t.Fatalf("create write credential: %d %s", code, body)
	}
	var created struct {
		ID string `json:"id"`
	}
	_ = json.Unmarshal([]byte(body), &created)
	if code, _ := do(t, ts, viewer, "PATCH", "/api/credentials/"+created.ID, `{"purpose":"both"}`); code != 403 {
		t.Errorf("viewer patch credential = %d", code)
	}
	if code, body := do(t, ts, admin, "PATCH", "/api/credentials/"+created.ID, `{"purpose":"admin"}`); code != 400 {
		t.Errorf("patch bad purpose = %d %s", code, body)
	}
	if code, body := do(t, ts, admin, "PATCH", "/api/credentials/nope", `{"purpose":"both"}`); code != 404 {
		t.Errorf("patch unknown = %d %s", code, body)
	}
	code, body = do(t, ts, admin, "PATCH", "/api/credentials/"+created.ID, `{"purpose":"both"}`)
	if code != 200 || !strings.Contains(body, `"purpose":"both"`) || strings.Contains(body, "writeComm") {
		t.Fatalf("patch credential: %d %s", code, body)
	}
	if code, body := do(t, ts, admin, "GET", "/api/audit", ""); code != 200 || !strings.Contains(body, "credential.update") || strings.Contains(body, "writeComm") {
		t.Fatalf("audit after patch: %d %s", code, body)
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
	// The device web page: an operator override is validated and resolved.
	if code, body := do(t, ts, admin, "PATCH", "/api/devices/"+d.ID, `{"webUrl":"http://admin:pw@10.0.0.9/"}`); code != 400 {
		t.Fatalf("web url with credentials: %d %s", code, body)
	}
	if code, body := do(t, ts, admin, "PATCH", "/api/devices/"+d.ID, `{"webUrl":"https://printer-9.example.net/admin"}`); code != 200 || !strings.Contains(body, `"webUrl":"https://printer-9.example.net/admin"`) {
		t.Fatalf("web url override: %d %s", code, body)
	}
	if code, body := do(t, ts, admin, "PATCH", "/api/devices/"+d.ID, `{"webUrl":""}`); code != 200 || strings.Contains(body, `"webUrlOverride"`) {
		t.Fatalf("web url cleared: %d %s", code, body)
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

func TestRefreshDeviceHostname(t *testing.T) {
	ts, s := newTestServer(t)
	s.Fleet.Resolver = &testResolver{
		ptrs:  map[string][]string{"10.0.0.5": {"printer-7.corp.example.com."}},
		hosts: map[string][]string{"printer-7.corp.example.com": {"10.0.0.5"}},
	}
	admin := login(t, ts, "admin", "adminpass123")
	viewer := login(t, ts, "viewer", "viewerpass1")

	d, _, err := s.Store.UpsertDevice(context.Background(), &model.Identity{Address: "10.0.0.5", Hostname: "adv-host", Serial: "S1", Model: "Acme 1", Method: "snmp"}, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if code, _ := do(t, ts, "", "POST", "/api/devices/"+d.ID+"/refresh-hostname", ""); code != 401 {
		t.Errorf("unauthenticated refresh-hostname = %d", code)
	}
	if code, _ := do(t, ts, viewer, "POST", "/api/devices/"+d.ID+"/refresh-hostname", ""); code != 403 {
		t.Errorf("viewer refresh-hostname = %d, want 403", code)
	}
	if code, _ := do(t, ts, admin, "POST", "/api/devices/nope/refresh-hostname", ""); code != 404 {
		t.Errorf("unknown device refresh-hostname = %d, want 404", code)
	}
	code, body := do(t, ts, admin, "POST", "/api/devices/"+d.ID+"/refresh-hostname", "")
	if code != 200 || !strings.Contains(body, `"dnsName":"printer-7"`) || !strings.Contains(body, `"hostname":"adv-host"`) {
		t.Fatalf("refresh-hostname: %d %s", code, body)
	}
	if code, body := do(t, ts, admin, "GET", "/api/audit", ""); code != 200 || !strings.Contains(body, "device.refresh_hostname") {
		t.Errorf("audit after refresh-hostname: %d %s", code, body)
	}
}

func TestRenameDevicesAndNameSourceSetting(t *testing.T) {
	ts, s := newTestServer(t)
	admin := login(t, ts, "admin", "adminpass123")
	viewer := login(t, ts, "viewer", "viewerpass1")
	ctx := context.Background()

	d1, _, err := s.Store.UpsertDevice(ctx, &model.Identity{
		Address: "10.0.0.11", Hostname: "mdns-11.local", SysName: "NPI11", Serial: "SN11", Make: "Acme", Model: "Laser 1", SysLocation: "Floor2", Method: "snmp",
	}, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Store.SetDeviceDNS(ctx, d1.ID, "printer-11", false, time.Now().UTC()); err != nil {
		t.Fatal(err)
	}
	tag := "AT-11"
	if _, err := s.Store.PatchDevice(ctx, d1.ID, store.DevicePatch{AssetTag: &tag}); err != nil {
		t.Fatal(err)
	}

	d2, _, err := s.Store.UpsertDevice(ctx, &model.Identity{
		Address: "10.0.0.12", Hostname: "", SysName: "NPI12", Serial: "SN12", Make: "Acme", Model: "Laser 2", Method: "snmp",
	}, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	// Suspect DNS name on d2 (IP-derived).
	if _, err := s.Store.SetDeviceDNS(ctx, d2.ID, "ip-10-0-0-12", true, time.Now().UTC()); err != nil {
		t.Fatal(err)
	}

	d3, _, err := s.Store.UpsertDevice(ctx, &model.Identity{
		Address: "10.0.0.13", Serial: "SN13", Make: "Acme", Model: "Laser 3", Method: "snmp",
	}, nil, nil)
	if err != nil {
		t.Fatal(err)
	}

	// Viewer cannot rename.
	if code, _ := do(t, ts, viewer, "POST", "/api/devices/rename", `{"deviceIds":["`+d1.ID+`"],"source":"dns"}`); code != 403 {
		t.Fatalf("viewer rename = %d, want 403", code)
	}

	// Missing dryRun defaults to dryRun:true; d1 will rename, d2 skipped (suspect), d3 skipped (empty).
	reqDNS := fmt.Sprintf(`{"deviceIds":[%q,%q,%q],"source":"dns"}`, d1.ID, d2.ID, d3.ID)
	code, body := do(t, ts, admin, "POST", "/api/devices/rename", reqDNS)
	if code != 200 || !strings.Contains(body, `"dryRun":true`) || !strings.Contains(body, `"updated":1`) || !strings.Contains(body, `"skipped":2`) || !strings.Contains(body, "DNS name is suspect") || !strings.Contains(body, "DNS name is empty") {
		t.Fatalf("dry-run dns rename: %d %s", code, body)
	}
	afterDry, _ := s.Store.GetDevice(ctx, d1.ID)
	if afterDry.DisplayName != "" {
		t.Fatalf("dry run must not persist displayName, got %q", afterDry.DisplayName)
	}

	// Apply with dryRun:false.
	applyDNS := fmt.Sprintf(`{"deviceIds":[%q,%q,%q],"source":"dns","dryRun":false}`, d1.ID, d2.ID, d3.ID)
	code, body = do(t, ts, admin, "POST", "/api/devices/rename", applyDNS)
	if code != 200 || !strings.Contains(body, `"dryRun":false`) || !strings.Contains(body, `"updated":1`) {
		t.Fatalf("apply dns rename: %d %s", code, body)
	}
	afterApply, _ := s.Store.GetDevice(ctx, d1.ID)
	if afterApply.DisplayName != "printer-11" || afterApply.DisplayNameSource != "dns" {
		t.Fatalf("after apply: displayName=%q source=%q", afterApply.DisplayName, afterApply.DisplayNameSource)
	}

	// Re-running the same rename skips d1 as "unchanged".
	code, body = do(t, ts, admin, "POST", "/api/devices/rename", fmt.Sprintf(`{"deviceIds":[%q],"source":"dns"}`, d1.ID))
	if code != 200 || !strings.Contains(body, `"skipped":1`) || !strings.Contains(body, `"reason":"unchanged"`) {
		t.Fatalf("unchanged skip: %d %s", code, body)
	}

	// Source = hostname and sysName.
	code, body = do(t, ts, admin, "POST", "/api/devices/rename", fmt.Sprintf(`{"deviceIds":[%q,%q],"source":"hostname","dryRun":false}`, d1.ID, d2.ID))
	if code != 200 || !strings.Contains(body, `"updated":1`) || !strings.Contains(body, `"hostname is empty"`) {
		t.Fatalf("hostname rename: %d %s", code, body)
	}
	d1Host, _ := s.Store.GetDevice(ctx, d1.ID)
	if d1Host.DisplayName != "mdns-11.local" || d1Host.DisplayNameSource != "hostname" {
		t.Fatalf("hostname apply: %+v", d1Host)
	}

	code, body = do(t, ts, admin, "POST", "/api/devices/rename", fmt.Sprintf(`{"deviceIds":[%q,%q],"source":"sysName","dryRun":false}`, d1.ID, d3.ID))
	if code != 200 || !strings.Contains(body, `"updated":1`) || !strings.Contains(body, `"sysName is empty"`) {
		t.Fatalf("sysName rename: %d %s", code, body)
	}

	// Pattern expansion with all supported tokens.
	patReq := fmt.Sprintf(`{"deviceIds":[%q,%q],"source":"pattern","pattern":"{location}-{make}-{model}-{serial}-{address}-{hostname}-{dnsName}-{sysName}-{assetTag}","dryRun":false}`, d1.ID, d2.ID)
	code, body = do(t, ts, admin, "POST", "/api/devices/rename", patReq)
	if code != 200 || !strings.Contains(body, `"updated":1`) || !strings.Contains(body, `"DNS name is suspect"`) {
		t.Fatalf("pattern rename: %d %s", code, body)
	}
	d1Pat, _ := s.Store.GetDevice(ctx, d1.ID)
	wantPat := "Floor2-Acme-Laser 1-SN11-10.0.0.11-mdns-11.local-printer-11-NPI11-AT-11"
	if d1Pat.DisplayName != wantPat || d1Pat.DisplayNameSource != "pattern" {
		t.Fatalf("pattern result = %q (source %q), want %q", d1Pat.DisplayName, d1Pat.DisplayNameSource, wantPat)
	}

	// Audit contains devices.rename and device.patch.
	code, auditBody := do(t, ts, admin, "GET", "/api/audit", "")
	if code != 200 || !strings.Contains(auditBody, "devices.rename") || !strings.Contains(auditBody, "device.patch") {
		t.Errorf("expected devices.rename and device.patch in audit: %d %s", code, auditBody)
	}

	// PATCH /api/devices/{id} accepts dnsSync: bool.
	code, body = do(t, ts, admin, "PATCH", "/api/devices/"+d1.ID, `{"dnsSync":true}`)
	if code != 200 || !strings.Contains(body, `"dnsSync":true`) {
		t.Fatalf("patch dnsSync: %d %s", code, body)
	}

	// Group scope enforcement for scoped API token on POST /api/devices/rename.
	grp := &model.Group{Name: "G1", Kind: "static", MemberIDs: []string{d1.ID}}
	if err := s.Store.CreateGroup(ctx, grp); err != nil {
		t.Fatal(err)
	}
	code, tokBody := do(t, ts, admin, "POST", "/api/tokens", fmt.Sprintf(`{"name":"scoped-op","role":"operator","scope":[%q]}`, grp.ID))
	if code != 201 {
		t.Fatalf("create scoped token: %d %s", code, tokBody)
	}
	var tokResp struct {
		Secret string `json:"secret"`
	}
	if err := json.Unmarshal([]byte(tokBody), &tokResp); err != nil {
		t.Fatal(err)
	}
	// Naming d2 (outside grp) is denied with 403.
	if code, _ := do(t, ts, tokResp.Secret, "POST", "/api/devices/rename", fmt.Sprintf(`{"deviceIds":[%q],"source":"sysName"}`, d2.ID)); code != 403 {
		t.Errorf("scoped token rename outside group = %d, want 403", code)
	}
	// Naming d1 (inside grp) is allowed.
	if code, _ := do(t, ts, tokResp.Secret, "POST", "/api/devices/rename", fmt.Sprintf(`{"deviceIds":[%q],"source":"dns"}`, d1.ID)); code != 200 {
		t.Errorf("scoped token rename inside group = %d, want 200", code)
	}

	// Runtime settings: names.source GET/PATCH validation.
	code, setBody := do(t, ts, admin, "GET", "/api/settings", "")
	if code != 200 || !strings.Contains(setBody, `"source":"advertised"`) {
		t.Fatalf("GET /api/settings default names.source: %d %s", code, setBody)
	}
	var settings struct {
		Runtime map[string]any `json:"runtime"`
	}
	_ = json.Unmarshal([]byte(setBody), &settings)
	settings.Runtime["names"] = map[string]any{"source": "dns"}
	rawPatch, _ := json.Marshal(settings.Runtime)
	code, patchBody := do(t, ts, admin, "PATCH", "/api/settings", string(rawPatch))
	if code != 200 || !strings.Contains(patchBody, `"source":"dns"`) {
		t.Fatalf("PATCH /api/settings names.source=dns: %d %s", code, patchBody)
	}
	settings.Runtime["names"] = map[string]any{"source": "bogus"}
	rawBad, _ := json.Marshal(settings.Runtime)
	if code, _ := do(t, ts, admin, "PATCH", "/api/settings", string(rawBad)); code != 400 {
		t.Errorf("invalid names.source should fail with 400, got %d", code)
	}

	// Reverse-DNS options are runtime settings too (issue #18), with their
	// defaults' origin reported.
	if !strings.Contains(setBody, `"reverseDns":{"enabled":false`) || !strings.Contains(setBody, `"sources":{"reverseDns":{"enabled":"default"`) {
		t.Fatalf("GET /api/settings should report reverseDns and its sources: %s", setBody)
	}
	settings.Runtime["names"] = map[string]any{"source": "dns"}
	settings.Runtime["reverseDns"] = map[string]any{"enabled": true, "server": "10.0.0.53", "shortNames": true, "timeout": "2s", "refreshInterval": "12h", "forwardConfirm": false}
	rawRD, _ := json.Marshal(settings.Runtime)
	code, rdBody := do(t, ts, admin, "PATCH", "/api/settings", string(rawRD))
	if code != 200 || !strings.Contains(rdBody, `"reverseDns":{"enabled":true,"server":"10.0.0.53","shortNames":true,"timeout":"2s","refreshInterval":"12h0m0s","forwardConfirm":false}`) {
		t.Fatalf("PATCH /api/settings reverseDns: %d %s", code, rdBody)
	}
	settings.Runtime["reverseDns"] = map[string]any{"enabled": true, "timeout": "5ms", "refreshInterval": "12h"}
	rawRDBad, _ := json.Marshal(settings.Runtime)
	if code, _ := do(t, ts, admin, "PATCH", "/api/settings", string(rawRDBad)); code != 400 {
		t.Errorf("invalid reverseDns.timeout should fail with 400, got %d", code)
	}
	if code, _ := do(t, ts, admin, "DELETE", "/api/settings", ""); code != 200 {
		t.Errorf("DELETE /api/settings: %d", code)
	}
	if code, body := do(t, ts, admin, "GET", "/api/settings", ""); code != 200 || !strings.Contains(body, `"reverseDns":{"enabled":false`) {
		t.Fatalf("reset should restore the reverseDns defaults: %d %s", code, body)
	}
}

// intent/dns-rename-lookup: renaming from DNS looks the name up when none is
// stored, in the preview as well as the apply.
func TestRenamePreviewLooksUpEmptyDNSName(t *testing.T) {
	ts, s := newTestServer(t)
	s.Fleet.Resolver = &testResolver{
		ptrs:  map[string][]string{"10.0.0.21": {"copier-21.corp.example.com."}},
		hosts: map[string][]string{"copier-21.corp.example.com": {"10.0.0.21"}},
	}
	admin := login(t, ts, "admin", "adminpass123")
	ctx := context.Background()
	d, _, err := s.Store.UpsertDevice(ctx, &model.Identity{Address: "10.0.0.21", Serial: "SN21", Make: "Acme", Model: "Laser 21", Method: "snmp"}, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	code, body := do(t, ts, admin, "POST", "/api/devices/rename", fmt.Sprintf(`{"deviceIds":[%q],"source":"dns","dryRun":true}`, d.ID))
	if code != 200 {
		t.Fatalf("dry-run dns rename: %d %s", code, body)
	}
	var resp struct {
		Updated int            `json:"updated"`
		Skipped int            `json:"skipped"`
		Results []renameResult `json:"results"`
	}
	if err := json.Unmarshal([]byte(body), &resp); err != nil {
		t.Fatal(err)
	}
	if resp.Updated != 1 || resp.Skipped != 0 || len(resp.Results) != 1 || resp.Results[0].Skipped || resp.Results[0].To != "copier-21" {
		t.Fatalf("preview did not look up the empty DNS name: %s", body)
	}
	after, _ := s.Store.GetDevice(ctx, d.ID)
	if after.DisplayName != "" {
		t.Fatalf("dry run must not persist displayName, got %q", after.DisplayName)
	}
}

// A looked-up name that is an address in disguise stays skipped.
func TestRenameSkipsSuspectAfterLookup(t *testing.T) {
	ts, s := newTestServer(t)
	s.Fleet.Resolver = &testResolver{
		ptrs:  map[string][]string{"10.0.0.9": {"10-0-0-9.example.net."}},
		hosts: map[string][]string{"10-0-0-9.example.net": {"10.0.0.9"}},
	}
	admin := login(t, ts, "admin", "adminpass123")
	ctx := context.Background()
	d, _, err := s.Store.UpsertDevice(ctx, &model.Identity{Address: "10.0.0.9", Serial: "SN9", Make: "Acme", Model: "Laser 9", Method: "snmp"}, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	code, body := do(t, ts, admin, "POST", "/api/devices/rename", fmt.Sprintf(`{"deviceIds":[%q],"source":"dns","dryRun":false}`, d.ID))
	if code != 200 || !strings.Contains(body, `"updated":0`) || !strings.Contains(body, "DNS name is suspect") {
		t.Fatalf("suspect name after lookup: %d %s", code, body)
	}
	after, _ := s.Store.GetDevice(ctx, d.ID)
	if after.DisplayName != "" {
		t.Fatalf("suspect name must not rename, got %q", after.DisplayName)
	}
}

// A device kept in sync with DNS is renamed by the lookup itself; the rename
// preview must report that rename, not "unchanged".
func TestRenameCountsSyncRename(t *testing.T) {
	ts, s := newTestServer(t)
	s.Fleet.Resolver = &testResolver{
		ptrs:  map[string][]string{"10.0.0.22": {"copier-22.corp.example.com."}},
		hosts: map[string][]string{"copier-22.corp.example.com": {"10.0.0.22"}},
	}
	admin := login(t, ts, "admin", "adminpass123")
	ctx := context.Background()
	d, _, err := s.Store.UpsertDevice(ctx, &model.Identity{Address: "10.0.0.22", Serial: "SN22", Make: "Acme", Model: "Laser 22", Method: "snmp"}, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	sync := true
	if _, err := s.Store.PatchDevice(ctx, d.ID, store.DevicePatch{DNSSync: &sync}); err != nil {
		t.Fatal(err)
	}
	code, body := do(t, ts, admin, "POST", "/api/devices/rename", fmt.Sprintf(`{"deviceIds":[%q],"source":"dns","dryRun":true}`, d.ID))
	if code != 200 {
		t.Fatalf("dry-run dns rename: %d %s", code, body)
	}
	var resp struct {
		Updated int            `json:"updated"`
		Skipped int            `json:"skipped"`
		Results []renameResult `json:"results"`
	}
	if err := json.Unmarshal([]byte(body), &resp); err != nil {
		t.Fatal(err)
	}
	if resp.Updated != 1 || resp.Skipped != 0 || len(resp.Results) != 1 {
		t.Fatalf("sync rename reported as unchanged: %s", body)
	}
	if r := resp.Results[0]; r.Skipped || r.From != "Laser 22" || r.To != "copier-22" {
		t.Fatalf("sync rename row: %+v", r)
	}
	after, _ := s.Store.GetDevice(ctx, d.ID)
	if after.DisplayName != "copier-22" || after.DisplayNameSource != "dns" {
		t.Fatalf("after sync rename: displayName=%q source=%q", after.DisplayName, after.DisplayNameSource)
	}
}
