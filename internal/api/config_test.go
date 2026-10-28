package api

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"printmarshal/internal/auth"
	"printmarshal/internal/config"
	"printmarshal/internal/configmgmt"
	"printmarshal/internal/driver/snmp"
	"printmarshal/internal/engine"
	"printmarshal/internal/fleet"
	"printmarshal/internal/jobs"
	"printmarshal/internal/model"
	"printmarshal/internal/secrets"
	"printmarshal/internal/store"
)

// newConfigServer is newTestServer plus a fixture-backed SNMP driver and the
// configuration service, with one identified device.
func newConfigServer(t *testing.T) (*httptest.Server, *Server, *model.Device, *snmp.Fixture) {
	t.Helper()
	fx, err := snmp.LoadFixture("../../testdata/fixtures/generic-laser.snmp")
	if err != nil {
		t.Fatal(err)
	}
	fx.Writable = true
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
	reg := engine.NewRegistry()
	reg.Register(snmp.New(func(ctx context.Context, address string, creds model.Creds) (snmp.Session, string, error) {
		return fx, "v2c", nil
	}))
	eng := engine.New(reg, log, 2)
	fl := fleet.New(st, eng, runner, cfg, log, hub.Publish, nil)
	cm := configmgmt.New(fl.DepsOf())
	a := auth.New(st, time.Hour, false, log)
	if err := a.Bootstrap(context.Background(), "adminpass123"); err != nil {
		t.Fatal(err)
	}
	if _, err := a.CreateUser(context.Background(), "viewer", "viewerpass1", model.RoleViewer); err != nil {
		t.Fatal(err)
	}
	if _, err := a.CreateUser(context.Background(), "op", "operatorpass1", model.RoleOperator); err != nil {
		t.Fatal(err)
	}
	ident, caps, err := eng.Identify(context.Background(), "10.0.0.5", model.Creds{})
	if err != nil {
		t.Fatal(err)
	}
	dev, _, err := st.UpsertDevice(context.Background(), ident, caps, []string{"scan"})
	if err != nil {
		t.Fatal(err)
	}
	// Real applies need a write credential bound (audit A19).
	if err := st.CreateCredential(context.Background(), &model.Credential{Name: "snmp write", Type: model.CredSNMPCommunity, Purpose: model.PurposeWrite}, &model.Secret{Community: "private"}); err != nil {
		t.Fatal(err)
	}
	s := &Server{Store: st, Fleet: fl, ConfigMgmt: cm, Auth: a, Hub: hub, Cfg: cfg, Log: log, Version: "test"}
	ts := httptest.NewServer(s.Handler())
	t.Cleanup(ts.Close)
	return ts, s, dev, fx
}

func waitJob(t *testing.T, s *Server, body string) *model.Job {
	t.Helper()
	var resp struct {
		Job model.Job `json:"job"`
	}
	if err := json.Unmarshal([]byte(body), &resp); err != nil || resp.Job.ID == "" {
		t.Fatalf("no job in %s", body)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	j, err := s.Fleet.Jobs.Wait(ctx, resp.Job.ID)
	if err != nil {
		t.Fatal(err)
	}
	return j
}

// Roles (spec §11): viewers read, never write; operators apply; every write is a job.
func TestConfigAPIRolesAndFlow(t *testing.T) {
	ts, s, dev, fx := newConfigServer(t)
	viewer := login(t, ts, "viewer", "viewerpass1")
	op := login(t, ts, "op", "operatorpass1")

	// Catalogue and device config are readable by a viewer.
	if code, body := do(t, ts, viewer, "GET", "/api/config/catalog", ""); code != 200 || !strings.Contains(body, "identity.location") {
		t.Fatalf("catalog: %d %s", code, body)
	}
	code, body := do(t, ts, viewer, "GET", "/api/devices/"+dev.ID+"/config?refresh=true", "")
	if code != 200 || !strings.Contains(body, "Floor 2 east wing") {
		t.Fatalf("device config: %d %s", code, body)
	}
	if code, body := do(t, ts, viewer, "GET", "/api/devices/"+dev.ID+"/config/snapshots", ""); code != 200 || !strings.Contains(body, `"total":1`) {
		t.Fatalf("snapshots: %d %s", code, body)
	}

	// Viewer cannot write anything.
	profile := `{"name":"lobby","settings":[{"key":"identity.location","value":"Lobby"}]}`
	if code, _ := do(t, ts, viewer, "POST", "/api/profiles", profile); code != 403 {
		t.Errorf("viewer create profile = %d", code)
	}
	if code, _ := do(t, ts, viewer, "POST", "/api/devices/"+dev.ID+"/config", `{"changes":[{"key":"identity.location","value":"X"}],"dryRun":true}`); code != 403 {
		t.Errorf("viewer ad-hoc apply = %d", code)
	}
	if code, _ := do(t, ts, "", "GET", "/api/profiles", ""); code != 401 {
		t.Errorf("unauthenticated = %d", code)
	}

	// Operator: profile CRUD with validation.
	code, body = do(t, ts, op, "POST", "/api/profiles", profile)
	if code != 201 {
		t.Fatalf("create profile: %d %s", code, body)
	}
	var p model.Profile
	_ = json.Unmarshal([]byte(body), &p)
	if code, body := do(t, ts, op, "POST", "/api/profiles", `{"name":"bad","settings":[{"key":"nope.key","value":1}]}`); code != 400 {
		t.Errorf("unknown key accepted: %d %s", code, body)
	}
	if code, body := do(t, ts, op, "POST", "/api/profiles", `{"name":"bad2","settings":[{"key":"protocols.snmp.readCommunity","value":"x"}]}`); code != 400 {
		t.Errorf("secret in profile accepted: %d %s", code, body)
	}
	if code, body := do(t, ts, op, "PATCH", "/api/profiles/"+p.ID, `{"description":"front desk"}`); code != 200 || !strings.Contains(body, "front desk") {
		t.Errorf("patch profile: %d %s", code, body)
	}
	if code, _ := do(t, ts, viewer, "GET", "/api/profiles/"+p.ID, ""); code != 200 {
		t.Errorf("viewer get profile = %d", code)
	}

	// Drift (viewer): the device is at "Floor 2 east wing", profile wants "Lobby".
	code, body = do(t, ts, viewer, "POST", "/api/profiles/"+p.ID+"/drift", `{"target":{"deviceIds":["`+dev.ID+`"]}}`)
	if code != 200 || !strings.Contains(body, `"drifted":1`) {
		t.Fatalf("drift: %d %s", code, body)
	}

	// Dry-run apply: 202 + job, nothing written on the device.
	code, body = do(t, ts, op, "POST", "/api/profiles/"+p.ID+"/apply", `{"target":{"deviceIds":["`+dev.ID+`"]},"dryRun":true}`)
	if code != 202 {
		t.Fatalf("apply dry run: %d %s", code, body)
	}
	if j := waitJob(t, s, body); j.Status != model.JobSucceeded {
		t.Fatalf("dry-run job = %+v", j)
	}
	if len(fx.Sets()) != 0 {
		t.Fatal("dry run wrote to the device")
	}
	if code, _ := do(t, ts, op, "POST", "/api/profiles/"+p.ID+"/apply", `{"target":{},"dryRun":true}`); code != 400 {
		t.Errorf("empty target = %d", code)
	}

	// Real apply through the ad-hoc endpoint.
	code, body = do(t, ts, op, "POST", "/api/devices/"+dev.ID+"/config", `{"changes":[{"key":"identity.location","value":"Lobby"}],"dryRun":false}`)
	if code != 202 {
		t.Fatalf("ad-hoc apply: %d %s", code, body)
	}
	j := waitJob(t, s, body)
	if j.Status != model.JobSucceeded {
		t.Fatalf("apply job = %+v", j)
	}
	if code, body := do(t, ts, viewer, "GET", "/api/jobs/"+j.ID, ""); code != 200 || !strings.Contains(body, "1 written") {
		t.Errorf("job results: %d %s", code, body)
	}
	code, body = do(t, ts, viewer, "GET", "/api/config/writes?device="+dev.ID, "")
	if code != 200 {
		t.Fatalf("writes: %d %s", code, body)
	}
	var writes struct {
		Writes []model.WriteRecord `json:"writes"`
	}
	_ = json.Unmarshal([]byte(body), &writes)
	var real *model.WriteRecord
	for i := range writes.Writes {
		if !writes.Writes[i].Result.DryRun {
			real = &writes.Writes[i]
		}
	}
	if real == nil || real.Written != 1 {
		t.Fatalf("real write record missing: %s", body)
	}
	if code, _ := do(t, ts, viewer, "POST", "/api/config/writes/"+real.ID+"/revert", `{"dryRun":false}`); code != 403 {
		t.Errorf("viewer revert = %d", code)
	}
	code, body = do(t, ts, op, "POST", "/api/config/writes/"+real.ID+"/revert", `{"dryRun":false}`)
	if code != 202 {
		t.Fatalf("revert: %d %s", code, body)
	}
	if j := waitJob(t, s, body); j.Status != model.JobSucceeded {
		t.Fatalf("revert job = %+v", j)
	}
	code, body = do(t, ts, viewer, "GET", "/api/devices/"+dev.ID+"/config", "")
	if code != 200 || !strings.Contains(body, "Floor 2 east wing") {
		t.Fatalf("after revert: %d %s", code, body)
	}

	// Diff of a device with itself, and the audit trail of the writes.
	if code, body := do(t, ts, viewer, "GET", "/api/config/diff?a="+dev.ID+"&b="+dev.ID, ""); code != 200 || !strings.Contains(body, `"items":[]`) {
		t.Errorf("diff: %d %s", code, body)
	}
	code, body = do(t, ts, op, "GET", "/api/audit", "")
	if code != 200 || !strings.Contains(body, "config.apply") || !strings.Contains(body, "profile.create") || !strings.Contains(body, "config.revert") {
		t.Errorf("audit: %d %s", code, body)
	}
	if code, _ := do(t, ts, op, "DELETE", "/api/profiles/"+p.ID, ""); code != 200 {
		t.Errorf("delete profile = %d", code)
	}
	if code, _ := do(t, ts, viewer, "GET", "/api/profiles/"+p.ID, ""); code != 404 {
		t.Errorf("deleted profile = %d", code)
	}
}
