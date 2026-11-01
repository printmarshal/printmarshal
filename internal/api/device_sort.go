package api

import (
	"cmp"
	"net/netip"
	"slices"
	"strings"

	"printmarshal/internal/firmware/fwversion"
	"printmarshal/internal/model"
)

// deviceSort is how one sort key orders devices. compare is ascending and is
// only called when both devices have a value; a device without one stays last
// in either direction. naturalDesc is the order a person expects first: the
// most pages, the most recently seen.
type deviceSort struct {
	has         func(*model.Device) bool
	compare     func(a, b *model.Device) int
	naturalDesc bool
}

func hasText(f func(*model.Device) string) func(*model.Device) bool {
	return func(d *model.Device) bool { return f(d) != "" }
}

func byText(f func(*model.Device) string) func(a, b *model.Device) int {
	return func(a, b *model.Device) int { return cmp.Compare(strings.ToLower(f(a)), strings.ToLower(f(b))) }
}

func deviceModel(d *model.Device) string    { return d.Model }
func deviceLocation(d *model.Device) string { return d.Location }
func deviceFirmware(d *model.Device) string { return d.Firmware }
func deviceState(d *model.Device) string    { return string(d.State) }

// lowestSupply is the device's lowest remaining level: the table's supply
// column turns on the same number. A receptacle that fills up has what is left.
func lowestSupply(d *model.Device) (int, bool) {
	lowest, found := 0, false
	for _, s := range d.Supplies {
		if s.Percent == nil || s.Class == "other" {
			continue
		}
		left := *s.Percent
		if s.Class == "filled" {
			left = 100 - left
		}
		if !found || left < lowest {
			lowest, found = left, true
		}
	}
	return lowest, found
}

// compareAddress puts IP addresses in numeric order, then names in text order.
func compareAddress(a, b string) int {
	ipA, errA := netip.ParseAddr(a)
	ipB, errB := netip.ParseAddr(b)
	switch {
	case errA == nil && errB == nil:
		return ipA.Compare(ipB)
	case errA == nil:
		return -1
	case errB == nil:
		return 1
	}
	return cmp.Compare(strings.ToLower(a), strings.ToLower(b))
}

// severityRank matches the store's severity order: the worst devices first.
func severityRank(s model.Severity) int {
	switch s {
	case model.SeverityCritical:
		return 0
	case "offline":
		return 1
	case model.SeverityWarning:
		return 2
	}
	return 3
}

var deviceSorts = map[string]deviceSort{
	"name": {compare: func(a, b *model.Device) int {
		return cmp.Compare(strings.ToLower(a.Name()), strings.ToLower(b.Name()))
	}},
	"address":  {has: hasText(func(d *model.Device) string { return d.Address }), compare: func(a, b *model.Device) int { return compareAddress(a.Address, b.Address) }},
	"model":    {has: hasText(deviceModel), compare: byText(deviceModel)},
	"location": {has: hasText(deviceLocation), compare: byText(deviceLocation)},
	"firmware": {has: hasText(deviceFirmware), compare: func(a, b *model.Device) int { return fwversion.Compare(a.Firmware, b.Firmware) }},
	"state":    {has: hasText(deviceState), compare: byText(deviceState)},
	"severity": {compare: func(a, b *model.Device) int { return cmp.Compare(severityRank(a.Severity), severityRank(b.Severity)) }},
	"supplies": {
		has: func(d *model.Device) bool { _, ok := lowestSupply(d); return ok },
		compare: func(a, b *model.Device) int {
			la, _ := lowestSupply(a)
			lb, _ := lowestSupply(b)
			return cmp.Compare(la, lb)
		},
	},
	"pages": {
		has:         func(d *model.Device) bool { return d.Counters != nil },
		compare:     func(a, b *model.Device) int { return cmp.Compare(a.Counters.Total, b.Counters.Total) },
		naturalDesc: true,
	},
	"lastSeen": {
		has:         func(d *model.Device) bool { return !d.LastSeen.IsZero() },
		compare:     func(a, b *model.Device) int { return a.LastSeen.Compare(b.LastSeen) },
		naturalDesc: true,
	},
}

// sortDevices orders a device list for the table's headers and the export. dir
// is "asc" or "desc"; empty means the key's natural order. With neither a key
// nor a direction, the store's order stands. Ties go by name, then id, so the
// order is the same on every read.
func sortDevices(devs []*model.Device, key, dir string) {
	if key == "" {
		if dir == "" {
			return
		}
		key = "name"
	}
	sorter, ok := deviceSorts[key]
	if !ok {
		return
	}
	desc := sorter.naturalDesc
	switch dir {
	case "asc":
		desc = false
	case "desc":
		desc = true
	}
	has := sorter.has
	if has == nil {
		has = func(*model.Device) bool { return true }
	}
	slices.SortStableFunc(devs, func(a, b *model.Device) int {
		hasA, hasB := has(a), has(b)
		if hasA != hasB {
			if hasA {
				return -1
			}
			return 1
		}
		if hasA {
			c := sorter.compare(a, b)
			if desc {
				c = -c
			}
			if c != 0 {
				return c
			}
		}
		if c := cmp.Compare(strings.ToLower(a.Name()), strings.ToLower(b.Name())); c != 0 {
			return c
		}
		return cmp.Compare(a.ID, b.ID)
	})
}

// pageDevices applies a filter's limit and offset to an already sorted list.
func pageDevices(devs []*model.Device, limit, offset int) []*model.Device {
	if offset > 0 {
		if offset >= len(devs) {
			return []*model.Device{}
		}
		devs = devs[offset:]
	}
	if limit > 0 && limit < len(devs) {
		devs = devs[:limit]
	}
	return devs
}
