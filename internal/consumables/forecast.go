package consumables

import (
	"math"
	"sort"
	"time"

	"printmarshal/internal/model"
)

// Rate is the consumption rate of one installed supply.
type Rate struct {
	PercentPerDay float64 `json:"percentPerDay"`
	Samples       int     `json:"samples"`
	SpanDays      float64 `json:"spanDays"`
	Known         bool    `json:"known"`
	Reason        string  `json:"reason,omitempty"` // why unknown
}

// ConsumptionRate estimates how fast a supply level drops from its telemetry
// (consumables spec §3). Rises of at least jumpPct are treated as replacements
// and excluded; smaller rises are measurement noise and count as no change.
// filled is true for receptacles (waste) whose level rises as they are used.
// The rate is unknown until the history spans minDays with enough samples.
func ConsumptionRate(samples []model.TelemetrySample, filled bool, jumpPct int, minDays float64) Rate {
	r := Rate{Samples: len(samples)}
	if len(samples) < 4 {
		r.Reason = "not enough samples yet"
		return r
	}
	sorted := make([]model.TelemetrySample, len(samples))
	copy(sorted, samples)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].At.Before(sorted[j].At) })
	val := func(s model.TelemetrySample) float64 {
		if filled {
			return 100 - s.Value
		}
		return s.Value
	}
	var drop, days float64
	for i := 1; i < len(sorted); i++ {
		d := val(sorted[i-1]) - val(sorted[i])
		dt := sorted[i].At.Sub(sorted[i-1].At).Hours() / 24
		if dt <= 0 {
			continue
		}
		if d < 0 && -d >= float64(jumpPct) {
			continue // replacement: a new unit went in
		}
		if d < 0 {
			d = 0
		}
		drop += d
		days += dt
	}
	r.SpanDays = sorted[len(sorted)-1].At.Sub(sorted[0].At).Hours() / 24
	if r.SpanDays < minDays {
		r.Reason = "history too short"
		return r
	}
	if days <= 0 || drop <= 0 {
		r.Reason = "no consumption observed"
		return r
	}
	r.PercentPerDay = drop / days
	r.Known = true
	return r
}

// DeviceForecast is one installed supply's projection.
type DeviceForecast struct {
	DeviceID    string   `json:"deviceId"`
	DeviceName  string   `json:"deviceName"`
	SupplyIndex int      `json:"supplyIndex"`
	Description string   `json:"description"`
	Percent     *int     `json:"percent"`
	Rate        Rate     `json:"rate"`
	DaysToEmpty *float64 `json:"daysToEmpty"` // nil when unknown
}

// ItemForecast projects a stock item across every device that uses it.
type ItemForecast struct {
	ConsumableID string            `json:"consumableId"`
	Name         string            `json:"name"`
	OnHand       int               `json:"onHand"`
	Status       model.StockStatus `json:"status"`
	LinkedCount  int               `json:"linkedDevices"`
	UnitsPerDay  float64           `json:"unitsPerDay"`
	StockDays    *float64          `json:"stockDays"` // days until on-hand stock is used up; nil when unknown
	ReorderSoon  bool              `json:"reorderSoon"`
	Known        bool              `json:"known"`
	Reason       string            `json:"reason,omitempty"`
	Devices      []DeviceForecast  `json:"devices"`
	NextNeed     *float64          `json:"nextNeedDays"` // soonest device replacement, days
}

// forecastHorizonDays bounds the replacement simulation.
const forecastHorizonDays = 3 * 365

// Project combines per-device rates into a stock projection (consumables spec §3).
// Each device needs a replacement when its current unit empties and then every
// 100/rate days; stock is depleted at the OnHand-th replacement.
func Project(item *model.Consumable, devs []DeviceForecast, leadDays int) ItemForecast {
	f := ItemForecast{ConsumableID: item.ID, Name: item.Name, OnHand: item.OnHand, Status: item.Status(), LinkedCount: len(devs), Devices: devs}
	if f.Devices == nil {
		f.Devices = []DeviceForecast{}
	}
	var times []float64
	known := 0
	for _, d := range devs {
		if !d.Rate.Known || d.DaysToEmpty == nil {
			continue
		}
		known++
		f.UnitsPerDay += d.Rate.PercentPerDay / 100
		if f.NextNeed == nil || *d.DaysToEmpty < *f.NextNeed {
			v := *d.DaysToEmpty
			f.NextNeed = &v
		}
		period := 100 / d.Rate.PercentPerDay
		for t := *d.DaysToEmpty; t <= forecastHorizonDays; t += period {
			times = append(times, t)
		}
	}
	if len(devs) == 0 {
		f.Reason = "no linked devices"
		return f
	}
	if known == 0 {
		f.Reason = "not enough history yet"
		return f
	}
	f.Known = true
	sort.Float64s(times)
	switch {
	case item.OnHand <= 0:
		z := 0.0
		f.StockDays = &z
	case len(times) >= item.OnHand:
		v := times[item.OnHand-1]
		f.StockDays = &v
	default:
		f.Reason = "stock outlasts the forecast horizon"
	}
	f.UnitsPerDay = math.Round(f.UnitsPerDay*1000) / 1000
	if f.StockDays != nil && *f.StockDays <= float64(leadDays) {
		f.ReorderSoon = true
	}
	return f
}

// Replacement is an inferred supply replacement between two polls.
type Replacement struct {
	Index       int    `json:"index"`
	Description string `json:"description"`
	From        int    `json:"from"`
	To          int    `json:"to"`
}

// DetectReplacements compares consecutive supply readings and reports slots
// whose level rose by at least jumpPct (fell, for receptacles). A slot absent
// from either reading is ignored.
func DetectReplacements(prev, cur []model.Supply, jumpPct int) []Replacement {
	if jumpPct <= 0 {
		jumpPct = 30
	}
	before := map[int]model.Supply{}
	for _, s := range prev {
		before[s.Index] = s
	}
	var out []Replacement
	for _, s := range cur {
		p, ok := before[s.Index]
		if !ok || p.Percent == nil || s.Percent == nil || s.Class == "other" {
			continue
		}
		from, to := *p.Percent, *s.Percent
		if s.Class == "filled" {
			from, to = 100-from, 100-to
		}
		if to-from >= jumpPct {
			out = append(out, Replacement{Index: s.Index, Description: s.Description, From: *p.Percent, To: *s.Percent})
		}
	}
	return out
}

// daysToEmpty converts a current percentage and rate into days.
func daysToEmpty(pct *int, filled bool, r Rate) *float64 {
	if pct == nil || !r.Known || r.PercentPerDay <= 0 {
		return nil
	}
	remaining := float64(*pct)
	if filled {
		remaining = 100 - remaining
	}
	d := remaining / r.PercentPerDay
	d = math.Round(d*10) / 10
	return &d
}

// forecastWindow is how much telemetry history the rate is computed over.
const forecastWindow = 30 * 24 * time.Hour
