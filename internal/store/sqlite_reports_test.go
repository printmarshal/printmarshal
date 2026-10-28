package store

import (
	"context"
	"testing"
	"time"

	"printmarshal/internal/model"
)

func day(t *testing.T, s string) time.Time {
	t.Helper()
	d, err := time.Parse(time.RFC3339, s)
	if err != nil {
		t.Fatal(err)
	}
	return d
}

// Report aggregates against seeded telemetry: last/first sample per day, the
// baseline before the range, offline minutes from status.online and alert
// statistics. The counter series includes a reset on day 3.
func TestReportAggregates(t *testing.T) {
	s := openTest(t)
	ctx := context.Background()
	d1, _, _ := s.UpsertDevice(ctx, &model.Identity{Address: "10.0.0.1", Serial: "A", Model: "M1", Method: "snmp"}, nil, nil)
	d2, _, _ := s.UpsertDevice(ctx, &model.Identity{Address: "10.0.0.2", Serial: "B", Model: "M1", Method: "snmp"}, nil, nil)
	at := func(s string) time.Time { return day(t, s) }
	var samples []model.TelemetrySample
	add := func(dev, metric, label string, v float64, when string) {
		samples = append(samples, model.TelemetrySample{DeviceID: dev, Metric: metric, Label: label, Value: v, At: at(when)})
	}
	// Baseline before the range, then three days; day 3 resets the counter.
	add(d1.ID, "counter.total", "", 1000, "2026-09-30T23:00:00Z")
	add(d1.ID, "counter.total", "", 1010, "2026-10-01T08:00:00.123456789Z")
	add(d1.ID, "counter.total", "", 1100, "2026-10-01T18:00:00Z")
	add(d1.ID, "counter.total", "", 1150, "2026-10-02T12:00:00Z")
	add(d1.ID, "counter.total", "", 20, "2026-10-03T12:00:00Z") // reset
	add(d2.ID, "counter.total", "", 500, "2026-10-02T12:00:00Z")
	// Reachability: d1 goes offline at 12:00 on day 2 and is back at 12:30.
	add(d1.ID, "status.online", "", 1, "2026-10-02T11:55:00Z")
	add(d1.ID, "status.online", "", 0, "2026-10-02T12:00:00Z")
	add(d1.ID, "status.online", "", 0, "2026-10-02T12:15:00Z")
	add(d1.ID, "status.online", "", 1, "2026-10-02T12:30:00Z")
	add(d2.ID, "status.online", "", 1, "2026-10-02T12:00:00Z")
	add(d1.ID, "supply.percent", "1:Black Toner", 80, "2026-10-01T08:00:00Z")
	add(d1.ID, "supply.percent", "1:Black Toner", 60, "2026-10-03T08:00:00Z")
	if err := s.AddSamples(ctx, samples); err != nil {
		t.Fatal(err)
	}
	r := ReportRange{From: at("2026-10-01T00:00:00Z"), To: at("2026-10-04T00:00:00Z")}

	daily, err := s.DailyTelemetry(ctx, "counter.total", r)
	if err != nil {
		t.Fatal(err)
	}
	if len(daily) != 4 {
		t.Fatalf("daily rows = %d: %+v", len(daily), daily)
	}
	find := func(dev, day string) DailyAgg {
		for _, a := range daily {
			if a.DeviceID == dev && a.Day == day {
				return a
			}
		}
		t.Fatalf("no aggregate for %s on %s", dev, day)
		return DailyAgg{}
	}
	first := find(d1.ID, "2026-10-01")
	if first.First != 1010 || first.Last != 1100 || first.Min != 1010 || first.Max != 1100 || first.Count != 2 {
		t.Errorf("day 1 agg = %+v", first)
	}
	if first.FirstAt.Hour() != 8 || first.LastAt.Hour() != 18 {
		t.Errorf("first/last at = %v %v", first.FirstAt, first.LastAt)
	}
	if reset := find(d1.ID, "2026-10-03"); reset.Last != 20 || reset.Count != 1 {
		t.Errorf("reset day = %+v", reset)
	}
	// Device filter: only d2.
	only, err := s.DailyTelemetry(ctx, "counter.total", ReportRange{From: r.From, To: r.To, DeviceIDs: []string{d2.ID}})
	if err != nil || len(only) != 1 || only[0].DeviceID != d2.ID {
		t.Errorf("filtered = %+v %v", only, err)
	}
	none, err := s.DailyTelemetry(ctx, "counter.total", ReportRange{From: r.From, To: r.To, DeviceIDs: []string{}})
	if err != nil || len(none) != 0 {
		t.Errorf("empty id set must match nothing: %+v %v", none, err)
	}

	base, err := s.LastBefore(ctx, "counter.total", r.From, nil)
	if err != nil || len(base) != 1 || base[0].DeviceID != d1.ID || base[0].Value != 1000 {
		t.Errorf("baseline = %+v %v", base, err)
	}

	off, err := s.OfflineStats(ctx, r)
	if err != nil {
		t.Fatal(err)
	}
	if len(off) != 2 {
		t.Fatalf("offline stats = %+v", off)
	}
	o1 := off[0]
	if o1.DeviceID != d1.ID {
		o1 = off[1]
	}
	if o1.Polls != 4 || o1.Failed != 2 || o1.Incidents != 1 || o1.OfflineMinutes < 29.9 || o1.OfflineMinutes > 30.1 {
		t.Errorf("d1 offline = %+v", o1)
	}

	sup, err := s.DailyTelemetry(ctx, "supply.percent", r)
	if err != nil || len(sup) != 2 || sup[0].Label != "1:Black Toner" || sup[1].Last != 60 {
		t.Errorf("supply agg = %+v %v", sup, err)
	}

	// Alerts: two on d1 (one acked after 10 min, one cleared), one on d2 outside the range.
	a1, _, _ := s.RaiseAlert(ctx, d1.ID, "k1", "poll", model.SeverityCritical, "paper jam")
	a2, _, _ := s.RaiseAlert(ctx, d1.ID, "k2", "supply", model.SeverityWarning, "toner low")
	_, _, _ = s.RaiseAlert(ctx, d2.ID, "k3", "poll", model.SeverityWarning, "old")
	set := func(id, col, v string) {
		if _, err := s.db.Exec(`UPDATE alerts SET `+col+`=? WHERE id=?`, v, id); err != nil {
			t.Fatal(err)
		}
	}
	set(a1.ID, "raised_at", "2026-10-02T10:00:00Z")
	set(a1.ID, "acked_at", "2026-10-02T10:10:00Z")
	set(a2.ID, "raised_at", "2026-10-03T10:00:00Z")
	set(a2.ID, "cleared_at", "2026-10-03T11:00:00Z")
	_, _ = s.db.Exec(`UPDATE alerts SET raised_at='2026-09-01T00:00:00Z' WHERE key='k3'`)

	days, err := s.AlertsByDay(ctx, r)
	if err != nil || len(days) != 2 || days[0].Day != "2026-10-02" || days[0].Severity != "critical" || days[0].Source != "poll" || days[0].Count != 1 {
		t.Errorf("alerts by day = %+v %v", days, err)
	}
	devs, err := s.AlertsByDevice(ctx, r)
	if err != nil || len(devs) != 1 {
		t.Fatalf("alerts by device = %+v %v", devs, err)
	}
	if st := devs[0]; st.DeviceID != d1.ID || st.Count != 2 || st.Critical != 1 || st.Warning != 1 || st.Acked != 1 || st.Cleared != 1 || st.AckMinutesSum < 9.9 || st.AckMinutesSum > 10.1 || st.ClearMinutesSum < 59.9 || st.ClearMinutesSum > 60.1 {
		t.Errorf("device alert stat = %+v", st)
	}

	// Stock: a replacement on d1 and operator movements.
	c := &model.Consumable{Name: "Black toner", Type: model.ConsumableToner, OnHand: 5}
	if err := s.CreateConsumable(ctx, c); err != nil {
		t.Fatal(err)
	}
	if _, err := s.AdjustStock(ctx, c.ID, &model.StockEvent{Kind: model.StockEventReplacement, Delta: -1, DeviceID: d1.ID, SupplyIndex: 1, Status: model.StockEventApplied, Reason: "Black Toner replaced"}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.AdjustStock(ctx, c.ID, &model.StockEvent{Kind: model.StockEventAdd, Delta: 3}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.AdjustStock(ctx, c.ID, &model.StockEvent{Kind: model.StockEventSet, Resulting: 7}); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	rr := ReportRange{From: now.Add(-time.Hour), To: now.Add(time.Hour), DeviceIDs: []string{d1.ID}}
	reps, err := s.ReplacementEvents(ctx, rr)
	if err != nil || len(reps) != 1 || reps[0].DeviceID != d1.ID {
		t.Errorf("replacements = %+v %v", reps, err)
	}
	reps, _ = s.ReplacementEvents(ctx, ReportRange{From: rr.From, To: rr.To, DeviceIDs: []string{d2.ID}})
	if len(reps) != 0 {
		t.Errorf("replacement filter leaked: %+v", reps)
	}
	cons, err := s.StockConsumption(ctx, rr.From, rr.To)
	if err != nil || len(cons) != 1 || cons[0].Name != "Black toner" || cons[0].UnitsOut != 1 || cons[0].UnitsIn != 3 || cons[0].Events != 2 {
		t.Errorf("consumption = %+v %v", cons, err)
	}
}
