package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
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
	"printmarshal/internal/security"
	"printmarshal/internal/store"
)

// contactLog records every way a test server can reach a device address: the
// SNMP session factory (all driver reads and writes), the TLS and port probes
// and the web-page probe. It is the tripwire of these tests: a decommissioned
// device must not appear in it.
type contactLog struct {
	mu   sync.Mutex
	hits map[string][]string
}

func (c *contactLog) hit(address, how string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.hits[address] = append(c.hits[address], how)
}

func (c *contactLog) of(address string) []string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]string{}, c.hits[address]...)
}

func (c *contactLog) reset() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.hits = map[string][]string{}
}

type webProbeTripwire struct{ log *contactLog }

func (w webProbeTripwire) RoundTrip(r *http.Request) (*http.Response, error) {
	w.log.hit(r.URL.Hostname(), "web probe")
	return nil, errors.New("web probe disabled in tests")
}

const (
	retiredAddr = "10.0.0.5"
	liveAddr    = "10.0.0.6"
)

// newDecommissionedServer has two identified printers on a tripwired SNMP
// driver: live, and retired, which stays in service until a test calls retire
// so that the test can store a snapshot first.
func newDecommissionedServer(t *testing.T) (ts *httptest.Server, s *Server, retired, live *model.Device, contacts *contactLog, fx *snmp.Fixture) {
	t.Helper()
	fx, err := snmp.LoadFixture("../../testdata/fixtures/generic-laser.snmp")
	if err != nil {
		t.Fatal(err)
	}
	fx.Writable = true
	contacts = &contactLog{hits: map[string][]string{}}
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
		contacts.hit(address, "snmp")
		return fx, "v2c", nil
	}))
	eng := engine.New(reg, log, 2)
	fl := fleet.New(st, eng, runner, cfg, log, hub.Publish, nil)
	fl.WebClient = &http.Client{Transport: webProbeTripwire{contacts}}
	a := auth.New(st, time.Hour, false, log)
	if err := a.Bootstrap(context.Background(), "adminpass123"); err != nil {
		t.Fatal(err)
	}
	sec := security.New(fl.DepsOf())
	sec.TLSProbe = func(_ context.Context, address string, _ []int) []security.TLSReport {
		contacts.hit(address, "tls probe")
		return nil
	}
	sec.PortProbe = nil
	upsert := func(address, serial string) *model.Device {
		ident, caps, err := eng.Identify(context.Background(), address, model.Creds{})
		if err != nil {
			t.Fatal(err)
		}
		ident.Serial, ident.UUID, ident.MAC = serial, "", ""
		d, _, err := st.UpsertDevice(context.Background(), ident, caps, []string{"scan"})
		if err != nil {
			t.Fatal(err)
		}
		return d
	}
	retired, live = upsert(retiredAddr, "RETIRED1"), upsert(liveAddr, "LIVE1")
	if err := st.CreateCredential(context.Background(), &model.Credential{Name: "snmp write", Type: model.CredSNMPCommunity, Purpose: model.PurposeWrite}, &model.Secret{Community: "private"}); err != nil {
		t.Fatal(err)
	}
	s = &Server{Store: st, Fleet: fl, ConfigMgmt: configmgmt.New(fl.DepsOf()), Security: sec, Auth: a, Hub: hub, Cfg: cfg, Log: log, Version: "test"}
	ts = httptest.NewServer(s.Handler())
	t.Cleanup(ts.Close)
	contacts.reset()
	return ts, s, retired, live, contacts, fx
}

// retire decommissions the device through the API and clears the contact log,
// so anything logged afterwards happened to a decommissioned printer.
func retire(t *testing.T, ts *httptest.Server, token string, d *model.Device, contacts *contactLog) {
	t.Helper()
	if code, body := do(t, ts, token, "POST", "/api/devices/"+d.ID+"/state", `{"state":"decommissioned"}`); code != 200 {
		t.Fatalf("decommission: %d %s", code, body)
	}
	contacts.reset()
}

func assertNotContacted(t *testing.T, contacts *contactLog, d *model.Device, doing string) {
	t.Helper()
	if hits := contacts.of(d.Address); len(hits) != 0 {
		t.Errorf("%s contacted the decommissioned device: %v", doing, hits)
	}
}

type configBody struct {
	Document           *model.ConfigDocument `json:"document"`
	Fresh              bool                  `json:"fresh"`
	SnapshotID         string                `json:"snapshotId"`
	Contacted          *bool                 `json:"contacted"`
	NotContactedReason string                `json:"notContactedReason"`
}

func parseConfig(t *testing.T, body string) configBody {
	t.Helper()
	var c configBody
	if err := json.Unmarshal([]byte(body), &c); err != nil {
		t.Fatalf("config body %s: %v", body, err)
	}
	return c
}

func createLobbyProfile(t *testing.T, ts *httptest.Server, token string) model.Profile {
	t.Helper()
	code, body := do(t, ts, token, "POST", "/api/profiles", `{"name":"lobby","settings":[{"key":"identity.location","value":"Atrium"}]}`)
	if code != 201 {
		t.Fatalf("profile: %d %s", code, body)
	}
	var p model.Profile
	if err := json.Unmarshal([]byte(body), &p); err != nil {
		t.Fatal(err)
	}
	return p
}

func createTelnetPolicy(t *testing.T, ts *httptest.Server, token string) model.Policy {
	t.Helper()
	code, body := do(t, ts, token, "POST", "/api/policies", `{"name":"Telnet off","items":[{"id":"telnet","key":"protocols.telnet.enabled","operator":"eq","value":false,"severity":"critical","remediate":true}]}`)
	if code != 201 {
		t.Fatalf("policy: %d %s", code, body)
	}
	var p model.Policy
	if err := json.Unmarshal([]byte(body), &p); err != nil {
		t.Fatal(err)
	}
	return p
}

// Issue #65: looking at a decommissioned printer with nothing stored for it
// must not fall through to a live read.
func TestDecommissionedConfigViewWithoutSnapshotDoesNotContactDevice(t *testing.T) {
	ts, _, retired, _, contacts, _ := newDecommissionedServer(t)
	admin := login(t, ts, "admin", "adminpass123")
	retire(t, ts, admin, retired, contacts)

	code, body := do(t, ts, admin, "GET", "/api/devices/"+retired.ID+"/config", "")
	assertNotContacted(t, contacts, retired, "GET config")
	if code != 200 {
		t.Fatalf("GET config = %d %s", code, body)
	}
	c := parseConfig(t, body)
	if c.Document != nil {
		t.Errorf("no snapshot was stored, yet a document came back: %+v", c.Document)
	}
	if c.Fresh {
		t.Error("fresh must be false: nothing was read")
	}
	if c.Contacted == nil || *c.Contacted {
		t.Errorf("contacted must say false, got %v: %s", c.Contacted, body)
	}
	if c.NotContactedReason != "decommissioned" {
		t.Errorf("notContactedReason = %q, want decommissioned: %s", c.NotContactedReason, body)
	}
}

// The last stored snapshot is what a decommissioned printer shows.
func TestDecommissionedConfigViewReturnsStoredSnapshot(t *testing.T) {
	ts, _, retired, _, contacts, _ := newDecommissionedServer(t)
	admin := login(t, ts, "admin", "adminpass123")
	if code, body := do(t, ts, admin, "GET", "/api/devices/"+retired.ID+"/config?refresh=true", ""); code != 200 {
		t.Fatalf("store a snapshot: %d %s", code, body)
	}
	retire(t, ts, admin, retired, contacts)

	code, body := do(t, ts, admin, "GET", "/api/devices/"+retired.ID+"/config", "")
	assertNotContacted(t, contacts, retired, "GET config")
	if code != 200 {
		t.Fatalf("GET config = %d %s", code, body)
	}
	c := parseConfig(t, body)
	if c.Document == nil || len(c.Document.Settings) == 0 || c.SnapshotID == "" {
		t.Errorf("stored snapshot missing: %s", body)
	}
	if c.Fresh || c.Contacted == nil || *c.Contacted || c.NotContactedReason != "decommissioned" {
		t.Errorf("must say it was not contacted: %s", body)
	}
}

// Every other request the device page sends on load stays off the device too.
func TestDecommissionedPageLoadRequestsDoNotContactDevice(t *testing.T) {
	ts, _, retired, _, contacts, _ := newDecommissionedServer(t)
	admin := login(t, ts, "admin", "adminpass123")
	retire(t, ts, admin, retired, contacts)
	for _, path := range []string{
		"/api/devices/" + retired.ID,
		"/api/devices/" + retired.ID + "/config",
		"/api/devices/" + retired.ID + "/config/snapshots?limit=20",
		"/api/config/writes?device=" + retired.ID + "&limit=20",
		"/api/devices/" + retired.ID + "/security",
		"/api/devices/" + retired.ID + "/certificates",
		"/api/devices/" + retired.ID + "/supplies",
		"/api/devices/" + retired.ID + "/alerts",
		"/api/devices/" + retired.ID + "/telemetry?metric=counter.total",
		"/api/devices/" + retired.ID + "/logs",
	} {
		if code, body := do(t, ts, admin, "GET", path, ""); code >= 500 {
			t.Errorf("GET %s = %d %s", path, code, body)
		}
		assertNotContacted(t, contacts, retired, "GET "+path)
	}
}

// A deliberate action aimed only at a decommissioned printer is refused with
// 409 and a reason, before a job exists and without any contact. Dry runs read
// the device, so they are refused too.
func TestDecommissionedDeliberateActionsAreRefused(t *testing.T) {
	ts, s, retired, live, contacts, fx := newDecommissionedServer(t)
	admin := login(t, ts, "admin", "adminpass123")
	ctx := context.Background()
	id := retired.ID

	// A real write made while the printer was in service, to attempt a revert of.
	code, body := do(t, ts, admin, "POST", "/api/devices/"+id+"/config", `{"changes":[{"key":"identity.location","value":"Lobby"}],"dryRun":false}`)
	if code != 202 {
		t.Fatalf("apply: %d %s", code, body)
	}
	if j := waitJob(t, s, body); j.Status != model.JobSucceeded {
		t.Fatalf("apply job = %+v", j)
	}
	writes, err := s.Store.ListWriteRecords(ctx, store.WriteRecordFilter{DeviceID: id})
	if err != nil || len(writes) == 0 {
		t.Fatalf("write records: %v %v", writes, err)
	}
	profile := createLobbyProfile(t, ts, admin)
	policy := createTelnetPolicy(t, ts, admin)
	code, body = do(t, ts, admin, "POST", "/api/groups", `{"name":"retired only"}`)
	if code != 201 {
		t.Fatalf("group: %d %s", code, body)
	}
	var group model.Group
	if err := json.Unmarshal([]byte(body), &group); err != nil {
		t.Fatal(err)
	}
	if code, body := do(t, ts, admin, "PUT", "/api/groups/"+group.ID+"/members", `{"deviceIds":["`+id+`"]}`); code != 200 {
		t.Fatalf("members: %d %s", code, body)
	}

	retire(t, ts, admin, retired, contacts)
	setsBefore := len(fx.Sets())
	jobsBefore, _ := s.Store.ListJobs(ctx, store.JobFilter{})

	devs := fmt.Sprintf(`{"deviceIds":[%q]}`, id)
	for _, tc := range []struct{ name, method, path, body string }{
		{"read from device", "GET", "/api/devices/" + id + "/config?refresh=true", ""},
		{"diff with refresh", "GET", "/api/config/diff?a=" + id + "&b=" + live.ID + "&refresh=true", ""},
		{"poll now", "POST", "/api/devices/" + id + "/poll", ""},
		{"poll job", "POST", "/api/poll", devs},
		{"poll job for a group", "POST", "/api/poll", `{"groupId":"` + group.ID + `"}`},
		{"live certificates", "GET", "/api/devices/" + id + "/certificates?live=true", ""},
		{"assess", "POST", "/api/policies/" + policy.ID + "/assess", devs},
		{"remediate dry run", "POST", "/api/policies/" + policy.ID + "/remediate", `{"deviceIds":["` + id + `"],"dryRun":true}`},
		{"remediate", "POST", "/api/policies/" + policy.ID + "/remediate", `{"deviceIds":["` + id + `"],"dryRun":false}`},
		{"rotate dry run", "POST", "/api/security/rotate", `{"kind":"` + model.RotateSNMPCommunity + `","target":` + devs + `,"dryRun":true}`},
		{"rotate", "POST", "/api/security/rotate", `{"kind":"` + model.RotateSNMPCommunity + `","target":` + devs + `,"dryRun":false}`},
		{"enroll", "POST", "/api/devices/" + id + "/certificates/enroll", `{"caProfileId":"any","dryRun":false}`},
		{"install certificate", "POST", "/api/devices/" + id + "/certificates/any/install", `{}`},
		{"apply dry run", "POST", "/api/devices/" + id + "/config", `{"changes":[{"key":"identity.location","value":"X"}],"dryRun":true}`},
		{"apply", "POST", "/api/devices/" + id + "/config", `{"changes":[{"key":"identity.location","value":"X"}],"dryRun":false}`},
		{"profile apply dry run", "POST", "/api/profiles/" + profile.ID + "/apply", `{"target":` + devs + `,"dryRun":true}`},
		{"profile apply", "POST", "/api/profiles/" + profile.ID + "/apply", `{"target":` + devs + `,"dryRun":false}`},
		{"revert", "POST", "/api/config/writes/" + writes[0].ID + "/revert", `{"dryRun":false}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			code, body := do(t, ts, admin, tc.method, tc.path, tc.body)
			assertNotContacted(t, contacts, retired, tc.name)
			if code != 409 {
				t.Errorf("status = %d, want 409: %s", code, body)
			}
			if !strings.Contains(body, "decommissioned") {
				t.Errorf("the refusal must say why: %s", body)
			}
		})
	}

	// Anything that was queued anyway has to finish before the tripwire is read.
	jobsAfter, _ := s.Store.ListJobs(ctx, store.JobFilter{})
	for _, j := range jobsAfter {
		wctx, cancel := context.WithTimeout(ctx, 20*time.Second)
		_, _ = s.Fleet.Jobs.Wait(wctx, j.ID)
		cancel()
	}
	assertNotContacted(t, contacts, retired, "the refused requests")
	if got := len(fx.Sets()); got != setsBefore {
		t.Errorf("device writes went from %d to %d", setsBefore, got)
	}
	if len(jobsAfter) != len(jobsBefore) {
		t.Errorf("refused requests queued jobs: %d before, %d after", len(jobsBefore), len(jobsAfter))
	}
	// A refused real write leaves an audit entry saying so.
	entries, err := s.Store.ListAudit(ctx, 200)
	if err != nil {
		t.Fatal(err)
	}
	refused := false
	for _, e := range entries {
		refused = refused || (e.Result == "refused" && e.Target == id)
	}
	if !refused {
		t.Error("no audit entry records the refused write")
	}
}

// In a target that also names printers in service, the decommissioned one is a
// visible skip and the rest proceed, like the firmware and print-provider paths.
func TestDecommissionedInMixedTargetIsSkipped(t *testing.T) {
	ts, s, retired, live, contacts, fx := newDecommissionedServer(t)
	admin := login(t, ts, "admin", "adminpass123")
	retire(t, ts, admin, retired, contacts)
	policy := createTelnetPolicy(t, ts, admin)
	profile := createLobbyProfile(t, ts, admin)
	both := fmt.Sprintf(`{"deviceIds":[%q,%q]}`, retired.ID, live.ID)

	check := func(name, jobID string) {
		t.Helper()
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		if _, err := s.Fleet.Jobs.Wait(ctx, jobID); err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		results, err := s.Store.ListJobResults(context.Background(), jobID)
		if err != nil {
			t.Fatal(err)
		}
		statuses := map[string]string{}
		messages := map[string]string{}
		for _, r := range results {
			statuses[r.DeviceID], messages[r.DeviceID] = r.Status, r.Message
		}
		if statuses[retired.ID] != "skipped" || !strings.Contains(messages[retired.ID], "decommissioned") {
			t.Errorf("%s: decommissioned result = %q %q, want skipped with the reason", name, statuses[retired.ID], messages[retired.ID])
		}
		if statuses[live.ID] == "" || statuses[live.ID] == "skipped" {
			t.Errorf("%s: device in service result = %q", name, statuses[live.ID])
		}
		assertNotContacted(t, contacts, retired, name)
		if len(contacts.of(live.Address)) == 0 {
			t.Errorf("%s: the device in service was not contacted", name)
		}
		contacts.reset()
	}

	code, body := do(t, ts, admin, "POST", "/api/poll", both)
	if code != 202 {
		t.Fatalf("poll: %d %s", code, body)
	}
	var j model.Job
	if err := json.Unmarshal([]byte(body), &j); err != nil {
		t.Fatal(err)
	}
	check("poll", j.ID)

	code, body = do(t, ts, admin, "POST", "/api/policies/"+policy.ID+"/assess", both)
	if code != 202 {
		t.Fatalf("assess: %d %s", code, body)
	}
	j = model.Job{}
	if err := json.Unmarshal([]byte(body), &j); err != nil {
		t.Fatal(err)
	}
	check("assess", j.ID)
	// No assessment means no verdict: nothing is scored for the skipped printer.
	as, err := s.Store.LatestAssessments(context.Background(), policy.ID, []string{retired.ID})
	if err != nil || len(as) != 0 {
		t.Errorf("a skipped printer gained an assessment: %v %v", as, err)
	}

	setsBefore := len(fx.Sets())
	code, body = do(t, ts, admin, "POST", "/api/profiles/"+profile.ID+"/apply", `{"target":`+both+`,"dryRun":false}`)
	if code != 202 {
		t.Fatalf("apply: %d %s", code, body)
	}
	var resp struct {
		Job model.Job `json:"job"`
	}
	if err := json.Unmarshal([]byte(body), &resp); err != nil {
		t.Fatal(err)
	}
	check("apply", resp.Job.ID)
	if got := len(fx.Sets()); got <= setsBefore {
		t.Error("the device in service should have been written")
	}
	writes, _ := s.Store.ListWriteRecords(context.Background(), store.WriteRecordFilter{DeviceID: retired.ID})
	if len(writes) != 0 {
		t.Errorf("a write record exists for the decommissioned device: %+v", writes[0])
	}
}

// Stored configuration still answers comparisons; a comparison that would need
// a read says so instead of reading.
func TestDecommissionedDiffAndDriftUseStoredConfigOnly(t *testing.T) {
	ts, _, retired, live, contacts, _ := newDecommissionedServer(t)
	admin := login(t, ts, "admin", "adminpass123")
	profile := createLobbyProfile(t, ts, admin)
	drift := func(refresh bool) (int, string) {
		return do(t, ts, admin, "POST", "/api/profiles/"+profile.ID+"/drift", fmt.Sprintf(`{"target":{"deviceIds":[%q]},"refresh":%v}`, retired.ID, refresh))
	}

	retire(t, ts, admin, retired, contacts)
	code, body := do(t, ts, admin, "GET", "/api/config/diff?a="+retired.ID+"&b="+live.ID, "")
	assertNotContacted(t, contacts, retired, "diff with nothing stored")
	if code != 409 || !strings.Contains(body, "decommissioned") {
		t.Errorf("diff with nothing stored = %d %s", code, body)
	}
	for _, refresh := range []bool{false, true} {
		code, body = drift(refresh)
		assertNotContacted(t, contacts, retired, fmt.Sprintf("drift refresh=%v", refresh))
		if code != 200 || !strings.Contains(body, "decommissioned") {
			t.Errorf("drift refresh=%v = %d %s", refresh, code, body)
		}
	}

	// Back in service long enough to store a snapshot, then retired again.
	if code, body := do(t, ts, admin, "POST", "/api/devices/"+retired.ID+"/state", `{"state":"onboarded"}`); code != 200 {
		t.Fatalf("reinstate: %d %s", code, body)
	}
	if code, body := do(t, ts, admin, "GET", "/api/devices/"+retired.ID+"/config?refresh=true", ""); code != 200 {
		t.Fatalf("snapshot: %d %s", code, body)
	}
	if len(contacts.of(retired.Address)) == 0 {
		t.Error("a reinstated device must read again")
	}
	retire(t, ts, admin, retired, contacts)
	code, body = do(t, ts, admin, "GET", "/api/config/diff?a="+retired.ID+"&b="+live.ID, "")
	assertNotContacted(t, contacts, retired, "diff from the stored snapshot")
	if code != 200 {
		t.Errorf("diff from the stored snapshot = %d %s", code, body)
	}
	code, body = drift(false)
	assertNotContacted(t, contacts, retired, "drift from the stored snapshot")
	if code != 200 || !strings.Contains(body, `"drifted":1`) {
		t.Errorf("drift from the stored snapshot = %d %s", code, body)
	}
}

// A device in service is unaffected: same read, same response shape.
func TestDeviceInServiceStillReads(t *testing.T) {
	ts, _, _, live, contacts, _ := newDecommissionedServer(t)
	admin := login(t, ts, "admin", "adminpass123")
	code, body := do(t, ts, admin, "GET", "/api/devices/"+live.ID+"/config", "")
	if code != 200 {
		t.Fatalf("GET config = %d %s", code, body)
	}
	if len(contacts.of(live.Address)) == 0 {
		t.Error("with nothing stored, a device in service is read")
	}
	c := parseConfig(t, body)
	if c.Document == nil || c.NotContactedReason != "" || c.Contacted != nil {
		t.Errorf("response changed for a device in service: %s", body)
	}
	if code, body := do(t, ts, admin, "POST", "/api/devices/"+live.ID+"/poll", ""); code != 200 {
		t.Errorf("poll = %d %s", code, body)
	}
}
