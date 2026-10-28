package api

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"printmarshal/internal/engine"
	"printmarshal/internal/model"
	"printmarshal/internal/security"
)

// apiFakeDriver reads and writes security settings in memory.
type apiFakeDriver struct {
	mu  sync.Mutex
	doc map[string]any
}

func (f *apiFakeDriver) Name() string { return "fakesec" }
func (f *apiFakeDriver) Capabilities() []model.Capability {
	return []model.Capability{model.CapReadSecurity, model.CapWriteSecurity}
}
func (f *apiFakeDriver) Supports(d *model.Device, cap model.Capability) bool {
	return engine.ProfileSupports(d, "fakesec", cap)
}
func (f *apiFakeDriver) Priority(model.Capability) int { return 10 }
func (f *apiFakeDriver) ReadSecurity(_ context.Context, d *model.Device, _ model.Creds) (*model.ConfigDocument, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	doc := &model.ConfigDocument{DeviceID: d.ID, ReadAt: time.Now()}
	for k, v := range f.doc {
		doc.Settings = append(doc.Settings, model.Setting{Key: k, Value: v, Writable: true})
	}
	return doc, nil
}
func (f *apiFakeDriver) WriteSecurity(_ context.Context, d *model.Device, _ model.Creds, changes []model.SettingChange, opts model.WriteOptions) (*model.WriteResult, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	res := &model.WriteResult{DeviceID: d.ID, DryRun: opts.DryRun}
	for _, c := range changes {
		r := model.SettingResult{Key: c.Key, Before: f.doc[c.Key], After: c.Value, Status: model.SettingWouldWrite}
		if !opts.DryRun {
			f.doc[c.Key] = c.Value
			r.Status = model.SettingWritten
		}
		res.Results = append(res.Results, r)
	}
	return res, nil
}

func newSecurityTestServer(t *testing.T) (*httptest.Server, *Server, *apiFakeDriver) {
	t.Helper()
	ts, s := newTestServer(t)
	fake := &apiFakeDriver{doc: map[string]any{"protocols.telnet.enabled": true, "protocols.snmp.readCommunity": "public", "security.adminPassword": "old"}}
	s.Fleet.Engine.Registry.Register(fake)
	svc := security.New(s.Fleet.DepsOf())
	svc.TLSProbe, svc.PortProbe = nil, nil // no network I/O in tests
	s.Security = svc
	// Real remediation and rotation need a write credential bound (audit A19).
	if err := s.Store.CreateCredential(context.Background(), &model.Credential{Name: "admin rw", Type: model.CredAdminPassword, Username: "admin", Purpose: model.PurposeWrite}, &model.Secret{Password: "harnessWritePw"}); err != nil {
		t.Fatal(err)
	}
	return ts, s, fake
}

func waitSecurityJob(t *testing.T, s *Server, body string) *model.Job {
	t.Helper()
	var j model.Job
	if err := json.Unmarshal([]byte(body), &j); err != nil {
		t.Fatalf("job body: %v %s", err, body)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	done, err := s.Fleet.Jobs.Wait(ctx, j.ID)
	if err != nil {
		t.Fatal(err)
	}
	return done
}

func TestSecurityRolesAndFlows(t *testing.T) {
	ts, s, fake := newSecurityTestServer(t)
	admin := login(t, ts, "admin", "adminpass123")
	viewer := login(t, ts, "viewer", "viewerpass1")
	if _, err := s.Auth.CreateUser(context.Background(), "op", "operatorpass1", model.RoleOperator); err != nil {
		t.Fatal(err)
	}
	op := login(t, ts, "op", "operatorpass1")
	dev, _, err := s.Store.UpsertDevice(context.Background(), &model.Identity{Address: "10.0.0.50", Serial: "S50", Model: "Acme", Method: "test"},
		map[model.Capability]string{model.CapReadSecurity: "fakesec", model.CapWriteSecurity: "fakesec", model.CapReadStatus: "snmp"}, nil)
	if err != nil {
		t.Fatal(err)
	}

	// Policies: viewer reads (the built-in Baseline exists), only operators write.
	code, body := do(t, ts, viewer, "GET", "/api/policies", "")
	if code != 200 || !strings.Contains(body, `"name":"Baseline"`) {
		t.Fatalf("list policies: %d %s", code, body)
	}
	if code, _ := do(t, ts, viewer, "POST", "/api/policies", `{"name":"x","items":[]}`); code != 403 {
		t.Errorf("viewer create policy = %d", code)
	}
	code, body = do(t, ts, op, "POST", "/api/policies", `{"name":"Telnet off","items":[{"id":"telnet","key":"protocols.telnet.enabled","operator":"eq","value":false,"severity":"critical","remediate":true}]}`)
	if code != 201 {
		t.Fatalf("create policy: %d %s", code, body)
	}
	var pol model.Policy
	_ = json.Unmarshal([]byte(body), &pol)
	if code, body := do(t, ts, op, "POST", "/api/policies", `{"name":"bad","items":[{"key":"a","operator":"nope","value":1}]}`); code != 400 {
		t.Errorf("invalid operator must be 400: %d %s", code, body)
	}
	if code, _ := do(t, ts, op, "POST", "/api/policies", `{"name":"Telnet off","items":[]}`); code != 409 {
		t.Errorf("duplicate name = %d", code)
	}
	if code, body := do(t, ts, op, "PATCH", "/api/policies/"+pol.ID, `{"description":"updated"}`); code != 200 || !strings.Contains(body, "updated") {
		t.Errorf("patch: %d %s", code, body)
	}
	if code, _ := do(t, ts, viewer, "DELETE", "/api/policies/"+pol.ID, ""); code != 403 {
		t.Error("viewer delete must be 403")
	}

	// Assess: operator only; runs as a job; compliance views for viewers.
	if code, _ := do(t, ts, viewer, "POST", "/api/policies/"+pol.ID+"/assess", `{"deviceIds":["`+dev.ID+`"]}`); code != 403 {
		t.Error("viewer assess must be 403")
	}
	if code, body := do(t, ts, op, "POST", "/api/policies/"+pol.ID+"/assess", `{}`); code != 400 {
		t.Errorf("empty target must be 400: %d %s", code, body)
	}
	if code, _ := do(t, ts, op, "POST", "/api/policies/nope/assess", `{"deviceIds":["`+dev.ID+`"]}`); code != 404 {
		t.Error("unknown policy must be 404")
	}
	code, body = do(t, ts, op, "POST", "/api/policies/"+pol.ID+"/assess", `{"deviceIds":["`+dev.ID+`"]}`)
	if code != 202 {
		t.Fatalf("assess: %d %s", code, body)
	}
	if j := waitSecurityJob(t, s, body); j.Status != model.JobSucceeded || j.Type != "security.assess" {
		t.Fatalf("assess job: %+v", j)
	}
	code, body = do(t, ts, viewer, "GET", "/api/compliance?policy="+pol.ID, "")
	if code != 200 || !strings.Contains(body, `"compliant":false`) || !strings.Contains(body, `"nonCompliant":1`) {
		t.Errorf("compliance: %d %s", code, body)
	}
	code, body = do(t, ts, viewer, "GET", "/api/compliance?device="+dev.ID, "")
	if code != 200 || !strings.Contains(body, `"itemId":"telnet"`) {
		t.Errorf("compliance by device: %d %s", code, body)
	}
	code, body = do(t, ts, viewer, "GET", "/api/compliance/history?policy="+pol.ID, "")
	if code != 200 || !strings.Contains(body, `"assessed":1`) {
		t.Errorf("history: %d %s", code, body)
	}
	if code, _ := do(t, ts, viewer, "GET", "/api/compliance/history?from=yesterday", ""); code != 400 {
		t.Error("bad from must be 400")
	}
	code, body = do(t, ts, viewer, "GET", "/api/devices/"+dev.ID+"/security", "")
	if code != 200 || !strings.Contains(body, `"assessments"`) || !strings.Contains(body, `"policy:`+pol.ID+`:telnet"`) {
		t.Errorf("device security: %d %s", code, body)
	}
	if code, _ := do(t, ts, viewer, "GET", "/api/devices/"+dev.ID+"/certificates", ""); code != 200 {
		t.Error("certificates must be readable by viewers")
	}

	// Remediate: default is dry run; nothing changes; explicit dryRun:false writes.
	code, body = do(t, ts, op, "POST", "/api/policies/"+pol.ID+"/remediate", `{"deviceIds":["`+dev.ID+`"]}`)
	if code != 202 {
		t.Fatalf("remediate: %d %s", code, body)
	}
	j := waitSecurityJob(t, s, body)
	if j.Status != model.JobSucceeded || j.Params["dryRun"] != true {
		t.Fatalf("remediate dry-run job: %+v", j)
	}
	if fake.doc["protocols.telnet.enabled"] != true {
		t.Fatal("dry run changed the device")
	}
	if code, _ := do(t, ts, viewer, "POST", "/api/policies/"+pol.ID+"/remediate", `{"deviceIds":["`+dev.ID+`"],"dryRun":false}`); code != 403 {
		t.Error("viewer remediate must be 403")
	}
	code, body = do(t, ts, op, "POST", "/api/policies/"+pol.ID+"/remediate", `{"deviceIds":["`+dev.ID+`"],"dryRun":false}`)
	if code != 202 {
		t.Fatalf("remediate real: %d %s", code, body)
	}
	j = waitSecurityJob(t, s, body)
	code, body = do(t, ts, viewer, "GET", "/api/jobs/"+j.ID, "")
	if code != 200 || !strings.Contains(body, `"status":"written"`) {
		t.Errorf("job results: %d %s", code, body)
	}
	if fake.doc["protocols.telnet.enabled"] != false {
		t.Error("real remediation did not write")
	}
	code, body = do(t, ts, admin, "GET", "/api/audit", "")
	if code != 200 || !strings.Contains(body, `"security.remediate"`) {
		t.Errorf("audit: %d %s", code, body)
	}

	// CA profiles: viewer reads, admin writes.
	if code, _ := do(t, ts, op, "POST", "/api/ca-profiles", `{"name":"ca","protocol":"est","url":"https://ca.example.test/.well-known/est"}`); code != 403 {
		t.Error("operator must not create CA profiles")
	}
	if code, body := do(t, ts, admin, "POST", "/api/ca-profiles", `{"name":"ca","protocol":"ldap","url":"https://x"}`); code != 400 {
		t.Errorf("bad protocol: %d %s", code, body)
	}
	if code, body := do(t, ts, admin, "POST", "/api/ca-profiles", `{"name":"ca","protocol":"est","url":"https://x","credentialId":"missing"}`); code != 400 {
		t.Errorf("missing credential: %d %s", code, body)
	}
	code, body = do(t, ts, admin, "POST", "/api/ca-profiles", `{"name":"ca","protocol":"est","url":"https://ca.example.test/.well-known/est","keyType":"ecdsa-p256","options":{"label":"printers"}}`)
	if code != 201 {
		t.Fatalf("create ca profile: %d %s", code, body)
	}
	var ca model.CAProfile
	_ = json.Unmarshal([]byte(body), &ca)
	if code, body := do(t, ts, viewer, "GET", "/api/ca-profiles", ""); code != 200 || !strings.Contains(body, `"printers"`) {
		t.Errorf("list ca profiles: %d %s", code, body)
	}

	// Enrollment: operator; dry run contacts no CA and reports what it would do.
	if code, _ := do(t, ts, viewer, "POST", "/api/devices/"+dev.ID+"/certificates/enroll", `{"caProfileId":"`+ca.ID+`","dryRun":true}`); code != 403 {
		t.Error("viewer enroll must be 403")
	}
	if code, _ := do(t, ts, op, "POST", "/api/devices/"+dev.ID+"/certificates/enroll", `{"caProfileId":"nope"}`); code != 404 {
		t.Error("unknown CA profile must be 404")
	}
	code, body = do(t, ts, op, "POST", "/api/devices/"+dev.ID+"/certificates/enroll", `{"caProfileId":"`+ca.ID+`","commonName":"p.example.test","sans":["p.example.test"],"dryRun":true}`)
	if code != 202 {
		t.Fatalf("enroll: %d %s", code, body)
	}
	j = waitSecurityJob(t, s, body)
	results, _ := s.Store.ListJobResults(context.Background(), j.ID)
	if j.Status != model.JobSucceeded || len(results) != 1 || !strings.Contains(results[0].Message, "would enroll") || results[0].Data["canInstall"] != false {
		t.Errorf("enroll dry run: %+v %+v", j, results)
	}
	if code, _ := do(t, ts, admin, "DELETE", "/api/ca-profiles/"+ca.ID, ""); code != 200 {
		t.Error("admin delete ca profile")
	}

	// Rotation: operator; secrets never appear in any response.
	if code, _ := do(t, ts, viewer, "POST", "/api/security/rotate", `{"kind":"snmpCommunity","target":{"deviceIds":["`+dev.ID+`"]},"dryRun":false}`); code != 403 {
		t.Error("viewer rotate must be 403")
	}
	if code, _ := do(t, ts, op, "POST", "/api/security/rotate", `{"kind":"wifi","target":{"deviceIds":["`+dev.ID+`"]}}`); code != 400 {
		t.Error("unknown kind must be 400")
	}
	code, body = do(t, ts, op, "POST", "/api/security/rotate", `{"kind":"snmpCommunity","target":{"deviceIds":["`+dev.ID+`"]},"dryRun":false}`)
	if code != 202 {
		t.Fatalf("rotate: %d %s", code, body)
	}
	j = waitSecurityJob(t, s, body)
	if j.Status != model.JobSucceeded {
		t.Fatalf("rotate job: %+v", j)
	}
	secret, _ := fake.doc["protocols.snmp.readCommunity"].(string)
	if secret == "public" || secret == "" {
		t.Fatal("device community not rotated")
	}
	for _, path := range []string{"/api/jobs/" + j.ID, "/api/audit", "/api/credentials", "/api/devices/" + dev.ID + "/security", "/api/jobs?type=security.rotate"} {
		code, body := do(t, ts, admin, "GET", path, "")
		if code != 200 {
			t.Errorf("%s: %d", path, code)
		}
		if strings.Contains(body, secret) {
			t.Errorf("secret leaked into %s", path)
		}
	}
	if code, body := do(t, ts, admin, "GET", "/api/credentials", ""); !strings.Contains(body, `"scope":"device:`+dev.ID+`"`) {
		t.Errorf("rotated credential not stored: %d %s", code, body)
	}
	if code, body := do(t, ts, viewer, "GET", "/api/security/catalog", ""); code != 200 || !strings.Contains(body, "protocols.telnet.enabled") {
		t.Errorf("catalog: %d %s", code, body)
	}
}
