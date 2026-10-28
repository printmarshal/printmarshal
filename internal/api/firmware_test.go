package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"printmarshal/internal/engine"
	"printmarshal/internal/firmware"
	"printmarshal/internal/model"
	"printmarshal/internal/store"
)

// apiFakeUpdater delivers firmware by remembering the version per device; the
// fake poller below reports it back so verification succeeds.
type apiFakeUpdater struct {
	mu    sync.Mutex
	fw    map[string]string
	calls int
	real  int
}

func (f *apiFakeUpdater) Name() string { return "fake" }
func (f *apiFakeUpdater) Capabilities() []model.Capability {
	return []model.Capability{model.CapFirmwareUpdate}
}
func (f *apiFakeUpdater) Priority(model.Capability) int { return 10 }
func (f *apiFakeUpdater) Supports(d *model.Device, c model.Capability) bool {
	return engine.ProfileSupports(d, "fake", c)
}
func (f *apiFakeUpdater) UpdateFirmware(ctx context.Context, d *model.Device, creds model.Creds, pkg *model.FirmwarePackage, opts model.WriteOptions) (*model.FirmwareResult, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls++
	if opts.DryRun {
		return &model.FirmwareResult{DeviceID: d.ID, DryRun: true, Accepted: true, Message: "validated"}, nil
	}
	f.real++
	f.fw[d.ID] = pkg.Version
	return &model.FirmwareResult{DeviceID: d.ID, Accepted: true}, nil
}

func (f *apiFakeUpdater) poll(ctx context.Context, d *model.Device) (*model.Device, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	cp := *d
	if v, ok := f.fw[d.ID]; ok {
		cp.Firmware = v
	}
	cp.Status = &model.Status{Online: true, State: "idle", At: time.Now()}
	return &cp, nil
}

func newFirmwareServer(t *testing.T) (*httptest.Server, *Server, *apiFakeUpdater) {
	t.Helper()
	ts, s := newTestServer(t)
	fake := &apiFakeUpdater{fw: map[string]string{}}
	s.Fleet.Engine.Registry.Register(fake)
	s.Cfg.DataDir = t.TempDir()
	s.Firmware = firmware.New(s.Fleet.DepsOf())
	// Mirror what fleet.PollDevice does: the poll result is written to the device record.
	s.Firmware.Poll = func(ctx context.Context, d *model.Device) (*model.Device, error) {
		cp, _ := fake.poll(ctx, d)
		_, _ = s.Store.ApplyPoll(ctx, d.ID, store.PollUpdate{Status: cp.Status, Firmware: cp.Firmware, Reachable: true, At: time.Now()})
		return cp, nil
	}
	s.Firmware.ControlPoll, s.Firmware.VerifyInterval, s.Firmware.VerifyTimeout, s.Firmware.RetryDelay = 10*time.Millisecond, 5*time.Millisecond, time.Second, time.Millisecond
	if _, err := s.Auth.CreateUser(context.Background(), "operator", "operatorpass1", model.RoleOperator); err != nil {
		t.Fatal(err)
	}
	return ts, s, fake
}

func upload(t *testing.T, ts *httptest.Server, token string, fields map[string]string, fileName, content string) (int, string) {
	t.Helper()
	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	for k, v := range fields {
		_ = mw.WriteField(k, v)
	}
	if fileName != "" {
		p, _ := mw.CreateFormFile("file", fileName)
		_, _ = p.Write([]byte(content))
	}
	_ = mw.Close()
	req, _ := http.NewRequest("POST", ts.URL+"/api/firmware/packages", &body)
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, string(b)
}

func waitCampaign(t *testing.T, ts *httptest.Server, token, id string, cond func(c map[string]any) bool) map[string]any {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	var last map[string]any
	for time.Now().Before(deadline) {
		_, body := do(t, ts, token, "GET", "/api/firmware/campaigns/"+id, "")
		var out struct {
			Campaign map[string]any `json:"campaign"`
		}
		_ = json.Unmarshal([]byte(body), &out)
		last = out.Campaign
		if last != nil && cond(last) {
			return last
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("campaign condition not met: %v", last)
	return nil
}

// Spec §11/§14: viewers never write, packages are admin-only, campaigns are
// operator-gated, every write is audited, and the two explicit gates hold.
func TestFirmwareAPIRolesAndLifecycle(t *testing.T) {
	ts, s, fake := newFirmwareServer(t)
	ctx := context.Background()
	admin := login(t, ts, "admin", "adminpass123")
	operator := login(t, ts, "operator", "operatorpass1")
	viewer := login(t, ts, "viewer", "viewerpass1")

	// Reads are open to viewers; nothing is registered yet.
	if code, body := do(t, ts, viewer, "GET", "/api/firmware/inventory", ""); code != 200 || !strings.Contains(body, `"devices":0`) {
		t.Errorf("viewer inventory: %d %s", code, body)
	}
	if code, _ := do(t, ts, viewer, "GET", "/api/firmware/outdated", ""); code != 200 {
		t.Errorf("viewer outdated: %d", code)
	}
	if code, _ := do(t, ts, "", "GET", "/api/firmware/packages", ""); code != 401 {
		t.Errorf("unauthenticated: %d", code)
	}

	// Packages: admin only.
	if code, _ := upload(t, ts, viewer, map[string]string{"version": "1"}, "fw.bin", "x"); code != 403 {
		t.Errorf("viewer upload = %d", code)
	}
	if code, _ := upload(t, ts, operator, map[string]string{"version": "1"}, "fw.bin", "x"); code != 403 {
		t.Errorf("operator upload = %d", code)
	}
	if code, body := upload(t, ts, admin, map[string]string{"vendor": "Acme"}, "fw.bin", "x"); code != 400 || !strings.Contains(body, "version") {
		t.Errorf("missing version = %d %s", code, body)
	}
	if code, body := upload(t, ts, admin, map[string]string{"version": "2"}, "", ""); code != 400 || !strings.Contains(body, "file") {
		t.Errorf("missing file = %d %s", code, body)
	}
	code, body := upload(t, ts, admin, map[string]string{"vendor": "Acme", "models": "4200, LaserPro 5", "version": "2.15.0", "minVersion": "2.10", "notes": "security fixes"}, "acme-2.15.0.bin", "firmware image bytes")
	if code != 201 || !strings.Contains(body, `"sha256":"`) || !strings.Contains(body, `"size":20`) || strings.Contains(body, "storagePath") || !strings.Contains(body, `"models":["4200","LaserPro 5"]`) {
		t.Fatalf("upload: %d %s", code, body)
	}
	var pkg model.FirmwarePackage
	_ = json.Unmarshal([]byte(body), &pkg)
	if code, body := do(t, ts, viewer, "GET", "/api/firmware/packages/"+pkg.ID, ""); code != 200 || !strings.Contains(body, pkg.SHA256) {
		t.Errorf("get package: %d %s", code, body)
	}
	if code, _ := do(t, ts, viewer, "GET", "/api/firmware/packages/nope", ""); code != 404 {
		t.Errorf("unknown package: %d", code)
	}

	// Devices: three Acme 4200s at 2.14.7 and one already current.
	var ids []string
	for i := 0; i < 4; i++ {
		fw := "2.14.7"
		if i == 3 {
			fw = "2.15.0"
		}
		d, _, err := s.Store.UpsertDevice(ctx, &model.Identity{Address: fmt.Sprintf("10.0.0.%d", 10+i), Serial: fmt.Sprintf("S%d", i), Make: "Acme", Model: "Acme LaserPro 4200",
			Firmware: fw, SysName: fmt.Sprintf("printer-%d", i), Method: "snmp"}, map[model.Capability]string{model.CapFirmwareUpdate: "fake"}, nil)
		if err != nil {
			t.Fatal(err)
		}
		ids = append(ids, d.ID)
	}
	code, body = do(t, ts, viewer, "GET", "/api/firmware/inventory", "")
	if code != 200 || !strings.Contains(body, `"outdated":3`) || !strings.Contains(body, `"latest":"2.15.0"`) {
		t.Errorf("inventory: %d %s", code, body)
	}
	code, body = do(t, ts, viewer, "GET", "/api/firmware/outdated", "")
	if code != 200 || strings.Count(body, `"installed":"2.14.7"`) != 3 {
		t.Errorf("outdated: %d %s", code, body)
	}

	// Campaigns: operator; viewers are refused; bodies are validated.
	idsJSON, _ := json.Marshal(ids)
	create := fmt.Sprintf(`{"name":"Roll out 2.15","packageId":%q,"deviceIds":%s,"canaryCount":1,"concurrency":2,"failureThresholdPercent":50}`, pkg.ID, idsJSON)
	if code, _ := do(t, ts, viewer, "POST", "/api/firmware/campaigns", create); code != 403 {
		t.Errorf("viewer create campaign = %d", code)
	}
	if code, _ := do(t, ts, operator, "POST", "/api/firmware/campaigns", `{"name":"x","packageId":"nope","deviceIds":["a"]}`); code != 400 {
		t.Errorf("unknown package = %d", code)
	}
	if code, _ := do(t, ts, operator, "POST", "/api/firmware/campaigns", `{"name":"x","bogus":1}`); code != 400 {
		t.Errorf("unknown field = %d", code)
	}
	if code, _ := do(t, ts, operator, "POST", "/api/firmware/campaigns", `{"name":"x","packageId":"`+pkg.ID+`","deviceIds":["a"],"window":{"start":"22:00"}}`); code != 400 {
		t.Errorf("half window = %d", code)
	}
	code, body = do(t, ts, operator, "POST", "/api/firmware/campaigns", create)
	if code != 201 || !strings.Contains(body, `"status":"draft"`) || !strings.Contains(body, `"recoveryNotes":"`) || !strings.Contains(body, `"packageVersion":"2.15.0"`) {
		t.Fatalf("create campaign: %d %s", code, body)
	}
	var cv struct {
		ID string `json:"id"`
	}
	_ = json.Unmarshal([]byte(body), &cv)
	cid := cv.ID
	if fake.calls != 0 {
		t.Fatal("creating a campaign must not deliver anything")
	}

	// The package is now referenced: deleting it is refused (409).
	if code, body := do(t, ts, admin, "DELETE", "/api/firmware/packages/"+pkg.ID, ""); code != 409 {
		t.Errorf("delete referenced package = %d %s", code, body)
	}

	// Drafts are editable; gates refuse out-of-order actions.
	if code, body := do(t, ts, operator, "PATCH", "/api/firmware/campaigns/"+cid, `{"description":"after hours","window":{"tz":"UTC"}}`); code != 200 || !strings.Contains(body, "after hours") {
		t.Errorf("patch: %d %s", code, body)
	}
	if code, _ := do(t, ts, operator, "POST", "/api/firmware/campaigns/"+cid+"/confirm", ""); code != 409 {
		t.Errorf("confirm draft = %d", code)
	}
	if code, _ := do(t, ts, viewer, "POST", "/api/firmware/campaigns/"+cid+"/start", `{"dryRun":false}`); code != 403 {
		t.Errorf("viewer start = %d", code)
	}

	// Explicit start = wave 1 gate.
	code, body = do(t, ts, operator, "POST", "/api/firmware/campaigns/"+cid+"/start", `{"dryRun":false}`)
	if code != 200 || !strings.Contains(body, `"canaryCount":1`) || !strings.Contains(body, `"startedBy":"operator"`) {
		t.Fatalf("start: %d %s", code, body)
	}
	if code, _ := do(t, ts, operator, "POST", "/api/firmware/campaigns/"+cid+"/start", ""); code != 409 {
		t.Errorf("double start = %d", code)
	}
	if code, _ := do(t, ts, operator, "PATCH", "/api/firmware/campaigns/"+cid, `{"name":"renamed"}`); code != 409 {
		t.Errorf("patch after start = %d", code)
	}
	c := waitCampaign(t, ts, viewer, cid, func(c map[string]any) bool { return c["status"] == "paused" && c["pauseReason"] == "canary" })
	sum := c["summary"].(map[string]any)
	if sum["succeeded"].(float64) != 1 || sum["pending"].(float64) != 2 || sum["skipped"].(float64) != 1 {
		t.Fatalf("after canary: %v", sum)
	}
	if code, body := do(t, ts, operator, "POST", "/api/firmware/campaigns/"+cid+"/resume", ""); code != 409 || !strings.Contains(body, "confirm") {
		t.Errorf("resume during canary gate = %d %s", code, body)
	}
	if code, _ := do(t, ts, viewer, "POST", "/api/firmware/campaigns/"+cid+"/confirm", ""); code != 403 {
		t.Errorf("viewer confirm = %d", code)
	}
	// Explicit confirm = wave 2 gate.
	if code, body := do(t, ts, operator, "POST", "/api/firmware/campaigns/"+cid+"/confirm", ""); code != 200 || !strings.Contains(body, `"confirmed":true`) {
		t.Fatalf("confirm: %d %s", code, body)
	}
	c = waitCampaign(t, ts, viewer, cid, func(c map[string]any) bool { return c["status"] == "completed" })
	sum = c["summary"].(map[string]any)
	if sum["succeeded"].(float64) != 3 || sum["failed"].(float64) != 0 {
		t.Fatalf("final: %v", sum)
	}
	if fake.real != 3 {
		t.Errorf("real deliveries = %d, want 3", fake.real)
	}
	code, body = do(t, ts, viewer, "GET", "/api/firmware/campaigns/"+cid+"/devices", "")
	if code != 200 || strings.Count(body, `"status":"succeeded"`) != 3 || !strings.Contains(body, `"name":"printer-0"`) || !strings.Contains(body, "already at version") {
		t.Errorf("devices: %d %s", code, body)
	}
	code, body = do(t, ts, viewer, "GET", "/api/devices/"+ids[0]+"/firmware", "")
	if code != 200 || !strings.Contains(body, `"campaignName":"Roll out 2.15"`) || !strings.Contains(body, `"firmware":"2.15.0"`) {
		t.Errorf("history: %d %s", code, body)
	}
	code, body = do(t, ts, viewer, "GET", "/api/firmware/campaigns?status=completed", "")
	if code != 200 || !strings.Contains(body, cid) {
		t.Errorf("list: %d %s", code, body)
	}
	if code, _ := do(t, ts, viewer, "GET", "/api/firmware/campaigns", ""); code != 200 {
		t.Errorf("list all = %d", code)
	}
	// The job carries per-device results too.
	j, _ := json.Marshal(c["jobId"])
	code, body = do(t, ts, viewer, "GET", "/api/jobs/"+strings.Trim(string(j), `"`), "")
	if code != 200 || !strings.Contains(body, `"type":"firmware.campaign"`) || strings.Count(body, `"status":"ok"`) != 3 {
		t.Errorf("job: %d %s", code, body)
	}

	// Audit: campaign actions, package registration and every real delivery.
	code, body = do(t, ts, admin, "GET", "/api/audit?limit=500", "")
	for _, want := range []string{"firmware.package.create", "firmware.campaign.create", "firmware.campaign.update", "firmware.campaign.start", "firmware.campaign.confirm", "firmware.update"} {
		if !strings.Contains(body, `"action":"`+want+`"`) {
			t.Errorf("audit missing %s", want)
		}
	}
	if strings.Count(body, `"action":"firmware.update"`) != 3 {
		t.Errorf("firmware.update audit entries = %d, want 3", strings.Count(body, `"action":"firmware.update"`))
	}
	if code, _ := do(t, ts, viewer, "GET", "/api/audit", ""); code != 403 {
		t.Errorf("viewer audit = %d", code)
	}

	// Finished campaigns can be deleted, and then so can the package.
	if code, _ := do(t, ts, viewer, "DELETE", "/api/firmware/campaigns/"+cid, ""); code != 403 {
		t.Errorf("viewer delete = %d", code)
	}
	if code, _ := do(t, ts, operator, "DELETE", "/api/firmware/campaigns/"+cid, ""); code != 204 {
		t.Errorf("delete campaign = %d", code)
	}
	if code, _ := do(t, ts, admin, "DELETE", "/api/firmware/packages/"+pkg.ID, ""); code != 204 {
		t.Errorf("delete package = %d", code)
	}
	if code, _ := do(t, ts, viewer, "GET", "/api/firmware/packages/"+pkg.ID, ""); code != 404 {
		t.Errorf("package after delete = %d", code)
	}
}

// Spec §14 safety: a dry run through the API delivers nothing; cancel and
// pause are operator actions and audited.
func TestFirmwareAPIDryRunAndCancel(t *testing.T) {
	ts, s, fake := newFirmwareServer(t)
	ctx := context.Background()
	admin := login(t, ts, "admin", "adminpass123")
	operator := login(t, ts, "operator", "operatorpass1")
	_, body := upload(t, ts, admin, map[string]string{"vendor": "Acme", "version": "3.0"}, "fw-3.0.bin", "image")
	var pkg model.FirmwarePackage
	_ = json.Unmarshal([]byte(body), &pkg)
	var ids []string
	for i := 0; i < 3; i++ {
		d, _, _ := s.Store.UpsertDevice(ctx, &model.Identity{Address: fmt.Sprintf("10.0.1.%d", i), Serial: fmt.Sprintf("D%d", i), Make: "Acme", Model: "X", Firmware: "2.0", Method: "snmp"},
			map[model.Capability]string{model.CapFirmwareUpdate: "fake"}, nil)
		ids = append(ids, d.ID)
	}
	idsJSON, _ := json.Marshal(ids)
	code, body := do(t, ts, operator, "POST", "/api/firmware/campaigns", fmt.Sprintf(`{"name":"rehearsal","packageId":%q,"deviceIds":%s,"canaryCount":0}`, pkg.ID, idsJSON))
	if code != 201 {
		t.Fatalf("create: %d %s", code, body)
	}
	var cv struct {
		ID string `json:"id"`
	}
	_ = json.Unmarshal([]byte(body), &cv)
	if code, body := do(t, ts, operator, "POST", "/api/firmware/campaigns/"+cv.ID+"/start", `{"dryRun":true}`); code != 200 || !strings.Contains(body, `"dryRun":true`) {
		t.Fatalf("start dry run: %d %s", code, body)
	}
	c := waitCampaign(t, ts, operator, cv.ID, func(c map[string]any) bool { return c["status"] == "completed" })
	if sum := c["summary"].(map[string]any); sum["succeeded"].(float64) != 3 {
		t.Fatalf("dry run summary %v", sum)
	}
	if fake.real != 0 || fake.calls != 3 {
		t.Fatalf("dry run delivered: real=%d calls=%d", fake.real, fake.calls)
	}
	for _, id := range ids {
		d, _ := s.Store.GetDevice(ctx, id)
		if d.Firmware != "2.0" {
			t.Errorf("dry run changed %s to %s", id, d.Firmware)
		}
	}
	_, body = do(t, ts, admin, "GET", "/api/audit?limit=500", "")
	if strings.Contains(body, `"action":"firmware.update"`) {
		t.Error("dry run must not produce firmware.update audit entries")
	}

	// A second, real campaign is cancelled from its draft state.
	code, body = do(t, ts, operator, "POST", "/api/firmware/campaigns", fmt.Sprintf(`{"name":"real","packageId":%q,"deviceIds":%s}`, pkg.ID, idsJSON))
	if code != 201 {
		t.Fatalf("create: %d %s", code, body)
	}
	_ = json.Unmarshal([]byte(body), &cv)
	if code, body := do(t, ts, operator, "POST", "/api/firmware/campaigns/"+cv.ID+"/cancel", ""); code != 200 || !strings.Contains(body, `"status":"cancelled"`) {
		t.Errorf("cancel draft: %d %s", code, body)
	}
	if code, _ := do(t, ts, operator, "POST", "/api/firmware/campaigns/"+cv.ID+"/pause", ""); code != 409 {
		t.Errorf("pause cancelled = %d", code)
	}
	if code, _ := do(t, ts, operator, "POST", "/api/firmware/campaigns/nope/cancel", ""); code != 404 {
		t.Errorf("cancel unknown = %d", code)
	}
	_, body = do(t, ts, admin, "GET", "/api/audit?limit=500", "")
	if !strings.Contains(body, `"action":"firmware.campaign.cancel"`) || !strings.Contains(body, `"result":"refused:`) {
		t.Errorf("cancel and refused actions must be audited: %s", body)
	}
}

// A server without the firmware service answers 503 rather than panicking.
func TestFirmwareAPIUnconfigured(t *testing.T) {
	ts, _ := newTestServer(t)
	admin := login(t, ts, "admin", "adminpass123")
	if code, _ := do(t, ts, admin, "GET", "/api/firmware/inventory", ""); code != 503 {
		t.Errorf("unconfigured = %d", code)
	}
}
