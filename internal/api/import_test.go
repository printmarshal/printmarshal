package api

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"printmarshal/internal/auth"
	"printmarshal/internal/config"
	"printmarshal/internal/engine"
	"printmarshal/internal/fleet"
	"printmarshal/internal/importcsv"
	"printmarshal/internal/jobs"
	"printmarshal/internal/model"
	"printmarshal/internal/secrets"
	"printmarshal/internal/store"
)

// importServerOpts shape a test server for the CSV import: without a fleet
// (the preview must not need one), with a resolver, and with the import's
// identification replaced so no test ever probes an address.
type importServerOpts struct {
	noFleet  bool
	resolver *testResolver
	failID   map[string]bool // addresses the fake identification cannot identify
}

func newImportServer(t *testing.T, o importServerOpts) (*httptest.Server, *Server) {
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
	a := auth.New(st, time.Hour, false, log)
	ctx := context.Background()
	if err := a.Bootstrap(ctx, "adminpass123"); err != nil {
		t.Fatal(err)
	}
	if _, err := a.CreateUser(ctx, "viewer", "viewerpass1", model.RoleViewer); err != nil {
		t.Fatal(err)
	}
	if _, err := a.CreateUser(ctx, "op", "operatorpass1", model.RoleOperator); err != nil {
		t.Fatal(err)
	}
	s := &Server{Store: st, Auth: a, Hub: hub, Cfg: cfg, Log: log, Version: "test"}
	if !o.noFleet {
		runner := jobs.New(st, log, 1, hub.Publish)
		t.Cleanup(runner.Stop)
		eng := engine.New(engine.NewRegistry(), log, 2)
		fl := fleet.New(st, eng, runner, cfg, log, hub.Publish, nil)
		fl.Resolver = &testResolver{}
		if o.resolver != nil {
			fl.Resolver = o.resolver
		}
		s.Fleet = fl
	}
	s.importIdentify = func(ctx context.Context, address, hostname string) (*model.Device, bool, error) {
		if o.failID[address] {
			return nil, false, errors.New("no answer on SNMP or IPP")
		}
		return st.UpsertDevice(ctx, &model.Identity{Address: address, Hostname: hostname, Serial: "SN-" + address, Model: "Sim 1", Method: "snmp"}, nil, []string{"import"})
	}
	ts := httptest.NewServer(s.Handler())
	t.Cleanup(ts.Close)
	return ts, s
}

func importUpload(t *testing.T, ts *httptest.Server, token, mode, replace string, file []byte) (int, string) {
	t.Helper()
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	if mode != "" {
		_ = mw.WriteField("mode", mode)
	}
	if replace != "" {
		_ = mw.WriteField("replace", replace)
	}
	fw, _ := mw.CreateFormFile("file", "printers.csv")
	_, _ = fw.Write(file)
	_ = mw.Close()
	req, _ := http.NewRequest("POST", ts.URL+"/api/discovery/import", &buf)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, string(b)
}

func decodePlan(t *testing.T, body string) importcsv.PlanResult {
	t.Helper()
	var p importcsv.PlanResult
	if err := json.Unmarshal([]byte(body), &p); err != nil {
		t.Fatalf("plan: %v: %s", err, body)
	}
	return p
}

// applyAndWait applies a file as token and waits for the import job.
func applyAndWait(t *testing.T, ts *httptest.Server, s *Server, token, replace string, file []byte) (*model.Job, []*model.JobResult) {
	t.Helper()
	code, body := importUpload(t, ts, token, "apply", replace, file)
	if code != http.StatusAccepted {
		t.Fatalf("apply: %d %s", code, body)
	}
	var j model.Job
	if err := json.Unmarshal([]byte(body), &j); err != nil || j.ID == "" || j.Type != "import" {
		t.Fatalf("apply job: %v %s", err, body)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	done, err := s.Fleet.Jobs.Wait(ctx, j.ID)
	if err != nil || done.FinishedAt == nil {
		t.Fatalf("job did not finish: %+v %v", done, err)
	}
	results, err := s.Store.ListJobResults(context.Background(), j.ID)
	if err != nil {
		t.Fatal(err)
	}
	return done, results
}

func seedImportFleet(t *testing.T, s *Server) (a, b *model.Device, g *model.Group) {
	t.Helper()
	ctx := context.Background()
	var err error
	a, _, err = s.Store.UpsertDevice(ctx, &model.Identity{Address: "10.0.0.5", Serial: "SA", Model: "Sim 1", Method: "snmp"}, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	b, _, err = s.Store.UpsertDevice(ctx, &model.Identity{Address: "10.0.0.6", Serial: "SB", Model: "Sim 1", Method: "snmp"}, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	name := "Front desk"
	if a, err = s.Store.PatchDevice(ctx, a.ID, store.DevicePatch{DisplayName: &name}); err != nil {
		t.Fatal(err)
	}
	g = &model.Group{Name: "Floor 2", Kind: "static"}
	if err := s.Store.CreateGroup(ctx, g); err != nil {
		t.Fatal(err)
	}
	return a, b, g
}

const importFile = "Address,Display name,Location,Asset tag,Group,Owner\n" +
	"10.0.0.5,Reception,Floor 2,A-5,,Kim\n" + // 2 update: name held back
	"10.0.0.6,,Floor 3,,Floor 2,\n" + // 3 update and join the group
	"10.0.0.8,New one,Floor 4,A-8,Floor 2,\n" + // 4 create
	"10.0.0.5,,Floor 9,,,\n" + // 5 duplicate of row 2
	"10.0.0.99,,Floor 5,,,\n" // 6 create that identification cannot finish

// R6: the preview answers from the store alone; with no fleet at all it
// still works, so it cannot have contacted a device. Viewers cannot use it.
func TestImportPreviewMakesNoFleetCall(t *testing.T) {
	ts, s := newImportServer(t, importServerOpts{noFleet: true})
	a, b, _ := seedImportFleet(t, s)
	op := login(t, ts, "op", "operatorpass1")
	viewer := login(t, ts, "viewer", "viewerpass1")
	if code, _ := importUpload(t, ts, viewer, "preview", "", []byte(importFile)); code != http.StatusForbidden {
		t.Fatalf("viewer preview: %d", code)
	}
	withHostname := strings.Replace(importFile, "Owner\n", "Owner,Hostname\n", 1) + ",,,,,,lp-1\n"
	code, body := importUpload(t, ts, op, "preview", "", []byte(withHostname))
	if code != http.StatusOK {
		t.Fatalf("preview: %d %s", code, body)
	}
	if !strings.Contains(body, `"heldBack":[{"field":"displayName","reason":"set by an operator; column not marked replace"}]`) {
		t.Errorf("held-back JSON shape: %s", body)
	}
	p := decodePlan(t, body)
	if p.Separator != "," || strings.Join(p.KnownColumns, ",") != "address,displayName,location,assetTag,group,hostname" || strings.Join(p.IgnoredColumns, ",") != "Owner" {
		t.Errorf("columns: %+v", p)
	}
	if p.Totals != (importcsv.Totals{Create: 2, Update: 2, Skip: 2}) {
		t.Errorf("totals: %+v", p.Totals)
	}
	if r := p.Rows[0]; r.Outcome != "update" || r.DeviceID != a.ID || r.Fields["location"] != "Floor 2" || r.Fields["assetTag"] != "A-5" {
		t.Errorf("row 2: %+v", r)
	}
	if r := p.Rows[1]; r.Outcome != "update" || r.DeviceID != b.ID || r.Fields["group"] != "Floor 2" {
		t.Errorf("row 3: %+v", r)
	}
	if r := p.Rows[3]; r.Outcome != "skip" || r.Reason != "duplicate of row 2" {
		t.Errorf("row 5: %+v", r)
	}
	if r := p.Rows[5]; r.Key != "host:lp-1" || r.Outcome != "skip" || r.Reason != "no address: hostname lp-1 did not resolve" {
		t.Errorf("hostname row without a resolver: %+v", r)
	}
	// Nothing changed.
	if d, _ := s.Store.GetDevice(context.Background(), a.ID); d.Location != "" || d.AssetTag != "" || len(d.FieldSources) != 0 {
		t.Errorf("preview changed a device: %+v", d)
	}
	devs, _ := s.Store.ListDevices(context.Background(), store.DeviceFilter{IncludeDecommissioned: true})
	if len(devs) != 2 {
		t.Errorf("preview created devices: %d", len(devs))
	}
	// With a fleet, hostnames are looked up (forward DNS only).
	ts2, s2 := newImportServer(t, importServerOpts{resolver: &testResolver{hosts: map[string][]string{"lp-1": {"10.0.0.40"}}}})
	seedImportFleet(t, s2)
	_, body = importUpload(t, ts2, login(t, ts2, "op", "operatorpass1"), "preview", "", []byte("hostname,location\nlp-1,Floor 1\n"))
	if p := decodePlan(t, body); len(p.Rows) != 1 || p.Rows[0].Outcome != "create" || p.Rows[0].Key != "host:lp-1" {
		t.Errorf("resolved hostname: %s", body)
	}
}

// R9-R11, R13: apply is administrator-only; it creates and updates with
// field sources, holds back operator values unless replaced, skips
// duplicates, reports one result per row, and is audited without cell values.
func TestImportApply(t *testing.T) {
	ts, s := newImportServer(t, importServerOpts{failID: map[string]bool{"10.0.0.99": true}})
	a, b, g := seedImportFleet(t, s)
	ctx := context.Background()
	admin := login(t, ts, "admin", "adminpass123")
	op := login(t, ts, "op", "operatorpass1")
	if code, body := importUpload(t, ts, op, "apply", "", []byte(importFile)); code != http.StatusForbidden {
		t.Fatalf("operator apply: %d %s", code, body)
	}

	job, results := applyAndWait(t, ts, s, admin, "", []byte(importFile))
	if len(results) != 5 {
		t.Fatalf("%d results, want one per row: %+v", len(results), results)
	}
	byRow := map[int]*model.JobResult{}
	for _, r := range results {
		n, _ := r.Data["row"].(float64)
		byRow[int(n)] = r
	}
	want := map[int]string{2: "updated", 3: "updated", 4: "created", 5: "skipped", 6: "failed"}
	for row, status := range want {
		if r := byRow[row]; r == nil || r.Status != status {
			t.Errorf("row %d: %+v, want %s", row, r, status)
		}
	}
	if r := byRow[5]; r != nil && r.Message != "duplicate of row 2" {
		t.Errorf("duplicate message: %q", r.Message)
	}
	if job.Status != model.JobPartial || job.Failed != 1 || job.Done != 5 || job.Total != 5 {
		t.Errorf("job: %+v", job)
	}

	src := func(d *model.Device, field string) string { return d.FieldSources[field].Source }
	da, _ := s.Store.GetDevice(ctx, a.ID)
	if da.DisplayName != "Front desk" || da.Location != "Floor 2" || da.AssetTag != "A-5" || src(da, "location") != "import" || src(da, "assetTag") != "import" || src(da, "displayName") != "" {
		t.Errorf("updated device: %+v %v", da, da.FieldSources)
	}
	if at := da.FieldSources["location"].At; time.Since(at) > time.Minute {
		t.Errorf("source time %v", at)
	}
	db, _ := s.Store.GetDevice(ctx, b.ID)
	if db.Location != "Floor 3" || src(db, "location") != "import" {
		t.Errorf("second device: %+v", db)
	}
	created, err := s.Store.ListDevices(ctx, store.DeviceFilter{Q: "10.0.0.8"})
	if err != nil || len(created) != 1 {
		t.Fatalf("created device: %v %v", created, err)
	}
	dc := created[0]
	if dc.DisplayName != "New one" || dc.Location != "Floor 4" || dc.AssetTag != "A-8" || src(dc, "displayName") != "import" {
		t.Errorf("created device: %+v %v", dc, dc.FieldSources)
	}
	if byRow[4].DeviceID != dc.ID {
		t.Errorf("created result names %q, want %q", byRow[4].DeviceID, dc.ID)
	}
	gg, _ := s.Store.GetGroup(ctx, g.ID)
	if len(gg.MemberIDs) != 2 || !slices.Contains(gg.MemberIDs, b.ID) || !slices.Contains(gg.MemberIDs, dc.ID) {
		t.Errorf("group members %v", gg.MemberIDs)
	}
	if devs, _ := s.Store.ListDevices(ctx, store.DeviceFilter{Q: "10.0.0.99"}); len(devs) != 0 {
		t.Errorf("a failed identification created a device")
	}

	// Audit: one devices.import entry with counts and flags, never a cell value.
	entries, _ := s.Store.ListAudit(ctx, 100)
	var imp *model.AuditEntry
	createdAudit := 0
	for _, e := range entries {
		switch e.Action {
		case "devices.import":
			imp = e
		case "device.created":
			createdAudit++
		}
	}
	if imp == nil {
		t.Fatal("no devices.import audit entry")
	}
	raw, _ := json.Marshal(imp)
	for _, cell := range []string{"Floor", "Reception", "A-5", "New one", "Kim", "10.0.0."} {
		if strings.Contains(string(raw), cell) {
			t.Errorf("import audit entry carries %q: %s", cell, raw)
		}
	}
	if imp.After["created"] != float64(1) || imp.After["updated"] != float64(2) || imp.After["skipped"] != float64(1) || imp.After["failed"] != float64(1) || imp.After["rows"] != float64(5) {
		t.Errorf("import audit counts: %v", imp.After)
	}
	if createdAudit != 1 {
		t.Errorf("%d device.created entries", createdAudit)
	}

	// Replace: the display-name column now replaces the operator's value.
	_, results = applyAndWait(t, ts, s, admin, "Display name", []byte("address,display name\n10.0.0.5,Reception\n"))
	if len(results) != 1 || results[0].Status != "updated" {
		t.Fatalf("replace apply: %+v", results)
	}
	if da, _ = s.Store.GetDevice(ctx, a.ID); da.DisplayName != "Reception" || src(da, "displayName") != "import" {
		t.Errorf("replace not honoured: %+v", da)
	}

	// The import is listed with discovery runs (R15).
	code, body := do(t, ts, admin, "GET", "/api/discovery/runs", "")
	if code != 200 || !strings.Contains(body, `"id":"`+job.ID+`"`) || !strings.Contains(body, `"type":"import"`) {
		t.Errorf("runs list: %d %s", code, body)
	}
}

// R12: a group-scoped administrator updates only its devices and creates
// only into its groups. Its upload passes the scope check whole, however
// large (the check peeks at request bodies).
func TestImportGroupScope(t *testing.T) {
	ts, s := newImportServer(t, importServerOpts{})
	g, in, out := scopedFixture(t, s)
	ctx := context.Background()
	admin := login(t, ts, "admin", "adminpass123")
	tok := mkToken(t, ts, admin, `{"name":"site-admin","role":"admin","scope":["`+g.ID+`"]}`).Secret
	file := "address,location,group\n" +
		"10.0.0.10,Floor A,\n" + // in scope: update
		"10.0.0.11,Floor B,\n" + // outside: skip
		"10.0.0.12,Floor C,\n" + // a create the scope would not contain: skip
		"10.0.0.13,Floor D,site-a\n" // a create into its group
	code, body := importUpload(t, ts, tok, "preview", "", []byte(file))
	if code != 200 {
		t.Fatalf("scoped preview: %d %s", code, body)
	}
	p := decodePlan(t, body)
	if p.Rows[1].Reason != "outside your groups" || p.Rows[2].Reason != "outside your groups" || p.Totals != (importcsv.Totals{Create: 1, Update: 1, Skip: 2}) {
		t.Fatalf("scoped plan: %s", body)
	}
	_, results := applyAndWait(t, ts, s, tok, "", []byte(file))
	statuses := []string{}
	for _, r := range results {
		statuses = append(statuses, r.Status)
	}
	if strings.Join(statuses, ",") != "updated,skipped,skipped,created" {
		t.Fatalf("scoped apply: %v", statuses)
	}
	if d, _ := s.Store.GetDevice(ctx, out.ID); d.Location != "" {
		t.Errorf("out-of-scope device changed: %+v", d)
	}
	if d, _ := s.Store.GetDevice(ctx, in.ID); d.Location != "Floor A" {
		t.Errorf("in-scope device: %+v", d)
	}
	if gg, _ := s.Store.GetGroup(ctx, g.ID); len(gg.MemberIDs) != 2 {
		t.Errorf("created device not added to the scoped group: %v", gg.MemberIDs)
	}

	// A large file (1.5 MB) from a scoped principal arrives whole.
	var big strings.Builder
	big.WriteString("address,owner\n")
	pad := strings.Repeat("x", 290)
	for i := 0; i < 5000; i++ {
		big.WriteString("10.1." + strconv.Itoa(i/250) + "." + strconv.Itoa(i%250+1) + "," + pad + "\n")
	}
	code, body = importUpload(t, ts, tok, "preview", "", []byte(big.String()))
	if code != 200 {
		t.Fatalf("large scoped preview: %d %.200s", code, body)
	}
	if p := decodePlan(t, body); len(p.Rows) != 5000 || p.Totals.Skip != 5000 {
		t.Errorf("large scoped preview: %d rows %+v", len(p.Rows), p.Totals)
	}
}

// R1, R2: the file itself is refused with a one-line 400.
func TestImportRefusesBadFiles(t *testing.T) {
	ts, _ := newImportServer(t, importServerOpts{})
	admin := login(t, ts, "admin", "adminpass123")
	prefix := "address,owner\n10.2.0.1,"
	exact := prefix + strings.Repeat("x", importcsv.MaxBytes-len(prefix)-1) + "\n"
	if len(exact) != importcsv.MaxBytes {
		t.Fatalf("fixture is %d bytes", len(exact))
	}
	if code, body := importUpload(t, ts, admin, "preview", "", []byte(exact)); code != 200 {
		t.Fatalf("a file of exactly 2 MB: %d %.200s", code, body)
	}
	for _, tc := range []struct {
		name, mode, replace, file, want string
	}{
		{"over 2 MB", "preview", "", exact + "x", "file is over 2 MB"},
		{"no key column", "preview", "", "location\nFloor 2\n", "no key column"},
		{"not UTF-8", "preview", "", "address\n10.0.0.1\xff\n", "not UTF-8"},
		{"empty", "preview", "", "", "file is empty"},
		{"no mode", "", "", "address\n10.0.0.1\n", "mode must be"},
		{"bad mode", "dry", "", "address\n10.0.0.1\n", "mode must be"},
		{"bad replace", "preview", "address", "address\n10.0.0.1\n", "replace:"},
	} {
		code, body := importUpload(t, ts, admin, tc.mode, tc.replace, []byte(tc.file))
		if code != http.StatusBadRequest || !strings.Contains(body, tc.want) || strings.Count(body, "\n") > 1 {
			t.Errorf("%s: %d %s", tc.name, code, body)
		}
	}
	// No file part at all.
	if code, body := do(t, ts, admin, "POST", "/api/discovery/import", `{"mode":"preview"}`); code != http.StatusBadRequest {
		t.Errorf("JSON body: %d %s", code, body)
	}
}

// Spec decision 3: the import's copy of the export header is the header the
// device export writes, so an export imports into another instance.
func TestImportKnowsTheExportHeader(t *testing.T) {
	ts, _ := newImportServer(t, importServerOpts{})
	admin := login(t, ts, "admin", "adminpass123")
	req, _ := http.NewRequest("GET", ts.URL+"/api/devices.csv", nil)
	req.Header.Set("Authorization", "Bearer "+admin)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	line, _ := bufio.NewReader(resp.Body).ReadString('\n')
	if got, want := strings.TrimRight(line, "\r\n"), strings.Join(importcsv.ExportHeader, ","); got != want {
		t.Fatalf("export header\n got %s\nwant %s", got, want)
	}
}

// R11: the existing write paths record where a value came from: an edit of
// one device and a bulk edit say "operator", the DNS-sync rename says "dns".
func TestFieldSourcesOnEditPaths(t *testing.T) {
	ts, s := newImportServer(t, importServerOpts{resolver: &testResolver{
		ptrs:  map[string][]string{"10.0.0.6": {"copier-3f.example.net."}},
		hosts: map[string][]string{"copier-3f.example.net": {"10.0.0.6"}},
	}})
	a, b, _ := seedImportFleet(t, s)
	ctx := context.Background()
	admin := login(t, ts, "admin", "adminpass123")
	if code, body := do(t, ts, admin, "PATCH", "/api/devices/"+a.ID, `{"location":"Room 1","assetTag":"T-1"}`); code != 200 {
		t.Fatalf("patch: %d %s", code, body)
	}
	d, _ := s.Store.GetDevice(ctx, a.ID)
	if d.FieldSources["location"].Source != "operator" || d.FieldSources["assetTag"].Source != "operator" || d.FieldSources["notes"].Source != "" {
		t.Errorf("patch sources: %v", d.FieldSources)
	}
	if code, body := do(t, ts, admin, "POST", "/api/devices/bulk", `{"deviceIds":["`+b.ID+`"],"set":{"location":"Room 2","dnsSync":true}}`); code != 200 {
		t.Fatalf("bulk: %d %s", code, body)
	}
	d, _ = s.Store.GetDevice(ctx, b.ID)
	if d.FieldSources["location"].Source != "operator" || d.FieldSources["dnsSync"].Source != "operator" {
		t.Errorf("bulk sources: %v", d.FieldSources)
	}
	if code, body := do(t, ts, admin, "POST", "/api/devices/"+b.ID+"/refresh-hostname", ""); code != 200 {
		t.Fatalf("refresh hostname: %d %s", code, body)
	}
	d, _ = s.Store.GetDevice(ctx, b.ID)
	if d.DisplayName != "copier-3f" || d.FieldSources["displayName"].Source != "dns" || d.FieldSources["location"].Source != "operator" {
		t.Errorf("dns sync sources: %q %v", d.DisplayName, d.FieldSources)
	}
}

// R11, the other two operator write paths: a rename and a lifecycle change.
func TestFieldSourcesOnRenameAndState(t *testing.T) {
	ts, s := newImportServer(t, importServerOpts{})
	a, _, _ := seedImportFleet(t, s)
	ctx := context.Background()
	admin := login(t, ts, "admin", "adminpass123")
	if code, body := do(t, ts, admin, "POST", "/api/devices/rename", `{"deviceIds":["`+a.ID+`"],"source":"pattern","pattern":"P-{serial}","dryRun":false}`); code != 200 {
		t.Fatalf("rename: %d %s", code, body)
	}
	if code, body := do(t, ts, admin, "POST", "/api/devices/"+a.ID+"/state", `{"state":"onboarded"}`); code != 200 {
		t.Fatalf("state: %d %s", code, body)
	}
	d, _ := s.Store.GetDevice(ctx, a.ID)
	if d.DisplayName != "P-SA" || d.DisplayNameSource != "pattern" || d.State != model.StateOnboarded {
		t.Errorf("device: %+v", d)
	}
	if d.FieldSources["displayName"].Source != "operator" || d.FieldSources["state"].Source != "operator" {
		t.Errorf("sources: %v", d.FieldSources)
	}
}
