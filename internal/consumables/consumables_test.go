package consumables

import (
	"testing"
	"time"

	"printmarshal/internal/model"
)

func pct(n int) *int { return &n }

func TestMatch(t *testing.T) {
	dev := &model.Device{Make: "Acme", Model: "LaserPrint 4200dn"}
	byPart := &model.Consumable{ID: "part", Name: "Toner TN-4200", Type: model.ConsumableToner, Color: "black", PartNumber: "TN-4200"}
	blackForModel := &model.Consumable{ID: "kmodel", Type: model.ConsumableToner, Color: "K", CompatibleModels: []string{"laserprint 4200"}}
	blackOther := &model.Consumable{ID: "kother", Type: model.ConsumableToner, Color: "black", CompatibleModels: []string{"Other 9000"}}
	cyanAny := &model.Consumable{ID: "cany", Type: model.ConsumableToner, Color: "cyan"}
	drumAny := &model.Consumable{ID: "drum", Type: model.ConsumableDrum}
	drumAny2 := &model.Consumable{ID: "drum2", Type: model.ConsumableDrum}
	items := []*model.Consumable{blackOther, blackForModel, byPart, cyanAny, drumAny, drumAny2}

	if m := Match(items, dev, model.Supply{Description: "Black Toner Cartridge tn-4200", Type: "toner", Colorant: "black"}); m == nil || m.ID != "part" {
		t.Errorf("part number should win: %+v", m)
	}
	if m := Match(items, dev, model.Supply{Description: "Black Toner Cartridge", Type: "toner", Colorant: "black"}); m == nil || m.ID != "kmodel" {
		t.Errorf("type+colour+model fallback: %+v", m)
	}
	if m := Match(items, dev, model.Supply{Description: "Cyan Toner", Type: "toner"}); m == nil || m.ID != "cany" {
		t.Errorf("single colour candidate without models should match: %+v", m)
	}
	if m := Match(items, dev, model.Supply{Description: "Imaging Drum", Type: "opc"}); m != nil {
		t.Errorf("two model-less drums are ambiguous, must not link: %+v", m)
	}
	if m := Match(items, dev, model.Supply{Description: "Magenta Toner", Type: "toner", Colorant: "magenta"}); m != nil {
		t.Errorf("no magenta item: %+v", m)
	}
	if m := Match(items, &model.Device{Model: "Other 9000"}, model.Supply{Description: "Black Toner", Type: "toner", Colorant: "black"}); m == nil || m.ID != "kother" {
		t.Errorf("other model: %+v", m)
	}
	if SupplyType("wasteToner") != model.ConsumableWaste || SupplyType("opc") != model.ConsumableDrum || SupplyType("inkCartridge") != model.ConsumableInk {
		t.Error("supply type mapping")
	}
	if SupplyColor(model.Supply{Description: "Light Cyan Ink"}) != "light cyan" || SupplyColor(model.Supply{Description: "Fuser Kit"}) != "" {
		t.Error("supply colour")
	}
	sug := Suggest(dev, model.Supply{Description: "Black Toner Cartridge", Type: "toner", Colorant: "black"}, model.DefaultConsumableSettings())
	if sug.Name != "Black Toner Cartridge" || sug.Type != model.ConsumableToner || sug.Color != "black" || len(sug.CompatibleModels) != 1 || sug.WarningAt != 2 {
		t.Errorf("suggest = %+v", sug)
	}
}

func series(start time.Time, stepHours float64, values ...float64) []model.TelemetrySample {
	var out []model.TelemetrySample
	for i, v := range values {
		out = append(out, model.TelemetrySample{DeviceID: "d", Metric: "supply.percent", Label: "1:Black", Value: v, At: start.Add(time.Duration(float64(i) * stepHours * float64(time.Hour)))})
	}
	return out
}

func TestConsumptionRateAndProjection(t *testing.T) {
	t0 := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	// 2% per day over 7 days with one replacement jump in the middle.
	s := series(t0, 24, 60, 58, 56, 54, 100, 98, 96, 94)
	r := ConsumptionRate(s, false, 30, 3)
	if !r.Known || r.PercentPerDay < 1.99 || r.PercentPerDay > 2.01 {
		t.Errorf("rate = %+v", r)
	}
	if r := ConsumptionRate(series(t0, 1, 60, 59, 58, 57, 56), false, 30, 3); r.Known || r.Reason != "history too short" {
		t.Errorf("thin history must be unknown: %+v", r)
	}
	if r := ConsumptionRate(series(t0, 24, 60, 60, 60, 60, 60), false, 30, 3); r.Known || r.Reason != "no consumption observed" {
		t.Errorf("flat series must be unknown: %+v", r)
	}
	if r := ConsumptionRate(series(t0, 24, 60, 58), false, 30, 3); r.Known {
		t.Errorf("two samples must be unknown: %+v", r)
	}
	// Receptacles fill up: level rises 2%/day.
	if r := ConsumptionRate(series(t0, 24, 10, 12, 14, 16, 18), true, 30, 3); !r.Known || r.PercentPerDay < 1.99 || r.PercentPerDay > 2.01 {
		t.Errorf("filled rate = %+v", r)
	}

	item := &model.Consumable{ID: "c", Name: "Black", OnHand: 2, WarningAt: 1}
	d1 := DeviceForecast{DeviceID: "a", Percent: pct(20), Rate: Rate{Known: true, PercentPerDay: 2}}
	d1.DaysToEmpty = daysToEmpty(d1.Percent, false, d1.Rate) // 10 days, then every 50 days
	d2 := DeviceForecast{DeviceID: "b", Percent: pct(50), Rate: Rate{Known: true, PercentPerDay: 5}}
	d2.DaysToEmpty = daysToEmpty(d2.Percent, false, d2.Rate) // 10 days, then every 20 days
	unknown := DeviceForecast{DeviceID: "u", Percent: pct(50)}
	f := Project(item, []DeviceForecast{d1, d2, unknown}, 14)
	if !f.Known || f.StockDays == nil || *f.StockDays != 10 || !f.ReorderSoon || f.NextNeed == nil || *f.NextNeed != 10 || f.LinkedCount != 3 {
		t.Errorf("projection = %+v", f)
	}
	item.OnHand = 3 // third replacement: device b again at day 30
	f = Project(item, []DeviceForecast{d1, d2}, 14)
	if f.StockDays == nil || *f.StockDays != 30 || f.ReorderSoon {
		t.Errorf("third unit = %+v", f)
	}
	item.OnHand = 0
	f = Project(item, []DeviceForecast{d1}, 14)
	if f.StockDays == nil || *f.StockDays != 0 || !f.ReorderSoon {
		t.Errorf("out of stock = %+v", f)
	}
	if f := Project(item, []DeviceForecast{unknown}, 14); f.Known || f.Reason == "" {
		t.Errorf("all unknown = %+v", f)
	}
	if f := Project(item, nil, 14); f.Known || f.Reason != "no linked devices" || f.Devices == nil {
		t.Errorf("no devices = %+v", f)
	}
}

func TestDetectReplacements(t *testing.T) {
	prev := []model.Supply{{Index: 1, Description: "Black", Class: "consumed", Percent: pct(8)}, {Index: 2, Description: "Drum", Class: "consumed", Percent: pct(40)},
		{Index: 3, Description: "Waste", Class: "filled", Percent: pct(95)}, {Index: 4, Description: "Unknown", Class: "consumed"}}
	cur := []model.Supply{{Index: 1, Description: "Black", Class: "consumed", Percent: pct(100)}, {Index: 2, Description: "Drum", Class: "consumed", Percent: pct(45)},
		{Index: 3, Description: "Waste", Class: "filled", Percent: pct(2)}, {Index: 4, Description: "Unknown", Class: "consumed", Percent: pct(100)}, {Index: 5, Description: "New slot", Percent: pct(100)}}
	reps := DetectReplacements(prev, cur, 30)
	if len(reps) != 2 || reps[0].Index != 1 || reps[0].From != 8 || reps[0].To != 100 || reps[1].Index != 3 {
		t.Errorf("replacements = %+v", reps)
	}
}
