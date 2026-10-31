package api

import (
	"context"
	"strings"
	"testing"
	"time"

	"printmarshal/internal/model"
	"printmarshal/internal/store"
)

// Issue 67: a supply alert's category follows the part it concerns. The key
// only carries the supply's index, so the part comes from the device's own
// supply data, and a part that cannot be identified is not called toner.
func TestPublicBoardSupplyAlertCategoryFollowsThePart(t *testing.T) {
	d := &model.Device{ID: "dev-1", Supplies: []model.Supply{
		{Index: 1, Type: "toner", Class: "consumed", Colorant: "black"},
		{Index: 2, Type: "tonerCartridge", Class: "consumed", Colorant: "cyan"},
		{Index: 3, Type: "toner-cartridge", Class: "consumed", Colorant: "magenta"},
		{Index: 4, Type: "ink", Class: "consumed", Colorant: "yellow"},
		{Index: 5, Type: "opc", Class: "consumed", Colorant: "black"},
		{Index: 6, Type: "developer", Class: "consumed"},
		{Index: 7, Type: "fuser", Class: "consumed"},
		{Index: 8, Type: "transferUnit", Class: "consumed"},
		{Index: 9, Type: "wasteToner", Class: "filled"},
		{Index: 10, Type: "waste-ink", Class: "filled"},
		{Index: 11, Type: "other", Class: "filled"}, // a receptacle, whatever it collects
		{Index: 12, Type: "toner", Class: "filled"}, // filling up is never a marker running out
		{Index: 13, Type: "other", Class: "consumed"},
		{Index: 14, Type: "unknown", Class: "consumed"},
		{Index: 15, Type: "", Class: "consumed"},
	}}
	cfg := publicStatusConfig{Enabled: true, Devices: []publicStatusDevice{{ID: d.ID, Label: "Library"}}, Show: publicShow{Alerts: true}}
	for _, c := range []struct {
		key, want string
	}{
		{"supply:1", "toner"},
		{"supply:2", "toner"},
		{"supply:3", "toner"},
		{"supply:4", "toner"},
		{"supply:5", "maintenance"},
		{"supply:6", "maintenance"},
		{"supply:7", "maintenance"},
		{"supply:8", "maintenance"},
		{"supply:9", "maintenance"},
		{"supply:10", "maintenance"},
		{"supply:11", "maintenance"},
		{"supply:12", "maintenance"},
		// The part cannot be told: broader, and still true.
		{"supply:13", "other"},
		{"supply:14", "other"},
		{"supply:15", "other"},
		{"supply:99", "other"}, // no longer reported by the device
		{"supply:", "other"},
		{"supply:abc", "other"},
		{"", "other"},
	} {
		alerts := []*model.Alert{{DeviceID: d.ID, Source: "supply", Key: c.key, Severity: model.SeverityCritical, Description: "Black Drum HP W9054MC at 5%"}}
		board := buildPublicBoard(cfg, []*model.Device{d}, alerts, time.Now(), 0, 15)
		if len(board.Devices) != 1 || board.Devices[0].Alerts == nil || len(*board.Devices[0].Alerts) != 1 {
			t.Fatalf("key %q: board = %+v", c.key, board)
		}
		got := (*board.Devices[0].Alerts)[0]
		if got.Category != c.want || got.Severity != "critical" {
			t.Errorf("key %q: alert = %+v, want category %q, severity critical", c.key, got, c.want)
		}
	}
}

// One device can hold a low toner and a low drum at once: each keeps its own
// category instead of collapsing into a single toner alert.
func TestPublicBoardTonerAndDrumAlertsStaySeparate(t *testing.T) {
	d := &model.Device{ID: "dev-1", Supplies: []model.Supply{
		{Index: 1, Type: "toner", Class: "consumed", Colorant: "black"},
		{Index: 2, Type: "opc", Class: "consumed", Colorant: "black"},
	}}
	cfg := publicStatusConfig{Enabled: true, Devices: []publicStatusDevice{{ID: d.ID}}, Show: publicShow{Alerts: true}}
	alerts := []*model.Alert{
		{DeviceID: d.ID, Source: "supply", Key: "supply:1", Severity: model.SeverityWarning},
		{DeviceID: d.ID, Source: "supply", Key: "supply:2", Severity: model.SeverityWarning},
	}
	board := buildPublicBoard(cfg, []*model.Device{d}, alerts, time.Now(), 0, 15)
	got := *board.Devices[0].Alerts
	if len(got) != 2 || got[0].Category != "toner" || got[1].Category != "maintenance" {
		t.Errorf("alerts = %+v, want toner then maintenance", got)
	}
}

// Through the real route: a low drum on a board with toner switched off says
// maintenance, never toner, and carries a severity and a fixed category only.
// The drum's description, part number and index stay out of the response.
func TestPublicStatusDrumAlertWithTonerOff(t *testing.T) {
	ts, s := newTestServer(t)
	admin := login(t, ts, "admin", "adminpass123")
	ctx := context.Background()
	d := seedBoardDevice(t, s.Store, 1, "Student print room")
	pct := func(v int) *int { return &v }
	if _, err := s.Store.ApplyPoll(ctx, d.ID, store.PollUpdate{
		Reachable: true, At: time.Now().UTC(),
		Status: &model.Status{Online: true, State: "idle", Severity: model.SeverityOK, At: time.Now().UTC()},
		Supplies: []model.Supply{
			{Index: 1, Description: "Black Cartridge HP W9040MC", Type: "toner", Class: "consumed", Colorant: "black", Level: 80, MaxCapacity: 100, Percent: pct(80)},
			{Index: 5, Description: "Black Drum HP W9054MC", Type: "opc", Class: "consumed", Colorant: "black", Level: 5, MaxCapacity: 100, Percent: pct(5)},
		},
	}); err != nil {
		t.Fatal(err)
	}
	if _, _, err := s.Store.RaiseAlert(ctx, d.ID, "supply:5", "supply", model.SeverityCritical, "Black Drum HP W9054MC at 5%"); err != nil {
		t.Fatal(err)
	}
	show := map[string]any{"alerts": true}
	if code, body := putBoard(t, ts, admin, map[string]any{"enabled": true, "devices": []map[string]any{{"id": d.ID}}, "show": show}); code != 200 {
		t.Fatalf("enable: %d %s", code, body)
	}
	resp, body := getPublic(t, ts, "/api/public/status", nil)
	if resp.StatusCode != 200 {
		t.Fatalf("board = %d %s", resp.StatusCode, body)
	}
	for _, leak := range []string{"Drum", "W9054MC", "supply", "5%"} {
		if strings.Contains(body, leak) {
			t.Errorf("response carries %q: %s", leak, body)
		}
	}
	list := boardDevices(t, decodeBoard(t, body))
	if len(list) != 1 {
		t.Fatalf("devices = %d, want 1", len(list))
	}
	wantKeys(t, "device (alerts only)", list[0], "label", "alerts")
	alerts := objects(t, list[0]["alerts"], "alerts")
	if len(alerts) != 1 {
		t.Fatalf("alerts = %v, want the one drum alert", list[0]["alerts"])
	}
	wantKeys(t, "alert entry", alerts[0], "severity", "category")
	if alerts[0]["severity"] != "critical" || alerts[0]["category"] != "maintenance" {
		t.Errorf("alert = %v, want critical maintenance", alerts[0])
	}
}
