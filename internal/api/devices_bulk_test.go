package api

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"printmarshal/internal/model"
	"printmarshal/internal/store"
)

type bulkResp struct {
	Changed int          `json:"changed"`
	Skipped int          `json:"skipped"`
	Results []bulkResult `json:"results"`
}

func TestBulkPatchDevices(t *testing.T) {
	ts, s := newTestServer(t)
	ctx := context.Background()
	admin := login(t, ts, "admin", "adminpass123")
	g, in, out := scopedFixture(t, s)
	// A device with a usable DNS name, one with a suspect one, one with a note already.
	if _, err := s.Store.SetDeviceDNS(ctx, in.ID, "copier-3f.example.net", false, in.FirstSeen); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Store.SetDeviceDNS(ctx, out.ID, "10-0-0-11.example.net", true, out.FirstSeen); err != nil {
		t.Fatal(err)
	}
	note := "Serviced 2026-10-01."
	if _, err := s.Store.PatchDevice(ctx, out.ID, store.DevicePatch{Notes: &note}); err != nil {
		t.Fatal(err)
	}

	// Validation.
	for _, body := range []string{`{}`, `{"deviceIds":["` + in.ID + `"]}`, `{"deviceIds":["` + in.ID + `"],"set":{"appendNote":"  "}}`} {
		if code, out := do(t, ts, admin, "POST", "/api/devices/bulk", body); code != 400 {
			t.Fatalf("body %s accepted: %d %s", body, code, out)
		}
	}
	// A viewer cannot; a scoped operator cannot reach a foreign device.
	viewer := login(t, ts, "viewer", "viewerpass1")
	if code, _ := do(t, ts, viewer, "POST", "/api/devices/bulk", `{"deviceIds":["`+in.ID+`"],"set":{"dnsSync":true}}`); code != 403 {
		t.Fatalf("viewer bulk edit: %d", code)
	}
	tok := mkToken(t, ts, admin, `{"name":"bot","role":"operator","scope":["`+g.ID+`"]}`).Secret
	if code, body := do(t, ts, tok, "POST", "/api/devices/bulk", `{"deviceIds":["`+in.ID+`","`+out.ID+`"],"set":{"dnsSync":true}}`); code != 403 || !strings.Contains(body, "outside your group scope") {
		t.Fatalf("scoped bulk edit reached a foreign device: %d %s", code, body)
	}

	// The real thing: rename from DNS, DNS sync on, a location, a note.
	code, body := do(t, ts, admin, "POST", "/api/devices/bulk", `{"deviceIds":["`+in.ID+`","`+out.ID+`","no-such-device"],"renameFromDns":true,"set":{"dnsSync":true,"location":"  Floor 3  ","appendNote":"Bulk edited."}}`)
	if code != 200 {
		t.Fatalf("bulk edit: %d %s", code, body)
	}
	var resp bulkResp
	if err := json.Unmarshal([]byte(body), &resp); err != nil {
		t.Fatal(err)
	}
	if resp.Changed != 2 || resp.Skipped != 1 || len(resp.Results) != 3 {
		t.Fatalf("totals: %+v", resp)
	}
	byID := map[string]bulkResult{}
	for _, r := range resp.Results {
		byID[r.ID] = r
	}
	if r := byID[in.ID]; strings.Join(r.Changed, ",") != "displayName,dnsSync,location,notes" || r.Skipped != "" {
		t.Fatalf("in-scope device result: %+v", r)
	}
	if r := byID[out.ID]; strings.Join(r.Changed, ",") != "dnsSync,location,notes" || !strings.Contains(r.Skipped, "suspect") {
		t.Fatalf("suspect-dns device result: %+v", r)
	}
	if r := byID["no-such-device"]; r.Skipped != "not found" {
		t.Fatalf("missing device result: %+v", r)
	}
	inDev, _ := s.Store.GetDevice(ctx, in.ID)
	outDev, _ := s.Store.GetDevice(ctx, out.ID)
	if inDev.DisplayName != "copier-3f.example.net" || inDev.DisplayNameSource != "dns" || !inDev.DNSSync || inDev.Location != "Floor 3" || inDev.Notes != "Bulk edited." {
		t.Fatalf("in-scope device after edit: %+v", inDev)
	}
	if outDev.DisplayName != "" || !outDev.DNSSync || outDev.Notes != "Serviced 2026-10-01.\n\nBulk edited." {
		t.Fatalf("suspect-dns device after edit: name=%q sync=%v notes=%q", outDev.DisplayName, outDev.DNSSync, outDev.Notes)
	}
	// Audited per device plus a summary row.
	entries, _ := s.Store.ListAudit(ctx, 20)
	patches, bulk := 0, 0
	for _, e := range entries {
		switch e.Action {
		case "device.patch":
			patches++
		case "devices.bulk":
			bulk++
		}
	}
	if patches < 2 || bulk != 1 {
		t.Fatalf("audit rows: device.patch=%d devices.bulk=%d", patches, bulk)
	}
	// A second identical edit changes nothing and says so.
	code, body = do(t, ts, admin, "POST", "/api/devices/bulk", `{"deviceIds":["`+in.ID+`"],"set":{"dnsSync":true,"location":"Floor 3"}}`)
	if code != 200 || !strings.Contains(body, `"skipped":"unchanged"`) {
		t.Fatalf("idempotent edit: %d %s", code, body)
	}
	_ = model.RoleOperator
}

func TestAppendNote(t *testing.T) {
	if got := appendNote("", "  new  "); got != "new" {
		t.Fatalf("empty existing: %q", got)
	}
	if got := appendNote("old\n", "new"); got != "old\n\nnew" {
		t.Fatalf("existing: %q", got)
	}
}

// intent/dns-rename-lookup: bulk rename from DNS looks up an empty DNS name.
func TestBulkRenameLooksUpEmptyDNSName(t *testing.T) {
	ts, s := newTestServer(t)
	s.Fleet.Resolver = &testResolver{
		ptrs:  map[string][]string{"10.0.0.31": {"copier-31.corp.example.com."}},
		hosts: map[string][]string{"copier-31.corp.example.com": {"10.0.0.31"}},
	}
	admin := login(t, ts, "admin", "adminpass123")
	ctx := context.Background()
	d, _, err := s.Store.UpsertDevice(ctx, &model.Identity{Address: "10.0.0.31", Serial: "SN31", Make: "Acme", Model: "Laser 31", Method: "snmp"}, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	code, body := do(t, ts, admin, "POST", "/api/devices/bulk", `{"deviceIds":["`+d.ID+`"],"renameFromDns":true}`)
	if code != 200 {
		t.Fatalf("bulk rename: %d %s", code, body)
	}
	var resp bulkResp
	if err := json.Unmarshal([]byte(body), &resp); err != nil {
		t.Fatal(err)
	}
	if resp.Changed != 1 || resp.Skipped != 0 || len(resp.Results) != 1 || strings.Join(resp.Results[0].Changed, ",") != "displayName" {
		t.Fatalf("bulk rename did not look up the empty DNS name: %s", body)
	}
	after, _ := s.Store.GetDevice(ctx, d.ID)
	if after.DisplayName != "copier-31" || after.DisplayNameSource != "dns" {
		t.Fatalf("after bulk rename: displayName=%q source=%q", after.DisplayName, after.DisplayNameSource)
	}
}

// A device already kept in sync is renamed by the lookup itself; the bulk
// result must still report the rename, not "unchanged".
func TestBulkRenameCountsSyncRename(t *testing.T) {
	ts, s := newTestServer(t)
	s.Fleet.Resolver = &testResolver{
		ptrs:  map[string][]string{"10.0.0.32": {"copier-32.corp.example.com."}},
		hosts: map[string][]string{"copier-32.corp.example.com": {"10.0.0.32"}},
	}
	admin := login(t, ts, "admin", "adminpass123")
	ctx := context.Background()
	d, _, err := s.Store.UpsertDevice(ctx, &model.Identity{Address: "10.0.0.32", Serial: "SN32", Make: "Acme", Model: "Laser 32", Method: "snmp"}, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	sync := true
	if _, err := s.Store.PatchDevice(ctx, d.ID, store.DevicePatch{DNSSync: &sync}); err != nil {
		t.Fatal(err)
	}
	code, body := do(t, ts, admin, "POST", "/api/devices/bulk", `{"deviceIds":["`+d.ID+`"],"renameFromDns":true}`)
	if code != 200 {
		t.Fatalf("bulk rename: %d %s", code, body)
	}
	var resp bulkResp
	if err := json.Unmarshal([]byte(body), &resp); err != nil {
		t.Fatal(err)
	}
	if resp.Changed != 1 || resp.Skipped != 0 || len(resp.Results) != 1 || strings.Join(resp.Results[0].Changed, ",") != "displayName" || resp.Results[0].Skipped != "" {
		t.Fatalf("sync rename reported as unchanged: %s", body)
	}
	after, _ := s.Store.GetDevice(ctx, d.ID)
	if after.DisplayName != "copier-32" || after.DisplayNameSource != "dns" || !after.DNSSync {
		t.Fatalf("after bulk rename: displayName=%q source=%q sync=%v", after.DisplayName, after.DisplayNameSource, after.DNSSync)
	}
}

// The bulk route names its devices in the body. A scoped principal can edit its
// own, and one foreign device in the list refuses the whole request.
func TestScopedBulkEdit(t *testing.T) {
	ts, s := newTestServer(t)
	admin := login(t, ts, "admin", "adminpass123")
	g, in, out := scopedFixture(t, s)
	tok := mkToken(t, ts, admin, `{"name":"bot","role":"operator","scope":["`+g.ID+`"]}`).Secret
	ctx := context.Background()

	if code, body := do(t, ts, tok, "POST", "/api/devices/bulk", `{"deviceIds":["`+in.ID+`"],"set":{"location":"Scoped room"}}`); code != 200 {
		t.Fatalf("own device: %d %s", code, body)
	}
	if d, _ := s.Store.GetDevice(ctx, in.ID); d.Location != "Scoped room" {
		t.Fatalf("own device not edited: %q", d.Location)
	}

	code, body := do(t, ts, tok, "POST", "/api/devices/bulk", `{"deviceIds":["`+in.ID+`","`+out.ID+`"],"set":{"location":"Everywhere"}}`)
	if code != 403 {
		t.Fatalf("foreign device: %d %s", code, body)
	}
	if d, _ := s.Store.GetDevice(ctx, in.ID); d.Location != "Scoped room" {
		t.Errorf("a refused request changed the device in scope: %q", d.Location)
	}
	if d, _ := s.Store.GetDevice(ctx, out.ID); d.Location == "Everywhere" {
		t.Errorf("a refused request changed the foreign device: %q", d.Location)
	}
	// The refusal is the body check's: the audit names the foreign device, not "bulk".
	code, audit := do(t, ts, admin, "GET", "/api/audit?limit=20", "")
	if code != 200 || !strings.Contains(audit, out.ID) {
		t.Errorf("the denial should name the foreign device: %d %s", code, audit)
	}

	for path, want := range map[string]string{"/api/devices/bulk": "", "/api/devices/rename": "", "/api/devices/abc": "abc", "/api/devices/abc/state": "abc"} {
		if got := devicePathID(path); got != want {
			t.Errorf("devicePathID(%q) = %q, want %q", path, got, want)
		}
	}
}
