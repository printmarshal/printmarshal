package reports

import (
	"context"
	"testing"
	"time"

	"printmarshal/internal/model"
	"printmarshal/internal/secrets"
	"printmarshal/internal/store"
)

func openStore(t *testing.T) *store.SQLite {
	t.Helper()
	box, _ := secrets.New(make([]byte, 32))
	s, err := store.OpenSQLite(t.TempDir()+"/t.db", box)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func ts(s string) time.Time {
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		panic(err)
	}
	return t
}

// Every report end to end against SQLite: two devices in a static group plus
// one outside it, three days of telemetry with a counter reset and an outage.
func TestReportsEndToEnd(t *testing.T) {
	st := openStore(t)
	ctx := context.Background()
	svc := New(st)
	svc.Now = func() time.Time { return ts("2026-10-04T00:00:00Z") }

	d1, _, _ := st.UpsertDevice(ctx, &model.Identity{Address: "10.0.0.1", Serial: "A", Make: "Acme", Model: "Laser 1", Firmware: "1.0", SysName: "Lobby", Method: "snmp"}, nil, nil)
	d2, _, _ := st.UpsertDevice(ctx, &model.Identity{Address: "10.0.0.2", Serial: "B", Make: "Acme", Model: "Laser 1", Firmware: "2.0", SysName: "Office", Method: "snmp"}, nil, nil)
	d3, _, _ := st.UpsertDevice(ctx, &model.Identity{Address: "10.0.0.3", Serial: "C", Make: "Acme", Model: "Ink 9", SysName: "Store", Method: "snmp"}, nil, nil)
	g := &model.Group{Name: "Floor 1", Kind: "static", MemberIDs: []string{d1.ID, d2.ID}}
	if err := st.CreateGroup(ctx, g); err != nil {
		t.Fatal(err)
	}
	rule := &model.Group{Name: "Lasers", Kind: "rule", Rule: &model.GroupRule{ModelContains: "laser"}}
	if err := st.CreateGroup(ctx, rule); err != nil {
		t.Fatal(err)
	}
	if err := st.CreateFirmwarePackage(ctx, &model.FirmwarePackage{Vendor: "Acme", Models: []string{"Laser 1"}, Version: "2.0", FileName: "fw.bin", SHA256: "x"}); err != nil {
		t.Fatal(err)
	}

	var samples []model.TelemetrySample
	add := func(dev, metric, label string, v float64, when string) {
		samples = append(samples, model.TelemetrySample{DeviceID: dev, Metric: metric, Label: label, Value: v, At: ts(when)})
	}
	// d1: 1000 baseline → 1100 (day1) → 1150 (day2) → reset to 20 (day3) → 60 (day3)
	add(d1.ID, "counter.total", "", 1000, "2026-09-30T23:00:00Z")
	add(d1.ID, "counter.total", "", 1100, "2026-10-01T18:00:00Z")
	add(d1.ID, "counter.total", "", 1150, "2026-10-02T18:00:00Z")
	add(d1.ID, "counter.total", "", 20, "2026-10-03T08:00:00Z")
	add(d1.ID, "counter.total", "", 60, "2026-10-03T18:00:00Z")
	add(d1.ID, "counter.mono", "", 900, "2026-09-30T23:00:00Z")
	add(d1.ID, "counter.mono", "", 980, "2026-10-01T18:00:00Z")
	add(d1.ID, "counter.color", "", 100, "2026-09-30T23:00:00Z")
	add(d1.ID, "counter.color", "", 120, "2026-10-01T18:00:00Z")
	// d2: no baseline, 300 → 300 (idle)
	add(d2.ID, "counter.total", "", 300, "2026-10-01T18:00:00Z")
	add(d2.ID, "counter.total", "", 300, "2026-10-03T18:00:00Z")
	// d3 (outside the group): 10 → 40
	add(d3.ID, "counter.total", "", 10, "2026-10-01T18:00:00Z")
	add(d3.ID, "counter.total", "", 40, "2026-10-02T18:00:00Z")
	// Reachability and severity.
	for _, day := range []string{"2026-10-01", "2026-10-02", "2026-10-03"} {
		add(d1.ID, "status.online", "", 1, day+"T06:00:00Z")
		add(d1.ID, "status.severity", "", 0, day+"T06:00:00Z")
		add(d2.ID, "status.online", "", 1, day+"T06:00:00Z")
		add(d2.ID, "status.severity", "", 1, day+"T06:00:00Z")
		add(d3.ID, "status.online", "", 1, day+"T06:00:00Z")
		add(d3.ID, "status.severity", "", 2, day+"T06:00:00Z")
	}
	add(d1.ID, "status.online", "", 0, "2026-10-02T12:00:00Z") // 60 min outage
	add(d1.ID, "status.online", "", 1, "2026-10-02T13:00:00Z")
	add(d1.ID, "status.severity", "", 2, "2026-10-02T13:00:00Z")
	// Supplies: d1 black toner 80 → 60 over two days, baseline 85 the day before.
	add(d1.ID, "supply.percent", "1:Black Toner", 85, "2026-09-30T23:00:00Z")
	add(d1.ID, "supply.percent", "1:Black Toner", 80, "2026-10-01T06:00:00Z")
	add(d1.ID, "supply.percent", "1:Black Toner", 60, "2026-10-03T06:00:00Z")
	// d2 cyan replaced: 5 → 100
	add(d2.ID, "supply.percent", "2:Cyan Toner", 5, "2026-10-01T06:00:00Z")
	add(d2.ID, "supply.percent", "2:Cyan Toner", 100, "2026-10-02T06:00:00Z")
	if err := st.AddSamples(ctx, samples); err != nil {
		t.Fatal(err)
	}
	// Alerts raised in range (timestamps rewritten through the store API are "now", so raise then patch).
	a1, _, _ := st.RaiseAlert(ctx, d1.ID, "jam", "poll", model.SeverityCritical, "paper jam")
	a2, _, _ := st.RaiseAlert(ctx, d2.ID, "toner", "supply", model.SeverityWarning, "toner low")
	_, _ = st.AckAlert(ctx, a1.ID, "op")
	_, _ = st.ClearAlert(ctx, a2.ID)
	if _, err := st.AdjustStock(ctx, "", nil); err == nil {
		t.Fatal("sanity: adjusting a missing consumable must fail")
	}

	p := Params{From: ts("2026-10-01T00:00:00Z"), To: ts("2026-10-04T00:00:00Z")}

	// Utilisation, fleet-wide.
	u, err := svc.Utilisation(ctx, p)
	if err != nil {
		t.Fatal(err)
	}
	if u.Range.Days != 3 || u.Range.Devices != 3 {
		t.Errorf("range = %+v", u.Range)
	}
	// d1: 100 + 50 + 0 (reset day) = 150; d2: 0; d3: 30 (no baseline → in-day growth 0 on day 1, +30 day 2)
	if u.Summary.Pages != 180 || u.Summary.Resets != 1 || u.Summary.IdleDevices != 1 || u.Summary.DevicesWithData != 3 || !u.Summary.HasSplit || u.Summary.Mono != 80 || u.Summary.Color != 20 {
		t.Errorf("summary = %+v", u.Summary)
	}
	if u.Devices[0].Name != "Lobby" || u.Devices[0].Pages != 150 || u.Devices[0].PerDay != 50 || u.Devices[0].ActiveDays != 2 || u.Devices[0].DataDays != 3 {
		t.Errorf("top device = %+v", u.Devices[0])
	}
	if u.Days[0].Pages != 100 || u.Days[1].Pages != 80 || u.Days[2].Pages != 0 || u.Days[2].Devices != 2 {
		t.Errorf("days = %+v", u.Days)
	}
	if u.Summary.PeakDay != "2026-10-01" || u.Summary.BusiestDevice != "Lobby" || u.Summary.BusiestDevicePct != 83.3 {
		t.Errorf("peak/busiest = %+v", u.Summary)
	}
	if len(u.Groups) != 2 || u.Groups[0].Pages != 150 || u.Groups[0].Devices != 2 {
		t.Errorf("groups = %+v", u.Groups)
	}
	// Scoped to the rule group: d3 excluded.
	u, err = svc.Utilisation(ctx, Params{From: p.From, To: p.To, Group: rule.ID})
	if err != nil || u.Range.Devices != 2 || u.Summary.Pages != 150 {
		t.Errorf("rule group: %+v %v", u.Summary, err)
	}
	u, err = svc.Utilisation(ctx, Params{From: p.From, To: p.To, Device: d3.ID, TopN: 1})
	if err != nil || u.Summary.Pages != 30 || len(u.Top) != 1 {
		t.Errorf("device scope: %+v %v", u.Summary, err)
	}
	if _, err := svc.Utilisation(ctx, Params{From: p.To, To: p.From}); err == nil {
		t.Error("reversed range must fail")
	}
	if _, err := svc.Utilisation(ctx, Params{Group: "nope"}); err == nil {
		t.Error("unknown group must fail")
	}

	// Fleet health.
	h, err := svc.FleetHealth(ctx, p)
	if err != nil {
		t.Fatal(err)
	}
	if h.Summary.Polled != 3 || h.Summary.Incidents != 1 || h.Summary.OfflineMinutes != 60 || h.Summary.DevicesOffline != 1 || h.Summary.DaysCritical != 3 {
		t.Errorf("health summary = %+v", h.Summary)
	}
	day2 := h.Days[1]
	if day2.Polled != 3 || day2.Reachable != 3 || day2.Critical != 2 || day2.Warning != 1 || day2.ReachablePct != 80 {
		t.Errorf("day 2 = %+v", day2)
	}
	if h.Devices[0].Name != "Lobby" || h.Devices[0].Polls != 5 || h.Devices[0].Failed != 1 || h.Devices[0].AvailabilityPct != 80 || h.Devices[0].WorstSeverity != "critical" || h.Devices[0].DaysCritical != 1 {
		t.Errorf("worst device = %+v", h.Devices[0])
	}
	if h.Summary.WorstDay != "2026-10-02" {
		t.Errorf("worst day = %+v", h.Summary)
	}

	// Supplies.
	sp, err := svc.Supplies(ctx, p)
	if err != nil {
		t.Fatal(err)
	}
	if sp.Summary.Supplies != 2 || sp.Summary.Declining != 1 || sp.Summary.Replacements != 1 || !sp.ConsumptionScoped {
		t.Errorf("supplies summary = %+v", sp.Summary)
	}
	bk := sp.Supplies[0]
	if bk.Description != "Black Toner" || bk.StartPct != 85 || bk.NowPct != 60 || bk.Consumed != 25 || bk.RatePerDay == nil || *bk.RatePerDay != 10.9 || bk.DaysToEmpty == nil || *bk.DaysToEmpty != 5.5 || bk.EmptyBy == nil {
		t.Errorf("black toner = %+v", bk)
	}
	cy := sp.Supplies[1]
	if cy.Description != "Cyan Toner" || cy.Replacements != 1 || cy.RatePerDay != nil || cy.NowPct != 100 {
		t.Errorf("cyan = %+v", cy)
	}

	// Alerts.
	al, err := svc.Alerts(ctx, Params{From: ts("2026-10-01T00:00:00Z"), To: time.Now().UTC().Add(time.Hour)})
	if err != nil {
		t.Fatal(err)
	}
	if al.Summary.Total != 2 || al.Summary.Critical != 1 || al.Summary.Acked != 1 || al.Summary.Cleared != 1 || al.Summary.MeanAckMinutes == nil || al.Summary.MeanClearMinutes == nil || al.Summary.DevicesAffected != 2 {
		t.Errorf("alerts summary = %+v", al.Summary)
	}
	if len(al.Sources) != 2 || al.Devices[0].Alerts != 1 || al.Summary.PeakCount != 2 {
		t.Errorf("alerts = %+v %+v", al.Sources, al.Devices)
	}

	// Firmware.
	fw, err := svc.Firmware(ctx, p)
	if err != nil {
		t.Fatal(err)
	}
	if fw.Summary.Devices != 3 || fw.Summary.Outdated != 1 || fw.Summary.Current != 1 || fw.Summary.Unknown != 1 || fw.Summary.Models != 2 || fw.Summary.Packages != 1 {
		t.Errorf("firmware summary = %+v", fw.Summary)
	}
	if fw.Devices[0].Status != FirmwareOutdated || fw.Devices[0].Installed != "1.0" || fw.Devices[0].Latest != "2.0" {
		t.Errorf("outdated first: %+v", fw.Devices[0])
	}
	if fw.Models[0].Model != "Laser 1" || fw.Models[0].Outdated != 1 || len(fw.Models[0].Versions) != 2 || fw.Models[0].Versions[0].Version != "2.0" {
		t.Errorf("model row = %+v", fw.Models[0])
	}

	// Compliance hook.
	c, err := svc.Compliance(ctx, p)
	if err != nil || c.Available || c.Reason == "" {
		t.Errorf("compliance = %+v %v", c, err)
	}

	// Tables carry the same columns as the JSON rows.
	for _, tb := range u.Tables() {
		for _, r := range tb.Rows {
			if len(r) != len(tb.Columns) {
				t.Errorf("table %s row width %d != %d columns", tb.Name, len(r), len(tb.Columns))
			}
		}
	}
	if tb := sp.Tables()[0]; len(tb.Rows) != 2 || tb.Rows[0][4] != "Black Toner" || tb.Rows[0][10] != "10.9" {
		t.Errorf("supplies table = %+v", tb.Rows)
	}
}
