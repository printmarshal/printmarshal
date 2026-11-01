package api

import (
	"context"
	"strings"
	"testing"
	"time"

	"printmarshal/internal/model"
)

func ids(devs []*model.Device) string {
	out := make([]string, len(devs))
	for i, d := range devs {
		out[i] = d.ID
	}
	return strings.Join(out, ",")
}

// #54: every column sorts, in either direction. Addresses are numeric, versions
// are in version order, and a device with no value stays last either way.
func TestSortDevices(t *testing.T) {
	pct := func(v int) *int { return &v }
	seen := time.Date(2026, 11, 1, 10, 0, 0, 0, time.UTC)
	a := &model.Device{ID: "a", Address: "10.0.0.10", DisplayName: "beta", Model: "M2", Location: "Lobby", Firmware: "2.10.0", State: "active", Severity: model.SeverityOK,
		Counters: &model.Counters{Total: 500}, LastSeen: seen.Add(time.Hour),
		Supplies: []model.Supply{{Index: 1, Class: "consumed", Percent: pct(60)}, {Index: 2, Class: "filled", Percent: pct(95)}}}
	b := &model.Device{ID: "b", Address: "10.0.0.2", DisplayName: "Alpha", Model: "m1", Location: "annex", Firmware: "2.9.0", State: "discovered", Severity: model.SeverityCritical,
		Counters: &model.Counters{Total: 1000}, LastSeen: seen,
		Supplies: []model.Supply{{Index: 1, Class: "consumed", Percent: pct(30)}}}
	c := &model.Device{ID: "c", Address: "printer.example", DisplayName: "Gamma", Severity: "offline", LastSeen: seen.Add(-time.Hour)}
	for _, tc := range []struct{ sort, dir, want string }{
		{"address", "", "b,a,c"},
		{"address", "asc", "b,a,c"},
		{"address", "desc", "c,a,b"},
		{"name", "desc", "c,a,b"},
		{"", "desc", "c,a,b"},
		{"model", "", "b,a,c"},
		{"location", "", "b,a,c"},
		{"location", "desc", "a,b,c"},
		{"firmware", "", "b,a,c"},
		{"firmware", "desc", "a,b,c"},
		{"state", "", "a,b,c"},
		{"severity", "", "b,c,a"},
		{"severity", "desc", "a,c,b"},
		// A receptacle at 95% full has 5% left, so a is the lowest.
		{"supplies", "", "a,b,c"},
		{"supplies", "desc", "b,a,c"},
		{"pages", "", "b,a,c"},
		{"pages", "asc", "a,b,c"},
		{"lastSeen", "", "a,b,c"},
		{"lastSeen", "asc", "c,b,a"},
	} {
		devs := []*model.Device{c, a, b}
		sortDevices(devs, tc.sort, tc.dir)
		if got := ids(devs); got != tc.want {
			t.Errorf("sort=%q dir=%q: %s, want %s", tc.sort, tc.dir, got, tc.want)
		}
	}
	// No sort and no direction leaves the store's order alone.
	devs := []*model.Device{c, a, b}
	sortDevices(devs, "", "")
	if got := ids(devs); got != "c,a,b" {
		t.Errorf("no sort: %s", got)
	}
}

// The route sorts before it pages, so a page is a page of the sorted list.
func TestDeviceListSortRoute(t *testing.T) {
	ts, s := newTestServer(t)
	admin := login(t, ts, "admin", "adminpass123")
	ctx := context.Background()
	seed := func(address string) {
		if _, _, err := s.Store.UpsertDevice(ctx, &model.Identity{Address: address, Serial: "S" + address, Method: "snmp"}, nil, nil); err != nil {
			t.Fatal(err)
		}
	}
	for _, a := range []string{"10.0.0.10", "10.0.0.2", "10.0.0.3"} {
		seed(a)
	}
	code, body := do(t, ts, admin, "GET", "/api/devices?sort=address&limit=2", "")
	if code != 200 || !strings.Contains(body, "10.0.0.2") || !strings.Contains(body, "10.0.0.3") || strings.Contains(body, "10.0.0.10") {
		t.Errorf("first page: %d %s", code, body)
	}
	code, body = do(t, ts, admin, "GET", "/api/devices?sort=address&limit=2&offset=2", "")
	if code != 200 || !strings.Contains(body, "10.0.0.10") || strings.Contains(body, "10.0.0.2\"") {
		t.Errorf("second page: %d %s", code, body)
	}
	code, body = do(t, ts, admin, "GET", "/api/devices.csv?sort=address&dir=desc", "")
	if code != 200 || strings.Index(body, "10.0.0.10") > strings.Index(body, "10.0.0.3") {
		t.Errorf("csv order: %d %s", code, body)
	}
}

// #54: Reports count retired printers, whose history still counts. The count is
// reported so the page can say so.
func TestReportRangeCountsDecommissioned(t *testing.T) {
	ts, s := newTestServer(t)
	admin := login(t, ts, "admin", "adminpass123")
	seedBoardDevice(t, s.Store, 1, "In service")
	retired := seedBoardDevice(t, s.Store, 2, "Retired")
	if code, body := do(t, ts, admin, "POST", "/api/devices/"+retired.ID+"/state", `{"state":"decommissioned"}`); code != 200 {
		t.Fatalf("decommission: %d %s", code, body)
	}
	code, body := do(t, ts, admin, "GET", "/api/reports/fleet-health", "")
	if code != 200 || !strings.Contains(body, `"devices":2`) || !strings.Contains(body, `"decommissioned":1`) {
		t.Errorf("fleet-health range: %d %s", code, body[:min(len(body), 300)])
	}
}
