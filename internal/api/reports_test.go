package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"printmarshal/internal/model"
)

// newReportServer mounts the report routes on their own mux so the branch is
// testable before server.go gains its s.reportRoutes(mux) line.
func newReportServer(t *testing.T) (*httptest.Server, *Server) {
	t.Helper()
	_, s := newTestServer(t)
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/auth/login", s.login)
	s.reportRoutes(mux)
	ts := httptest.NewServer(s.logging(secureHeaders(mux)))
	t.Cleanup(ts.Close)
	return ts, s
}

func TestReportsAPI(t *testing.T) {
	ts, s := newReportServer(t)
	ctx := context.Background()
	viewer := login(t, ts, "viewer", "viewerpass1")

	if code, _ := do(t, ts, "", "GET", "/api/reports/utilisation", ""); code != 401 {
		t.Errorf("unauthenticated = %d", code)
	}
	code, body := do(t, ts, viewer, "GET", "/api/reports", "")
	if code != 200 || !strings.Contains(body, `"available":true`) || !strings.Contains(body, `"fleet-health"`) {
		t.Errorf("list: %d %s", code, body)
	}

	d, _, _ := s.Store.UpsertDevice(ctx, &model.Identity{Address: "10.0.0.9", Serial: "R1", Make: "Acme", Model: "Laser 1", SysName: "Lobby", Firmware: "1.0", Method: "snmp"}, nil, nil)
	g := &model.Group{Name: "Floor", Kind: "static", MemberIDs: []string{d.ID}}
	if err := s.Store.CreateGroup(ctx, g); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	day := func(n int, h int) time.Time {
		return now.Add(-time.Duration(n) * 24 * time.Hour).Truncate(24 * time.Hour).Add(time.Duration(h) * time.Hour)
	}
	samples := []model.TelemetrySample{
		{DeviceID: d.ID, Metric: "counter.total", Value: 100, At: day(3, 12)},
		{DeviceID: d.ID, Metric: "counter.total", Value: 160, At: day(2, 12)},
		{DeviceID: d.ID, Metric: "counter.total", Value: 200, At: day(1, 12)},
		{DeviceID: d.ID, Metric: "status.online", Value: 1, At: day(2, 12)},
		{DeviceID: d.ID, Metric: "status.online", Value: 0, At: day(1, 12)},
		{DeviceID: d.ID, Metric: "status.online", Value: 1, At: day(1, 13)},
		{DeviceID: d.ID, Metric: "status.severity", Value: 1, At: day(1, 13)},
		{DeviceID: d.ID, Metric: "supply.percent", Label: "1:Black Toner", Value: 50, At: day(2, 12)},
		{DeviceID: d.ID, Metric: "supply.percent", Label: "1:Black Toner", Value: 40, At: day(1, 12)},
	}
	if err := s.Store.AddSamples(ctx, samples); err != nil {
		t.Fatal(err)
	}
	if _, _, err := s.Store.RaiseAlert(ctx, d.ID, "jam", "poll", model.SeverityCritical, "paper jam"); err != nil {
		t.Fatal(err)
	}

	// Viewer reads JSON: 7-day window by date, group scope.
	from, to := day(7, 0).Format("2006-01-02"), now.Format("2006-01-02")
	code, body = do(t, ts, viewer, "GET", "/api/reports/utilisation?from="+from+"&to="+to+"&group="+g.ID, "")
	if code != 200 {
		t.Fatalf("utilisation: %d %s", code, body)
	}
	var u struct {
		Range   struct{ Days, Devices int }
		Summary struct{ Pages int64 }
		Devices []struct{ Name string }
	}
	_ = json.Unmarshal([]byte(body), &u)
	if u.Range.Days != 8 || u.Range.Devices != 1 || u.Summary.Pages != 100 || len(u.Devices) != 1 || u.Devices[0].Name != "Lobby" {
		t.Errorf("utilisation = %+v %s", u, body)
	}
	// ?days= shorthand.
	code, body = do(t, ts, viewer, "GET", "/api/reports/fleet-health?days=7", "")
	if code != 200 || !strings.Contains(body, `"incidents":1`) || !strings.Contains(body, `"offlineMinutes":60`) {
		t.Errorf("health: %d %s", code, body)
	}
	code, body = do(t, ts, viewer, "GET", "/api/reports/supplies?days=7&device="+d.ID, "")
	if code != 200 || !strings.Contains(body, `"description":"Black Toner"`) || !strings.Contains(body, `"ratePerDay":10`) {
		t.Errorf("supplies: %d %s", code, body)
	}
	code, body = do(t, ts, viewer, "GET", "/api/reports/alerts?days=1", "")
	if code != 200 || !strings.Contains(body, `"total":1`) || !strings.Contains(body, `"topSource":"poll"`) {
		t.Errorf("alerts: %d %s", code, body)
	}
	code, body = do(t, ts, viewer, "GET", "/api/reports/firmware", "")
	if code != 200 || !strings.Contains(body, `"status":"unregistered"`) {
		t.Errorf("firmware: %d %s", code, body)
	}
	code, body = do(t, ts, viewer, "GET", "/api/reports/compliance", "")
	if code != 200 || !strings.Contains(body, `"available":false`) {
		t.Errorf("compliance: %d %s", code, body)
	}

	// Validation.
	if code, _ := do(t, ts, viewer, "GET", "/api/reports/utilisation?from=2026-13-01", ""); code != 400 {
		t.Errorf("bad date = %d", code)
	}
	if code, _ := do(t, ts, viewer, "GET", "/api/reports/utilisation?from=2026-10-10&to=2026-10-01", ""); code != 400 {
		t.Errorf("reversed = %d", code)
	}
	if code, _ := do(t, ts, viewer, "GET", "/api/reports/utilisation?from=2020-01-01&to=2026-01-01", ""); code != 400 {
		t.Errorf("too long = %d", code)
	}
	if code, _ := do(t, ts, viewer, "GET", "/api/reports/utilisation?group=nope", ""); code != 404 {
		t.Errorf("unknown group = %d", code)
	}
	if code, _ := do(t, ts, viewer, "GET", "/api/reports/utilisation?group=x&device=y", ""); code != 400 {
		t.Errorf("group and device = %d", code)
	}
	if code, _ := do(t, ts, viewer, "GET", "/api/reports/utilisation?format=csv&table=nope", ""); code != 400 {
		t.Errorf("unknown table = %d", code)
	}

	// CSV: same columns as the JSON table, default and named tables.
	req, _ := http.NewRequest("GET", ts.URL+"/api/reports/utilisation?days=7&format=csv", nil)
	req.Header.Set("Authorization", "Bearer "+viewer)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if ct := resp.Header.Get("Content-Type"); !strings.HasPrefix(ct, "text/csv") {
		t.Errorf("content type = %s", ct)
	}
	if cd := resp.Header.Get("Content-Disposition"); !strings.Contains(cd, "utilisation-devices.csv") {
		t.Errorf("disposition = %s", cd)
	}
	_, csvBody := do(t, ts, viewer, "GET", "/api/reports/utilisation?days=7&format=csv", "")
	lines := strings.Split(strings.TrimSpace(csvBody), "\n")
	if len(lines) != 2 || lines[0] != "deviceId,name,model,location,pages,mono,color,perDay,activeDays,dataDays,resets,idle" || !strings.Contains(lines[1], ",Lobby,Laser 1,,100,") {
		t.Errorf("csv = %q", csvBody)
	}
	_, csvBody = do(t, ts, viewer, "GET", "/api/reports/utilisation?days=7&format=csv&table=days", "")
	// now-7×24h starts on a partial day, so 8 calendar days plus the header.
	if !strings.HasPrefix(csvBody, "day,pages,mono,color,devices\n") || strings.Count(csvBody, "\n") != 9 {
		t.Errorf("days csv = %q", csvBody)
	}
	_, csvBody = do(t, ts, viewer, "GET", "/api/reports/supplies?days=7&format=csv&table=consumption", "")
	if !strings.HasPrefix(csvBody, "consumableId,name,unit,unitsOut,unitsIn,events\n") {
		t.Errorf("consumption csv = %q", csvBody)
	}
}

// A device that does not report a mono/colour split has unknown mono and
// colour, sent as null and exported as an empty cell, never 0, even when
// another device in the fleet reports a split; totals sum known values only
// and pagesKnown counts the devices that contributed a split (#46).
func TestUtilisationUnknownSplitIsNull(t *testing.T) {
	ts, s := newReportServer(t)
	ctx := context.Background()
	viewer := login(t, ts, "viewer", "viewerpass1")
	split, _, _ := s.Store.UpsertDevice(ctx, &model.Identity{Address: "10.0.0.41", Serial: "S1", Make: "Acme", Model: "Colour 1", SysName: "Split", Method: "snmp"}, nil, nil)
	plain, _, _ := s.Store.UpsertDevice(ctx, &model.Identity{Address: "10.0.0.42", Serial: "S2", Make: "Acme", Model: "Laser 2", SysName: "Plain", Method: "snmp"}, nil, nil)
	now := time.Now().UTC()
	day := func(n int) time.Time { return now.Add(-time.Duration(n) * 24 * time.Hour).Truncate(24 * time.Hour).Add(12 * time.Hour) }
	samples := []model.TelemetrySample{
		{DeviceID: split.ID, Metric: "counter.total", Value: 100, At: day(2)},
		{DeviceID: split.ID, Metric: "counter.total", Value: 150, At: day(1)},
		{DeviceID: split.ID, Metric: "counter.mono", Value: 70, At: day(2)},
		{DeviceID: split.ID, Metric: "counter.mono", Value: 110, At: day(1)},
		{DeviceID: split.ID, Metric: "counter.color", Value: 30, At: day(2)},
		{DeviceID: split.ID, Metric: "counter.color", Value: 40, At: day(1)},
		{DeviceID: plain.ID, Metric: "counter.total", Value: 500, At: day(2)},
		{DeviceID: plain.ID, Metric: "counter.total", Value: 520, At: day(1)},
	}
	if err := s.Store.AddSamples(ctx, samples); err != nil {
		t.Fatal(err)
	}
	code, body := do(t, ts, viewer, "GET", "/api/reports/utilisation?days=7", "")
	if code != 200 {
		t.Fatalf("utilisation: %d %s", code, body)
	}
	var u struct {
		Summary struct {
			Pages, Mono, Color int64
			HasSplit           bool
			PagesKnown         *int `json:"pagesKnown"`
		}
		Devices []struct {
			Name        string
			Pages       int64
			Mono, Color *int64
		}
	}
	if err := json.Unmarshal([]byte(body), &u); err != nil {
		t.Fatal(err)
	}
	byName := map[string]int{}
	for i, d := range u.Devices {
		byName[d.Name] = i
	}
	if len(u.Devices) != 2 {
		t.Fatalf("devices = %+v", u.Devices)
	}
	sd, pd := u.Devices[byName["Split"]], u.Devices[byName["Plain"]]
	if sd.Mono == nil || *sd.Mono != 40 || sd.Color == nil || *sd.Color != 10 {
		t.Errorf("split device mono/colour = %v/%v, want 40/10", sd.Mono, sd.Color)
	}
	if pd.Mono != nil || pd.Color != nil {
		t.Errorf("device without a split must send null mono/colour, got %v/%v", pd.Mono, pd.Color)
	}
	if !strings.Contains(body, `"mono":null`) {
		t.Errorf("no null mono in %s", body)
	}
	if u.Summary.Pages != 70 || u.Summary.Mono != 40 || u.Summary.Color != 10 || !u.Summary.HasSplit || u.Summary.PagesKnown == nil || *u.Summary.PagesKnown != 1 {
		t.Errorf("summary = %+v (pagesKnown %v)", u.Summary, u.Summary.PagesKnown)
	}
	_, csvBody := do(t, ts, viewer, "GET", "/api/reports/utilisation?days=7&format=csv", "")
	for _, line := range strings.Split(strings.TrimSpace(csvBody), "\n") {
		if strings.Contains(line, ",Plain,") && !strings.Contains(line, ",20,,,") {
			t.Errorf("CSV row for a device without a split must leave mono/colour empty: %q", line)
		}
	}
}
