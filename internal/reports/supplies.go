package reports

import (
	"context"
	"sort"
	"time"

	"printmarshal/internal/model"
)

// ReplacementJump is the minimum rise (percentage points) between two
// consecutive samples that counts as a supply replacement rather than noise.
const ReplacementJump = 20

// SupplyRow is one installed supply's level trend over the range.
type SupplyRow struct {
	DeviceID     string     `json:"deviceId"`
	DeviceName   string     `json:"deviceName"`
	Model        string     `json:"model"`
	Index        int        `json:"index"`
	Description  string     `json:"description"`
	StartPct     float64    `json:"startPct"` // level at the start of the range (last sample before it, else first in it)
	NowPct       float64    `json:"nowPct"`   // last sample in the range
	StartAt      time.Time  `json:"startAt"`
	NowAt        time.Time  `json:"nowAt"`
	Consumed     float64    `json:"consumed"`    // percentage points used (replacements excluded)
	RatePerDay   *float64   `json:"ratePerDay"`  // nil until at least half a day of history
	DaysToEmpty  *float64   `json:"daysToEmpty"` // nil when the rate is unknown or zero
	EmptyBy      *time.Time `json:"emptyBy,omitempty"`
	Replacements int        `json:"replacements"` // level jumps seen in the range
	Samples      int        `json:"samples"`
}

// ReplacementRow is a stock replacement event attributed to a device.
type ReplacementRow struct {
	At             time.Time `json:"at"`
	DeviceID       string    `json:"deviceId"`
	DeviceName     string    `json:"deviceName"`
	SupplyIndex    int       `json:"supplyIndex"`
	ConsumableID   string    `json:"consumableId"`
	ConsumableName string    `json:"consumableName"`
	Status         string    `json:"status"`
	Reason         string    `json:"reason"`
	Actor          string    `json:"actor"`
}

// ConsumptionRow is a consumable's stock movement over the range.
type ConsumptionRow struct {
	ConsumableID string `json:"consumableId"`
	Name         string `json:"name"`
	Unit         string `json:"unit"`
	UnitsOut     int    `json:"unitsOut"`
	UnitsIn      int    `json:"unitsIn"`
	Events       int    `json:"events"`
}

// SuppliesSummary is the supplies KPI set.
type SuppliesSummary struct {
	Supplies      int      `json:"supplies"`
	Declining     int      `json:"declining"` // supplies with a known positive rate
	DueWithin30   int      `json:"dueWithin30"`
	DueWithin7    int      `json:"dueWithin7"`
	Empty         int      `json:"empty"`
	Replacements  int      `json:"replacements"` // level jumps seen in telemetry
	StockEvents   int      `json:"stockEvents"`  // replacement stock events
	UnitsOut      int      `json:"unitsOut"`
	MeanRate      *float64 `json:"meanRate"` // mean %/day over declining supplies
	MeanConsumed  float64  `json:"meanConsumed"`
	FastestSupply string   `json:"fastestSupply,omitempty"`
}

// SuppliesReport is supply levels, rates, projections and stock movement.
type SuppliesReport struct {
	Range        Range            `json:"range"`
	Summary      SuppliesSummary  `json:"summary"`
	Supplies     []SupplyRow      `json:"supplies"` // sorted by days to empty asc (unknown last)
	Replacements []ReplacementRow `json:"replacements"`
	Consumption  []ConsumptionRow `json:"consumption"`
	// ConsumptionScoped is false when a device/group filter is set: stock
	// movements are not attributable to devices, so the table stays fleet-wide.
	ConsumptionScoped bool `json:"consumptionScoped"`
}

// Point is a timestamped level used by Trend.
type Point struct {
	At    time.Time
	Value float64
}

// Trend derives consumption from a level series: points used per unit of time,
// ignoring rises (replacements). Returns the points consumed, the span in
// days and the number of replacement jumps (rises of at least ReplacementJump).
func Trend(points []Point) (consumed, spanDays float64, replacements int) {
	if len(points) == 0 {
		return 0, 0, 0
	}
	for i := 1; i < len(points); i++ {
		diff := points[i-1].Value - points[i].Value
		switch {
		case diff > 0:
			consumed += diff
		case -diff >= ReplacementJump:
			replacements++
		}
	}
	spanDays = points[len(points)-1].At.Sub(points[0].At).Hours() / 24
	return consumed, spanDays, replacements
}

// Projection returns the %/day rate and days to empty for a level, or nils
// when the history is too short (< half a day) or nothing was consumed.
func Projection(consumed, spanDays, nowPct float64) (rate, daysToEmpty *float64) {
	if spanDays < 0.5 || consumed <= 0 {
		return nil, nil
	}
	r := round1(consumed / spanDays)
	if r <= 0 {
		r = 0.1
	}
	d := round1(nowPct / r)
	return &r, &d
}

// Supplies computes the supplies report.
func (s *Service) Supplies(ctx context.Context, p Params) (*SuppliesReport, error) {
	sc, err := s.resolve(ctx, p)
	if err != nil {
		return nil, err
	}
	daily, err := s.Store.DailyTelemetry(ctx, "supply.percent", sc.rangeOf())
	if err != nil {
		return nil, err
	}
	base, err := s.Store.LastBefore(ctx, "supply.percent", sc.rng.From, sc.ids)
	if err != nil {
		return nil, err
	}
	events, err := s.Store.ReplacementEvents(ctx, sc.rangeOf())
	if err != nil {
		return nil, err
	}
	cons, err := s.Store.StockConsumption(ctx, sc.rng.From, sc.rng.To)
	if err != nil {
		return nil, err
	}
	rep := &SuppliesReport{Range: sc.rng, Supplies: []SupplyRow{}, Replacements: []ReplacementRow{}, Consumption: []ConsumptionRow{}, ConsumptionScoped: sc.ids == nil}

	type key struct{ dev, label string }
	series := map[key][]Point{}
	var order []key
	samples := map[key]int{}
	for _, b := range base {
		k := key{b.DeviceID, b.Label}
		series[k] = append(series[k], Point{b.At, b.Value})
		order = append(order, k)
	}
	for _, a := range daily {
		k := key{a.DeviceID, a.Label}
		if _, ok := series[k]; !ok {
			order = append(order, k)
		}
		series[k] = append(series[k], Point{a.FirstAt, a.First})
		if a.Count > 1 {
			series[k] = append(series[k], Point{a.LastAt, a.Last})
		}
		samples[k] += a.Count
	}
	now := s.Now()
	var rateSum, consumedSum float64
	fastest := 0.0
	for _, k := range order {
		pts := series[k]
		if samples[k] == 0 { // baseline only: nothing in range
			continue
		}
		if _, known := sc.byID[k.dev]; !known && sc.ids != nil {
			continue
		}
		idx, desc := splitLabel(k.label)
		row := SupplyRow{DeviceID: k.dev, DeviceName: sc.name(k.dev), Model: sc.field(k.dev, func(d *model.Device) string { return d.Model }), Index: idx, Description: desc,
			StartPct: pts[0].Value, StartAt: pts[0].At, NowPct: pts[len(pts)-1].Value, NowAt: pts[len(pts)-1].At, Samples: samples[k]}
		consumed, span, repl := Trend(pts)
		row.Consumed, row.Replacements = round1(consumed), repl
		row.RatePerDay, row.DaysToEmpty = Projection(consumed, span, row.NowPct)
		if row.DaysToEmpty != nil {
			t := now.Add(time.Duration(*row.DaysToEmpty * 24 * float64(time.Hour)))
			row.EmptyBy = &t
			rep.Summary.Declining++
			rateSum += *row.RatePerDay
			if *row.DaysToEmpty <= 30 {
				rep.Summary.DueWithin30++
			}
			if *row.DaysToEmpty <= 7 {
				rep.Summary.DueWithin7++
			}
			if *row.RatePerDay > fastest {
				fastest, rep.Summary.FastestSupply = *row.RatePerDay, row.DeviceName+" · "+desc
			}
		}
		if row.NowPct <= 0 {
			rep.Summary.Empty++
		}
		consumedSum += consumed
		rep.Summary.Replacements += repl
		rep.Supplies = append(rep.Supplies, row)
	}
	rep.Summary.Supplies = len(rep.Supplies)
	if rep.Summary.Declining > 0 {
		m := round1(rateSum / float64(rep.Summary.Declining))
		rep.Summary.MeanRate = &m
	}
	if len(rep.Supplies) > 0 {
		rep.Summary.MeanConsumed = round1(consumedSum / float64(len(rep.Supplies)))
	}
	sort.SliceStable(rep.Supplies, func(i, j int) bool {
		a, b := rep.Supplies[i], rep.Supplies[j]
		switch {
		case a.DaysToEmpty != nil && b.DaysToEmpty != nil:
			return *a.DaysToEmpty < *b.DaysToEmpty
		case a.DaysToEmpty != nil:
			return true
		case b.DaysToEmpty != nil:
			return false
		}
		return a.NowPct < b.NowPct
	})
	names := map[string]string{}
	for _, c := range cons {
		names[c.ConsumableID] = c.Name
		rep.Consumption = append(rep.Consumption, ConsumptionRow{ConsumableID: c.ConsumableID, Name: c.Name, Unit: c.Unit, UnitsOut: c.UnitsOut, UnitsIn: c.UnitsIn, Events: c.Events})
		rep.Summary.UnitsOut += c.UnitsOut
	}
	for _, e := range events {
		rep.Replacements = append(rep.Replacements, ReplacementRow{At: e.At, DeviceID: e.DeviceID, DeviceName: sc.name(e.DeviceID), SupplyIndex: e.SupplyIndex,
			ConsumableID: e.ConsumableID, ConsumableName: names[e.ConsumableID], Status: e.Status, Reason: e.Reason, Actor: e.Actor})
	}
	rep.Summary.StockEvents = len(rep.Replacements)
	return rep, nil
}

// Tables implements CSV export: supplies (default), replacements, consumption.
func (r *SuppliesReport) Tables() []Table {
	sup := Table{Name: "supplies", Columns: []string{"deviceId", "device", "model", "supplyIndex", "supply", "startPct", "nowPct", "startAt", "nowAt", "consumed", "ratePerDay", "daysToEmpty", "replacements", "samples"}}
	for _, s := range r.Supplies {
		sup.Rows = append(sup.Rows, []string{s.DeviceID, s.DeviceName, s.Model, fmtI(s.Index), s.Description, fmtF(s.StartPct), fmtF(s.NowPct), s.StartAt.Format(time.RFC3339), s.NowAt.Format(time.RFC3339), fmtF(s.Consumed), fmtP(s.RatePerDay), fmtP(s.DaysToEmpty), fmtI(s.Replacements), fmtI(s.Samples)})
	}
	repl := Table{Name: "replacements", Columns: []string{"at", "deviceId", "device", "supplyIndex", "consumableId", "consumable", "status", "reason", "actor"}}
	for _, e := range r.Replacements {
		repl.Rows = append(repl.Rows, []string{e.At.Format(time.RFC3339), e.DeviceID, e.DeviceName, fmtI(e.SupplyIndex), e.ConsumableID, e.ConsumableName, e.Status, e.Reason, e.Actor})
	}
	cons := Table{Name: "consumption", Columns: []string{"consumableId", "name", "unit", "unitsOut", "unitsIn", "events"}}
	for _, c := range r.Consumption {
		cons.Rows = append(cons.Rows, []string{c.ConsumableID, c.Name, c.Unit, fmtI(c.UnitsOut), fmtI(c.UnitsIn), fmtI(c.Events)})
	}
	return []Table{sup, repl, cons}
}
