// Package consumables implements the consumables add-on (docs/consumables-feature-spec.md):
// matching installed supplies to stock items, consumption forecasting,
// replacement detection and the stock bookkeeping around them. It reads device
// telemetry that polls already produce and never touches a device.
package consumables

import (
	"fmt"
	"strings"

	"printmarshal/internal/model"
)

// SupplyType maps a driver-reported supply type (Printer MIB
// PrtMarkerSuppliesTypeTC names, PWG 5100.13 strings) to a consumable type.
func SupplyType(t string) model.ConsumableType {
	switch strings.ToLower(strings.TrimSpace(t)) {
	case "toner", "tonercartridge", "toner-cartridge":
		return model.ConsumableToner
	case "ink", "inkcartridge", "ink-cartridge", "inkribbon", "ink-ribbon", "solidwax", "solid-wax":
		return model.ConsumableInk
	case "opc", "drum", "developer", "imagingunit", "imaging-unit", "photoconductor":
		return model.ConsumableDrum
	case "fuser", "fuseroil", "fuser-oil", "fuser-oil-wick", "fuser-cleaning-pad", "transferunit", "transfer-unit", "transferroller":
		return model.ConsumableFuser
	case "wastetoner", "waste-toner", "wasteink", "waste-ink", "wastewax", "waste-wax", "wastewater", "waste-water", "cleanerunit", "cleaner-unit":
		return model.ConsumableWaste
	case "staples", "stapler":
		return model.ConsumableStaples
	case "maintenancekit", "maintenance-kit", "matte-kit":
		return model.ConsumableMaintenanceKit
	}
	return model.ConsumableOther
}

var knownColors = []string{"cyan", "magenta", "yellow", "black", "light cyan", "light magenta", "gray", "grey", "photo black", "matte black", "red", "green", "blue", "orange", "white", "clear"}

// SupplyColor derives a normalised colour from the colorant, falling back to
// the description. Returns "" when none is recognisable (drums, fusers, kits).
func SupplyColor(s model.Supply) string {
	for _, src := range []string{s.Colorant, s.Description} {
		n := strings.ToLower(strings.TrimSpace(src))
		if n == "" {
			continue
		}
		// Longer names first so "light cyan" wins over "cyan".
		for _, c := range []string{"light cyan", "light magenta", "photo black", "matte black"} {
			if strings.Contains(n, c) {
				return c
			}
		}
		for _, c := range knownColors {
			if strings.Contains(n, c) {
				if c == "grey" {
					return "gray"
				}
				return c
			}
		}
	}
	return ""
}

// NormalizeColor canonicalises an operator-entered colour (C/M/Y/K shorthands).
func NormalizeColor(c string) string {
	n := strings.ToLower(strings.TrimSpace(c))
	switch n {
	case "c", "cyan":
		return "cyan"
	case "m", "magenta":
		return "magenta"
	case "y", "yellow":
		return "yellow"
	case "k", "black", "bk":
		return "black"
	case "grey":
		return "gray"
	}
	return n
}

// ModelMatches reports whether any compatible-model entry names the device's
// make/model (case-insensitive substring either way).
func ModelMatches(compat []string, dev *model.Device) bool {
	if dev == nil {
		return false
	}
	full := strings.ToLower(strings.TrimSpace(dev.Make + " " + dev.Model))
	mdl := strings.ToLower(strings.TrimSpace(dev.Model))
	for _, m := range compat {
		m = strings.ToLower(strings.TrimSpace(m))
		if m == "" {
			continue
		}
		if m == "*" || strings.Contains(full, m) || (mdl != "" && strings.Contains(m, mdl)) {
			return true
		}
	}
	return false
}

// Match resolves an installed supply to a stock item (consumables spec §3):
// primarily by part number appearing in the supply's description, else by
// type + colour + compatible model. An item without compatible models matches
// on type + colour only when it is the sole candidate, so ambiguous fleets
// never link silently. Returns nil when nothing matches.
func Match(items []*model.Consumable, dev *model.Device, s model.Supply) *model.Consumable {
	desc := strings.ToLower(s.Description)
	// 1. Part number.
	var byPart []*model.Consumable
	for _, it := range items {
		pn := strings.ToLower(strings.TrimSpace(it.PartNumber))
		if pn != "" && desc != "" && strings.Contains(desc, pn) {
			byPart = append(byPart, it)
		}
	}
	if len(byPart) == 1 {
		return byPart[0]
	}
	if len(byPart) > 1 {
		for _, it := range byPart {
			if ModelMatches(it.CompatibleModels, dev) {
				return it
			}
		}
		return byPart[0]
	}
	// 2. Type + colour + model.
	typ := SupplyType(s.Type)
	color := SupplyColor(s)
	var withModel, anyModel []*model.Consumable
	for _, it := range items {
		if it.Type != typ {
			continue
		}
		ic := NormalizeColor(it.Color)
		if ic != "" && ic != color {
			continue
		}
		if ic == "" && color != "" && (typ == model.ConsumableToner || typ == model.ConsumableInk) {
			// A colourless toner/ink item only fits colourless (mono) supplies.
			continue
		}
		if len(it.CompatibleModels) == 0 {
			anyModel = append(anyModel, it)
			continue
		}
		if ModelMatches(it.CompatibleModels, dev) {
			withModel = append(withModel, it)
		}
	}
	if len(withModel) > 0 {
		return withModel[0]
	}
	if len(anyModel) == 1 {
		return anyModel[0]
	}
	return nil
}

// Suggest builds a new stock item pre-filled from an unmatched installed supply.
func Suggest(dev *model.Device, s model.Supply, defaults model.ConsumableSettings) model.Consumable {
	typ := SupplyType(s.Type)
	color := SupplyColor(s)
	name := strings.TrimSpace(s.Description)
	if name == "" {
		name = strings.TrimSpace(strings.Title(color) + " " + strings.ReplaceAll(string(typ), "_", " "))
	}
	var compat []string
	if dev != nil && dev.Model != "" {
		compat = []string{dev.Model}
	}
	c := model.Consumable{Name: name, Type: typ, Color: color, CompatibleModels: compat, Unit: "each",
		WarningAt: defaults.DefaultWarningAt, CriticalAt: defaults.DefaultCriticalAt}
	if compat == nil {
		c.CompatibleModels = []string{}
	}
	return c
}

// SlotLabel is the telemetry label polls record for a supply ("index:description").
func SlotLabel(s model.Supply) string { return fmt.Sprintf("%d:%s", s.Index, s.Description) }
