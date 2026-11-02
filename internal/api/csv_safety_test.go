package api

import (
	"context"
	"strings"
	"testing"
	"time"

	"printmarshal/internal/model"
	"printmarshal/internal/store"
)

// A printer's name and supply descriptions come from the network. No CSV the
// console writes may hand them to a spreadsheet as a live formula.
func TestCSVExportsNeutraliseFormulas(t *testing.T) {
	ts, s := newTestServer(t)
	admin := login(t, ts, "admin", "adminpass123")
	ctx := context.Background()
	const evil = `=HYPERLINK("http://evil.example","x")`
	const quoted = `'=HYPERLINK(""http://evil.example"",""x"")`
	d := seedBoardDevice(t, s.Store, 1, evil)
	pct := 40
	if _, err := s.Store.ApplyPoll(ctx, d.ID, store.PollUpdate{Reachable: true, At: time.Now().UTC(),
		Status:   &model.Status{Online: true, State: "idle", Severity: model.SeverityOK, At: time.Now().UTC()},
		Supplies: []model.Supply{{Index: 1, Description: evil, Type: "toner", Class: "consumed", Level: 40, MaxCapacity: 100, Percent: &pct}}}); err != nil {
		t.Fatal(err)
	}
	if err := s.Store.AddSamples(ctx, []model.TelemetrySample{{DeviceID: d.ID, Metric: "status.online", Value: 1, At: time.Now().UTC()}}); err != nil {
		t.Fatal(err)
	}
	if code, body := do(t, ts, admin, "POST", "/api/consumables/import", "name,type,onHand,notes\n\"=HYPERLINK(1)\",toner,-5,\"+cmd\"\n"); code != 200 {
		t.Fatalf("seed consumable: %d %s", code, body)
	}
	for path, want := range map[string]string{
		"/api/devices.csv":  quoted,
		"/api/supplies.csv": quoted,
		"/api/reports/fleet-health?format=csv&table=devices": quoted,
		"/api/consumables.csv":                               "'=HYPERLINK(1)",
	} {
		code, body := do(t, ts, admin, "GET", path, "")
		if code != 200 {
			t.Fatalf("%s: %d %s", path, code, body)
		}
		if !strings.Contains(body, want) {
			t.Errorf("%s: no neutralised value %q in:\n%s", path, want, body)
		}
		for _, line := range strings.Split(body, "\n") {
			if strings.HasPrefix(line, `"=`) || strings.Contains(line, `,"=`) || strings.Contains(line, ",=") {
				t.Errorf("%s: live formula in %q", path, line)
			}
		}
	}

	// The console's own round trip changes nothing: the value comes back without
	// the apostrophe, and a real negative number is still a number.
	code, body := do(t, ts, admin, "GET", "/api/consumables.csv", "")
	if code != 200 {
		t.Fatalf("export: %d", code)
	}
	if !strings.Contains(body, "'+cmd") {
		t.Errorf("notes not neutralised: %s", body)
	}
	if code, b := do(t, ts, admin, "POST", "/api/consumables/import", body); code != 200 {
		t.Fatalf("re-import: %d %s", code, b)
	}
	items, err := s.Store.ListConsumables(ctx, store.ConsumableFilter{})
	if err != nil || len(items) != 1 {
		t.Fatalf("round trip made %d items (%v): %+v", len(items), err, items)
	}
	if items[0].Name != "=HYPERLINK(1)" || items[0].Notes != "+cmd" {
		t.Errorf("round trip changed the values: %q %q", items[0].Name, items[0].Notes)
	}
}
