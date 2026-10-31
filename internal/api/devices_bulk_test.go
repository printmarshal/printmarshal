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
