package alerts

import (
	"fmt"
	"strings"

	"printmarshal/internal/engine"
	"printmarshal/internal/model"
)

// Thresholds configure the rules.
type Thresholds struct {
	SupplyWarningPercent  int
	SupplyCriticalPercent int
	OfflineAfterFailures  int
}

// Desired is an alert the rules want active after a poll.
type Desired struct {
	Key         string
	Source      string
	Severity    model.Severity
	Description string
}

// Sources lists the alert sources the poll rules own; alerts of these sources
// that are no longer desired after a poll are cleared, but only for the
// sources the poll actually observed (see Evaluate).
var Sources = []string{"offline", "poll", "supply"}

// Evaluate derives the alerts a device should have from its latest poll
// (spec §8.3 alert model and noise rules). observed lists the alert sources
// the poll was able to judge: "offline" always, "poll" only when the status
// read succeeded and "supply" only when the supplies read succeeded. A fault
// alert is cleared only after a successful observation shows it resolved
// (code review H-012); an unreachable or partial poll leaves the others as
// they are instead of reporting them cleared.
func Evaluate(d *model.Device, res *engine.PollResult, failures int, t Thresholds) (out []Desired, observed []string) {
	observed = []string{"offline"}
	if !res.Reachable {
		if failures >= t.OfflineAfterFailures {
			out = append(out, Desired{Key: "offline", Source: "offline", Severity: model.SeverityCritical,
				Description: fmt.Sprintf("Device unreachable (%d consecutive failed polls)", failures)})
		}
		return out, observed
	}
	if res.Status != nil {
		observed = append(observed, "poll")
	}
	if res.Supplies != nil {
		observed = append(observed, "supply")
	}

	if st := res.Status; st != nil {
		if st.State == "down" {
			out = append(out, Desired{Key: "state:down", Source: "poll", Severity: model.SeverityCritical, Description: "Device reports it is down"})
		}
		seen := map[string]bool{}
		for _, e := range st.Errors {
			sev, desc := errorSeverity(e)
			if sev == model.SeverityOK || seen[e] {
				continue
			}
			seen[e] = true
			out = append(out, Desired{Key: "error:" + e, Source: "poll", Severity: sev, Description: desc})
		}
		for _, a := range st.Alerts {
			if a.Severity == model.SeverityOK {
				continue
			}
			key := fmt.Sprintf("device:%s:%s:%d", a.Group, a.Code, a.GroupIndex)
			if a.Code == "" {
				key = fmt.Sprintf("device:%s:%d", strings.ToLower(a.Description), a.Index)
			}
			desc := a.Description
			if a.Group != "" && a.GroupIndex > 0 {
				desc = fmt.Sprintf("%s (%s %d)", a.Description, a.Group, a.GroupIndex)
			}
			out = append(out, Desired{Key: key, Source: "poll", Severity: a.Severity, Description: desc})
		}
	}

	// Supplies: only consumables with a known percentage.
	for _, s := range res.Supplies {
		if s.Percent == nil || s.Class == "other" {
			continue
		}
		pct := *s.Percent
		key := fmt.Sprintf("supply:%d", s.Index)
		name := s.Description
		if name == "" {
			name = fmt.Sprintf("supply %d", s.Index)
		}
		if s.Class == "filled" { // waste receptacle: alert when nearly full
			remaining := 100 - pct
			switch {
			case remaining <= t.SupplyCriticalPercent:
				out = append(out, Desired{Key: key, Source: "supply", Severity: model.SeverityCritical, Description: fmt.Sprintf("%s is full (%d%%)", name, pct)})
			case remaining <= t.SupplyWarningPercent:
				out = append(out, Desired{Key: key, Source: "supply", Severity: model.SeverityWarning, Description: fmt.Sprintf("%s is nearly full (%d%%)", name, pct)})
			}
			continue
		}
		switch {
		case pct <= t.SupplyCriticalPercent:
			out = append(out, Desired{Key: key, Source: "supply", Severity: model.SeverityCritical, Description: fmt.Sprintf("%s at %d%%", name, pct)})
		case pct <= t.SupplyWarningPercent:
			out = append(out, Desired{Key: key, Source: "supply", Severity: model.SeverityWarning, Description: fmt.Sprintf("%s low (%d%%)", name, pct)})
		}
	}

	// Paper: raise a fleet-level "no paper" only when every input tray of a media
	// size is empty; per-tray state stays visible on the device (spec §8.3). "Any"
	// is not a size (#54): those trays and the ones with no known size raise only
	// when every eligible tray of the printer is empty.
	everyTrayEmpty := true
	bySize := map[string][]model.Tray{}
	for _, t := range res.Trays {
		if t.Kind != "input" || t.Type == "sheetFeedManual" {
			continue
		}
		size := t.MediaSize
		if size == "" {
			size = t.MediaName
		}
		if size == "" || strings.EqualFold(size, "any") {
			size = "*"
		}
		if t.Status != "empty" {
			everyTrayEmpty = false
		}
		bySize[size] = append(bySize[size], t)
	}
	for size, trays := range bySize {
		if size == "*" && !everyTrayEmpty {
			continue
		}
		allEmpty := len(trays) > 0
		for _, t := range trays {
			if t.Status != "empty" {
				allEmpty = false
				break
			}
		}
		if allEmpty {
			label := "all input trays"
			if size != "*" {
				label = "all " + size + " trays"
			}
			out = append(out, Desired{Key: "paper:" + size, Source: "poll", Severity: model.SeverityWarning, Description: "Out of paper: " + label + " are empty"})
		}
	}
	return out, observed
}

// errorSeverity classifies hrPrinterDetectedErrorState bits and IPP printer-state-reasons.
func errorSeverity(e string) (model.Severity, string) {
	switch e {
	case "noPaper", "media-empty-error", "media-empty":
		return model.SeverityOK, "" // covered by the paper rule using tray state
	case "noToner", "toner-empty-error", "marker-supply-empty-error":
		return model.SeverityCritical, "Out of toner"
	case "doorOpen", "door-open-error", "cover-open-error", "cover-open":
		return model.SeverityCritical, "Door or cover open"
	case "jammed", "media-jam-error", "media-jam":
		return model.SeverityCritical, "Paper jam"
	case "offline":
		return model.SeverityCritical, "Printer is offline"
	case "serviceRequested":
		return model.SeverityCritical, "Service requested"
	case "markerSupplyMissing", "marker-supply-missing-error":
		return model.SeverityCritical, "Marker supply missing"
	case "inputTrayMissing", "input-tray-missing-error":
		return model.SeverityCritical, "Input tray missing"
	case "outputFull", "output-area-full-error":
		return model.SeverityCritical, "Output tray full"
	case "lowToner", "toner-low-warning", "marker-supply-low-warning", "toner-low":
		return model.SeverityOK, "" // supply rule covers it with real percentages
	case "lowPaper", "media-low-warning", "media-low":
		return model.SeverityWarning, "Paper low"
	case "outputNearFull", "output-area-almost-full-warning":
		return model.SeverityWarning, "Output tray nearly full"
	case "outputTrayMissing", "output-tray-missing-error":
		return model.SeverityWarning, "Output tray missing"
	case "inputTrayEmpty":
		return model.SeverityOK, ""
	case "overduePreventMaint":
		return model.SeverityWarning, "Preventive maintenance overdue"
	}
	switch {
	case strings.HasSuffix(e, "-error"):
		return model.SeverityCritical, strings.ReplaceAll(strings.TrimSuffix(e, "-error"), "-", " ")
	case strings.HasSuffix(e, "-warning"):
		return model.SeverityWarning, strings.ReplaceAll(strings.TrimSuffix(e, "-warning"), "-", " ")
	}
	return model.SeverityOK, ""
}

// ConsumableSource is the alert source for stock alerts; they are global (no device).
const ConsumableSource = "consumable"

// ConsumableDesired evaluates stock thresholds (consumables spec §3): critical
// when out or at/below the critical threshold, warning at/below the warning
// threshold. Keys are stable per item so an alert raises once and clears when
// stock is replenished.
func ConsumableDesired(items []*model.Consumable) []Desired {
	var out []Desired
	for _, it := range items {
		key := "consumable:" + it.ID
		label := it.Name
		if it.PartNumber != "" {
			label += " (" + it.PartNumber + ")"
		}
		switch it.Status() {
		case model.StockOut:
			out = append(out, Desired{Key: key, Source: ConsumableSource, Severity: model.SeverityCritical, Description: fmt.Sprintf("Out of stock: %s", label)})
		case model.StockCritical:
			out = append(out, Desired{Key: key, Source: ConsumableSource, Severity: model.SeverityCritical, Description: fmt.Sprintf("Stock of %s is critical (%d %s left)", label, it.OnHand, unit(it))})
		case model.StockLow:
			out = append(out, Desired{Key: key, Source: ConsumableSource, Severity: model.SeverityWarning, Description: fmt.Sprintf("Stock of %s is low (%d %s left)", label, it.OnHand, unit(it))})
		}
	}
	return out
}

func unit(c *model.Consumable) string {
	if c.Unit == "" {
		return "each"
	}
	return c.Unit
}
