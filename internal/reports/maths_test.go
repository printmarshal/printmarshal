package reports

import (
	"testing"
	"time"

	"printmarshal/internal/store"
)

func f(v float64) *float64 { return &v }

func agg(first, last float64) store.DailyAgg {
	return store.DailyAgg{First: first, Last: last, Count: 2}
}

// Counter deltas: baseline → day deltas; a drop is a reset that counts 0 and
// re-bases; without a baseline the first day uses its in-day growth.
func TestDailyDeltas(t *testing.T) {
	days := []string{"d1", "d2", "d3", "d4", "d5"}
	byDay := map[string]store.DailyAgg{
		"d1": agg(1010, 1100),
		"d2": agg(1120, 1150),
		"d4": agg(5, 20), // reset (and a day without data in between)
		"d5": agg(25, 70),
	}
	pages, resets := DailyDeltas(days, f(1000), byDay)
	want := map[string]int64{"d1": 100, "d2": 50, "d4": 0, "d5": 50}
	if len(pages) != len(want) || resets != 1 {
		t.Fatalf("pages=%v resets=%d", pages, resets)
	}
	for d, n := range want {
		if pages[d] != n {
			t.Errorf("%s = %d, want %d", d, pages[d], n)
		}
	}
	pages, resets = DailyDeltas(days, nil, byDay)
	if pages["d1"] != 90 || pages["d2"] != 50 || resets != 1 {
		t.Errorf("no baseline: %v resets=%d", pages, resets)
	}
	// A reset inside the first day without baseline is clamped to 0.
	pages, resets = DailyDeltas([]string{"d1"}, nil, map[string]store.DailyAgg{"d1": agg(500, 10)})
	if pages["d1"] != 0 || resets != 1 {
		t.Errorf("in-day reset: %v %d", pages, resets)
	}
}

// Supply trend: declines accumulate, a jump of 20+ points is a replacement
// (not negative consumption), small rises are noise; projections need half a
// day of history.
func TestTrendAndProjection(t *testing.T) {
	t0 := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	pts := []Point{{t0, 80}, {t0.Add(24 * time.Hour), 70}, {t0.Add(48 * time.Hour), 72}, {t0.Add(72 * time.Hour), 60}, {t0.Add(96 * time.Hour), 100}, {t0.Add(120 * time.Hour), 90}}
	consumed, span, repl := Trend(pts)
	if consumed != 32 || span != 5 || repl != 1 {
		t.Errorf("consumed=%v span=%v repl=%d", consumed, span, repl)
	}
	rate, days := Projection(consumed, span, 90)
	if rate == nil || *rate != 6.4 || days == nil || *days != 14.1 {
		t.Errorf("rate=%v days=%v", rate, days)
	}
	if r, d := Projection(10, 0.2, 50); r != nil || d != nil {
		t.Error("too short a span must not project")
	}
	if r, d := Projection(0, 10, 50); r != nil || d != nil {
		t.Error("no consumption must not project")
	}
	if c, s, r := Trend(nil); c != 0 || s != 0 || r != 0 {
		t.Error("empty series")
	}
}

func TestFirmwareStatus(t *testing.T) {
	cases := []struct{ installed, latest, want string }{
		{"", "2.0", FirmwareUnknown}, {"1.0", "", FirmwareUnregistered}, {"1.0", "2.0", FirmwareOutdated}, {"2.0", "2.0", FirmwareCurrent}, {"2.1", "2.0", FirmwareCurrent},
	}
	for _, c := range cases {
		if got := FirmwareStatus(c.installed, c.latest); got != c.want {
			t.Errorf("FirmwareStatus(%q,%q) = %s, want %s", c.installed, c.latest, got, c.want)
		}
	}
}

func TestDayListAndMeans(t *testing.T) {
	from := time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)
	days := dayList(from, from.Add(48*time.Hour))
	if len(days) != 3 || days[0] != "2026-10-01" || days[2] != "2026-10-03" {
		t.Errorf("days = %v", days)
	}
	if d := dayList(from, from.Add(12*time.Hour)); len(d) != 1 {
		t.Errorf("same day = %v", d)
	}
	if m := MeanMinutes(30, 4); m == nil || *m != 7.5 {
		t.Errorf("mean = %v", m)
	}
	if MeanMinutes(30, 0) != nil {
		t.Error("mean of nothing must be nil")
	}
	if i, d := splitLabel("3:Cyan Toner"); i != 3 || d != "Cyan Toner" {
		t.Errorf("splitLabel = %d %q", i, d)
	}
}
