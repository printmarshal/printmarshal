package alerts

import (
	"strings"
	"testing"

	"printmarshal/internal/engine"
	"printmarshal/internal/model"
)

func pct(n int) *int { return &n }

func TestEvaluate(t *testing.T) {
	th := Thresholds{SupplyWarningPercent: 15, SupplyCriticalPercent: 5, OfflineAfterFailures: 2}
	dev := &model.Device{ID: "d1"}
	res := &engine.PollResult{Reachable: true,
		Status:   &model.Status{State: "idle", Errors: []string{"doorOpen", "lowToner"}, Alerts: []model.DeviceAlert{{Code: "jam", Group: "mediaPath", GroupIndex: 2, Severity: model.SeverityCritical, Description: "Jam"}}},
		Supplies: []model.Supply{{Index: 1, Description: "Black", Class: "consumed", Percent: pct(3)}, {Index: 2, Description: "Cyan", Class: "consumed", Percent: pct(10)}, {Index: 3, Description: "Waste", Class: "filled", Percent: pct(97)}, {Index: 4, Description: "Unknown", Class: "consumed"}},
		Trays:    []model.Tray{{Kind: "input", Name: "Tray 1", MediaSize: "210x297mm", Status: "empty"}, {Kind: "input", Name: "Tray 2", MediaSize: "210x297mm", Status: "empty"}, {Kind: "input", Name: "Tray 3", MediaSize: "216x279mm", Status: "ok"}},
	}
	got := map[string]Desired{}
	desired, observed := Evaluate(dev, res, 0, th)
	for _, d := range desired {
		got[d.Key] = d
	}
	if strings.Join(observed, ",") != "offline,poll,supply" {
		t.Errorf("observed = %v", observed)
	}
	if got["error:doorOpen"].Severity != model.SeverityCritical {
		t.Errorf("doorOpen: %+v", got["error:doorOpen"])
	}
	if _, ok := got["error:lowToner"]; ok {
		t.Error("lowToner bit should defer to supply percentages")
	}
	if got["device:mediaPath:jam:2"].Severity != model.SeverityCritical {
		t.Errorf("device alert missing: %+v", got)
	}
	if got["supply:1"].Severity != model.SeverityCritical || got["supply:2"].Severity != model.SeverityWarning || got["supply:3"].Severity != model.SeverityCritical {
		t.Errorf("supplies: %+v", got)
	}
	if _, ok := got["supply:4"]; ok {
		t.Error("unknown level must not alert")
	}
	if _, ok := got["paper:210x297mm"]; !ok {
		t.Error("all A4 trays empty should raise")
	}
	if _, ok := got["paper:216x279mm"]; ok {
		t.Error("letter tray is fine")
	}

	off, observed := Evaluate(dev, &engine.PollResult{Reachable: false}, 1, th)
	if len(off) != 0 {
		t.Errorf("one failure should not alert yet: %+v", off)
	}
	if strings.Join(observed, ",") != "offline" {
		t.Errorf("an unreachable poll must only own offline, got %v", observed)
	}
	off, _ = Evaluate(dev, &engine.PollResult{Reachable: false}, 2, th)
	if len(off) != 1 || off[0].Key != "offline" {
		t.Errorf("offline: %+v", off)
	}
	// A partial poll (counters answered, status and supplies did not) owns
	// neither poll nor supply alerts, so existing faults are not cleared.
	_, observed = Evaluate(dev, &engine.PollResult{Reachable: true, Counters: &model.Counters{Total: 1}}, 0, th)
	if strings.Join(observed, ",") != "offline" {
		t.Errorf("partial poll observed %v", observed)
	}
}

func TestConsumableDesired(t *testing.T) {
	items := []*model.Consumable{
		{ID: "a", Name: "Black toner", PartNumber: "TN-1", OnHand: 0, WarningAt: 3, CriticalAt: 1},
		{ID: "b", Name: "Cyan toner", OnHand: 1, WarningAt: 3, CriticalAt: 1},
		{ID: "c", Name: "Drum", OnHand: 3, WarningAt: 3, CriticalAt: 1},
		{ID: "d", Name: "Fuser", OnHand: 9, WarningAt: 3, CriticalAt: 1},
		{ID: "e", Name: "No thresholds", OnHand: 2},
	}
	got := map[string]Desired{}
	for _, d := range ConsumableDesired(items) {
		got[d.Key] = d
		if d.Source != ConsumableSource {
			t.Errorf("source = %q", d.Source)
		}
	}
	if got["consumable:a"].Severity != model.SeverityCritical || !strings.Contains(got["consumable:a"].Description, "Out of stock") || !strings.Contains(got["consumable:a"].Description, "TN-1") {
		t.Errorf("out: %+v", got["consumable:a"])
	}
	if got["consumable:b"].Severity != model.SeverityCritical {
		t.Errorf("critical threshold: %+v", got["consumable:b"])
	}
	if got["consumable:c"].Severity != model.SeverityWarning {
		t.Errorf("warning threshold: %+v", got["consumable:c"])
	}
	if _, ok := got["consumable:d"]; ok {
		t.Error("healthy stock must not alert")
	}
	if _, ok := got["consumable:e"]; ok {
		t.Error("stock above zero with no thresholds must not alert")
	}
}

// #54: "Any" is not a media size. An empty Any tray is not out of paper while
// another tray has some, and the alert never reads "all Any trays".
func TestPaperRuleTreatsAnyAsNoSize(t *testing.T) {
	th := Thresholds{SupplyWarningPercent: 20, SupplyCriticalPercent: 5, OfflineAfterFailures: 3}
	eval := func(trays ...model.Tray) map[string]Desired {
		out, _ := Evaluate(&model.Device{}, &engine.PollResult{Reachable: true, Trays: trays}, 0, th)
		got := map[string]Desired{}
		for _, d := range out {
			got[d.Key] = d
		}
		return got
	}
	anyEmpty := model.Tray{Kind: "input", Index: 1, MediaSize: "Any", Status: "empty"}
	a4OK := model.Tray{Kind: "input", Index: 2, MediaSize: "A4", Status: "ok"}
	a4Empty := model.Tray{Kind: "input", Index: 2, MediaSize: "A4", Status: "empty"}
	noSizeEmpty := model.Tray{Kind: "input", Index: 3, Status: "empty"}

	if got := eval(anyEmpty, a4OK); len(got) != 0 {
		t.Errorf("A4 trays have paper: %+v", got)
	}
	if got := eval(noSizeEmpty, a4OK); len(got) != 0 {
		t.Errorf("a tray of unknown size is not every tray: %+v", got)
	}
	got := eval(anyEmpty, a4Empty)
	if d, ok := got["paper:*"]; !ok || d.Description != "Out of paper: all input trays are empty" {
		t.Errorf("every tray is empty: %+v", got)
	}
	if _, ok := got["paper:Any"]; ok {
		t.Errorf("Any is not a size: %+v", got)
	}
	if got := eval(noSizeEmpty); got["paper:*"].Description != "Out of paper: all input trays are empty" {
		t.Errorf("one tray of unknown size, empty: %+v", got)
	}
}
